package clitest

// Hand-written fixtures with invented names. No real guilds, channels,
// people, or messages ever appear here.

// Guilds is the /users/@me/guilds response.
func Guilds() []map[string]any {
	return []map[string]any{
		{"id": "1001", "name": "Cooey COE", "owner": false, "approximate_member_count": 1200, "approximate_presence_count": 80},
		{"id": "1002", "name": "📚 Book Club", "owner": true, "approximate_member_count": 12, "approximate_presence_count": 3},
		{"id": "1003", "name": "Cooey Alumni", "owner": false, "approximate_member_count": 300, "approximate_presence_count": 10},
	}
}

// Guild is the /guilds/{id} response for a guild in Guilds.
func Guild(id string) map[string]any {
	for _, g := range Guilds() {
		if g["id"] == id {
			return g
		}
	}
	return nil
}

// Channels is the /guilds/{id}/channels response for guild 1001.
func Channels() []map[string]any {
	return []map[string]any{
		{"id": "2000", "name": "Text Channels", "type": 4, "position": 0},
		{"id": "2001", "name": "🔮general", "type": 0, "position": 0, "parent_id": "2000"},
		{"id": "2002", "name": "📰news", "type": 5, "position": 1, "parent_id": "2000"},
		{"id": "2003", "name": "cmmc-general", "type": 0, "position": 2, "parent_id": "2000"},
		{"id": "2004", "name": "Voice", "type": 4, "position": 1},
		{"id": "2005", "name": "Lounge", "type": 2, "position": 0, "parent_id": "2004"},
		{"id": "2006", "name": "help-forum", "type": 15, "position": 0, "parent_id": "2010"},
		{"id": "2010", "name": "Support", "type": 4, "position": 2},
		{"id": "2007", "name": "random", "type": 0, "position": 5},
	}
}

// Threads returns the thread objects for a parent channel, split into
// active and archived, for the per-channel thread search endpoint.
func Threads(parentID string) (active, archived []map[string]any) {
	switch parentID {
	case "2001":
		active = []map[string]any{
			{"id": "3001", "name": "welcome thread", "type": 11, "parent_id": "2001", "thread_metadata": map[string]any{"archived": false}},
		}
		archived = []map[string]any{
			{"id": "3002", "name": "old planning", "type": 11, "parent_id": "2001", "thread_metadata": map[string]any{"archived": true, "archive_timestamp": "2026-01-02T03:04:05.000000+00:00"}},
		}
	case "2006":
		active = []map[string]any{
			{"id": "3003", "name": "How do I scope?", "type": 11, "parent_id": "2006", "thread_metadata": map[string]any{"archived": false}},
		}
	}
	return active, archived
}
