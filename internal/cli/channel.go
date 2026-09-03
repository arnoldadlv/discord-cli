package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/resolve"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// typeLabel names a channel type for humans and JSON.
func typeLabel(t int) string {
	switch t {
	case discord.ChannelText:
		return "text"
	case discord.ChannelAnnouncement:
		return "announcement"
	case discord.ChannelForum:
		return "forum"
	case discord.ChannelMedia:
		return "media"
	case discord.ChannelVoice:
		return "voice"
	case discord.ChannelStage:
		return "stage"
	case discord.ChannelCategory:
		return "category"
	case discord.ChannelPublicThread, discord.ChannelAnnThread:
		return "thread"
	case discord.ChannelPrivThread:
		return "private-thread"
	case discord.ChannelDM:
		return "dm"
	case discord.ChannelGroupDM:
		return "group-dm"
	}
	return fmt.Sprintf("type-%d", t)
}

type threadJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Archived bool   `json:"archived"`
}

type channelJSON struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Type       int          `json:"type"`
	TypeLabel  string       `json:"type_label"`
	Category   string       `json:"category,omitempty"`
	CategoryID string       `json:"category_id,omitempty"`
	Position   int          `json:"position"`
	Threads    []threadJSON `json:"threads,omitempty"`
}

// channelGroup is one category (or the uncategorised group) in display order.
type channelGroup struct {
	Category *discord.Channel // nil for uncategorised
	Channels []discord.Channel
}

// groupChannels keeps only text, announcement, and forum channels and orders
// them: uncategorised first, then categories by position, channels by
// position within each.
func groupChannels(chs []discord.Channel) []channelGroup {
	cats := map[string]*discord.Channel{}
	var catList []*discord.Channel
	for i := range chs {
		if chs[i].Type == discord.ChannelCategory {
			cats[chs[i].ID] = &chs[i]
			catList = append(catList, &chs[i])
		}
	}
	sort.SliceStable(catList, func(i, j int) bool { return catList[i].Position < catList[j].Position })
	byParent := map[string][]discord.Channel{}
	for _, ch := range chs {
		if !discord.IsMessageChannel(ch.Type) {
			continue
		}
		parent := ch.ParentID
		if _, ok := cats[parent]; !ok {
			parent = ""
		}
		byParent[parent] = append(byParent[parent], ch)
	}
	for k := range byParent {
		list := byParent[k]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Position < list[j].Position })
		byParent[k] = list
	}
	var groups []channelGroup
	if len(byParent[""]) > 0 {
		groups = append(groups, channelGroup{Channels: byParent[""]})
	}
	for _, cat := range catList {
		if list := byParent[cat.ID]; len(list) > 0 {
			groups = append(groups, channelGroup{Category: cat, Channels: list})
		}
	}
	return groups
}

// threadsByParent fetches threads for every message channel.
func (a *app) threadsByParent(ctx context.Context, chs []discord.Channel) (map[string][]discord.Channel, error) {
	c, _, err := a.client()
	if err != nil {
		return nil, err
	}
	out := map[string][]discord.Channel{}
	for _, ch := range chs {
		if !discord.IsMessageChannel(ch.Type) {
			continue
		}
		ts, err := c.Threads(ctx, ch.ID)
		if err != nil {
			return nil, a.apiError(err)
		}
		if len(ts) > 0 {
			out[ch.ID] = ts
		}
	}
	return out, nil
}

func (a *app) channelCommands() []*cobra.Command {
	var listGuild string
	var listThreads bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List a guild's text, announcement, and forum channels by category",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.channelList(cmd, listGuild, listThreads)
		},
	}
	addGuildFlag(list, &listGuild)
	list.Flags().BoolVar(&listThreads, "threads", false, "include active and archived threads under their parent channels")

	var readGuild string
	var readThreads bool
	var readLimit int
	read := &cobra.Command{
		Use:   "read <channel>",
		Short: "Print the most recent messages of a channel, oldest first",
		Long: `Print the most recent messages of a channel, oldest first, with
attachments, embeds, and reactions. Name the channel by name or id; the
emoji prefix may be left off (news finds 📰news). With --threads a thread
name is accepted too.`,
		Example: `  discord channel read general
  discord channel read news --limit 5
  discord channel read "welcome thread" --threads --json`,
		Args: exactOnePositional("channel"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.channelRead(cmd, readGuild, args[0], readThreads, readLimit)
		},
	}
	addGuildFlag(read, &readGuild)
	read.Flags().BoolVar(&readThreads, "threads", false, "allow a thread name or id as the channel")
	read.Flags().IntVarP(&readLimit, "limit", "n", 25, "number of messages to show")
	return []*cobra.Command{list, read}
}

