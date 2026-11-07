package share

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xin.dart.zchat/internal/i18n"
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

// A transfer range lets an interrupted HTTP download resume instead of
// starting from zero.
func TestServeTransferRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "range.bin")
	content := "0123456789abcdef"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	tr, err := s.AddTransfer(path)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/t/"+tr.Token+"/range.bin", nil)
	req.Header.Set("Range", "bytes=5-")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != content[5:] {
		t.Fatalf("body = %q, want %q", got, content[5:])
	}
	if cr := rec.Header().Get("Content-Range"); cr == "" {
		t.Fatal("missing Content-Range")
	}
}

type transferProgress struct {
	token string
	sent  int64
	total int64
	err   error
}

// Attachment downloads report their start and completion, including the
// offset of a resumed range.
func TestServeTransferProgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.bin")
	content := strings.Repeat("0123456789", 1000)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New("")
	tr, err := s.AddTransfer(path)
	if err != nil {
		t.Fatal(err)
	}
	var updates []transferProgress
	s.SetProgress(func(token string, sent, total int64, err error) {
		updates = append(updates, transferProgress{token, sent, total, err})
	})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/"+tr.Token+"/progress.bin", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != len(content) {
		t.Fatalf("status = %d, body = %d", rec.Code, rec.Body.Len())
	}
	if len(updates) < 2 {
		t.Fatalf("updates = %+v", updates)
	}
	if first := updates[0]; first.token != tr.Token || first.sent != 0 || first.total != int64(len(content)) || first.err != nil {
		t.Fatalf("first update = %+v", first)
	}
	if last := updates[len(updates)-1]; last.sent != int64(len(content)) || last.total != int64(len(content)) || last.err != nil {
		t.Fatalf("last update = %+v", last)
	}

	updates = nil
	req := httptest.NewRequest(http.MethodGet, "/t/"+tr.Token+"/progress.bin", nil)
	req.Header.Set("Range", "bytes=1234-")
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d", rec.Code)
	}
	if len(updates) < 2 {
		t.Fatalf("range updates = %+v", updates)
	}
	if first := updates[0]; first.sent != 1234 || first.total != int64(len(content)) {
		t.Fatalf("range first update = %+v", first)
	}
	if last := updates[len(updates)-1]; last.sent != int64(len(content)) || last.err != nil {
		t.Fatalf("range last update = %+v", last)
	}
}

// The listing shows ordinary files and leaves dot files out.
func TestServeDirListing(t *testing.T) {
	dir := t.TempDir()
	visible := filepath.Join(dir, "visible.txt")
	if err := os.WriteFile(visible, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2024, 3, 4, 5, 6, 0, 0, time.Local)
	if err := os.Chtimes(visible, when, when); err != nil {
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
	if !strings.Contains(body, "1 B") || !strings.Contains(body, "2024-03-04 05:06") {
		t.Fatalf("listing is missing size or modified time: %q", body)
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

// The directory listing follows the app's selected language.
func TestDirPageEnglish(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := i18n.Current()
	i18n.Set(i18n.English)
	defer i18n.Set(old)
	s := New("")
	s.SetDir(dir)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "Search in shared folder") || !strings.Contains(body, "visible.txt") {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(body, "共享目录") {
		t.Fatalf("English page still has Chinese: %q", body)
	}
}

// Uploads go to the configured upload directory, keep only the file's
// base name, and return to the page the form was submitted from.
func TestUploadSavesFiles(t *testing.T) {
	root, upload := t.TempDir(), t.TempDir()
	s := New("")
	s.SetDir(root)
	s.SetUploadDir(upload)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("next", "/sub/"); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"../../evil.txt": "evil",
		"hello 世界.txt":   "hello",
	} {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(fw, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/sub/?uploaded=2" {
		t.Fatalf("location = %q", loc)
	}
	if got, err := os.ReadFile(filepath.Join(upload, "evil.txt")); err != nil || string(got) != "evil" {
		t.Fatalf("evil.txt = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(upload, "hello 世界.txt")); err != nil || string(got) != "hello" {
		t.Fatalf("hello file = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("an upload wrote outside the upload directory")
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	bodyText := rec.Body.String()
	if !strings.Contains(bodyText, `action="/upload"`) || !strings.Contains(bodyText, `enctype="multipart/form-data"`) {
		t.Fatalf("listing has no upload form: %q", bodyText)
	}
}

// Uploads with the same name do not overwrite each other.
func TestUploadNameCollision(t *testing.T) {
	root, upload := t.TempDir(), t.TempDir()
	s := New("")
	s.SetDir(root)
	s.SetUploadDir(upload)

	send := func(content string) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, err := mw.CreateFormFile("files", "note.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(fw, content); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/upload", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
		}
	}
	send("first")
	send("second")
	if got, err := os.ReadFile(filepath.Join(upload, "note.txt")); err != nil || string(got) != "first" {
		t.Fatalf("note.txt = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(upload, "note (2).txt")); err != nil || string(got) != "second" {
		t.Fatalf("note (2).txt = %q, %v", got, err)
	}
}

// Without an upload directory the listing is read-only and the upload
// endpoint refuses requests.
func TestUploadDisabled(t *testing.T) {
	s := New("")
	s.SetDir(t.TempDir())

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), `action="/upload"`) {
		t.Fatal("read-only listing has an upload form")
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/upload", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("upload status = %d, want 403", rec.Code)
	}
}

// Uploads work even when no shared folder is configured.
func TestUploadWithoutSharedDir(t *testing.T) {
	upload := t.TempDir()
	s := New("")
	s.SetUploadDir(upload)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `action="/upload"`) {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

// Filenames from browsers cannot escape the upload directory, and names
// Windows rejects are made safe there.
func TestUploadNameSanitization(t *testing.T) {
	cases := []struct {
		name, goos, want string
	}{
		{"../../evil.txt", "linux", "evil.txt"},
		{`C:\Users\me\photo.jpg`, "linux", "photo.jpg"},
		{"..", "linux", "upload"},
		{"CON.txt", "windows", "_CON.txt"},
		{`a:b?.txt`, "windows", "a_b_.txt"},
		{"trailing. ", "windows", "trailing"},
	}
	for _, c := range cases {
		if got := uploadNameFor(c.name, c.goos); got != c.want {
			t.Errorf("uploadNameFor(%q, %q) = %q, want %q", c.name, c.goos, got, c.want)
		}
	}
}

func TestCleanUploadNext(t *testing.T) {
	cases := map[string]string{
		"":               "/",
		"/sub/":          "/sub/",
		"//evil.example": "/",
		"/%2F%2Fevil":    "/",
		"https://evil":   "/",
		"/upload":        "/",
	}
	for in, want := range cases {
		if got := cleanUploadNext(in); got != want {
			t.Errorf("cleanUploadNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUploadNameLength(t *testing.T) {
	got := uploadNameFor(strings.Repeat("a", 300)+".txt", "linux")
	if len([]rune(got)) != 120 || !strings.HasSuffix(got, ".txt") {
		t.Fatalf("long upload name = %d runes, %q", len([]rune(got)), got)
	}
}
