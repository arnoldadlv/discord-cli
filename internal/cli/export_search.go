package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/resolve"
	"github.com/arnoldadlv/discord-cli/internal/search"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

type searchResultJSON struct {
	Guild     namedJSON `json:"guild"`
	Channel   namedJSON `json:"channel"`
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Timestamp string    `json:"timestamp"`
	Content   string    `json:"content"`
	File      string    `json:"file"`
}

type exportSearchJSON struct {
	Source       string             `json:"source"` // always "export"; export search only ever reads exports on disk
	TotalMatches int                `json:"total_matches"`
	Shown        int                `json:"shown"`
	Results      []searchResultJSON `json:"results"`
}

func toSearchResultJSON(r search.Result) searchResultJSON {
	return searchResultJSON{
		Guild:     namedJSON{ID: r.GuildID, Name: r.GuildName},
		Channel:   namedJSON{ID: r.ChannelID, Name: r.ChannelName},
		ID:        r.MessageID,
		Author:    r.Author,
		Timestamp: r.Timestamp,
		Content:   r.Content,
		File:      r.File,
	}
}

func (a *app) exportSearchCommand() *cobra.Command {
	var (
		guild, query, author, after, before string
		all                                 bool
		limit                               int
	)
	cmd := &cobra.Command{
		Use:   "search [<query>]",
		Short: "Search the exports on disk, without limits",
		Long: `Search the exports on disk. A message matches when any query term appears
in it, case-insensitively. Scope with --guild <name> or --all; one of them is
required. Results are newest first across every export and dialect.`,
		Example: `  discord export search "access control" --guild cooey-coe
  discord export search policy --all --author kyle --after 2026-01-01
  discord export search --query MFA --guild dm --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := queryArg(args, query)
			if err != nil {
				return err
			}
			return a.exportSearch(cmd, q, guild, all, author, after, before, limit)
		},
	}
	cmd.Flags().StringVarP(&guild, "guild", "g", "", "search one guild's exports (name, id, or directory); use dm for DMs")
	cmd.Flags().BoolVar(&all, "all", false, "search every export on disk")
	cmd.Flags().StringVarP(&query, "query", "q", "", "text to search for (alias of the argument)")
	cmd.Flags().StringVar(&author, "author", "", "only messages whose author name contains this")
	cmd.Flags().StringVar(&after, "after", "", "only messages after this date (YYYY-MM-DD or RFC 3339)")
	cmd.Flags().StringVar(&before, "before", "", "only messages before this date (YYYY-MM-DD or RFC 3339)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "number of results to show")
	return cmd
}

// exportSearchScope picks the exports to search.
func (a *app) exportSearchScope(guild string, all bool) ([]export.Item, error) {
	switch {
	case guild == "" && !all:
		return nil, UsageError("say where to search: --guild <name> for one guild, or --all for every export").
			WithHint("Run 'discord export list' to see the guilds on disk.")
	case guild != "" && all:
		return nil, UsageError("--guild and --all cannot be combined")
	}
	return a.inventory(guild)
}

func (a *app) exportSearch(cmd *cobra.Command, query, guild string, all bool, author, afterFlag, beforeFlag string, limit int) error {
	if limit <= 0 {
		return UsageError("--limit must be at least 1")
	}
	after, err := parseDate("after", afterFlag)
	if err != nil {
		return err
	}
	before, err := parseDate("before", beforeFlag)
	if err != nil {
		return err
	}
	items, err := a.exportSearchScope(guild, all)
	if err != nil {
		return err
	}
	q := search.Query{Terms: search.Terms(query), Author: strings.ToLower(author), After: after, Before: before}
	results, err := a.runLocalSearch(cmd, items, q)
	if err != nil {
		return err
	}
	shown := results
	if len(shown) > limit {
		shown = shown[:limit]
	}
	a.notice("Searched %s on disk, the newest covering messages up to %s. Anything newer is not in them; 'discord guild search' asks Discord instead.", plural(len(items), "export"), shortDate(newestExportDate(items)))
	if a.flags.JSON {
		out := exportSearchJSON{Source: "export", TotalMatches: len(results), Shown: len(shown), Results: make([]searchResultJSON, 0, len(shown))}
		for _, r := range shown {
			out.Results = append(out.Results, toSearchResultJSON(r))
		}
		return term.WriteJSON(a.stdout(), out)
	}
	if a.flags.Compact {
		rows := make([]compactRow, len(shown))
		for i, r := range shown {
			rows[i] = compactRow{GuildSlug: resolve.Key(r.GuildName), ChannelSlug: resolve.Key(r.ChannelName), ID: r.MessageID, Timestamp: r.Timestamp, Author: r.Author, Content: r.Content}
		}
		a.writeCompact(rows)
		return nil
	}
	w := a.stdout()
	if len(results) == 0 {
		fmt.Fprintf(w, "No matches for %q in %s.\n", query, plural(len(items), "export"))
		return nil
	}
	fmt.Fprintf(w, "%s\n\n", a.out.Dim(fmt.Sprintf("%s across %s (showing %d, newest first)", plural(len(results), "match"), plural(len(items), "export"), len(shown))))
	for i, r := range shown {
		if i > 0 {
			fmt.Fprintln(w)
		}
		a.writeSearchResult(r)
	}
	if rest := len(results) - len(shown); rest > 0 {
		fmt.Fprintf(w, "\n%s\n", a.out.Dim(fmt.Sprintf("... and %d more; raise --limit to see them", rest)))
	}
	return nil
}

// newestExportDate is the newest message date any of the searched exports
// covers, the same value the "TO" column of 'export list' shows. An export
// whose messages have not changed since it was last read is not fresher,
// so this reads the covered date, never the file's modification time.
func newestExportDate(items []export.Item) *string {
	var best *string
	var bestTime time.Time
	for _, it := range items {
		if it.DateRange.Before == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, *it.DateRange.Before)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestTime) {
			best, bestTime = it.DateRange.Before, t
		}
	}
	return best
}

func (a *app) writeSearchResult(r search.Result) {
	s := a.out
	w := a.stdout()
	when := r.Timestamp
	if t := r.Time(); !t.IsZero() {
		when = t.Local().Format("2006-01-02 15:04")
	}
	where := "#" + r.ChannelName
	if r.GuildID == export.DMGuildID {
		where = "DM " + r.ChannelName
	}
	fmt.Fprintf(w, "%s  %s  %s %s\n", s.Dim(when), s.Bold(r.Author), s.Cyan(where), s.Dim("("+r.GuildName+")"))
	for _, line := range strings.Split(strings.TrimRight(r.Content, "\n"), "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
}
