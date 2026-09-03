package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

var messageHeader = regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2} \d{2}:\d{2}  `)

// countMessages counts rendered messages by their timestamp header line.
func countMessages(out string) int { return len(messageHeader.FindAllString(out, -1)) }

func readRunner(t *testing.T, n int) (*clitest.Runner, *clitest.MessageStore) {
	t.Helper()
	r := channelRunner(t)
	s := clitest.ServeMessages(r.Fake, "2001", clitest.Messages("2001", n))
	return r, s
}

func TestChannelReadDefault25OldestFirst(t *testing.T) {
	r, _ := readRunner(t, 40)
	res := r.Run("channel", "read", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.Stdout, "message 15 ") || !strings.Contains(res.Stdout, "message 16 ") || !strings.Contains(res.Stdout, "message 40 ") {
		t.Errorf("wrong window:\n%s", res.Stdout)
	}
	if strings.Index(res.Stdout, "message 16 ") > strings.Index(res.Stdout, "message 40 ") {
		t.Errorf("not oldest first:\n%s", res.Stdout)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("limit") != "25" || reqs[0].Query.Get("before") != "" || reqs[0].Query.Get("after") != "" {
		t.Errorf("requests: %+v", reqs)
	}
}

func TestChannelReadLimitFiveAndThreePages(t *testing.T) {
	r, _ := readRunner(t, 300)
	res := r.Run("channel", "read", "general", "--limit", "5")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if countMessages(res.Stdout) != 5 || !strings.Contains(res.Stdout, "message 296 ") {
		t.Errorf("limit 5:\n%s", res.Stdout)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("limit") != "5" {
		t.Errorf("limit 5 requests: %+v", reqs)
	}

	r.Fake.Reset()
	var j struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	res = r.Run("channel", "read", "general", "--limit", "250", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if len(j.Messages) != 250 || j.Messages[0].ID != clitest.MessageID(51) || j.Messages[249].ID != clitest.MessageID(300) {
		t.Errorf("250: got %d, first %s last %s", len(j.Messages), j.Messages[0].ID, j.Messages[len(j.Messages)-1].ID)
	}
	reqs = r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 3 {
		t.Fatalf("want 3 pages, got %d", len(reqs))
	}
	if reqs[0].Query.Get("before") != "" || reqs[1].Query.Get("before") != clitest.MessageID(201) || reqs[2].Query.Get("before") != clitest.MessageID(101) {
		t.Errorf("before sequence: %v %v %v", reqs[0].Query, reqs[1].Query, reqs[2].Query)
	}
	for _, q := range reqs {
		if q.Query.Get("after") != "" {
			t.Errorf("after sent on a read")
		}
	}
	if reqs[0].Query.Get("limit") != "100" || reqs[2].Query.Get("limit") != "50" {
		t.Errorf("limits: %v %v", reqs[0].Query, reqs[2].Query)
	}
}

func TestChannelReadRendersAttachmentsEmbedsReactions(t *testing.T) {
	r, _ := readRunner(t, 5)
	res := r.Run("channel", "read", "📰news")
	if res.ExitCode == 0 {
		t.Fatalf("news has no messages served, expected a failure or empty; got %q", res.Stdout)
	}
	res = r.Run("channel", "read", "2001")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	out := res.Stdout
	for _, want := range []string{
		"Ana", "Kyle B", "newsbot",
		"report.pdf", "https://cdn.example.test/report.pdf",
		"Weekly digest", "Three things happened this week.", "https://news.example.test/digest",
		"👍 3", "🎉 1",
		"2026-08-01",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("embed html not stripped:\n%s", out)
	}
}

func TestChannelReadJSONShapeSnapshot(t *testing.T) {
	r, _ := readRunner(t, 5)
	res := r.Run("channel", "read", "general", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	golden := filepath.Join("testdata", "channel_read.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(res.Stdout), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden missing (run with UPDATE_GOLDEN=1): %v", err)
	}
	if string(want) != res.Stdout {
		t.Errorf("JSON shape changed; diff against %s:\n%s", golden, res.Stdout)
	}
	var j struct {
		Guild   struct{ ID, Name string }
		Channel struct {
			ID, Name string
			Type     int
		}
		Messages []json.RawMessage
	}
	res.JSON(t, &j)
	if j.Guild.Name != "Cooey COE" || j.Channel.Name != "🔮general" || j.Channel.Type != 0 || len(j.Messages) != 5 {
		t.Errorf("%+v", j)
	}
	var raw map[string]any
	if err := json.Unmarshal(j.Messages[1], &raw); err != nil {
		t.Fatal(err)
	}
	if raw["timestamp"] != "2026-08-01T10:02:00.000000+00:00" || raw["attachments"] == nil {
		t.Errorf("raw message altered: %v", raw)
	}
}

func TestChannelReadEmptyChannel(t *testing.T) {
	r, _ := readRunner(t, 0)
	res := r.Run("channel", "read", "general")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "No messages") {
		t.Errorf("%d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	var j struct {
		Messages []any `json:"messages"`
	}
	r.Run("channel", "read", "general", "--json").JSON(t, &j)
	if j.Messages == nil || len(j.Messages) != 0 {
		t.Errorf("messages should be an empty array")
	}
}

func TestChannelReadResolutionAndSuggestions(t *testing.T) {
	r, _ := readRunner(t, 3)
	for _, input := range []string{"2001", "🔮general", "🔮GENERAL", "general", "GENERAL"} {
		res := r.Run("channel", "read", input)
		if res.ExitCode != 0 {
			t.Errorf("%q: exit %d: %s", input, res.ExitCode, res.Stderr)
		}
	}
	res := r.Run("channel", "read", "gen")
	if res.ExitCode != 4 {
		t.Fatalf("exit %d, want 4: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, `channel "gen" not found`) || !strings.Contains(res.Stderr, "🔮general") || !strings.Contains(res.Stderr, "cmmc-general") {
		t.Errorf("stderr: %q", res.Stderr)
	}
	res = r.Run("channel", "read")
	if res.ExitCode != 2 {
		t.Errorf("missing positional: exit %d", res.ExitCode)
	}
	res = r.Run("channel", "read", "a", "b")
	if res.ExitCode != 2 {
		t.Errorf("two positionals: exit %d", res.ExitCode)
	}
}

func TestChannelReadThreadWithThreadsFlag(t *testing.T) {
	r, _ := readRunner(t, 3)
	clitest.ServeMessages(r.Fake, "3001", clitest.Messages("3001", 2))
	res := r.Run("channel", "read", "welcome thread")
	if res.ExitCode != 4 {
		t.Errorf("thread without --threads should not resolve: %d", res.ExitCode)
	}
	res = r.Run("channel", "read", "welcome thread", "--threads")
	if res.ExitCode != 0 || countMessages(res.Stdout) != 2 || !strings.Contains(res.Stdout, "message 1 ") {
		t.Errorf("exit %d: %s %s", res.ExitCode, res.Stdout, res.Stderr)
	}
}

func TestChannelReadFlagsInAnyOrder(t *testing.T) {
	r, _ := readRunner(t, 3)
	for _, args := range [][]string{
		{"channel", "read", "general", "--limit", "2"},
		{"channel", "read", "--limit", "2", "general"},
		{"--limit", "2", "channel", "read", "general"},
		{"channel", "--limit=2", "read", "general"},
	} {
		res := r.Run(args...)
		if res.ExitCode != 0 || countMessages(res.Stdout) != 2 {
			t.Errorf("%v: exit %d out %q err %q", args, res.ExitCode, res.Stdout, res.Stderr)
		}
	}
}
