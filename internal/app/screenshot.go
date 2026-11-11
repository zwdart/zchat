package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/annotate"
	"xin.dart.zchat/internal/capture"
	"xin.dart.zchat/internal/model"
)

// Screenshot phases.
const (
	shotSelecting = iota
	shotAnnotating
)

// Drag modes for the overlay.
const (
	shotDragNone = iota
	shotDragSelect
	shotDragMove
	shotDragResize
	shotDragDraw
	shotDragErase
)

// shotSession is one screenshot in progress: a full-display image, the
// selection, the annotation document and the overlay window.
type shotSession struct {
	win     *mygo.Window
	display capture.Display
	caps    capture.Capability
	frame   capture.Frame

	base      *ui.Bitmap
	pixelated *ui.Bitmap
	annLayer  *ui.Bitmap
	doc       *annotate.Doc
	annDirty  bool

	scale      float32
	winW, winH float32
	wasVisible bool
	startedAt  time.Time

	phase int
	sel   image.Rectangle

	drag      int
	handle    int
	dragStart image.Point
	dragSel   image.Rectangle

	tool        annotate.Tool
	ink         color.RGBA
	width       float32
	filled      bool
	points      []annotate.Point
	draft       *annotate.Op
	textEditing bool
	text        string
	textAt      annotate.Point
	nextNumber  int

	cursor      image.Point
	cursorOn    bool
	hoverTool   string
	hoverHandle int
	windows     []capture.WindowInfo
	windowRects []image.Rectangle
	hoverWindow int
	clickWindow int
	clickCount  int
	toolbar     shotToolbar
	notice      string
}

// openScreenshotChooser asks whether the next screenshot includes the chat
// window, bringing zchat forward so the dialog is visible when the action
// came from the tray or the settings window.
func (a *app) openScreenshotChooser() {
	if !a.settings.ScreenshotEnabled {
		return
	}
	if a.win != nil && !a.win.IsDestroyed() && (!a.win.IsVisible() || !a.win.IsFocused()) {
		a.showMain()
	}
	a.shotChooserOpen = true
	a.redraw()
}

// setScreenshotIncludeChat changes whether the composer's screenshot button
// starts by including the chat window, and keeps the choice for the next
// launch.
func (a *app) setScreenshotIncludeChat(include bool) {
	if a.settings.ScreenshotIncludeChat == include {
		return
	}
	a.settings.ScreenshotIncludeChat = include
	_ = a.settings.save()
	a.redraw()
}

// openScreenshotSettings opens the settings window scrolled to the
// screenshot group, where the chat window default lives.
func (a *app) openScreenshotSettings() {
	a.shotSettingsFocus = true
	a.openSettings()
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		mygo.App.Focus()
	}
	a.redraw()
}

// startScreenshot captures the display under the pointer. Unless includeChat
// is set zchat and its pins hide first, so they are left out of the shot.
func (a *app) startScreenshot(includeChat bool) {
	mygo.RunOnMain(func() {
		if a.quitting || a.shot != nil || a.shotPending || !a.settings.ScreenshotEnabled {
			return
		}
		a.shotPending = true
		wasVisible := a.win != nil && !a.win.IsDestroyed() && a.win.IsVisible()
		restoreMain := wasVisible && !includeChat
		if includeChat {
			a.shotPins = nil
		} else {
			a.shotPins = a.hidePinsForCapture()
		}
		// Hide after the frame that handled the click, not inside it: a
		// paint, focus change or activation that follows the click would
		// otherwise be free to put the window back before the capture.
		time.AfterFunc(30*time.Millisecond, func() {
			mygo.RunOnMain(func() {
				if a.quitting {
					return
				}
				if wasVisible && !includeChat && a.win != nil && !a.win.IsDestroyed() {
					// Cloak and exclude the window from capture before it
					// hides, so no frame of it reaches the shot even if the
					// compositor is slow to drop it.
					a.shotReveal = concealWindowForCapture(a.win)
					a.win.Hide()
				}
				// Let the compositor paint the window (and the chooser) out
				// of the way before reading the screen.
				time.AfterFunc(180*time.Millisecond, func() {
					mygo.RunOnMain(func() {
						if a.quitting {
							return
						}
						a.captureScreenshot(restoreMain)
					})
				})
			})
		})
	})
}

