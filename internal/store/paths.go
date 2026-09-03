// Package store knows where everything lives on disk: XDG directories for
// what the tool writes, plus the two legacy read locations (ADR-0004).
package store

import (
	"path/filepath"
)

const appDir = "discord-cli"

// Paths are the directories one run uses.
type Paths struct {
	Home      string
	ConfigDir string // <XDG_CONFIG_HOME>/discord-cli
	DataDir   string // <XDG_DATA_HOME>/discord-cli
	CacheDir  string // <XDG_CACHE_HOME>/discord-cli
}

// PathsFromEnv resolves the directories from HOME and the XDG variables.
func PathsFromEnv(getenv func(string) string) Paths {
	home := getenv("HOME")
	pick := func(v, fallback string) string {
		if x := getenv(v); x != "" {
			return filepath.Join(x, appDir)
		}
		return filepath.Join(home, fallback, appDir)
	}
	return Paths{
		Home:      home,
		ConfigDir: pick("XDG_CONFIG_HOME", ".config"),
		DataDir:   pick("XDG_DATA_HOME", filepath.Join(".local", "share")),
		CacheDir:  pick("XDG_CACHE_HOME", ".cache"),
	}
}

// TokenFile holds the user token, mode 0600.
func (p Paths) TokenFile() string { return filepath.Join(p.ConfigDir, "token") }

// ConfigFile holds the JSON configuration (default guild).
func (p Paths) ConfigFile() string { return filepath.Join(p.ConfigDir, "config.json") }

// ExportsDir is where new exports are written.
func (p Paths) ExportsDir() string { return filepath.Join(p.DataDir, "exports") }

// LegacyExportsDir is the Node CLI's export directory, read only.
func (p Paths) LegacyExportsDir() string { return filepath.Join(p.Home, ".discord-cli", "exports") }

// DCEExportsDir is the DiscordChatExporter folder, read only.
func (p Paths) DCEExportsDir() string {
	return filepath.Join(p.Home, "DiscordChatExporter.Cli.osx-arm64", "exports")
}

// ReadLocations are every place exports are read from, in precedence order.
func (p Paths) ReadLocations() []string {
	return []string{p.ExportsDir(), p.LegacyExportsDir(), p.DCEExportsDir()}
}

// LookupCacheDir holds the cached guild, channel, and DM lists.
func (p Paths) LookupCacheDir() string { return filepath.Join(p.CacheDir, "lookup") }

// IndexFile is the SQLite search index.
func (p Paths) IndexFile() string { return filepath.Join(p.CacheDir, "index.sqlite") }
