package menu

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/erikh/trade/iochan"
	"github.com/sirupsen/logrus"
)

const escapeKey byte = 0x5 // this is ^E

// Proxy is the menu proxy wrapper; it provides basic access to the menu
// through a set of i/o channels.
type Proxy struct {
	menuActive bool
	mutex      sync.Mutex

	input  chan []byte
	output chan []byte

	dialog Dialog
}

// NewProxy sets up a new Proxy struct properly; the BasicDialog dialog is used
// in this returned value. If you want to change the proxy dialog, use
// SetDialog().
func NewProxy(input, output chan []byte) *Proxy {
	return &Proxy{
		input:  input,
		output: output,
		dialog: &BasicDialog{},
	}
}

func (p *Proxy) setActive(active bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.menuActive = active
}

func (p *Proxy) getActive() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.menuActive
}

func (p *Proxy) createDialogProxy(ctx context.Context, initialBuf []byte) (Dialog, error) {
	rr, rw := io.Pipe() // i/o reader for dialog
	wr, ww := io.Pipe() // i/o writer for dialog

	iochanP := &iochan.Proxy{
		Input:  p.input,
		Output: p.output,
		Reader: rr,
		Writer: ww,
	}

	dialog, err := p.dialog.Instance(rw, wr)
	if err != nil {
		return nil, err
	}

	defer func() { p.input <- initialBuf }()
	go iochanP.Start(ctx)

	return dialog, nil
}

// Start starts the proxy with the appropriate context; if it is completed, the
// proxy will finish automatically.
func (p *Proxy) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case buf := <-p.input:
			i := bytes.Index(buf, []byte{escapeKey})
			if i >= 0 {
				p.setActive(true)
				if len(buf) > i {
					buf = buf[i+1:]
				}
			}

			if p.getActive() {
				dialogCtx, dialogCancel := context.WithCancel(ctx)

				dialog, err := p.createDialogProxy(dialogCtx, buf)
				if err != nil {
					logrus.Error("createDialogProxy():", err)
					dialogCancel()
					return
				}

				for p.getActive() {
					command, err := dialog.GetCommand()
					if err != nil {
						logrus.Errorf("getCommand: %v", err)
						dialogCancel()
						break
					}

					switch command {
					case "quit":
						p.setActive(false)
						if err := dialog.Close(); err != nil {
							logrus.Error("dialog.Close:", err)
						}

						b := dialog.Left()
						if len(b) > 0 {
							p.output <- b
						}

						break
					case "echo":
						p.output <- []byte("echo")
					default:
						p.output <- []byte("invalid command")
					}
				}

				dialogCancel()
			} else {
				p.output <- buf
			}
		}
	}
}
