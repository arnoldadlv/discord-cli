package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func TestDMExportFreshAndGolden(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 4))
	res := r.Run("dm", "export", "kyle")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	path := filepath.Join(r.Home.ExportsDir(), "dm", "kyle.json")
	if !strings.Contains(res.Stdout, path) || !strings.Contains(res.Stdout, "4 messages") {
		t.Errorf("stdout %q", res.Stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "dm_export.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		_ = os.WriteFile(golden, got, 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("dm export differs from golden:\n%s", got)
	}
	e := readExport(t, path)
	if e.Guild.ID != "@me" || e.Guild.Name != "DM" || e.Channel.ID != "6001" || e.Channel.Name != "kyle" || e.Channel.Type != 1 || e.MessageCount != 4 {
		t.Errorf("%+v", e)
	}
	meta := readMeta(t, filepath.Join(r.Home.ExportsDir(), "dm"))
	if meta.Channels["6001"].LastMessageID != clitest.MessageID(4) || meta.Channels["6001"].MessageCount != 4 {
		t.Errorf("meta %+v", meta)
	}
	var j struct {
		Guild        struct{ ID, Name string }
		Channel      struct{ ID, Name string }
		Status       string `json:"status"`
		MessageCount int    `json:"message_count"`
	}
	r.Run("dm", "export", "kyle", "--json").JSON(t, &j)
	if j.Guild.ID != "@me" || j.Channel.Name != "kyle" || j.Status != "up-to-date" || j.MessageCount != 4 {
		t.Errorf("%+v", j)
	}
}

func TestDMExportIncremental(t *testing.T) {
	r := dmRunner(t)
	store := clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 3))
	r.Run("dm", "export", "kyle")
	for n := 4; n <= 130; n++ {
		store.Append(clitest.Message("6001", n))
	}
	r.Fake.Reset()
	res := r.Run("dm", "export", "kyle")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "127 new") {
		t.Errorf("exit %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	reqs := r.Fake.RequestsTo("/channels/6001/messages")
	if len(reqs) != 2 || reqs[0].Query.Get("after") != clitest.MessageID(3) || reqs[1].Query.Get("after") != clitest.MessageID(103) {
		t.Errorf("requests %+v", reqs)
	}
	e := readExport(t, filepath.Join(r.Home.ExportsDir(), "dm", "kyle.json"))
	if e.MessageCount != 130 || e.Messages[129]["id"] != clitest.MessageID(130) {
		t.Errorf("count %d", e.MessageCount)
	}
}

func TestDMExportGroupAndCollision(t *testing.T) {
	r := dmRunner(t)
	r.Fake.JSON("/users/@me/channels", clitest.DMsWithCollision())
	clitest.ServeMessages(r.Fake, "6002", clitest.Messages("6002", 2))
	clitest.ServeMessages(r.Fake, "6003", clitest.Messages("6003", 3))
	clitest.ServeMessages(r.Fake, "6005", clitest.Messages("6005", 5))
	if res := r.Run("dm", "export", "Study Group", "--no-cache"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	if res := r.Run("dm", "export", "maria"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	if res := r.Run("dm", "export", "6005"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	dir := filepath.Join(r.Home.ExportsDir(), "dm")
	if e := readExport(t, filepath.Join(dir, "study-group.json")); e.Channel.ID != "6002" || e.Channel.Type != 3 || e.Channel.Name != "Study Group" {
		t.Errorf("group: %+v", e.Channel)
	}
	if e := readExport(t, filepath.Join(dir, "maria.json")); e.Channel.ID != "6003" || e.MessageCount != 3 {
		t.Errorf("maria.json: %+v", e.Channel)
	}
	if e := readExport(t, filepath.Join(dir, "maria-6005.json")); e.Channel.ID != "6005" || e.MessageCount != 5 {
		t.Errorf("maria-6005.json: %+v", e.Channel)
	}
	meta := readMeta(t, dir)
	if len(meta.Channels) != 3 {
		t.Errorf("meta %+v", meta.Channels)
	}
	res := r.Run("dm", "export", "6005")
	if !strings.Contains(res.Stdout, "up to date") {
		t.Errorf("collision file not found by id on rerun: %q", res.Stdout)
	}
}

func TestDMExportUnknownExits4(t *testing.T) {
	r := dmRunner(t)
	res := r.Run("dm", "export", "nobody")
	if res.ExitCode != 4 {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
}
