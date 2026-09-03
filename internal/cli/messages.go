package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
)

// namedJSON is a resolved guild or channel in JSON output.
type namedJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type *int   `json:"type,omitempty"`
}

// messagesJSON is the JSON document for reads: the raw messages plus the
// resolved names. Searches embed the same shape.
type messagesJSON struct {
	Guild    namedJSON         `json:"guild"`
	Channel  namedJSON         `json:"channel"`
	Messages []json.RawMessage `json:"messages"`
}

func rawMessages(ms []discord.Message) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Raw)
	}
	return out
}

func intPtr(i int) *int { return &i }

var htmlTag = regexp.MustCompile(`<[^>]+>`)

// messageWriter renders messages for a terminal.
type messageWriter struct {
	a        *app
	w        io.Writer
	loc      *time.Location
	channels map[string]string // channel id -> name, for search results
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
		if e.Description != "" {
			d := discord.Truncate(htmlTag.ReplaceAllString(e.Description, ""), 300)
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
