package iochan

import (
	"context"
	"io"

	"github.com/sirupsen/logrus"
)

// Proxy is a proxy from channels to standard i/o in golang. Write to the input
// channel to write to the writer, read from the output channel to get
// information from the reader.
type Proxy struct {
	Input  chan []byte
	Output chan []byte

	Reader io.ReadCloser
	Writer io.WriteCloser
}

// Start starts the proxy. Cancel the context to terminate it.
func (p *Proxy) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				buf := <-p.Input
				if _, err := p.Writer.Write(buf); err != nil {
					if err != io.EOF {
						logrus.Errorf("iochan: failed to write to writer: %v", err)
					}

					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			buf := make([]byte, 32)
			n, err := p.Reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					logrus.Errorf("read error: %v", err)
				}

				return
			}

			p.Output <- buf[:n]
		}
	}
}
