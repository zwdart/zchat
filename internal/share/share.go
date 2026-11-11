// Package share serves a directory and zchat's file attachments over the
// LAN with HTTP. A directory listing makes browsing from a phone easy, and
// unguessable tokens serve the files messages point at.
package share

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/netutil"
)

// Transfer is a file a message points at. Its token is the unguessable
// part of the URL, and it stays usable for as long as the app keeps the
// file.
type Transfer struct {
	Token   string    `json:"token"`
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	MIME    string    `json:"mime"`
	Created time.Time `json:"created"`
}

// Server is the HTTP server. SetDir and Start are separate so the app may
// share a directory only when the user turns sharing on.
type Server struct {
	mu        sync.RWMutex
	dir       string
	uploadDir string
	port      int
	ln        net.Listener
	srv       *http.Server
	transfers map[string]Transfer
	persist   string
	stopped   chan struct{}
	progress  func(token string, sent, total int64, err error)
}

// New makes a server that keeps its transfer tokens in persist, so links
// in old messages keep working after a restart.
func New(persist string) *Server {
	s := &Server{transfers: map[string]Transfer{}, persist: persist}
	s.loadTransfers()
	return s
}

// SetDir changes the shared directory. Passing "" stops listing files but
// leaves attachment links working.
func (s *Server) SetDir(dir string) {
	s.mu.Lock()
	s.dir = dir
	s.mu.Unlock()
}

// Dir is the shared directory.
func (s *Server) Dir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dir
}

// SetUploadDir chooses where HTTP visitors may upload files. Passing ""
// turns uploads off without affecting directory browsing.
func (s *Server) SetUploadDir(dir string) {
	s.mu.Lock()
	s.uploadDir = dir
	s.mu.Unlock()
}

// UploadDir is the directory visitors upload into.
func (s *Server) UploadDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uploadDir
}

// SetProgress installs a callback for attachment bytes sent to receivers.
// It is called from HTTP handler goroutines, so callers must not block on
// the UI thread there.
func (s *Server) SetProgress(fn func(token string, sent, total int64, err error)) {
	s.mu.Lock()
	s.progress = fn
	s.mu.Unlock()
}

func (s *Server) reportProgress(token string, sent, total int64, err error) {
	s.mu.RLock()
	fn := s.progress
	s.mu.RUnlock()
	if fn != nil {
		fn(token, sent, total, err)
	}
}

// Start listens on port, or on any free port when port is 0. Calling Start
// while running stops the old listener first.
func (s *Server) Start(port int) error {
	s.Stop()
	if port < 0 || port > 65535 {
		return errors.New("share: bad port")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.ln = ln
	s.stopped = make(chan struct{})
	stopped := s.stopped
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv
	s.mu.Unlock()
	go func() {
		_ = srv.Serve(ln)
		close(stopped)
	}()
	return nil
}

// Stop stops listening. Transfer tokens stay in memory for the next Start.
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.srv
	ln := s.ln
	stopped := s.stopped
	s.srv, s.ln = nil, nil
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
	if ln != nil {
		_ = ln.Close()
	}
	if stopped != nil {
		select {
		case <-stopped:
		case <-time.After(time.Second):
		}
	}
}

// Running reports whether the server is listening.
func (s *Server) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ln != nil
}

// Port is the port it listens on, 0 when stopped.
func (s *Server) Port() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// URLs are the addresses other devices may open, one per IPv4 interface.
func (s *Server) URLs() []string {
	port := s.Port()
	if port == 0 {
		return nil
	}
	ips := netutil.InterfaceIPs()
	urls := make([]string, 0, len(ips))
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("http://%s:%d/", ip, port))
	}
	return urls
}

