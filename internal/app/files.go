package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/netutil"
	"xin.dart.zchat/internal/share"
	"xin.dart.zchat/internal/transfer"
)

// startShare starts the HTTP server, showing a toast when asked.
func (a *app) startShare(announce bool) {
	if a.share == nil {
		return
	}
	if a.settings.ShareDir != "" {
		if info, err := os.Stat(a.settings.ShareDir); err != nil || !info.IsDir() {
			a.setToast(tr("共享目录不存在"))
			return
		}
	}
	if a.settings.UploadDir != "" {
		if info, err := os.Stat(a.settings.UploadDir); err != nil || !info.IsDir() {
			a.setToast(tr("上传目录不存在"))
			return
		}
	}
	a.share.SetDir(a.settings.ShareDir)
	a.share.SetUploadDir(a.settings.UploadDir)
	if err := a.share.Start(a.settings.SharePort); err != nil {
		a.shareExplicit = false
		a.setToast(tr("无法开启文件共享：") + err.Error())
		return
	}
	a.shareExplicit = true
	if announce {
		// A share the user started should come back on the next launch.
		a.settings.ShareOnStart = true
		_ = a.settings.save()
	}
	a.generateShareQR()
	if announce {
		a.setToast(tr("文件共享已开启"))
	}
	a.redraw()
}

func (a *app) stopShare() {
	if a.share != nil {
		a.share.Stop()
		a.share.SetUploadDir("")
	}
	a.shareExplicit = false
	a.settings.ShareOnStart = false
	_ = a.settings.save()
	a.redraw()
}

func (a *app) shareRunning() bool {
	return a.share != nil && a.share.Running()
}

func (a *app) shareUploadsActive() bool {
	return a.shareExplicit && a.share != nil && a.share.UploadDir() != ""
}

// ensureShareServer starts the HTTP attachment server when a chat file
// needs an HTTP fallback for peers that cannot use the TCP transfer, or
// for old messages that only carry a download URL. A missing share folder
// is not fatal: attachments still work while directory browsing stays off.
func (a *app) ensureShareServer() bool {
	if a.share == nil {
		return false
	}
	if a.share.Running() {
		return true
	}
	dir := a.settings.ShareDir
	if dir != "" {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = ""
		}
	}
	a.share.SetDir(dir)
	// Uploads belong to an explicitly started share; the fallback HTTP
	// server used for attachments stays read-only.
	a.share.SetUploadDir("")
	a.shareExplicit = false
	if err := a.share.Start(a.settings.SharePort); err != nil {
		return false
	}
	a.generateShareQR()
	return true
}

func (a *app) shareURL() string {
	if a.share == nil {
		return ""
	}
	return a.share.PrimaryURL()
}

func (a *app) generateShareQR() {
	url := a.shareURL()
	if url == "" {
		a.shareQR = nil
		a.qrForURL = ""
		return
	}
	if a.shareQR != nil && a.qrForURL == url {
		return
	}
	img, err := share.QRCode(url, 5)
	if err != nil {
		return
	}
	a.shareQR = ui.NewBitmap(img)
	a.qrForURL = url
}

