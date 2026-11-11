package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/capture"
)

// pinWindow is a screenshot pinned above other windows. The user can move
// it, resize it, change its opacity, copy it, save it or close it.
type pinWindow struct {
	win     *mygo.Window
	img     *image.RGBA
	bitmap  *ui.Bitmap
	scale   float32
	opacity float64
	offset  int
	ignore  bool
	closing bool

	dragging   bool
	dragCursor mygo.Point
	dragPos    mygo.Point
}

// createPin shows img in a frameless always-on-top window. scale is the
// display scale the screenshot was taken at, so the pin appears at the size
// the user selected on screen.
func (a *app) createPin(img *image.RGBA, scale float64) {
	if img == nil || img.Bounds().Dx() < 1 || img.Bounds().Dy() < 1 {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	p := &pinWindow{
		img:     img,
		bitmap:  ui.NewBitmap(img),
		scale:   float32(scale),
		opacity: 1,
		offset:  len(a.pins),
	}
	w := float64(img.Bounds().Dx()) / scale
	h := float64(img.Bounds().Dy()) / scale
	// Keep the pin inside a sensible part of the display it lands on.
	work := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).WorkArea
	maxW := float64(work.Width) * 0.86
	maxH := float64(work.Height) * 0.86
	if maxW > 40 && w > maxW {
		h *= maxW / w
		w = maxW
	}
	if maxH > 40 && h > maxH {
		w *= maxH / h
		h = maxH
	}
	if w < 24 {
		w = 24
	}
	if h < 24 {
		h = 24
	}
	x := work.X + (work.Width-int(w))/2 + p.offset*28
	y := work.Y + (work.Height-int(h))/2 + p.offset*28
	if x+int(w) > work.X+work.Width {
		x = work.X + work.Width - int(w) - 8
	}
	if y+int(h) > work.Y+work.Height {
		y = work.Y + work.Height - int(h) - 8
	}
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           tr("贴图"),
		X:               x,
		Y:               y,
		Width:           int(w),
		Height:          int(h),
		MinWidth:        40,
		MinHeight:       40,
		Frameless:       true,
		AlwaysOnTop:     true,
		SkipTaskbar:     true,
		DisableMaximize: true,
		BackgroundColor: "#000000",
		Content:         ui.View(p.view),
	})
	p.win.SetVisibleOnAllWorkspaces(true)
	p.win.OnClosed(func() {
		p.closing = true
		a.removePin(p)
	})
	a.pins = append(a.pins, p)
	p.win.Show()
	p.win.Focus()
}

func (a *app) removePin(p *pinWindow) {
	for i, q := range a.pins {
		if q == p {
			a.pins = append(a.pins[:i], a.pins[i+1:]...)
			break
		}
	}
	if p.bitmap != nil {
		p.bitmap.ReleasePixels()
		p.bitmap = nil
	}
	if p.win != nil && !p.win.IsDestroyed() && !p.closing {
		p.closing = true
		p.win.Close()
	}
}

// pinCount reports how many pins are open.
func (a *app) pinCount() int {
	n := 0
	for _, p := range a.pins {
		if p.win != nil && !p.win.IsDestroyed() {
			n++
		}
	}
	return n
}

// closeAllPins closes every pinned screenshot.
func (a *app) closeAllPins() {
	for _, p := range append([]*pinWindow(nil), a.pins...) {
		p.close()
	}
}

// showAllPins brings every pin back into view.
func (a *app) showAllPins() {
	for _, p := range a.pins {
		if p.win != nil && !p.win.IsDestroyed() {
			p.win.Show()
			p.win.Focus()
		}
	}
}

// setPinsInteractive turns mouse interaction on or off for every pin. Off
// lets clicks reach the windows behind the pins, so a pin can sit over a
// document and still be readable without being in the way.
func (a *app) setPinsInteractive(on bool) {
	for _, p := range a.pins {
		p.setInteractive(on)
	}
	if on {
		toastAnywhere(tr("贴图已恢复鼠标交互"))
	} else {
		toastAnywhere(tr("贴图已开启鼠标穿透"))
	}
}

