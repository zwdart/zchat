//go:build linux

package capture

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// waylandBackend uses the desktop's screenshot tool. Wayland gives ordinary
// clients no way to read the screen or to place a full-screen overlay, so
// zchat captures first and edits in its own window afterwards.
type waylandBackend struct {
	mu       sync.Mutex
	cached   *image.RGBA
	cachedAt time.Time
	tool     string
}

func (b *waylandBackend) Capabilities() Capability {
	return Capability{
		PreCaptureOverlay: false,
		WindowDetection:   false,
		AllDisplays:       true,
		Cursor:            false,
		GlobalShortcut:    false,
		Notes: []string{
			"Wayland cannot show a selection overlay before the capture; zchat captures the screen and opens the editor instead.",
			"Install grim, gnome-screenshot or spectacle for screen capture.",
		},
	}
}

func (b *waylandBackend) Permission() Permission { return PermissionNotRequired }

func (b *waylandBackend) RequestPermission(context.Context) Permission {
	return PermissionNotRequired
}

func (b *waylandBackend) Displays() ([]Display, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	img, err := b.grab(context.Background())
	if err != nil {
		return nil, err
	}
	return displayForImage(img), nil
}

func displayForImage(img *image.RGBA) []Display {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w < 1 || h < 1 {
		return nil
	}
	return []Display{{
		ID:           "wayland",
		Name:         "Screen",
		Bounds:       image.Rect(0, 0, w, h),
		NativeBounds: image.Rect(0, 0, w, h),
		Scale:        1,
		Primary:      true,
	}}
}

func (b *waylandBackend) Capture(ctx context.Context, opts Options) ([]Frame, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var img *image.RGBA
	if b.cached != nil && time.Since(b.cachedAt) < 2*time.Second {
		img = b.cached
		b.cached = nil
	} else {
		var err error
		img, err = b.grab(ctx)
		if err != nil {
			return nil, err
		}
	}
	cal := displayForImage(img)
	if len(cal) == 0 {
		return nil, ErrNoDisplay
	}
	d := cal[0]
	frame := Frame{DisplayID: d.ID, Bounds: d.Bounds, Scale: 1, Image: img, CapturedAt: time.Now()}
	frame.Calibrate()
	return []Frame{frame}, nil
}

func (b *waylandBackend) Windows(context.Context, string) ([]WindowInfo, error) {
	return nil, nil
}

// grab runs the first available capture command and decodes its PNG.
func (b *waylandBackend) grab(ctx context.Context) (*image.RGBA, error) {
	type tool struct {
		name string
		run  func(context.Context, string) error
	}
	grim := tool{name: "grim", run: func(ctx context.Context, path string) error {
		cmd := exec.CommandContext(ctx, "grim", "-t", "png", "-")
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		return os.WriteFile(path, out, 0o600)
	}}
	gnome := tool{name: "gnome-screenshot", run: func(ctx context.Context, path string) error {
		return exec.CommandContext(ctx, "gnome-screenshot", "-f", path).Run()
	}}
	spectacle := tool{name: "spectacle", run: func(ctx context.Context, path string) error {
		// -f captures the whole screen; without it Spectacle reuses the last
		// interactive mode, which can be a single window or a region and so
		// drops the desktop panel and menu bars.
		return exec.CommandContext(ctx, "spectacle", "-b", "-n", "-f", "-o", path).Run()
	}}
	// ImageMagick's "import -window root" is deliberately missing: in a
	// Wayland session it reads only the XWayland root, so native Wayland
	// windows, menus and the desktop panel are absent from the result.
	tools := []tool{grim, gnome, spectacle}

	dir, err := os.MkdirTemp("", "zchat-wayland-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCaptureFailed, err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "shot.png")
	var lastErr error
	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			continue
		}
		runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := t.run(runCtx, path)
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", t.name, err)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) < 8 {
			lastErr = fmt.Errorf("%s: no image", t.name)
			continue
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", t.name, err)
			continue
		}
		b.tool = t.name
		return toRGBA(img), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no screenshot tool found (install grim, gnome-screenshot or spectacle)")
	}
	return nil, fmt.Errorf("%w: %v", ErrCaptureFailed, lastErr)
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	dst := image.NewRGBA(img.Bounds())
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			i := dst.PixOffset(x-b.Min.X, y-b.Min.Y)
			dst.Pix[i+0] = uint8(r >> 8)
			dst.Pix[i+1] = uint8(g >> 8)
			dst.Pix[i+2] = uint8(bl >> 8)
			dst.Pix[i+3] = 255
		}
	}
	return dst
}
