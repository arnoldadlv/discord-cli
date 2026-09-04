package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// A DiscordChatExporter run that was interrupted leaves a file cut off in
// the middle of a message. The index must skip it, say so, and still serve
// every other export; the scan already skips it, so results stay identical.
func TestCacheRebuildSkipsTruncatedExport(t *testing.T) {
	r := inventoryRunner(t)
	full, _ := json.MarshalIndent(clitest.LegacyExport("8000", "Microsoft Azure", "8002", "random", 30), "", "  ")
	cut := filepath.Join(r.Home.ChatExporterDir(), "microsoft-azure", "Microsoft Azure - random [8002].json")
	r.Home.WriteFile(t, cut, full[:len(full)*2/3], 0o644)

	scan := r.Run("export", "search", "policy", "--all", "--limit", "100", "--json")
	if scan.ExitCode != 0 {
		t.Fatalf("scan: %d %s", scan.ExitCode, scan.Stderr)
	}

	res := r.Run("cache", "rebuild")
	if res.ExitCode != 0 {
		t.Fatalf("rebuild must not fail on one bad file: exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "random [8002].json") || !strings.Contains(res.Stderr, "unreadable") {
		t.Errorf("rebuild should name the skipped file on stderr: %q", res.Stderr)
	}
	if !strings.Contains(res.Stdout, "5 files") {
		t.Errorf("stdout: %q", res.Stdout)
	}

	var st struct {
		Index struct {
			FilesIndexed    int `json:"files_indexed"`
			FilesOnDisk     int `json:"files_on_disk"`
			FilesStale      int `json:"files_stale"`
			FilesUnreadable int `json:"files_unreadable"`
		} `json:"index"`
	}
	r.Run("cache", "status", "--json").JSON(t, &st)
	if st.Index.FilesIndexed != 5 || st.Index.FilesOnDisk != 6 || st.Index.FilesStale != 0 || st.Index.FilesUnreadable != 1 {
		t.Errorf("status: %+v", st.Index)
	}
	human := r.Run("cache", "status").Stdout
	if !strings.Contains(human, "1 unreadable") {
		t.Errorf("human status: %q", human)
	}

	idx := r.Run("export", "search", "policy", "--all", "--limit", "100", "--json")
	if idx.ExitCode != 0 || strings.Contains(idx.Stderr, "scan") {
		t.Fatalf("index should serve after a rebuild with a bad file: %d %q", idx.ExitCode, idx.Stderr)
	}
	if idx.Stdout != scan.Stdout {
		t.Errorf("index and scan differ with a truncated file present:\n%s\n---\n%s", scan.Stdout, idx.Stdout)
	}
}
