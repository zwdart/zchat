package capture

import (
	"image"
	"testing"
)

func TestDisplayConversion(t *testing.T) {
	d := Display{
		Bounds:       image.Rect(100, 50, 300, 250),
		NativeBounds: image.Rect(200, 100, 600, 500),
		Scale:        2,
	}
	if got := d.DIPToPixel(image.Pt(150, 80)); got != (image.Point{X: 100, Y: 60}) {
		t.Fatalf("DIPToPixel = %v", got)
	}
	if got := d.PixelToDIP(image.Pt(100, 60)); got != (image.Point{X: 150, Y: 80}) {
		t.Fatalf("PixelToDIP = %v", got)
	}
	sub := d.SubDisplay()
	if sub.Bounds != image.Rect(0, 0, 200, 200) || sub.NativeBounds != image.Rect(0, 0, 400, 400) {
		t.Fatalf("SubDisplay = %+v", sub)
	}
}

func TestDisplayAt(t *testing.T) {
	displays := []Display{
		{Bounds: image.Rect(0, 0, 100, 100)},
		{Bounds: image.Rect(100, 0, 200, 100)},
	}
	if d, ok := DisplayAt(displays, image.Pt(50, 50)); !ok || d.Bounds.Min.X != 0 {
		t.Fatalf("inside = %v %v", d, ok)
	}
	if d, ok := DisplayAt(displays, image.Pt(250, 50)); !ok || d.Bounds.Min.X != 100 {
		t.Fatalf("nearest = %v %v", d, ok)
	}
	if _, ok := DisplayAt(nil, image.Pt(0, 0)); ok {
		t.Fatal("empty list matched")
	}
}

func TestRectHelpers(t *testing.T) {
	if got := NormalizeRect(image.Pt(10, 20), image.Pt(2, 4)); got != image.Rect(2, 4, 10, 20) {
		t.Fatalf("NormalizeRect = %v", got)
	}
	if r, ok := ClipRect(image.Rect(0, 0, 10, 10), image.Rect(5, 5, 20, 20)); !ok || r != image.Rect(5, 5, 10, 10) {
		t.Fatalf("ClipRect = %v %v", r, ok)
	}
	if _, ok := ClipRect(image.Rect(0, 0, 4, 4), image.Rect(5, 5, 9, 9)); ok {
		t.Fatal("disjoint rectangles overlapped")
	}
}

func TestFrameCalibrate(t *testing.T) {
	f := FakeFrame("1", image.Rect(0, 0, 100, 50), 1, 200, 100)
	f.Calibrate()
	if f.Scale != 2 {
		t.Fatalf("scale = %v", f.Scale)
	}
}

func TestWindowHelpers(t *testing.T) {
	windows := []WindowInfo{
		{ID: 1, Title: "Back", Bounds: image.Rect(0, 0, 100, 100), ZOrder: 1},
		{ID: 2, Title: "Front", Bounds: image.Rect(10, 10, 60, 60), ZOrder: 0},
		{ID: 3, Title: "Empty", ZOrder: 2},
		{ID: 4, Title: "zchat", Bounds: image.Rect(0, 0, 50, 50), ZOrder: 3},
		{ID: 2, Title: "Duplicate", Bounds: image.Rect(0, 0, 50, 50), ZOrder: 4},
	}
	got := FilterWindows(windows, image.Rect(0, 0, 200, 200), "zchat")
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("FilterWindows = %+v", got)
	}
	if w, ok := WindowAt(windows, image.Pt(20, 20)); !ok || w.ID != 2 {
		t.Fatalf("WindowAt = %+v %v", w, ok)
	}
}

func TestFakeBackend(t *testing.T) {
	b := &FakeBackend{
		DisplayList: []Display{{ID: "a", Bounds: image.Rect(0, 0, 10, 10), NativeBounds: image.Rect(0, 0, 10, 10), Scale: 1}},
		FrameList:   []Frame{FakeFrame("a", image.Rect(0, 0, 10, 10), 1, 10, 10)},
	}
	if _, err := b.Displays(); err != nil {
		t.Fatal(err)
	}
	frames, err := b.Capture(nil, Options{DisplayID: "a"})
	if err != nil || len(frames) != 1 || b.CaptureCalls != 1 {
		t.Fatalf("Capture = %v %v %d", frames, err, b.CaptureCalls)
	}
	if _, err := b.Capture(nil, Options{DisplayID: "b"}); err != ErrNoDisplay {
		t.Fatalf("missing display error = %v", err)
	}
}

func TestEncodeAndClone(t *testing.T) {
	src := FakeFrame("a", image.Rect(0, 0, 4, 2), 1, 4, 2).Image
	clone := CloneRGBA(src)
	clone.Pix[0] = 7
	if src.Pix[0] == 7 {
		t.Fatal("CloneRGBA shared pixels")
	}
	data, err := EncodePNG(src, 0)
	if err != nil || len(data) == 0 {
		t.Fatalf("EncodePNG = %d %v", len(data), err)
	}
}
