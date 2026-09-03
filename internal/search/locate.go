package search

import (
	"database/sql"
	"errors"

	"github.com/arnoldadlv/discord-cli/internal/export"
)

// Location is where one message lives: the export file holding it, and the
// guild and channel that file covers. It is the address a message id has to
// be turned into before the message itself can be read.
type Location struct {
	File        string
	GuildID     string
	GuildName   string
	ChannelID   string
	ChannelName string
}

// Locate finds the export holding a message id, or nil when the index has
// never seen it. A message stored in more than one export resolves to the
// first file by path; they hold the same message.
func (ix *Index) Locate(messageID string) (*Location, error) {
	row := ix.db.QueryRow(`SELECT f.path, f.guild_id, f.guild_name, f.channel_id, f.channel_name
		FROM messages m JOIN files f ON f.path = m.file
		WHERE m.message_id = ? ORDER BY f.path LIMIT 1`, messageID)
	var l Location
	err := row.Scan(&l.File, &l.GuildID, &l.GuildName, &l.ChannelID, &l.ChannelName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ScanLocate finds the export holding a message id by reading the exports,
// the way Scan searches them: the permanent fallback for when the index is
// missing or out of date. It returns nil when no export holds the id.
func ScanLocate(items []export.Item, messageID string) *Location {
	for _, it := range items {
		h, msgs, err := export.Read(it.Path)
		if err != nil {
			continue
		}
		for _, r := range Messages(h, msgs) {
			if r.MessageID == messageID {
				return &Location{
					File:        h.Path,
					GuildID:     h.Guild.ID,
					GuildName:   h.Guild.Name,
					ChannelID:   h.Channel.ID,
					ChannelName: h.Channel.Name,
				}
			}
		}
	}
	return nil
}
