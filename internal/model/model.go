// Package model holds the data zchat keeps and shares: peers, chats and
// messages. It has no dependencies on the UI or the network, so the same
// values travel through the wire protocol and the store.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Presence is what a peer says of themselves. Invisible peers stay
// connected but announce themselves as offline.
type Presence string

const (
	Online    Presence = "online"
	Away      Presence = "away"
	Busy      Presence = "busy"
	DND       Presence = "dnd"
	Invisible Presence = "invisible"
	Offline   Presence = "offline"
)

// PresenceOptions are the statuses the app offers, in order.
var PresenceOptions = []struct {
	Value Presence
	Label string
}{
	{Online, "在线"},
	{Away, "离开"},
	{Busy, "忙碌"},
	{DND, "请勿打扰"},
	{Invisible, "隐身"},
}

// Label names a presence in Chinese.
func (p Presence) Label() string {
	for _, o := range PresenceOptions {
		if o.Value == p {
			return o.Label
		}
	}
	if p == Offline {
		return "离线"
	}
	return "在线"
}

// Available reports whether messages may be delivered to a peer with p.
func (p Presence) Available() bool {
	return p != Offline && p != Invisible
}

// Peer is another zchat app on the LAN, as far as this one knows it.
type Peer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Signature string    `json:"signature,omitempty"`
	Avatar    string    `json:"avatar,omitempty"` // PNG, base64
	Status    Presence  `json:"status"`
	IP        string    `json:"ip,omitempty"`
	Port      int       `json:"port,omitempty"`
	Version   string    `json:"version,omitempty"`
	Platform  string    `json:"platform,omitempty"`
	LastSeen  time.Time `json:"lastSeen"`
	Connected bool      `json:"-"`

	// Alias is a local remark that replaces Name in the UI.
	Alias string `json:"alias,omitempty"`
	// Manual peers were added by address and stay in the list while
	// offline.
	Manual bool `json:"manual,omitempty"`
}

// Title is what the app calls the peer.
func (p Peer) Title() string {
	if strings.TrimSpace(p.Alias) != "" {
		return p.Alias
	}
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	if p.IP != "" {
		return p.IP
	}
	return "未知设备"
}

// Addr is where to reach the peer's TCP server.
func (p Peer) Addr() string {
	if p.Port == 0 {
		return p.IP
	}
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

// Profile is the user's own identity, kept in the data directory and
// announced on the LAN.
type Profile struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Signature string   `json:"signature,omitempty"`
	Avatar    string   `json:"avatar,omitempty"` // PNG, base64
	Status    Presence `json:"status"`
}

// Peer returns the profile as a peer, at the address the node listens on.
func (p Profile) Peer(ip string, port int) Peer {
	return Peer{
		ID:        p.ID,
		Name:      p.Name,
		Signature: p.Signature,
		Avatar:    p.Avatar,
		Status:    p.Status,
		IP:        ip,
		Port:      port,
		LastSeen:  time.Now(),
		Connected: true,
	}
}

// Kind is a conversation's kind.
type Kind string

const (
	KindDirect Kind = "direct"
	KindGroup  Kind = "group"
)

// Conversation is a direct chat with a peer or a group chat.
type Conversation struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	PeerID    string    `json:"peerId,omitempty"`
	Title     string    `json:"title,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	Members   []string  `json:"members,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Unread    int       `json:"unread"`
	Pinned    bool      `json:"pinned,omitempty"`
	Muted     bool      `json:"muted,omitempty"`
	Draft     string    `json:"draft,omitempty"`
}

// Group is the shared description of a group chat, as it travels between
// its members.
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Owner     string    `json:"owner"`
	Members   []string  `json:"members"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GroupConversation turns a group description into a conversation.
func (g Group) Conversation() *Conversation {
	return &Conversation{
		ID:        g.ID,
		Kind:      KindGroup,
		Title:     g.Name,
		Owner:     g.Owner,
		Members:   append([]string(nil), g.Members...),
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

// HasMember reports whether id is in the group.
func (g Group) HasMember(id string) bool {
	for _, m := range g.Members {
		if m == id {
			return true
		}
	}
	return false
}

// MessageKind is what a message carries.
type MessageKind string

const (
	KindText   MessageKind = "text"
	KindImage  MessageKind = "image"
	KindFile   MessageKind = "file"
	KindSystem MessageKind = "system"
)

// Status is how far an outgoing message got.
type Status string

const (
	Sending   Status = "sending"
	Sent      Status = "sent"
	Delivered Status = "delivered"
	Read      Status = "read"
	Failed    Status = "failed"
)

// File is an attachment. URL serves it from the sender's LAN HTTP server;
// Path is where this device keeps its copy, if any.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	MIME   string `json:"mime,omitempty"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Message is one row of a conversation.
type Message struct {
	ID         string      `json:"id"`
	ConvID     string      `json:"convId"`
	SenderID   string      `json:"senderId"`
	SenderName string      `json:"senderName,omitempty"`
	Kind       MessageKind `json:"kind"`
	Text       string      `json:"text,omitempty"`
	File       *File       `json:"file,omitempty"`
	SentAt     time.Time   `json:"sentAt"`
	EditedAt   *time.Time  `json:"editedAt,omitempty"`
	Status     Status      `json:"status,omitempty"`
	Delivered  bool        `json:"delivered,omitempty"`
	Read       bool        `json:"read,omitempty"`
	Recalled   bool        `json:"recalled,omitempty"`
	// Mine is not persisted; it says whether the profile sent the
	// message, which the view needs and every receiver knows.
	Mine bool `json:"-"`
}

// Preview is the one-line text a conversation row shows for a message.
func (m Message) Preview() string {
	if m.Recalled {
		return "消息已撤回"
	}
	switch m.Kind {
	case KindImage:
		if m.Text != "" {
			return "[图片] " + m.Text
		}
		return "[图片]"
	case KindFile:
		if m.File != nil {
			return "[文件] " + m.File.Name
		}
		return "[文件]"
	case KindSystem:
		return m.Text
	default:
		s := strings.ReplaceAll(m.Text, "\n", " ")
		if len([]rune(s)) > 80 {
			s = string([]rune(s)[:80]) + "…"
		}
		return s
	}
}

// NewID returns a random identifier with a prefix, such as "m" for a
// message.
func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	if prefix == "" {
		return hex.EncodeToString(b[:])
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// HumanSize formats a byte count as people read it.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// ShortTime formats a message's time for a chat row.
func ShortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}

// DayLabel names the day of t relative to now: 今天, 昨天 or a date.
func DayLabel(t, now time.Time) string {
	y, m, d := t.Date()
	ny, nm, nd := now.Date()
	today := time.Date(ny, nm, nd, 0, 0, 0, 0, now.Location())
	day := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	switch {
	case day.Equal(today):
		return "今天"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "昨天"
	case y == ny:
		return fmt.Sprintf("%d月%d日", m, d)
	default:
		return fmt.Sprintf("%d年%d月%d日", y, m, d)
	}
}

// SameDay reports whether a and b fall on the same calendar day.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
