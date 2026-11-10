package app

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/annotate"
	"xin.dart.zchat/internal/capture"
)

// shotTool is a tool button on the overlay toolbar.
type shotTool struct {
	key  string
	tool annotate.Tool
	icon *ui.SVG
	text string
}

var shotTools = []shotTool{
	{"rect", annotate.ToolRect, iconShotRect, "矩形"},
	{"ellipse", annotate.ToolEllipse, iconShotEllipse, "椭圆"},
	{"arrow", annotate.ToolArrow, iconShotArrow, "箭头"},
	{"line", annotate.ToolLine, iconShotLine, "直线"},
	{"pen", annotate.ToolPen, iconShotPen, "画笔"},
	{"highlight", annotate.ToolHighlight, iconShotHL, "荧光笔"},
	{"text", annotate.ToolText, iconShotText, "文字"},
	{"mosaic", annotate.ToolMosaic, iconShotMosaic, "马赛克"},
	{"number", annotate.ToolNumber, iconShotNumber, "序号"},
	{"eraser", annotate.ToolEraser, iconShotEraser, "橡皮"},
}

// shotColors are the annotation colors, chosen for contrast on screenshots.
var shotColors = []color.RGBA{
	{R: 0xea, G: 0x3d, B: 0x3d, A: 0xff},
	{R: 0xf5, G: 0xa6, B: 0x23, A: 0xff},
	{R: 0x35, G: 0xc7, B: 0x59, A: 0xff},
	{R: 0x0a, G: 0x84, B: 0xff, A: 0xff},
	{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff},
}

type shotAction struct {
	key  string
	icon *ui.SVG
	text string
}

var shotActions = []shotAction{
	{"undo", iconUndo, "撤销"},
	{"redo", iconRedo, "重做"},
	{"copy", iconCopy, "复制"},
	{"save", iconDownload, "保存"},
	{"pin", iconPin, "贴图"},
	{"send", iconSend, "发送"},
	{"close", iconX, "关闭"},
}

type shotToolbarButton struct {
	key  string
	rect ui.Rect
	kind int // 0 tool, 1 color, 2 action
}

type shotToolbar struct {
	rect    ui.Rect
	buttons []shotToolbarButton
}

func (t shotToolbar) at(x, y float32) string {
	for _, b := range t.buttons {
		if x >= b.rect.X && x < b.rect.X+b.rect.W && y >= b.rect.Y && y < b.rect.Y+b.rect.H {
			return b.key
		}
	}
	return ""
}

// shotView builds the full-window overlay.
func (a *app) shotView(c *ui.Context) {
	s := a.shot
	if s == nil {
		return
	}
	c.Root().Background(ui.Hex("#000000"))
	box := ui.Box(c).Fill().Focusable().Focus().HandleInput(s.input)
	box.Cursor(s.cursorShape())
	if s.textEditing {
		box.TextCaret(s.caretRect())
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r) })
}

func (s *shotSession) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		return s.key(ev)
	case ui.InputText:
		if s.textEditing {
			text := strings.ReplaceAll(ev.Text, "\r", "")
			text = strings.ReplaceAll(text, "\n", "")
			s.text += text
			return true
		}
		return false
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		return s.pointerDown(ev)
	case ui.InputPointerMove:
		s.pointerMove(ev)
		return true
	case ui.InputPointerUp:
		if ev.Button != 0 {
			return false
		}
		return s.pointerUp(ev)
	case ui.InputScroll:
		return false
	}
	return false
}