func (a *app) captureScreenshot(wasVisible bool) {
	a.shotPending = false
	reveal := a.shotReveal
	a.shotReveal = nil
	if reveal != nil && a.win != nil && !a.win.IsDestroyed() && a.win.IsVisible() {
		// Something showed the window again after it was hidden for the
		// shot. Hide it once more; the cloak kept it out of sight anyway.
		a.win.Hide()
	}
	if a.shot != nil {
		if reveal != nil {
			reveal()
		}
		return
	}
	cursor := mygo.Screen.CursorScreenPoint()
	pt := image.Pt(cursor.X, cursor.Y)
	includeCursor := a.settings.ScreenshotCursor
	go func() {
		if reveal != nil {
			defer reveal()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		b := capture.Default()
		perm := b.Permission()
		if perm == capture.PermissionNotDetermined {
			perm = b.RequestPermission(ctx)
		}
		if perm == capture.PermissionDenied {
			a.post(func() {
				a.shotFailed(wasVisible, tr("没有屏幕录制权限。请在系统设置中允许 zchat 录制屏幕后重试。"))
			})
			return
		}
		displays, err := b.Displays()
		if err != nil || len(displays) == 0 {
			a.post(func() {
				msg := tr("无法读取显示器信息")
				if err != nil {
					msg += "：" + err.Error()
				}
				a.shotFailed(wasVisible, msg)
			})
			return
		}
		target, _ := capture.DisplayAt(displays, pt)
		frames, err := b.Capture(ctx, capture.Options{
			DisplayID:     target.ID,
			IncludeCursor: includeCursor && b.Capabilities().PreCaptureOverlay,
		})
		if err != nil || len(frames) == 0 {
			a.post(func() {
				msg := tr("截图失败")
				if err != nil {
					msg += "：" + err.Error()
				}
				a.shotFailed(wasVisible, msg)
			})
			return
		}
		frame := frames[0]
		caps := b.Capabilities()
		var wins []capture.WindowInfo
		if caps.WindowDetection && b != nil {
			if list, werr := b.Windows(ctx, target.ID); werr == nil {
				wins = list
			}
		}
		a.post(func() { a.showShotOverlay(frame, target, caps, wasVisible, wins) })
	}()
}

func (a *app) shotFailed(wasVisible bool, msg string) {
	a.restorePins(a.shotPins)
	a.shotPins = nil
	if wasVisible && a.win != nil {
		a.showMain()
	}
	mygo.Dialog.Error("zchat", msg)
}

// showShotOverlay puts the captured display in a frameless window and lets
// the user select and annotate it.
func (a *app) showShotOverlay(frame capture.Frame, d capture.Display, caps capture.Capability, wasVisible bool, wins []capture.WindowInfo) {
	if a.shot != nil || frame.Image == nil {
		return
	}
	frame.Calibrate()
	bounds := frame.Bounds
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		bounds = d.Bounds
	}
	frame.Bounds = bounds
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		return
	}
	scale := frame.Scale
	if scale <= 0 {
		scale = 1
	}
	s := &shotSession{
		display:     d,
		caps:        caps,
		frame:       frame,
		scale:       float32(scale),
		winW:        float32(bounds.Dx()),
		winH:        float32(bounds.Dy()),
		wasVisible:  wasVisible,
		startedAt:   time.Now(),
		phase:       shotSelecting,
		tool:        annotate.ToolRect,
		ink:         annotate.DefaultColor,
		width:       3,
		nextNumber:  1,
		handle:      -1,
		hoverWindow: -1,
		clickWindow: -1,
	}
	s.doc = annotate.New(frame.Image.Bounds().Dx(), frame.Image.Bounds().Dy())
	s.base = ui.NewBitmap(frame.Image)
	s.pixelated = ui.NewBitmap(frame.Image).Pixelated()
	if caps.WindowDetection {
		s.windows = localizeWindows(capture.FilterWindows(wins, frame.Bounds, "zchat"), frame.Bounds)
		s.windowRects = make([]image.Rectangle, len(s.windows))
		for i, w := range s.windows {
			s.windowRects[i] = s.windowRect(w)
		}
	}
	if !caps.PreCaptureOverlay {
		s.notice = tr("已截取屏幕，请拖选区域后编辑")
	}
	a.shot = s
	// A Linux desktop panel (the top bar with the clock, the app menu and the
	// tray) lives in a layer above ordinary always-on-top windows, and the
	// window manager keeps normal windows inside the work area, below it.
	// Such an overlay would hide the top of the screenshot and would not
	// receive the pointer there, so the menu and status bars could not be
	// selected. A fullscreen window is composited over the panel (which also
	// hides itself), so the overlay can show and edit the whole display. It
	// is left resizable because size hints could otherwise keep it from
	// growing to the display when the compositor makes it fullscreen.
	linuxFullscreen := runtime.GOOS == "linux"
	s.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "zchat",
		X:                 bounds.Min.X,
		Y:                 bounds.Min.Y,
		Width:             bounds.Dx(),
		Height:            bounds.Dy(),
		Frameless:         true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableShadow:     true,
		Panel:             runtime.GOOS == "darwin",
		FullScreen:        linuxFullscreen,
		DisableResize:     !linuxFullscreen,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		DisableClose:      true,
		BackgroundColor:   "#000000",
		Content:           ui.View(a.shotView),
	})
	s.win.SetVisibleOnAllWorkspaces(true)
	s.win.OnClosed(func() {
		if a.shot == s {
			a.shot = nil
			s.release()
			if wasVisible && a.win != nil {
				a.showMain()
			}
		}
	})
	s.win.Show()
	s.win.Focus()
	mygo.App.Focus()
	s.win.Invalidate()
}

