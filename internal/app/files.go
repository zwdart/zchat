package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
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
)

// startShare starts the HTTP server, showing a toast when asked.
func (a *app) startShare(announce bool) {
	if a.share == nil {
		return
	}
	if a.settings.ShareDir != "" {
		if info, err := os.Stat(a.settings.ShareDir); err != nil || !info.IsDir() {
			a.setToast("共享目录不存在")
			return
		}
	}
	a.share.SetDir(a.settings.ShareDir)
	if err := a.share.Start(a.settings.SharePort); err != nil {
		a.setToast("无法开启文件共享：" + err.Error())
		return
	}
	if announce {
		// Sending a file starts the server too, but should not make it
		// share the directory on every launch.
		a.settings.ShareOnStart = true
		_ = a.settings.save()
	}
	a.generateShareQR()
	if announce {
		a.setToast("文件共享已开启")
	}
	a.redraw()
}

func (a *app) stopShare() {
	if a.share != nil {
		a.share.Stop()
	}
	a.settings.ShareOnStart = false
	_ = a.settings.save()
	a.redraw()
}

func (a *app) shareRunning() bool {
	return a.share != nil && a.share.Running()
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

// sendFiles shares files through the attachment server and posts them to
// the active conversation.
func (a *app) sendFiles(paths []string) {
	c := a.currentConv()
	if c == nil {
		a.setToast("先打开一个聊天")
		return
	}
	if len(paths) == 0 {
		return
	}
	if !a.shareRunning() {
		a.startShare(false)
	}
	if !a.shareRunning() {
		a.setToast("文件服务器未启动，无法发送文件")
		return
	}
	host := netutil.PrimaryIP()
	added := 0
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		tr, err := a.share.AddTransfer(path)
		if err != nil {
			continue
		}
		file := &model.File{
			Name: tr.Name,
			Size: tr.Size,
			MIME: tr.MIME,
			URL:  a.share.TransferURL(host, tr),
			Path: path,
		}
		kind := model.KindFile
		if strings.HasPrefix(tr.MIME, "image/") {
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
		a.addLocalMessage(c, &m)
		added++
	}
	if added == 0 {
		a.setToast("没有可发送的文件")
	}
}

func (a *app) sendClipboardImage() {
	if !a.shareRunning() {
		a.startShare(false)
	}
	if !a.shareRunning() {
		a.setToast("文件服务器未启动，无法发送图片")
		return
	}
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		a.setToast("剪贴板里没有图片")
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
		a.setToast("无法保存粘贴的图片")
		return
	}
	a.sendFiles([]string{path})
}

func (a *app) sendShareLink() {
	c := a.currentConv()
	if c == nil {
		a.setToast("先打开一个聊天")
		return
	}
	url := a.shareURL()
	if url == "" {
		a.setToast("文件共享还没有开启")
		return
	}
	a.sendTextTo(c.ID, "我在 zchat 共享了一个目录："+url)
}

// autoDownloadImage fetches an incoming image into the cache.
func (a *app) autoDownloadImage(convID, msgID string) {
	m := a.message(convID, msgID)
	if m == nil || m.File == nil || m.File.URL == "" || m.File.Path != "" {
		return
	}
	ext := filepath.Ext(m.File.Name)
	if ext == "" {
		ext = ".img"
	}
	dest := filepath.Join(a.store.CacheDir(), "images", m.ID+ext)
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
		if m.Kind == model.KindImage && m.File != nil && m.File.Path == "" && m.File.URL != "" {
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
	name := filepath.Base(m.File.Name)
	if name == "." || name == "" || name == string(filepath.Separator) {
		name = "zchat-file"
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "保存文件",
			DefaultPath: filepath.Join(a.settings.DownloadDir, name),
			ButtonLabel: "保存",
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
		a.setToast("先打开一个聊天")
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "发送图片",
			Multiple:    true,
			ButtonLabel: "发送",
			Filters:     []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.sendFiles(paths) })
	}()
}

func (a *app) downloadMessage(convID, msgID string, path string) {
	if path == "" {
		path = filepath.Join(a.settings.DownloadDir, a.suggestName(convID, msgID))
	}
	a.downloadTo(convID, msgID, path, false)
}

func (a *app) suggestName(convID, msgID string) string {
	m := a.message(convID, msgID)
	if m == nil || m.File == nil {
		return "zchat-file"
	}
	name := filepath.Base(m.File.Name)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "zchat-file"
	}
	return uniquePath(filepath.Join(a.settings.DownloadDir, name))
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
	if m.File.URL == "" {
		a.setToast("这个文件没有下载地址，发送方可能已经离线")
		return
	}
	if st := a.transfers[msgID]; st != nil && st.Working {
		return
	}
	a.transfers[msgID] = &transferState{Total: m.File.Size, Working: true}
	a.redraw()
	go func() {
		_ = os.MkdirAll(filepath.Dir(dest), 0o700)
		tmp := dest + ".part"
		resp, err := http.Get(m.File.URL)
		if err != nil {
			a.finishDownload(convID, msgID, dest, 0, err, imageOnly)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			a.finishDownload(convID, msgID, dest, 0, fmt.Errorf("HTTP %s", resp.Status), imageOnly)
			return
		}
		if resp.ContentLength > 0 {
			total := resp.ContentLength
			a.post(func() {
				if st := a.transfers[msgID]; st != nil {
					st.Total = total
				}
			})
		}
		f, err := os.Create(tmp)
		if err != nil {
			a.finishDownload(convID, msgID, dest, 0, err, imageOnly)
			return
		}
		var done int64
		buf := make([]byte, 64*1024)
		last := time.Now()
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					f.Close()
					os.Remove(tmp)
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
				os.Remove(tmp)
				a.finishDownload(convID, msgID, dest, done, rerr, imageOnly)
				return
			}
		}
		if err := f.Close(); err != nil {
			os.Remove(tmp)
			a.finishDownload(convID, msgID, dest, done, err, imageOnly)
			return
		}
		dest = uniquePath(dest)
		if err := os.Rename(tmp, dest); err != nil {
			os.Remove(tmp)
			a.finishDownload(convID, msgID, dest, done, err, imageOnly)
			return
		}
		a.finishDownload(convID, msgID, dest, done, nil, imageOnly)
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
			st.Err = err.Error()
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
				a.setToast("已保存到 " + dest)
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
		a.post(func() {
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
		a.setToast("文件不存在或已被移动")
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		a.setToast("无法打开文件：" + err.Error())
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
