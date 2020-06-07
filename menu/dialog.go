package menu

import (
	"bytes"
	"io"

	"github.com/chzyer/readline"
)

const basicMaxBuf = 32

// Dialog is the configuration of the dialog used to enter input and
// provide output.
type Dialog interface {
	Instance(io.WriteCloser, io.ReadCloser) (Dialog, error)
	GetCommand() (string, error)
	Left() []byte
	Close() error
}

// BasicDialog is one that accepts one command per line. It is primarily
// intended for testing, but could also be used in simple scenarios, such as
// talking to a pipe.
type BasicDialog struct {
	writer io.WriteCloser
	reader io.ReadCloser

	temp []byte
}

// Instance clones self, adds the file handles and returns itself.
func (bd *BasicDialog) Instance(writer io.WriteCloser, reader io.ReadCloser) (Dialog, error) {
	self := *bd
	self.writer = writer
	self.reader = reader

	return &self, nil
}

// GetCommand returns the command provided.
func (bd *BasicDialog) GetCommand() (string, error) {
	// NOTE not using bufio.Scanner here because it will cause unnecessary
	// buffering that is hard to push back in our design.
loop:
	buf := make([]byte, basicMaxBuf)
	n, err := bd.reader.Read(buf)
	if err != nil {
		return "", err
	}

	switch i := bytes.Index(buf, []byte{'\n'}); i {
	case -1:
		if len(bd.temp) != 0 {
			bd.temp = append(bd.temp, buf[:n]...)
		} else {
			bd.temp = buf[:n]
		}
		goto loop
	default:
		var tmpbuf []byte
		if len(bd.temp) != 0 {
			tmpbuf = append(bd.temp, buf[:n]...)
		} else {
			tmpbuf = buf[:i]
		}

		bd.temp = buf[i+1 : n]

		return string(bytes.TrimSpace(tmpbuf)), nil
	}
}

// Left returns any leftover contents in the buffer.
func (bd *BasicDialog) Left() []byte {
	return bd.temp
}

// Close closes the dialog.
func (bd *BasicDialog) Close() error {
	bd.reader.Close()
	return bd.writer.Close()
}

// ReadlineDialog is a readline-capable Dialog, provided by chyzer/readline.
type ReadlineDialog struct {
	writer   io.WriteCloser
	reader   io.ReadCloser
	rlConfig *readline.Config
	rl       *readline.Instance
}

// NewReadlineDialog creates a new dialog for use. A writer, reader, and
// readline config must be provided; they will be merged at the time the dialog
// is requested.
//
// Call GetCommand() to get a line-oriented command.
func NewReadlineDialog(config readline.Config) Dialog {
	return &ReadlineDialog{
		rlConfig: &config,
	}
}

// Instance creates a new instance after setting the file handles.
func (rd *ReadlineDialog) Instance(writer io.WriteCloser, reader io.ReadCloser) (Dialog, error) {
	self := *rd
	self.writer = writer
	self.reader = reader

	var err error
	self.rl, err = readline.NewEx(rd.mkConfig())
	if err != nil {
		return nil, err
	}

	return &self, nil
}

func (rd *ReadlineDialog) mkConfig() *readline.Config {
	config2 := *rd.rlConfig
	config2.Stdin = rd.reader
	config2.StdinWriter = rd.writer
	config2.Stdout = rd.writer
	config2.Stderr = rd.writer
	return &config2
}

// GetCommand returns a line-oriented command
func (rd *ReadlineDialog) GetCommand() (string, error) {
	return rd.rl.Readline()
}

// Close closes the underlying readline handle.
func (rd *ReadlineDialog) Close() error {
	rd.writer.Close()
	rd.reader.Close()

	return rd.rl.Close()
}

// Left returns any i/o left in the reader
func (rd *ReadlineDialog) Left() []byte {
	buf := bytes.NewBuffer(nil)
	io.Copy(buf, rd.reader)
	return buf.Bytes()
}
