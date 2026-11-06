package share

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A token serves the file it stands for, and an unknown token does not.
func TestServeTransfer(t *testing.T) {
	dir := t.TempDir()
	name := "hello 世界.txt"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("zchat attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	tr, err := s.AddTransfer(path)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/"+tr.Token+"/"+url.PathEscape(name), nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "zchat attachment" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content type = %q", ct)
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/nope/"+url.PathEscape(name), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown token status = %d", rec.Code)
	}
}

// Asking for a transfer as a download marks it as an attachment.
func TestServeTransferDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	tr, err := s.AddTransfer(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/"+tr.Token+"/a.bin?download=1", nil))
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("content disposition = %q", got)
	}
}

// The listing shows ordinary files and leaves dot files out.
func TestServeDirListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	s.SetDir(dir)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(body, "visible.txt") {
		t.Fatal("listing does not show a file")
	}
	if strings.Contains(body, ".secret") {
		t.Fatal("listing shows a dot file")
	}
}

// A symlink inside the shared directory cannot point outside it.
func TestSafeJoinRefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := safeJoin(root, "link/secret.txt"); err == nil {
		t.Fatal("safeJoin allowed a symlink out of the shared directory")
	}
	rec := httptest.NewRecorder()
	s := New("")
	s.SetDir(root)
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/link/secret.txt", nil))
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("served a file outside the shared directory: %d %q", rec.Code, rec.Body.String())
	}
}

// A removed transfer stops being downloadable, and a persisted one comes
// back only while its file exists.
func TestTransferPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	persist := filepath.Join(dir, "transfers.json")
	first := New(persist)
	tr, err := first.AddTransfer(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	second := New(persist)
	if _, ok := second.TransferFor(tr.Token); ok {
		t.Fatal("a transfer survived its file")
	}
}
