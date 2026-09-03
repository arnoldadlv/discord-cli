package search

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, no C compiler needed

	"github.com/arnoldadlv/discord-cli/internal/export"
)

// Index is the disposable SQLite full-text index over the exports. The
// JSON exports stay the source of truth; the index is rebuilt from them.
type Index struct {
	Path string
	db   *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS files (
	path TEXT PRIMARY KEY,
	size INTEGER NOT NULL,
	mtime INTEGER NOT NULL,
	guild_id TEXT, guild_name TEXT,
	channel_id TEXT, channel_name TEXT,
	dialect TEXT,
	message_count INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY,
	file TEXT NOT NULL,
	message_id TEXT,
	author TEXT,
	timestamp TEXT,
	ts INTEGER,
	content TEXT
);
CREATE INDEX IF NOT EXISTS messages_file ON messages(file);
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	content, content='messages', content_rowid='id', tokenize='trigram'
);
`

// Open opens or creates the index file.
func Open(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=MEMORY", "PRAGMA synchronous=OFF", "PRAGMA temp_store=MEMORY"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating index schema: %w", err)
	}
	return &Index{Path: path, db: db}, nil
}

// Close closes the database.
func (ix *Index) Close() error { return ix.db.Close() }

// Exists reports whether an index file is on disk.
func Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// Remove deletes the index file and its journals.
func Remove(path string) error {
	for _, p := range []string{path, path + "-journal", path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// fileState is what the index remembers about one export file.
type fileState struct {
	Size  int64
	MTime int64
}

func (ix *Index) files() (map[string]fileState, error) {
	rows, err := ix.db.Query(`SELECT path, size, mtime FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]fileState{}
	for rows.Next() {
		var p string
		var s fileState
		if err := rows.Scan(&p, &s.Size, &s.MTime); err != nil {
			return nil, err
		}
		out[p] = s
	}
	return out, rows.Err()
}

// Stale counts the exports on disk that are missing from the index or
// indexed at a different size or modification time.
func (ix *Index) Stale(items []export.Item) (int, error) {
	known, err := ix.files()
	if err != nil {
		return 0, err
	}
	stale := 0
	for _, it := range items {
		s, ok := known[it.Path]
		if !ok || s.Size != it.Size || s.MTime != it.ModTime {
			stale++
		}
	}
	return stale, nil
}

// Update re-indexes every export whose size or modification time changed
// since it was indexed, and forgets files no longer on disk. It returns how
// many files were (re)indexed.
func (ix *Index) Update(items []export.Item, progress func(path string)) (int, error) {
	known, err := ix.files()
	if err != nil {
		return 0, err
	}
	onDisk := map[string]bool{}
	changed := 0
	for _, it := range items {
		onDisk[it.Path] = true
		if s, ok := known[it.Path]; ok && s.Size == it.Size && s.MTime == it.ModTime {
			continue
		}
		if progress != nil {
			progress(it.Path)
		}
		if err := ix.indexFile(it); err != nil {
			return changed, err
		}
		changed++
	}
	for p := range known {
		if !onDisk[p] {
			if err := ix.forget(p); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

func (ix *Index) forget(path string) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO messages_fts(messages_fts, rowid, content) SELECT 'delete', id, content FROM messages WHERE file = ?`, path); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE file = ?`, path); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM files WHERE path = ?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

func (ix *Index) indexFile(it export.Item) error {
	h, msgs, err := export.Read(it.Path)
	if err != nil {
		return err
	}
	results := Messages(h, msgs)
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO messages_fts(messages_fts, rowid, content) SELECT 'delete', id, content FROM messages WHERE file = ?`, it.Path); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE file = ?`, it.Path); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO files(path, size, mtime, guild_id, guild_name, channel_id, channel_name, dialect, message_count) VALUES (?,?,?,?,?,?,?,?,?)`,
		it.Path, it.Size, it.ModTime, h.Guild.ID, h.Guild.Name, h.Channel.ID, h.Channel.Name, string(h.Dialect), len(results)); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT INTO messages(file, message_id, author, timestamp, ts, content) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	fts, err := tx.Prepare(`INSERT INTO messages_fts(rowid, content) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer fts.Close()
	for _, r := range results {
		var ts int64
		if !r.time.IsZero() {
			ts = r.time.UnixNano()
		}
		res, err := ins.Exec(it.Path, r.MessageID, r.Author, r.Timestamp, ts, r.Content)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := fts.Exec(id, r.Content); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Stats reports what the index holds.
func (ix *Index) Stats() (files int, messages int, err error) {
	if err := ix.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(message_count), 0) FROM files`).Scan(&files, &messages); err != nil {
		return 0, 0, err
	}
	return files, messages, nil
}

// Search returns the matches within the given files, newest first, in the
// same order Scan produces. Terms of three or more characters go through
// the trigram index; every candidate is re-checked with the same matcher
// the scan uses, so the two never disagree.
func (ix *Index) Search(paths []string, q Query) ([]Result, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var (
		where []string
		args  []any
	)
	var long, short []string
	for _, t := range q.Terms {
		if len([]rune(t)) >= 3 {
			long = append(long, t)
		} else {
			short = append(short, t)
		}
	}
	if len(q.Terms) > 0 && len(short) == 0 {
		parts := make([]string, len(long))
		for i, t := range long {
			parts[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		}
		where = append(where, `m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)`)
		args = append(args, strings.Join(parts, " OR "))
	}
	if q.Author != "" {
		where = append(where, `instr(lower(m.author), ?) > 0`)
		args = append(args, q.Author)
	}
	if !q.After.IsZero() {
		where = append(where, `m.ts >= ?`)
		args = append(args, q.After.UnixNano())
	}
	if !q.Before.IsZero() {
		where = append(where, `m.ts <= ?`)
		args = append(args, q.Before.UnixNano())
	}
	var out []Result
	const chunk = 500
	for start := 0; start < len(paths); start += chunk {
		end := min(start+chunk, len(paths))
		ph := strings.TrimSuffix(strings.Repeat("?,", end-start), ",")
		conds := append([]string{`m.file IN (` + ph + `)`}, where...)
		cargs := make([]any, 0, len(args)+end-start)
		for _, p := range paths[start:end] {
			cargs = append(cargs, p)
		}
		cargs = append(cargs, args...)
		rows, err := ix.db.Query(`SELECT f.guild_id, f.guild_name, f.channel_id, f.channel_name, m.message_id, m.author, m.timestamp, m.content, m.file
			FROM messages m JOIN files f ON f.path = m.file WHERE `+strings.Join(conds, " AND "), cargs...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r Result
			if err := rows.Scan(&r.GuildID, &r.GuildName, &r.ChannelID, &r.ChannelName, &r.MessageID, &r.Author, &r.Timestamp, &r.Content, &r.File); err != nil {
				rows.Close()
				return nil, err
			}
			r.time = ParseTime(r.Timestamp)
			if q.Matches(r) {
				out = append(out, r)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	Sort(out)
	return out, nil
}
