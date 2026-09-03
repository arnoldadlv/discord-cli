package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
)

// namedJSON is a resolved guild or channel in JSON output.
type namedJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type *int   `json:"type,omitempty"`
}

// messagesJSON is the JSON document for channel reads: the compact messages
// plus the resolved guild and channel.
type messagesJSON struct {
	Guild    namedJSON            `json:"guild"`
	Channel  namedJSON            `json:"channel"`
	Messages []compactMessageJSON `json:"messages"`
}

func rawMessages(ms []discord.Message) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Raw)
	}
	return out
}

// compactAuthorJSON is a message author or mention: just enough to address
// and label them, never the rest of the user object Discord sends.
type compactAuthorJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func toCompactAuthor(a discord.Author) compactAuthorJSON {
	return compactAuthorJSON{ID: a.ID, Name: a.DisplayName()}
}

// compactAttachmentJSON is a message attachment, without Discord's internal id.
type compactAttachmentJSON struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
}

// compactEmbedJSON is the part of an embed a reader needs: title, url, and
// description, truncated the same way the human layout truncates them.
// thumbnail, video, provider, placeholder, and content_scan_version never
// parse into discord.Embed, so they are dropped for free.
type compactEmbedJSON struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// compactReactionJSON is one emoji and its count. burst_colors,
// count_details, and me_burst never parse into discord.Reaction, so they
// are dropped for free.
type compactReactionJSON struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

// compactMessageJSON is the message shape channel and DM reads emit: enough
// for an agent to read and act on a conversation, without the full Discord
// API object. Fidelity belongs to exports, not this shape.
type compactMessageJSON struct {
	ID          string                  `json:"id"`
	Timestamp   string                  `json:"timestamp"`
	Edited      bool                    `json:"edited"`
	Author      compactAuthorJSON       `json:"author"`
	Content     string                  `json:"content"`
	ReplyTo     string                  `json:"reply_to,omitempty"`
	Mentions    []compactAuthorJSON     `json:"mentions,omitempty"`
	Attachments []compactAttachmentJSON `json:"attachments,omitempty"`
	Embeds      []compactEmbedJSON      `json:"embeds,omitempty"`
	Reactions   []compactReactionJSON   `json:"reactions,omitempty"`
	// Match marks the one message a read was asked for, when the others are
	// only there for context. Reads that were not asked for a single message
	// leave it out.
	Match bool `json:"match,omitempty"`
}

// embedDescription is an embed's description as the human layout shows it:
// HTML tags stripped, then truncated to 300 runes.
func embedDescription(e discord.Embed) string {
	if e.Description == "" {
		return ""
	}
	return discord.Truncate(htmlTag.ReplaceAllString(e.Description, ""), 300)
}

// toCompactMessage projects one raw API message onto the compact shape.
func toCompactMessage(m discord.Message) compactMessageJSON {
	c := compactMessageJSON{
		ID:        m.ID,
		Timestamp: m.Timestamp,
		Edited:    m.Edited(),
		Author:    toCompactAuthor(m.Author),
		Content:   m.Content,
		ReplyTo:   m.ReplyTo(),
	}
	for _, u := range m.Mentions {
		c.Mentions = append(c.Mentions, toCompactAuthor(u))
	}
	for _, att := range m.Attachments {
		c.Attachments = append(c.Attachments, compactAttachmentJSON{Filename: att.Filename, URL: att.URL, Size: att.Size})
	}
	for _, e := range m.Embeds {
		c.Embeds = append(c.Embeds, compactEmbedJSON{Title: e.Title, URL: e.URL, Description: embedDescription(e)})
	}
	for _, r := range m.Reactions {
		c.Reactions = append(c.Reactions, compactReactionJSON{Emoji: r.Emoji.Name, Count: r.Count})
	}
	return c
}

// compactMessages projects messages onto the compact shape that channel and
// DM reads emit as --json.
func compactMessages(ms []discord.Message) []compactMessageJSON {
	out := make([]compactMessageJSON, 0, len(ms))
	for _, m := range ms {
		out = append(out, toCompactMessage(m))
	}
	return out
}

// legacyMessage is a message as DiscordChatExporter stores it. Legacy
// exports are the one place a stored message is not a raw API object, so
// their fields are mapped onto the ones the compact shape needs.
type legacyMessage struct {
	ID              string  `json:"id"`
	Content         string  `json:"content"`
	Timestamp       string  `json:"timestamp"`
	TimestampEdited *string `json:"timestampEdited"`
	Author          struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Nickname string `json:"nickname"`
	} `json:"author"`
	Reference *struct {
		MessageID string `json:"messageId"`
	} `json:"reference"`
	Mentions []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Nickname string `json:"nickname"`
	} `json:"mentions"`
	Attachments []struct {
		FileName      string `json:"fileName"`
		FileSizeBytes int64  `json:"fileSizeBytes"`
		URL           string `json:"url"`
	} `json:"attachments"`
	Embeds []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	} `json:"embeds"`
	Reactions []struct {
		Emoji struct {
			Name string `json:"name"`
		} `json:"emoji"`
		Count int `json:"count"`
	} `json:"reactions"`
}

