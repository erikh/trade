package iochan

import (
	"context"
	"io"

	"github.com/sirupsen/logrus"
)

const defaultReadBufSize = 32

// Proxy is a proxy from channels to standard i/o in golang. Write to the input
// channel to write to the writer, read from the output channel to get
// information from the reader.
type Proxy struct {
	Input  chan []byte
	Output chan []byte

	readBufSize int

	Reader io.ReadCloser
	Writer io.WriteCloser
}

// SetReadBufSize sets the read buffer size which can affect both performance
// and memory usage.
func (p *Proxy) SetReadBufSize(i int) {
	p.readBufSize = i
}

// Start starts the proxy. Cancel the context to terminate it.
func (p *Proxy) Start(ctx context.Context) {
	if p.readBufSize == 0 {
		p.readBufSize = defaultReadBufSize
	}

	go func() {
		for {
			select {
			case buf := <-p.Input:
				if _, err := p.Writer.Write(buf); err != nil {
					if err != io.EOF && err != io.ErrClosedPipe {
						logrus.Errorf("iochan: failed to write to writer: %v", err)
					}

					p.Output <- buf
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			buf := make([]byte, p.readBufSize)
			n, err := p.Reader.Read(buf)
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					logrus.Errorf("read error: %v", err)
				}

				return
			}

			select {
			case p.Output <- buf[:n]:
			case <-ctx.Done():
				return
			}
		}
	}
}
