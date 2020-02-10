package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"golang.org/x/text/encoding/charmap"
)

const (
	// Author is the author
	Author = "Erik Hollensbe <erik@hollensbe.org>"
	// Version is the version
	Version = "0.1.0"
	// Usage is some informative text that shows at the top
	Usage = "SSH -> Telnet gateway"
	// Description is the meat of the help.
	Description = `
	trade is a ssh -> telnet gateway with command shell and other features.

	To start:

		$ trade &

	And connect to localhost:2002 over SSH. Pass the "-l" flag to specify a
	listening address:
	
		$ trade -l :2002 & # listens on public addresses!

	For a listing of flags:

		$ trade --help
`

	// UsageText is the argument format for the command. We simplify it here since there are no subcommands... yet!
	UsageText = "trade [flags]"
)

func main() {
	app := cli.NewApp()

	app.Author = Author
	app.Version = Version
	app.Usage = Usage
	app.Description = Description
	app.UsageText = UsageText

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen,l",
			Usage: "host:port of SSH listener",
			Value: "localhost:2002",
		},
	}

	app.Action = start

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func start(cliCtx *cli.Context) error {
	if len(cliCtx.Args()) != 0 {
		return errors.New("invalid args -- none should be provided")
	}

	signer, err := genSigner()
	if err != nil {
		return errors.Wrap(err, "Could not generate host key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		cancel()
		os.Exit(0)
	}()
	signal.Notify(sigChan, unix.SIGTERM, unix.SIGINT)

	s := newSSHServer(cliCtx.GlobalString("listen"), signer)

	inputChan := make(chan []byte)
	outputChan := make(chan []byte)
	s.setChans(inputChan, outputChan)
	s.setCodec(charmap.CodePage437)

	if err := s.start(ctx); err != nil {
		return errors.Wrap(err, "Could not start SSH service")
	}

	mp := newMenuProxy()
	mp.start(ctx, inputChan, outputChan)

	return nil
}
