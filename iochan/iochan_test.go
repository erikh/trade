package iochan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

func assertOut(t *testing.T, output chan []byte, checkOut string) {
	after := time.After(time.Second)

	select {
	case out := <-output:
		if string(out) != checkOut {
			t.Fatalf("assertOut: response did not match expectation: was %q, expected %q", out, checkOut)
		}
	case <-after:
		t.Fatalf("assertOut: timeout waiting for %q", checkOut)
	}
}

func makeIO(p *Proxy) (io.ReadCloser, io.WriteCloser) {
	rr, rw := io.Pipe()
	wr, ww := io.Pipe()

	p.Reader = rr
	p.Writer = ww

	return wr, rw
}

func TestProxy(t *testing.T) {
	input, output := make(chan []byte), make(chan []byte)
	p := &Proxy{
		Input:  input,
		Output: output,
	}

	r, w := makeIO(p)
	defer w.Close()
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)

	io.WriteString(w, "hello")
	assertOut(t, output, "hello")
	input <- []byte("hello")
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatal("could not read")
	}
	if n != 5 {
		t.Fatal("short read")
	}

	if string(buf) != "hello" {
		t.Fatal("hello was not returned")
	}

	go io.Copy(w, r)

	input <- []byte("hello")
	assertOut(t, output, "hello")
}

func BenchmarkProxy(b *testing.B) {
	input, output := make(chan []byte), make(chan []byte)
	p := &Proxy{
		Input:  input,
		Output: output,
	}

	r, w := makeIO(p)
	defer w.Close()
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)

	go io.Copy(w, r)

	test := func(b *testing.B, size int) {
		buf := bytes.Repeat([]byte{0, 1}, size) // size*2 buffer

		for i := 0; i < b.N; i++ {
			input <- buf

			// this keeps the first and last byte of the response, respectively, so
			// they can be compared later. This avoids a nasty array
			// traversal+compare during the benchmark, keeping numbers relatively
			// even for different buffer sizes.
			var resultPair [2]byte
			for i := 0; i < size*2; i++ {
				out := <-output
				if i == 0 {
					resultPair[0] = out[0]
				}

				resultPair[1] = out[len(out)-1]
				i += len(out) - 1
			}

			if resultPair[0] != 0 || resultPair[1] != 1 {
				b.Log("buffers were not equal")
				b.FailNow()
			}
		}
	}

	b.Run("default allocation", func(b *testing.B) { test(b, defaultReadBufSize) })

	for _, size := range []int{64, 256, 1024, 4096} {
		b.Run(fmt.Sprintf("%d allocation", size), func(b *testing.B) {
			p.SetReadBufSize(size)
			test(b, size)
		})
	}
}
