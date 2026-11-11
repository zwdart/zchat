// Package store keeps zchat's profile, conversations, known peers and
// messages in the user data directory. Chat history lives in a small SQLite
// database so that it stays ordered, searchable and easy to maintain, while
// the slowly changing app state remains a JSON file.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
)

const databaseFile = "zchat.db"

// State is what the app keeps between launches besides messages and its
// settings.
type State struct {
	Profile       model.Profile            `json:"profile"`
	Conversations []*model.Conversation    `json:"conversations"`
	Peers         map[string]*model.Peer   `json:"peers,omitempty"`
	Outbox        map[string][]proto.Frame `json:"outbox,omitempty"`
	SavedAt       time.Time                `json:"savedAt,omitempty"`
}

// Stats describes how much chat history the database holds.
type Stats struct {
	Conversations int       `json:"conversations"`
	Messages      int       `json:"messages"`
	Bytes         int64     `json:"bytes"`
	Oldest        time.Time `json:"oldest,omitempty"`
	Newest        time.Time `json:"newest,omitempty"`
	Path          string    `json:"path"`
}

// Store is a directory with the app state and a SQLite chat database.
type Store struct {
	dir string
	db  *sql.DB
}

// Open prepares dir, creating it if needed, and opens the chat database.
// Message files written by older versions are imported once.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("store: empty directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, databaseFile)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A single connection keeps PRAGMAs in effect and avoids writer
	// contention in this single-user app.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	s := &Store{dir: dir, db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS messages (
	seq          INTEGER PRIMARY KEY AUTOINCREMENT,
	id           TEXT    NOT NULL,
	conv_id      TEXT    NOT NULL,
	sender_id    TEXT    NOT NULL,
	sender_name  TEXT    NOT NULL DEFAULT '',
	kind         TEXT    NOT NULL DEFAULT 'text',
	text         TEXT    NOT NULL DEFAULT '',
	file_name    TEXT,
	file_size    INTEGER NOT NULL DEFAULT 0,
	file_mime    TEXT,
	file_url     TEXT,
	file_token   TEXT,
	file_path    TEXT,
	file_width   INTEGER NOT NULL DEFAULT 0,
	file_height  INTEGER NOT NULL DEFAULT 0,
	sent_at      INTEGER NOT NULL,
	edited_at    INTEGER,
	status       TEXT    NOT NULL DEFAULT '',
	delivered    INTEGER NOT NULL DEFAULT 0,
	read         INTEGER NOT NULL DEFAULT 0,
	recalled     INTEGER NOT NULL DEFAULT 0,
	UNIQUE(conv_id, id)
);
CREATE INDEX IF NOT EXISTS messages_conv_time ON messages(conv_id, sent_at, seq);
CREATE INDEX IF NOT EXISTS messages_conv_sender ON messages(conv_id, sender_id);
CREATE INDEX IF NOT EXISTS messages_time ON messages(sent_at);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`); err != nil {
		return err
	}
	// Databases created before socket transfers lack this column.
	_, _ = s.db.Exec(`ALTER TABLE messages ADD COLUMN file_token TEXT`)
	return s.importJSONMessages()
}

// importJSONMessages reads the per-conversation JSON files used before the
// database, then records that they were seen. The old files are left in
// place as a backup.
func (s *Store) importJSONMessages() error {
	var done string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'json_messages_imported'`).Scan(&done)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "messages"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		convID := strings.TrimSuffix(e.Name(), ".json")
		if err := validConvID(convID); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, "messages", e.Name()))
		if err != nil {
			continue
		}
		var msgs []model.Message
		if json.Unmarshal(data, &msgs) != nil {
			continue
		}
		if err := s.SaveMessages(convID, msgs); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('json_messages_imported', ?)`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Dir is where the store keeps its files.
func (s *Store) Dir() string { return s.dir }

// DBPath is the SQLite database holding chat history.
func (s *Store) DBPath() string { return filepath.Join(s.dir, databaseFile) }

// Close flushes and closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.db.Close()
}

// Load reads the state, returning an empty one when there is none yet.
func (s *Store) Load() (State, error) {
	var st State
	b, err := os.ReadFile(filepath.Join(s.dir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		st.Peers = map[string]*model.Peer{}
		st.Outbox = map[string][]proto.Frame{}
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if st.Peers == nil {
		st.Peers = map[string]*model.Peer{}
	}
	if st.Outbox == nil {
		st.Outbox = map[string][]proto.Frame{}
	}
	return st, nil
}

// Save writes the state atomically.
func (s *Store) Save(st State) error {
	st.SavedAt = time.Now()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "state.json"), b)
}

// LoadMessages reads a conversation's messages, oldest first.
func (s *Store) LoadMessages(convID string) ([]model.Message, error) {
	if err := validConvID(convID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT id, conv_id, sender_id, sender_name, kind, text,
       file_name, file_size, file_mime, file_url, file_token, file_path, file_width, file_height,
       sent_at, edited_at, status, delivered, read, recalled
FROM messages
WHERE conv_id = ?
ORDER BY sent_at, seq`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Message
	for rows.Next() {
		var (
			m          model.Message
			kind       string
			status     string
			fileName   sql.NullString
			fileSize   int64
			fileMIME   sql.NullString
			fileURL    sql.NullString
			fileToken  sql.NullString
			filePath   sql.NullString
			fileWidth  int64
			fileHeight int64
			sentAt     int64
			editedAt   sql.NullInt64
			delivered  bool
			read       bool
			recalled   bool
		)
		if err := rows.Scan(
			&m.ID, &m.ConvID, &m.SenderID, &m.SenderName, &kind, &m.Text,
			&fileName, &fileSize, &fileMIME, &fileURL, &fileToken, &filePath, &fileWidth, &fileHeight,
			&sentAt, &editedAt, &status, &delivered, &read, &recalled,
		); err != nil {
			return nil, err
		}
		m.Kind = model.MessageKind(kind)
		m.Status = model.Status(status)
		if sentAt != 0 {
			m.SentAt = time.UnixMilli(sentAt)
		}
		if editedAt.Valid {
			at := time.UnixMilli(editedAt.Int64)
			m.EditedAt = &at
		}
		m.Delivered, m.Read, m.Recalled = delivered, read, recalled
		if fileName.Valid || fileURL.Valid || fileToken.Valid || filePath.Valid {
			m.File = &model.File{
				Name:   fileName.String,
				Size:   fileSize,
				MIME:   fileMIME.String,
				URL:    fileURL.String,
				Token:  fileToken.String,
				Path:   filePath.String,
				Width:  int(fileWidth),
				Height: int(fileHeight),
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveMessages writes a conversation's messages, replacing what it held.
// The write is one transaction, so a crash never leaves half a chat.
func (s *Store) SaveMessages(convID string, msgs []model.Message) error {
	if err := validConvID(convID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM messages WHERE conv_id = ?`, convID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
INSERT OR REPLACE INTO messages(
	id, conv_id, sender_id, sender_name, kind, text,
	file_name, file_size, file_mime, file_url, file_token, file_path, file_width, file_height,
	sent_at, edited_at, status, delivered, read, recalled
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i := range msgs {
		m := msgs[i]
		kind := string(m.Kind)
		if kind == "" {
			kind = string(model.KindText)
		}
		var sentAt int64
		if !m.SentAt.IsZero() {
			sentAt = m.SentAt.UnixMilli()
		}
		var editedAt any
		if m.EditedAt != nil && !m.EditedAt.IsZero() {
			editedAt = m.EditedAt.UnixMilli()
		}
		var (
			fileName, fileMIME, fileURL, fileToken, filePath any
			fileSize, fileWidth, fileHeight                  int64
		)
		if m.File != nil {
			fileName, fileMIME, fileURL, fileToken, filePath = m.File.Name, m.File.MIME, m.File.URL, m.File.Token, m.File.Path
			fileSize, fileWidth, fileHeight = m.File.Size, int64(m.File.Width), int64(m.File.Height)
		}
		if _, err := stmt.Exec(
			m.ID, convID, m.SenderID, m.SenderName, kind, m.Text,
			fileName, fileSize, fileMIME, fileURL, fileToken, filePath, fileWidth, fileHeight,
			sentAt, editedAt, string(m.Status), boolInt(m.Delivered), boolInt(m.Read), boolInt(m.Recalled),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteConversation removes a conversation's messages.
func (s *Store) DeleteConversation(convID string) error {
	if err := validConvID(convID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM messages WHERE conv_id = ?`, convID)
	return err
}

// ClearMessages removes every conversation's messages, keeping the
// conversations themselves.
func (s *Store) ClearMessages() error {
	_, err := s.db.Exec(`DELETE FROM messages`)
	if err == nil {
		_, err = s.db.Exec(`DELETE FROM sqlite_sequence WHERE name = 'messages'`)
	}
	return err
}

// Stats counts the stored history and measures the database.
func (s *Store) Stats() Stats {
	st := Stats{Path: s.DBPath()}
	var oldest, newest int64
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT conv_id), COUNT(*), COALESCE(MIN(sent_at), 0), COALESCE(MAX(sent_at), 0) FROM messages`).
		Scan(&st.Conversations, &st.Messages, &oldest, &newest)
	if oldest > 0 {
		st.Oldest = time.UnixMilli(oldest)
	}
	if newest > 0 {
		st.Newest = time.UnixMilli(newest)
	}
	for _, path := range []string{s.DBPath(), s.DBPath() + "-wal", s.DBPath() + "-shm"} {
		if info, err := os.Stat(path); err == nil {
			st.Bytes += info.Size()
		}
	}
	return st
}

// Vacuum rewrites the database compactly after history was deleted.
func (s *Store) Vacuum() error {
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return err
	}
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return nil
}

// ExportMessages writes a conversation's messages as JSON, for a portable
// backup of what the database holds.
func (s *Store) ExportMessages(convID string, w io.Writer) error {
	msgs, err := s.LoadMessages(convID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(msgs)
}

// CacheDir is a directory for images and other downloads the app may lose.
func (s *Store) CacheDir() string {
	dir := filepath.Join(s.dir, "cache")
	os.MkdirAll(dir, 0o700)
	return dir
}

func validConvID(convID string) error {
	if convID == "" || convID == "." || convID == ".." ||
		filepath.Base(convID) != convID || strings.ContainsAny(convID, `/\`) {
		return errors.New("store: bad conversation id")
	}
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
