package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/resolve"
	"github.com/arnoldadlv/discord-cli/internal/search"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// messageReadJSON is the JSON document a message read emits: the compact
// messages, the guild and channel they came from, and where they were read.
type messageReadJSON struct {
	Guild    *namedJSON           `json:"guild,omitempty"`
	Channel  namedJSON            `json:"channel"`
	Source   string               `json:"source"` // export or discord
	File     string               `json:"file,omitempty"`
	Messages []compactMessageJSON `json:"messages"`
}

func (a *app) messageCommands() []*cobra.Command {
	var (
		guild, channel string
		around         int
	)
	read := &cobra.Command{
		Use:   "read <id>",
		Short: "Print one message by id, with the messages around it",
		Long: `Print one message by its id, the id that 'discord export search' and
'discord guild search' return for every match.

The id is looked up in the search index first, so a message that is already
in an export is read from disk with no network call, whichever guild, DM,
or channel it belongs to. When no export holds it, --channel says where to
fetch it from; without --channel there is nothing to ask Discord for and
the command exits 4.

--context N also prints the N messages either side of it, oldest first,
with the message asked for marked by a leading > (in JSON, by "match":
true). At the start or end of a channel there are fewer, and that is all
you get.`,
		Example: `  discord message read 1542961568172740660
  discord message read 1542961568172740660 --context 5
  discord message read 1542961568172740660 --context 5 --json
  discord message read 1542961568172740660 --channel general`,
		Args: exactOnePositional("message id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.messageRead(cmd, args[0], guild, channel, around)
		},
	}
	addGuildFlag(read, &guild)
	read.Flags().StringVar(&channel, "channel", "", "channel to fetch from when no export holds the message (a channel name needs the guild; a thread or DM is given by id)")
	read.Flags().IntVar(&around, "context", 0, "also show this many messages either side of the message")
	return []*cobra.Command{read}
}

func (a *app) messageRead(cmd *cobra.Command, id, guildFlag, channelFlag string, n int) error {
	if n < 0 {
		return UsageError("--context cannot be negative")
	}
	if !resolve.IsID(id) {
		return UsageError("%q is not a message id", id).
			WithHint("Message ids are numeric. Every match from 'discord export search' and 'discord guild search' carries one.")
	}
	if loc := a.locateMessage(id); loc != nil {
		return a.messageFromExport(loc, id, n)
	}
	return a.messageFromDiscord(cmd.Context(), id, guildFlag, channelFlag, n)
}

// locateMessage finds the export holding a message id: through the search
// index when it covers every export on disk, and by reading the exports
// otherwise, the way local search degrades. nil means no export holds it.
func (a *app) locateMessage(id string) *search.Location {
	items := a.allExports()
	if len(items) == 0 {
		return nil
	}
	if ix := a.localIndex(); ix != nil {
		defer ix.Close()
		loc, err := ix.Locate(id)
		if err == nil {
			return loc
		}
		a.notice("Search index failed (%v); scanning exports instead. Run 'discord cache rebuild' to repair it.", err)
	}
	return search.ScanLocate(items, id)
}

// messageFromExport reads the message and its neighbours out of the export
// that holds it. Exports store messages oldest first, so the window is a
// slice of the file.
func (a *app) messageFromExport(loc *search.Location, id string, n int) error {
	h, raws, err := export.Read(loc.File)
	if err != nil {
		return Errorf(ExitUnexpected, "reading %s: %v", a.shortPath(loc.File), err)
	}
	at := -1
	for i, raw := range raws {
		if storedMessageID(raw) == id {
			at = i
			break
		}
	}
	if at < 0 {
		return Errorf(ExitNotFound, "message %s is no longer in %s", id, a.shortPath(loc.File)).
			WithHint("The export changed after it was indexed. Run 'discord cache rebuild' and try again.")
	}
	lo, hi := max(at-n, 0), min(at+n, len(raws)-1)
	var ms []discord.Message
	for _, raw := range raws[lo : hi+1] {
		if m, ok := storedMessage(h.Dialect, raw); ok {
			ms = append(ms, m)
		}
	}
	out := messageReadJSON{
		Channel: namedJSON{ID: loc.ChannelID, Name: loc.ChannelName},
		Source:  "export",
		File:    loc.File,
	}
	if loc.GuildID != "" {
		out.Guild = &namedJSON{ID: loc.GuildID, Name: loc.GuildName}
	}
	if h.Dialect == export.Native {
		// Only native exports record the channel type as a number.
		out.Channel.Type = intPtr(h.Channel.Type)
	}
	return a.writeMessageRead(out, ms, id)
}

// messageFromDiscord fetches the message and its neighbours from Discord,
// which needs the channel: a message id alone is not an address there.
func (a *app) messageFromDiscord(ctx context.Context, id, guildFlag, channelFlag string, n int) error {
	if channelFlag == "" {
		return Errorf(ExitNotFound, "message %s is in no export on disk, and no channel was given", id).
			WithHint("Name the channel with --channel <name or id>, or export the channel it is in with 'discord channel export <channel>' and read it again.")
	}
	want := 2*n + 1
	if want > discord.MessagePage {
		return UsageError("--context above %d needs the message to be in an export; Discord returns at most %d messages around one", (discord.MessagePage-1)/2, discord.MessagePage).
			WithHint("Run 'discord channel export %s' once, then read the message again.", channelFlag)
	}
	if want < 3 {
		// Discord's around wants room either side of the message; the window
		// is cut back to what was asked for below.
		want = 3
	}
	out := messageReadJSON{Source: "discord"}
	channelID := channelFlag
	if resolve.IsID(channelFlag) {
		// An id is an address on its own, so a channel in any guild, and a
		// DM, is readable without naming a guild. Only the id is known.
		out.Channel = namedJSON{ID: channelFlag}
	} else {
		input, err := a.guildArg(guildFlag)
		if err != nil {
			return err
		}
		g, err := a.resolveGuild(ctx, input)
		if err != nil {
			return err
		}
		ch, err := a.resolveChannel(ctx, g.ID, channelFlag, false)
		if err != nil {
			return err
		}
		channelID = ch.ID
		out.Guild = &namedJSON{ID: g.ID, Name: g.Name}
		out.Channel = namedJSON{ID: ch.ID, Name: ch.Name, Type: intPtr(ch.Type)}
	}
	c, _, err := a.client()
	if err != nil {
		return err
	}
	page, err := c.Messages(ctx, channelID, want, "", "", id)
	if err != nil {
		return a.apiError(err)
	}
	// newest first -> oldest first
	for i, j := 0, len(page)-1; i < j; i, j = i+1, j-1 {
		page[i], page[j] = page[j], page[i]
	}
	at := -1
	for i, m := range page {
		if m.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return Errorf(ExitNotFound, "message %s is not in channel %s", id, channelFlag).
			WithHint("Check the id and the channel; the account can only read messages in channels it can see.")
	}
	lo, hi := max(at-n, 0), min(at+n, len(page)-1)
	return a.writeMessageRead(out, page[lo:hi+1], id)
}

// writeMessageRead prints the window, marking the message that was asked
// for: a leading > on its timestamp line, or "match": true in JSON.
func (a *app) writeMessageRead(out messageReadJSON, ms []discord.Message, id string) error {
	if a.flags.JSON {
		out.Messages = compactMessages(ms)
		for i := range out.Messages {
			if out.Messages[i].ID == id {
				out.Messages[i].Match = true
			}
		}
		return term.WriteJSON(a.stdout(), out)
	}
	mw := a.messageWriter()
	mw.mark = id
	mw.writeAll(ms)
	return nil
}
