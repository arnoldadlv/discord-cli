// Package export reads and writes exports: the DiscordChatExporter envelope
// around raw API messages (ADR-0003), the per-guild export meta, and the
// rules for finding an existing export by channel id across the three read
// locations (ADR-0004).
package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/arnoldadlv/discord-cli/internal/store"
)

// Dialect says who wrote an export.
type Dialect string

const (
	// Native exports are written by this tool or the Node CLI: raw API messages.
	Native Dialect = "native"
	// Legacy exports are written by DiscordChatExporter: its own message shape.
	Legacy Dialect = "legacy"
)

// Guild is the envelope's guild.
type Guild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Channel is the envelope's channel. Type is numeric in native exports and a
// string in legacy ones; both are kept.
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	TypeName string `json:"-"`
}

// DateRange is the span of the stored messages (native) or the requested
// filter (legacy).
type DateRange struct {
	After  *string `json:"after"`
	Before *string `json:"before"`
}

// Envelope is a native export as written to disk. Field order is the file's
// key order, matching the Node CLI.
type Envelope struct {
	Guild        Guild             `json:"guild"`
	Channel      Channel           `json:"channel"`
	DateRange    DateRange         `json:"dateRange"`
	Messages     []json.RawMessage `json:"messages"`
	MessageCount int               `json:"messageCount"`
}

// Header is what can be learned about an export without reading its
// messages: the envelope minus the messages, plus the dialect.
type Header struct {
	Path         string
	Dialect      Dialect
	Guild        Guild
	Channel      Channel
	DateRange    DateRange
	ExportedAt   string
	MessageCount int // -1 when it could not be read cheaply
	Size         int64
	ModTime      int64 // unix nanoseconds
}

type rawChannel struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Type json.RawMessage `json:"type"`
}

func (rc rawChannel) channel() (Channel, bool) {
	c := Channel{ID: rc.ID, Name: rc.Name}
	legacy := false
	var n int
	if err := json.Unmarshal(rc.Type, &n); err == nil {
		c.Type = n
	} else {
		var s string
		if json.Unmarshal(rc.Type, &s) == nil {
			c.TypeName = s
			legacy = true
		}
	}
	return c, legacy
}

var messageCountTail = regexp.MustCompile(`"messageCount"\s*:\s*(\d+)`)

// ReadHeader reads the envelope of an export without parsing its messages.
// It streams the top-level keys before "messages" and reads messageCount
// from the file's tail, so it is cheap even on very large files.
func ReadHeader(path string) (*Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	h := &Header{Path: path, Dialect: Native, MessageCount: -1, Size: info.Size(), ModTime: info.ModTime().UnixNano()}

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("%s: not an export (top level is not an object)", path)
	}
	sawChannel, sawGuild := false, false
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		key, _ := keyTok.(string)
		switch key {
		case "guild":
			if err := dec.Decode(&h.Guild); err != nil {
				return nil, fmt.Errorf("%s: guild: %w", path, err)
			}
			sawGuild = true
		case "channel":
			var rc rawChannel
			if err := dec.Decode(&rc); err != nil {
				return nil, fmt.Errorf("%s: channel: %w", path, err)
			}
			var legacy bool
			h.Channel, legacy = rc.channel()
			if legacy {
				h.Dialect = Legacy
			}
			sawChannel = true
		case "dateRange":
			if err := dec.Decode(&h.DateRange); err != nil {
				return nil, fmt.Errorf("%s: dateRange: %w", path, err)
			}
		case "exportedAt":
			if err := dec.Decode(&h.ExportedAt); err != nil {
				return nil, fmt.Errorf("%s: exportedAt: %w", path, err)
			}
			h.Dialect = Legacy
		case "messageCount":
			if err := dec.Decode(&h.MessageCount); err != nil {
				return nil, fmt.Errorf("%s: messageCount: %w", path, err)
			}
		case "messages":
			// Everything after this is the bulk of the file; stop here.
			if !sawGuild || !sawChannel {
				return nil, fmt.Errorf("%s: not an export (messages before guild and channel)", path)
			}
			if h.MessageCount < 0 {
				h.MessageCount = countFromTail(f, info.Size())
			}
			return h, nil
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, key, err)
			}
		}
	}
	if !sawGuild || !sawChannel {
		return nil, fmt.Errorf("%s: not an export (no guild or channel)", path)
	}
	return h, nil
}

// countFromTail reads messageCount from the last bytes of the file, where
// both dialects put it.
func countFromTail(f *os.File, size int64) int {
	const tail = 512
	off := size - tail
	if off < 0 {
		off = 0
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return -1
	}
	m := messageCountTail.FindAllSubmatch(buf, -1)
	if len(m) == 0 {
		return -1
	}
	n, err := strconv.Atoi(string(m[len(m)-1][1]))
	if err != nil {
		return -1
	}
	return n
}

// Read parses a whole export. Messages are returned as raw objects in file
// order, whichever dialect wrote them.
func Read(path string) (*Header, []json.RawMessage, error) {
	h, err := ReadHeader(path)
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var e struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	h.MessageCount = len(e.Messages)
	return h, e.Messages, nil
}

// Write writes a native export atomically with two-space indentation, the
// way the Node CLI did.
func Write(path string, env *Envelope) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return err
	}
	data := bytes.TrimRight(buf.Bytes(), "\n")
	return store.WriteFileAtomic(path, data, 0o644)
}

// IsExportFile reports whether a directory entry looks like an export.
func IsExportFile(name string) bool {
	return strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".")
}