// release frees the large bitmaps of a finished session.
func (s *shotSession) release() {
	if s == nil {
		return
	}
	if s.base != nil {
		s.base.ReleasePixels()
		s.base = nil
	}
	if s.pixelated != nil {
		s.pixelated.ReleasePixels()
		s.pixelated = nil
	}
	if s.annLayer != nil {
		s.annLayer.ReleasePixels()
		s.annLayer = nil
	}
}

// shotClose closes the overlay and optionally brings zchat back.
func (a *app) shotClose(restore bool) {
	s := a.shot
	if s == nil {
		return
	}
	a.shot = nil
	if s.win != nil && !s.win.IsDestroyed() {
		s.win.Close()
	}
	s.release()
	a.restorePins(a.shotPins)
	a.shotPins = nil
	if restore && s.wasVisible && a.win != nil {
		a.showMain()
	}
}

// resultImage flattens the selection and its annotations.
func (s *shotSession) resultImage() *image.RGBA {
	crop := s.sel.Intersect(image.Rect(0, 0, s.doc.Width, s.doc.Height))
	if crop.Empty() {
		crop = image.Rect(0, 0, s.doc.Width, s.doc.Height)
	}
	return annotate.Render(s.frame.Image, s.doc, crop)
}

// resultPNG encodes the flattened image for the clipboard or a file.
func (s *shotSession) resultPNG(level png.CompressionLevel) ([]byte, error) {
	img := s.resultImage()
	if img == nil {
		return nil, errors.New("empty image")
	}
	return capture.EncodePNG(img, level)
}

