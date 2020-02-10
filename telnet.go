package main

import (
	"bytes"
	"context"

	"github.com/ziutek/telnet"
)

type telnetProxy struct {
	addr string
	conn *telnet.Conn
}

func newTelnetProxy(addr string) (*telnetProxy, error) {
	conn, err := telnet.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &telnetProxy{addr: addr, conn: conn}, nil
}

func (tp *telnetProxy) start(ctx context.Context, input <-chan []byte, output chan<- []byte) {
	go func() {
		<-ctx.Done()
		tp.conn.Close()
	}()

	go func() {
		for {
			buf := make([]byte, 32)
			n, err := tp.conn.Read(buf)
			if err != nil {
				tp.conn.Close()
				break
			}

			output <- buf[:n]
		}
	}()

	for byt := range input {
		// I guess we should consult the tty settings for this; but right now this
		// works.
		byt = bytes.ReplaceAll(byt, []byte{127}, []byte{8})

		if _, err := tp.conn.Write(byt); err != nil {
			tp.conn.Close()
			break
		}
	}
}
