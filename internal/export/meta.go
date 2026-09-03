package export

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/store"
)

// MetaFile is the name of the per-directory export meta.
const MetaFile = ".meta.json"

// ChannelMeta records where one channel's last export stopped.
type ChannelMeta struct {
	LastMessageID string `json:"lastMessageId"`
	LastExport    string `json:"lastExport"`
	MessageCount  int    `json:"messageCount"`
}

// Meta is the per-guild (or DM directory) export meta, in the Node CLI's
// shape so its existing files are read as-is.
type Meta struct {
	Channels   map[string]ChannelMeta `json:"channels"`
	LastExport *string                `json:"lastExport"`
}

// LoadMeta reads the meta of a directory; missing means empty.
func LoadMeta(dir string) (*Meta, error) {
	m := &Meta{Channels: map[string]ChannelMeta{}}
	b, err := os.ReadFile(filepath.Join(dir, MetaFile))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, m); err != nil {
		// A corrupt meta only costs a full refetch; start over.
		return &Meta{Channels: map[string]ChannelMeta{}}, nil
	}
	if m.Channels == nil {
		m.Channels = map[string]ChannelMeta{}
	}
	return m, nil
}

// MetaStore serialises meta updates per directory, so concurrent channel
// exports never lose each other's entries.
type MetaStore struct {
	Now func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewMetaStore builds a store using now for timestamps.
func NewMetaStore(now func() time.Time) *MetaStore {
	return &MetaStore{Now: now, locks: map[string]*sync.Mutex{}}
}

func (s *MetaStore) lock(dir string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[dir]
	if !ok {
		l = &sync.Mutex{}
		s.locks[dir] = l
	}
	return l
}

// lockDir takes a directory's lock and returns the function that releases
// it, for callers that must choose a file name and write it as one step.
func (s *MetaStore) lockDir(dir string) func() {
	l := s.lock(dir)
	l.Lock()
	return l.Unlock
}

// Get returns the meta entry of a channel in a directory.
func (s *MetaStore) Get(dir, channelID string) (ChannelMeta, bool, error) {
	l := s.lock(dir)
	l.Lock()
	defer l.Unlock()
	m, err := LoadMeta(dir)
	if err != nil {
		return ChannelMeta{}, false, err
	}
	cm, ok := m.Channels[channelID]
	return cm, ok, nil
}

// Set records a channel's state and the directory's last export time,
// reading and rewriting the file under the directory's lock.
func (s *MetaStore) Set(dir, channelID string, lastMessageID string, count int) error {
	l := s.lock(dir)
	l.Lock()
	defer l.Unlock()
	m, err := LoadMeta(dir)
	if err != nil {
		return err
	}
	now := s.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	m.Channels[channelID] = ChannelMeta{LastMessageID: lastMessageID, LastExport: now, MessageCount: count}
	m.LastExport = &now
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(filepath.Join(dir, MetaFile), b, 0o644)
}
