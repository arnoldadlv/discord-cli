package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// compactLines splits compact output into its records, dropping the final
// blank line a trailing newline would otherwise add.
func compactLines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// compactAddress splits the first field of a compact record the way
// `cut -d: -f1` does, and returns the guild and channel slug either side
// of the slash.
func compactAddress(t *testing.T, line string) (guild, channel string) {
	t.Helper()
	first := strings.SplitN(line, ":", 2)[0]
	parts := strings.SplitN(first, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("no guild/channel address in %q", line)
	}
	return parts[0], parts[1]
}

// withContent copies a fixture message with its content replaced, so a test
// can control exactly what a compact record has to flatten or truncate.
func withContent(base map[string]any, content string) map[string]any {
	m := make(map[string]any, len(base))
	for k, v := range base {
		m[k] = v
	}
	m["content"] = content
	return m
}

// tsvLines splits tsv output into its rows.
func tsvLines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// --- --format validation ---

func TestFormatInvalidValueIsAUsageError(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "list", "--format=bogus")
	if res.ExitCode != 2 || !strings.Contains(res.Stderr, `"bogus"`) {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
}

func TestFormatDisagreeingWithJSONExits2(t *testing.T) {
	r := guildRunner(t)
	for _, format := range []string{"human", "compact", "tsv"} {
		res := r.Run("guild", "list", "--json", "--format="+format)
		if res.ExitCode != 2 {
			t.Errorf("--json --format=%s: exit %d, want 2: %s", format, res.ExitCode, res.Stderr)
		}
	}
	// Agreeing is fine.
	res := r.Run("guild", "list", "--json", "--format=json")
	if res.ExitCode != 0 {
		t.Errorf("--json --format=json: exit %d: %s", res.ExitCode, res.Stderr)
	}
}

func TestFormatJSONIsByteIdenticalToJSONFlag(t *testing.T) {
	r, _ := readRunner(t, 5)
	viaFlag := r.Run("channel", "read", "general", "--json")
	if viaFlag.ExitCode != 0 {
		t.Fatalf("--json: exit %d: %s", viaFlag.ExitCode, viaFlag.Stderr)
	}
	viaFormat := r.Run("channel", "read", "general", "--format=json")
	if viaFormat.ExitCode != 0 {
		t.Fatalf("--format=json: exit %d: %s", viaFormat.ExitCode, viaFormat.Stderr)
	}
	if viaFlag.Stdout != viaFormat.Stdout || viaFlag.Stdout == "" {
		t.Errorf("--json and --format=json differ:\n%s\n---\n%s", viaFlag.Stdout, viaFormat.Stdout)
	}

	r2 := guildRunner(t)
	a := r2.Run("guild", "list", "--json")
	b := r2.Run("guild", "list", "--format=json")
	if a.Stdout != b.Stdout || a.Stdout == "" {
		t.Errorf("guild list: --json and --format=json differ:\n%s\n---\n%s", a.Stdout, b.Stdout)
	}
}

// --- compact: golden files and the guild/channel round trip ---

func TestChannelReadCompactGoldenAndRoundTrip(t *testing.T) {
	r, _ := readRunner(t, 3)
	res := r.Run("channel", "read", "general", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	golden := filepath.Join("testdata", "channel_read.golden.compact")
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
		t.Errorf("compact output changed; diff against %s:\n%s", golden, res.Stdout)
	}

	lines := compactLines(res.Stdout)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), res.Stdout)
	}
	gslug, cslug := compactAddress(t, lines[0])
	if gslug != "cooey-coe" || cslug != "general" {
		t.Fatalf("address: %s/%s", gslug, cslug)
	}
	// The slug this line carries resolves back to the same channel, exactly
	// as --guild and a channel argument already accept it.
	round := r.Run("channel", "read", "--guild", gslug, cslug, "--limit", "3", "--format=compact")
	if round.ExitCode != 0 {
		t.Fatalf("round trip: exit %d: %s", round.ExitCode, round.Stderr)
	}
	if round.Stdout != res.Stdout {
		t.Errorf("round trip changed the output:\n%s\n---\n%s", res.Stdout, round.Stdout)
	}
}

func TestDMReadCompactUsesTheDMSlug(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 3))
	res := r.Run("dm", "read", "kyle", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	golden := filepath.Join("testdata", "dm_read.golden.compact")
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
		t.Errorf("compact output changed; diff against %s:\n%s", golden, res.Stdout)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), res.Stdout)
	}
	gslug, cslug := compactAddress(t, lines[0])
	if gslug != "dm" || cslug != "kyle" {
		t.Errorf("address: %s/%s", gslug, cslug)
	}
}

