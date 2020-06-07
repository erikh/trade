package menu

import (
	"context"
	"fmt"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func assertOut(t *testing.T, output chan []byte, checkOut string) {
	after := time.After(time.Second)

	_, file, line, ok := runtime.Caller(1)
	if !ok {
		logrus.Error("Could not retrieve caller info")
	}

	from := fmt.Sprintf("%v:%v", path.Base(file), line)

	select {
	case out := <-output:
		if string(out) != checkOut {
			t.Fatalf("assertOut (%v): response did not match expectation: was %q, expected %q", from, out, checkOut)
		}
	case <-after:
		t.Fatalf("assertOut (%v): timeout waiting for %q", from, checkOut)
	}
}

func TestProxy(t *testing.T) {
	input, output := make(chan []byte), make(chan []byte)
	p := NewProxy(input, output)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)

	input <- []byte("echoed to server 1\n")
	assertOut(t, output, "echoed to server 1\n")

	input <- append([]byte{escapeKey}, []byte("echo\n")...)
	assertOut(t, output, "echo")

	input <- []byte("quit\nechoed to server 2\n")
	assertOut(t, output, "echoed to server 2\n")

	input <- []byte("echoed to server 3\n")
	assertOut(t, output, "echoed to server 3\n")

	input <- append([]byte{escapeKey}, []byte("echo\nquit\n")...)
	assertOut(t, output, "echo")
}
