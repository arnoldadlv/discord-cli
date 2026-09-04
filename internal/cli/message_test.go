package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// messageWindow is the JSON a message read emits.
type messageWindow struct {
	Guild *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"guild"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Source   string `json:"source"`
	File     string `json:"file"`
	Messages []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
		Match bool `json:"match"`
	} `json:"messages"`
}

// window decodes one message read into a fresh value, which JSON reuse of
// an old one would otherwise leave stale fields in.
func window(t *testing.T, res clitest.Result) messageWindow {
	t.Helper()
	var w messageWindow
	res.JSON(t, &w)
	return w
}

// ids is the message ids of a window, in the order they were printed.
func (w messageWindow) ids() []string {
	out := make([]string, len(w.Messages))
	for i, m := range w.Messages {
		out[i] = m.ID
	}
	return out
}

// matched is the id of the message marked as the one asked for.
func (w messageWindow) matched(t *testing.T) string {
	t.Helper()
	found := ""
	for _, m := range w.Messages {
		if m.Match {
			if found != "" {
				t.Errorf("more than one message marked: %s and %s", found, m.ID)
			}
			found = m.ID
		}
	}
	if found == "" {
		t.Errorf("no message marked in %v", w.ids())
	}
	return found
}

// fixtureMessages builds the fixture messages numbered from through to,
// so exports in one test can hold ranges of ids that do not overlap.
func fixtureMessages(channelID string, from, to int) []map[string]any {
	var out []map[string]any
	for n := from; n <= to; n++ {
		out = append(out, clitest.Message(channelID, n))
	}
	return out
}

// messageRunner lays out a guild export (ids 5000001 to 5000012), a DM
// export (5000101 to 5000105), and a legacy export (4000001 to 4000007).
// Nothing is indexed yet.
func messageRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := channelRunner(t)
	writeJSON(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json"),
		clitest.NativeExport("1001", "Cooey COE", "2001", "🔮general", 0, fixtureMessages("2001", 1, 12)))
	writeJSON(t, filepath.Join(r.Home.ExportsDir(), "dm", "kyle.json"),
		clitest.NativeExport("@me", "DM", "6001", "kyle", 1, fixtureMessages("6001", 101, 105)))
	writeJSON(t, filepath.Join(r.Home.ChatExporterDir(), "cooey-coe", "Cooey COE - access-control [2020].json"),
		clitest.LegacyExport("1001", "Cooey COE", "2020", "access-control", 7))
	return r
}

// indexedMessageRunner is messageRunner with the search index built, which
// is the state after any export.
func indexedMessageRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := messageRunner(t)
	if res := r.Run("cache", "rebuild"); res.ExitCode != 0 {
		t.Fatalf("cache rebuild: exit %d: %s", res.ExitCode, res.Stderr)
	}
	r.Fake.Reset()
	return r
}

func TestMessageReadResolvesThroughTheIndexWithoutNetwork(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", clitest.MessageID(6), "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	w := window(t, res)
	if len(w.Messages) != 1 || w.Messages[0].ID != clitest.MessageID(6) {
		t.Fatalf("window: %v", w.ids())
	}
	if w.matched(t) != clitest.MessageID(6) {
		t.Errorf("wrong message marked")
	}
	if w.Source != "export" || !strings.HasSuffix(w.File, "general.json") {
		t.Errorf("source %q file %q", w.Source, w.File)
	}
	if w.Guild == nil || w.Guild.Name != "Cooey COE" || w.Channel.ID != "2001" || w.Channel.Name != "🔮general" {
		t.Errorf("guild and channel: %+v %+v", w.Guild, w.Channel)
	}
	if !strings.Contains(w.Messages[0].Content, "message 6 ") {
		t.Errorf("content: %q", w.Messages[0].Content)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("message read through the index must not talk to Discord: %+v", r.Fake.Requests())
	}
	wantNotice := "Read from the export at ~/.local/share/discord-cli/exports/cooey-coe/general.json, which covers messages up to 2026-08-01. Newer messages are not in it.\n"
	if res.Stderr != wantNotice {
		t.Errorf("stderr:\ngot:  %q\nwant: %q", res.Stderr, wantNotice)
	}

	// Human output marks the message with a leading > on its timestamp line,
	// so the two neighbours are the only lines the plain header matches.
	res = r.Run("message", "read", clitest.MessageID(5), "--context", "1")
	if res.ExitCode != 0 {
		t.Fatalf("human: exit %d: %s", res.ExitCode, res.Stderr)
	}
	if countMessages(res.Stdout) != 2 || !strings.Contains(res.Stdout, "message 4 ") || !strings.Contains(res.Stdout, "message 6 ") {
		t.Errorf("human window:\n%s", res.Stdout)
	}
	marked := 0
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(line, "> ") {
			marked++
			if !strings.Contains(line, ":") {
				t.Errorf("the mark is not on a timestamp line: %q", line)
			}
		}
	}
	if marked != 1 {
		t.Errorf("marked %d lines in\n%s", marked, res.Stdout)
	}
}

