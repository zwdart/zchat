// Package capture takes screenshots of the desktop and describes the
// displays and windows it can see.
//
// The package has no dependency on the UI toolkit or on cgo. Each platform
// supplies a Backend behind build tags; unsupported systems get a backend
// that reports ErrUnsupported so the chat feature keeps working.
package capture

import (
	"context"
	"errors"
	"image"
	"math"
	"sync"
	"time"
)

// Errors reported by the platform backends.
var (
	ErrPermissionDenied = errors.New("screen capture permission denied")
	ErrUnsupported      = errors.New("screen capture unsupported")
	ErrNoDisplay        = errors.New("no display")
	ErrCanceled         = errors.New("screen capture canceled")
	ErrCaptureFailed    = errors.New("screen capture failed")
	ErrTooLarge         = errors.New("screen capture too large")
)

// Permission is the state of the system's screen-recording permission.
type Permission uint8

const (
	// PermissionNotRequired is used where the desktop has no such permission.
	PermissionNotRequired Permission = iota
	// PermissionGranted means screen capture may proceed.
	PermissionGranted
	// PermissionDenied means the user refused, or revoked, the permission.
	PermissionDenied
	// PermissionNotDetermined means the system has not asked the user yet.
	PermissionNotDetermined
	// PermissionUnavailable means the check could not be made.
	PermissionUnavailable
)

func (p Permission) String() string {
	switch p {
	case PermissionGranted:
		return "granted"
	case PermissionDenied:
		return "denied"
	case PermissionNotDetermined:
		return "not determined"
	case PermissionUnavailable:
		return "unavailable"
	default:
		return "not required"
	}
}

// Display is one monitor. Bounds is in DIPs in the same coordinate space as
// the window toolkit (origin at the top-left of the primary display, which
// may be negative). NativeBounds is in the platform's physical pixels.
type Display struct {
	ID           string
	Name         string
	Bounds       image.Rectangle
	NativeBounds image.Rectangle
	Scale        float64
	Primary      bool
}

// Frame is one display's pixels. Bounds matches Display.Bounds and Image has
// the physical pixels of the display. The image's origin is the top-left of
// Bounds.
type Frame struct {
	DisplayID  string
	Bounds     image.Rectangle
	Scale      float64
	Image      *image.RGBA
	CapturedAt time.Time
}

// Calibrate makes the frame's scale match the pixels actually returned. A
// driver or compositor may round a display's size, and every later
// coordinate conversion should use the pixels that exist.
func (f *Frame) Calibrate() {
	if f == nil || f.Image == nil {
		return
	}
	w := f.Bounds.Dx()
	if w <= 0 {
		w = f.Image.Bounds().Dx()
		f.Bounds = image.Rect(f.Bounds.Min.X, f.Bounds.Min.Y, f.Bounds.Min.X+w, f.Bounds.Min.Y+f.Image.Bounds().Dy())
	}
	f.Scale = float64(f.Image.Bounds().Dx()) / float64(w)
	if f.Scale <= 0 || math.IsNaN(f.Scale) || math.IsInf(f.Scale, 0) {
		f.Scale = 1
	}
}

// WindowInfo is a top-level window in DIP coordinates.
type WindowInfo struct {
	ID      uint64
	Title   string
	AppName string
	Bounds  image.Rectangle
	Layer   int
	ZOrder  int
}

// Capability describes what a backend can do, so the app can explain a
// degraded experience instead of failing silently.
type Capability struct {
	PreCaptureOverlay bool
	WindowDetection   bool
	AllDisplays       bool
	Cursor            bool
	GlobalShortcut    bool
	Notes             []string
}

// Options selects what to capture.
type Options struct {
	DisplayID     string
	IncludeCursor bool
	AllDisplays   bool
}

// Backend is the platform's screen capture implementation.
type Backend interface {
	Displays() ([]Display, error)
	Capabilities() Capability
	Capture(ctx context.Context, opts Options) ([]Frame, error)
	Windows(ctx context.Context, displayID string) ([]WindowInfo, error)
	Permission() Permission
	RequestPermission(ctx context.Context) Permission
}

