package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type menuProxy struct {
}

func prompt(s string) []byte {
	return []byte(s + ": ")
}

func respond(s string) []byte {
	return []byte(s + "\r\n")
}

func newMenuProxy() *menuProxy {
	return &menuProxy{}
}

func newlineify(s string) []byte {
	return []byte(strings.ReplaceAll(s, "\n", "\r\n"))
}

func (mp *menuProxy) menu(output chan<- []byte) {
	output <- newlineify(`
?: This menu
C: Connect to a host:port pair
S: Shutdown the proxy
`)
}

func (mp *menuProxy) connect(ctx context.Context, byt []byte, input chan []byte, output chan<- []byte) {
	output <- prompt("Connect")
	for !bytes.Contains(byt, []byte{'\r'}) {
		byt2 := <-input
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
				fmt.Print(string([]byte{8}) + "\033[K")
			}
		}
	end:
		fmt.Print(string(byt2))
		byt = append(byt, byt2...)
	}

	parts := bytes.SplitN(byt, []byte{'\r'}, 2)
	host := string(parts[0])
	byt = parts[1]

	output <- respond(fmt.Sprintf("connecting to host: %v", host))
	tp, err := newTelnetProxy(host)
	if err != nil {
		output <- respond(errors.Wrap(err, "could not connect").Error())
		return
	}

	go func() { input <- byt }()
	tp.start(ctx, input, output)
}

func (mp *menuProxy) start(ctx context.Context, input chan []byte, output chan<- []byte) {
	for byt := range input {
		select {
		case <-ctx.Done():
			return
		default:
		}

		for x := 0; x < len(byt); x++ {
			b := byt[x]

			select {
			case <-ctx.Done():
				return
			default:
			}

			switch b {
			case '?':
				mp.menu(output)
			case 's', 'S':
				output <- respond("shutting down")
				return
			case 'c', 'C':
				mp.connect(ctx, byt[x+1:], input, output)
			default:
				output <- respond("invalid command")
			}
		}
	}
}
