package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func TestExportSearchIndexMatchesScanExactly(t *testing.T) {
	r := inventoryRunner(t)
	queries := [][]string{
		{"policy", "--all", "--limit", "100"},
		{"policy evidence", "--guild", "cooey-coe", "--limit", "100"},
		{"note", "--all", "--author", "tim", "--after", "2025-03-01T16:02:30Z"},
		{"ol", "--all", "--limit", "100"}, // a two-letter term must still be a substring match
		{"MESSAGE", "--all", "--before", "2026-08-01T10:05:00Z", "--limit", "100"},
	}
	var scanned []string
	for _, q := range queries {
		res := r.Run(append([]string{"export", "search", "--json"}, q...)...)
		if res.ExitCode != 0 {
			t.Fatalf("%v: exit %d: %s", q, res.ExitCode, res.Stderr)
		}
		if !strings.Contains(res.Stderr, "scan") {
			t.Errorf("%v: expected the fallback notice without an index: %q", q, res.Stderr)
		}
		scanned = append(scanned, res.Stdout)
	}
	if res := r.Run("cache", "rebuild"); res.ExitCode != 0 {
		t.Fatalf("rebuild: %d %s", res.ExitCode, res.Stderr)
	}
	for i, q := range queries {
		res := r.Run(append([]string{"export", "search", "--json"}, q...)...)
		if res.ExitCode != 0 {
			t.Fatalf("%v: exit %d: %s", q, res.ExitCode, res.Stderr)
		}
		if strings.Contains(res.Stderr, "scan") {
			t.Errorf("%v: no fallback notice expected when the index is used: %q", q, res.Stderr)
		}
		if !strings.Contains(res.Stderr, "Searched") {
			t.Errorf("%v: missing the export search source note: %q", q, res.Stderr)
		}
		if res.Stdout != scanned[i] {
			t.Errorf("%v: index differs from scan\nscan:\n%s\nindex:\n%s", q, scanned[i], res.Stdout)
		}
	}
}