func TestMessageReadContextIsOldestFirstAndClamps(t *testing.T) {
	r := indexedMessageRunner(t)

	w := window(t, r.Run("message", "read", clitest.MessageID(6), "--context", "2", "--json"))
	want := []string{clitest.MessageID(4), clitest.MessageID(5), clitest.MessageID(6), clitest.MessageID(7), clitest.MessageID(8)}
	if strings.Join(w.ids(), ",") != strings.Join(want, ",") {
		t.Errorf("2N+1 oldest first: %v", w.ids())
	}
	if w.matched(t) != clitest.MessageID(6) {
		t.Errorf("marked %s", w.matched(t))
	}

	// The first message of the channel: nothing before it.
	w = window(t, r.Run("message", "read", clitest.MessageID(1), "--context", "3", "--json"))
	if len(w.Messages) != 4 || w.ids()[0] != clitest.MessageID(1) || w.matched(t) != clitest.MessageID(1) {
		t.Errorf("start of channel: %v", w.ids())
	}

	// The last message of the channel: nothing after it.
	w = window(t, r.Run("message", "read", clitest.MessageID(12), "--context", "3", "--json"))
	if len(w.Messages) != 4 || w.ids()[3] != clitest.MessageID(12) || w.matched(t) != clitest.MessageID(12) {
		t.Errorf("end of channel: %v", w.ids())
	}

	// A context wider than the channel returns the channel.
	w = window(t, r.Run("message", "read", clitest.MessageID(6), "--context", "50", "--json"))
	if len(w.Messages) != 12 {
		t.Errorf("whole channel: %d", len(w.Messages))
	}
	if res := r.Run("message", "read", clitest.MessageID(6), "--context", "-1"); res.ExitCode != 2 {
		t.Errorf("negative context: exit %d", res.ExitCode)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("context reads must not talk to Discord")
	}
}

func TestMessageReadScansWhenTheIndexDoesNotCoverTheExports(t *testing.T) {
	// No index at all: the same notice local search gives, and the message
	// still resolves.
	r := messageRunner(t)
	res := r.Run("message", "read", clitest.MessageID(6), "--context", "1", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "No search index yet; scanning exports instead.") {
		t.Errorf("notice: %q", res.Stderr)
	}
	w := window(t, res)
	if len(w.Messages) != 3 || w.matched(t) != clitest.MessageID(6) {
		t.Errorf("window: %v", w.ids())
	}

	// An export written after the index was built is not indexed yet: the
	// scan finds the message and stderr says the index is out of date.
	r = indexedMessageRunner(t)
	writeJSON(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "random.json"),
		clitest.NativeExport("1001", "Cooey COE", "2007", "random", 0, fixtureMessages("2007", 201, 205)))
	res = r.Run("message", "read", clitest.MessageID(203), "--context", "1", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("unindexed: exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "out of date") || !strings.Contains(res.Stderr, "scanning exports instead") {
		t.Errorf("notice: %q", res.Stderr)
	}
	w = window(t, res)
	if len(w.Messages) != 3 || w.matched(t) != clitest.MessageID(203) || w.Channel.Name != "random" {
		t.Errorf("window: %v in %s", w.ids(), w.Channel.Name)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("the scan must not talk to Discord")
	}
}

func TestMessageReadResolvesADMMessageTheSameWay(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", clitest.MessageID(103), "--context", "1", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	w := window(t, res)
	if len(w.Messages) != 3 || w.matched(t) != clitest.MessageID(103) {
		t.Errorf("window: %v", w.ids())
	}
	if w.Channel.ID != "6001" || w.Channel.Name != "kyle" || w.Guild == nil || w.Guild.ID != "@me" {
		t.Errorf("DM channel: %+v %+v", w.Channel, w.Guild)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("a DM message in an export must not talk to Discord")
	}
}

