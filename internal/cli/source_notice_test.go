package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// wantExportNotice is the note 'message read' prints on stderr when a
// message is resolved from an export, naming the file (with ~ for home)
// and the date its messages run up to.
func wantExportNotice(file, date string) string {
	return "Read from the export at " + file + ", which covers messages up to " + date + ". Newer messages are not in it.\n"
}

const wantDiscordNotice = "Fetched from Discord.\n"

func TestMessageReadFromExportPrintsTheSourceNoteInEveryFormat(t *testing.T) {
	want := wantExportNotice("~/.local/share/discord-cli/exports/cooey-coe/general.json", "2026-08-01")
	for _, format := range []string{"human", "json", "compact", "tsv"} {
		r := indexedMessageRunner(t)
		args := []string{"message", "read", clitest.MessageID(6)}
		if format == "json" {
			args = append(args, "--json")
		} else if format != "human" {
			args = append(args, "--format", format)
		}
		res := r.Run(args...)
		if res.ExitCode != 0 {
			t.Fatalf("%s: exit %d: %s", format, res.ExitCode, res.Stderr)
		}
		if res.Stderr != want {
			t.Errorf("%s: stderr:\ngot:  %q\nwant: %q", format, res.Stderr, want)
		}
		if strings.Contains(res.Stdout, "Read from the export") {
			t.Errorf("%s: notice leaked onto stdout:\n%s", format, res.Stdout)
		}
	}
}

func TestMessageReadFromDiscordPrintsTheSourceNoteInEveryFormat(t *testing.T) {
	for _, format := range []string{"human", "json", "compact", "tsv"} {
		r := indexedMessageRunner(t)
		clitest.ServeMessages(r.Fake, "2001", fixtureMessages("2001", 1, 40))
		args := []string{"message", "read", clitest.MessageID(30), "--channel", "general", "--context", "1"}
		if format == "json" {
			args = append(args, "--json")
		} else if format != "human" {
			args = append(args, "--format", format)
		}
		res := r.Run(args...)
		if res.ExitCode != 0 {
			t.Fatalf("%s: exit %d: %s", format, res.ExitCode, res.Stderr)
		}
		if res.Stderr != wantDiscordNotice {
			t.Errorf("%s: stderr:\ngot:  %q\nwant: %q", format, res.Stderr, wantDiscordNotice)
		}
		if strings.Contains(res.Stdout, "Fetched from Discord") {
			t.Errorf("%s: notice leaked onto stdout:\n%s", format, res.Stdout)
		}
	}
}

