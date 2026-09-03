package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

type dateRangeJSON struct {
	After  *string `json:"after"`
	Before *string `json:"before"`
}

type exportItemJSON struct {
	Guild        namedJSON     `json:"guild"`
	Channel      namedJSON     `json:"channel"`
	Path         string        `json:"path"`
	Location     string        `json:"location"`
	Dialect      string        `json:"dialect"`
	MessageCount int           `json:"message_count"`
	DateRange    dateRangeJSON `json:"date_range"`
	LastExport   *string       `json:"last_export"`
}

func toExportItemJSON(it export.Item) exportItemJSON {
	j := exportItemJSON{
		Guild:        namedJSON{ID: it.Guild.ID, Name: it.Guild.Name},
		Channel:      namedJSON{ID: it.Channel.ID, Name: it.Channel.Name},
		Path:         it.Path,
		Location:     it.Location,
		Dialect:      string(it.Dialect),
		MessageCount: it.MessageCount,
		DateRange:    dateRangeJSON{After: it.DateRange.After, Before: it.DateRange.Before},
	}
	if it.Dialect == export.Native {
		j.Channel.Type = intPtr(it.Channel.Type)
	}
	if it.LastExport != "" {
		le := it.LastExport
		j.LastExport = &le
	}
	return j
}

// inventory lists the exports on disk, narrowed to one guild when given.
func (a *app) inventory(guild string) ([]export.Item, error) {
	all := export.Inventory(a.paths().ReadLocations())
	if len(all) == 0 {
		return nil, Errorf(ExitNoExports, "no exports found in %s, %s, or %s",
			a.paths().ExportsDir(), a.paths().LegacyExportsDir(), a.paths().DCEExportsDir()).
			WithHint("Run 'discord guild export' or 'discord channel export <channel>' to create one.")
	}
	if guild == "" {
		return all, nil
	}
	var out []export.Item
	for _, it := range all {
		if it.MatchesGuild(guild) {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		return nil, Errorf(ExitNoExports, "no exports found for guild %q", guild).
			WithHint("Run 'discord export list' to see every export on disk, or 'discord guild export --guild %q'.", guild)
	}
	return out, nil
}

func (a *app) exportListCommand() *cobra.Command {
	var guild string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every export on disk across all read locations",
		Long: `List every export on disk: new exports in the data directory, the Node
CLI's ~/.discord-cli/exports, and the DiscordChatExporter folder. Shows the
guild, channel or DM, message count, date range, dialect, and the last
export time where the meta knows it. Nothing on disk exits 5.`,
		Example: `  discord export list
  discord export list --guild cooey-coe --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := a.inventory(guild)
			if err != nil {
				return err
			}
			if a.flags.JSON {
				out := make([]exportItemJSON, 0, len(items))
				for _, it := range items {
					out = append(out, toExportItemJSON(it))
				}
				return term.WriteJSON(a.stdout(), out)
			}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				rows = append(rows, []string{
					it.Guild.Name, it.Channel.Name, strconv.Itoa(max(it.MessageCount, 0)),
					shortDate(it.DateRange.After), shortDate(it.DateRange.Before),
					string(it.Dialect), shortDate(&it.LastExport), a.shortPath(it.Path),
				})
			}
			term.Table(a.stdout(), a.out, []term.Column{
				{Header: "GUILD"}, {Header: "CHANNEL"}, {Header: "MESSAGES", Right: true},
				{Header: "FROM"}, {Header: "TO"}, {Header: "DIALECT"}, {Header: "LAST EXPORT"}, {Header: "PATH"},
			}, rows)
			return nil
		},
	}
	cmd.Flags().StringVarP(&guild, "guild", "g", "", "only exports of this guild (name, id, or directory); use dm for DMs")
	return cmd
}

// shortDate renders a timestamp as a date, or a dash.
func shortDate(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339Nano, *s); err == nil {
		return t.Format("2006-01-02")
	}
	if len(*s) >= 10 {
		return (*s)[:10]
	}
	return *s
}

// shortPath replaces the home directory with ~ for display.
func (a *app) shortPath(p string) string {
	home := a.paths().Home
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// exportStatusJSON is one channel's export status in guild show.
type exportStatusJSON struct {
	Channel      namedJSON `json:"channel"`
	Exported     bool      `json:"exported"`
	Path         string    `json:"path,omitempty"`
	Location     string    `json:"location,omitempty"`
	Dialect      string    `json:"dialect,omitempty"`
	MessageCount int       `json:"message_count"`
	LastExport   *string   `json:"last_export"`
	NewestAt     *string   `json:"newest_message_at"`
}

func (a *app) exportStatusLine(s exportStatusJSON) string {
	if !s.Exported {
		return fmt.Sprintf("  #%s  %s", s.Channel.Name, a.out.Dim("no export"))
	}
	when := shortDate(s.LastExport)
	if when == "-" {
		when = shortDate(s.NewestAt)
	}
	return fmt.Sprintf("  #%s  %s, last export %s  %s", s.Channel.Name, plural(s.MessageCount, "message"), when, a.out.Dim("("+s.Location+", "+s.Dialect+")"))
}
