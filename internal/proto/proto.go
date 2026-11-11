// Package proto is zchat's line protocol: one JSON frame per line over
// TCP, with binary file bodies handed over HTTP instead.
package proto

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"xin.dart.zchat/internal/model"
)

// Version is the protocol version peers must share.
const Version = 1

// MaxFrame bounds a frame, so a peer cannot make the app read forever.
const MaxFrame = 2 << 20

// Frame types.
const (
	TypeHello    = "hello"
	TypePresence = "presence"
	TypeBye      = "bye"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeChat     = "chat"
	TypeAck      = "ack"
	TypeRead     = "read"
	TypeTyping   = "typing"
	TypeRecall   = "recall"
	TypeGroup    = "group"
)

// Frame is the envelope of every message. Data holds the payload of Type.
type Frame struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	From string          `json:"from"`
	To   string          `json:"to,omitempty"`
	ID   string          `json:"id,omitempty"`
	Time int64           `json:"time,omitempty"` // Unix milliseconds
	Data json.RawMessage `json:"data,omitempty"`
}

// Hello is the first frame either side sends on a TCP connection.
type Hello struct {
	Peer model.Peer `json:"peer"`
}

// PresenceUpdate tells a peer about a change of name, signature, avatar or
// status. Peers send it to everyone connected when it changes.
type PresenceUpdate struct {
	Name      string         `json:"name"`
	Signature string         `json:"signature,omitempty"`
	Avatar    string         `json:"avatar,omitempty"`
	Status    model.Presence `json:"status"`
}

// Chat carries one message to one peer. A group message is sent once to
// each member; ConvID says which group it belongs to.
type Chat struct {
	Message model.Message `json:"message"`
}

// Ack confirms that a message arrived.
type Ack struct {
	MessageID string `json:"messageId"`
	Time      int64  `json:"time"`
}

// Read marks every message in a conversation up to Time as read.
type Read struct {
	ConvID string `json:"convId"`
	Time   int64  `json:"time"`
}

// Typing says whether a peer is typing in a conversation.
type Typing struct {
	ConvID string `json:"convId"`
	On     bool   `json:"on"`
}

// Recall asks receivers to mark a message withdrawn.
type Recall struct {
	MessageID string `json:"messageId"`
}

// Group actions.
const (
	GroupInvite  = "invite"
	GroupUpdate  = "update"
	GroupLeave   = "leave"
	GroupDisband = "disband"
)

// GroupAction shares a group chat's description or says a member left.
type GroupAction struct {
	Action string      `json:"action"`
	Group  model.Group `json:"group"`
	Intro  string      `json:"intro,omitempty"`
}

// Payload encodes a value as a frame's data.
func Payload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// Decode reads a frame's data into v.
func (f Frame) Decode(v any) error {
	if len(f.Data) == 0 {
		return errors.New("proto: frame has no data")
	}
	return json.Unmarshal(f.Data, v)
}

// WriteFrame writes f as one line.
func WriteFrame(w io.Writer, f Frame) error {
	if f.V == 0 {
		f.V = Version
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(b) > MaxFrame {
		return fmt.Errorf("proto: frame of %d bytes is too large", len(b))
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// ReadFrame reads one frame, refusing one over MaxFrame.
func ReadFrame(r *bufio.Reader) (Frame, error) {
	line, err := readLine(r)
	if err != nil {
		return Frame{}, err
	}
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		return Frame{}, fmt.Errorf("proto: %w", err)
	}
	if f.V != Version {
		return Frame{}, fmt.Errorf("proto: unsupported version %d", f.V)
	}
	if f.Type == "" {
		return Frame{}, errors.New("proto: frame has no type")
	}
	return f, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, more, err := r.ReadLine()
		if len(out)+len(chunk) > MaxFrame {
			return nil, errors.New("proto: frame is too large")
		}
		out = append(out, chunk...)
		if err != nil {
			if err == io.EOF && len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if !more {
			return out, nil
		}
	}
}

// NewFrame makes a frame from the local peer.
func NewFrame(from, typ string, data any) Frame {
	return Frame{V: Version, Type: typ, From: from, Data: Payload(data)}
}