var (
	backendOnce sync.Once
	backend     Backend
)

// Default returns the backend for the current platform.
func Default() Backend {
	backendOnce.Do(func() {
		backend = platformBackend()
		if backend == nil {
			backend = unsupportedBackend{}
		}
	})
	return backend
}

// DIPToPixel converts a DIP point on d to a pixel offset in a frame.
func (d Display) DIPToPixel(p image.Point) image.Point {
	s := d.Scale
	if s <= 0 {
		s = 1
	}
	return image.Pt(
		int(math.Round(float64(p.X-d.Bounds.Min.X)*s)),
		int(math.Round(float64(p.Y-d.Bounds.Min.Y)*s)),
	)
}

// PixelToDIP converts a pixel offset in d's frame to a DIP point.
func (d Display) PixelToDIP(p image.Point) image.Point {
	s := d.Scale
	if s <= 0 {
		s = 1
	}
	return image.Pt(
		d.Bounds.Min.X+int(math.Round(float64(p.X)/s)),
		d.Bounds.Min.Y+int(math.Round(float64(p.Y)/s)),
	)
}

// SubDisplay returns d with its DIP and native origins removed, so the
// rectangle starts at (0,0). It is useful when describing a display-local
// coordinate space to the UI.
func (d Display) SubDisplay() Display {
	d.Bounds = image.Rect(0, 0, d.Bounds.Dx(), d.Bounds.Dy())
	d.NativeBounds = image.Rect(0, 0, d.NativeBounds.Dx(), d.NativeBounds.Dy())
	return d
}

// DisplayAt returns the display containing p, or the nearest display when p
// is in a gap between displays.
func DisplayAt(displays []Display, p image.Point) (Display, bool) {
	if len(displays) == 0 {
		return Display{}, false
	}
	var best Display
	bestDist := int64(math.MaxInt64)
	for _, d := range displays {
		if p.In(d.Bounds) {
			return d, true
		}
		dx, dy := axisGap(d.Bounds.Min.X, d.Bounds.Max.X, p.X), axisGap(d.Bounds.Min.Y, d.Bounds.Max.Y, p.Y)
		dist := int64(dx)*int64(dx) + int64(dy)*int64(dy)
		if dist < bestDist {
			best, bestDist = d, dist
		}
	}
	return best, true
}

func axisGap(lo, hi, v int) int {
	switch {
	case v < lo:
		return lo - v
	case v > hi:
		return v - hi
	default:
		return 0
	}
}

// ClipRect intersects r with bounds and reports whether at least one pixel
// remains.
func ClipRect(r, bounds image.Rectangle) (image.Rectangle, bool) {
	r = r.Intersect(bounds)
	return r, r.Dx() >= 1 && r.Dy() >= 1
}

// NormalizeRect makes a rectangle out of two drag points, whichever order
// they came in.
func NormalizeRect(a, b image.Point) image.Rectangle {
	if a.X > b.X {
		a.X, b.X = b.X, a.X
	}
	if a.Y > b.Y {
		a.Y, b.Y = b.Y, a.Y
	}
	return image.Rect(a.X, a.Y, b.X, b.Y)
}

// unsupportedBackend is returned on platforms without a native backend.
type unsupportedBackend struct{}

func (unsupportedBackend) Displays() ([]Display, error) { return nil, ErrUnsupported }
func (unsupportedBackend) Capabilities() Capability     { return Capability{} }
func (unsupportedBackend) Capture(context.Context, Options) ([]Frame, error) {
	return nil, ErrUnsupported
}
func (unsupportedBackend) Windows(context.Context, string) ([]WindowInfo, error) {
	return nil, ErrUnsupported
}
func (unsupportedBackend) Permission() Permission { return PermissionUnavailable }
func (unsupportedBackend) RequestPermission(context.Context) Permission {
	return PermissionUnavailable
}
