// Command discord reads a person's own Discord account from the terminal.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/arnoldadlv/discord-cli/internal/cli"
)

// version is set with -ldflags "-X main.version=..." for releases.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env := cli.Env{
		Args:             os.Args[1:],
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		Getenv:           os.Getenv,
		StdinIsTerminal:  term.IsTerminal(int(os.Stdin.Fd())),
		StdoutIsTerminal: term.IsTerminal(int(os.Stdout.Fd())),
		StderrIsTerminal: term.IsTerminal(int(os.Stderr.Fd())),
		Version:          version,
		ReadPassword: func() (string, error) {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			return string(b), err
		},
	}
	os.Exit(cli.Run(ctx, env))
}