func (s *shotSession) key(ev ui.InputEvent) bool {
	a := shotApp
	if a == nil || a.shot != s {
		return false
	}
	mods := ev.Mods
	if s.textEditing {
		switch ev.Key {
		case ui.KeyEscape:
			s.textEditing = false
			s.text = ""
			return true
		case ui.KeyEnter:
			if mods&ui.Shift != 0 {
				s.text += "\n"
				return true
			}
			s.commitText()
			return true
		case ui.KeyBackspace:
			r := []rune(s.text)
			if len(r) > 0 {
				s.text = string(r[:len(r)-1])
			}
			return true
		}
		return false
	}
	switch ev.Key {
	case ui.KeyEscape:
		a.shotClose(true)
		return true
	case ui.KeyEnter:
		if mods&ui.Cmd != 0 {
			a.shotSend(s)
		} else {
			a.shotCopy(s)
		}
		return true
	case ui.KeyBackspace, ui.KeyDelete:
		if s.doc.Len() > 0 {
			s.doc.RemoveAt(s.doc.Len() - 1)
			s.annDirty = true
		}
		return true
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown:
		if s.sel.Empty() {
			return false
		}
		step := 1
		if mods&ui.Cmd != 0 {
			step = 10
		}
		dx, dy := 0, 0
		switch ev.Key {
		case ui.KeyLeft:
			dx = -step
		case ui.KeyRight:
			dx = step
		case ui.KeyUp:
			dy = -step
		case ui.KeyDown:
			dy = step
		}
		if mods&ui.Shift != 0 {
			s.resizeSelectionBy(dx, dy)
		} else {
			s.moveSelectionBy(dx, dy)
		}
		return true
	case ui.KeyZ:
		if mods&ui.Cmd != 0 {
			if mods&ui.Shift != 0 {
				s.redo()
			} else {
				s.undo()
			}
			return true
		}
	case ui.KeyY:
		if mods&ui.Cmd != 0 {
			s.redo()
			return true
		}
	case ui.KeyA:
		if mods&ui.Cmd != 0 {
			s.sel = image.Rect(0, 0, s.doc.Width, s.doc.Height)
			s.phase = shotAnnotating
			return true
		}
		if mods == 0 {
			s.chooseTool("arrow")
			return true
		}
	case ui.KeyC:
		if mods&ui.Cmd != 0 {
			a.shotCopy(s)
			return true
		}
	case ui.KeyS:
		if mods&ui.Cmd != 0 {
			if mods&ui.Shift != 0 {
				a.shotSaveAs(s)
			} else {
				a.shotSave(s)
			}
			return true
		}
		if mods == 0 {
			s.chooseTool("rect")
			return true
		}
	case ui.KeyT:
		if mods&ui.Cmd != 0 {
			a.shotPin(s)
			return true
		}
		if mods == 0 {
			s.chooseTool("text")
			return true
		}
	case ui.KeyP:
		if mods == 0 {
			s.chooseTool("pen")
			return true
		}
	case ui.KeyL:
		if mods == 0 {
			s.chooseTool("line")
			return true
		}
	case ui.KeyH:
		if mods == 0 {
			s.chooseTool("highlight")
			return true
		}
	case ui.KeyX:
		if mods == 0 {
			s.chooseTool("eraser")
			return true
		}
	case ui.KeyM:
		if mods == 0 {
			s.chooseTool("mosaic")
			return true
		}
	case ui.KeyN:
		if mods == 0 {
			s.chooseTool("number")
			return true
		}
	case ui.KeyO:
		if mods == 0 {
			s.chooseTool("ellipse")
			return true
		}
	case ui.KeyE:
		if mods == 0 {
			s.chooseTool("ellipse")
			return true
		}
	}
	return false
}

// shotApp lets the input handler reach the app without storing it in the
// session, which tests could otherwise copy.
var shotApp *app

func (s *shotSession) pointerDown(ev ui.InputEvent) bool {
	if key := s.toolbar.at(ev.X, ev.Y); key != "" {
		s.activate(key)
		return true
	}
	pt := s.toImage(ev.X, ev.Y)
	if s.textEditing {
		s.commitText()
	}
	s.cursor = pt
	s.cursorOn = true
	s.dragStart = pt
	s.dragSel = s.sel
	s.clickCount = ev.Clicks
	s.clickWindow = -1
	if s.phase == shotSelecting && s.hoverWindow >= 0 {
		s.clickWindow = s.hoverWindow
	}

	inside := !s.sel.Empty() && pt.In(s.sel)
	if inside && s.phase == shotAnnotating && ev.Clicks >= 2 && s.draft == nil {
		if shotApp != nil {
			shotApp.shotCopy(s)
			return true
		}
	}
	if s.phase == shotSelecting || !inside {
		if h := s.hitHandle(ev.X, ev.Y); h >= 0 {
			s.drag, s.handle = shotDragResize, h
			return true
		}
		if inside {
			s.drag = shotDragMove
			return true
		}
		s.drag = shotDragSelect
		s.sel = image.Rect(pt.X, pt.Y, pt.X, pt.Y)
		s.phase = shotSelecting
		return true
	}
	if s.sel.Empty() {
		s.drag = shotDragSelect
		s.sel = image.Rect(pt.X, pt.Y, pt.X, pt.Y)
		return true
	}
	switch s.tool {
	case annotate.ToolText:
		s.textEditing = true
		s.text = ""
		s.textAt = annotate.Point{X: float32(pt.X), Y: float32(pt.Y)}
		return true
	case annotate.ToolNumber:
		s.doc.Add(annotate.Op{
			Tool:   annotate.ToolNumber,
			Start:  annotate.Point{X: float32(pt.X), Y: float32(pt.Y)},
			Color:  s.ink,
			Width:  26 * s.scale,
			Number: s.nextNumber,
		})
		s.nextNumber++
		s.annDirty = true
		return true
	case annotate.ToolEraser:
		s.eraseAt(pt)
		s.drag = shotDragErase
		return true
	}
	op := annotate.Op{
		Tool:  s.tool,
		Start: annotate.Point{X: float32(pt.X), Y: float32(pt.Y)},
		End:   annotate.Point{X: float32(pt.X), Y: float32(pt.Y)},
		Color: s.ink,
		Width: 3 * s.scale,
	}
	switch s.tool {
	case annotate.ToolMosaic:
		op.Width = 14 * s.scale
	case annotate.ToolHighlight:
		op.Width = 12 * s.scale
	}
	if s.tool == annotate.ToolPen || s.tool == annotate.ToolHighlight {
		op.Points = []annotate.Point{op.Start}
	}
	s.draft = &op
	s.drag = shotDragDraw
	return true
}

