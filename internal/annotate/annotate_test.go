package annotate

import (
	"image"
	"image/color"
	"testing"
)

func filled(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	return img
}

func TestHistory(t *testing.T) {
	d := New(100, 50)
	if d.Width != 100 || d.Height != 50 {
		t.Fatalf("size = %dx%d", d.Width, d.Height)
	}
	if d.CanUndo() || d.CanRedo() {
		t.Fatal("new document has history")
	}
	d.Add(Op{Tool: ToolRect})
	d.Add(Op{Tool: ToolPen})
	if d.Len() != 2 {
		t.Fatalf("len = %d", d.Len())
	}
	d.Undo()
	if d.Len() != 1 || !d.CanRedo() {
		t.Fatalf("after undo len=%d redo=%v", d.Len(), d.CanRedo())
	}
	d.Add(Op{Tool: ToolArrow})
	if d.CanRedo() {
		t.Fatal("add should clear redo")
	}
	d.Redo()
	if d.Len() != 2 {
		t.Fatalf("redo len = %d", d.Len())
	}
	clone := d.Clone()
	d.Clear()
	if clone.Len() != 2 || d.Len() != 0 {
		t.Fatal("clone or clear failed")
	}
}

func TestRenderRectAndCrop(t *testing.T) {
	if minLen(New(0, 0)) {
		t.Fatal("zero document should clamp")
	}
	d := New(20, 20)
	d.Add(Op{Tool: ToolRect, Start: Point{5, 5}, End: Point{12, 12}, Color: DefaultColor, Width: 2})
	img := Render(filled(20, 20), d, image.Rect(4, 4, 14, 14))
	if img.Bounds() != image.Rect(0, 0, 10, 10) {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	// The border at 5,5 is one pixel from the crop origin.
	c := img.RGBAAt(1, 1)
	if c.R < 100 || c.G > 100 {
		t.Fatalf("border color = %+v", c)
	}
}

func minLen(d *Doc) bool { return d.Width != 1 || d.Height != 1 }

func TestRenderShapes(t *testing.T) {
	d := New(80, 80)
	d.Add(Op{Tool: ToolEllipse, Start: Point{10, 10}, End: Point{70, 70}, Color: DefaultColor, Width: 3})
	d.Add(Op{Tool: ToolArrow, Start: Point{10, 70}, End: Point{70, 10}, Color: DefaultColor, Width: 3})
	d.Add(Op{Tool: ToolPen, Points: []Point{{X: 5, Y: 5}, {X: 20, Y: 30}, {X: 40, Y: 10}}, Color: DefaultColor, Width: 4})
	d.Add(Op{Tool: ToolHighlight, Points: []Point{{X: 10, Y: 60}, {X: 60, Y: 60}}, Color: color.RGBA{255, 255, 0, 255}, Width: 8})
	d.Add(Op{Tool: ToolNumber, Start: Point{30, 30}, Color: DefaultColor, Width: 24, Number: 2})
	img := Render(filled(80, 80), d, image.Rectangle{})
	changed := 0
	base := filled(80, 80)
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != base.Pix[i] || img.Pix[i+1] != base.Pix[i+1] {
			changed++
		}
	}
	if changed < 100 {
		t.Fatalf("too few changed pixels: %d", changed)
	}
}

func TestMosaic(t *testing.T) {
	base := filled(64, 64)
	d := New(64, 64)
	d.Add(Op{Tool: ToolMosaic, Start: Point{16, 16}, End: Point{48, 48}, Width: 8})
	img := Render(base, d, image.Rectangle{})
	// Inside a block adjacent pixels must match after pixelation.
	a := img.RGBAAt(20, 20)
	b := img.RGBAAt(21, 21)
	if a != b {
		t.Fatalf("mosaic block differs: %+v %+v", a, b)
	}
	if img.RGBAAt(2, 2) != base.RGBAAt(2, 2) {
		t.Fatal("mosaic changed pixels outside its rectangle")
	}
}

func TestText(t *testing.T) {
	d := New(120, 40)
	d.Add(Op{Tool: ToolText, Start: Point{4, 4}, Text: "Hi", Color: DefaultColor, FontSize: 18})
	img := Render(filled(120, 40), d, image.Rectangle{})
	if w, h := Measure("Hi", 18); w <= 0 || h <= 0 {
		t.Fatalf("Measure = %v %v", w, h)
	}
	found := false
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] != 0 && img.Pix[i] > 200 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("text drew nothing")
	}
}

func TestLineBoundsAndHit(t *testing.T) {
	d := New(100, 100)
	d.Add(Op{Tool: ToolLine, Start: Point{10, 10}, End: Point{60, 40}, Color: DefaultColor, Width: 3})
	d.Add(Op{Tool: ToolRect, Start: Point{70, 70}, End: Point{90, 90}, Color: DefaultColor, Width: 2})
	if i := d.Hit(Point{30, 20}); i != 0 {
		t.Fatalf("Hit near line = %d", i)
	}
	if i := d.Hit(Point{80, 80}); i != 1 {
		t.Fatalf("Hit inside rect = %d", i)
	}
	if i := d.Hit(Point{5, 95}); i != -1 {
		t.Fatalf("Hit empty spot = %d", i)
	}
	b := d.Hit(Point{60, 40})
	if b < 0 {
		t.Fatal("line end should be inside its bounds")
	}
}

func TestRemoveAtAndUndo(t *testing.T) {
	d := New(50, 50)
	d.Add(Op{Tool: ToolRect, Start: Point{1, 1}, End: Point{10, 10}})
	d.Add(Op{Tool: ToolEllipse, Start: Point{20, 20}, End: Point{30, 30}})
	if !d.RemoveAt(0) {
		t.Fatal("RemoveAt failed")
	}
	if d.Len() != 1 || !d.CanRedo() {
		t.Fatalf("after remove len=%d redo=%v", d.Len(), d.CanRedo())
	}
	if !d.Redo() {
		t.Fatal("Redo after remove failed")
	}
	if d.Len() != 2 {
		t.Fatalf("redo after remove len = %d", d.Len())
	}
	ops := d.Ops()
	if ops[0].Tool != ToolRect || ops[1].Tool != ToolEllipse {
		t.Fatalf("removed op restored out of order: %v", ops)
	}
	if d.RemoveAt(5) {
		t.Fatal("RemoveAt out of range succeeded")
	}
}

func TestLineRenders(t *testing.T) {
	base := filled(60, 60)
	d := New(60, 60)
	d.Add(Op{Tool: ToolLine, Start: Point{5, 50}, End: Point{50, 5}, Color: DefaultColor, Width: 3})
	img := Render(base, d, image.Rectangle{})
	changed := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != base.Pix[i] || img.Pix[i+1] != base.Pix[i+1] {
			changed++
		}
	}
	if changed < 20 {
		t.Fatalf("line drew too little: %d", changed)
	}
}

func TestPointIn(t *testing.T) {
	r := image.Rect(10, 10, 20, 20)
	if !(Point{10, 10}).In(r) || (Point{20, 20}).In(r) || (Point{5, 15}).In(r) {
		t.Fatal("Point.In is wrong")
	}
}
