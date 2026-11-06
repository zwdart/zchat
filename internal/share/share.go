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
	"sort"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

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
	port      int
	ln        net.Listener
	srv       *http.Server
	transfers map[string]Transfer
	persist   string
	stopped   chan struct{}
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
		http.Error(w, "文件已不存在", http.StatusNotFound)
		return
	}
	defer f.Close()
	if tr.MIME != "" {
		w.Header().Set("Content-Type", tr.MIME)
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(tr.Name)))
	}
	http.ServeContent(w, r, tr.Name, tr.Created, f)
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
	Title   string
	Rel     string
	Entries []dirEntry
	Parent  string
	Query   string
	Count   string
}

var listing = template.Must(template.New("dir").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
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
.name{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}.meta{color:var(--muted);font-size:12px;white-space:nowrap}
.empty{color:var(--muted);text-align:center;padding:48px 20px}
form{display:flex;gap:8px;margin:0 0 16px}input{flex:1;padding:10px 14px;border-radius:10px;border:1px solid var(--line);background:var(--card);color:var(--text);font:inherit}
button{padding:10px 16px;border:0;border-radius:10px;background:var(--accent);color:#fff;font:inherit;font-weight:600}
footer{color:var(--muted);font-size:12px;text-align:center;margin-top:26px}
</style></head><body><main>
<header><div class="logo">Z</div><div><h1>{{.Title}}</h1><div class="sub">{{.Count}}</div></div></header>
<form method="get"><input name="q" value="{{.Query}}" placeholder="在共享目录中搜索" aria-label="搜索"><button>搜索</button></form>
{{if .Parent}}<div class="card" style="margin-bottom:14px"><a class="row" href="{{.Parent}}"><span class="ico">↑</span><span class="name">返回上一级</span></a></div>{{end}}
<div class="card">
{{range .Entries}}<a class="row" href="{{.URL}}"><span class="ico">{{if .Dir}}📁{{else}}📄{{end}}</span><span class="name">{{.Name}}</span><span class="meta">{{.Size}}</span></a>{{else}}<div class="empty">这个目录是空的</div>{{end}}
</div>
<footer>由 zchat 在局域网共享 · 只读</footer>
</main></body></html>`))

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request) {
	root := s.Dir()
	if root == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>zchat</title><body style="font:16px system-ui;padding:48px"><h1>zchat 文件共享</h1><p>这个服务正在运行，但还没有选择共享目录。</p></body>`)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/")
	full, err := safeJoin(root, rel)
	if err != nil {
		http.Error(w, "无效的路径", http.StatusBadRequest)
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
		http.Error(w, "无法读取目录", http.StatusForbidden)
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
				row.Size = "文件夹"
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
	title := "共享目录"
	if strings.Trim(rel, "/") != "" {
		title = path.Base(strings.Trim(rel, "/"))
	}
	count := fmt.Sprintf("%d 个文件", len(rows))
	if q != "" {
		count = fmt.Sprintf("搜索 “%s” · %d 个结果", q, len(rows))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = listing.Execute(w, dirPage{Title: title, Rel: rel, Entries: rows, Parent: parent, Query: q, Count: count})
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
