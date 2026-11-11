package proto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"xin.dart.zchat/internal/model"
)

// A chat frame survives a line on the wire with its message intact.
func TestFrameRoundTrip(t *testing.T) {
	want := model.Message{
		ID:       "m_1",
		ConvID:   "u_peer",
		SenderID: "u_me",
		Kind:     model.KindText,
		Text:     "你好\nzchat",
	}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, NewFrame("u_me", TypeChat, Chat{Message: want})); err != nil {
		t.Fatal(err)
	}
	f, err := ReadFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != TypeChat || f.From != "u_me" {
		t.Fatalf("frame = %+v", f)
	}
	var got Chat
	if err := f.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Message.ID != want.ID || got.Message.Text != want.Text {
		t.Fatalf("message = %+v", got.Message)
	}
}

// A frame at end of input without its newline is still read.
func TestReadFrameAtEOF(t *testing.T) {
	line := `{"v":1,"type":"ping","from":"u_me"}`
	f, err := ReadFrame(bufio.NewReader(strings.NewReader(line)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != TypePing {
		t.Fatalf("type = %q", f.Type)
	}
}

// Frames from another protocol version are refused rather than guessed at.
func TestReadFrameRefusesOtherVersion(t *testing.T) {
	line := `{"v":2,"type":"ping","from":"u_me"}`
	if _, err := ReadFrame(bufio.NewReader(strings.NewReader(line))); err == nil {
		t.Fatal("expected an unsupported version error")
	}
}

// A frame with no type is not a frame.
func TestReadFrameNeedsType(t *testing.T) {
	line := `{"v":1,"from":"u_me"}`
	if _, err := ReadFrame(bufio.NewReader(strings.NewReader(line))); err == nil {
		t.Fatal("expected a missing type error")
	}
}

// Writing a frame over MaxFrame fails instead of allocating an unbounded
// line.
func TestWriteFrameRefusesOversize(t *testing.T) {
	f := Frame{V: Version, Type: TypeChat, From: "u_me", Data: json.RawMessage(`"` + strings.Repeat("a", MaxFrame) + `"`)}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, f); err == nil {
		t.Fatal("expected an oversize frame error")
	}
}

// Decoding a frame without data says so rather than giving a zero value.
func TestDecodeWithoutData(t *testing.T) {
	var v Chat
	if err := (Frame{}).Decode(&v); err == nil {
		t.Fatal("expected an error")
	}
}