func TestExportUpdatesIndexAndStaleFileFallsBack(t *testing.T) {
	r, store := readRunner(t, 6)
	if res := r.Run("channel", "export", "general"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	var st struct {
		Index struct {
			Present      bool   `json:"present"`
			Path         string `json:"path"`
			SizeBytes    int64  `json:"size_bytes"`
			MessageCount int    `json:"message_count"`
			FilesIndexed int    `json:"files_indexed"`
			FilesOnDisk  int    `json:"files_on_disk"`
			FilesStale   int    `json:"files_stale"`
		} `json:"index"`
	}
	r.Run("cache", "status", "--json").JSON(t, &st)
	if !st.Index.Present || st.Index.MessageCount != 6 || st.Index.FilesIndexed != 1 || st.Index.FilesOnDisk != 1 || st.Index.FilesStale != 0 {
		t.Errorf("after export: %+v", st.Index)
	}
	res := r.Run("export", "search", "policy", "--all")
	if res.ExitCode != 0 || strings.Contains(res.Stderr, "scan") {
		t.Errorf("index should serve: %d %q", res.ExitCode, res.Stderr)
	}

	// An incremental export re-indexes the changed file.
	store.Append(clitest.Message("2001", 7), clitest.Message("2001", 8))
	r.Run("channel", "export", "general")
	r.Run("cache", "status", "--json").JSON(t, &st)
	if st.Index.MessageCount != 8 || st.Index.FilesStale != 0 {
		t.Errorf("after incremental: %+v", st.Index)
	}

	// A file changed behind the tool's back: fall back with a notice.
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	res = r.Run("export", "search", "policy", "--all")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "scan") || !strings.Contains(res.Stderr, "cache rebuild") {
		t.Errorf("stale: %d %q", res.ExitCode, res.Stderr)
	}
	r.Run("cache", "status", "--json").JSON(t, &st)
	if st.Index.FilesStale != 1 {
		t.Errorf("status should count the stale file: %+v", st.Index)
	}
	if res := r.Run("cache", "rebuild"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	res = r.Run("export", "search", "policy", "--all")
	if strings.Contains(res.Stderr, "scan") {
		t.Errorf("after rebuild: %q", res.Stderr)
	}
}

func TestCacheStatusRebuildClear(t *testing.T) {
	r := inventoryRunner(t)
	r.Run("guild", "list")   // populates the lookup cache
	r.Run("channel", "list") // and the per-guild channel list
	res := r.Run("cache", "status")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"index", "no index", "guilds", "channels"} {
		if !strings.Contains(strings.ToLower(res.Stdout), want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	res = r.Run("cache", "rebuild")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr+res.Stdout, "5 files") {
		t.Errorf("rebuild: %d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	var st struct {
		Index struct {
			Present      bool `json:"present"`
			MessageCount int  `json:"message_count"`
			FilesIndexed int  `json:"files_indexed"`
		} `json:"index"`
		Lookup []struct {
			Name string `json:"name"`
			Age  string `json:"age"`
		} `json:"lookup"`
	}
	r.Run("cache", "status", "--json").JSON(t, &st)
	if !st.Index.Present || st.Index.MessageCount != 64 || st.Index.FilesIndexed != 5 || len(st.Lookup) < 2 {
		t.Errorf("%+v", st)
	}
	if _, err := os.Stat(filepath.Join(r.Home.ToolCacheDir(), "index.sqlite")); err != nil {
		t.Errorf("index file missing: %v", err)
	}

	tokenBefore := r.Home.ReadFile(t, filepath.Join(r.Home.ToolConfigDir(), "token"))
	res = r.Run("cache", "clear")
	if res.ExitCode != 0 {
		t.Fatalf("clear: %s", res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(r.Home.ToolCacheDir(), "index.sqlite")); err == nil {
		t.Errorf("index not deleted")
	}
	if _, err := os.Stat(filepath.Join(r.Home.ToolCacheDir(), "lookup")); err == nil {
		t.Errorf("lookup cache not deleted")
	}
	if _, err := os.Stat(filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")); err != nil {
		t.Errorf("export deleted by cache clear")
	}
	if _, err := os.Stat(filepath.Join(r.Home.ToolConfigDir(), "config.json")); err != nil {
		t.Errorf("config deleted by cache clear")
	}
	if string(r.Home.ReadFile(t, filepath.Join(r.Home.ToolConfigDir(), "token"))) != string(tokenBefore) {
		t.Errorf("token touched by cache clear")
	}
	r.Run("cache", "status", "--json").JSON(t, &st)
	if st.Index.Present {
		t.Errorf("index still present after clear")
	}
	if res := r.Run("cache", "clear"); res.ExitCode != 0 {
		t.Errorf("clear twice should be fine: %s", res.Stderr)
	}
}

func TestIndexedSearchIsFast(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("large fixture; timing is only meaningful without -short and without the race detector")
	}
	r := channelRunner(t)
	dir := filepath.Join(r.Home.ExportsDir(), "big-guild")
	for f := 0; f < 20; f++ {
		msgs := make([]map[string]any, 3000)
		for i := range msgs {
			m := clitest.Message(fmt.Sprintf("70%02d", f), i+1)
			m["content"] = fmt.Sprintf("bulk line %d in file %d discussing %s", i, f, []string{"assessment", "evidence", "scoping", "poam"}[i%4])
			msgs[i] = m
		}
		writeJSON(t, filepath.Join(dir, fmt.Sprintf("chan-%02d.json", f)), clitest.NativeExport("7000", "Big Guild", fmt.Sprintf("70%02d", f), fmt.Sprintf("chan-%02d", f), 0, msgs))
	}
	if res := r.Run("cache", "rebuild"); res.ExitCode != 0 {
		t.Fatal(res.Stderr)
	}
	start := time.Now()
	res := r.Run("export", "search", "poam", "--all", "--author", "kyle", "--json")
	elapsed := time.Since(start)
	if res.ExitCode != 0 || strings.Contains(res.Stderr, "scan") {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	var j searchOut
	res.JSON(t, &j)
	if j.TotalMatches == 0 {
		t.Errorf("no matches over 60000 messages")
	}
	if elapsed > time.Second {
		t.Errorf("indexed search took %s over 60000 messages", elapsed)
	}
}
