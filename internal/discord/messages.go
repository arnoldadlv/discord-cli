package discord

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// Message is one raw API message. Raw keeps the object exactly as Discord
// returned it, which is what exports store and --json emits; the parsed
// fields serve the human layout and filtering.
type Message struct {
	Raw json.RawMessage `json:"-"`

	ID              string            `json:"id"`
	ChannelID       string            `json:"channel_id"`
	Content         string            `json:"content"`
	Timestamp       string            `json:"timestamp"`
	EditedTimestamp *string           `json:"edited_timestamp,omitempty"`
	Author          Author            `json:"author"`
	Mentions        []Author          `json:"mentions,omitempty"`
	Reference       *MessageReference `json:"message_reference,omitempty"`
	Attachments     []Attachment      `json:"attachments"`
	Embeds          []Embed           `json:"embeds"`
	Reactions       []Reaction        `json:"reactions"`
}

// MessageReference points at the message this one replies to. Discord also
// sends the full referenced_message alongside it; the tool only keeps the
// id, since that is the address and the message stays fetchable by it.
type MessageReference struct {
	MessageID string `json:"message_id"`
}

// Edited reports whether the message has been edited since it was sent.
func (m Message) Edited() bool {
	return m.EditedTimestamp != nil
}

// ReplyTo is the id of the message this one replies to, or "" if it is not
// a reply.
func (m Message) ReplyTo() string {
	if m.Reference == nil {
		return ""
	}
	return m.Reference.MessageID
}

// Author is the part of a message author the tool uses.
type Author struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Bot        bool   `json:"bot"`
}

// DisplayName is the name shown for the author: display name, else handle.
func (a Author) DisplayName() string {
	if a.GlobalName != "" {
		return a.GlobalName
	}
	return a.Username
}

// Attachment is a file on a message.
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

// Embed is the part of an embed the human layout shows.
type Embed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// Reaction is one emoji and its count.
type Reaction struct {
	Count int `json:"count"`
	Emoji struct {
		Name string `json:"name"`
	} `json:"emoji"`
}

// UnmarshalJSON keeps the raw bytes and parses the known fields.
func (m *Message) UnmarshalJSON(b []byte) error {
	type plain Message
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*m = Message(p)
	m.Raw = append(json.RawMessage(nil), b...)
	return nil
}

// MarshalJSON writes the raw object back out untouched.
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.Raw) > 0 {
		return m.Raw, nil
	}
	type plain Message
	return json.Marshal(plain(m))
}

// Time parses the message timestamp.
func (m Message) Time() time.Time {
	t, err := time.Parse(time.RFC3339Nano, m.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// MessagePage is the largest page the messages endpoint serves.
const MessagePage = 100

// Messages fetches one page of a channel's messages, newest first. Exactly
// one of before and after may be set; the API rejects both together.
func (c *Client) Messages(ctx context.Context, channelID string, limit int, before, after string) ([]Message, error) {
	if limit <= 0 || limit > MessagePage {
		limit = MessagePage
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if before != "" {
		q.Set("before", before)
	} else if after != "" {
		q.Set("after", after)
	}
	var out []Message
	if err := c.Get(ctx, "/channels/"+channelID+"/messages", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Recent returns the newest n messages of a channel, oldest first, paging
// backwards with before.
func (c *Client) Recent(ctx context.Context, channelID string, n int) ([]Message, error) {
	var all []Message
	before := ""
	for len(all) < n {
		want := n - len(all)
		page, err := c.Messages(ctx, channelID, want, before, "")
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		all = append(all, page...)
		if len(page) < min(want, MessagePage) {
			break
		}
		before = page[len(page)-1].ID
	}
	// newest first -> oldest first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// History walks a channel's messages. With after set it pages forwards,
// each page's newest id becoming the next after; otherwise it pages
// backwards from the newest. onPage receives each page (newest first within
// the page) and the running total.
func (c *Client) History(ctx context.Context, channelID string, after string, onPage func(page []Message, total int)) ([]Message, error) {
	var all []Message
	before := ""
	for {
		page, err := c.Messages(ctx, channelID, MessagePage, before, after)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		all = append(all, page...)
		if onPage != nil {
			onPage(page, len(all))
		}
		if len(page) < MessagePage {
			break
		}
		if after != "" {
			after = page[0].ID // newest in the page
		} else {
			before = page[len(page)-1].ID // oldest in the page
		}
	}
	return all, nil
}
