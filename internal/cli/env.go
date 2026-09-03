package cli

import (
	"io"
	"time"
)

// Env is everything the tool touches outside its own process, so tests can
// drive it through the command boundary without a real terminal, a real
// Discord, or the real home directory.
type Env struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Getenv reads environment variables (HOME, XDG_*, DISCORD_TOKEN, NO_COLOR, TERM).
	Getenv func(string) string

	StdinIsTerminal  bool
	StdoutIsTerminal bool
	StderrIsTerminal bool

	// Sleep is used for every rate-limit wait so tests can record delays.
	Sleep func(time.Duration)

	// APIBaseURL replaces Discord's API base; empty means the real one.
	APIBaseURL string

	// Now returns the current time.
	Now func() time.Time

	// Version is the version string printed by --version.
	Version string
}

func (e *Env) defaults() {
	if e.Getenv == nil {
		e.Getenv = func(string) string { return "" }
	}
	if e.Sleep == nil {
		e.Sleep = time.Sleep
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Version == "" {
		e.Version = "dev"
	}
}
