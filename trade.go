package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"

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

	To start, generate a host key:

		$ trade gen

	Then, to run the proxy:

		$ trade &

	And connect to localhost:2002 over SSH. Once connected, press '?' for a list
	of options.

	Pass the "-l" flag to specify a listening address:
	
		$ trade -l :2002 & # listens on public addresses!

	For a listing of flags:

		$ trade --help
`

	// UsageText is the argument format for the command. We simplify it here since there are no subcommands... yet!
	UsageText = "trade [flags]"
)

var configPath = path.Join(os.Getenv("HOME"), ".config/trade")
var hostKeyPath = path.Join(configPath, "host_key")

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
		cli.StringFlag{
			Name:  "host-key,k",
			Usage: "Path to host key",
			Value: hostKeyPath,
		},
		cli.BoolFlag{
			Name:  "auto-key,a",
			Usage: "Auto-generate a host key for use",
		},
		cli.BoolTFlag{
			Name:  "use-dos,d",
			Usage: "Use DOS codepage 437 for translation",
		},
	}

	app.Action = start

	app.Commands = []cli.Command{
		{
			Name:      "generate-host-key",
			ShortName: "gen",
			Action:    generateHostKey,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "output,o",
					Usage: "Output host key to this file",
					Value: hostKeyPath,
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func generateHostKey(cliCtx *cli.Context) error {
	pk, err := genKey()
	if err != nil {
		return errors.Wrap(err, "while generating key")
	}

	derBytes, err := x509.MarshalECPrivateKey(pk)
	if err != nil {
		return errors.Wrap(err, "while converting key to x.509 format")
	}

	if _, err := os.Stat(cliCtx.String("output")); err != nil {
		if err := os.MkdirAll(filepath.Dir(cliCtx.String("output")), 0700); err != nil {
			return errors.Wrap(err, "while creating directory")
		}
	} else {
		if err := os.Remove(cliCtx.String("output")); err != nil {
			return errors.Wrap(err, "while clearing file to be replaced")
		}
	}

	f, err := os.OpenFile(cliCtx.String("output"), unix.O_CREAT|unix.O_TRUNC|unix.O_WRONLY, 0400)
	if err != nil {
		return errors.Wrap(err, "while replacing file")
	}

	return pem.Encode(f, &pem.Block{Bytes: derBytes, Type: "ECDSA PRIVATE KEY"})
}

func start(cliCtx *cli.Context) error {
	if len(cliCtx.Args()) != 0 {
		return errors.New("invalid args -- none should be provided")
	}

	var pk *ecdsa.PrivateKey

	if cliCtx.Bool("auto-key") {
		var err error
		pk, err = genKey()
		if err != nil {
			return errors.Wrap(err, "while generating key")
		}
	} else {
		content, err := ioutil.ReadFile(cliCtx.GlobalString("host-key"))
		if err != nil {
			return errors.Wrap(err, "could not read host key")
		}

		b, _ := pem.Decode(content)
		pk, err = x509.ParseECPrivateKey(b.Bytes)
		if err != nil {
			return errors.Wrap(err, "while parsing key")
		}
	}

	signer, err := genSigner(pk)
	if err != nil {
		return errors.Wrap(err, "Could not generate host key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		cancel()
	}()
	signal.Notify(sigChan, unix.SIGTERM, unix.SIGINT)

	s := newSSHServer(cliCtx.GlobalString("listen"), signer)

	inputChan := make(chan []byte, 1)
	outputChan := make(chan []byte)

	s.setChans(inputChan, outputChan)

	if cliCtx.BoolT("use-dos") {
		s.setCodec(charmap.CodePage437)
	}

	if err := s.start(ctx); err != nil {
		return errors.Wrap(err, "Could not start SSH service")
	}

	mp := newMenuProxy(inputChan, outputChan)
	mp.start(ctx)

	return nil
}