func (s *shotSession) pointerMove(ev ui.InputEvent) {
	s.cursor = s.toImage(ev.X, ev.Y)
	s.cursorOn = true
	if s.drag == shotDragNone {
		if s.phase == shotSelecting {
			s.hoverHandle = s.hitHandle(ev.X, ev.Y)
			s.hoverWindow = -1
			if s.hoverHandle < 0 && len(s.windows) > 0 {
				if w, ok := capture.WindowAt(s.windows, image.Pt(int(ev.X), int(ev.Y))); ok {
					for i := range s.windows {
						if s.windows[i].ID == w.ID {
							s.hoverWindow = i
							break
						}
					}
				}
			}
		} else {
			s.hoverHandle = -1
			s.hoverWindow = -1
		}
		return
	}
	pt := s.cursor
	bounds := image.Rect(0, 0, s.doc.Width, s.doc.Height)
	snap := ev.Mods&ui.Alt == 0
	switch s.drag {
	case shotDragSelect:
		p := pt
		if snap {
			p = s.snapPoint(p)
		}
		s.sel = captureNormalize(s.dragStart, p).Intersect(bounds)
	case shotDragMove:
		d := pt.Sub(s.dragStart)
		r := s.dragSel.Add(d)
		r = clampMoved(r, bounds)
		if snap {
			r = s.snapMoved(r)
		}
		s.sel = r
	case shotDragResize:
		p := pt
		if snap {
			p = s.snapPoint(p)
		}
		s.sel = resizeSelection(s.dragSel, s.handle, p, bounds)
	case shotDragDraw:
		if s.draft == nil {
			return
		}
		s.draft.End = annotate.Point{X: float32(pt.X), Y: float32(pt.Y)}
		if s.tool == annotate.ToolPen || s.tool == annotate.ToolHighlight {
			s.draft.Points = append(s.draft.Points, s.draft.End)
		}
	case shotDragErase:
		s.eraseAt(pt)
	}
}

func (s *shotSession) pointerUp(ev ui.InputEvent) bool {
	pt := s.toImage(ev.X, ev.Y)
	switch s.drag {
	case shotDragSelect:
		s.sel = captureNormalize(s.dragStart, pt).Intersect(image.Rect(0, 0, s.doc.Width, s.doc.Height))
		if s.sel.Dx() < 3 || s.sel.Dy() < 3 {
			s.sel = image.Rectangle{}
			s.phase = shotSelecting
			if s.clickWindow >= 0 && s.clickWindow < len(s.windowRects) {
				if r := s.windowRects[s.clickWindow]; !r.Empty() {
					s.sel = r
					s.phase = shotAnnotating
					s.notice = ""
					break
				}
			}
			if s.clickCount >= 2 {
				s.sel = image.Rect(0, 0, s.doc.Width, s.doc.Height)
				s.phase = shotAnnotating
				s.notice = ""
			}
		} else {
			s.phase = shotAnnotating
			s.notice = ""
		}
	case shotDragDraw:
		if s.draft != nil {
			s.finishDraft(pt)
		}
	}
	s.drag = shotDragNone
	s.handle = -1
	return true
}

func (s *shotSession) finishDraft(pt image.Point) {
	op := *s.draft
	s.draft = nil
	op.End = annotate.Point{X: float32(pt.X), Y: float32(pt.Y)}
	switch op.Tool {
	case annotate.ToolPen, annotate.ToolHighlight:
		if len(op.Points) < 2 {
			return
		}
	case annotate.ToolMosaic:
		if absf(op.End.X-op.Start.X) < 4 || absf(op.End.Y-op.Start.Y) < 4 {
			return
		}
	default:
		if absf(op.End.X-op.Start.X) < 2 && absf(op.End.Y-op.Start.Y) < 2 {
			return
		}
	}
	s.doc.Add(op)
	s.annDirty = true
}

func (s *shotSession) eraseAt(pt image.Point) {
	if i := s.doc.Hit(annotate.Point{X: float32(pt.X), Y: float32(pt.Y)}); i >= 0 {
		s.doc.RemoveAt(i)
		s.annDirty = true
	}
}

func (s *shotSession) commitText() {
	if !s.textEditing {
		return
	}
	text := strings.TrimRight(s.text, "\n")
	s.textEditing = false
	s.text = ""
	if strings.TrimSpace(text) == "" {
		return
	}
	s.doc.Add(annotate.Op{
		Tool:     annotate.ToolText,
		Start:    s.textAt,
		Text:     text,
		Color:    s.ink,
		FontSize: 18 * s.scale,
		Width:    18 * s.scale,
	})
	s.annDirty = true
}

func (s *shotSession) undo() {
	if s.doc.Undo() {
		s.annDirty = true
	}
}

func (s *shotSession) redo() {
	if s.doc.Redo() {
		s.annDirty = true
	}
}

// moveSelectionBy nudges the selection, keeping it on the screenshot.
func (s *shotSession) moveSelectionBy(dx, dy int) {
	if s.sel.Empty() {
		return
	}
	bounds := image.Rect(0, 0, s.doc.Width, s.doc.Height)
	s.sel = clampMoved(s.sel.Add(image.Pt(dx, dy)), bounds)
	s.phase = shotAnnotating
}

