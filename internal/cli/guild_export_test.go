package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// guildExportRunner serves messages for every message channel of the
// fixture guild, with a small delay so concurrency is observable.
func guildExportRunner(t *testing.T, delay time.Duration) *clitest.Runner {
	t.Helper()
	r := channelRunner(t)
	counts := map[string]int{"2001": 3, "2002": 2, "2003": 4, "2006": 1, "2007": 5}
	for id, n := range counts {
		clitest.ServeMessages(r.Fake, id, clitest.Messages(id, n))
	}
	if delay > 0 {
		r.Fake.Delay = delay
	}
	return r
}

func TestGuildExportExportsEveryMessageChannel(t *testing.T) {
	r := guildExportRunner(t, 0)
	res := r.Run("guild", "export")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	for _, f := range []string{"general.json", "news.json", "cmmc-general.json", "help-forum.json", "random.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "lounge.json")); err == nil {
		t.Errorf("voice channel exported")
	}
	for _, want := range []string{"5 exported", "0 up to date", "0 failed", "15 messages"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("summary missing %q:\n%s", want, res.Stdout)
		}
	}
	if res.Stderr != "" {
		t.Errorf("no progress expected when stderr is not a terminal: %q", res.Stderr)
	}
	meta := readMeta(t, dir)
	if len(meta.Channels) != 5 || meta.Channels["2007"].MessageCount != 5 || meta.Channels["2001"].LastMessageID != clitest.MessageID(3) {
		t.Errorf("meta %+v", meta)
	}
	// Second run: everything up to date, one request per channel.
	r.Fake.Reset()
	res = r.Run("guild", "export")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "5 up to date") {
		t.Errorf("second run: %d %q", res.ExitCode, res.Stdout)
	}
	for id := range map[string]bool{"2001": true, "2007": true} {
		if n := len(r.Fake.RequestsTo("/channels/" + id + "/messages")); n != 1 {
			t.Errorf("channel %s: %d requests", id, n)
		}
	}
}

func TestGuildExportJSONSummary(t *testing.T) {
	r := guildExportRunner(t, 0)
	var j struct {
		Guild         struct{ ID, Name string }
		Exported      int `json:"exported"`
		UpToDate      int `json:"up_to_date"`
		Failed        int `json:"failed"`
		TotalMessages int `json:"total_messages"`
		NewMessages   int `json:"new_messages"`
		Channels      []struct {
			Channel struct{ ID, Name string }
			Status  string `json:"status"`
			Path    string `json:"path"`
		} `json:"channels"`
	}
	res := r.Run("guild", "export", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if j.Guild.Name != "Cooey COE" || j.Exported != 5 || j.UpToDate != 0 || j.Failed != 0 || j.TotalMessages != 15 || j.NewMessages != 15 || len(j.Channels) != 5 {
		t.Errorf("%+v", j)
	}
	for _, c := range j.Channels {
		if c.Status != "exported" || !strings.HasSuffix(c.Path, ".json") {
			t.Errorf("%+v", c)
		}
	}
}

func TestGuildExportRespectsConcurrency(t *testing.T) {
	r := guildExportRunner(t, 40*time.Millisecond)
	res := r.Run("guild", "export", "--concurrency", "2")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if m := r.Fake.MaxInFlight(); m > 2 {
		t.Errorf("max in flight %d with --concurrency 2", m)
	}

	r2 := guildExportRunner(t, 40*time.Millisecond)
	res = r2.Run("guild", "export")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if m := r2.Fake.MaxInFlight(); m < 2 || m > 4 {
		t.Errorf("max in flight %d with the default of 4", m)
	}
	res = r2.Run("guild", "export", "--concurrency", "0")
	if res.ExitCode != 2 {
		t.Errorf("concurrency 0 should be a usage error, got %d", res.ExitCode)
	}
}

func TestGuildExportThreads(t *testing.T) {
	r := guildExportRunner(t, 0)
	clitest.ServeMessages(r.Fake, "3001", clitest.Messages("3001", 2))
	clitest.ServeMessages(r.Fake, "3002", clitest.Messages("3002", 1))
	clitest.ServeMessages(r.Fake, "3003", clitest.Messages("3003", 3))
	res := r.Run("guild", "export", "--threads")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	for _, f := range []string{
		filepath.Join("threads", "general", "welcome-thread.json"),
		filepath.Join("threads", "general", "old-planning.json"),
		filepath.Join("threads", "help-forum", "how-do-i-scope.json"),
	} {
		e := readExport(t, filepath.Join(dir, f))
		if e.Channel.Type != 11 {
			t.Errorf("%s: type %d", f, e.Channel.Type)
		}
	}
	if !strings.Contains(res.Stdout, "8 exported") {
		t.Errorf("summary: %q", res.Stdout)
	}
	for _, req := range r.Fake.Requests() {
		if strings.Contains(req.Path, "/threads/active") || strings.Contains(req.Path, "/threads/archived") {
			t.Errorf("forbidden endpoint %s", req.Path)
		}
	}
	meta := readMeta(t, dir)
	if meta.Channels["3002"].MessageCount != 1 {
		t.Errorf("thread meta: %+v", meta.Channels)
	}
}

func TestGuildExportOneFailureKeepsOthers(t *testing.T) {
	r := guildExportRunner(t, 0)
	r.Fake.Queue("/channels/2003/messages", clitest.Response{Status: 500, Body: `{"message":"Internal Server Error","code":0}`})
	res := r.Run("guild", "export")
	if res.ExitCode != 1 {
		t.Errorf("exit %d, want 1: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "4 exported") || !strings.Contains(res.Stdout, "1 failed") || !strings.Contains(res.Stdout, "cmmc-general") {
		t.Errorf("summary: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "cmmc-general") || !strings.Contains(res.Stderr, "500") {
		t.Errorf("stderr should explain the failure: %q", res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	meta := readMeta(t, dir)
	if len(meta.Channels) != 4 {
		t.Errorf("meta should hold the 4 finished channels: %+v", meta.Channels)
	}
	if _, ok := meta.Channels["2003"]; ok {
		t.Errorf("failed channel recorded in meta")
	}
	if _, err := os.Stat(filepath.Join(dir, "cmmc-general.json")); err == nil {
		t.Errorf("failed channel has a file")
	}
	// Next run resumes: the finished channels are up to date, the failed one exports.
	r.Fake.Reset()
	res = r.Run("guild", "export")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "1 exported") || !strings.Contains(res.Stdout, "4 up to date") {
		t.Errorf("resume: %d %q", res.ExitCode, res.Stdout)
	}
	var j struct {
		Failed   int `json:"failed"`
		Channels []struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"channels"`
	}
	r.Fake.Queue("/channels/2007/messages", clitest.Response{Status: 500, Body: `{"message":"boom","code":0}`})
	res = r.Run("guild", "export", "--full", "--json")
	res.JSON(t, &j)
	if j.Failed != 1 {
		t.Errorf("json failed count: %+v", j)
	}
	found := false
	for _, c := range j.Channels {
		if c.Status == "failed" && strings.Contains(c.Error, "500") {
			found = true
		}
	}
	if !found {
		t.Errorf("failed channel missing from json: %+v", j.Channels)
	}
}

func TestGuildExportProgressOnlyOnTerminal(t *testing.T) {
	r := guildExportRunner(t, 0)
	r.StderrTTY = true
	res := r.Run("guild", "export")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "fetched") {
		t.Errorf("expected progress on a terminal stderr: %q", res.Stderr)
	}
}
