package app

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
	"xin.dart.zchat/internal/share"
	"xin.dart.zchat/internal/transfer"
)

// A finished download may live on a different volume than the cache, so
// the copy fallback must preserve the bytes and remove the source.
func TestMoveFileFallback(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.part")
	dst := filepath.Join(dir, "target.bin")
	content := []byte("cross-volume copy")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("copy = %q, want %q", got, content)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist after copyFile: %v", err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after moveFile: %v", err)
	}
}

// The TCP cache can finish before the user's download folder exists; the
// final move must create it instead of failing.
func TestCompleteTransferCreatesDestinationDir(t *testing.T) {
	root := t.TempDir()
	part := filepath.Join(root, "cache", "transfers", "file.part")
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("downloaded mp3")
	if err := os.WriteFile(part, content, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "missing", "nested", "歌曲.mp3")
	a := &app{msgs: map[string][]model.Message{}, transfers: map[string]*transferState{}}
	a.completeTransfer("conv", "msg", part, dest, int64(len(content)), false)
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("destination not created: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content = %q, want %q", got, content)
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
}

// A sent file carries a TCP token for fast transfers and an HTTP URL for
// peers that cannot use the socket, and both actually serve the file.
func TestSendFilesAddsSocketAndHTTP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	content := []byte("zchat dual transport\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	sh := share.New("")
	if err := sh.Start(0); err != nil {
		t.Fatal(err)
	}
	defer sh.Stop()
	tf := transfer.New("")
	if err := tf.Start(0); err != nil {
		t.Fatal(err)
	}
	defer tf.Stop()

	conv := &model.Conversation{ID: "u_peer", Kind: model.KindDirect, PeerID: "u_peer"}
	a := &app{
		profile:  model.Profile{ID: "u_me", Name: "Me"},
		active:   conv.ID,
		convs:    map[string]*model.Conversation{conv.ID: conv},
		msgs:     map[string][]model.Message{},
		outbox:   map[string][]proto.Frame{},
		saveDue:  map[string]bool{},
		lastRead: map[string]time.Time{},
		settings: settings{DownloadDir: dir, SharePort: 0, TransferPort: 0},
		share:    sh,
		transfer: tf,
	}
	a.sendFiles([]string{path})

	msgs := a.msgs[conv.ID]
	if len(msgs) != 1 {
		t.Fatalf("messages = %d", len(msgs))
	}
	f := msgs[0].File
	if f == nil || f.Token == "" || f.URL == "" {
		t.Fatalf("file = %+v", f)
	}

	var socket bytes.Buffer
	if _, err := transfer.Download("127.0.0.1:"+strconv.Itoa(tf.Port()), f.Token, 0, &socket, nil); err != nil {
		t.Fatalf("socket download: %v", err)
	}
	if !bytes.Equal(socket.Bytes(), content) {
		t.Fatal("socket download content mismatch")
	}

	u, err := url.Parse(f.URL)
	if err != nil {
		t.Fatal(err)
	}
	u.Host = "127.0.0.1:" + strconv.Itoa(sh.Port())
	resp, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("HTTP download: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("HTTP status %d, body %q", resp.StatusCode, body)
	}
}