// author is the legacy author as an API one: the guild nickname is the
// display name, the handle is the username, which is how local search names
// legacy authors too.
func legacyAuthor(id, name, nickname string) discord.Author {
	return discord.Author{ID: id, Username: name, GlobalName: nickname}
}

// storedMessage parses one message out of an export. Native exports hold
// the raw API object, so it parses straight into a message; legacy ones
// hold DiscordChatExporter's shape and are mapped onto the same fields.
func storedMessage(dialect export.Dialect, raw json.RawMessage) (discord.Message, bool) {
	if dialect != export.Legacy {
		var m discord.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			return discord.Message{}, false
		}
		return m, true
	}
	var l legacyMessage
	if err := json.Unmarshal(raw, &l); err != nil {
		return discord.Message{}, false
	}
	m := discord.Message{
		Raw:             append(json.RawMessage(nil), raw...),
		ID:              l.ID,
		Content:         l.Content,
		Timestamp:       l.Timestamp,
		EditedTimestamp: l.TimestampEdited,
		Author:          legacyAuthor(l.Author.ID, l.Author.Name, l.Author.Nickname),
	}
	if l.Reference != nil && l.Reference.MessageID != "" {
		m.Reference = &discord.MessageReference{MessageID: l.Reference.MessageID}
	}
	for _, u := range l.Mentions {
		m.Mentions = append(m.Mentions, legacyAuthor(u.ID, u.Name, u.Nickname))
	}
	for _, att := range l.Attachments {
		m.Attachments = append(m.Attachments, discord.Attachment{Filename: att.FileName, Size: att.FileSizeBytes, URL: att.URL})
	}
	for _, e := range l.Embeds {
		m.Embeds = append(m.Embeds, discord.Embed{Title: e.Title, URL: e.URL, Description: e.Description})
	}
	for _, r := range l.Reactions {
		var rr discord.Reaction
		rr.Count = r.Count
		rr.Emoji.Name = r.Emoji.Name
		m.Reactions = append(m.Reactions, rr)
	}
	return m, true
}

// storedMessageID reads only the id of a stored message, so finding one in
// a large export does not parse every message in it.
func storedMessageID(raw json.RawMessage) string {
	var probe struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	return probe.ID
}

func intPtr(i int) *int { return &i }

var htmlTag = regexp.MustCompile(`<[^>]+>`)

// messageWriter renders messages for a terminal.
type messageWriter struct {
	a        *app
	w        io.Writer
	loc      *time.Location
	channels map[string]string // channel id -> name, for search results
	mark     string            // message id to point at with a leading >
}

func (a *app) messageWriter() *messageWriter {
	return &messageWriter{a: a, w: a.stdout(), loc: time.Local}
}

func (mw *messageWriter) timestamp(m discord.Message) string {
	t := m.Time()
	if t.IsZero() {
		return m.Timestamp
	}
	return t.In(mw.loc).Format("2006-01-02 15:04")
}

// write renders one message: timestamp and author, content, then
// attachments, embeds, and reactions.
func (mw *messageWriter) write(m discord.Message) {
	s := mw.a.out
	head := s.Dim(mw.timestamp(m)) + "  " + s.Bold(m.Author.DisplayName())
	if mw.mark != "" && m.ID == mw.mark {
		head = "> " + head
	}
	if mw.channels != nil {
		if name, ok := mw.channels[m.ChannelID]; ok {
			head += "  " + s.Cyan("#"+name)
		} else if m.ChannelID != "" {
			head += "  " + s.Cyan("#"+m.ChannelID)
		}
	}
	fmt.Fprintln(mw.w, head)
	if m.Content != "" {
		for _, line := range strings.Split(strings.TrimRight(m.Content, "\n"), "\n") {
			fmt.Fprintf(mw.w, "  %s\n", line)
		}
	}
	for _, att := range m.Attachments {
		fmt.Fprintf(mw.w, "  %s %s (%s)\n", s.Dim("attachment:"), att.Filename, att.URL)
	}
	for _, e := range m.Embeds {
		if e.Title != "" {
			fmt.Fprintf(mw.w, "  %s %s\n", s.Dim("embed:"), s.Bold(e.Title))
		}
		if d := embedDescription(e); d != "" {
			for _, line := range strings.Split(strings.TrimRight(d, "\n"), "\n") {
				fmt.Fprintf(mw.w, "  %s %s\n", s.Dim("|"), line)
			}
		}
		if e.URL != "" {
			fmt.Fprintf(mw.w, "  %s %s\n", s.Dim("|"), e.URL)
		}
	}
	if len(m.Reactions) > 0 {
		parts := make([]string, len(m.Reactions))
		for i, r := range m.Reactions {
			parts[i] = fmt.Sprintf("%s %d", r.Emoji.Name, r.Count)
		}
		fmt.Fprintf(mw.w, "  %s\n", strings.Join(parts, "  "))
	}
}

// writeAll renders messages separated by blank lines.
func (mw *messageWriter) writeAll(ms []discord.Message) {
	for i, m := range ms {
		if i > 0 {
			fmt.Fprintln(mw.w)
		}
		mw.write(m)
	}
}
