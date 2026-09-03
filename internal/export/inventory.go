package export

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/arnoldadlv/discord-cli/internal/resolve"
)

// Location labels for the three read locations, in precedence order: the
// XDG data directory, the Node CLI's directory, and DiscordChatExporter's.
const (
	LocationXDG          = "xdg"
	LocationNode         = "node"
	LocationChatExporter = "chatexporter"
)

// LocationLabels names the read locations in the order Paths.ReadLocations
// returns them.
var LocationLabels = []string{LocationXDG, LocationNode, LocationChatExporter}

// Item is one export on disk with everything the inventory shows.
type Item struct {
	*Header
	Location   string
	LastExport string // from meta, empty when unknown
}

// Inventory lists every export under the read locations with its meta.
func Inventory(locations []string) []Item {
	metas := map[string]*Meta{}
	metaFor := func(dir string) *Meta {
		if m, ok := metas[dir]; ok {
			return m
		}
		m, err := LoadMeta(dir)
		if err != nil {
			m = &Meta{Channels: map[string]ChannelMeta{}}
		}
		metas[dir] = m
		return m
	}
	var items []Item
	for i, loc := range locations {
		label := ""
		if i < len(LocationLabels) {
			label = LocationLabels[i]
		}
		for _, h := range Walk([]string{loc}) {
			it := Item{Header: h, Location: label}
			if dir := metaDirFor(h.Path, []string{loc}); dir != "" {
				if cm, ok := metaFor(dir).Channels[h.Channel.ID]; ok {
					it.LastExport = cm.LastExport
				}
			}
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(a, b int) bool {
		ga, gb := strings.ToLower(items[a].Guild.Name), strings.ToLower(items[b].Guild.Name)
		if ga != gb {
			return ga < gb
		}
		ca, cb := strings.ToLower(items[a].Channel.Name), strings.ToLower(items[b].Channel.Name)
		if ca != cb {
			return ca < cb
		}
		return items[a].Path < items[b].Path
	})
	return items
}

// MatchesGuild reports whether an item belongs to the guild named by input:
// the envelope guild id, its name (exact or normalised), or the directory
// the export sits in.
func (it Item) MatchesGuild(input string) bool {
	in := strings.TrimSpace(input)
	if in == "" {
		return true
	}
	if resolve.IsID(in) {
		return it.Guild.ID == in
	}
	lower := strings.ToLower(in)
	norm := resolve.Key(in)
	if strings.ToLower(it.Guild.Name) == lower || resolve.Key(it.Guild.Name) == norm {
		return true
	}
	dir := filepath.Base(filepath.Dir(it.Path))
	if strings.Contains(it.Path, string(filepath.Separator)+"threads"+string(filepath.Separator)) {
		// threads/<parent>/<thread>.json: the guild dir is two levels up.
		dir = filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(it.Path))))
	}
	return strings.ToLower(dir) == lower || resolve.Key(dir) == norm
}