// sendFiles offers files through the TCP transfer server and posts them to
// the active conversation. The HTTP attachment server also gets a copy so
// older peers, or peers whose TCP transfer is blocked, can still download.
func (a *app) sendFiles(paths []string) {
	c := a.currentConv()
	if c == nil {
		a.setToast(tr("先打开一个聊天"))
		return
	}
	if len(paths) == 0 {
		return
	}
	if a.transfer == nil || !a.transfer.Running() {
		a.startTransfer()
	}
	tcpReady := a.transfer != nil && a.transfer.Running()
	httpReady := a.ensureShareServer()
	if !tcpReady && !httpReady {
		a.setToast(tr("文件传输服务未启动，无法发送文件"))
		return
	}
	added := 0
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		var file *model.File
		if tcpReady {
			tr, err := a.transfer.Add(path)
			if err != nil {
				log.Println("zchat: cannot register TCP transfer:", err)
			} else {
				file = &model.File{
					Name:  tr.Name,
					Size:  tr.Size,
					MIME:  tr.MIME,
					Token: tr.Token,
					Path:  path,
				}
			}
		}
		if httpReady {
			st, err := a.share.AddTransfer(path)
			if err == nil {
				if file == nil {
					file = &model.File{
						Name: st.Name,
						Size: st.Size,
						MIME: st.MIME,
						Path: path,
					}
				} else {
					// Keep the TCP token as the fast path; the URL is
					// only a fallback for peers that cannot use it.
					file.URL = a.share.TransferURL(netutil.PrimaryIP(), st)
				}
			}
		}
		if file == nil {
			continue
		}
		kind := model.KindFile
		if strings.HasPrefix(file.MIME, "image/") {
			kind = model.KindImage
			file.Width, file.Height = imageSize(path)
			a.loadImage(path)
		}
		m := model.Message{
			ID:         model.NewID("m"),
			SenderID:   a.profile.ID,
			SenderName: a.profile.Name,
			Kind:       kind,
			File:       file,
			SentAt:     time.Now(),
			Status:     model.Sent,
		}
		if a.sendTokenMsg == nil {
			a.sendTokenMsg = map[string]string{}
		}
		if file.Token != "" {
			a.sendTokenMsg[file.Token] = m.ID
		}
		if tok := shareToken(file.URL); tok != "" {
			a.sendTokenMsg[tok] = m.ID
		}
		if a.sends == nil {
			a.sends = map[string]*sendState{}
		}
		a.sends[m.ID] = &sendState{Total: file.Size}
		a.addLocalMessage(c, &m)
		added++
	}
	if added == 0 {
		a.setToast(tr("没有可发送的文件"))
	}
}

func (a *app) sendClipboardImage() {
	if a.transfer == nil || !a.transfer.Running() {
		a.startTransfer()
	}
	if (a.transfer == nil || !a.transfer.Running()) && !a.ensureShareServer() {
		a.setToast(tr("文件传输服务未启动，无法发送图片"))
		return
	}
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		a.setToast(tr("剪贴板里没有图片"))
		return
	}
	c := a.currentConv()
	if c == nil {
		return
	}
	dir := filepath.Join(a.store.CacheDir(), "pasted")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, model.NewID("img")+".png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		a.setToast(tr("无法保存粘贴的图片"))
		return
	}
	a.sendFiles([]string{path})
}

func (a *app) sendShareLink() {
	c := a.currentConv()
	if c == nil {
		a.setToast(tr("先打开一个聊天"))
		return
	}
	url := a.shareURL()
	if url == "" {
		a.setToast(tr("文件共享还没有开启"))
		return
	}
	a.sendTextTo(c.ID, tr("我在 zchat 共享了一个目录：")+url)
}

// autoDownloadImage fetches an incoming image into the cache.
func (a *app) autoDownloadImage(convID, msgID string) {
	m := a.message(convID, msgID)
	if m == nil || m.File == nil || (m.File.URL == "" && m.File.Token == "") || m.File.Path != "" {
		return
	}
	ext := filepath.Ext(safeFileName(m.File.Name))
	if ext == "" {
		ext = ".img"
	}
	dest := filepath.Join(a.store.CacheDir(), "images", messageKey(m.ID)+ext)
	a.downloadTo(convID, msgID, dest, true)
}

// downloadMessage saves a received file to the downloads directory, or to
// path when the user chose one.
// queueActiveImages fetches the images of the open conversation that this
// device does not have yet, as after a restart.
func (a *app) queueActiveImages() {
	c := a.currentConv()
	if c == nil || !a.settings.AutoDownloadImages {
		return
	}
	for i := range a.msgs[c.ID] {
		m := &a.msgs[c.ID][i]
		if m.Kind == model.KindImage && m.File != nil && m.File.Path == "" && (m.File.URL != "" || m.File.Token != "") {
			a.autoDownloadImage(c.ID, m.ID)
		}
	}
}

// saveMessageAs asks where to keep a received attachment.
func (a *app) saveMessageAs(convID, msgID string) {
	m := a.message(convID, msgID)
	if m == nil || m.File == nil {
		return
	}
	name := safeFileName(m.File.Name)
	dir, _ := a.ensureDownloadDir()
	defaultPath := name
	if dir != "" {
		defaultPath = filepath.Join(dir, name)
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       tr("保存文件"),
			DefaultPath: defaultPath,
			ButtonLabel: tr("保存"),
		})
		if err != nil || path == "" {
			return
		}
		a.post(func() { a.downloadMessage(convID, msgID, path) })
	}()
}

