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
// semantics: newest first, limit, before/after as exclusive id bounds, and
// around as a window centred on a message. The store is returned so a test
// can append messages between runs.
func ServeMessages(f *FakeDiscord, channelID string, msgs []map[string]any) *MessageStore {
	s := &MessageStore{msgs: msgs}
	f.Handle("/channels/"+channelID+"/messages", func(req *http.Request) Response {
		q := req.URL.Query()
		limit := 50
		if l := q.Get("limit"); l != "" {
			limit = atoi(l)
		}
		before, after, around := q.Get("before"), q.Get("after"), q.Get("around")
		if (before != "" && after != "") || (around != "" && (before != "" || after != "")) {
			return Response{Status: 400, Body: `{"message":"before, after, and around are exclusive","code":50035}`}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if around != "" {
			return Response{Status: 200, Body: nonNil(s.around(around, limit))}
		}
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

// around returns the window of messages centred on a message id, newest
// first, as Discord's around parameter does: half the limit either side,
// fewer at the ends of the channel. An unknown id matches nothing. The
// caller holds the lock.
func (s *MessageStore) around(id string, limit int) []map[string]any {
	at := -1
	for i, m := range s.msgs {
		if m["id"].(string) == id {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	half := (limit - 1) / 2
	lo := max(at-half, 0)
	hi := min(at+half, len(s.msgs)-1)
	out := make([]map[string]any, 0, hi-lo+1)
	for i := hi; i >= lo; i-- { // newest first
		out = append(out, s.msgs[i])
	}
	return out
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

// ChannelsWithCollision is Channels plus a second channel that normalises
// to "general", for the filename collision test.
func ChannelsWithCollision() []map[string]any {
	return append(Channels(), map[string]any{"id": "2011", "name": "✨general", "type": 0, "position": 9, "parent_id": "2000"})
}

// NativeExport builds a Node-CLI-shaped export envelope for a channel with
// the given fixture messages.
func NativeExport(guildID, guildName, channelID, channelName string, channelType int, msgs []map[string]any) map[string]any {
	var after, before any
	if len(msgs) > 0 {
		after = msgs[0]["timestamp"]
		before = msgs[len(msgs)-1]["timestamp"]
	}
	return map[string]any{
		"guild":        map[string]any{"id": guildID, "name": guildName},
		"channel":      map[string]any{"id": channelID, "name": channelName, "type": channelType},
		"dateRange":    map[string]any{"after": after, "before": before},
		"messages":     msgs,
		"messageCount": len(msgs),
	}
}

// LegacyExport builds a DiscordChatExporter-shaped export with n messages.
func LegacyExport(guildID, guildName, channelID, channelName string, n int) map[string]any {
	msgs := make([]map[string]any, n)
	for i := range msgs {
		ts := time.Date(2025, 3, 1, 9, 0, 0, 0, time.FixedZone("PDT", -7*3600)).Add(time.Duration(i) * time.Minute)
		msgs[i] = map[string]any{
			"id":                 fmt.Sprintf("%d", 4000000+i+1),
			"type":               "Default",
			"timestamp":          ts.Format("2006-01-02T15:04:05.000-07:00"),
			"timestampEdited":    nil,
			"callEndedTimestamp": nil,
			"isPinned":           false,
			"content":            fmt.Sprintf("legacy note %d about %s", i+1, []string{"policy", "evidence"}[i%2]),
			"author": map[string]any{
				"id": "9101", "name": "tim.h", "discriminator": "0000", "nickname": "Tim", "color": nil, "isBot": false,
				"roles": []any{}, "avatarUrl": "https://cdn.example.test/a.png",
			},
			"attachments": []any{}, "embeds": []any{}, "stickers": []any{}, "reactions": []any{}, "mentions": []any{}, "inlineEmojis": []any{},
		}
	}
	return map[string]any{
		"guild":        map[string]any{"id": guildID, "name": guildName, "iconUrl": "https://cdn.example.test/icon.png"},
		"channel":      map[string]any{"id": channelID, "type": "GuildTextChat", "categoryId": "2000", "category": "Text Channels", "name": channelName, "topic": nil},
		"dateRange":    map[string]any{"after": nil, "before": nil},
		"exportedAt":   "2025-03-02T00:00:00.000-07:00",
		"messages":     msgs,
		"messageCount": n,
	}
}

// ServeSearch registers the guild message search endpoint over a pool of
// n hit messages spread across channels, newest first, honouring limit and
// offset and echoing total_results. Messages come back as array of arrays.
func ServeSearch(f *FakeDiscord, guildID string, n int) {
	channels := []string{"2001", "2002", "2003"}
	pool := make([]map[string]any, n)
	for i := range pool {
		m := Message(channels[i%3], n-i) // newest first
		m["hit"] = true
		delete(m, "reactions")
		pool[i] = m
	}
	f.Handle("/guilds/"+guildID+"/messages/search", func(req *http.Request) Response {
		q := req.URL.Query()
		limit := 25
		if l := q.Get("limit"); l != "" {
			limit = atoi(l)
		}
		if limit > 25 {
			limit = 25
		}
		offset := atoi(q.Get("offset"))
		var page []any
		for i := offset; i < len(pool) && len(page) < limit; i++ {
			page = append(page, []any{pool[i]})
		}
		if page == nil {
			page = []any{}
		}
		return Response{Status: 200, Body: map[string]any{
			"analytics_id":                "x",
			"doing_deep_historical_index": false,
			"total_results":               n,
			"messages":                    page,
		}}
	})
}

// DMs is the /users/@me/channels response: two DMs and two group DMs.
func DMs() []map[string]any {
	kyle := map[string]any{"id": "9002", "username": "kyle", "global_name": "Kyle B", "discriminator": "0"}
	ana := map[string]any{"id": "9001", "username": "ana", "global_name": "Ana", "discriminator": "0"}
	maria := map[string]any{"id": "9004", "username": "maria", "global_name": "Maria", "discriminator": "0"}
	return []map[string]any{
		{"id": "6001", "type": 1, "recipients": []any{kyle}, "last_message_id": "5000010"},
		{"id": "6002", "type": 3, "name": "Study Group", "recipients": []any{kyle, ana}, "last_message_id": "5000020", "owner_id": "100"},
		{"id": "6003", "type": 1, "recipients": []any{maria}, "last_message_id": nil},
		{"id": "6004", "type": 3, "name": nil, "recipients": []any{ana, maria}, "last_message_id": "5000030", "owner_id": "9001"},
	}
}

// DMsWithCollision is DMs plus a group whose name normalises to "maria",
// colliding with the DM with maria.
func DMsWithCollision() []map[string]any {
	ana := map[string]any{"id": "9001", "username": "ana", "global_name": "Ana", "discriminator": "0"}
	maria := map[string]any{"id": "9004", "username": "maria", "global_name": "Maria", "discriminator": "0"}
	return append(DMs(), map[string]any{"id": "6005", "type": 3, "name": "Maria!", "recipients": []any{ana, maria}, "last_message_id": "5000040", "owner_id": "9004"})
}
