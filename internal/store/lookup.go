package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// LookupTTL is how long the guild, channel, and DM lists are trusted.
const LookupTTL = 24 * time.Hour

type lookupEntry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Data      json.RawMessage `json:"data"`
}

// LookupCache stores short-lived lists used to turn names into ids.
type LookupCache struct {
	Dir    string
	Now    func() time.Time
	Bypass bool // --no-cache: always fetch, but still store
}

// Get returns the cached value for name when it is younger than the TTL,
// or fetches, stores, and returns a fresh one.
func (l LookupCache) Get(name string, fetch func() (any, error), out any) (fromCache bool, err error) {
	path := filepath.Join(l.Dir, name+".json")
	if !l.Bypass {
		if b, err := os.ReadFile(path); err == nil {
			var e lookupEntry
			if json.Unmarshal(b, &e) == nil && l.Now().Sub(e.FetchedAt) < LookupTTL && l.Now().After(e.FetchedAt.Add(-time.Minute)) {
				if json.Unmarshal(e.Data, out) == nil {
					return true, nil
				}
			}
		}
	}
	v, err := fetch()
	if err != nil {
		return false, err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false, err
	}
	b, err := json.MarshalIndent(lookupEntry{FetchedAt: l.Now(), Data: data}, "", "  ")
	if err != nil {
		return false, err
	}
	if err := WriteFileAtomic(path, b, 0o644); err != nil {
		return false, err
	}
	return false, nil
}

// Age returns how old a cached entry is, or false when it is absent.
func (l LookupCache) Age(name string) (time.Duration, bool) {
	b, err := os.ReadFile(filepath.Join(l.Dir, name+".json"))
	if err != nil {
		return 0, false
	}
	var e lookupEntry
	if json.Unmarshal(b, &e) != nil {
		return 0, false
	}
	return l.Now().Sub(e.FetchedAt), true
}

// Clear removes every cached list.
func (l LookupCache) Clear() error {
	err := os.RemoveAll(l.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
