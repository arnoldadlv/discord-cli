// Package search is local search over exports on disk: a scan that reads
// every export (the permanent fallback) and a SQLite full-text index
// derived from the same files (ADR-0006). Both produce the same results in
// the same order.
package search

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/export"
)

// Result is one matching message, normalised across both export dialects.
type Result struct {
	GuildID     string
	GuildName   string
	ChannelID   string
	ChannelName string
	MessageID   string
	Author      string // display name: native author.username; legacy nickname, else name
	Timestamp   string // as stored in the export
	Content     string
	File        string
	time        time.Time
}

// Time is the parsed timestamp.
func (r Result) Time() time.Time { return r.time }

// Query is what a search filters on.
type Query struct {
	Terms  []string // lowercase; any term matching content is a hit
	Author string   // lowercase substring of the display name; empty for any
	After  time.Time
	Before time.Time
}

// Terms splits a query string into lowercase terms.
func Terms(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// Matches reports whether a result satisfies the query.
func (q Query) Matches(r Result) bool {
	if len(q.Terms) > 0 {
		content := strings.ToLower(r.Content)
		hit := false
		for _, t := range q.Terms {
			if strings.Contains(content, t) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if q.Author != "" && !strings.Contains(strings.ToLower(r.Author), q.Author) {
		return false
	}
	if !q.After.IsZero() && r.time.Before(q.After) {
		return false
	}
	if !q.Before.IsZero() && r.time.After(q.Before) {
		return false
	}
	return true
}

// FromMessage builds a result with just what a query can filter on, for
// callers that hold live messages rather than an export.
func FromMessage(content, author, timestamp string) Result {
	return Result{Content: content, Author: author, Timestamp: timestamp, time: ParseTime(timestamp)}
}

// rawMessage is the union of the fields both dialects carry.
type rawMessage struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		Username string `json:"username"` // native
		Name     string `json:"name"`     // legacy handle
		Nickname string `json:"nickname"` // legacy guild nickname
	} `json:"author"`
}

// ParseTime parses either dialect's timestamp.
func ParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Messages turns one export's raw messages into results.
func Messages(h *export.Header, msgs []json.RawMessage) []Result {
	out := make([]Result, 0, len(msgs))
	for _, raw := range msgs {
		var m rawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		author := m.Author.Username
		if h.Dialect == export.Legacy {
			author = m.Author.Nickname
			if author == "" {
				author = m.Author.Name
			}
		}
		out = append(out, Result{
			GuildID:     h.Guild.ID,
			GuildName:   h.Guild.Name,
			ChannelID:   h.Channel.ID,
			ChannelName: h.Channel.Name,
			MessageID:   m.ID,
			Author:      author,
			Timestamp:   m.Timestamp,
			Content:     m.Content,
			File:        h.Path,
			time:        ParseTime(m.Timestamp),
		})
	}
	return out
}

// Scan reads every export and returns the matches, newest first.
func Scan(items []export.Item, q Query) ([]Result, error) {
	var hits []Result
	for _, it := range items {
		_, msgs, err := export.Read(it.Path)
		if err != nil {
			continue
		}
		for _, r := range Messages(it.Header, msgs) {
			if q.Matches(r) {
				hits = append(hits, r)
			}
		}
	}
	Sort(hits)
	return hits, nil
}

// Sort orders results newest first, then by message id descending, then by
// file, so the scan and the index agree exactly.
func Sort(rs []Result) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].time.Equal(rs[j].time) {
			return rs[i].time.After(rs[j].time)
		}
		if rs[i].MessageID != rs[j].MessageID {
			return export.CompareIDs(rs[i].MessageID, rs[j].MessageID) > 0
		}
		return rs[i].File < rs[j].File
	})
}
