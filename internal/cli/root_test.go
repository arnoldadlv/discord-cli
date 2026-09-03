package cli_test

import (
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func TestBareCommandPrintsConciseHelp(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run()
	if res.ExitCode != 0 {
		t.Fatalf("exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"Usage:", "discord guild", "--help", "Examples:", "github.com/arnoldadlv/discord-cli"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.Stdout)
		}
	}
}

func TestVersionPrintsVersionAndBuildInfo(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("--version")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "discord version test") {
		t.Errorf("stdout: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "go") {
		t.Errorf("expected build info (go version) in %q", res.Stdout)
	}
}

func TestHelpWorksAnywhere(t *testing.T) {
	r := clitest.NewRunner(t)
	for _, args := range [][]string{
		{"help"},
		{"--help"},
		{"-h"},
		{"guild", "--help"},
		{"guild", "-h"},
		{"help", "guild"},
		{"--json", "guild", "-h"},
	} {
		res := r.Run(args...)
		if res.ExitCode != 0 {
			t.Errorf("%v: exit %d, stderr %s", args, res.ExitCode, res.Stderr)
		}
		if !strings.Contains(res.Stdout, "Usage:") {
			t.Errorf("%v: no usage in stdout: %q", args, res.Stdout)
		}
	}
}

func TestNounAloneShowsItsHelp(t *testing.T) {
	r := clitest.NewRunner(t)
	for _, noun := range []string{"guild", "channel", "dm", "export", "auth", "config", "cache"} {
		res := r.Run(noun)
		if res.ExitCode != 0 {
			t.Errorf("%s: exit %d stderr %s", noun, res.ExitCode, res.Stderr)
		}
		if !strings.Contains(res.Stdout, "discord "+noun) {
			t.Errorf("%s: help does not mention the noun: %q", noun, res.Stdout)
		}
	}
}

func TestUnknownCommandSuggestsClosest(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("guilds")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2", res.ExitCode)
	}
	if res.Stdout != "" {
		t.Errorf("stdout should be empty, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "guild") || !strings.Contains(strings.ToLower(res.Stderr), "did you mean") {
		t.Errorf("stderr should suggest guild: %q", res.Stderr)
	}
}

func TestUnknownVerbSuggestsClosest(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("guild", "lst")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2; stderr %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "unknown") {
		t.Errorf("stderr: %q", res.Stderr)
	}
}

func TestAbbreviationsAreNotAccepted(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("gui")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2", res.ExitCode)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("guild", "--bogus")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "bogus") {
		t.Errorf("stderr: %q", res.Stderr)
	}
}

func TestErrorsGoToStderrWithPrefix(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("nope")
	if res.Stdout != "" {
		t.Errorf("stdout not empty: %q", res.Stdout)
	}
	if !strings.HasPrefix(res.Stderr, "discord: ") {
		t.Errorf("stderr should be prefixed for humans: %q", res.Stderr)
	}
}

func TestColorOnlyOnTerminalAndNotWithNoColor(t *testing.T) {
	cases := []struct {
		name    string
		tty     bool
		env     map[string]string
		args    []string
		wantAns bool
	}{
		{"piped", false, nil, nil, false},
		{"tty", true, nil, nil, true},
		{"tty NO_COLOR", true, map[string]string{"NO_COLOR": "1"}, nil, false},
		{"tty TERM=dumb", true, map[string]string{"TERM": "dumb"}, nil, false},
		{"tty --no-color", true, nil, []string{"--no-color"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := clitest.NewRunner(t)
			r.StdoutTTY = c.tty
			for k, v := range c.env {
				r.Env[k] = v
			}
			res := r.Run(append([]string{}, c.args...)...)
			gotAnsi := strings.Contains(res.Stdout, "\x1b[")
			if gotAnsi != c.wantAns {
				t.Errorf("ansi in stdout = %v, want %v:\n%q", gotAnsi, c.wantAns, res.Stdout)
			}
		})
	}
}