func TestMessageReadCompactFromAGuildExport(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", clitest.MessageID(6), "--context", "1", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (one message either side), got %d:\n%s", len(lines), res.Stdout)
	}
	for _, l := range lines {
		gslug, cslug := compactAddress(t, l)
		if gslug != "cooey-coe" || cslug != "general" {
			t.Errorf("address: %s/%s in %q", gslug, cslug, l)
		}
	}
	if !strings.Contains(res.Stdout, clitest.MessageID(6)) {
		t.Errorf("missing the message asked for:\n%s", res.Stdout)
	}
}

func TestMessageReadCompactFromADMExportUsesTheDMSlug(t *testing.T) {
	r := indexedMessageRunner(t)
	res := r.Run("message", "read", clitest.MessageID(102), "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d:\n%s", len(lines), res.Stdout)
	}
	gslug, cslug := compactAddress(t, lines[0])
	if gslug != "dm" || cslug != "kyle" {
		t.Errorf("address: %s/%s", gslug, cslug)
	}
}

func TestMessageReadCompactFallsBackToTheChannelIDWhenNamesAreUnknown(t *testing.T) {
	r := indexedMessageRunner(t)
	clitest.ServeMessages(r.Fake, "2001", fixtureMessages("2001", 1, 40))
	// A raw channel id needs no guild, so nothing resolves it to a name;
	// the compact record falls back to the id, still one line, still
	// something --channel accepts back.
	res := r.Run("message", "read", clitest.MessageID(30), "--channel", "2001", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d:\n%s", len(lines), res.Stdout)
	}
	gslug, cslug := compactAddress(t, lines[0])
	if gslug != "" || cslug != "2001" {
		t.Errorf("address: %q/%q", gslug, cslug)
	}
}

func TestExportSearchCompactUsesTheDMSlugForDMResults(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("export", "search", "policy", "--all", "--limit", "100", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) == 0 {
		t.Fatalf("no lines:\n%s", res.Stdout)
	}
	sawDM, sawGuild := false, false
	for _, l := range lines {
		g, c := compactAddress(t, l)
		switch {
		case g == "dm" && c == "kyle":
			sawDM = true
		case g == "cooey-coe":
			sawGuild = true
		}
	}
	if !sawDM {
		t.Errorf("no dm/kyle line among:\n%s", res.Stdout)
	}
	if !sawGuild {
		t.Errorf("no cooey-coe line among:\n%s", res.Stdout)
	}
}

