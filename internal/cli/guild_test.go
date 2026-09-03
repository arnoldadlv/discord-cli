package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func guildRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := clitest.NewRunner(t)
	r.Fake.JSON("/users/@me/guilds", clitest.Guilds())
	r.Fake.JSON("/guilds/1001", clitest.Guild("1001"))
	r.Fake.JSON("/guilds/1001/channels", clitest.Channels())
	return r
}

func TestGuildListHuman(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"Cooey COE", "1001", "1200", "📚 Book Club", "NAME", "ID", "MEMBERS"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	q := r.Fake.RequestsTo("/users/@me/guilds")[0].Query
	if q.Get("with_counts") != "true" || q.Get("limit") != "200" {
		t.Errorf("query %v", q)
	}
}

func TestGuildListJSON(t *testing.T) {
	r := guildRunner(t)
	var out []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
		OnlineCount int    `json:"online_count"`
		Owner       bool   `json:"owner"`
	}
	r.Run("guild", "list", "--json").JSON(t, &out)
	if len(out) != 3 || out[0].ID != "1001" || out[0].Name != "Cooey COE" || out[0].MemberCount != 1200 || out[0].OnlineCount != 80 || !out[1].Owner {
		t.Errorf("%+v", out)
	}
}

func TestGuildShowByFlagHumanAndJSON(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "show", "--guild", "Cooey COE")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"Cooey COE", "1001", "1200", "5"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	var j struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		MemberCount  int    `json:"member_count"`
		ChannelCount int    `json:"channel_count"`
	}
	r.Run("guild", "show", "--guild", "1001", "--json").JSON(t, &j)
	if j.ID != "1001" || j.Name != "Cooey COE" || j.MemberCount != 1200 || j.ChannelCount != 5 {
		t.Errorf("%+v", j)
	}
}

func TestGuildResolutionSteps(t *testing.T) {
	cases := map[string]string{
		"1001":      "1001",
		"cooey coe": "1001",
		"COOEY COE": "1001",
		"cooey-coe": "1001",
		"book-club": "1002",
	}
	for input, wantID := range cases {
		r := guildRunner(t)
		r.Fake.JSON("/guilds/1002", clitest.Guild("1002"))
		r.Fake.JSON("/guilds/1002/channels", []any{})
		var j struct {
			ID string `json:"id"`
		}
		res := r.Run("guild", "show", "--guild", input, "--json")
		if res.ExitCode != 0 {
			t.Errorf("%q: exit %d: %s", input, res.ExitCode, res.Stderr)
			continue
		}
		res.JSON(t, &j)
		if j.ID != wantID {
			t.Errorf("%q resolved to %s, want %s", input, j.ID, wantID)
		}
	}
}

func TestGuildNotFoundSuggests(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "show", "--guild", "cooey")
	if res.ExitCode != 4 {
		t.Fatalf("exit %d, want 4: %s", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "" {
		t.Errorf("stdout not empty: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, `guild "cooey" not found`) {
		t.Errorf("stderr: %q", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Cooey COE") || !strings.Contains(res.Stderr, "Cooey Alumni") {
		t.Errorf("suggestions missing: %q", res.Stderr)
	}
	if strings.Contains(res.Stderr, "Book Club") {
		t.Errorf("unrelated guild suggested: %q", res.Stderr)
	}
}

func TestGuildNotFoundNoSuggestionsListsAvailable(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "show", "--guild", "zzz")
	if res.ExitCode != 4 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "guild list") {
		t.Errorf("should point at guild list: %q", res.Stderr)
	}
}

func TestMissingGuildIsUsageErrorWithHint(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "show")
	if res.ExitCode != 2 {
		t.Fatalf("exit %d, want 2: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "config set default-guild") || !strings.Contains(res.Stderr, "--guild") {
		t.Errorf("stderr: %q", res.Stderr)
	}
}

func TestConfigSetDefaultGuildAndGet(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("config", "set", "default-guild", "cooey-coe")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Cooey COE") {
		t.Errorf("should confirm the resolved guild: %q", res.Stderr)
	}
	cfg := string(r.Home.ReadFile(t, r.Home.ToolConfigDir()+"/config.json"))
	if !strings.Contains(cfg, `"default-guild": "cooey-coe"`) {
		t.Errorf("config.json: %s", cfg)
	}

	res = r.Run("config", "get")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "default-guild") || !strings.Contains(res.Stdout, "cooey-coe") {
		t.Errorf("config get: %d %q", res.ExitCode, res.Stdout)
	}
	res = r.Run("config", "get", "default-guild")
	if strings.TrimSpace(res.Stdout) != "cooey-coe" {
		t.Errorf("config get default-guild: %q", res.Stdout)
	}
	var j map[string]string
	r.Run("config", "get", "--json").JSON(t, &j)
	if j["default-guild"] != "cooey-coe" {
		t.Errorf("json %v", j)
	}

	// The default now feeds guild show; --guild still wins.
	var g struct {
		ID string `json:"id"`
	}
	res = r.Run("guild", "show", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("guild show with default: exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &g)
	if g.ID != "1001" {
		t.Errorf("default guild not used: %+v", g)
	}
	r.Fake.JSON("/guilds/1002", clitest.Guild("1002"))
	r.Fake.JSON("/guilds/1002/channels", []any{})
	r.Run("guild", "show", "--guild", "book-club", "--json").JSON(t, &g)
	if g.ID != "1002" {
		t.Errorf("--guild did not override: %+v", g)
	}
}

func TestConfigSetUnknownGuildFails(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("config", "set", "default-guild", "nowhere")
	if res.ExitCode != 4 {
		t.Errorf("exit %d, want 4: %s", res.ExitCode, res.Stderr)
	}
}

func TestConfigSetUnknownKeyIsUsageError(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("config", "set", "colour", "blue")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2: %s", res.ExitCode, res.Stderr)
	}
	res = r.Run("config", "get", "colour")
	if res.ExitCode != 2 {
		t.Errorf("get: exit %d, want 2: %s", res.ExitCode, res.Stderr)
	}
}

func TestConfigGetEmpty(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("config", "get")
	if res.ExitCode != 0 {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "default-guild") {
		t.Errorf("should list keys even when unset: %q", res.Stdout)
	}
}

func TestGuildListIsCachedForADay(t *testing.T) {
	r := guildRunner(t)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { return now }

	r.Run("guild", "list")
	if n := len(r.Fake.Requests()); n == 0 {
		t.Fatal("first run should hit the server")
	}
	r.Fake.Reset()

	res := r.Run("guild", "list")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Cooey COE") {
		t.Fatalf("cached run: %d %q", res.ExitCode, res.Stdout)
	}
	if n := len(r.Fake.Requests()); n != 0 {
		t.Errorf("second run within 24h made %d requests", n)
	}

	// Resolution uses the cache too.
	r.Fake.Reset()
	r.Run("guild", "show", "--guild", "cooey-coe")
	for _, req := range r.Fake.Requests() {
		if req.Path == "/users/@me/guilds" {
			t.Errorf("guild list refetched during resolution")
		}
	}

	r.Fake.Reset()
	r.Run("guild", "list", "--no-cache")
	if n := len(r.Fake.RequestsTo("/users/@me/guilds")); n != 1 {
		t.Errorf("--no-cache made %d guild requests, want 1", n)
	}

	r.Fake.Reset()
	now = now.Add(25 * time.Hour)
	r.Run("guild", "list")
	if n := len(r.Fake.RequestsTo("/users/@me/guilds")); n != 1 {
		t.Errorf("stale cache made %d guild requests, want 1", n)
	}
}
