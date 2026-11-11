//go:build darwin

package capture

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

// CoreGraphics display types.
type cgPoint struct{ X, Y float64 }
type cgSize struct{ Width, Height float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

var (
	cgOnce sync.Once
	cg     cgFuncs
	cgErr  error
)

type cgFuncs struct {
	getActiveDisplayList   func(max uint32, displays *uint32, count *uint32) int32
	displayBounds          func(display uint32) cgRect
	displayPixelsWide      func(display uint32) uintptr
	displayPixelsHigh      func(display uint32) uintptr
	mainDisplayID          func() uint32
	preflightScreenCapture func() bool
	requestScreenCapture   func() bool
}

func loadCoreGraphics() (cgFuncs, error) {
	cgOnce.Do(func() {
		handle, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", 2)
		if err != nil {
			cgErr = err
			return
		}
		sym := func(name string, fn any) {
			if cgErr != nil {
				return
			}
			addr, err := purego.Dlsym(handle, name)
			if err != nil {
				return
			}
			purego.RegisterFunc(fn, addr)
		}
		sym("CGGetActiveDisplayList", &cg.getActiveDisplayList)
		sym("CGDisplayBounds", &cg.displayBounds)
		sym("CGDisplayPixelsWide", &cg.displayPixelsWide)
		sym("CGDisplayPixelsHigh", &cg.displayPixelsHigh)
		sym("CGMainDisplayID", &cg.mainDisplayID)
		sym("CGPreflightScreenCaptureAccess", &cg.preflightScreenCapture)
		sym("CGRequestScreenCaptureAccess", &cg.requestScreenCapture)
	})
	return cg, cgErr
}

// darwinBackend enumerates displays with CoreGraphics and captures their
// pixels with the system's screencapture tool. The tool needs no cgo and
// handles Retina scaling and the screen-recording permission prompt.
type darwinBackend struct {
	mu sync.Mutex
}

func platformBackend() Backend { return &darwinBackend{} }

func (b *darwinBackend) Capabilities() Capability {
	return Capability{
		PreCaptureOverlay: true,
		WindowDetection:   true,
		AllDisplays:       true,
		Cursor:            true,
		GlobalShortcut:    true,
		Notes:             []string{"macOS asks for Screen Recording permission the first time."},
	}
}

func (b *darwinBackend) Permission() Permission {
	g, err := loadCoreGraphics()
	if err != nil || g.preflightScreenCapture == nil {
		return PermissionUnavailable
	}
	if g.preflightScreenCapture() {
		return PermissionGranted
	}
	return PermissionNotDetermined
}

func (b *darwinBackend) RequestPermission(context.Context) Permission {
	g, err := loadCoreGraphics()
	if err != nil || g.requestScreenCapture == nil {
		return PermissionUnavailable
	}
	if g.requestScreenCapture() {
		return PermissionGranted
	}
	return PermissionDenied
}

func (b *darwinBackend) Displays() ([]Display, error) {
	g, err := loadCoreGraphics()
	if err != nil || g.getActiveDisplayList == nil {
		// Without CoreGraphics there is no trustworthy display geometry.
		return nil, fmt.Errorf("%w: CoreGraphics: %v", ErrCaptureFailed, err)
	}
	var ids [64]uint32
	var count uint32
	if r := g.getActiveDisplayList(uint32(len(ids)), &ids[0], &count); r != 0 {
		return nil, fmt.Errorf("%w: CGGetActiveDisplayList: %d", ErrCaptureFailed, r)
	}
	if count == 0 {
		return nil, ErrNoDisplay
	}
	main := uint32(0)
	if g.mainDisplayID != nil {
		main = g.mainDisplayID()
	}
	out := make([]Display, 0, count)
	for i := uint32(0); i < count && i < uint32(len(ids)); i++ {
		id := ids[i]
		r := g.displayBounds(id)
		w := int(r.Size.Width)
		h := int(r.Size.Height)
		if w <= 0 || h <= 0 {
			continue
		}
		pxW, pxH := w, h
		if g.displayPixelsWide != nil {
			if v := int(g.displayPixelsWide(id)); v > 0 {
				pxW = v
			}
		}
		if g.displayPixelsHigh != nil {
			if v := int(g.displayPixelsHigh(id)); v > 0 {
				pxH = v
			}
		}
		scale := float64(pxW) / float64(w)
		if scale <= 0 {
			scale = 1
		}
		out = append(out, Display{
			ID:           strconv.FormatUint(uint64(id), 10),
			Name:         "Display " + strconv.Itoa(int(i)+1),
			Bounds:       image.Rect(int(r.Origin.X), int(r.Origin.Y), int(r.Origin.X)+w, int(r.Origin.Y)+h),
			NativeBounds: image.Rect(int(r.Origin.X)*int(scale), int(r.Origin.Y)*int(scale), int(r.Origin.X)*int(scale)+pxW, int(r.Origin.Y)*int(scale)+pxH),
			Scale:        scale,
			Primary:      id == main,
		})
	}
	if len(out) == 0 {
		return nil, ErrNoDisplay
	}
	return out, nil
}

func (b *darwinBackend) Capture(ctx context.Context, opts Options) ([]Frame, error) {
	displays, err := b.Displays()
	if err != nil {
		return nil, err
	}
	var selected []Display
	for _, d := range displays {
		if opts.AllDisplays || opts.DisplayID == "" || d.ID == opts.DisplayID {
			selected = append(selected, d)
		}
	}
	if len(selected) == 0 {
		return nil, ErrNoDisplay
	}
	frames := make([]Frame, 0, len(selected))
	var firstErr error
	for _, d := range selected {
		frame, err := b.captureDisplay(ctx, d, opts.IncludeCursor && !opts.AllDisplays)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrCaptureFailed
	}
	return frames, nil
}

func (b *darwinBackend) captureDisplay(ctx context.Context, d Display, cursor bool) (Frame, error) {
	dir, err := os.MkdirTemp("", "zchat-capture-")
	if err != nil {
		return Frame{}, fmt.Errorf("%w: %v", ErrCaptureFailed, err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "shot.png")
	args := []string{"-x", "-t", "png",
		"-R" + strconv.Itoa(d.Bounds.Min.X) + "," + strconv.Itoa(d.Bounds.Min.Y) + "," +
			strconv.Itoa(d.Bounds.Dx()) + "," + strconv.Itoa(d.Bounds.Dy())}
	if cursor {
		args = append(args, "-C")
	}
	args = append(args, path)
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/usr/sbin/screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Frame{}, fmt.Errorf("%w: screencapture: %v %s", ErrCaptureFailed, err, strings.TrimSpace(string(out)))
	}
	f, err := os.Open(path)
	if err != nil {
		return Frame{}, fmt.Errorf("%w: %v", ErrCaptureFailed, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return Frame{}, fmt.Errorf("%w: %v", ErrCaptureFailed, err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(img.Bounds())
		drawOpaque(rgba, img)
	}
	frame := Frame{DisplayID: d.ID, Bounds: d.Bounds, Scale: d.Scale, Image: rgba, CapturedAt: time.Now()}
	frame.Calibrate()
	return frame, nil
}

func (b *darwinBackend) Windows(ctx context.Context, displayID string) ([]WindowInfo, error) {
	// The scripting bridge is the only cgo-free way to read other apps'
	// windows on macOS, and it is not always present. Returning an empty
	// list keeps the overlay usable without window detection.
	return nil, nil
}

// drawOpaque copies src into dst, ignoring alpha as a screen capture would.
func drawOpaque(dst *image.RGBA, src image.Image) {
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := src.At(x, y).RGBA()
			i := dst.PixOffset(x-b.Min.X, y-b.Min.Y)
			dst.Pix[i+0] = uint8(r >> 8)
			dst.Pix[i+1] = uint8(g >> 8)
			dst.Pix[i+2] = uint8(bl >> 8)
			dst.Pix[i+3] = 255
		}
	}
}