// PrimaryURL is the first LAN address, or empty when there is none.
func (s *Server) PrimaryURL() string {
	urls := s.URLs()
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

// AddTransfer makes path downloadable through a new token.
func (s *Server) AddTransfer(filePath string) (Transfer, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return Transfer{}, err
	}
	if info.IsDir() {
		return Transfer{}, errors.New("share: a directory cannot be an attachment")
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath)))
	if mimeType == "" {
		mimeType = sniffMIME(filePath)
	}
	tr := Transfer{
		Token:   token(),
		Path:    filePath,
		Name:    filepath.Base(filePath),
		Size:    info.Size(),
		MIME:    mimeType,
		Created: time.Now(),
	}
	s.mu.Lock()
	s.transfers[tr.Token] = tr
	s.mu.Unlock()
	s.saveTransfers()
	return tr, nil
}

// RemoveTransfer forgets a token.
func (s *Server) RemoveTransfer(tok string) {
	s.mu.Lock()
	delete(s.transfers, tok)
	s.mu.Unlock()
	s.saveTransfers()
}

// TransferFor returns the transfer of a token.
func (s *Server) TransferFor(tok string) (Transfer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tr, ok := s.transfers[tok]
	return tr, ok
}

// TransferURL is the URL of tr by which another device can fetch it.
func (s *Server) TransferURL(host string, tr Transfer) string {
	if host == "" {
		host = s.primaryHost()
	}
	return fmt.Sprintf("http://%s:%d/t/%s/%s", host, s.Port(), tr.Token, url.PathEscape(tr.Name))
}

// ServeHTTP routes directory listings, attachments and a small status page.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/t/") {
		s.serveTransfer(w, r)
		return
	}
	if r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "zchat\n")
		return
	}
	if r.URL.Path == "/upload" {
		s.serveUpload(w, r)
		return
	}
	s.serveDir(w, r)
}

func (s *Server) serveTransfer(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/t/")
	tok, _, _ := strings.Cut(rest, "/")
	tr, ok := s.TransferFor(tok)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(tr.Path)
	if err != nil {
		http.Error(w, i18n.T("文件已不存在"), http.StatusNotFound)
		return
	}
	defer f.Close()
	if tr.MIME != "" {
		w.Header().Set("Content-Type", tr.MIME)
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(tr.Name)))
	}
	var pw *httpProgressWriter
	out := w
	if r.Method == http.MethodGet {
		if start, ok := httpRangeStart(r.Header.Get("Range"), tr.Size); ok {
			pw = &httpProgressWriter{
				ResponseWriter: w,
				token:          tok,
				start:          start,
				total:          tr.Size,
				report:         s.reportProgress,
			}
			out = pw
		}
	}
	http.ServeContent(out, r, tr.Name, tr.Created, f)
	if pw == nil || !pw.started {
		return
	}
	done := pw.start + pw.sent
	if done >= pw.total {
		pw.report(pw.token, pw.total, pw.total, nil)
		return
	}
	err = r.Context().Err()
	if err == nil {
		err = errors.New("transfer interrupted")
	}
	pw.report(pw.token, done, pw.total, err)
}

// httpRangeStart extracts the start of a single open-ended byte range. The
// app resumes with "bytes=N-"; other range forms are left without progress.
func httpRangeStart(header string, size int64) (int64, bool) {
	if size == 0 || header == "" {
		return 0, true
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, false
	}
	startText, endText, ok := strings.Cut(spec, "-")
	if !ok || startText == "" || strings.TrimSpace(endText) != "" {
		return 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startText), 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, false
	}
	return start, true
}

// httpProgressWriter counts response body bytes and reports them through
// the server's progress callback, throttled to a few updates per second.
type httpProgressWriter struct {
	http.ResponseWriter
	token   string
	start   int64
	total   int64
	sent    int64
	status  int
	started bool
	last    time.Time
	report  func(token string, sent, total int64, err error)
}