func (a *app) shotCopy(s *shotSession) {
	data, err := s.resultPNG(png.BestSpeed)
	if err != nil {
		a.setToast(tr("无法生成截图"))
		return
	}
	if err := mygo.Clipboard.WriteImage(data); err != nil {
		a.setToast(tr("无法复制截图：") + err.Error())
		return
	}
	a.shotClose(true)
	a.setToast(tr("截图已复制到剪贴板"))
}

// shotSave writes the screenshot to the configured folder without asking.
func (a *app) shotSave(s *shotSession) {
	img := s.resultImage()
	dir := a.screenshotDir()
	path := uniqueScreenshotPath(dir)
	a.finishScreenshotTo(s, path, img)
}

// shotSaveAs asks where to save the screenshot.
func (a *app) shotSaveAs(s *shotSession) {
	img := s.resultImage()
	dir := a.screenshotDir()
	name := screenshotName()
	parent := s.win
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      parent,
			Title:       tr("保存截图"),
			DefaultPath: filepath.Join(dir, name),
			ButtonLabel: tr("保存"),
			Filters:     []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}},
		})
		if err != nil || strings.TrimSpace(path) == "" {
			return
		}
		if filepath.Ext(path) == "" {
			path += ".png"
		}
		a.post(func() { a.finishScreenshotTo(s, path, img) })
	}()
}

// finishScreenshotTo encodes img to path, then closes the overlay and tells
// the user where the file went.
func (a *app) finishScreenshotTo(s *shotSession, path string, img *image.RGBA) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.setToast(tr("无法创建保存目录：") + err.Error())
		return
	}
	data, err := capture.EncodePNG(img, png.DefaultCompression)
	if err != nil {
		a.setToast(tr("无法生成截图"))
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		a.setToast(tr("无法保存截图：") + err.Error())
		return
	}
	a.shotClose(true)
	a.setToast(tr("截图已保存：") + path)
}

// screenshotName is the default file name for a saved screenshot.
func screenshotName() string {
	return "zchat-" + time.Now().Format("20060102-150405") + ".png"
}

// uniqueScreenshotPath returns a path in dir that does not overwrite an
// existing file, adding a counter when two screenshots share a second.
func uniqueScreenshotPath(dir string) string {
	base := strings.TrimSuffix(screenshotName(), ".png")
	path := filepath.Join(dir, base+".png")
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.png", base, i))
	}
	return filepath.Join(dir, base+"-"+model.NewID("shot")+".png")
}

func (a *app) shotPin(s *shotSession) {
	img := s.resultImage()
	a.createPin(img, s.frame.Scale)
	a.shotClose(true)
}

func (a *app) shotSend(s *shotSession) {
	if a.currentConv() == nil {
		a.setToast(tr("先打开一个聊天"))
		return
	}
	data, err := s.resultPNG(png.BestSpeed)
	if err != nil {
		a.setToast(tr("无法生成截图"))
		return
	}
	dir := ""
	if a.store != nil {
		dir = filepath.Join(a.store.CacheDir(), "screenshots")
	}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "zchat-screenshots")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		a.setToast(tr("无法保存截图：") + err.Error())
		return
	}
	path := filepath.Join(dir, model.NewID("shot")+".png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		a.setToast(tr("无法保存截图：") + err.Error())
		return
	}
	a.shotClose(false)
	a.showMain()
	a.sendFiles([]string{path})
}

// screenshotDir returns the folder Save starts in, creating it when needed.
func (a *app) screenshotDir() string {
	dir := strings.TrimSpace(a.settings.ScreenshotDir)
	if dir == "" {
		if pictures, err := mygo.App.Path(mygo.PathPictures); err == nil {
			dir = filepath.Join(pictures, appFolderName)
		}
	}
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// shortcutLabel shows a configured accelerator, or that none is set.
func shortcutLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return tr("未设置")
	}
	return value
}

