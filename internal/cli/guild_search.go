package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// hasValues are what Discord's search accepts for --has.
const hasValues = "image, video, sound, file, embed, link, sticker, poll"

type searchJSON struct {
	Guild        namedJSON         `json:"guild"`
	TotalResults int               `json:"total_results"`
	Messages     []json.RawMessage `json:"messages"`
	ChannelNames map[string]string `json:"channel_names"`
}

// queryArg merges the positional query and --query, which are aliases.
func queryArg(args []string, flag string) (string, error) {
	pos := ""
	if len(args) > 0 {
		pos = args[0]
	}
	switch {
	case pos != "" && flag != "" && pos != flag:
		return "", UsageError("the query was given twice: %q and --query %q", pos, flag).WithHint("Give it once, as the argument or as --query.")
	case pos != "":
		return pos, nil
	case flag != "":
		return flag, nil
	}
	return "", UsageError("missing the query").WithHint("Usage: discord guild search <query> [flags], or --query <text>.")
}

func (a *app) guildSearchCommand() *cobra.Command {
	var (
		guild, query, channel, has string
		limit, offset              int
	)
	cmd := &cobra.Command{
		Use:   "search [<query>]",
		Short: "Search a guild's messages through Discord's own search",
		Long: `Search a guild's messages through Discord's search endpoint, newest first.
The query may be given as the argument or as --query. Limits above 25 are
satisfied by fetching further pages for you.`,
		Example: `  discord guild search "access control"
  discord guild search MFA --channel general --limit 50
  discord guild search --query "policy" --has link --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := queryArg(args, query)
			if err != nil {
				return err
			}
			return a.guildSearch(cmd, guild, q, channel, has, limit, offset)
		},
	}
	addGuildFlag(cmd, &guild)
	cmd.Flags().StringVarP(&query, "query", "q", "", "text to search for (alias of the argument)")
	cmd.Flags().StringVar(&channel, "channel", "", "narrow to one channel, by name or id")
	cmd.Flags().StringVar(&has, "has", "", "only messages that have one of: "+hasValues)
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "number of results to show")
	cmd.Flags().IntVar(&offset, "offset", 0, "skip this many results")
	return cmd
}

func (a *app) guildSearch(cmd *cobra.Command, guildFlag, query, channel, has string, limit, offset int) error {
	ctx := cmd.Context()
	if limit <= 0 {
		return UsageError("--limit must be at least 1")
	}
	if offset < 0 {
		return UsageError("--offset cannot be negative")
	}
	input, err := a.guildArg(guildFlag)
	if err != nil {
		return err
	}
	g, err := a.resolveGuild(ctx, input)
	if err != nil {
		return err
	}
	opts := discord.SearchOptions{Content: query, Has: has, Limit: limit, Offset: offset}
	if channel != "" {
		ch, err := a.resolveChannel(ctx, g.ID, channel, false)
		if err != nil {
			return err
		}
		opts.ChannelID = ch.ID
	}
	c, _, err := a.client()
	if err != nil {
		return err
	}
	res, err := c.Search(ctx, g.ID, opts)
	if err != nil {
		return a.apiError(err)
	}
	if res.Indexing {
		a.notice("Discord has not finished building the search index for %s yet; try again in a minute.", g.Name)
	}
	names := a.channelNames(ctx, g.ID, res.Messages)

	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), searchJSON{
			Guild:        namedJSON{ID: g.ID, Name: g.Name},
			TotalResults: res.TotalResults,
			Messages:     rawMessages(res.Messages),
			ChannelNames: names,
		})
	}
	if len(res.Messages) == 0 {
		fmt.Fprintf(a.stdout(), "No results for %q in %s.\n", query, g.Name)
		return nil
	}
	fmt.Fprintf(a.stdout(), "%s\n\n", a.out.Dim(fmt.Sprintf("%d results in %s (showing %d, newest first)", res.TotalResults, g.Name, len(res.Messages))))
	mw := a.messageWriter()
	mw.channels = names
	mw.writeAll(res.Messages)
	return nil
}

// channelNames maps the channel ids in messages to names, from the cached
// channel list; unknown ids are left out.
func (a *app) channelNames(ctx context.Context, guildID string, ms []discord.Message) map[string]string {
	names := map[string]string{}
	if len(ms) == 0 {
		return names
	}
	chs, err := a.channels(ctx, guildID)
	if err != nil {
		return names
	}
	byID := map[string]string{}
	for _, ch := range chs {
		byID[ch.ID] = ch.Name
	}
	for _, m := range ms {
		if n, ok := byID[m.ChannelID]; ok {
			names[m.ChannelID] = n
		}
	}
	return names
}
