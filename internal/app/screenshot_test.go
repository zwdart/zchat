package app

import (
	"image"
	"testing"

	"xin.dart.zchat/internal/annotate"
	"xin.dart.zchat/internal/capture"
)

func TestCaptureNormalize(t *testing.T) {
	cases := []struct {
		a, b image.Point
		want image.Rectangle
	}{
		{image.Pt(10, 20), image.Pt(30, 40), image.Rect(10, 20, 30, 40)},
		{image.Pt(30, 40), image.Pt(10, 20), image.Rect(10, 20, 30, 40)},
		{image.Pt(10, 40), image.Pt(30, 20), image.Rect(10, 20, 30, 40)},
	}
	for _, c := range cases {
		if got := captureNormalize(c.a, c.b); got != c.want {
			t.Fatalf("captureNormalize(%v, %v) = %v", c.a, c.b, got)
		}
	}
}

func TestClampMoved(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	r := image.Rect(70, 60, 90, 70)
	got := clampMoved(r.Add(image.Pt(30, 30)), bounds)
	if got.Max.X > bounds.Max.X || got.Max.Y > bounds.Max.Y {
		t.Fatalf("moved rect escaped bounds: %v", got)
	}
	if got.Dx() != r.Dx() || got.Dy() != r.Dy() {
		t.Fatalf("clamping changed size: %v", got)
	}
	got = clampMoved(image.Rect(-20, -10, 20, 10), bounds)
	if got.Min.X != 0 || got.Min.Y != 0 {
		t.Fatalf("rect not moved into bounds: %v", got)
	}
}

func TestResizeSelectionHandles(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	orig := image.Rect(20, 20, 80, 80)
	if got := resizeSelection(orig, 4, image.Pt(95, 95), bounds); got != image.Rect(20, 20, 95, 95) {
		t.Fatalf("bottom-right resize = %v", got)
	}
	if got := resizeSelection(orig, 0, image.Pt(30, 25), bounds); got != image.Rect(30, 25, 80, 80) {
		t.Fatalf("top-left resize = %v", got)
	}
	// Dragging the left edge past the right edge flips the rectangle instead
	// of making it negative.
	got := resizeSelection(orig, 7, image.Pt(90, 50), bounds)
	if got.Dx() <= 0 || got.Max.X != 90 {
		t.Fatalf("flipped resize = %v", got)
	}
	// The pointer is clamped to the screenshot.
	if got := resizeSelection(orig, 4, image.Pt(500, 500), bounds); got.Max != bounds.Max {
		t.Fatalf("unclamped resize = %v", got)
	}
}

func TestSnapPointAndMoved(t *testing.T) {
	s := &shotSession{
		scale:       1,
		doc:         annotate.New(200, 120),
		windowRects: []image.Rectangle{image.Rect(40, 30, 160, 100)},
	}
	if got := s.snapPoint(image.Pt(43, 27)); got != image.Pt(40, 30) {
		t.Fatalf("snapPoint = %v", got)
	}
	if got := s.snapPoint(image.Pt(100, 60)); got != image.Pt(100, 60) {
		t.Fatalf("snapPoint moved a distant point: %v", got)
	}
	r := image.Rect(11, 8, 31, 26)
	if got := s.snapMoved(r); got.Min != image.Pt(11, 12) {
		t.Fatalf("snapMoved = %v", got)
	}
}

func TestNudgeAndResizeSelection(t *testing.T) {
	s := &shotSession{
		scale: 1,
		doc:   annotate.New(100, 100),
		sel:   image.Rect(10, 10, 30, 30),
	}
	s.moveSelectionBy(-20, 5)
	if s.sel != image.Rect(0, 15, 20, 35) {
		t.Fatalf("move = %v", s.sel)
	}
	s.moveSelectionBy(200, 200)
	if s.sel.Max != image.Pt(100, 100) {
		t.Fatalf("move escaped = %v", s.sel)
	}
	s.sel = image.Rect(10, 10, 30, 30)
	s.resizeSelectionBy(5, -5)
	if s.sel != image.Rect(10, 5, 35, 30) {
		t.Fatalf("resize = %v", s.sel)
	}
}

func TestLocalizeWindows(t *testing.T) {
	frame := image.Rect(100, 50, 300, 250)
	wins := []capture.WindowInfo{
		{ID: 1, Title: "a", Bounds: image.Rect(110, 60, 210, 160)},
		{ID: 2, Title: "tiny", Bounds: image.Rect(100, 50, 100, 51)},
	}
	got := localizeWindows(wins, frame)
	if len(got) != 1 {
		t.Fatalf("localizeWindows kept %d windows", len(got))
	}
	if got[0].Bounds != image.Rect(10, 10, 110, 110) {
		t.Fatalf("local bounds = %v", got[0].Bounds)
	}
}

func TestWindowRect(t *testing.T) {
	s := &shotSession{
		scale: 2,
		frame: capture.Frame{Bounds: image.Rect(100, 50, 300, 250)},
		doc:   annotate.New(400, 400),
	}
	r := s.windowRect(capture.WindowInfo{Bounds: image.Rect(110, 60, 210, 160)})
	if r != image.Rect(20, 20, 220, 220) {
		t.Fatalf("windowRect = %v", r)
	}
}

func TestWindowAtUsesTopmost(t *testing.T) {
	wins := []capture.WindowInfo{
		{ID: 1, ZOrder: 2, Bounds: image.Rect(0, 0, 100, 100)},
		{ID: 2, ZOrder: 1, Bounds: image.Rect(20, 20, 80, 80)},
	}
	w, ok := capture.WindowAt(wins, image.Pt(40, 40))
	if !ok || w.ID != 2 {
		t.Fatalf("WindowAt = %+v, %v", w, ok)
	}
}
