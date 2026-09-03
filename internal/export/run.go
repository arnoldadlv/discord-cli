package export

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
)

// Status of one channel export.
type Status string

const (
	StatusExported Status = "exported"
	StatusUpToDate Status = "up-to-date"
	StatusFailed   Status = "failed"
)

// Result is what one channel export produced.
type Result struct {
	Path         string
	Status       Status
	MessageCount int
	NewMessages  int
}

// Target says where a channel's export belongs when none exists yet.
type Target struct {
	Guild   Guild
	Channel Channel
	// Dir is the directory under the write location, for example
	// <exports>/<guild> or <exports>/<guild>/threads/<parent>.
	Dir string
	// MetaDir is where the meta file lives (the guild directory).
	MetaDir string
}

// Runner exports one channel at a time with the incremental rules.
type Runner struct {
	Client    *discord.Client
	Locations []string // read locations, write location first
	Meta      *MetaStore
	Full      bool
	// Progress, when set, is told about each page fetched.
	Progress func(fetched int)
}

// Run exports one channel: find the existing export by id, fetch forwards
// from the recorded last message, merge, sort, write atomically, update meta.
func (r *Runner) Run(ctx context.Context, t Target) (Result, error) {
	existing := FindExisting(r.Locations, t.Channel.ID)

	path := ""
	metaDir := t.MetaDir
	var have []json.RawMessage
	if existing != nil && existing.Dialect == Native {
		path = existing.Path
		metaDir = metaDirFor(existing.Path, r.Locations)
		if metaDir == "" {
			metaDir = t.MetaDir
		}
	}
	if path == "" {
		path = FileName(t.Dir, t.Channel.Name, t.Channel.ID)
	}

	after := ""
	if !r.Full && existing != nil && existing.Dialect == Native {
		cm, ok, err := r.Meta.Get(metaDir, t.Channel.ID)
		if err != nil {
			return Result{}, err
		}
		if ok && cm.LastMessageID != "" {
			after = cm.LastMessageID
		} else if existing.DateRange.Before != nil {
			// No meta for this file: read it to learn the newest id.
			_, msgs, err := Read(existing.Path)
			if err == nil && len(msgs) > 0 {
				have = msgs
				after = newestID(msgs)
			}
		}
	}

	fetched, err := r.Client.History(ctx, t.Channel.ID, after, func(_ []discord.Message, total int) {
		if r.Progress != nil {
			r.Progress(total)
		}
	})
	if err != nil {
		return Result{Path: path, Status: StatusFailed}, err
	}
	if after != "" && len(fetched) == 0 {
		count := existing.MessageCount
		if count < 0 {
			count = 0
		}
		return Result{Path: path, Status: StatusUpToDate, MessageCount: count}, nil
	}

	var merged []json.RawMessage
	if after != "" {
		if have == nil {
			_, have, err = Read(existing.Path)
			if err != nil {
				return Result{Path: path, Status: StatusFailed}, err
			}
		}
		merged = have
	}
	seen := make(map[string]bool, len(merged))
	for _, m := range merged {
		seen[messageID(m)] = true
	}
	newCount := 0
	for _, m := range fetched {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		merged = append(merged, m.Raw)
		newCount++
	}
	sortMessages(merged)

	env := &Envelope{
		Guild:        t.Guild,
		Channel:      t.Channel,
		Messages:     merged,
		MessageCount: len(merged),
	}
	if len(merged) > 0 {
		first := messageTimestamp(merged[0])
		last := messageTimestamp(merged[len(merged)-1])
		env.DateRange = DateRange{After: &first, Before: &last}
	}
	if err := Write(path, env); err != nil {
		return Result{Path: path, Status: StatusFailed}, err
	}
	lastID := ""
	if len(merged) > 0 {
		lastID = messageID(merged[len(merged)-1])
	}
	if err := r.Meta.Set(metaDir, t.Channel.ID, lastID, len(merged)); err != nil {
		return Result{Path: path, Status: StatusFailed}, fmt.Errorf("updating export meta: %w", err)
	}
	return Result{Path: path, Status: StatusExported, MessageCount: len(merged), NewMessages: newCount}, nil
}

// metaDirFor finds the guild directory (direct child of a read location)
// that an export path sits under, where its meta file lives.
func metaDirFor(path string, locations []string) string {
	for _, loc := range locations {
		rel, err := filepath.Rel(loc, path)
		if err != nil || rel == "." || len(rel) > 1 && rel[:2] == ".." {
			continue
		}
		parts := splitPath(rel)
		if len(parts) >= 2 {
			return filepath.Join(loc, parts[0])
		}
	}
	return ""
}

func splitPath(p string) []string {
	var parts []string
	for p != "" && p != "." {
		dir, file := filepath.Split(p)
		parts = append([]string{file}, parts...)
		p = filepath.Clean(dir)
		if p == "/" || p == "." {
			break
		}
	}
	return parts
}

type messageKey struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
}

func messageID(raw json.RawMessage) string {
	var k messageKey
	_ = json.Unmarshal(raw, &k)
	return k.ID
}

func messageTimestamp(raw json.RawMessage) string {
	var k messageKey
	_ = json.Unmarshal(raw, &k)
	return k.Timestamp
}

func newestID(msgs []json.RawMessage) string {
	best := ""
	for _, m := range msgs {
		if id := messageID(m); compareIDs(id, best) > 0 {
			best = id
		}
	}
	return best
}

// compareIDs orders snowflakes numerically.
func compareIDs(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// sortMessages orders by timestamp ascending, then id, so the file is stable.
func sortMessages(msgs []json.RawMessage) {
	type keyed struct {
		t   time.Time
		raw string
		id  string
		m   json.RawMessage
	}
	ks := make([]keyed, len(msgs))
	for i, m := range msgs {
		var k messageKey
		_ = json.Unmarshal(m, &k)
		t, _ := time.Parse(time.RFC3339Nano, k.Timestamp)
		ks[i] = keyed{t: t, raw: k.Timestamp, id: k.ID, m: m}
	}
	sort.SliceStable(ks, func(i, j int) bool {
		if !ks[i].t.Equal(ks[j].t) {
			if ks[i].t.IsZero() || ks[j].t.IsZero() {
				return ks[i].raw < ks[j].raw
			}
			return ks[i].t.Before(ks[j].t)
		}
		return compareIDs(ks[i].id, ks[j].id) < 0
	})
	for i := range ks {
		msgs[i] = ks[i].m
	}
}