// applyScreenshotShortcut registers the global capture shortcuts. An empty
// setting, "off", or the disabled screenshot feature turns one off.
func (a *app) applyScreenshotShortcut() {
	if !a.settings.ScreenshotEnabled {
		a.unregisterScreenshotShortcuts()
		return
	}
	a.setScreenshotShortcut(a.settings.ScreenshotIncludeShortcut, true)
	a.setScreenshotShortcut(a.settings.ScreenshotExcludeShortcut, false)
}

// unregisterScreenshotShortcuts removes both capture shortcuts.
func (a *app) unregisterScreenshotShortcuts() {
	if a.shotIncludeShortcut != "" {
		mygo.GlobalShortcut.Unregister(a.shotIncludeShortcut)
		a.shotIncludeShortcut = ""
	}
	if a.shotExcludeShortcut != "" {
		mygo.GlobalShortcut.Unregister(a.shotExcludeShortcut)
		a.shotExcludeShortcut = ""
	}
}

// setScreenshotShortcut registers next as the global shortcut of one capture
// mode, keeping the previous one when the platform rejects it. An empty value
// or "off" disables the shortcut.
func (a *app) setScreenshotShortcut(value string, includeChat bool) bool {
	reg := &a.shotIncludeShortcut
	setting := &a.settings.ScreenshotIncludeShortcut
	if !includeChat {
		reg = &a.shotExcludeShortcut
		setting = &a.settings.ScreenshotExcludeShortcut
	}
	next := strings.TrimSpace(value)
	if strings.EqualFold(next, "off") {
		next = ""
	}
	if next == *reg {
		*setting = next
		_ = a.settings.save()
		return true
	}
	prev := *reg
	if prev != "" {
		mygo.GlobalShortcut.Unregister(prev)
		*reg = ""
	}
	action := func() { a.startScreenshot(includeChat) }
	if next == "" {
		*setting = ""
		_ = a.settings.save()
		return true
	}
	if err := mygo.GlobalShortcut.Register(next, action); err != nil {
		if prev != "" {
			_ = mygo.GlobalShortcut.Register(prev, action)
			*reg = prev
		}
		a.setToast(fmt.Sprintf("%s%s", tr("截图快捷键注册失败："), err.Error()))
		return false
	}
	*reg = next
	*setting = next
	_ = a.settings.save()
	return true
}

// chooseScreenshotDir picks the folder Save starts in.
func (a *app) chooseScreenshotDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.settingsWin,
			Title:       tr("选择截图保存目录"),
			ButtonLabel: tr("选择"),
			DefaultPath: a.screenshotDir(),
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.ScreenshotDir = paths[0]
			_ = a.settings.save()
			a.setToast(tr("截图保存目录已更新"))
			a.redraw()
		})
	}()
}

// localizeWindows moves window bounds into the overlay's DIP coordinates.
func localizeWindows(windows []capture.WindowInfo, frame image.Rectangle) []capture.WindowInfo {
	out := make([]capture.WindowInfo, 0, len(windows))
	for _, w := range windows {
		w.Bounds = w.Bounds.Sub(frame.Min)
		if w.Bounds.Dx() < 2 || w.Bounds.Dy() < 2 {
			continue
		}
		out = append(out, w)
	}
	return out
}

// windowRect converts a window's DIP bounds to the screenshot's pixels.
func (s *shotSession) windowRect(w capture.WindowInfo) image.Rectangle {
	conv := func(p image.Point) image.Point {
		return image.Pt(
			int(math.Round(float64(p.X-s.frame.Bounds.Min.X)*float64(s.scale))),
			int(math.Round(float64(p.Y-s.frame.Bounds.Min.Y)*float64(s.scale))),
		)
	}
	r := image.Rect(
		conv(w.Bounds.Min).X, conv(w.Bounds.Min).Y,
		conv(w.Bounds.Max).X, conv(w.Bounds.Max).Y,
	)
	return r.Intersect(image.Rect(0, 0, s.doc.Width, s.doc.Height))
}
