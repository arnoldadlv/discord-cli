package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/resolve"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

type participantJSON struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type dmJSON struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Participants []participantJSON `json:"participants"`
}

func toDMJSON(d discord.DMChannel) dmJSON {
	j := dmJSON{ID: d.ID, Type: "dm", Name: d.DisplayName(), Participants: []participantJSON{}}
	if d.IsGroup() {
		j.Type = "group"
	}
	for _, r := range d.Recipients {
		j.Participants = append(j.Participants, participantJSON{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName()})
	}
	return j
}

// dms returns the DM list, from the lookup cache when fresh.
func (a *app) dms(ctx context.Context) ([]discord.DMChannel, error) {
	var out []discord.DMChannel
	_, err := a.lookupCache().Get("dms", func() (any, error) {
		c, _, err := a.client()
		if err != nil {
			return nil, err
		}
		ds, err := c.DMs(ctx)
		if err != nil {
			return nil, a.apiError(err)
		}
		return ds, nil
	}, &out)
	return out, err
}

// resolveDM turns a name or id into a DM: the other participant's username
// (or display name) for a DM, the group name or any participant for a
// group DM.
func (a *app) resolveDM(ctx context.Context, input string) (discord.DMChannel, error) {
	ds, err := a.dms(ctx)
	if err != nil {
		return discord.DMChannel{}, err
	}
	cands := make([]resolve.Candidate, len(ds))
	for i, d := range ds {
		c := resolve.Candidate{ID: d.ID, Name: d.DisplayName()}
		for _, r := range d.Recipients {
			if d.IsGroup() {
				c.Aliases = append(c.Aliases, r.Username)
				if r.GlobalName != "" {
					c.Aliases = append(c.Aliases, r.GlobalName)
				}
			} else if r.GlobalName != "" {
				c.AlsoNames = append(c.AlsoNames, r.GlobalName)
			}
		}
		cands[i] = c
	}
	m, err := resolve.Match("DM", input, cands)
	if err != nil {
		return discord.DMChannel{}, a.resolveError(err, "dm list")
	}
	for _, d := range ds {
		if d.ID == m.ID {
			return d, nil
		}
	}
	return discord.DMChannel{}, Errorf(ExitNotFound, "DM %q not found", input).WithHint("Run 'discord dm list' to see your DMs; use --no-cache if it is new.")
}

func (a *app) dmCommands() []*cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List your DMs and group DMs with the other participants",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ds, err := a.dms(cmd.Context())
			if err != nil {
				return err
			}
			if a.flags.JSON {
				out := make([]dmJSON, 0, len(ds))
				for _, d := range ds {
					out = append(out, toDMJSON(d))
				}
				return term.WriteJSON(a.stdout(), out)
			}
			rows := make([][]string, 0, len(ds))
			for _, d := range ds {
				j := toDMJSON(d)
				parts := make([]string, 0, len(j.Participants))
				for _, p := range j.Participants {
					if p.DisplayName != "" && p.DisplayName != p.Username {
						parts = append(parts, p.Username+" ("+p.DisplayName+")")
					} else {
						parts = append(parts, p.Username)
					}
				}
				rows = append(rows, []string{j.Name, j.Type, joinStrings(parts, ", "), d.ID})
			}
			term.Table(a.stdout(), a.out, []term.Column{{Header: "NAME"}, {Header: "TYPE"}, {Header: "PARTICIPANTS"}, {Header: "ID"}}, rows)
			return nil
		},
	}

	var readLimit int
	read := &cobra.Command{
		Use:   "read <dm>",
		Short: "Print recent messages of a DM or group DM, oldest first",
		Example: `  discord dm read kyle
  discord dm read "Study Group" --limit 50 --json`,
		Args: exactOnePositional("dm"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.dmRead(cmd, args[0], readLimit)
		},
	}
	read.Flags().IntVarP(&readLimit, "limit", "n", 25, "number of messages to show")

	var (
		query, after, before string
		limit                int
	)
	search := &cobra.Command{
		Use:   "search <dm> [<query>]",
		Short: "Search one DM by fetching its history and filtering it",
		Long: `Search one DM or group DM. Discord has no search endpoint for DMs, so the
whole history is fetched and filtered here: a message matches when any
query term appears in it. For repeated research, export the DM once with
'discord dm export' and use 'discord export search'.`,
		Example: `  discord dm search kyle --query "meeting"
  discord dm search kyle meeting --after 2026-01-01 --json`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := queryArg(args[1:], query)
			if err != nil {
				return err
			}
			return a.dmSearch(cmd, args[0], q, after, before, limit)
		},
	}
	search.Flags().StringVarP(&query, "query", "q", "", "text to search for (alias of the second argument)")
	search.Flags().StringVar(&after, "after", "", "only messages after this date (YYYY-MM-DD or RFC 3339)")
	search.Flags().StringVar(&before, "before", "", "only messages before this date (YYYY-MM-DD or RFC 3339)")
	search.Flags().IntVarP(&limit, "limit", "n", 25, "number of results to show")
	return []*cobra.Command{list, read, search}
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// dmMessagesJSON is the JSON for DM reads: the same shape as channel reads
// without a guild.
type dmMessagesJSON struct {
	Channel  namedJSON         `json:"channel"`
	Messages []json.RawMessage `json:"messages"`
}

