package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// The four values --format accepts. json is an alias of the older --json
// flag; the tool never distinguishes how JSON was asked for.
const (
	formatHuman   = "human"
	formatJSON    = "json"
	formatCompact = "compact"
	formatTSV     = "tsv"
)

// resolveFormat reconciles --format with --json into the three booleans the
// commands read (a.flags.JSON, a.flags.Compact, a.flags.TSV), or fails with
// a usage error when the two disagree.
func (a *app) resolveFormat() error {
	raw := strings.ToLower(strings.TrimSpace(a.flags.FormatFlag))
	if raw == "" {
		// --json with no --format is the same as --format=json.
		return nil
	}
	switch raw {
	case formatHuman:
		if a.flags.JSON {
			return UsageError("--json and --format=human disagree").
				WithHint("Drop --json, or use --format=json.")
		}
	case formatJSON:
		a.flags.JSON = true
	case formatCompact:
		if a.flags.JSON {
			return UsageError("--json and --format=compact disagree").
				WithHint("Drop --json, or use --format=json.")
		}
		a.flags.Compact = true
	case formatTSV:
		if a.flags.JSON {
			return UsageError("--json and --format=tsv disagree").
				WithHint("Drop --json, or use --format=json.")
		}
		a.flags.TSV = true
	default:
		return UsageError("--format must be one of human, json, compact, tsv (got %q)", a.flags.FormatFlag)
	}
	return nil
}

// compactRow is one message rendered by --format=compact:
// guild-slug/channel-slug:message-id:timestamp:author: content
type compactRow struct {
	GuildSlug   string
	ChannelSlug string
	ID          string
	Timestamp   string // the raw timestamp, as Discord or an export stored it
	Author      string
	Content     string
}

// writeCompact prints one line per row. The timestamp is normalised to UTC
// RFC 3339 at second precision (compactTimestamp); content is flattened to
// one line and truncated to --width (0 disables truncation) with a
// trailing ellipsis, the same way discord.Truncate marks every other
// truncation in this tool.
func (a *app) writeCompact(rows []compactRow) {
	w := a.stdout()
	for _, r := range rows {
		content := flattenContent(r.Content)
		if a.flags.Width > 0 {
			content = discord.Truncate(content, a.flags.Width)
		}
		fmt.Fprintf(w, "%s/%s:%s:%s:%s: %s\n", r.GuildSlug, r.ChannelSlug, r.ID, compactTimestamp(r.Timestamp), r.Author, content)
	}
}

// compactTimestamp renders a message timestamp as UTC RFC 3339 at second
// precision, so it ends in Z: 2026-08-28T18:18:24Z, not the sub-second,
// numeric-offset form Discord and exports store it in. A timestamp that
// cannot be parsed is passed through unchanged rather than dropping the
// record over it.
func compactTimestamp(raw string) string {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return t.UTC().Format(time.RFC3339)
}

// flattenContent puts message content on one line: newlines become the two
// characters \n, tabs become \t, so the compact record they sit in stays
// exactly one line and stays parseable.
func flattenContent(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// writeTable renders rows as a human table, or as --format=tsv when asked:
// the same columns, tab-separated, with a header row --no-header
// suppresses. Every list command builds its columns and rows once and ends
// with this, so the renderer is the only thing that changes with --format.
func (a *app) writeTable(cols []term.Column, rows [][]string) {
	if a.flags.TSV {
		term.TSV(a.stdout(), cols, rows, a.flags.NoHeader)
		return
	}
	term.Table(a.stdout(), a.out, cols, rows)
}