func (p *pinWindow) setInteractive(on bool) {
	p.ignore = !on
	if p.win != nil && !p.win.IsDestroyed() {
		p.win.SetIgnoreMouseEvents(!on)
	}
}

func (p *pinWindow) close() {
	if p.closing {
		return
	}
	p.closing = true
	if p.win != nil && !p.win.IsDestroyed() {
		p.win.Close()
	}
}

// hidePinsForCapture hides every pin so the screenshot does not contain
// them, returning those that were visible.
func (a *app) hidePinsForCapture() []*pinWindow {
	var hidden []*pinWindow
	for _, p := range a.pins {
		if p.win != nil && !p.win.IsDestroyed() && p.win.IsVisible() {
			p.win.Hide()
			hidden = append(hidden, p)
		}
	}
	return hidden
}

func (a *app) restorePins(pins []*pinWindow) {
	for _, p := range pins {
		if p.win != nil && !p.win.IsDestroyed() && p.bitmap != nil {
			p.win.Show()
		}
	}
}

func (p *pinWindow) view(c *ui.Context) {
	c.Root().Background(ui.Hex("#000000"))
	box := ui.Box(c).Fill().Focusable().Focus().HandleInput(p.input).Cursor(ui.CursorGrab).
		PopoverMenu(func(m *ui.Menu) {
			if m.Item(tr("复制")).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
				p.copyToClipboard()
			}
			if m.Item(tr("保存")).Shortcut(ui.Cmd, ui.KeyS).Chosen() {
				p.saveDirect()
			}
			if m.Item(tr("保存为…")).Shortcut(ui.Cmd|ui.Shift, ui.KeyS).Chosen() {
				p.saveAs()
			}
			m.Separator()
			if m.Item(tr("鼠标穿透")).Checked(p.ignore).Chosen() {
				p.setInteractive(p.ignore)
			}
			m.Separator()
			for _, o := range []float64{1, 0.85, 0.7, 0.55, 0.4} {
				label := fmt.Sprintf("%s %d%%", tr("不透明度"), int(o*100))
				if m.Item(label).Checked(absf64(p.opacity-o) < 0.001).Chosen() {
					p.setOpacity(o)
				}
			}
			m.Separator()
			if m.Item(tr("关闭")).Shortcut(0, ui.KeyEscape).Chosen() {
				p.close()
			}
			if m.Item(tr("关闭全部贴图")).Chosen() {
				if globalApp != nil {
					globalApp.closeAllPins()
				}
			}
		})
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		if p.bitmap != nil {
			pt.Image(p.bitmap, ui.Rect{X: 0, Y: 0, W: r.W, H: r.H}, ui.Contain)
		}
	})
}

func (p *pinWindow) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		switch ev.Key {
		case ui.KeyEscape:
			p.close()
			return true
		case ui.KeyC:
			if ev.Mods&ui.Cmd != 0 {
				p.copyToClipboard()
				return true
			}
		case ui.KeyS:
			if ev.Mods&ui.Cmd != 0 {
				if ev.Mods&ui.Shift != 0 {
					p.saveAs()
				} else {
					p.saveDirect()
				}
				return true
			}
		}
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		if ev.Clicks >= 2 {
			p.close()
			return true
		}
		if p.win != nil && !p.win.IsDestroyed() {
			x, y := p.win.Position()
			p.dragging = true
			p.dragCursor = mygo.Screen.CursorScreenPoint()
			p.dragPos = mygo.Point{X: x, Y: y}
		}
		return true
	case ui.InputPointerMove:
		if p.dragging && p.win != nil && !p.win.IsDestroyed() {
			cur := mygo.Screen.CursorScreenPoint()
			p.win.SetPosition(
				p.dragPos.X+cur.X-p.dragCursor.X,
				p.dragPos.Y+cur.Y-p.dragCursor.Y,
			)
			return true
		}
	case ui.InputPointerUp:
		if p.dragging {
			p.dragging = false
			return true
		}
	case ui.InputScroll:
		factor := 1.0
		if ev.DY > 0 {
			factor = 1.1
		} else if ev.DY < 0 {
			factor = 1 / 1.1
		}
		if factor == 1 {
			return false
		}
		switch {
		case ev.Mods&ui.Cmd != 0 && ev.Mods&ui.Shift != 0:
			p.setOpacity(clampF64(p.opacity+(factor-1)*2, 0.2, 1))
			return true
		case ev.Mods&ui.Cmd != 0:
			p.resize(factor)
			return true
		default:
			p.setOpacity(clampF64(p.opacity-(factor-1)*2, 0.2, 1))
			return true
		}
	}
	return false
}