// resizeSelectionBy grows or shrinks the selection from its far edges, so
// the arrow keys follow the direction they point.
func (s *shotSession) resizeSelectionBy(dx, dy int) {
	if s.sel.Empty() {
		return
	}
	r := s.sel
	if dx > 0 {
		r.Max.X += dx
	} else if dx < 0 {
		r.Min.X += dx
	}
	if dy > 0 {
		r.Max.Y += dy
	} else if dy < 0 {
		r.Min.Y += dy
	}
	bounds := image.Rect(0, 0, s.doc.Width, s.doc.Height)
	r = r.Intersect(bounds)
	if r.Dx() < 1 || r.Dy() < 1 {
		return
	}
	s.sel = r
	s.phase = shotAnnotating
}

func (s *shotSession) chooseTool(key string) {
	for _, t := range shotTools {
		if t.key == key {
			s.tool = t.tool
			if !s.sel.Empty() {
				s.phase = shotAnnotating
			}
			return
		}
	}
}

func (s *shotSession) activate(key string) {
	switch key {
	case "undo":
		s.undo()
	case "redo":
		s.redo()
	case "copy":
		shotApp.shotCopy(s)
	case "save":
		shotApp.shotSave(s)
	case "pin":
		shotApp.shotPin(s)
	case "send":
		shotApp.shotSend(s)
	case "close":
		shotApp.shotClose(true)
	default:
		if strings.HasPrefix(key, "c") {
			if i := atoiSafe(key[1:]); i >= 0 && i < len(shotColors) {
				s.ink = shotColors[i]
				return
			}
		}
		s.chooseTool(key)
	}
}