func TestGuildSearchCompact(t *testing.T) {
	r := searchRunner(t, 10)
	res := r.Run("guild", "search", "access control", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 10 {
		t.Fatalf("want 10 lines, got %d:\n%s", len(lines), res.Stdout)
	}
	for _, l := range lines {
		g, c := compactAddress(t, l)
		if g != "cooey-coe" {
			t.Errorf("guild slug: %s", g)
		}
		if c == "" {
			t.Errorf("empty channel slug in %q", l)
		}
	}
}

// --- compact: content flattening and --width ---

func TestCompactFlattensNewlinesTabsAndKeepsColons(t *testing.T) {
	r, s := readRunner(t, 0)
	tricky := "line one\nline two\twith:colon:pairs"
	s.Append(withContent(clitest.Message("2001", 1), tricky))
	res := r.Run("channel", "read", "general", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 1 {
		t.Fatalf("content broke the one-record-per-line rule: %d lines:\n%q", len(lines), res.Stdout)
	}
	line := lines[0]
	if strings.Contains(line, "\n") || strings.Contains(line, "\t") {
		t.Errorf("a raw newline or tab leaked into the record: %q", line)
	}
	if !strings.Contains(line, `line one\nline two\twith:colon:pairs`) {
		t.Errorf("content not flattened as expected: %q", line)
	}
	gslug, cslug := compactAddress(t, line)
	if gslug != "cooey-coe" || cslug != "general" {
		t.Errorf("address still parses despite the colons in content: %s/%s", gslug, cslug)
	}
}

func TestCompactWidthDefaultTruncatesAt200WithEllipsis(t *testing.T) {
	r, s := readRunner(t, 0)
	long := strings.Repeat("a", 250)
	s.Append(withContent(clitest.Message("2001", 1), long))
	res := r.Run("channel", "read", "general", "--format=compact")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	parts := strings.SplitN(lines[0], ": ", 2)
	if len(parts) != 2 {
		t.Fatalf("no content in %q", lines[0])
	}
	body := parts[1]
	if !strings.HasSuffix(body, "...") {
		t.Errorf("default width did not truncate: %q", body)
	}
	if len(body) != 203 { // 200 kept runes + the ellipsis
		t.Errorf("truncated body length = %d, want 203: %q", len(body), body)
	}
}

func TestCompactWidthZeroDisablesTruncation(t *testing.T) {
	r, s := readRunner(t, 0)
	long := strings.Repeat("a", 250)
	s.Append(withContent(clitest.Message("2001", 1), long))
	res := r.Run("channel", "read", "general", "--format=compact", "--width", "0")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := compactLines(res.Stdout)
	if len(lines) != 1 || !strings.HasSuffix(lines[0], long) || strings.Contains(lines[0], "...") {
		t.Errorf("--width 0 truncated: %q", lines[0])
	}
}

// --- tsv ---

func TestGuildListTSVWithAndWithoutHeader(t *testing.T) {
	r := guildRunner(t)
	res := r.Run("guild", "list", "--format=tsv")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := tsvLines(res.Stdout)
	if len(lines) != 4 { // header + 3 guilds
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), res.Stdout)
	}
	if lines[0] != "NAME\tID\tMEMBERS\tONLINE" {
		t.Errorf("header: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Cooey COE\t1001\t1200\t80") {
		t.Errorf("row: %q", lines[1])
	}

	noHeader := r.Run("guild", "list", "--format=tsv", "--no-header")
	if got := tsvLines(noHeader.Stdout); len(got) != 3 {
		t.Fatalf("--no-header: want 3 rows, got %d:\n%s", len(got), noHeader.Stdout)
	}
}

func TestDMListTSV(t *testing.T) {
	r := dmRunner(t)
	res := r.Run("dm", "list", "--format=tsv", "--no-header")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := tsvLines(res.Stdout)
	if len(lines) != 4 {
		t.Fatalf("want 4 rows, got %d:\n%s", len(lines), res.Stdout)
	}
	if !strings.HasPrefix(lines[0], "kyle\tdm\t") {
		t.Errorf("row 0: %q", lines[0])
	}
}

func TestChannelListTSVIncludesThreadsWithTheirParent(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("channel", "list", "--threads", "--format=tsv")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := tsvLines(res.Stdout)
	if len(lines) == 0 || lines[0] != "CATEGORY\tCHANNEL\tTYPE\tID\tPARENT" {
		t.Fatalf("header: %q", lines[0])
	}
	sawThread := false
	for _, l := range lines[1:] {
		cols := strings.Split(l, "\t")
		if len(cols) != 5 {
			t.Fatalf("row shape: %q", l)
		}
		if cols[4] != "" {
			sawThread = true
		}
	}
	if !sawThread {
		t.Errorf("no thread row (a filled PARENT column) among:\n%s", res.Stdout)
	}
}

func TestExportListTSV(t *testing.T) {
	r := inventoryRunner(t)
	res := r.Run("export", "list", "--format=tsv", "--no-header")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Cooey COE\t🔮general\t12\t") {
		t.Errorf("missing native row:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "DM\tkyle\t3\t") {
		t.Errorf("missing dm row:\n%s", res.Stdout)
	}
}

func TestCacheStatusTSVListsLookupEntries(t *testing.T) {
	r := guildRunner(t)
	if res := r.Run("channel", "list", "--guild", "1001"); res.ExitCode != 0 {
		t.Fatalf("channel list: exit %d: %s", res.ExitCode, res.Stderr)
	}
	res := r.Run("cache", "status", "--format=tsv")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	lines := tsvLines(res.Stdout)
	if len(lines) == 0 || lines[0] != "NAME\tAGE\tFETCHED_AT\tFRESH" {
		t.Fatalf("header: %q", lines[0])
	}
	names := map[string]bool{}
	for _, l := range lines[1:] {
		cols := strings.Split(l, "\t")
		if len(cols) != 4 {
			t.Fatalf("row shape: %q", l)
		}
		names[cols[0]] = true
		if cols[3] != "true" && cols[3] != "false" {
			t.Errorf("FRESH is not a bool: %q", l)
		}
	}
	if !names["guilds"] || !names["channels-1001"] {
		t.Errorf("missing lookup rows: %v", names)
	}

	noHeader := r.Run("cache", "status", "--format=tsv", "--no-header")
	if got := len(tsvLines(noHeader.Stdout)); got != len(lines)-1 {
		t.Errorf("--no-header: got %d rows, want %d", got, len(lines)-1)
	}
}

// --- the skill documents when to reach for compact ---

func TestSkillExplainsWhenCompactBeatsJSON(t *testing.T) {
	r := clitest.NewRunner(t)
	skill := strings.ToLower(r.Run("help", "--skill").Stdout)
	for _, want := range []string{"--format", "compact", "tsv", "counting", "grouping", "scanning", "distribution"} {
		if !strings.Contains(skill, want) {
			t.Errorf("skill missing %q", want)
		}
	}
}