// pickImages sends images chosen in a file dialog.
func (a *app) pickImages() {
	if a.currentConv() == nil {
		a.setToast(tr("先打开一个聊天"))
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("发送图片"),
			Multiple:    true,
			ButtonLabel: tr("发送"),
			Filters:     []mygo.FileFilter{{Name: tr("图片"), Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.sendFiles(paths) })
	}()
}

func (a *app) downloadMessage(convID, msgID string, path string) {
	if path == "" {
		dir, err := a.ensureDownloadDir()
		if err != nil {
			a.setToast(tr("无法创建下载目录：") + err.Error())
			return
		}
		path = a.suggestName(convID, msgID, dir)
	}
	a.downloadTo(convID, msgID, path, false)
}

func (a *app) suggestName(convID, msgID, dir string) string {
	m := a.message(convID, msgID)
	name := "zchat-file"
	if m != nil && m.File != nil {
		name = safeFileName(m.File.Name)
	}
	return uniquePath(filepath.Join(dir, name))
}

func (a *app) downloadTo(convID, msgID, dest string, imageOnly bool) {
	m := a.message(convID, msgID)
	if m == nil || m.File == nil {
		return
	}
	if m.File.Path != "" {
		if imageOnly {
			a.loadImage(m.File.Path)
		}
		return
	}
	if st := a.transfers[msgID]; st != nil && st.Working {
		return
	}
	st := &transferState{Total: m.File.Size, Working: true}
	a.transfers[msgID] = st
	a.redraw()
	if m.File.Token != "" {
		if peer := a.peers[m.SenderID]; peer != nil {
			if addr := peer.TransferAddr(); addr != "" {
				st.Transport = "TCP"
				log.Printf("zchat: downloading %s over TCP from %s", msgID, addr)
				a.downloadSocket(convID, msgID, m, dest, addr, imageOnly)
				return
			}
		}
	}
	if m.File.URL != "" {
		st.Transport = "HTTP"
		log.Printf("zchat: downloading %s over HTTP", msgID)
		a.downloadHTTP(convID, msgID, m.File.URL, dest, m.File.Size, imageOnly)
		return
	}
	delete(a.transfers, msgID)
	a.setToast(tr("无法下载：发送方离线，或不支持文件传输"))
	a.redraw()
}

// downloadSocket fetches an attachment over the sender's TCP transfer
// service. A partial file in the cache is resumed from its last byte, and
// the HTTP URL, when present, is tried if the socket transfer fails.
func (a *app) downloadSocket(convID, msgID string, m *model.Message, dest, addr string, imageOnly bool) {
	token, want, fallback := m.File.Token, m.File.Size, m.File.URL
	part := a.partPath(msgID)
	fail := func(done int64, err error) {
		if fallback != "" {
			log.Println("zchat: TCP transfer failed, trying HTTP fallback:", err)
			a.startHTTPFallback(convID, msgID, fallback, dest, want, imageOnly)
			return
		}
		a.finishDownload(convID, msgID, part, done, err, imageOnly)
	}
	go func() {
		if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
			fail(0, fmt.Errorf("%s", tf("无法创建缓存目录 %s：%v", filepath.Dir(part), err)))
			return
		}
		var offset int64
		if info, err := os.Stat(part); err == nil {
			offset = info.Size()
		}
		if want > 0 && offset > want {
			_ = os.Remove(part)
			offset = 0
		}
		if want > 0 && offset == want {
			a.completeTransfer(convID, msgID, part, dest, offset, imageOnly)
			return
		}
		if offset > 0 {
			a.post(func() {
				if st := a.transfers[msgID]; st != nil {
					st.Done = offset
				}
			})
		}
		f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			fail(offset, err)
			return
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			fail(offset, err)
			return
		}
		last := time.Now()
		done, err := transfer.Download(addr, token, offset, f, func(done, total int64) {
			if time.Since(last) < 120*time.Millisecond {
				return
			}
			last = time.Now()
			a.post(func() {
				if st := a.transfers[msgID]; st != nil {
					st.Done, st.Total = done, total
				}
			})
		})
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			fail(done, err)
			return
		}
		if want > 0 && done != want {
			_ = os.Remove(part)
			fail(done, fmt.Errorf(tr("文件大小不一致：收到 %d，预期 %d"), done, want))
			return
		}
		a.completeTransfer(convID, msgID, part, dest, done, imageOnly)
	}()
}

