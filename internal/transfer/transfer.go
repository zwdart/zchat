// Package transfer moves chat attachments directly over TCP. A receiver
// asks the sender's transfer server for a file by its unguessable token and
// the byte it has already saved, so a broken download can resume instead of
// starting again.
package transfer

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xin.dart.zchat/internal/i18n"
)

const (
	requestTimeout = 8 * time.Second
	dialTimeout    = 6 * time.Second
	maxLine        = 16 << 10
	bufferSize     = 256 << 10
)

// Entry is a file the sender offers.
type Entry struct {
	Token   string    `json:"token"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	MIME    string    `json:"mime,omitempty"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"modTime"`
}

type request struct {
	Token  string `json:"token"`
	Offset int64  `json:"offset"`
}

type response struct {
	OK    bool   `json:"ok"`
	Size  int64  `json:"size,omitempty"`
	Name  string `json:"name,omitempty"`
	MIME  string `json:"mime,omitempty"`
	Error string `json:"error,omitempty"`
}

// Server serves files a peer asked for. Tokens are kept in memory and, when
// a registry path was given, on disk so old messages keep working.
type Server struct {
	mu    sync.RWMutex
	files map[string]Entry

	ln       net.Listener
	port     int
	path     string
	stop     chan struct{}
	wg       sync.WaitGroup
	conns    map[net.Conn]struct{}
	closed   bool
	progress func(token string, sent, total int64, err error)
}

// New makes a transfer server. When registry is not empty, offered files
// are remembered there so links survive a restart.
func New(registry string) *Server {
	s := &Server{files: map[string]Entry{}, conns: map[net.Conn]struct{}{}, stop: make(chan struct{}), path: registry}
	if registry != "" {
		if b, err := os.ReadFile(registry); err == nil {
			var files map[string]Entry
			if json.Unmarshal(b, &files) == nil {
				for token, e := range files {
					if _, err := os.Stat(e.Path); err == nil {
						s.files[token] = e
					}
				}
			}
		}
	}
	return s
}

// Start listens on port, or on the first free port when port is zero.
func (s *Server) Start(port int) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("transfer: server is closed")
	}
	s.mu.Unlock()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil && port != 0 {
		// The configured port may be taken by another program; a free
		// port is better than no file transfer at all.
		ln, err = net.Listen("tcp", ":0")
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ln)
	}()
	return nil
}

// Stop closes the listener and all transfers in flight.
func (s *Server) Stop() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	ln := s.ln
	s.ln = nil
	close(s.stop)
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
	s.wg.Wait()
}

// Running reports whether the server is listening.
func (s *Server) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ln != nil
}

// Port is the TCP port peers reach.
func (s *Server) Port() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// SetProgress installs a callback for bytes sent to receivers. It is
// called from transfer goroutines, so callers must not block on the UI
// thread there.
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

// Add registers path and returns the entry carrying its token.
func (s *Server) Add(path string) (Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	if info.IsDir() {
		return Entry{}, errors.New("transfer: is a directory")
	}
	token, err := newToken()
	if err != nil {
		return Entry{}, err
	}
	name := filepath.Base(path)
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	e := Entry{
		Token:   token,
		Name:    name,
		Size:    info.Size(),
		MIME:    mimeType,
		Path:    path,
		ModTime: info.ModTime(),
	}
	s.mu.Lock()
	s.files[token] = e
	s.mu.Unlock()
	s.save()
	return e, nil
}

// Remove forgets a token, as when its message is deleted.
func (s *Server) Remove(token string) {
	s.RemoveMany([]string{token})
}

// RemoveMany forgets several tokens and writes the registry once.
func (s *Server) RemoveMany(tokens []string) {
	if len(tokens) == 0 {
		return
	}
	s.mu.Lock()
	changed := false
	for _, token := range tokens {
		if _, ok := s.files[token]; ok {
			delete(s.files, token)
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		s.save()
	}
}

func (s *Server) save() {
	if s.path == "" {
		return
	}
	s.mu.RLock()
	copy := make(map[string]Entry, len(s.files))
	for token, e := range s.files {
		copy[token] = e
	}
	s.mu.RUnlock()
	b, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
			}
			continue
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetNoDelay(true)
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
			}()
			s.serve(conn)
		}()
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	br := bufio.NewReaderSize(conn, 4096)
	line, err := readLine(br)
	if err != nil {
		return
	}
	var req request
	if json.Unmarshal(line, &req) != nil {
		_ = writeResponse(conn, response{Error: i18n.T("请求格式错误")})
		return
	}
	s.mu.RLock()
	e, ok := s.files[req.Token]
	s.mu.RUnlock()
	if !ok {
		_ = writeResponse(conn, response{Error: i18n.T("文件不存在或发送方已重启")})
		return
	}
	if _, err := os.Stat(e.Path); err != nil {
		_ = writeResponse(conn, response{Error: i18n.T("文件已被移动或删除")})
		return
	}
	if req.Offset < 0 || req.Offset > e.Size {
		_ = writeResponse(conn, response{Error: i18n.T("续传位置无效")})
		return
	}
	f, err := os.Open(e.Path)
	if err != nil {
		_ = writeResponse(conn, response{Error: i18n.T("无法读取文件")})
		return
	}
	defer f.Close()
	if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
		_ = writeResponse(conn, response{Error: i18n.T("无法定位文件")})
		return
	}
	if err := writeResponse(conn, response{OK: true, Size: e.Size, Name: e.Name, MIME: e.MIME}); err != nil {
		s.reportProgress(req.Token, req.Offset, e.Size, err)
		return
	}
	// A transfer may take minutes; do not cut it off with the request
	// deadline. The receiver closes its end when done.
	_ = conn.SetDeadline(time.Time{})
	s.reportProgress(req.Token, req.Offset, e.Size, nil)
	last := time.Now()
	pw := &progressWriter{w: conn, done: req.Offset, total: e.Size, report: func(done, total int64) {
		// The progress callback becomes UI work; keep it to a few
		// updates per second and always report the final byte.
		if done != total && time.Since(last) < 100*time.Millisecond {
			return
		}
		last = time.Now()
		s.reportProgress(req.Token, done, total, nil)
	}}
	buf := make([]byte, bufferSize)
	if _, err := io.CopyBuffer(pw, f, buf); err != nil {
		s.reportProgress(req.Token, pw.done, e.Size, err)
		return
	}
	s.reportProgress(req.Token, pw.done, e.Size, nil)
}

func writeResponse(w io.Writer, r response) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// Download asks addr for token, starting at offset, and writes the bytes to
// w. progress reports the total bytes saved so far.
func Download(addr, token string, offset int64, w io.Writer, progress func(done, total int64)) (int64, error) {
	if addr == "" || token == "" {
		return offset, errors.New("transfer: missing address or token")
	}
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return offset, err
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	req, err := json.Marshal(request{Token: token, Offset: offset})
	if err != nil {
		return offset, err
	}
	req = append(req, '\n')
	if _, err := conn.Write(req); err != nil {
		return offset, err
	}
	br := bufio.NewReaderSize(conn, bufferSize)
	line, err := readLine(br)
	if err != nil {
		return offset, err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return offset, err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = i18n.T("无法传输文件")
		}
		return offset, errors.New(resp.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	total := resp.Size
	if total <= 0 {
		total = offset
	}
	pw := &progressWriter{w: w, done: offset, total: total, report: progress}
	buf := make([]byte, bufferSize)
	if _, err := io.CopyBuffer(pw, br, buf); err != nil {
		return pw.done, err
	}
	if pw.done != total {
		return pw.done, fmt.Errorf("transfer: received %d of %d bytes", pw.done, total)
	}
	return pw.done, nil
}

type progressWriter struct {
	w      io.Writer
	done   int64
	total  int64
	report func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.report != nil {
		p.report(p.done, p.total)
	}
	return n, err
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, more, err := r.ReadLine()
		if len(out)+len(chunk) > maxLine {
			return nil, errors.New("transfer: line is too long")
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

func newToken() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "t_" + hex.EncodeToString(b[:]), nil
}
