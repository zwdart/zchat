package app

import (
	"testing"
	"time"

	"xin.dart.zchat/internal/model"
)

// The chat list shows day headings and keeps only the messages matching a
// search.
func TestRebuildRowsFiltersAndGroups(t *testing.T) {
	day1 := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	day2 := day1.AddDate(0, 0, 1)
	conv := &model.Conversation{ID: "u_peer", Kind: model.KindDirect, PeerID: "u_peer"}
	a := &app{
		profile: model.Profile{ID: "u_me", Name: "Me"},
		convs:   map[string]*model.Conversation{conv.ID: conv},
		msgs: map[string][]model.Message{
			conv.ID: {
				{ID: "m_1", Text: "hello", SentAt: day1},
				{ID: "m_2", Text: "other", SentAt: day2},
				{ID: "m_3", Kind: model.KindFile, File: &model.File{Name: "needle.pdf"}, SentAt: day2},
			},
		},
	}

	a.rebuildRows(conv)
	if len(a.rows) != 5 { // 2 headings and 3 messages
		t.Fatalf("rows = %+v", a.rows)
	}

	a.msgQuery = "NEEDLE"
	a.rebuildRows(conv)
	if len(a.rows) != 2 || a.rows[0].index != -1 || a.rows[1].index != 2 {
		t.Fatalf("filtered rows = %+v", a.rows)
	}

	a.msgQuery = ""
	a.rebuildRows(conv)
	if len(a.rows) != 5 {
		t.Fatalf("cleared rows = %+v", a.rows)
	}

	a.msgs[conv.ID] = append(a.msgs[conv.ID], model.Message{ID: "m_4", Text: "new", SentAt: day2})
	a.rebuildRows(conv)
	if len(a.rows) != 6 {
		t.Fatalf("appended rows = %+v", a.rows)
	}
}

// A direct conversation is keyed by the peer, and opening it again keeps
// the same conversation.
func TestEnsureDirect(t *testing.T) {
	peer := &model.Peer{ID: "u_peer", Name: "Peer"}
	a := &app{
		profile: model.Profile{ID: "u_me"},
		convs:   map[string]*model.Conversation{},
		msgs:    map[string][]model.Message{},
	}
	c := a.ensureDirect(peer)
	if c == nil || c.ID != peer.ID || c.Kind != model.KindDirect {
		t.Fatalf("conversation = %+v", c)
	}
	again := a.ensureDirect(peer)
	if again != c {
		t.Fatal("ensureDirect replaced an existing conversation")
	}
}
