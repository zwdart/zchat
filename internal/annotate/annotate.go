// Package annotate keeps annotation operations and draws them onto an
// image. Coordinates are image pixels, which makes the exported PNG crisp
// on every display scale.
//
// The package depends on the standard library and golang.org/x/image only,
// so it has no UI toolkit and is easy to unit test.
package annotate

import (
	"image"
	"image/color"
	"math"
)

// Tool is an annotation kind.
type Tool int

const (
	// ToolRect draws a rectangle, filled when Op.Filled is set.
	ToolRect Tool = iota
	// ToolEllipse draws an ellipse.
	ToolEllipse
	// ToolArrow draws a line with a triangular head.
	ToolArrow
	// ToolLine draws a straight line between Start and End.
	ToolLine
	// ToolPen draws a freehand line through Op.Points.
	ToolPen
	// ToolHighlight draws a translucent marker stroke.
	ToolHighlight
	// ToolText draws Op.Text at Op.Start.
	ToolText
	// ToolMosaic pixelates the rectangle between Start and End.
	ToolMosaic
	// ToolNumber draws a numbered circle at Start.
	ToolNumber
	// ToolEraser is an editing gesture: it removes a whole annotation and
	// is never stored in a document.
	ToolEraser
)

// Point is a position in image pixels.
type Point struct {
	X, Y float32
}

func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// In reports whether p is inside r.
func (p Point) In(r image.Rectangle) bool {
	return p.X >= float32(r.Min.X) && p.Y >= float32(r.Min.Y) &&
		p.X < float32(r.Max.X) && p.Y < float32(r.Max.Y)
}

// Bounds is the rectangle an operation covers, with its line width included.
// It is used for hit testing, not for clipping the final image.
func (o Op) Bounds() image.Rectangle {
	pad := o.Width/2 + 2
	minX, minY := o.Start.X, o.Start.Y
	maxX, maxY := o.End.X, o.End.Y
	if len(o.Points) > 0 {
		minX, minY = o.Points[0].X, o.Points[0].Y
		maxX, maxY = minX, minY
		for _, p := range o.Points[1:] {
			minX = min(minX, p.X)
			minY = min(minY, p.Y)
			maxX = max(maxX, p.X)
			maxY = max(maxY, p.Y)
		}
	}
	switch o.Tool {
	case ToolText:
		w, h := Measure(o.Text, o.FontSize)
		if o.FontSize <= 0 {
			w, h = Measure(o.Text, o.Width)
		}
		if w > 0 {
			maxX = max(maxX, minX+w)
		}
		if h > 0 {
			maxY = max(maxY, minY+h)
		}
	case ToolNumber:
		r := o.Width / 2
		if r <= 0 {
			r = 12
		}
		pad = r + 2
	case ToolMosaic:
		pad = 1
	}
	return image.Rect(
		int(math.Floor(float64(minX-pad))),
		int(math.Floor(float64(minY-pad))),
		int(math.Ceil(float64(maxX+pad))),
		int(math.Ceil(float64(maxY+pad))),
	)
}

// Op is one annotation.
type Op struct {
	Tool     Tool
	Start    Point
	End      Point
	Points   []Point
	Text     string
	Color    color.RGBA
	Width    float32
	Filled   bool
	Number   int
	FontSize float32
}

// DefaultColor is the annotation red used when a tool has no color yet.
var DefaultColor = color.RGBA{R: 0xea, G: 0x3d, B: 0x3d, A: 0xff}

// Doc is a stack of annotations over a base image of Width×Height pixels.
// The zero value is not usable; call New.
type Doc struct {
	Width  int
	Height int
	ops    []Op
	redo   []redoOp
}

// redoOp is one step in the redo history. A plain undo restores op at the
// end; a removed op goes back at its original index.
type redoOp struct {
	op      Op
	removed bool
	index   int
}

// New returns an empty document for an image of w×h pixels.
func New(w, h int) *Doc {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Doc{Width: w, Height: h}
}

// Add appends an operation and clears the redo stack.
func (d *Doc) Add(op Op) {
	if d == nil {
		return
	}
	d.ops = append(d.ops, op)
	d.redo = nil
}

// Ops returns the operations in drawing order. The slice must not be
// modified by the caller.
func (d *Doc) Ops() []Op {
	if d == nil {
		return nil
	}
	return d.ops
}

// Len returns the number of operations.
func (d *Doc) Len() int {
	if d == nil {
		return 0
	}
	return len(d.ops)
}

// Empty reports whether there is nothing to draw.
func (d *Doc) Empty() bool { return d.Len() == 0 }

// Undo removes the last operation and keeps it for Redo.
func (d *Doc) Undo() bool {
	if d == nil || len(d.ops) == 0 {
		return false
	}
	last := d.ops[len(d.ops)-1]
	d.ops = d.ops[:len(d.ops)-1]
	d.redo = append(d.redo, redoOp{op: last})
	return true
}

// Redo restores the last undone operation.
func (d *Doc) Redo() bool {
	if d == nil || len(d.redo) == 0 {
		return false
	}
	step := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	if step.removed {
		i := step.index
		if i < 0 || i > len(d.ops) {
			i = len(d.ops)
		}
		d.ops = append(d.ops, Op{})
		copy(d.ops[i+1:], d.ops[i:])
		d.ops[i] = step.op
		return true
	}
	d.ops = append(d.ops, step.op)
	return true
}

// CanUndo and CanRedo report whether the history has an entry.
func (d *Doc) CanUndo() bool { return d != nil && len(d.ops) > 0 }
func (d *Doc) CanRedo() bool { return d != nil && len(d.redo) > 0 }

// Clear removes every operation.
func (d *Doc) Clear() {
	if d == nil {
		return
	}
	d.ops = nil
	d.redo = nil
}

// RemoveAt deletes the operation at i; Redo can put it back in place.
func (d *Doc) RemoveAt(i int) bool {
	if d == nil || i < 0 || i >= len(d.ops) {
		return false
	}
	op := d.ops[i]
	d.ops = append(d.ops[:i], d.ops[i+1:]...)
	d.redo = append(d.redo, redoOp{op: op, removed: true, index: i})
	return true
}

// Hit returns the topmost operation containing p, or -1.
func (d *Doc) Hit(p Point) int {
	if d == nil {
		return -1
	}
	for i := len(d.ops) - 1; i >= 0; i-- {
		if p.In(d.ops[i].Bounds()) {
			return i
		}
	}
	return -1
}

// Clone copies the document and its history.
func (d *Doc) Clone() *Doc {
	if d == nil {
		return nil
	}
	out := &Doc{Width: d.Width, Height: d.Height}
	out.ops = append([]Op(nil), d.ops...)
	out.redo = append([]redoOp(nil), d.redo...)
	return out
}

// Crop is the part of the base image that will be exported. A zero
// rectangle means the whole image.
func (d *Doc) Crop() image.Rectangle {
	if d == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, d.Width, d.Height)
}
