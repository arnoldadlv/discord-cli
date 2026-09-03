package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the JSON configuration file.
type Config struct {
	DefaultGuild string `json:"default-guild,omitempty"`
}

// ConfigKeys lists every configuration key, for help and validation.
var ConfigKeys = []string{"default-guild"}

// LoadConfig reads the config file; a missing file is an empty config.
func LoadConfig(p Paths) (Config, error) {
	var c Config
	b, err := os.ReadFile(p.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", p.ConfigFile(), err)
	}
	return c, nil
}

// SaveConfig writes the config file atomically.
func SaveConfig(p Paths, c Config) error {
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(p.ConfigFile(), append(b, '\n'), 0o644)
}

// Get returns the value of a key.
func (c Config) Get(key string) (string, bool) {
	switch key {
	case "default-guild":
		return c.DefaultGuild, true
	}
	return "", false
}

// WriteFileAtomic writes to a temporary file in the same directory and
// renames it into place, so a crash never leaves a half-written file.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