func atoiSafe(s string) int {
	if s == "" {
		return -1
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (s *shotSession) toImage(x, y float32) image.Point {
	scale := float64(s.scale)
	if scale <= 0 {
		scale = 1
	}
	px := int(math.Round(float64(x) * scale))
	py := int(math.Round(float64(y) * scale))
	px = max(0, min(s.doc.Width, px))
	py = max(0, min(s.doc.Height, py))
	return image.Pt(px, py)
}

func (s *shotSession) selDip() ui.Rect {
	return dipRect(s.sel, s.scale)
}

func (s *shotSession) snapTargets() (xs, ys []int) {
	xs = []int{0, s.doc.Width}
	ys = []int{0, s.doc.Height}
	for _, r := range s.windowRects {
		if r.Empty() {
			continue
		}
		xs = append(xs, r.Min.X, r.Max.X)
		ys = append(ys, r.Min.Y, r.Max.Y)
	}
	return xs, ys
}

func (s *shotSession) snapPoint(p image.Point) image.Point {
	xs, ys := s.snapTargets()
	t := int(math.Round(8 * float64(s.scale)))
	p.X = snapValue(p.X, xs, t)
	p.Y = snapValue(p.Y, ys, t)
	return p
}

func (s *shotSession) snapMoved(r image.Rectangle) image.Rectangle {
	xs, ys := s.snapTargets()
	t := int(math.Round(8 * float64(s.scale)))
	dx := snapDelta(r.Min.X, xs, t)
	if d := snapDelta(r.Max.X, xs, t); d != 0 && (dx == 0 || absInt(d) < absInt(dx)) {
		dx = d
	}
	dy := snapDelta(r.Min.Y, ys, t)
	if d := snapDelta(r.Max.Y, ys, t); d != 0 && (dy == 0 || absInt(d) < absInt(dy)) {
		dy = d
	}
	if dx == 0 && dy == 0 {
		return r
	}
	return r.Add(image.Pt(dx, dy))
}

func snapValue(v int, targets []int, threshold int) int {
	if d := snapDelta(v, targets, threshold); d != 0 {
		return v + d
	}
	return v
}

func snapDelta(v int, targets []int, threshold int) int {
	best, bestDist := 0, threshold+1
	for _, t := range targets {
		d := t - v
		ad := absInt(d)
		if ad < bestDist {
			best, bestDist = d, ad
		}
	}
	return best
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (s *shotSession) hitHandle(x, y float32) int {
	if s.sel.Empty() {
		return -1
	}
	r := s.selDip()
	pts := [][2]float32{
		{r.X, r.Y}, {r.X + r.W/2, r.Y}, {r.X + r.W, r.Y},
		{r.X + r.W, r.Y + r.H/2}, {r.X + r.W, r.Y + r.H},
		{r.X + r.W/2, r.Y + r.H}, {r.X, r.Y + r.H}, {r.X, r.Y + r.H/2},
	}
	for i, p := range pts {
		if absf(x-p[0]) <= 7 && absf(y-p[1]) <= 7 {
			return i
		}
	}
	return -1
}

func (s *shotSession) cursorShape() ui.Cursor {
	if s.drag == shotDragResize {
		return resizeCursor(s.handle)
	}
	if s.hoverHandle >= 0 {
		return resizeCursor(s.hoverHandle)
	}
	if s.drag == shotDragMove {
		return ui.CursorGrabbing
	}
	if s.tool == annotate.ToolText && s.pointerInSelection() {
		return ui.CursorText
	}
	if !s.sel.Empty() && s.pointerInSelection() && s.phase == shotSelecting {
		return ui.CursorMove
	}
	return ui.CursorCrosshair
}

func (s *shotSession) pointerInSelection() bool {
	return s.cursorOn && s.cursor.In(s.sel)
}

func resizeCursor(handle int) ui.Cursor {
	switch handle {
	case 0, 4:
		return ui.CursorResizeNWSE
	case 2, 6:
		return ui.CursorResizeNESW
	case 1, 5:
		return ui.CursorResizeNS
	case 3, 7:
		return ui.CursorResizeEW
	}
	return ui.CursorDefault
}

func (s *shotSession) caretRect() ui.Rect {
	x := s.textAt.X / s.scale
	y := s.textAt.Y / s.scale
	w, _ := annotate.Measure(s.text, 18*s.scale)
	return ui.Rect{X: x + w/s.scale, Y: y, W: 1.5, H: 20}
}

// paint draws the screenshot, the selection mask, annotations and toolbar.
func (s *shotSession) paint(p *ui.Painter, r ui.Rect) {
	s.winW, s.winH = r.W, r.H
	full := ui.Rect{X: 0, Y: 0, W: r.W, H: r.H}
	if s.base != nil {
		p.Image(s.base, full, ui.FillBox)
	}
	if s.annDirty || s.annLayer == nil {
		img := annotate.Render(nil, s.doc, s.doc.Crop())
		if s.annLayer == nil {
			s.annLayer = ui.NewBitmap(img)
		} else {
			s.annLayer.Update(img)
		}
		s.annDirty = false
	}
	if s.annLayer != nil {
		p.Image(s.annLayer, full, ui.FillBox)
	}
	if s.draft != nil {
		s.paintOp(p, *s.draft)
	}
	if s.textEditing {
		x, y := s.textAt.X/s.scale, s.textAt.Y/s.scale
		p.Text(x, y, s.text, 18, ui.RGBA(s.ink.R, s.ink.G, s.ink.B, 1))
		w, _ := annotate.Measure(s.text, 18*s.scale)
		p.Line(x+w/s.scale, y, x+w/s.scale, y+20, 1.5, ui.RGBA(s.ink.R, s.ink.G, s.ink.B, 1))
	}

	if s.phase == shotSelecting && s.sel.Empty() && s.hoverWindow >= 0 && s.hoverWindow < len(s.windowRects) {
		if wr := s.windowRects[s.hoverWindow]; !wr.Empty() {
			rect := dipRect(wr, s.scale)
			p.Fill(rect, ui.RGBA(10, 132, 255, 0.14), 0)
			p.Stroke(rect, ui.RGBA(10, 132, 255, 0.95), 0, 1.5)
			s.paintWindowTitle(p, rect, s.windows[s.hoverWindow].Title)
		}
	}

	if !s.sel.Empty() {
		sel := s.selDip()
		mask := ui.RGBA(0, 0, 0, 0.46)
		if sel.Y > 0 {
			p.Fill(ui.Rect{X: 0, Y: 0, W: r.W, H: sel.Y}, mask, 0)
		}
		if bottom := sel.Y + sel.H; bottom < r.H {
			p.Fill(ui.Rect{X: 0, Y: bottom, W: r.W, H: r.H - bottom}, mask, 0)
		}
		if sel.X > 0 {
			p.Fill(ui.Rect{X: 0, Y: sel.Y, W: sel.X, H: sel.H}, mask, 0)
		}
		if right := sel.X + sel.W; right < r.W {
			p.Fill(ui.Rect{X: right, Y: sel.Y, W: r.W - right, H: sel.H}, mask, 0)
		}
		p.Stroke(sel, ui.RGBA(10, 132, 255, 1), 0, 1.5)
		s.paintHandles(p, sel)
		s.paintSizeLabel(p, sel)
		s.toolbar = s.layoutToolbar()
		s.paintToolbar(p)
	} else {
		s.toolbar = shotToolbar{}
		s.paintHint(p, r)
	}
	if s.phase == shotSelecting && s.cursorOn {
		s.paintMagnifier(p, r)
	}
}

func (s *shotSession) paintWindowTitle(p *ui.Painter, rect ui.Rect, title string) {
	if strings.TrimSpace(title) == "" {
		return
	}
	const size = 12
	w, h := p.MeasureText(0, ui.Span{Text: title, Size: size})
	w = min(w, s.winW-16)
	x := rect.X + 6
	y := rect.Y - h - 8
	if y < 4 {
		y = rect.Y + 6
	}
	if x+w+12 > s.winW {
		x = s.winW - w - 18
	}
	if x < 4 {
		x = 4
	}
	p.Fill(ui.Rect{X: x, Y: y, W: w + 12, H: h + 6}, ui.RGBA(20, 20, 24, 0.85), 4)
	p.Text(x+6, y+3, title, size, ui.RGBA(255, 255, 255, 0.95))
}

func (s *shotSession) paintHandles(p *ui.Painter, sel ui.Rect) {
	const half = 4
	pts := [][2]float32{
		{sel.X, sel.Y}, {sel.X + sel.W/2, sel.Y}, {sel.X + sel.W, sel.Y},
		{sel.X + sel.W, sel.Y + sel.H/2}, {sel.X + sel.W, sel.Y + sel.H},
		{sel.X + sel.W/2, sel.Y + sel.H}, {sel.X, sel.Y + sel.H}, {sel.X, sel.Y + sel.H/2},
	}
	for _, pt := range pts {
		sq := ui.Rect{X: pt[0] - half, Y: pt[1] - half, W: half * 2, H: half * 2}
		p.Fill(sq, ui.RGBA(255, 255, 255, 1), 1)
		p.Stroke(sq, ui.RGBA(10, 132, 255, 1), 1, 1)
	}
}

func (s *shotSession) paintSizeLabel(p *ui.Painter, sel ui.Rect) {
	label := fmt.Sprintf("%d × %d", s.sel.Dx(), s.sel.Dy())
	w, h := p.MeasureText(0, ui.Span{Text: label, Size: 11})
	x := sel.X
	y := sel.Y + sel.H + 6
	if y+h > s.winH-4 {
		y = sel.Y - h - 6
	}
	if y < 2 {
		y = sel.Y + 6
	}
	bg := ui.Rect{X: x, Y: y, W: w + 12, H: h + 6}
	p.Fill(bg, ui.RGBA(20, 20, 24, 0.82), 4)
	p.Text(x+6, y+3, label, 11, ui.RGBA(255, 255, 255, 1))
}

func (s *shotSession) paintHint(p *ui.Painter, r ui.Rect) {
	hint := s.notice
	if hint == "" {
		hint = tr("拖动选择区域，按 Esc 取消")
	}
	w, h := p.MeasureText(0, ui.Span{Text: hint, Size: 13})
	x := (r.W - w) / 2
	y := float32(26)
	if x < 12 {
		x = 12
	}
	box := ui.Rect{X: x - 14, Y: y - 8, W: w + 28, H: h + 16}
	p.Fill(box, ui.RGBA(20, 20, 24, 0.78), 8)
	p.Text(x, y, hint, 13, ui.RGBA(255, 255, 255, 0.92))
}

func (s *shotSession) paintMagnifier(p *ui.Painter, r ui.Rect) {
	const box = 138
	const zoom = 6
	srcW := float32(box) * s.scale / zoom
	cx := float32(s.cursor.X)
	cy := float32(s.cursor.Y)
	src := ui.Rect{X: cx - srcW/2, Y: cy - srcW/2, W: srcW, H: srcW}
	if src.X < 0 {
		src.X = 0
	}
	if src.Y < 0 {
		src.Y = 0
	}
	if src.X+src.W > float32(s.doc.Width) {
		src.X = float32(s.doc.Width) - src.W
	}
	if src.Y+src.H > float32(s.doc.Height) {
		src.Y = float32(s.doc.Height) - src.H
	}
	if src.W <= 0 || src.H <= 0 {
		return
	}
	x := float32(s.cursor.X)/s.scale + 22
	y := float32(s.cursor.Y)/s.scale + 22
	if x+box > r.W-10 {
		x = float32(s.cursor.X)/s.scale - box - 22
	}
	if y+box+26 > r.H-10 {
		y = float32(s.cursor.Y)/s.scale - box - 26 - 22
	}
	if x < 10 {
		x = 10
	}
	if y < 10 {
		y = 10
	}
	dst := ui.Rect{X: x, Y: y, W: box, H: box}
	p.Fill(ui.Rect{X: x - 2, Y: y - 2, W: box + 4, H: box + 4}, ui.RGBA(0, 0, 0, 0.6), 6)
	if s.pixelated != nil {
		p.ImageRegion(s.pixelated, src, dst, ui.ImageOptions{})
	}
	p.Stroke(dst, ui.RGBA(255, 255, 255, 0.7), 0, 1)
	// Crosshair at the cursor's pixel.
	scale := float32(box) / srcW
	hx := x + (cx-src.X)*scale
	hy := y + (cy-src.Y)*scale
	p.Line(hx, y, hx, y+box, 1, ui.RGBA(10, 132, 255, 0.8))
	p.Line(x, hy, x+box, hy, 1, ui.RGBA(10, 132, 255, 0.8))
	c := s.frame.Image.RGBAAt(s.cursor.X, s.cursor.Y)
	hex := fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	info := fmt.Sprintf("%s   %d, %d", hex, s.cursor.X, s.cursor.Y)
	iw, ih := p.MeasureText(0, ui.Span{Text: info, Size: 10.5})
	ib := ui.Rect{X: x, Y: y + box + 4, W: box, H: ih + 6}
	p.Fill(ib, ui.RGBA(20, 20, 24, 0.85), 4)
	p.Text(x+(box-iw)/2, y+box+7, info, 10.5, ui.RGBA(255, 255, 255, 0.9))
}

func (s *shotSession) layoutToolbar() shotToolbar {
	if s.sel.Empty() {
		return shotToolbar{}
	}
	const (
		btn   = float32(30)
		gap   = float32(3)
		pad   = float32(6)
		sep   = float32(6)
		color = float32(24)
	)
	x := pad
	y := pad
	var buttons []shotToolbarButton
	add := func(key string, w float32, kind int) {
		buttons = append(buttons, shotToolbarButton{key: key, rect: ui.Rect{X: x, Y: y, W: w, H: btn}, kind: kind})
		x += w + gap
	}
	for _, t := range shotTools {
		add(t.key, btn, 0)
	}
	x += sep
	for i := range shotColors {
		add(fmt.Sprintf("c%d", i), color, 1)
	}
	x += sep
	for _, act := range shotActions {
		add(act.key, btn, 2)
	}
	total := x - gap + pad
	height := btn + pad*2
	sel := s.selDip()
	tx := sel.X + sel.W - total
	if tx < 8 {
		tx = 8
	}
	if tx+total > s.winW-8 {
		tx = s.winW - total - 8
	}
	if tx < 8 {
		tx = 8
	}
	ty := sel.Y + sel.H + 10
	if ty+height > s.winH-8 {
		ty = sel.Y - height - 10
	}
	if ty < 8 {
		ty = 8
	}
	for i := range buttons {
		buttons[i].rect.X += tx
		buttons[i].rect.Y += ty
	}
	return shotToolbar{rect: ui.Rect{X: tx, Y: ty, W: total, H: height}, buttons: buttons}
}

func (s *shotSession) paintToolbar(p *ui.Painter) {
	t := s.toolbar
	if t.rect.W <= 0 {
		return
	}
	p.Shadow(t.rect, 12, 0, 6, 22, 0, ui.RGBA(0, 0, 0, 0.45))
	p.Fill(t.rect, ui.RGBA(28, 28, 32, 0.96), 11)
	for _, b := range t.buttons {
		switch b.kind {
		case 0:
			tool, ok := shotToolByKey(b.key)
			if !ok {
				continue
			}
			if s.tool == tool.tool {
				p.Fill(b.rect, ui.RGBA(10, 132, 255, 0.9), 7)
			}
			inner := inset(b.rect, 7)
			color := ui.RGBA(255, 255, 255, 0.92)
			if s.tool == tool.tool {
				color = ui.RGBA(255, 255, 255, 1)
			}
			p.Icon(tool.icon, inner, color)
		case 1:
			i := atoiSafe(strings.TrimPrefix(b.key, "c"))
			if i < 0 || i >= len(shotColors) {
				continue
			}
			c := shotColors[i]
			cx, cy := b.rect.X+b.rect.W/2, b.rect.Y+b.rect.H/2
			var path ui.Path
			path.Circle(cx, cy, 8)
			p.FillPath(&path, ui.RGBA(c.R, c.G, c.B, 1))
			if s.ink == c {
				p.StrokePath(&path, 2, ui.RGBA(10, 132, 255, 1))
			} else {
				p.StrokePath(&path, 1, ui.RGBA(255, 255, 255, 0.25))
			}
		case 2:
			act, ok := shotActionByKey(b.key)
			if !ok {
				continue
			}
			color := ui.RGBA(255, 255, 255, 0.9)
			if (b.key == "undo" && !s.doc.CanUndo()) || (b.key == "redo" && !s.doc.CanRedo()) {
				color = ui.RGBA(255, 255, 255, 0.28)
			}
			p.Icon(act.icon, inset(b.rect, 7), color)
		}
	}
}

func shotToolByKey(key string) (shotTool, bool) {
	for _, t := range shotTools {
		if t.key == key {
			return t, true
		}
	}
	return shotTool{}, false
}

func shotActionByKey(key string) (shotAction, bool) {
	for _, a := range shotActions {
		if a.key == key {
			return a, true
		}
	}
	return shotAction{}, false
}

func inset(r ui.Rect, n float32) ui.Rect {
	return ui.Rect{X: r.X + n, Y: r.Y + n, W: r.W - n*2, H: r.H - n*2}
}

// paintOp previews the annotation being dragged. Export draws the same
// operation at full resolution.
func (s *shotSession) paintOp(p *ui.Painter, op annotate.Op) {
	scale := s.scale
	if scale <= 0 {
		scale = 1
	}
	rect := func(a, b annotate.Point) ui.Rect {
		x0, x1 := a.X/scale, b.X/scale
		y0, y1 := a.Y/scale, b.Y/scale
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	}
	col := ui.RGBA(op.Color.R, op.Color.G, op.Color.B, 1)
	width := op.Width / scale
	switch op.Tool {
	case annotate.ToolRect:
		r := rect(op.Start, op.End)
		if op.Filled {
			p.Fill(r, col, 0)
		} else {
			p.Stroke(r, col, 0, width)
		}
	case annotate.ToolEllipse:
		r := rect(op.Start, op.End)
		var path ui.Path
		ellipsePath(&path, r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2)
		if op.Filled {
			p.FillPath(&path, col)
		} else {
			p.StrokePath(&path, width, col)
		}
	case annotate.ToolArrow:
		a := ui.Point{X: op.Start.X / scale, Y: op.Start.Y / scale}
		b := ui.Point{X: op.End.X / scale, Y: op.End.Y / scale}
		p.Line(a.X, a.Y, b.X, b.Y, width, col)
		drawArrowHead(p, a, b, max(width*4, 10), col)
	case annotate.ToolPen, annotate.ToolHighlight:
		hl := op.Tool == annotate.ToolHighlight
		c := col
		if hl {
			c = col.Alpha(0.35)
			width = max(width, 12)
		}
		pts := op.Points
		if len(pts) == 0 {
			pts = []annotate.Point{op.Start, op.End}
		}
		line := make([][2]float32, len(pts))
		for i, pt := range pts {
			line[i] = [2]float32{pt.X / scale, pt.Y / scale}
		}
		p.StrokePolyline(line, width, c)
	case annotate.ToolMosaic:
		r := rect(op.Start, op.End)
		src := ui.Rect{X: r.X * scale, Y: r.Y * scale, W: r.W * scale, H: r.H * scale}
		if s.pixelated != nil {
			p.ImageRegion(s.pixelated, src, r, ui.ImageOptions{})
		}
		p.Stroke(r, ui.RGBA(255, 255, 255, 0.5), 0, 1)
	case annotate.ToolNumber:
		cx, cy := op.Start.X/scale, op.Start.Y/scale
		radius := op.Width / scale / 2
		var path ui.Path
		path.Circle(cx, cy, radius)
		p.FillPath(&path, col)
		label := fmt.Sprintf("%d", max(1, op.Number))
		tw, th := p.MeasureText(0, ui.Span{Text: label, Size: radius * 1.1})
		p.Text(cx-tw/2, cy-th/2, label, radius*1.1, ui.RGBA(255, 255, 255, 1))
	}
}

func ellipsePath(path *ui.Path, cx, cy, rx, ry float32) {
	const k = 0.5522847498
	ox, oy := rx*k, ry*k
	path.MoveTo(cx+rx, cy)
	path.CubeTo(cx+rx, cy+oy, cx+ox, cy+ry, cx, cy+ry)
	path.CubeTo(cx-ox, cy+ry, cx-rx, cy+oy, cx-rx, cy)
	path.CubeTo(cx-rx, cy-oy, cx-ox, cy-ry, cx, cy-ry)
	path.CubeTo(cx+ox, cy-ry, cx+rx, cy-oy, cx+rx, cy)
	path.Close()
}

func drawArrowHead(p *ui.Painter, a, b ui.Point, head float32, c ui.Color) {
	length := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
	if length < 1 {
		return
	}
	ux := (b.X - a.X) / length
	uy := (b.Y - a.Y) / length
	head = min(head, length*0.6)
	base := ui.Point{X: b.X - ux*head, Y: b.Y - uy*head}
	half := head * 0.42
	var path ui.Path
	path.MoveTo(b.X, b.Y)
	path.LineTo(base.X-uy*half, base.Y+ux*half)
	path.LineTo(base.X+uy*half, base.Y-ux*half)
	path.Close()
	p.FillPath(&path, c)
}

func dipRect(r image.Rectangle, scale float32) ui.Rect {
	if scale <= 0 {
		scale = 1
	}
	return ui.Rect{
		X: float32(r.Min.X) / scale,
		Y: float32(r.Min.Y) / scale,
		W: float32(r.Dx()) / scale,
		H: float32(r.Dy()) / scale,
	}
}

func captureNormalize(a, b image.Point) image.Rectangle {
	return image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X), max(a.Y, b.Y))
}

func clampMoved(r, bounds image.Rectangle) image.Rectangle {
	dx := 0
	if r.Min.X < bounds.Min.X {
		dx = bounds.Min.X - r.Min.X
	} else if r.Max.X > bounds.Max.X {
		dx = bounds.Max.X - r.Max.X
	}
	dy := 0
	if r.Min.Y < bounds.Min.Y {
		dy = bounds.Min.Y - r.Min.Y
	} else if r.Max.Y > bounds.Max.Y {
		dy = bounds.Max.Y - r.Max.Y
	}
	return r.Add(image.Pt(dx, dy)).Intersect(bounds)
}

func resizeSelection(orig image.Rectangle, handle int, pt image.Point, bounds image.Rectangle) image.Rectangle {
	r := orig
	pt.X = max(bounds.Min.X, min(bounds.Max.X, pt.X))
	pt.Y = max(bounds.Min.Y, min(bounds.Max.Y, pt.Y))
	switch handle {
	case 0:
		r.Min.X, r.Min.Y = pt.X, pt.Y
	case 1:
		r.Min.Y = pt.Y
	case 2:
		r.Max.X, r.Min.Y = pt.X, pt.Y
	case 3:
		r.Max.X = pt.X
	case 4:
		r.Max.X, r.Max.Y = pt.X, pt.Y
	case 5:
		r.Max.Y = pt.Y
	case 6:
		r.Min.X, r.Max.Y = pt.X, pt.Y
	case 7:
		r.Min.X = pt.X
	}
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	return r.Intersect(bounds)
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// sortedKeys is a tiny helper used by tests.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// keep sort imported for older Go versions where helpers used it.
var _ = sort.Strings
