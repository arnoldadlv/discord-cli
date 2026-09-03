package clitest

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Hand-written fixtures with invented names. No real guilds, channels,
// people, or messages ever appear here.

// Guilds is the /users/@me/guilds response.
func Guilds() []map[string]any {
	return []map[string]any{
		{"id": "1001", "name": "Cooey COE", "owner": false, "approximate_member_count": 1200, "approximate_presence_count": 80},
		{"id": "1002", "name": "📚 Book Club", "owner": true, "approximate_member_count": 12, "approximate_presence_count": 3},
		{"id": "1003", "name": "Cooey Alumni", "owner": false, "approximate_member_count": 300, "approximate_presence_count": 10},
	}
}

// Guild is the /guilds/{id} response for a guild in Guilds.
func Guild(id string) map[string]any {
	for _, g := range Guilds() {
		if g["id"] == id {
			return g
		}
	}
	return nil
}

// Channels is the /guilds/{id}/channels response for guild 1001.
func Channels() []map[string]any {
	return []map[string]any{
		{"id": "2000", "name": "Text Channels", "type": 4, "position": 0},
		{"id": "2001", "name": "🔮general", "type": 0, "position": 0, "parent_id": "2000"},
		{"id": "2002", "name": "📰news", "type": 5, "position": 1, "parent_id": "2000"},
		{"id": "2003", "name": "cmmc-general", "type": 0, "position": 2, "parent_id": "2000"},
		{"id": "2004", "name": "Voice", "type": 4, "position": 1},
		{"id": "2005", "name": "Lounge", "type": 2, "position": 0, "parent_id": "2004"},
		{"id": "2006", "name": "help-forum", "type": 15, "position": 0, "parent_id": "2010"},
		{"id": "2010", "name": "Support", "type": 4, "position": 2},
		{"id": "2007", "name": "random", "type": 0, "position": 5},
	}
}

// Threads returns the thread objects for a parent channel, split into
// active and archived, for the per-channel thread search endpoint.
func Threads(parentID string) (active, archived []map[string]any) {
	switch parentID {
	case "2001":
		active = []map[string]any{
			{"id": "3001", "name": "welcome thread", "type": 11, "parent_id": "2001", "thread_metadata": map[string]any{"archived": false}},
		}
		archived = []map[string]any{
			{"id": "3002", "name": "old planning", "type": 11, "parent_id": "2001", "thread_metadata": map[string]any{"archived": true, "archive_timestamp": "2026-01-02T03:04:05.000000+00:00"}},
		}
	case "2006":
		active = []map[string]any{
			{"id": "3003", "name": "How do I scope?", "type": 11, "parent_id": "2006", "thread_metadata": map[string]any{"archived": false}},
		}
	}
	return active, archived
}

// Message builds one raw API message object with invented content. Ids and
// timestamps rise with n so paging tests can reason about order.
func Message(channelID string, n int) map[string]any {
	ts := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Minute)
	authors := []map[string]any{
		{"id": "9001", "username": "ana", "global_name": "Ana", "discriminator": "0"},
		{"id": "9002", "username": "kyle", "global_name": "Kyle B", "discriminator": "0"},
		{"id": "9003", "username": "newsbot", "global_name": nil, "discriminator": "0"},
	}
	m := map[string]any{
		"id":               MessageID(n),
		"type":             0,
		"channel_id":       channelID,
		"author":           authors[n%3],
		"content":          fmt.Sprintf("message %d about topic %s", n, []string{"access control", "policy", "scoping"}[n%3]),
		"timestamp":        ts.Format("2006-01-02T15:04:05.000000+00:00"),
		"edited_timestamp": nil,
		"attachments":      []any{},
		"embeds":           []any{},
		"mentions":         []any{},
		"mention_roles":    []any{},
		"pinned":           false,
		"mention_everyone": false,
		"tts":              false,
		"flags":            0,
		"components":       []any{},
	}
	switch n % 5 {
	case 1:
		m["attachments"] = []map[string]any{{"id": "7" + MessageID(n), "filename": "report.pdf", "size": 1234, "url": "https://cdn.example.test/report.pdf"}}
	case 2:
		m["content"] = ""
		m["embeds"] = []map[string]any{{"type": "rich", "title": "Weekly digest", "description": "Three <b>things</b> happened this week.", "url": "https://news.example.test/digest"}}
	case 3:
		m["reactions"] = []map[string]any{{"count": 3, "emoji": map[string]any{"id": nil, "name": "👍"}}, {"count": 1, "emoji": map[string]any{"id": nil, "name": "🎉"}}}
	}
	return m
}

// MessageID is the id of the nth fixture message.
func MessageID(n int) string { return fmt.Sprintf("%d", 5000000+n) }

// Messages builds n messages for a channel, oldest first.
func Messages(channelID string, n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = Message(channelID, i+1)
	}
	return out
}

// ServeMessages registers the channel messages endpoint with Discord's
// semantics: newest first, limit, and before/after as exclusive id bounds.
// The store is returned so a test can append messages between runs.
func ServeMessages(f *FakeDiscord, channelID string, msgs []map[string]any) *MessageStore {
	s := &MessageStore{msgs: msgs}
	f.Handle("/channels/"+channelID+"/messages", func(req *http.Request) Response {
		q := req.URL.Query()
		limit := 50
		if l := q.Get("limit"); l != "" {
			limit = atoi(l)
		}
		before, after := q.Get("before"), q.Get("after")
		if before != "" && after != "" {
			return Response{Status: 400, Body: `{"message":"before and after together","code":50035}`}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		var pick []map[string]any
		if after != "" {
			// Oldest first above the bound, then take the oldest limit and return newest first.
			for _, m := range s.msgs {
				if atoi(m["id"].(string)) > atoi(after) {
					pick = append(pick, m)
				}
			}
			if len(pick) > limit {
				pick = pick[:limit]
			}
		} else {
			for i := len(s.msgs) - 1; i >= 0; i-- {
				m := s.msgs[i]
				if before != "" && atoi(m["id"].(string)) >= atoi(before) {
					continue
				}
				pick = append(pick, m)
				if len(pick) == limit {
					break
				}
			}
			return Response{Status: 200, Body: nonNil(pick)}
		}
		// reverse to newest first
		out := make([]map[string]any, len(pick))
		for i, m := range pick {
			out[len(pick)-1-i] = m
		}
		return Response{Status: 200, Body: nonNil(out)}
	})
	return s
}

// MessageStore holds a channel's messages for the fake server.
type MessageStore struct {
	mu   sync.Mutex
	msgs []map[string]any
}

// Append adds messages (oldest first) after the existing ones.
func (s *MessageStore) Append(msgs ...map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msgs...)
}

func nonNil(m []map[string]any) []map[string]any {
	if m == nil {
		return []map[string]any{}
	}
	return m
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