// TestMessageReadExportNoticeUsesTheNewestMessageNotTheFileModTime covers
// the sharpest way this note can go wrong: an export refreshed with
// nothing new in it still gets its file touched, so the file's mtime can
// sit long after the newest message it actually holds. The note must say
// the date the messages cover, never the mtime.
func TestMessageReadExportNoticeUsesTheNewestMessageNotTheFileModTime(t *testing.T) {
	r := channelRunner(t)
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	writeJSON(t, path, clitest.NativeExport("1001", "Cooey COE", "2001", "🔮general", 0, fixtureMessages("2001", 1, 5)))
	future := time.Date(2031, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	res := r.Run("message", "read", clitest.MessageID(3), "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	want := wantExportNotice("~/.local/share/discord-cli/exports/cooey-coe/general.json", "2026-08-01")
	if !strings.Contains(res.Stderr, want) {
		t.Errorf("notice should use the newest message date, not the file's mtime:\ngot:  %q\nwant it to contain: %q", res.Stderr, want)
	}
	if strings.Contains(res.Stderr, "2031") {
		t.Errorf("notice leaked the file's modification time: %q", res.Stderr)
	}
}

// TestMessageReadFromExportWithNoDateRangeDropsTheDateClause covers a
// native export whose dateRange is missing entirely (never written, or
// corrupted): shortDate would render "-", so the note must drop the date
// clause rather than say "up to -".
func TestMessageReadFromExportWithNoDateRangeDropsTheDateClause(t *testing.T) {
	r := channelRunner(t)
	path := filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json")
	env := map[string]any{
		"guild":        map[string]any{"id": "1001", "name": "Cooey COE"},
		"channel":      map[string]any{"id": "2001", "name": "🔮general", "type": 0},
		"dateRange":    map[string]any{"after": nil, "before": nil},
		"messages":     fixtureMessages("2001", 1, 5),
		"messageCount": 5,
	}
	writeJSON(t, path, env)
	res := r.Run("message", "read", clitest.MessageID(3), "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	want := "Read from the export at ~/.local/share/discord-cli/exports/cooey-coe/general.json. It may not hold newer messages.\n"
	if !strings.Contains(res.Stderr, want) {
		t.Errorf("stderr:\ngot:  %q\nwant it to contain: %q", res.Stderr, want)
	}
	if strings.Contains(res.Stderr, "which covers messages up to") || strings.Contains(res.Stderr, "up to -") {
		t.Errorf("date clause should be dropped, not rendered as a dash: %q", res.Stderr)
	}
}

// TestMessageReadFromLegacyExportNeverQuotesItsRequestedFilterDate covers
// a legacy DiscordChatExporter export whose dateRange.before is the date
// range it was asked to export, not what it actually stored, and here is
// later than the newest message on disk. Quoting it would assert
// something the file does not support, so the note must drop the date
// clause for every legacy export, not just ones with no dateRange at all.
func TestMessageReadFromLegacyExportNeverQuotesItsRequestedFilterDate(t *testing.T) {
	r := channelRunner(t)
	legacy := clitest.LegacyExport("1001", "Cooey COE", "2020", "access-control", 5)
	legacy["dateRange"] = map[string]any{"after": nil, "before": "2099-12-31T00:00:00.000-07:00"}
	writeJSON(t, filepath.Join(r.Home.ChatExporterDir(), "cooey-coe", "Cooey COE - access-control [2020].json"), legacy)
	res := r.Run("message", "read", "4000003", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.Stderr, "2099") {
		t.Errorf("notice quoted the legacy export's requested filter date: %q", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "It may not hold newer messages.") {
		t.Errorf("legacy export should get the no-date wording: %q", res.Stderr)
	}
}

// TestExportSearchExcludesLegacyDatesFromTheNewestCalculation covers a
// legacy export whose dateRange.before is later than any native export's
// newest message: it must never win the "newest covering" comparison,
// since it is a requested filter date, not a message date.
func TestExportSearchExcludesLegacyDatesFromTheNewestCalculation(t *testing.T) {
	r := channelRunner(t)
	writeJSON(t, filepath.Join(r.Home.ExportsDir(), "cooey-coe", "general.json"),
		clitest.NativeExport("1001", "Cooey COE", "2001", "🔮general", 0, fixtureMessages("2001", 1, 3)))
	legacy := clitest.LegacyExport("1001", "Cooey COE", "2020", "access-control", 5)
	legacy["dateRange"] = map[string]any{"after": nil, "before": "2099-12-31T00:00:00.000-07:00"}
	writeJSON(t, filepath.Join(r.Home.ChatExporterDir(), "cooey-coe", "Cooey COE - access-control [2020].json"), legacy)

	res := r.Run("export", "search", "policy", "--all", "--limit", "100")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.Stderr, "2099") {
		t.Errorf("the legacy export's requested filter date leaked into the notice: %q", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "the newest covering messages up to 2026-08-01") {
		t.Errorf("the native export's date should still be reported: %q", res.Stderr)
	}
}

// TestExportSearchWithNoUsableDateDropsTheDateClause covers searching
// only legacy exports, none of which can supply a message date: the note
// must drop the date clause rather than render shortDate's "-".
func TestExportSearchWithNoUsableDateDropsTheDateClause(t *testing.T) {
	r := channelRunner(t)
	writeJSON(t, filepath.Join(r.Home.ChatExporterDir(), "cooey-coe", "Cooey COE - access-control [2020].json"),
		clitest.LegacyExport("1001", "Cooey COE", "2020", "access-control", 5))
	res := r.Run("export", "search", "policy", "--all")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	want := "Searched 1 export on disk. Anything newer may not be in them; 'discord guild search' asks Discord instead.\n"
	if !strings.Contains(res.Stderr, want) {
		t.Errorf("stderr:\ngot:  %q\nwant it to contain: %q", res.Stderr, want)
	}
	if strings.Contains(res.Stderr, "the newest covering") || strings.Contains(res.Stderr, "up to -") {
		t.Errorf("date clause should be dropped, not rendered as a dash: %q", res.Stderr)
	}
}

func TestExportSearchPrintsTheSourceNoteInEveryFormat(t *testing.T) {
	want := "Searched 5 exports on disk, the newest covering messages up to 2026-08-01. Anything newer is not in them; 'discord guild search' asks Discord instead.\n"
	for _, format := range []string{"human", "json", "compact"} {
		r := inventoryRunner(t)
		args := []string{"export", "search", "policy", "--all", "--limit", "100"}
		if format == "json" {
			args = append(args, "--json")
		} else if format != "human" {
			args = append(args, "--format", format)
		}
		res := r.Run(args...)
		if res.ExitCode != 0 {
			t.Fatalf("%s: exit %d: %s", format, res.ExitCode, res.Stderr)
		}
		if !strings.Contains(res.Stderr, want) {
			t.Errorf("%s: stderr:\ngot:  %q\nwant it to contain: %q", format, res.Stderr, want)
		}
		if strings.Contains(res.Stdout, "Searched") {
			t.Errorf("%s: notice leaked onto stdout:\n%s", format, res.Stdout)
		}
	}
	// --format=tsv is a valid global flag value even though export search
	// has no tsv rendering of its own (it falls back to human); the note
	// still lands on stderr, never stdout.
	r := inventoryRunner(t)
	res := r.Run("export", "search", "policy", "--all", "--limit", "100", "--format", "tsv")
	if res.ExitCode != 0 {
		t.Fatalf("tsv: exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, want) {
		t.Errorf("tsv: stderr:\ngot:  %q\nwant it to contain: %q", res.Stderr, want)
	}
}

func TestExportSearchJSONCarriesSourceExportField(t *testing.T) {
	r := inventoryRunner(t)
	var j struct {
		Source string `json:"source"`
	}
	res := r.Run("export", "search", "policy", "--all", "--limit", "100", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if j.Source != "export" {
		t.Errorf(`source = %q, want "export"`, j.Source)
	}
}

// TestGuildSearchChannelReadDMReadPrintNoSourceNote covers the three
// commands whose name already says where they read from: they must stay
// silent, unlike 'message read' and 'export search'.
func TestGuildSearchChannelReadDMReadPrintNoSourceNote(t *testing.T) {
	r := searchRunner(t, 5)
	if res := r.Run("guild", "search", "access control"); res.ExitCode != 0 || res.Stderr != "" {
		t.Errorf("guild search: exit %d stderr %q", res.ExitCode, res.Stderr)
	}

	r2, _ := readRunner(t, 5)
	if res := r2.Run("channel", "read", "general"); res.ExitCode != 0 || res.Stderr != "" {
		t.Errorf("channel read: exit %d stderr %q", res.ExitCode, res.Stderr)
	}

	r3 := dmRunner(t)
	clitest.ServeMessages(r3.Fake, "6001", clitest.Messages("6001", 5))
	if res := r3.Run("dm", "read", "kyle"); res.ExitCode != 0 || res.Stderr != "" {
		t.Errorf("dm read: exit %d stderr %q", res.ExitCode, res.Stderr)
	}
}

// TestSkillDocumentsTheSourceNote covers the last acceptance criterion:
// an agent reading the embedded skill must learn that stderr carries this
// note and that "source" in JSON is the machine-readable form of it.
func TestSkillDocumentsTheSourceNote(t *testing.T) {
	r := clitest.NewRunner(t)
	skill := r.Run("help", "--skill").Stdout
	for _, want := range []string{"stderr", "source"} {
		if !strings.Contains(strings.ToLower(skill), want) {
			t.Errorf("skill missing %q", want)
		}
	}
	if !strings.Contains(skill, "message read") || !strings.Contains(skill, "export search") {
		t.Errorf("skill does not name the two commands that carry the note")
	}
}
