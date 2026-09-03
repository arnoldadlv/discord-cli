package export

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/arnoldadlv/discord-cli/internal/resolve"
)

// Walk lists every export header under the read locations, in location
// order then path order. Unreadable files are skipped; a non-export JSON
// file is skipped too.
func Walk(locations []string) []*Header {
	var out []*Header
	for _, loc := range locations {
		var paths []string
		_ = filepath.WalkDir(loc, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if IsExportFile(d.Name()) {
				paths = append(paths, path)
			}
			return nil
		})
		sort.Strings(paths)
		for _, p := range paths {
			h, err := ReadHeader(p)
			if err != nil {
				continue
			}
			out = append(out, h)
		}
	}
	return out
}

// FindExisting locates the export of a channel by its envelope id, never by
// filename, looking through the read locations in order.
func FindExisting(locations []string, channelID string) *Header {
	for _, h := range Walk(locations) {
		if h.Channel.ID == channelID {
			return h
		}
	}
	return nil
}

// GuildDirName is the directory a guild's exports live in.
func GuildDirName(guildName string) string {
	n := resolve.Normalize(guildName)
	if n == "" || n == "-" {
		return "guild"
	}
	return n
}

// FileName picks the file for a new export: the normalised name, or with
// the id appended when a different channel already owns that name.
func FileName(dir, name, channelID string) string {
	base := resolve.Normalize(name)
	if base == "" || base == "-" {
		base = channelID
	}
	path := filepath.Join(dir, base+".json")
	if h, err := ReadHeader(path); err == nil && h.Channel.ID != channelID {
		return filepath.Join(dir, base+"-"+channelID+".json")
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := ReadHeader(path); err != nil {
			// Not an export; do not clobber it.
			return filepath.Join(dir, base+"-"+channelID+".json")
		}
	}
	return path
}