// startHTTPFallback switches a failed socket download to the sender's HTTP
// attachment URL on the UI thread, where the transfer state is owned.
func (a *app) startHTTPFallback(convID, msgID, url, dest string, want int64, imageOnly bool) {
	a.post(func() {
		st := a.transfers[msgID]
		if st == nil {
			st = &transferState{}
			a.transfers[msgID] = st
		}
		st.Working = true
		st.Done = 0
		st.Err = ""
		st.Total = want
		st.Transport = "HTTP"
		a.downloadHTTP(convID, msgID, url, dest, want, imageOnly)
	})
}

func (a *app) completeTransfer(convID, msgID, part, dest string, done int64, imageOnly bool) {
	final := uniquePath(dest)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		a.finishDownload(convID, msgID, part, done, fmt.Errorf("%s", tf("无法创建下载目录 %s：%v", filepath.Dir(final), err)), imageOnly)
		return
	}
	if err := moveFile(part, final); err != nil {
		a.finishDownload(convID, msgID, part, done, fmt.Errorf("%s", tf("无法保存到 %s：%v", final, err)), imageOnly)
		return
	}
	a.finishDownload(convID, msgID, final, done, nil, imageOnly)
}

// moveFile renames src to dst, copying when they are on different volumes
// (Windows returns an error for a cross-drive rename).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// forgetMessageFiles stops serving attachments that are being deleted.
func (a *app) forgetMessageFiles(msgs []model.Message) {
	tokens := make([]string, 0, len(msgs))
	for i := range msgs {
		f := msgs[i].File
		if f == nil {
			continue
		}
		delete(a.sends, msgs[i].ID)
		if f.Token != "" {
			tokens = append(tokens, f.Token)
			delete(a.sendTokenMsg, f.Token)
		}
		if tok := shareToken(f.URL); tok != "" {
			delete(a.sendTokenMsg, tok)
			if a.share != nil {
				a.share.RemoveTransfer(tok)
			}
		}
	}
	if a.transfer != nil {
		a.transfer.RemoveMany(tokens)
	}
}

// shareToken extracts the token from an HTTP attachment URL.
func shareToken(raw string) string {
	const marker = "/t/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return ""
	}
	rest := raw[i+len(marker):]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func (a *app) partPath(msgID string) string {
	return filepath.Join(a.store.CacheDir(), "transfers", messageKey(msgID)+".part")
}

func messageKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func (a *app) downloadHTTP(convID, msgID, url, dest string, want int64, imageOnly bool) {
	go func() {
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			a.finishDownload(convID, msgID, dest, 0, fmt.Errorf("%s", tf("无法创建下载目录 %s：%v", filepath.Dir(dest), err)), imageOnly)
			return
		}
		tmp := dest + ".part"
		var offset int64
		if info, err := os.Stat(tmp); err == nil {
			offset = info.Size()
		}
		if want > 0 && offset > want {
			_ = os.Remove(tmp)
			offset = 0
		}
		if want > 0 && offset == want {
			a.completeTransfer(convID, msgID, tmp, dest, offset, imageOnly)
			return
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			a.finishDownload(convID, msgID, dest, offset, err, imageOnly)
			return
		}
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			a.finishDownload(convID, msgID, dest, offset, err, imageOnly)
			return
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusPartialContent:
			// The server honored our resume request.
		case http.StatusOK:
			// Range was ignored; start over so the file is not duplicated.
			offset = 0
			_ = os.Remove(tmp)
		case http.StatusRequestedRangeNotSatisfiable:
			if want > 0 && offset == want {
				a.completeTransfer(convID, msgID, tmp, dest, offset, imageOnly)
				return
			}
			a.finishDownload(convID, msgID, dest, offset, fmt.Errorf("HTTP %s", resp.Status), imageOnly)
			return
		default:
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
				a.finishDownload(convID, msgID, dest, offset, errors.New(tr("文件不存在或已过期")), imageOnly)
				return
			}
			a.finishDownload(convID, msgID, dest, offset, fmt.Errorf("HTTP %s", resp.Status), imageOnly)
			return
		}
		total := want
		if total <= 0 && resp.ContentLength > 0 {
			total = offset + resp.ContentLength
		}
		a.post(func() {
			if st := a.transfers[msgID]; st != nil {
				st.Total = total
				st.Done = offset
			}
		})
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			a.finishDownload(convID, msgID, dest, offset, err, imageOnly)
			return
		}
		if offset == 0 {
			if err := f.Truncate(0); err != nil {
				f.Close()
				a.finishDownload(convID, msgID, dest, offset, err, imageOnly)
				return
			}
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			a.finishDownload(convID, msgID, dest, offset, err, imageOnly)
			return
		}
		done := offset
		buf := make([]byte, 256*1024)
		last := time.Now()
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					f.Close()
					a.finishDownload(convID, msgID, dest, done, werr, imageOnly)
					return
				}
				done += int64(n)
				if time.Since(last) > 150*time.Millisecond {
					last = time.Now()
					a.post(func() {
						if st := a.transfers[msgID]; st != nil {
							st.Done = done
						}
					})
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				f.Close()
				a.finishDownload(convID, msgID, dest, done, rerr, imageOnly)
				return
			}
		}
		if err := f.Close(); err != nil {
			a.finishDownload(convID, msgID, dest, done, err, imageOnly)
			return
		}
		if want > 0 && done != want {
			_ = os.Remove(tmp)
			a.finishDownload(convID, msgID, dest, done, fmt.Errorf(tr("文件大小不一致：收到 %d，预期 %d"), done, want), imageOnly)
			return
		}
		a.completeTransfer(convID, msgID, tmp, dest, done, imageOnly)
	}()
}

func (a *app) finishDownload(convID, msgID, dest string, done int64, err error, imageOnly bool) {
	a.post(func() {
		st := a.transfers[msgID]
		if st == nil {
			st = &transferState{}
			a.transfers[msgID] = st
		}
		st.Working = false
		st.Done = done
		if err != nil {
			st.Err = downloadErrorText(err)
			return
		}
		delete(a.transfers, msgID)
		if m := a.message(convID, msgID); m != nil && m.File != nil {
			m.File.Path = dest
			a.saveMessages(convID)
			if imageOnly || strings.HasPrefix(m.File.MIME, "image/") {
				a.loadImage(dest)
			}
			if !imageOnly {
				a.setToast(tr("已保存到 ") + dest)
			}
		}
	})
}

func (a *app) message(convID, msgID string) *model.Message {
	msgs := a.msgs[convID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			return &msgs[i]
		}
	}
	return nil
}

// loadImage decodes an image off the main thread and keeps it for the
// conversation view.
func (a *app) loadImage(path string) *ui.Bitmap {
	if path == "" {
		return nil
	}
	if bmp, ok := a.images[path]; ok {
		return bmp
	}
	if a.imageFail[path] || a.imageLoading[path] {
		return nil
	}
	a.imageLoading[path] = true
	go func() {
		b, err := os.ReadFile(path)
		var bmp *ui.Bitmap
		if err == nil {
			bmp, err = ui.DecodeBitmap(b)
		}
		// Run on the main thread without going through post: the first
		// view renders while NewWindow is still creating the window, so
		// a.win is nil and post would drop the result, leaving the path
		// stuck in imageLoading and the image missing until a restart.
		mygo.RunOnMain(func() {
			a.imageLoading[path] = false
			if err != nil || bmp == nil {
				a.imageFail[path] = true
				return
			}
			a.images[path] = bmp
			a.redraw()
		})
	}()
	return nil
}

// avatarBitmap decodes a peer's base64 PNG once.
func (a *app) avatarBitmap(encoded string) *ui.Bitmap {
	if encoded == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(encoded))
	key := hex.EncodeToString(sum[:])
	if bmp, ok := a.avatars[key]; ok {
		return bmp
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	a.avatars[key] = bmp
	return bmp
}

func (a *app) openPath(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		a.setToast(tr("文件不存在或已被移动"))
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		a.setToast(tr("无法打开文件：") + err.Error())
	}
}

func imageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 10000; i++ {
		p := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
	return path
}

func mimeOf(path string) string {
	m := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if m == "" {
		m = "application/octet-stream"
	}
	return m
}
