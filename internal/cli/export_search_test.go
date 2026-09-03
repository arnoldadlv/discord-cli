package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type searchResult struct {
	Guild     struct{ ID, Name string }
	Channel   struct{ ID, Name string }
	ID        string `json:"id"`
	Author    string `json:"author"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
	File      string `json:"file"`
}

type searchOut struct {
	TotalMatches int            `json:"total_matches"`
	Shown        int            `json:"shown"`
	Results      []searchResult `json:"results"`
}

func TestExportSearchAllDialects(t *testing.T) {
	r := inventoryRunner(t)
	var j searchOut
	res := r.Run("export", "search", "policy", "--all", "--limit", "100", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	// native: general 3, cmmc-general 11, dm 1; legacy: access-control 4, azure 1
	if j.TotalMatches != 20 || j.Shown != 20 || len(j.Results) != 20 {
		t.Errorf("total %d shown %d", j.TotalMatches, j.Shown)
	}
	// Newest first: native 2026 results before legacy 2025 ones.
	if !strings.HasPrefix(j.Results[0].Timestamp, "2026-08-01") || !strings.HasPrefix(j.Results[19].Timestamp, "2025-03-01") {
		t.Errorf("order: %s ... %s", j.Results[0].Timestamp, j.Results[19].Timestamp)
	}
	sawLegacy, sawDM := false, false
	for _, x := range j.Results {
		if x.Author == "Tim" && x.Channel.Name == "access-control" && x.Guild.Name == "Cooey COE" && strings.Contains(x.Content, "legacy note") {
			sawLegacy = true
		}
		if x.Guild.ID == "@me" && x.Channel.Name == "kyle" && x.Author == "kyle" {
			sawDM = true
		}
		if x.File == "" || x.ID == "" {
			t.Errorf("missing file or id: %+v", x)
		}
	}
	if !sawLegacy || !sawDM {
		t.Errorf("legacy %v dm %v", sawLegacy, sawDM)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("export search must not talk to Discord")
	}
}

func TestExportSearchScoping(t *testing.T) {
	r := inventoryRunner(t)
	var j searchOut
	r.Run("export", "search", "policy", "--guild", "cooey-coe", "--limit", "100", "--json").JSON(t, &j)
	if j.TotalMatches != 18 {
		t.Errorf("cooey-coe: %d", j.TotalMatches)
	}
	r.Run("export", "search", "policy", "--guild", "dm", "--json").JSON(t, &j)
	if j.TotalMatches != 1 {
		t.Errorf("dm: %d", j.TotalMatches)
	}
	res := r.Run("export", "search", "policy")
	if res.ExitCode != 2 || !strings.Contains(res.Stderr, "--guild") || !strings.Contains(res.Stderr, "--all") {
		t.Errorf("neither: exit %d %q", res.ExitCode, res.Stderr)
	}
	res = r.Run("export", "search", "--all")
	if res.ExitCode != 2 {
		t.Errorf("no query: exit %d", res.ExitCode)
	}
	res = r.Run("export", "search", "policy", "--guild", "nowhere")
	if res.ExitCode != 5 {
		t.Errorf("unknown guild: exit %d", res.ExitCode)
	}
}

func TestExportSearchAuthorAndDates(t *testing.T) {
	r := inventoryRunner(t)
	var j searchOut
	r.Run("export", "search", "policy", "--all", "--author", "tim", "--json").JSON(t, &j)
	if j.TotalMatches != 5 {
		t.Errorf("author tim (legacy nickname): %d", j.TotalMatches)
	}
	r.Run("export", "search", "policy", "--all", "--author", "KYLE", "--limit", "100", "--json").JSON(t, &j)
	if j.TotalMatches != 15 {
		t.Errorf("author kyle (native username): %d", j.TotalMatches)
	}
	r.Run("export", "search", "policy", "--all", "--after", "2026-01-01", "--limit", "100", "--json").JSON(t, &j)
	if j.TotalMatches != 15 {
		t.Errorf("after 2026: %d", j.TotalMatches)
	}
	r.Run("export", "search", "policy", "--all", "--before", "2025-12-31", "--json").JSON(t, &j)
	if j.TotalMatches != 5 {
		t.Errorf("before 2026: %d", j.TotalMatches)
	}
	// Legacy timestamps carry a -07:00 offset: 09:00 PDT is 16:00 UTC.
	r.Run("export", "search", "note", "--all", "--after", "2025-03-01T16:02:30Z", "--json").JSON(t, &j)
	if j.TotalMatches != 4 { // access-control notes 4..7 (16:03 to 16:06 UTC); azure has only two notes
		t.Errorf("legacy offset window: %d", j.TotalMatches)
	}
	res := r.Run("export", "search", "policy", "--all", "--after", "soon")
	if res.ExitCode != 2 {
		t.Errorf("bad date: %d", res.ExitCode)
	}
}

func TestExportSearchAnyTerm(t *testing.T) {
	r := inventoryRunner(t)
	var j searchOut
	r.Run("export", "search", "policy evidence", "--guild", "dm", "--json").JSON(t, &j)
	if j.TotalMatches != 1 {
		t.Errorf("any term dm: %d", j.TotalMatches)
	}
	r.Run("export", "search", "policy evidence", "--guild", "Microsoft Azure", "--json").JSON(t, &j)
	if j.TotalMatches != 2 {
		t.Errorf("any term azure: %d", j.TotalMatches)
	}
}

func TestExportSearchHumanOutputAndLimit(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("export", "search", "policy", "--all", "--limit", "3")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	out := res.Stdout
	for _, want := range []string{"20 matches", "showing 3", "#cmmc-general", "Cooey COE", "17 more", "--limit"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if countMessages(out) != 3 {
		t.Errorf("shown %d", countMessages(out))
	}
	res = r.Run("export", "search", "nothing-matches-this", "--all")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "No matches") {
		t.Errorf("zero: %d %q", res.ExitCode, res.Stdout)
	}
}

func TestExportSearchJSONShapeSnapshot(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("export", "search", "policy", "--guild", "dm", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	got := strings.ReplaceAll(res.Stdout, r.Home.Dir, "$HOME")
	golden := filepath.Join("testdata", "export_search.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		_ = os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("result JSON shape changed:\n%s", got)
	}
}

func TestExportSearchNoExportsExits5(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("export", "search", "x", "--all")
	if res.ExitCode != 5 {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
}
