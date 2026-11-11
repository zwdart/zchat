package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xin.dart.zchat/internal/model"
)

// State and messages survive a save and load, in order.
func TestSaveAndLoad(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	st := State{
		Profile: model.Profile{ID: "u_me", Name: "Me", Status: model.Online},
		Conversations: []*model.Conversation{
			{ID: "u_peer", Kind: model.KindDirect, PeerID: "u_peer", CreatedAt: at, UpdatedAt: at},
		},
		Peers: map[string]*model.Peer{"u_peer": {ID: "u_peer", Name: "Peer"}},
	}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	msgs := []model.Message{
		{ID: "m_1", ConvID: "u_peer", SenderID: "u_me", Kind: model.KindText, Text: "first", SentAt: at, Status: model.Sent},
		{ID: "m_2", ConvID: "u_peer", SenderID: "u_peer", Kind: model.KindFile, SentAt: at.Add(time.Minute), File: &model.File{Name: "a.pdf", Size: 42, Token: "t_abc"}},
	}
	if err := s.SaveMessages("u_peer", msgs); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.ID != "u_me" || len(got.Conversations) != 1 || got.Peers["u_peer"].Name != "Peer" {
		t.Fatalf("state = %+v", got)
	}
	loaded, err := s.LoadMessages("u_peer")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].Text != "first" || loaded[1].File == nil || loaded[1].File.Name != "a.pdf" || loaded[1].File.Token != "t_abc" {
		t.Fatalf("messages = %+v", loaded)
	}
	if !loaded[0].SentAt.Equal(at) {
		t.Fatalf("SentAt = %v, want %v", loaded[0].SentAt, at)
	}
}

// Loading a directory with no state yet gives an empty state, not an
// error.
func TestLoadEmpty(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.Profile.ID != "" || st.Peers == nil || st.Outbox == nil {
		t.Fatalf("state = %+v", st)
	}
	if msgs, err := s.LoadMessages("u_peer"); err != nil || msgs != nil {
		t.Fatalf("messages = %v, %v", msgs, err)
	}
}

// A conversation id cannot climb out of the data directory.
func TestBadConversationID(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`} {
		if err := s.SaveMessages(id, nil); err == nil {
			t.Errorf("SaveMessages(%q) accepted a bad id", id)
		}
		if _, err := s.LoadMessages(id); err == nil {
			t.Errorf("LoadMessages(%q) accepted a bad id", id)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "x")); err == nil {
		t.Fatal("a file escaped the store")
	}
}

// Deleting a conversation removes its rows, and deleting it again is
// harmless.
func TestDeleteConversation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SaveMessages("u_peer", []model.Message{{ID: "m_1", ConvID: "u_peer", Kind: model.KindText}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConversation("u_peer"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConversation("u_peer"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadMessages("u_peer"); err != nil || got != nil {
		t.Fatalf("messages = %v, %v", got, err)
	}
}

// JSON conversations written by older versions are imported once.
func TestImportsLegacyJSONMessages(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "messages")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.Local)
	data := []byte(`[{"id":"m_1","convId":"u_peer","senderId":"u_peer","kind":"text","text":"old","sentAt":"` + at.Format(time.RFC3339) + `"}]`)
	if err := os.WriteFile(filepath.Join(legacy, "u_peer.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := s.LoadMessages("u_peer")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Text != "old" {
		t.Fatalf("imported messages = %+v", msgs)
	}
	s.Close()

	// A second open keeps the rows and does not duplicate them.
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	msgs, err = s.LoadMessages("u_peer")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages after reopen = %+v", msgs)
	}
}

// Stats and maintenance reflect the database contents.
func TestStatsAndMaintenance(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().Add(-time.Hour)
	if err := s.SaveMessages("u_peer", []model.Message{
		{ID: "m_1", ConvID: "u_peer", Kind: model.KindText, Text: "one", SentAt: at},
		{ID: "m_2", ConvID: "u_peer", Kind: model.KindText, Text: "two", SentAt: at.Add(time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMessages("g_team", []model.Message{
		{ID: "m_3", ConvID: "g_team", Kind: model.KindText, Text: "three", SentAt: at.Add(2 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	st := s.Stats()
	if st.Conversations != 2 || st.Messages != 3 || st.Bytes == 0 {
		t.Fatalf("stats = %+v", st)
	}
	var out bytes.Buffer
	if err := s.ExportMessages("u_peer", &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"one"`)) {
		t.Fatalf("export = %s", out.String())
	}
	if err := s.ClearMessages(); err != nil {
		t.Fatal(err)
	}
	if st := s.Stats(); st.Messages != 0 || st.Conversations != 0 {
		t.Fatalf("stats after clear = %+v", st)
	}
	if err := s.Vacuum(); err != nil {
		t.Fatal(err)
	}
}
