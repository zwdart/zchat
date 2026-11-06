// Package store keeps zchat's profile, conversations, known peers and
// messages in the user data directory, one JSON file per conversation so
// that a busy chat does not rewrite every other chat.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
)

// State is what the app keeps between launches besides messages and its
// settings.
type State struct {
	Profile       model.Profile            `json:"profile"`
	Conversations []*model.Conversation    `json:"conversations"`
	Peers         map[string]*model.Peer   `json:"peers,omitempty"`
	Outbox        map[string][]proto.Frame `json:"outbox,omitempty"`
	SavedAt       time.Time                `json:"savedAt,omitempty"`
}

// Store is a directory of JSON files.
type Store struct {
	dir     string
	msgsDir string
}

// Open prepares dir, creating it if needed.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("store: empty directory")
	}
	msgs := filepath.Join(dir, "messages")
	if err := os.MkdirAll(msgs, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, msgsDir: msgs}, nil
}

// Dir is where the store keeps its files.
func (s *Store) Dir() string { return s.dir }

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
	path, err := s.messagePath(convID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var msgs []model.Message
	if err := json.Unmarshal(b, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// SaveMessages writes a conversation's messages atomically.
func (s *Store) SaveMessages(convID string, msgs []model.Message) error {
	path, err := s.messagePath(convID)
	if err != nil {
		return err
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}

// DeleteConversation removes a conversation's messages.
func (s *Store) DeleteConversation(convID string) error {
	path, err := s.messagePath(convID)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ClearMessages removes every conversation's messages, keeping the
// conversations themselves.
func (s *Store) ClearMessages() error {
	entries, err := os.ReadDir(s.msgsDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if err := os.Remove(filepath.Join(s.msgsDir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// CacheDir is a directory for images and other downloads the app may lose.
func (s *Store) CacheDir() string {
	dir := filepath.Join(s.dir, "cache")
	os.MkdirAll(dir, 0o700)
	return dir
}

func (s *Store) messagePath(convID string) (string, error) {
	if convID == "" || convID == "." || convID == ".." ||
		filepath.Base(convID) != convID || strings.ContainsAny(convID, `/\`) {
		return "", errors.New("store: bad conversation id")
	}
	return filepath.Join(s.msgsDir, convID+".json"), nil
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
