package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
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
	return []*cobra.Command{list, show}
}

type guildShowJSON struct {
	guildJSON
	ChannelCount  int `json:"channel_count"`
	CategoryCount int `json:"category_count"`
	VoiceCount    int `json:"voice_count"`
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
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), out)
	}
	w := a.stdout()
	fmt.Fprintf(w, "%s  %s\n", a.out.Bold(g.Name), a.out.Dim(g.ID))
	fmt.Fprintf(w, "Members:   %d (%d online)\n", g.ApproximateMemberCount, g.ApproximatePresenceCount)
	fmt.Fprintf(w, "Channels:  %d text, announcement, and forum; %d voice; %d categories\n", out.ChannelCount, out.VoiceCount, out.CategoryCount)
	return nil
}
