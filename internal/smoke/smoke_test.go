// Package smoke holds the one test that talks to the real Discord API. It
// runs only when DISCORD_CLI_LIVE_TEST is set, so CI never touches a real
// account. It prints no names, ids, or messages.
package smoke

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/cli"
	"github.com/arnoldadlv/discord-cli/internal/discord"
)

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("DISCORD_CLI_LIVE_TEST") == "" {
		t.Skip("set DISCORD_CLI_LIVE_TEST=1 and DISCORD_TOKEN to run this against the real Discord API")
	}
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		t.Fatal("DISCORD_CLI_LIVE_TEST is set but DISCORD_TOKEN is empty")
	}
	home := t.TempDir()
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return home
		case "XDG_CONFIG_HOME":
			return filepath.Join(home, ".config")
		case "XDG_DATA_HOME":
			return filepath.Join(home, ".local", "share")
		case "XDG_CACHE_HOME":
			return filepath.Join(home, ".cache")
		case "DISCORD_TOKEN":
			return token
		}
		return ""
	}
	run := func(args ...string) (string, int) {
		var stdout, stderr strings.Builder
		code := cli.Run(context.Background(), cli.Env{
			Args:    args,
			Stdin:   strings.NewReader(""),
			Stdout:  &stdout,
			Stderr:  &stderr,
			Getenv:  getenv,
			Sleep:   discord.ContextSleep,
			Version: "smoke",
		})
		if code != 0 {
			// stderr may name guilds or channels; report only the code.
			t.Logf("%v exited %d", args, code)
		}
		return stdout.String(), code
	}

	out, code := run("auth", "status", "--json")
	if code != 0 {
		t.Fatalf("auth status failed with exit %d", code)
	}

	out, code = run("guild", "list", "--json")
	if code != 0 {
		t.Fatalf("guild list failed with exit %d", code)
	}
	var guilds []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &guilds); err != nil || len(guilds) == 0 {
		t.Fatalf("guild list returned no guilds (%v)", err)
	}

	out, code = run("channel", "list", "--guild", guilds[0].ID, "--json")
	if code != 0 {
		t.Fatalf("channel list failed with exit %d", code)
	}
	var channels []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &channels); err != nil || len(channels) == 0 {
		t.Fatalf("channel list returned no channels (%v)", err)
	}

	out, code = run("channel", "read", channels[0].ID, "--guild", guilds[0].ID, "--limit", "3", "--json")
	if code != 0 {
		t.Fatalf("channel read failed with exit %d", code)
	}
	var read struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &read); err != nil {
		t.Fatalf("channel read output is not JSON: %v", err)
	}
	t.Logf("live smoke passed: %d guilds, %d channels in the first, %d messages read", len(guilds), len(channels), len(read.Messages))
}
