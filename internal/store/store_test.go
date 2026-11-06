package store

import (
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
		{ID: "m_1", ConvID: "u_peer", SenderID: "u_me", Kind: model.KindText, Text: "first", SentAt: at},
		{ID: "m_2", ConvID: "u_peer", SenderID: "u_peer", Kind: model.KindText, Text: "second", SentAt: at.Add(time.Minute)},
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
	if len(loaded) != 2 || loaded[0].Text != "first" || loaded[1].Text != "second" {
		t.Fatalf("messages = %+v", loaded)
	}
}

// Loading a directory with no state yet gives an empty state, not an
// error.
func TestLoadEmpty(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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

// A conversation id cannot climb out of the messages directory.
func TestMessagePathRefusesEscape(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`} {
		if err := s.SaveMessages(id, nil); err == nil {
			t.Errorf("SaveMessages(%q) accepted a bad id", id)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "x.json")); err == nil {
		t.Fatal("a file escaped the store")
	}
}

// Deleting a conversation removes its file, and deleting it again is
// harmless.
func TestDeleteConversation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMessages("u_peer", []model.Message{{ID: "m_1"}}); err != nil {
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
