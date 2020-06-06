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

	test := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := bytes.Repeat([]byte{0, 1}, 32) // 64B buffer
			input <- buf

			tmp := bytes.Repeat([]byte{0, 1}, 32) // 64B buffer
			result := []byte{}
			for len(tmp) > 0 {
				out := <-output
				result = append(result, out...)

				if len(out) < len(tmp) {
					tmp = tmp[len(out):]
				} else {
					break
				}
			}

			if !bytes.Equal(result, buf) {
				b.Logf("buffers were not equal: %v %v", buf, result)
				b.FailNow()
			}
		}
	}

	b.Run("default allocation", test)

	for _, size := range []int{64, 256, 1024, 4096, 32768} {
		b.Run(fmt.Sprintf("%d allocation", size), func(b *testing.B) {
			p.SetReadBufSize(size)
			test(b)
		})
	}
}