func (w *httpProgressWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	if !w.started && (code == http.StatusOK || code == http.StatusPartialContent) {
		w.started = true
		w.last = time.Now()
		w.report(w.token, w.start, w.total, nil)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *httpProgressWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.sent += int64(n)
	if !w.started {
		return n, err
	}
	done := w.start + w.sent
	if done > w.total {
		done = w.total
	}
	if done < w.total && time.Since(w.last) < 100*time.Millisecond {
		return n, err
	}
	w.last = time.Now()
	w.report(w.token, done, w.total, nil)
	return n, err
}

type dirEntry struct {
	Name  string
	URL   string
	Dir   bool
	Size  string
	MIME  string
	Added string
}

type dirPage struct {
	Title             string
	Rel               string
	Entries           []dirEntry
	Parent            string
	Query             string
	Count             string
	Lang              string
	Search            string
	SearchPlaceholder string
	Back              string
	Empty             string
	Footer            string
	ShowSearch        bool
	Upload            bool
	UploadAction      string
	UploadHint        string
	UploadNotice      string
	Next              string
}

var listing = template.Must(template.New("dir").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · zchat</title>
<style>
:root{color-scheme:light dark;--bg:#f5f5f7;--card:#fff;--text:#1d1d1f;--muted:#6e6e73;--line:#00000012;--accent:#0a84ff}
@media (prefers-color-scheme:dark){:root{--bg:#141416;--card:#1c1c1f;--text:#f5f5f7;--muted:#a1a1aa;--line:#ffffff14;--accent:#0a84ff}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
main{max-width:860px;margin:0 auto;padding:32px 20px 64px}
header{display:flex;align-items:center;gap:12px;margin-bottom:18px}
.logo{width:38px;height:38px;border-radius:12px;background:var(--accent);color:#fff;display:grid;place-items:center;font-weight:700}
h1{font-size:20px;margin:0}.sub{color:var(--muted);font-size:13px;margin-top:2px}
.card{background:var(--card);border:1px solid var(--line);border-radius:16px;overflow:hidden}
.row{display:flex;align-items:center;gap:14px;padding:13px 16px;text-decoration:none;color:inherit;border-bottom:1px solid var(--line)}
.row:last-child{border-bottom:0}.row:hover{background:color-mix(in srgb,var(--accent) 8%,transparent)}
.ico{width:34px;height:34px;border-radius:9px;background:color-mix(in srgb,var(--accent) 14%,transparent);display:grid;place-items:center;flex:0 0 auto}
.name{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}.meta{color:var(--muted);font-size:12px;white-space:nowrap;text-align:right;line-height:1.4}
.empty{color:var(--muted);text-align:center;padding:48px 20px}
form.search{display:flex;gap:8px;margin:0 0 16px}form.search input{flex:1}
input{padding:10px 14px;border-radius:10px;border:1px solid var(--line);background:var(--card);color:var(--text);font:inherit}
form.upload{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;margin:0 0 16px;padding:12px 14px;border:1px solid var(--line);border-radius:16px;background:var(--card);align-items:center}
form.upload .uploadHint{grid-column:1/-1;color:var(--muted);font-size:12px;line-height:1.35}
form.upload input[type=file]{min-width:0;border:0;background:transparent;padding:7px 4px;color:var(--muted)}
.notice{padding:12px 16px;margin:0 0 16px;border-radius:12px;border:1px solid color-mix(in srgb,var(--accent) 24%,var(--line));background:color-mix(in srgb,var(--accent) 10%,var(--card));font-size:14px}
button{padding:10px 16px;border:0;border-radius:10px;background:var(--accent);color:#fff;font:inherit;font-weight:600;white-space:nowrap}
footer{color:var(--muted);font-size:12px;text-align:center;margin-top:26px}
</style></head><body><main>
<header><div class="logo">Z</div><div><h1>{{.Title}}</h1><div class="sub">{{.Count}}</div></div></header>
{{if .ShowSearch}}<form class="search" method="get"><input name="q" value="{{.Query}}" placeholder="{{.SearchPlaceholder}}" aria-label="{{.Search}}"><button>{{.Search}}</button></form>{{end}}
{{if .UploadNotice}}<div class="notice">{{.UploadNotice}}</div>{{end}}
{{if .Upload}}<form class="upload" method="post" action="/upload" enctype="multipart/form-data">
<input type="hidden" name="next" value="{{.Next}}">
<div class="uploadHint">{{.UploadHint}}</div>
<input type="file" name="files" multiple required aria-label="{{.UploadHint}}" title="{{.UploadHint}}">
<button type="submit">{{.UploadAction}}</button>
</form>{{end}}
{{if .Parent}}<div class="card" style="margin-bottom:14px"><a class="row" href="{{.Parent}}"><span class="ico">↑</span><span class="name">{{.Back}}</span></a></div>{{end}}
{{if or .Entries .Empty}}<div class="card">
{{range .Entries}}<a class="row" href="{{.URL}}"><span class="ico">{{if .Dir}}📁{{else}}📄{{end}}</span><span class="name">{{.Name}}</span><span class="meta">{{.Size}}{{if .Added}}<br>{{.Added}}{{end}}</span></a>{{else}}<div class="empty">{{.Empty}}</div>{{end}}
</div>{{end}}
<footer>{{.Footer}}</footer>
</main></body></html>`))

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request) {
	root := s.Dir()
	uploadDir := s.UploadDir()
	uploadEnabled := uploadDir != ""
	if uploadEnabled {
		if info, err := os.Stat(uploadDir); err != nil || !info.IsDir() {
			uploadEnabled = false
		}
	}
	if root == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if uploadEnabled {
			_ = listing.Execute(w, dirPage{
				Title:        i18n.T("zchat 文件共享"),
				Count:        i18n.T("共享目录未设置，但可以上传文件"),
				Lang:         pageLang(),
				Upload:       true,
				UploadAction: i18n.T("上传"),
				UploadHint:   i18n.T("选择文件后上传到共享者的上传目录"),
				Next:         "/",
				Footer:       i18n.T("由 zchat 在局域网共享"),
			})
			return
		}
		io.WriteString(w, `<!doctype html><html lang="`+pageLang()+`"><head><meta charset="utf-8"><title>zchat</title></head><body style="font:16px system-ui;padding:48px"><h1>`+template.HTMLEscapeString(i18n.T("zchat 文件共享"))+`</h1><p>`+template.HTMLEscapeString(i18n.T("这个服务正在运行，但还没有选择共享目录。"))+`</p></body></html>`)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/")
	full, err := safeJoin(root, rel)
	if err != nil {
		http.Error(w, i18n.T("无效的路径"), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !info.IsDir() {
		if r.URL.Query().Get("download") == "1" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(info.Name())))
		}
		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		return
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		http.Error(w, i18n.T("无法读取目录"), http.StatusForbidden)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var rows []dirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Name()), strings.ToLower(q)) {
			continue
		}
		child := path.Join(strings.Trim(rel, "/"), e.Name())
		href := "/" + escapePath(child)
		if e.IsDir() {
			href += "/"
		}
		ei, _ := e.Info()
		row := dirEntry{Name: e.Name(), URL: href, Dir: e.IsDir()}
		if ei != nil {
			row.Added = ei.ModTime().Format("2006-01-02 15:04")
			if e.IsDir() {
				row.Size = i18n.T("文件夹")
			} else {
				row.Size = humanSize(ei.Size())
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Dir != rows[j].Dir {
			return rows[i].Dir
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	parent := ""
	if strings.Trim(rel, "/") != "" {
		dir := path.Dir(strings.Trim(rel, "/"))
		if dir == "." {
			parent = "/"
		} else {
			parent = "/" + escapePath(dir) + "/"
		}
	}
	title := i18n.T("共享目录")
	if strings.Trim(rel, "/") != "" {
		title = path.Base(strings.Trim(rel, "/"))
	}
	count := i18n.TF("%d 个文件", len(rows))
	if q != "" {
		count = i18n.TF("搜索 “%s” · %d 个结果", q, len(rows))
	}
	notice := ""
	if n, err := strconv.Atoi(r.URL.Query().Get("uploaded")); err == nil && n > 0 {
		notice = i18n.TF("已上传 %d 个文件", n)
	}
	footer := i18n.T("由 zchat 在局域网共享 · 只读")
	if uploadEnabled {
		footer = i18n.T("由 zchat 在局域网共享 · 可上传")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = listing.Execute(w, dirPage{
		Title: title, Rel: rel, Entries: rows, Parent: parent, Query: q, Count: count,
		Lang: pageLang(), Search: i18n.T("搜索"), SearchPlaceholder: i18n.T("在共享目录中搜索"),
		Back: i18n.T("返回上一级"), Empty: i18n.T("这个目录是空的"),
		Footer: footer, ShowSearch: true, Upload: uploadEnabled,
		UploadAction: i18n.T("上传"), UploadHint: i18n.T("选择文件后上传到共享者的上传目录"),
		UploadNotice: notice, Next: r.URL.Path,
	})
}

func pageLang() string {
	if i18n.Current() == i18n.English {
		return "en"
	}
	return "zh-CN"
}

const maxUploadSize int64 = 8 << 30

// serveUpload accepts multipart form uploads from the browser and saves
// each file in the configured upload directory.
func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, i18n.T("请求格式错误"), http.StatusMethodNotAllowed)
		return
	}
	dir := s.UploadDir()
	if dir == "" {
		http.Error(w, i18n.T("未开启上传"), http.StatusForbidden)
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		http.Error(w, i18n.T("上传目录不存在"), http.StatusForbidden)
		return
	}
	if r.ContentLength > maxUploadSize {
		http.Error(w, i18n.T("文件过大"), http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, i18n.T("请求格式错误"), http.StatusBadRequest)
		return
	}
	next := "/"
	saved := make([]string, 0, 1)
	ok := false
	defer func() {
		if ok {
			return
		}
		for _, path := range saved {
			_ = os.Remove(path)
		}
	}()
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if uploadTooLarge(err) {
				http.Error(w, i18n.T("文件过大"), http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, i18n.T("请求格式错误"), http.StatusBadRequest)
			}
			return
		}
		name := part.FileName()
		if name == "" {
			if part.FormName() == "next" {
				b, _ := io.ReadAll(io.LimitReader(part, 2048))
				next = cleanUploadNext(string(b))
			}
			_ = part.Close()
			continue
		}
		f, path, err := createUpload(dir, name)
		if err != nil {
			_ = part.Close()
			http.Error(w, i18n.T("无法保存文件"), http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(f, part)
		closeErr := f.Close()
		_ = part.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if uploadTooLarge(copyErr) {
				http.Error(w, i18n.T("文件过大"), http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, i18n.T("无法保存文件"), http.StatusInternalServerError)
			}
			return
		}
		saved = append(saved, path)
	}
	if len(saved) == 0 {
		http.Error(w, i18n.T("没有选择文件"), http.StatusBadRequest)
		return
	}
	ok = true
	dest := &url.URL{Path: next, RawQuery: url.Values{"uploaded": {strconv.Itoa(len(saved))}}.Encode()}
	http.Redirect(w, r, dest.String(), http.StatusSeeOther)
}

func uploadTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func createUpload(dir, name string) (*os.File, string, error) {
	name = uploadName(name)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem, ext = name, ""
	}
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i+1, ext)
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, path, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("share: no free upload name")
}

func cleanUploadNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	p := u.Path
	if p == "" || p == "/upload" || strings.HasPrefix(p, "//") {
		return "/"
	}
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, p)
}

func uploadName(name string) string {
	return uploadNameFor(name, runtime.GOOS)
}

func uploadNameFor(name, goos string) string {
	// Treat both separators as separators so a browser's fake path cannot
	// become part of a server-side filename.
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == ".." || name == "/" || name == "" {
		return "upload"
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, name)
	if goos == "windows" {
		var b strings.Builder
		for _, r := range name {
			if strings.ContainsRune(`<>:"/\|?*`, r) {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
		name = strings.TrimRight(b.String(), " .")
		if name == "" {
			return "upload"
		}
		base := name
		if i := strings.IndexByte(base, '.'); i >= 0 {
			base = base[:i]
		}
		base = strings.TrimRight(base, " .")
		switch strings.ToUpper(base) {
		case "CON", "PRN", "AUX", "NUL",
			"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			name = "_" + name
		}
	}
	name = trimUploadName(name, 120)
	if goos == "windows" {
		name = strings.TrimRight(name, " .")
	}
	if name == "" || name == "." || name == ".." {
		return "upload"
	}
	return name
}

func trimUploadName(name string, max int) string {
	runes := []rune(name)
	if len(runes) <= max {
		return name
	}
	ext := path.Ext(name)
	extRunes := []rune(ext)
	if len(extRunes) >= max {
		extRunes = extRunes[:max/2]
	}
	room := max - len(extRunes)
	if room < 1 {
		room = 1
	}
	baseRunes := []rune(strings.TrimSuffix(name, ext))
	if len(baseRunes) > room {
		baseRunes = baseRunes[:room]
	}
	if len(baseRunes) == 0 {
		baseRunes = []rune("upload")
	}
	return string(baseRunes) + string(extRunes)
}

func safeJoin(root, rel string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel = filepath.FromSlash(rel)
	clean := filepath.Clean("/" + rel)
	full := filepath.Join(root, clean)
	rp, err := filepath.Rel(root, full)
	if err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", errors.New("share: path escapes the shared directory")
	}
	// A symlink inside the directory may point outside it. Resolve the
	// longest existing prefix and check the result too.
	if real, err := filepath.EvalSymlinks(full); err == nil {
		if realRoot, err := filepath.EvalSymlinks(root); err == nil {
			rr, err := filepath.Rel(realRoot, real)
			if err != nil || rr == ".." || strings.HasPrefix(rr, ".."+string(filepath.Separator)) {
				return "", errors.New("share: symlink escapes the shared directory")
			}
		}
	}
	return full, nil
}

func escapePath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func humanSize(n int64) string {
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

func (s *Server) primaryHost() string {
	ips := netutil.InterfaceIPs()
	if len(ips) == 0 {
		return "127.0.0.1"
	}
	return ips[0]
}

// QRCode draws text as a QR code, with a quiet zone, as an image.
func QRCode(text string, scale int) (image.Image, error) {
	if scale < 1 {
		scale = 4
	}
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	const quiet = 4
	n := code.Size + quiet*2
	img := image.NewRGBA(image.Rect(0, 0, n*scale, n*scale))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	black := &image.Uniform{color.Black}
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			r := image.Rect((x+quiet)*scale, (y+quiet)*scale, (x+quiet+1)*scale, (y+quiet+1)*scale)
			draw.Draw(img, r, black, image.Point{}, draw.Src)
		}
	}
	return img, nil
}

func token() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func sniffMIME(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	var b [512]byte
	n, _ := f.Read(b[:])
	return http.DetectContentType(b[:n])
}

func (s *Server) loadTransfers() {
	if s.persist == "" {
		return
	}
	b, err := os.ReadFile(s.persist)
	if err != nil {
		return
	}
	var list []Transfer
	if json.Unmarshal(b, &list) != nil {
		return
	}
	s.mu.Lock()
	for _, tr := range list {
		if info, err := os.Stat(tr.Path); err == nil && !info.IsDir() {
			s.transfers[tr.Token] = tr
		}
	}
	s.mu.Unlock()
}

func (s *Server) saveTransfers() {
	if s.persist == "" {
		return
	}
	s.mu.RLock()
	list := make([]Transfer, 0, len(s.transfers))
	for _, tr := range s.transfers {
		list = append(list, tr)
	}
	s.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	tmp := s.persist + ".part"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, s.persist)
	}
}
