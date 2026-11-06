package model

import (
	"strings"
	"testing"
	"time"
)

// Preview names the sort of thing a message carries, as the conversation
// row shows it.
func TestPreview(t *testing.T) {
	cases := []struct {
		name string
		m    Message
		want string
	}{
		{"text", Message{Kind: KindText, Text: "one\ntwo"}, "one two"},
		{"image", Message{Kind: KindImage, Text: "caption"}, "[图片] caption"},
		{"file", Message{Kind: KindFile, File: &File{Name: "a.pdf"}}, "[文件] a.pdf"},
		{"system", Message{Kind: KindSystem, Text: "joined"}, "joined"},
		{"recalled", Message{Kind: KindText, Text: "gone", Recalled: true}, "消息已撤回"},
	}
	for _, tc := range cases {
		if got := tc.m.Preview(); got != tc.want {
			t.Errorf("%s: Preview() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A long preview is cut by runes, not bytes, so Chinese text stays valid.
func TestPreviewTruncatesRunes(t *testing.T) {
	m := Message{Kind: KindText, Text: strings.Repeat("中", 100)}
	got := m.Preview()
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Preview() = %q", got)
	}
	if n := len([]rune(got)); n != 81 {
		t.Fatalf("preview has %d runes, want 81", n)
	}
}

// HumanSize gives the unit people expect at each boundary.
func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:           "0 B",
		1023:        "1023 B",
		1024:        "1.0 KB",
		1536:        "1.5 KB",
		1024 * 1024: "1.0 MB",
	}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

// DayLabel says today and yesterday in words and older days as dates.
func TestDayLabel(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, 11, 6, 15, 0, 0, 0, loc)
	cases := []struct {
		at   time.Time
		want string
	}{
		{now, "今天"},
		{now.AddDate(0, 0, -1), "昨天"},
		{time.Date(2026, 1, 2, 9, 0, 0, 0, loc), "1月2日"},
		{time.Date(2025, 12, 31, 9, 0, 0, 0, loc), "2025年12月31日"},
	}
	for _, tc := range cases {
		if got := DayLabel(tc.at, now); got != tc.want {
			t.Errorf("DayLabel(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

// NewID makes a prefixed, unique identifier.
func TestNewID(t *testing.T) {
	a, b := NewID("m"), NewID("m")
	if a == b {
		t.Fatal("two ids are equal")
	}
	if !strings.HasPrefix(a, "m_") {
		t.Fatalf("id = %q", a)
	}
}

// Presence labels and availability drive the status dots and whether a
// message is queued.
func TestPresence(t *testing.T) {
	if Online.Label() != "在线" || Offline.Label() != "离线" {
		t.Fatal("unexpected labels")
	}
	if Invisible.Available() || Offline.Available() {
		t.Fatal("invisible and offline peers are not available")
	}
	if !Away.Available() {
		t.Fatal("away is available")
	}
}
