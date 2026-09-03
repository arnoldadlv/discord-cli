package cli

import (
	"io"
	"sync"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
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
	// The production sleeper returns early when the context is cancelled.
	Sleep discord.SleepFunc

	// ReadPassword reads a secret from the terminal with echo off. Only used
	// when StdinIsTerminal; nil means prompting is impossible.
	ReadPassword func() (string, error)

	// APIBaseURL replaces Discord's API base; empty means the real one.
	APIBaseURL string

	// Now returns the current time.
	Now func() time.Time

	// Version is the version string printed by --version.
	Version string
}

// syncWriter serialises writes from concurrent export workers, so progress
// lines and notices never interleave mid-line.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func (e *Env) defaults() {
	e.Stdout = &syncWriter{w: e.Stdout}
	e.Stderr = &syncWriter{w: e.Stderr}
	if e.Getenv == nil {
		e.Getenv = func(string) string { return "" }
	}
	if e.Sleep == nil {
		e.Sleep = discord.ContextSleep
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Version == "" {
		e.Version = "dev"
	}
}
