package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

type guildJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	OnlineCount int    `json:"online_count"`
	Owner       bool   `json:"owner"`
}

func toGuildJSON(g discord.Guild) guildJSON {
	return guildJSON{ID: g.ID, Name: g.Name, MemberCount: g.ApproximateMemberCount, OnlineCount: g.ApproximatePresenceCount, Owner: g.Owner}
}

func (a *app) guildCommands() []*cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List every guild the account belongs to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			gs, err := a.guilds(cmd.Context())
			if err != nil {
				return err
			}
			if a.flags.JSON {
				out := make([]guildJSON, len(gs))
				for i, g := range gs {
					out[i] = toGuildJSON(g)
				}
				return term.WriteJSON(a.stdout(), out)
			}
			rows := make([][]string, len(gs))
			for i, g := range gs {
				rows[i] = []string{g.Name, g.ID, strconv.Itoa(g.ApproximateMemberCount), strconv.Itoa(g.ApproximatePresenceCount)}
			}
			term.Table(a.stdout(), a.out, []term.Column{{Header: "NAME"}, {Header: "ID"}, {Header: "MEMBERS", Right: true}, {Header: "ONLINE", Right: true}}, rows)
			return nil
		},
	}

	var showGuild string
	show := &cobra.Command{
		Use:   "show",
		Short: "Summarise one guild: counts and the state of its exports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.guildShow(cmd, showGuild)
		},
	}
	addGuildFlag(show, &showGuild)

	var exGuild string
	var exThreads, exFull bool
	var exConcurrency int
	exp := &cobra.Command{
		Use:   "export",
		Short: "Export every text, announcement, and forum channel of a guild",
		Long: `Export every text, announcement, and forum channel of a guild, several at a
time, incrementally. --threads also exports each channel's active and
archived threads as their own files under threads/<channel>/. The export
meta is written after each channel completes, so a run you kill keeps its
finished channels and the next run resumes from them.`,
		Example: `  discord guild export
  discord guild export --guild "My Guild" --threads
  discord guild export --concurrency 2 --full`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.guildExport(cmd, exGuild, exThreads, exFull, exConcurrency)
		},
	}
	addGuildFlag(exp, &exGuild)
	exp.Flags().BoolVar(&exThreads, "threads", false, "also export active and archived threads")
	exp.Flags().BoolVar(&exFull, "full", false, "ignore the export meta and refetch every message")
	exp.Flags().IntVar(&exConcurrency, "concurrency", 4, "how many channels to export at once")
	return []*cobra.Command{list, show, a.guildSearchCommand(), exp}
}

type guildShowJSON struct {
	guildJSON
	ChannelCount  int                `json:"channel_count"`
	CategoryCount int                `json:"category_count"`
	VoiceCount    int                `json:"voice_count"`
	Exports       []exportStatusJSON `json:"exports"`
}

func (a *app) guildShow(cmd *cobra.Command, guildFlag string) error {
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
	out := guildShowJSON{guildJSON: toGuildJSON(g)}
	for _, ch := range chs {
		switch {
		case discord.IsMessageChannel(ch.Type):
			out.ChannelCount++
		case ch.Type == discord.ChannelCategory:
			out.CategoryCount++
		case ch.Type == discord.ChannelVoice || ch.Type == discord.ChannelStage:
			out.VoiceCount++
		}
	}
	// Export status per message channel, from whatever is on disk.
	byChannel := map[string]export.Item{}
	for _, it := range export.Inventory(a.paths().ReadLocations()) {
		if it.Guild.ID == g.ID {
			if _, seen := byChannel[it.Channel.ID]; !seen {
				byChannel[it.Channel.ID] = it
			}
		}
	}
	out.Exports = []exportStatusJSON{}
	for _, grp := range groupChannels(chs) {
		for _, ch := range grp.Channels {
			st := exportStatusJSON{Channel: namedJSON{ID: ch.ID, Name: ch.Name}}
			if it, ok := byChannel[ch.ID]; ok {
				st.Exported = true
				st.Path = it.Path
				st.Location = it.Location
				st.Dialect = string(it.Dialect)
				st.MessageCount = max(it.MessageCount, 0)
				if it.LastExport != "" {
					le := it.LastExport
					st.LastExport = &le
				}
				st.NewestAt = it.DateRange.Before
			}
			out.Exports = append(out.Exports, st)
		}
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), out)
	}
	w := a.stdout()
	fmt.Fprintf(w, "%s  %s\n", a.out.Bold(g.Name), a.out.Dim(g.ID))
	fmt.Fprintf(w, "Members:   %d (%d online)\n", g.ApproximateMemberCount, g.ApproximatePresenceCount)
	fmt.Fprintf(w, "Channels:  %d text, announcement, and forum; %d voice; %d categories\n", out.ChannelCount, out.VoiceCount, out.CategoryCount)
	exported := 0
	for _, st := range out.Exports {
		if st.Exported {
			exported++
		}
	}
	fmt.Fprintf(w, "\n%s %d of %d channels\n", a.out.Bold("Exports:"), exported, len(out.Exports))
	for _, st := range out.Exports {
		fmt.Fprintln(w, a.exportStatusLine(st))
	}
	return nil
}