func (a *app) channelRead(cmd *cobra.Command, guildFlag, channelInput string, withThreads bool, limit int) error {
	ctx := cmd.Context()
	if limit <= 0 {
		return UsageError("--limit must be at least 1")
	}
	input, err := a.guildArg(guildFlag)
	if err != nil {
		return err
	}
	g, err := a.resolveGuild(ctx, input)
	if err != nil {
		return err
	}
	ch, err := a.resolveChannel(ctx, g.ID, channelInput, withThreads)
	if err != nil {
		return err
	}
	c, _, err := a.client()
	if err != nil {
		return err
	}
	ms, err := c.Recent(ctx, ch.ID, limit)
	if err != nil {
		return a.apiError(err)
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), messagesJSON{
			Guild:    namedJSON{ID: g.ID, Name: g.Name},
			Channel:  namedJSON{ID: ch.ID, Name: ch.Name, Type: intPtr(ch.Type)},
			Messages: rawMessages(ms),
		})
	}
	if len(ms) == 0 {
		fmt.Fprintf(a.stdout(), "No messages in #%s.\n", ch.Name)
		return nil
	}
	a.messageWriter().writeAll(ms)
	return nil
}

func (a *app) channelList(cmd *cobra.Command, guildFlag string, withThreads bool) error {
	ctx := cmd.Context()
	input, err := a.guildArg(guildFlag)
	if err != nil {
		return err
	}
	g, err := a.resolveGuild(ctx, input)
	if err != nil {
		return err
	}
	chs, err := a.channels(ctx, g.ID)
	if err != nil {
		return err
	}
	groups := groupChannels(chs)
	threads := map[string][]discord.Channel{}
	if withThreads {
		if threads, err = a.threadsByParent(ctx, chs); err != nil {
			return err
		}
	}
	if a.flags.JSON {
		var out []channelJSON
		for _, grp := range groups {
			for _, ch := range grp.Channels {
				j := channelJSON{ID: ch.ID, Name: ch.Name, Type: ch.Type, TypeLabel: typeLabel(ch.Type), Position: ch.Position}
				if grp.Category != nil {
					j.Category = grp.Category.Name
					j.CategoryID = grp.Category.ID
				}
				for _, t := range threads[ch.ID] {
					j.Threads = append(j.Threads, threadJSON{ID: t.ID, Name: t.Name, Type: t.Type, Archived: t.ThreadMetadata != nil && t.ThreadMetadata.Archived})
				}
				out = append(out, j)
			}
		}
		if out == nil {
			out = []channelJSON{}
		}
		return term.WriteJSON(a.stdout(), out)
	}
	w := a.stdout()
	for i, grp := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if grp.Category == nil {
			fmt.Fprintln(w, a.out.Bold("(no category)"))
		} else {
			fmt.Fprintln(w, a.out.Bold(grp.Category.Name))
		}
		for _, ch := range grp.Channels {
			fmt.Fprintf(w, "  #%s  %s  %s\n", ch.Name, a.out.Dim(typeLabel(ch.Type)), a.out.Dim(ch.ID))
			for _, t := range threads[ch.ID] {
				state := ""
				if t.ThreadMetadata != nil && t.ThreadMetadata.Archived {
					state = "archived"
				} else {
					state = "active"
				}
				fmt.Fprintf(w, "    └ %s  %s  %s\n", t.Name, a.out.Dim(state), a.out.Dim(t.ID))
			}
		}
	}
	return nil
}

// resolveChannel turns a name or id into one of the guild's message channels,
// or one of their threads when withThreads is set.
func (a *app) resolveChannel(ctx context.Context, guildID, input string, withThreads bool) (discord.Channel, error) {
	chs, err := a.channels(ctx, guildID)
	if err != nil {
		return discord.Channel{}, err
	}
	var pool []discord.Channel
	for _, ch := range chs {
		if discord.IsMessageChannel(ch.Type) {
			pool = append(pool, ch)
		}
	}
	if withThreads {
		threads, err := a.threadsByParent(ctx, chs)
		if err != nil {
			return discord.Channel{}, err
		}
		for _, ts := range threads {
			pool = append(pool, ts...)
		}
	}
	cands := make([]resolve.Candidate, len(pool))
	for i, ch := range pool {
		cands[i] = resolve.Candidate{ID: ch.ID, Name: ch.Name}
	}
	m, err := resolve.Match("channel", input, cands)
	if err != nil {
		return discord.Channel{}, a.resolveError(err, "channel list")
	}
	for _, ch := range pool {
		if ch.ID == m.ID {
			return ch, nil
		}
	}
	// An id we do not know: it may be a thread or a channel outside the
	// cached list. Ask Discord for it.
	c, _, err := a.client()
	if err != nil {
		return discord.Channel{}, err
	}
	var ch discord.Channel
	if err := c.Get(ctx, "/channels/"+m.ID, nil, &ch); err != nil {
		var se *discord.StatusError
		if isNotFound(err, &se) {
			return discord.Channel{}, Errorf(ExitNotFound, "channel %q not found", input).WithHint("Run 'discord channel list' to see the channels in this guild.")
		}
		return discord.Channel{}, a.apiError(err)
	}
	if ch.GuildID != "" && ch.GuildID != guildID {
		return discord.Channel{}, Errorf(ExitNotFound, "channel %q belongs to another guild", input)
	}
	return ch, nil
}