func (a *app) dmRead(cmd *cobra.Command, input string, limit int) error {
	ctx := cmd.Context()
	if limit <= 0 {
		return UsageError("--limit must be at least 1")
	}
	d, err := a.resolveDM(ctx, input)
	if err != nil {
		return err
	}
	c, _, err := a.client()
	if err != nil {
		return err
	}
	ms, err := c.Recent(ctx, d.ID, limit)
	if err != nil {
		return a.apiError(err)
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), dmMessagesJSON{
			Channel:  namedJSON{ID: d.ID, Name: d.DisplayName(), Type: intPtr(d.Type)},
			Messages: rawMessages(ms),
		})
	}
	if len(ms) == 0 {
		fmt.Fprintf(a.stdout(), "No messages with %s.\n", d.DisplayName())
		return nil
	}
	a.messageWriter().writeAll(ms)
	return nil
}

type dmSearchJSON struct {
	Channel      namedJSON         `json:"channel"`
	TotalMatches int               `json:"total_matches"`
	Messages     []json.RawMessage `json:"messages"`
}

func (a *app) dmSearch(cmd *cobra.Command, input, query, afterFlag, beforeFlag string, limit int) error {
	ctx := cmd.Context()
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
	d, err := a.resolveDM(ctx, input)
	if err != nil {
		return err
	}
	c, _, err := a.client()
	if err != nil {
		return err
	}
	pages := 0
	history, err := c.History(ctx, d.ID, "", func(page []discord.Message, total int) {
		pages++
		if a.env.StderrIsTerminal {
			fmt.Fprintf(a.stderr(), "\rFetching history... %d messages", total)
		}
	})
	if a.env.StderrIsTerminal && pages > 0 {
		fmt.Fprint(a.stderr(), "\r\033[K")
	}
	if err != nil {
		return a.apiError(err)
	}
	if pages > 1 {
		a.notice("Searched all %d messages of this DM; Discord has no search for DMs. For repeated searches run 'discord dm export %q' once, then 'discord export search'.", len(history), d.DisplayName())
	}
	terms := queryTerms(query)
	var matches []discord.Message
	for _, m := range history {
		if !matchesTerms(m.Content, terms) {
			continue
		}
		if !inRange(m.Time(), after, before) {
			continue
		}
		matches = append(matches, m)
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Time().After(matches[j].Time()) })
	shown := matches
	if len(shown) > limit {
		shown = shown[:limit]
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), dmSearchJSON{
			Channel:      namedJSON{ID: d.ID, Name: d.DisplayName(), Type: intPtr(d.Type)},
			TotalMatches: len(matches),
			Messages:     rawMessages(shown),
		})
	}
	if len(matches) == 0 {
		fmt.Fprintf(a.stdout(), "No matches for %q with %s.\n", query, d.DisplayName())
		return nil
	}
	fmt.Fprintf(a.stdout(), "%s\n\n", a.out.Dim(fmt.Sprintf("%s with %s (showing %d, newest first)", plural(len(matches), "match"), d.DisplayName(), len(shown))))
	a.messageWriter().writeAll(shown)
	return nil
}
