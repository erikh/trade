package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

type menuProxy struct {
	connected  bool
	menuActive bool
	mutex      sync.Mutex

	pInput  chan []byte
	pOutput chan []byte

	// "connected" channels. that's what the c means. exciting eh?
	cInput  chan []byte
	cOutput chan []byte
}

func prompt(s string) []byte {
	return []byte("\r\n" + s + ": ")
}

func respond(s string) []byte {
	return []byte(s + "\r\n")
}

func newMenuProxy(input, output chan []byte) *menuProxy {
	return &menuProxy{
		pInput:  input,
		pOutput: output,
		cInput:  make(chan []byte, 1),
		cOutput: make(chan []byte),
	}
}

func newlineify(s string) []byte {
	return []byte(strings.ReplaceAll(s, "\n", "\r\n"))
}

func (mp *menuProxy) menu(output chan<- []byte) {
	if mp.connected {
		output <- newlineify(`
?: This menu
R: Re-attach to the proxy
S: Shutdown the proxy
> `)
	} else {
		output <- newlineify(`
?: This menu
C: Connect to a host:port pair
S: Shutdown the proxy
> `)
	}
}

func (mp *menuProxy) readline(byt []byte) string {
	for !bytes.Contains(byt, []byte{'\r'}) {
		byt2 := <-mp.pInput
		for {
			pruned := false
			idx := bytes.Index(byt2, []byte{127})
			switch idx {
			case -1:
				goto end
			case 0:
				if len(byt) != 0 {
					byt = byt[:len(byt)-1]
					pruned = true
				} else {
					byt = []byte{}
				}
				if len(byt2) == 1 {
					byt2 = []byte{}
				} else {
					byt2 = byt2[:idx-1]
					pruned = true
				}
			default:
				byt2 = append(byt2[:idx-1], byt2[idx:]...)
				pruned = true
			}
			if pruned {
				mp.pOutput <- []byte{8, '\x1b', '[', 'K'}
			}
		}
	end:
		mp.pOutput <- byt2
		byt = append(byt, byt2...)
	}

	parts := bytes.SplitN(byt, []byte{'\r'}, 2)
	response := string(parts[0])
	byt = parts[1]

	go func() { mp.pInput <- byt }()

	return response
}

func (mp *menuProxy) toggleConnected() {
	mp.mutex.Lock()
	defer mp.mutex.Unlock()
	mp.connected = !mp.connected
}

func (mp *menuProxy) establishConnection(ctx context.Context) {
	mp.mutex.Lock()
	defer mp.mutex.Unlock()
	mp.connected = true

	go func() {
		defer func() {
			close(mp.cInput)
			close(mp.cOutput)
			close(mp.pOutput)
			close(mp.pInput)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case buf := <-mp.cOutput:
				mp.pOutput <- buf
			case buf := <-mp.pInput:
				if mp.menuActive {
					if mp.readMenu(ctx, buf, mp.pOutput, mp.pInput) {
						return
					}
				} else {
					i := bytes.Index(buf, []byte{0x5})
					if i >= 0 {
						mp.mutex.Lock()
						mp.menuActive = true
						mp.mutex.Unlock()

						mp.menu(mp.pOutput)

						if len(buf) > i {
							if mp.readMenu(ctx, buf[i+1:], mp.pOutput, mp.pInput) {
								return
							}
						}
					} else {
						mp.cInput <- buf
					}
				}
			}
		}
	}()
}

func (mp *menuProxy) connect(ctx context.Context, byt []byte) {
	mp.pOutput <- prompt("Connect")
	host := mp.readline(byt)
	mp.pOutput <- respond(fmt.Sprintf("connecting to host: %v", host))
	tp, err := newTelnetProxy(host)
	if err != nil {
		mp.pOutput <- respond(errors.Wrap(err, "could not connect").Error())
		return
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	mp.establishConnection(ctx)
	tp.start(ctx, mp.cInput, mp.cOutput)
}

func (mp *menuProxy) start(ctx context.Context) {
	for byt := range mp.pInput {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if mp.readMenu(ctx, byt, mp.pOutput, mp.pInput) {
			return
		}
	}
}

func (mp *menuProxy) readMenu(ctx context.Context, byt []byte, output, input chan []byte) bool {
	for x := 0; x < len(byt); x++ {
		b := byt[x]

		select {
		case <-ctx.Done():
			return true
		default:
		}

		switch b {
		case '?':
			mp.menu(output)
		case 's', 'S':
			output <- respond("shutting down")
			return true
		case 'c', 'C':
			mp.mutex.Lock()
			connected := mp.connected
			mp.mutex.Unlock()
			if !connected {
				mp.connect(ctx, byt[x+1:])
			}
		case 'r', 'R':
			mp.mutex.Lock()
			if mp.connected {
				mp.menuActive = false
			}
			mp.mutex.Unlock()
			output <- respond("reattached")
		default:
			mp.menu(output)
		}
	}

	return false
}
