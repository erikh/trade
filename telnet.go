package main

import (
	"context"
	"os"

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
				os.Exit(1)
			}

			output <- buf[:n]
		}
	}()

	go func() {
		for byt := range input {
			if _, err := tp.conn.Write(byt); err != nil {
				tp.conn.Close()
				os.Exit(1)
			}
		}
	}()
}