func TestMessageReadResolvesALegacyExportMessage(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", "4000003", "--context", "1", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	w := window(t, res)
	if len(w.Messages) != 3 || w.matched(t) != "4000003" {
		t.Fatalf("window: %v", w.ids())
	}
	if w.Channel.Name != "access-control" {
		t.Errorf("channel: %+v", w.Channel)
	}
	for _, m := range w.Messages {
		if m.Author.Name != "Tim" || !strings.Contains(m.Content, "legacy note") {
			t.Errorf("legacy message: %+v", m)
		}
	}
}

func TestMessageReadFallsBackToDiscordWithAChannel(t *testing.T) {
	r := indexedMessageRunner(t)
	clitest.ServeMessages(r.Fake, "2001", fixtureMessages("2001", 1, 40))
	res := r.Run("message", "read", clitest.MessageID(30), "--channel", "general", "--context", "2", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	w := window(t, res)
	want := []string{clitest.MessageID(28), clitest.MessageID(29), clitest.MessageID(30), clitest.MessageID(31), clitest.MessageID(32)}
	if strings.Join(w.ids(), ",") != strings.Join(want, ",") {
		t.Errorf("window: %v", w.ids())
	}
	if w.matched(t) != clitest.MessageID(30) || w.Source != "discord" || w.File != "" {
		t.Errorf("source %q file %q", w.Source, w.File)
	}
	if w.Channel.ID != "2001" || w.Channel.Name != "🔮general" || w.Guild == nil || w.Guild.ID != "1001" {
		t.Errorf("channel: %+v %+v", w.Channel, w.Guild)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 {
		t.Fatalf("requests: %+v", reqs)
	}
	q := reqs[0].Query
	if q.Get("around") != clitest.MessageID(30) || q.Get("before") != "" || q.Get("after") != "" || q.Get("limit") != "5" {
		t.Errorf("query: %v", q)
	}

	// A channel id needs no guild, which is how a DM is read.
	r.Fake.Reset()
	res = r.Run("message", "read", clitest.MessageID(30), "--channel", "2001", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("by id: exit %d: %s", res.ExitCode, res.Stderr)
	}
	w = window(t, res)
	if len(w.Messages) != 1 || w.matched(t) != clitest.MessageID(30) {
		t.Errorf("by id window: %v", w.ids())
	}
	reqs = r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("around") != clitest.MessageID(30) {
		t.Errorf("by id requests: %+v", reqs)
	}

	// An id the channel does not hold is not found.
	if res := r.Run("message", "read", "5009999", "--channel", "2001"); res.ExitCode != 4 {
		t.Errorf("unknown id in a known channel: exit %d %q", res.ExitCode, res.Stderr)
	}
}

func TestMessageReadWithoutAChannelExits4NamingBothWaysForward(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", "5009999")
	if res.ExitCode != 4 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"no export", "--channel", "channel export"} {
		if !strings.Contains(res.Stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, res.Stderr)
		}
	}
	if res.Stdout != "" {
		t.Errorf("stdout: %q", res.Stdout)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("nothing to fetch, so nothing should be fetched")
	}

	// With no exports at all the answer is the same.
	bare := channelRunner(t)
	if res := bare.Run("message", "read", "5009999"); res.ExitCode != 4 {
		t.Errorf("no exports: exit %d %q", res.ExitCode, res.Stderr)
	}

	// An argument that is not a message id is a usage error.
	if res := r.Run("message", "read", "general"); res.ExitCode != 2 {
		t.Errorf("not an id: exit %d %q", res.ExitCode, res.Stderr)
	}
	if res := r.Run("message", "read"); res.ExitCode != 2 {
		t.Errorf("no id: exit %d", res.ExitCode)
	}
}

func TestMessageHelpAndSkillCoverTheReadPair(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("help", "message", "read")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"discord message read <id>", "--context", "--channel", "export search"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("help missing %q:\n%s", want, res.Stdout)
		}
	}
	if res := r.Run("message"); res.ExitCode != 0 || !strings.Contains(res.Stdout, "discord message") {
		t.Errorf("bare noun: %d %q", res.ExitCode, res.Stdout)
	}
	if res := r.Run(); !strings.Contains(res.Stdout, "message") {
		t.Errorf("short help does not list the message noun:\n%s", res.Stdout)
	}
	skill := r.Run("help", "--skill").Stdout
	for _, want := range []string{"message read", "--context", "\"match\": true"} {
		if !strings.Contains(skill, want) {
			t.Errorf("skill missing %q", want)
		}
	}
	// The skill shows search then read as one loop: the read follows a
	// search in the same block.
	if !strings.Contains(skill, "discord export search") || !strings.Contains(skill, "discord message read <id from a search hit>") {
		t.Errorf("skill does not show the search then read pair")
	}
}
