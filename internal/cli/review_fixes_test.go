package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// Two channels that normalise alike, exported concurrently, must end up in
// distinct files with their own ids.
func TestGuildExportCollisionUnderConcurrency(t *testing.T) {
	r := channelRunner(t)
	r.Fake.JSON("/guilds/1001/channels", clitest.ChannelsWithCollision())
	for id, n := range map[string]int{"2001": 3, "2002": 2, "2003": 4, "2006": 1, "2007": 5, "2011": 6} {
		clitest.ServeMessages(r.Fake, id, clitest.Messages(id, n))
	}
	r.Fake.Delay = 20 * time.Millisecond
	res := r.Run("guild", "export", "--no-cache")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	seen := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "general") {
			continue
		}
		ex := readExport(t, filepath.Join(dir, e.Name()))
		seen[ex.Channel.ID] = e.Name()
	}
	if len(seen) != 2 || seen["2001"] == "" || seen["2011"] == "" || seen["2001"] == seen["2011"] {
		t.Errorf("collision handling under concurrency: %v", seen)
	}
	meta := readMeta(t, dir)
	if meta.Channels["2001"].MessageCount != 3 || meta.Channels["2011"].MessageCount != 6 {
		t.Errorf("meta %+v", meta.Channels)
	}
}

func TestChannelExportThreadsExportsChannelAndItsThreads(t *testing.T) {
	r, _ := readRunner(t, 2)
	clitest.ServeMessages(r.Fake, "3001", clitest.Messages("3001", 2))
	clitest.ServeMessages(r.Fake, "3002", clitest.Messages("3002", 1))
	var j struct {
		Channel struct{ ID string }
		Status  string `json:"status"`
		Threads []struct {
			Channel struct{ ID, Name string }
			Path    string `json:"path"`
			Status  string `json:"status"`
		} `json:"threads"`
	}
	res := r.Run("channel", "export", "general", "--threads", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if j.Channel.ID != "2001" || j.Status != "exported" || len(j.Threads) != 2 {
		t.Fatalf("%+v", j)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	for _, f := range []string{"general.json", filepath.Join("threads", "general", "welcome-thread.json"), filepath.Join("threads", "general", "old-planning.json")} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	res = r.Run("channel", "export", "general", "--threads")
	if res.ExitCode != 0 || strings.Count(res.Stdout, "up to date") != 3 {
		t.Errorf("rerun: %d %q", res.ExitCode, res.Stdout)
	}
	// Without --threads nothing thread-related is fetched.
	r.Fake.Reset()
	r.Run("channel", "export", "general")
	if n := len(r.Fake.RequestsTo("/channels/2001/threads/search")); n != 0 {
		t.Errorf("threads fetched without --threads: %d", n)
	}
}

func TestGuildSearchChannelDoesNotFetchThreads(t *testing.T) {
	r := searchRunner(t, 3)
	res := r.Run("guild", "search", "x", "--channel", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, req := range r.Fake.Requests() {
		if strings.Contains(req.Path, "/threads/search") {
			t.Fatalf("thread search called while resolving --channel: %s", req.Path)
		}
	}
}

func TestForbiddenChannelIsNotFoundNotAuth(t *testing.T) {
	r, _ := readRunner(t, 1)
	r.Fake.Queue("/channels/2001/messages", clitest.Response{Status: 403, Body: `{"message":"Missing Access","code":50001}`})
	res := r.Run("channel", "read", "general")
	if res.ExitCode != 4 {
		t.Errorf("exit %d, want 4: %s", res.ExitCode, res.Stderr)
	}
}

func TestSilentCommandsEmitJSON(t *testing.T) {
	r := guildRunner(t)
	removeToken(t, r)
	r.Stdin = strings.NewReader("tok-json\n")
	var set struct {
		Stored bool `json:"stored"`
	}
	r.Run("auth", "set", "--json").JSON(t, &set)
	if !set.Stored {
		t.Errorf("auth set json: %+v", set)
	}
	var cfg struct {
		Default string `json:"default-guild"`
	}
	r.Run("config", "set", "default-guild", "cooey-coe", "--json").JSON(t, &cfg)
	if cfg.Default != "cooey-coe" {
		t.Errorf("config set json: %+v", cfg)
	}
	var clr struct {
		Cleared bool `json:"cleared"`
	}
	r.Run("cache", "clear", "--json").JSON(t, &clr)
	if !clr.Cleared {
		t.Errorf("cache clear json: %+v", clr)
	}
}

func TestExportListShowsLocationLabels(t *testing.T) {
	r := inventoryRunner(t)
	var j []struct {
		Location string `json:"location"`
	}
	r.Run("export", "list", "--json").JSON(t, &j)
	labels := map[string]bool{}
	for _, e := range j {
		labels[e.Location] = true
	}
	for _, want := range []string{"xdg", "node", "chatexporter"} {
		if !labels[want] {
			t.Errorf("missing location %q in %v", want, labels)
		}
	}
	if labels["legacy"] || labels["dce"] {
		t.Errorf("glossary-conflicting labels present: %v", labels)
	}
}
