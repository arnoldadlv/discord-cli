package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// inventoryRunner lays out exports in all three read locations: a native
// one in the XDG directory with meta, a native one from the Node CLI with
// its meta, a DM export, and two legacy DiscordChatExporter exports.
func inventoryRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := channelRunner(t)
	xdg := filepath.Join(r.Home.ExportsDir(), "cooey-coe")
	writeJSON(t, filepath.Join(xdg, "general.json"), clitest.NativeExport("1001", "Cooey COE", "2001", "🔮general", 0, clitest.Messages("2001", 12)))
	writeJSON(t, filepath.Join(xdg, ".meta.json"), map[string]any{
		"channels":   map[string]any{"2001": map[string]any{"lastMessageId": clitest.MessageID(12), "lastExport": "2026-09-01T08:00:00.000Z", "messageCount": 12}},
		"lastExport": "2026-09-01T08:00:00.000Z",
	})
	writeJSON(t, filepath.Join(r.Home.ExportsDir(), "dm", "kyle.json"), clitest.NativeExport("@me", "DM", "6001", "kyle", 1, clitest.Messages("6001", 3)))

	legacy := filepath.Join(r.Home.NodeExportsDir(), "cooey-coe")
	writeJSON(t, filepath.Join(legacy, "cmmc-general.json"), clitest.NativeExport("1001", "Cooey COE", "2003", "cmmc-general", 0, clitest.Messages("2003", 40)))
	writeJSON(t, filepath.Join(legacy, ".meta.json"), map[string]any{
		"channels":   map[string]any{"2003": map[string]any{"lastMessageId": clitest.MessageID(40), "lastExport": "2026-05-07T21:31:08.502Z", "messageCount": 40}},
		"lastExport": "2026-05-07T21:31:08.502Z",
	})

	dce := r.Home.ChatExporterDir()
	writeJSON(t, filepath.Join(dce, "cooey-coe", "Cooey COE - access-control [2020].json"), clitest.LegacyExport("1001", "Cooey COE", "2020", "access-control", 7))
	writeJSON(t, filepath.Join(dce, "microsoft-azure", "Microsoft Azure - general [8001].json"), clitest.LegacyExport("8000", "Microsoft Azure", "8001", "general", 2))
	// Not an export: must be ignored, not crash.
	r.Home.WriteFile(t, filepath.Join(xdg, "notes.json"), []byte(`{"hello":"world"}`), 0o644)
	return r
}

func TestExportListAllLocationsAndDialects(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("export", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"Cooey COE", "🔮general", "cmmc-general", "access-control", "Microsoft Azure", "kyle", "DM", "native", "legacy", "node", "chatexporter", "12", "40", "2026-09-01", "2026-05-07"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	if strings.Contains(res.Stdout, "notes") {
		t.Errorf("non-export listed")
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("export list must not talk to Discord")
	}

	var j []struct {
		Guild        struct{ ID, Name string }
		Channel      struct{ ID, Name string }
		Path         string `json:"path"`
		Location     string `json:"location"`
		Dialect      string `json:"dialect"`
		MessageCount int    `json:"message_count"`
		DateRange    struct {
			After  *string `json:"after"`
			Before *string `json:"before"`
		} `json:"date_range"`
		LastExport *string `json:"last_export"`
	}
	r.Run("export", "list", "--json").JSON(t, &j)
	if len(j) != 5 {
		t.Fatalf("want 5 exports, got %d: %+v", len(j), j)
	}
	byID := map[string]int{}
	for i, e := range j {
		byID[e.Channel.ID] = i
	}
	g := j[byID["2001"]]
	if g.Dialect != "native" || g.Location != "xdg" || g.MessageCount != 12 || g.LastExport == nil || *g.LastExport != "2026-09-01T08:00:00.000Z" || g.DateRange.After == nil {
		t.Errorf("general: %+v", g)
	}
	c := j[byID["2003"]]
	if c.Dialect != "native" || c.Location != "node" || c.MessageCount != 40 || c.LastExport == nil {
		t.Errorf("cmmc: %+v", c)
	}
	d := j[byID["2020"]]
	if d.Dialect != "legacy" || d.Location != "chatexporter" || d.MessageCount != 7 || d.LastExport != nil || d.Guild.Name != "Cooey COE" {
		t.Errorf("dce: %+v", d)
	}
	k := j[byID["6001"]]
	if k.Guild.ID != "@me" || k.Guild.Name != "DM" || k.MessageCount != 3 {
		t.Errorf("dm: %+v", k)
	}
	if j[0].Guild.Name > j[len(j)-1].Guild.Name {
		t.Errorf("not sorted by guild: %+v", j)
	}
}

func TestExportListGuildFilter(t *testing.T) {
	r := inventoryRunner(t)
	var j []struct {
		Channel struct{ ID string }
	}
	r.Run("export", "list", "--guild", "cooey-coe", "--json").JSON(t, &j)
	if len(j) != 3 {
		t.Errorf("cooey-coe: %d exports", len(j))
	}
	r.Run("export", "list", "--guild", "Microsoft Azure", "--json").JSON(t, &j)
	if len(j) != 1 || j[0].Channel.ID != "8001" {
		t.Errorf("azure: %+v", j)
	}
	r.Run("export", "list", "--guild", "dm", "--json").JSON(t, &j)
	if len(j) != 1 || j[0].Channel.ID != "6001" {
		t.Errorf("dm: %+v", j)
	}
	res := r.Run("export", "list", "--guild", "nowhere")
	if res.ExitCode != 5 || !strings.Contains(res.Stderr, "nowhere") {
		t.Errorf("unknown guild: exit %d %q", res.ExitCode, res.Stderr)
	}
}

func TestExportListNothingExits5(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("export", "list")
	if res.ExitCode != 5 || !strings.Contains(res.Stderr, "export") {
		t.Errorf("exit %d %q", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "" {
		t.Errorf("stdout %q", res.Stdout)
	}
}

func TestGuildShowReportsExportStatus(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("guild", "show")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	out := res.Stdout
	for _, want := range []string{"Exports", "🔮general", "12 messages", "2026-09-01", "cmmc-general", "40 messages", "📰news", "no export", "help-forum", "random"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	var j struct {
		ChannelCount int `json:"channel_count"`
		Exports      []struct {
			Channel      struct{ ID, Name string }
			Exported     bool    `json:"exported"`
			Path         string  `json:"path"`
			Location     string  `json:"location"`
			Dialect      string  `json:"dialect"`
			MessageCount int     `json:"message_count"`
			LastExport   *string `json:"last_export"`
		} `json:"exports"`
	}
	r.Run("guild", "show", "--json").JSON(t, &j)
	if len(j.Exports) != 5 {
		t.Fatalf("exports: %+v", j.Exports)
	}
	for _, e := range j.Exports {
		switch e.Channel.ID {
		case "2001":
			if !e.Exported || e.Location != "xdg" || e.MessageCount != 12 || e.LastExport == nil {
				t.Errorf("general: %+v", e)
			}
		case "2003":
			if !e.Exported || e.Location != "node" || e.MessageCount != 40 {
				t.Errorf("cmmc: %+v", e)
			}
		default:
			if e.Exported || e.Path != "" {
				t.Errorf("%s should have no export: %+v", e.Channel.Name, e)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(r.Home.ExportsDir(), "cooey-coe", "notes.json")); err != nil {
		t.Errorf("stray file removed")
	}
}