func (p *pinWindow) setOpacity(v float64) {
	v = clampF64(v, 0.2, 1)
	if v == p.opacity {
		return
	}
	p.opacity = v
	if p.win != nil && !p.win.IsDestroyed() {
		p.win.SetOpacity(v)
	}
}

// resize scales the pin around its center, keeping the image's aspect.
func (p *pinWindow) resize(factor float64) {
	if p.win == nil || p.win.IsDestroyed() || p.img == nil {
		return
	}
	x, y := p.win.Position()
	w, h := p.win.Size()
	if w < 1 || h < 1 {
		return
	}
	nw := int(float64(w) * factor)
	nh := int(float64(h) * factor)
	if nw < 40 {
		nw, nh = 40, int(float64(h)*40/float64(w))
	}
	if nh < 40 {
		nh, nw = 40, int(float64(w)*40/float64(h))
	}
	if nw > 10000 || nh > 10000 {
		return
	}
	cx := x + w/2
	cy := y + h/2
	p.win.SetBounds(mygo.Rectangle{
		X: cx - nw/2, Y: cy - nh/2, Width: nw, Height: nh,
	})
}

func (p *pinWindow) copyToClipboard() {
	if p.img == nil {
		return
	}
	data, err := capture.EncodePNG(p.img, png.BestSpeed)
	if err != nil {
		toastAnywhere(tr("无法生成截图"))
		return
	}
	if err := mygo.Clipboard.WriteImage(data); err != nil {
		toastAnywhere(tr("无法复制截图：") + err.Error())
		return
	}
	toastAnywhere(tr("截图已复制到剪贴板"))
}

// saveDirect writes the pin to the screenshot folder without asking.
func (p *pinWindow) saveDirect() {
	if p.img == nil {
		return
	}
	dir := ""
	if globalApp != nil {
		dir = globalApp.screenshotDir()
	} else if home, err := mygo.App.Path(mygo.PathPictures); err == nil {
		dir = filepath.Join(home, appFolderName)
	}
	base := "zchat-pin-" + time.Now().Format("20060102-150405")
	path := filepath.Join(dir, base+".png")
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.png", base, i))
	}
	p.write(path)
}

// saveAs asks where to save the pin.
func (p *pinWindow) saveAs() {
	if p.img == nil {
		return
	}
	dir := ""
	if globalApp != nil {
		dir = globalApp.screenshotDir()
	}
	name := "zchat-pin-" + time.Now().Format("20060102-150405") + ".png"
	parent := p.win
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
		p.write(path)
	}()
}

func (p *pinWindow) write(path string) {
	if p.img == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		toastAnywhere(tr("无法创建保存目录：") + err.Error())
		return
	}
	data, err := capture.EncodePNG(p.img, png.DefaultCompression)
	if err != nil {
		toastAnywhere(tr("无法生成截图"))
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		toastAnywhere(tr("无法保存截图：") + err.Error())
		return
	}
	toastAnywhere(tr("截图已保存：") + path)
}

// toastAnywhere shows a toast on the main window when it exists; pin windows
// are usable while zchat is hidden.
func toastAnywhere(msg string) {
	if globalApp != nil {
		globalApp.post(func() { globalApp.setToast(msg) })
	}
}

func clampF64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absf64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
