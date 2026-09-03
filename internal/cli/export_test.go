package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type exportFile struct {
	Guild   struct{ ID, Name string }
	Channel struct {
		ID, Name string
		Type     int
	}
	DateRange    struct{ After, Before *string }
	Messages     []map[string]any
	MessageCount int
}

func readExport(t *testing.T, path string) exportFile {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var e exportFile
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return e
}

type metaFile struct {
	Channels map[string]struct {
		LastMessageID string `json:"lastMessageId"`
		LastExport    string `json:"lastExport"`
		MessageCount  int    `json:"messageCount"`
	} `json:"channels"`
	LastExport string `json:"lastExport"`
}

func readMeta(t *testing.T, dir string) metaFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m metaFile
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func ids(msgs []map[string]any) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m["id"].(string)
	}
	return out
}

func TestChannelExportFreshWritesEnvelope(t *testing.T) {
	r, _ := readRunner(t, 5)
	res := r.Run("channel", "export", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	if !strings.Contains(res.Stdout, path) || !strings.Contains(res.Stdout, "5 messages") {
		t.Errorf("stdout: %q", res.Stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "channel_export.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		_ = os.WriteFile(golden, got, 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("export differs from golden %s:\n%s", golden, got)
	}
	// Key order matches the Node CLI: guild, channel, dateRange, messages, messageCount.
	s := string(got)
	if !(strings.Index(s, `"guild"`) < strings.Index(s, `"channel"`) && strings.Index(s, `"channel"`) < strings.Index(s, `"dateRange"`) && strings.Index(s, `"dateRange"`) < strings.Index(s, `"messages"`) && strings.Index(s, `"messages"`) < strings.Index(s, `"messageCount"`)) {
		t.Errorf("key order wrong")
	}
	e := readExport(t, path)
	if e.Guild.ID != "1001" || e.Guild.Name != "Cooey COE" || e.Channel.ID != "2001" || e.Channel.Name != "🔮general" || e.Channel.Type != 0 || e.MessageCount != 5 {
		t.Errorf("%+v", e)
	}
	if *e.DateRange.After != "2026-08-01T10:01:00.000000+00:00" || *e.DateRange.Before != "2026-08-01T10:05:00.000000+00:00" {
		t.Errorf("dateRange %v %v", *e.DateRange.After, *e.DateRange.Before)
	}
	meta := readMeta(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe"))
	if meta.Channels["2001"].LastMessageID != clitest.MessageID(5) || meta.Channels["2001"].MessageCount != 5 || meta.LastExport == "" {
		t.Errorf("meta %+v", meta)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("after") != "" || reqs[0].Query.Get("limit") != "100" {
		t.Errorf("fresh export requests: %+v", reqs)
	}
}

func TestChannelExportJSON(t *testing.T) {
	r, _ := readRunner(t, 3)
	var j struct {
		Guild        struct{ ID, Name string }
		Channel      struct{ ID, Name string }
		Path         string `json:"path"`
		MessageCount int    `json:"message_count"`
		NewMessages  int    `json:"new_messages"`
		Status       string `json:"status"`
	}
	res := r.Run("channel", "export", "general", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if j.Guild.Name != "Cooey COE" || j.Channel.ID != "2001" || j.MessageCount != 3 || j.NewMessages != 3 || j.Status != "exported" || !strings.HasSuffix(j.Path, "general.json") {
		t.Errorf("%+v", j)
	}
}

func TestChannelExportIncrementalPagesForwardWithAfterOnly(t *testing.T) {
	r, store := readRunner(t, 10)
	if res := r.Run("channel", "export", "general"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	for n := 11; n <= 260; n++ {
		store.Append(clitest.Message("2001", n))
	}
	r.Fake.Reset()
	res := r.Run("channel", "export", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "250 new") {
		t.Errorf("stdout: %q", res.Stdout)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 3 {
		t.Fatalf("want 3 pages, got %d", len(reqs))
	}
	wantAfter := []string{clitest.MessageID(10), clitest.MessageID(110), clitest.MessageID(210)}
	for i, q := range reqs {
		if q.Query.Get("after") != wantAfter[i] || q.Query.Get("before") != "" || q.Query.Get("limit") != "100" {
			t.Errorf("page %d query %v", i, q.Query)
		}
	}
	e := readExport(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json"))
	if e.MessageCount != 260 || len(e.Messages) != 260 {
		t.Fatalf("count %d/%d", e.MessageCount, len(e.Messages))
	}
	seen := map[string]bool{}
	prev := ""
	for _, id := range ids(e.Messages) {
		if seen[id] {
			t.Errorf("duplicate %s", id)
		}
		seen[id] = true
		if id <= prev {
			t.Errorf("not sorted: %s after %s", id, prev)
		}
		prev = id
	}
	meta := readMeta(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe"))
	if meta.Channels["2001"].LastMessageID != clitest.MessageID(260) || meta.Channels["2001"].MessageCount != 260 {
		t.Errorf("meta %+v", meta.Channels["2001"])
	}
}

func TestChannelExportUpToDateMakesOneRequest(t *testing.T) {
	r, _ := readRunner(t, 4)
	r.Run("channel", "export", "general")
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	before, _ := os.Stat(path)
	r.Fake.Reset()
	res := r.Run("channel", "export", "general")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "up to date") {
		t.Errorf("exit %d out %q err %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	if n := len(r.Fake.RequestsTo("/channels/2001/messages")); n != 1 {
		t.Errorf("requests %d, want 1", n)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("file rewritten when up to date")
	}
	var j struct {
		Status      string `json:"status"`
		NewMessages int    `json:"new_messages"`
	}
	r.Run("channel", "export", "general", "--json").JSON(t, &j)
	if j.Status != "up-to-date" || j.NewMessages != 0 {
		t.Errorf("%+v", j)
	}
}

func TestChannelExportFindsLegacyLocationByChannelID(t *testing.T) {
	r, store := readRunner(t, 3)
	legacyDir := filepath.Join(r.Home.NodeExportsDir(), "cooey-coe")
	// The legacy export for #🔮general has 2 messages and, confusingly, is
	// named general.json while cmmc-general.json also contains "general".
	generalPath := filepath.Join(legacyDir, "general.json")
	writeJSON(t, generalPath, clitest.NativeExport("1001", "Cooey COE", "2001", "🔮general", 0, clitest.Messages("2001", 2)))
	cmmcPath := filepath.Join(legacyDir, "cmmc-general.json")
	writeJSON(t, cmmcPath, clitest.NativeExport("1001", "Cooey COE", "2003", "cmmc-general", 0, clitest.Messages("2003", 7)))
	writeJSON(t, filepath.Join(legacyDir, ".meta.json"), map[string]any{
		"channels": map[string]any{
			"2001": map[string]any{"lastMessageId": clitest.MessageID(2), "lastExport": "2026-04-07T06:32:31.274Z", "messageCount": 2},
			"2003": map[string]any{"lastMessageId": clitest.MessageID(7), "lastExport": "2026-04-07T06:32:31.274Z", "messageCount": 7},
		},
		"lastExport": "2026-04-07T06:32:31.274Z",
	})
	cmmcBefore, _ := os.ReadFile(cmmcPath)
	store.Append() // server has messages 1..3

	res := r.Run("channel", "export", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	// Meta was read: only messages after id 2 were requested.
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("after") != clitest.MessageID(2) {
		t.Errorf("requests %+v", reqs)
	}
	// Updated in place in the legacy location, nothing written to XDG.
	e := readExport(t, generalPath)
	if e.MessageCount != 3 || strings.Join(ids(e.Messages), ",") != clitest.MessageID(1)+","+clitest.MessageID(2)+","+clitest.MessageID(3) {
		t.Errorf("legacy export: %d %v", e.MessageCount, ids(e.Messages))
	}
	if _, err := os.Stat(filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")); err == nil {
		t.Errorf("a new export was written instead of updating the legacy one")
	}
	if !strings.Contains(res.Stdout, generalPath) {
		t.Errorf("stdout should show the legacy path: %q", res.Stdout)
	}
	cmmcAfter, _ := os.ReadFile(cmmcPath)
	if string(cmmcBefore) != string(cmmcAfter) {
		t.Errorf("cmmc-general.json was touched")
	}
	meta := readMeta(t, legacyDir)
	if meta.Channels["2001"].LastMessageID != clitest.MessageID(3) || meta.Channels["2001"].MessageCount != 3 || meta.Channels["2003"].MessageCount != 7 || meta.LastExport == "2026-04-07T06:32:31.274Z" {
		t.Errorf("meta %+v", meta)
	}
}

func TestChannelExportNameCollisionAppendsID(t *testing.T) {
	r, _ := readRunner(t, 2)
	r.Fake.JSON("/guilds/1001/channels", clitest.ChannelsWithCollision())
	clitest.ServeMessages(r.Fake, "2011", clitest.Messages("2011", 3))
	if res := r.Run("channel", "export", "2001", "--no-cache"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	if res := r.Run("channel", "export", "2011"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	if e := readExport(t, filepath.Join(dir, "general.json")); e.Channel.ID != "2001" || e.MessageCount != 2 {
		t.Errorf("general.json overwritten: %+v", e.Channel)
	}
	if e := readExport(t, filepath.Join(dir, "general-2011.json")); e.Channel.ID != "2011" || e.MessageCount != 3 {
		t.Errorf("general-2011.json: %+v", e.Channel)
	}
	// Re-exporting the second channel finds its own file by id.
	res := r.Run("channel", "export", "2011")
	if !strings.Contains(res.Stdout, "up to date") {
		t.Errorf("second run: %q %q", res.Stdout, res.Stderr)
	}
}

func TestChannelExportFullRefetches(t *testing.T) {
	r, _ := readRunner(t, 6)
	r.Run("channel", "export", "general")
	r.Fake.Reset()
	res := r.Run("channel", "export", "general", "--full")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("after") != "" {
		t.Errorf("--full requests %+v", reqs)
	}
	e := readExport(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json"))
	if e.MessageCount != 6 {
		t.Errorf("count %d", e.MessageCount)
	}
	if !strings.Contains(res.Stdout, "6 new") {
		t.Errorf("stdout %q", res.Stdout)
	}
}

func TestChannelExportFailureLeavesPreviousExportIntact(t *testing.T) {
	r, store := readRunner(t, 3)
	r.Run("channel", "export", "general")
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	before, _ := os.ReadFile(path)
	metaBefore, _ := os.ReadFile(filepath.Join(r.Home.ExportsDir(), "cooey-coe", ".meta.json"))
	for n := 4; n <= 150; n++ {
		store.Append(clitest.Message("2001", n))
	}
	// First page succeeds, the second page fails hard.
	r.Fake.Queue("/channels/2001/messages",
		clitest.Response{Status: 200, Body: reverse(clitest.Messages("2001", 103)[3:])},
		clitest.Response{Status: 500, Body: `{"message":"Internal Server Error","code":0}`},
	)
	res := r.Run("channel", "export", "general")
	if res.ExitCode != 1 {
		t.Errorf("exit %d, want 1: %s", res.ExitCode, res.Stderr)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("export changed after a failed run")
	}
	metaAfter, _ := os.ReadFile(filepath.Join(r.Home.ExportsDir(), "cooey-coe", ".meta.json"))
	if string(metaBefore) != string(metaAfter) {
		t.Errorf("meta changed after a failed run")
	}
	entries, _ := os.ReadDir(filepath.Join(r.Home.ExportsDir(), "cooey-coe"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func reverse(msgs []map[string]any) []map[string]any {
	out := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		out[len(msgs)-1-i] = m
	}
	return out
}

func TestChannelExportNeverWritesLegacyDialect(t *testing.T) {
	r, _ := readRunner(t, 4)
	dcePath := filepath.Join(r.Home.ChatExporterDir(), "cooey-coe", "Cooey COE - general [2001].json")
	writeJSON(t, dcePath, clitest.LegacyExport("1001", "Cooey COE", "2001", "general", 2))
	dceBefore, _ := os.ReadFile(dcePath)
	res := r.Run("channel", "export", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	dceAfter, _ := os.ReadFile(dcePath)
	if string(dceBefore) != string(dceAfter) {
		t.Errorf("legacy export modified")
	}
	e := readExport(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json"))
	if e.MessageCount != 4 {
		t.Errorf("native export should hold all 4 messages, got %d", e.MessageCount)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("after") != "" {
		t.Errorf("should be a full fetch: %+v", reqs)
	}
}

func TestChannelExportThreadPath(t *testing.T) {
	r, _ := readRunner(t, 1)
	clitest.ServeMessages(r.Fake, "3001", clitest.Messages("3001", 2))
	res := r.Run("channel", "export", "welcome thread", "--threads")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "threads", "general", "welcome-thread.json")
	e := readExport(t, path)
	if e.Channel.ID != "3001" || e.Channel.Type != 11 || e.MessageCount != 2 {
		t.Errorf("%+v", e)
	}
	meta := readMeta(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe"))
	if meta.Channels["3001"].MessageCount != 2 {
		t.Errorf("thread meta missing: %+v", meta)
	}
}

func TestChannelExportUnknownChannelExits4(t *testing.T) {
	r, _ := readRunner(t, 1)
	res := r.Run("channel", "export", "nothing-here")
	if res.ExitCode != 4 {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
}
