package transfer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A file can be fetched whole, then from an offset as a resumed download.
func TestDownloadAndResume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	content := bytes.Repeat([]byte("zchat-file-transfer\n"), 20000)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	if err := s.Start(0); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	e, err := s.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	addr := "127.0.0.1:" + itoa(s.Port())

	var full bytes.Buffer
	var lastDone, lastTotal int64
	n, err := Download(addr, e.Token, 0, &full, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(content)) || !bytes.Equal(full.Bytes(), content) {
		t.Fatalf("download = %d bytes, equal=%v", n, bytes.Equal(full.Bytes(), content))
	}
	if lastDone != n || lastTotal != n {
		t.Fatalf("progress = %d/%d, want %d", lastDone, lastTotal, n)
	}

	const offset = 12345
	var resumed bytes.Buffer
	n, err = Download(addr, e.Token, offset, &resumed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resumed.Bytes(), content[offset:]) {
		t.Fatal("resumed bytes do not match the rest of the file")
	}
	if n != int64(len(content)) {
		t.Fatalf("resumed total = %d, want %d", n, len(content))
	}
}

type progressUpdate struct {
	token string
	sent  int64
	total int64
	err   error
}

// The sender is told how far a receiver has read, including a final
// complete update.
func TestServerProgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.bin")
	content := bytes.Repeat([]byte("progress\n"), 50000)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	if err := s.Start(0); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	e, err := s.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var updates []progressUpdate
	done := make(chan struct{}, 1)
	s.SetProgress(func(token string, sent, total int64, err error) {
		mu.Lock()
		updates = append(updates, progressUpdate{token, sent, total, err})
		mu.Unlock()
		if err == nil && sent == total {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})
	var got bytes.Buffer
	if _, err := Download("127.0.0.1:"+itoa(s.Port()), e.Token, 0, &got, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sender did not report completion")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(updates) < 2 {
		t.Fatalf("updates = %+v", updates)
	}
	first, last := updates[0], updates[len(updates)-1]
	if first.token != e.Token || first.sent != 0 || first.total != int64(len(content)) || first.err != nil {
		t.Fatalf("first update = %+v", first)
	}
	if last.token != e.Token || last.sent != int64(len(content)) || last.total != int64(len(content)) || last.err != nil {
		t.Fatalf("last update = %+v", last)
	}
}

// A bad token is refused without reading a file.
func TestBadToken(t *testing.T) {
	s := New("")
	if err := s.Start(0); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	_, err := Download("127.0.0.1:"+itoa(s.Port()), "t_missing", 0, &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "存在") {
		t.Fatalf("error = %v", err)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// A token saved before a restart can still serve a resumed download.
func TestRegistrySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	content := []byte("hello over tcp")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(dir, "transfers.json")
	first := New(registry)
	e, err := first.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	first.Stop()

	second := New(registry)
	if err := second.Start(0); err != nil {
		t.Fatal(err)
	}
	defer second.Stop()
	var got bytes.Buffer
	if _, err := Download("127.0.0.1:"+itoa(second.Port()), e.Token, 0, &got, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), content) {
		t.Fatalf("got %q", got.Bytes())
	}
}
