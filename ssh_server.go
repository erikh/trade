package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

type sshServer struct {
	config     *ssh.ServerConfig
	listenAddr string
	chans      []ssh.Channel
	conns      []*ssh.ServerConn
	mutex      sync.RWMutex

	inputChannel  chan<- []byte
	outputChannel <-chan []byte

	encoder *encoding.Encoder
	decoder *encoding.Decoder
}

func genKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
}

func genSigner(pk *ecdsa.PrivateKey) (ssh.Signer, error) {
	signer, err := ssh.NewSignerFromKey(pk)
	if err != nil {
		return nil, err
	}

	return signer, nil
}

func newSSHServer(listener string, signer ssh.Signer) *sshServer {
	conf := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	conf.AddHostKey(signer)

	return &sshServer{
		config:     conf,
		listenAddr: listener,
		chans:      []ssh.Channel{},
		conns:      []*ssh.ServerConn{},
	}
}

func (s *sshServer) setCodec(cm *charmap.Charmap) {
	s.encoder = cm.NewEncoder()
	s.decoder = cm.NewDecoder()
}

func (s *sshServer) connections() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.conns)
}

func (s *sshServer) setChans(input chan<- []byte, output <-chan []byte) {
	s.inputChannel = input
	s.outputChannel = output
}

func (s *sshServer) start(ctx context.Context) error {
	l, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}

	go func() {
		for byt := range s.outputChannel {
			if s.decoder != nil {
				byt, err = s.decoder.Bytes(byt)
				if err != nil {
					continue
				}
			}

			chanList := []ssh.Channel{}
			connList := []*ssh.ServerConn{}
			s.mutex.Lock()
			for i, c := range s.chans {
				n, err := c.Write(byt)
				if err != nil {
					goto prune
				}
				if n != len(byt) {
					goto prune
				}

				chanList = append(chanList, c)
				connList = append(connList, s.conns[i])
				continue
			prune:
				s.conns[i].Close()
			}
			s.chans = chanList
			s.conns = connList
			s.mutex.Unlock()
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				l.Close()
				return
			default:
			}

			conn, err := l.Accept()
			if err != nil {
				continue
			}

			sc, ch, _, err := ssh.NewServerConn(conn, s.config)
			if err != nil {
				continue
			}

			c, _, err := (<-ch).Accept()
			if err != nil {
				continue
			}

			go func() {
				for {
					byt := make([]byte, 32)
					n, err := c.Read(byt)
					if err != nil {
						break
					}

					if s.encoder != nil {
						byt, err = s.encoder.Bytes(byt[:n])
						if err != nil {
							continue
						}
					} else {
						byt = byt[:n]
					}

					s.inputChannel <- byt
				}
			}()
			s.mutex.Lock()
			s.chans = append(s.chans, c)
			s.conns = append(s.conns, sc)
			s.mutex.Unlock()
		}
	}()

	return nil
}
