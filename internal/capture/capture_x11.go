//go:build linux

package capture

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	xZPixmap       = 2
	xAllPlanes     = ^uintptr(0)
	xAnyProperty   = 0
	xSuccess       = 0
	xMapViewable   = 2
	xPropertyRead  = 0
	xArrayLongSize = 4
)

type x11API struct {
	lib uintptr
	xi  uintptr

	openDisplay           func(name uintptr) uintptr
	closeDisplay          func(dpy uintptr) int32
	defaultScreen         func(dpy uintptr) int32
	rootWindow            func(dpy uintptr, screen int32) uintptr
	displayWidth          func(dpy uintptr, screen int32) int32
	displayHeight         func(dpy uintptr, screen int32) int32
	resourceManagerString func(dpy uintptr) uintptr
	internAtom            func(dpy uintptr, name string, onlyIfExists int32) uintptr
	getWindowProperty     func(dpy, w, property uintptr, offset, length int64, del int32, reqType uintptr,
		actualType *uintptr, actualFormat *int32, nitems *uint64, bytesAfter *uint64, prop *uintptr) int32
	free             func(data uintptr) int32
	getGeometry      func(dpy, drawable uintptr, root *uintptr, x, y *int32, w, h *uint32, border *uint32, depth *uint32) int32
	translateCoords  func(dpy, src, dst uintptr, srcX, srcY int32, dstX, dstY *int32, child *uintptr) int32
	getWindowAttrs   func(dpy, w uintptr, attrs unsafe.Pointer) int32
	getImage         func(dpy, drawable uintptr, x, y int32, w, h uint32, planeMask uintptr, format int32) uintptr
	destroyImage     func(ximage uintptr) int32
	xineramaIsActive func(dpy uintptr) int32
	xineramaScreens  func(dpy uintptr, count *int32) uintptr
}

type x11Backend struct {
	once sync.Once
	api  x11API
	err  error

	mu       sync.Mutex
	dpy      uintptr
	displays []Display
	scale    float64
}

func newX11Backend() Backend { return &x11Backend{} }

type xineramaScreenInfo struct {
	ScreenNumber int32
	XOrg         int16
	YOrg         int16
	Width        int16
	Height       int16
}

// xImage mirrors the leading fields of Xlib's XImage. Only these fields are
// read; the function table after them is owned by Xlib.
type xImage struct {
	Width        int32
	Height       int32
	XOffset      int32
	Format       int32
	Data         uintptr
	ByteOrder    int32
	BitmapUnit   int32
	BitOrder     int32
	BitmapPad    int32
	Depth        int32
	BytesPerLine int32
	BitsPerPixel int32
	RedMask      uint64
	GreenMask    uint64
	BlueMask     uint64
}

var (
	x11Once sync.Once
	x11APIv x11API
	x11Err  error
)

func loadX11() (x11API, error) {
	x11Once.Do(func() {
		lib, err := purego.Dlopen("libX11.so.6", 2)
		if err != nil {
			lib, err = purego.Dlopen("libX11.so", 2)
		}
		if err != nil {
			x11Err = err
			return
		}
		x11APIv.lib = lib
		sym := func(name string, fn any) {
			addr, err := purego.Dlsym(lib, name)
			if err != nil {
				return
			}
			purego.RegisterFunc(fn, addr)
		}
		sym("XOpenDisplay", &x11APIv.openDisplay)
		sym("XCloseDisplay", &x11APIv.closeDisplay)
		sym("XDefaultScreen", &x11APIv.defaultScreen)
		sym("XRootWindow", &x11APIv.rootWindow)
		sym("XDisplayWidth", &x11APIv.displayWidth)
		sym("XDisplayHeight", &x11APIv.displayHeight)
		sym("XResourceManagerString", &x11APIv.resourceManagerString)
		sym("XInternAtom", &x11APIv.internAtom)
		sym("XGetWindowProperty", &x11APIv.getWindowProperty)
		sym("XFree", &x11APIv.free)
		sym("XGetGeometry", &x11APIv.getGeometry)
		sym("XTranslateCoordinates", &x11APIv.translateCoords)
		sym("XGetWindowAttributes", &x11APIv.getWindowAttrs)
		sym("XGetImage", &x11APIv.getImage)
		sym("XDestroyImage", &x11APIv.destroyImage)

		if x11APIv.openDisplay == nil || x11APIv.getImage == nil || x11APIv.getWindowProperty == nil ||
			x11APIv.internAtom == nil || x11APIv.getGeometry == nil || x11APIv.translateCoords == nil {
			x11Err = fmt.Errorf("%w: libX11 is missing symbols", ErrUnsupported)
			return
		}
		if xi, err := purego.Dlopen("libXinerama.so.1", 2); err == nil {
			x11APIv.xi = xi
			if addr, err := purego.Dlsym(xi, "XineramaIsActive"); err == nil {
				purego.RegisterFunc(&x11APIv.xineramaIsActive, addr)
			}
			if addr, err := purego.Dlsym(xi, "XineramaQueryScreens"); err == nil {
				purego.RegisterFunc(&x11APIv.xineramaScreens, addr)
			}
		}
	})
	return x11APIv, x11Err
}

func (b *x11Backend) init() (x11API, error) {
	b.once.Do(func() {
		api, err := loadX11()
		if err != nil {
			b.err = err
			return
		}
		b.api = api
		b.dpy = api.openDisplay(0)
		if b.dpy == 0 {
			b.err = fmt.Errorf("%w: cannot open the X11 display", ErrUnsupported)
			return
		}
		b.scale = x11Scale(api, b.dpy)
		b.loadDisplays()
	})
	if b.err != nil {
		return b.api, b.err
	}
	return b.api, nil
}

func x11Scale(api x11API, dpy uintptr) float64 {
	// GDK and Qt scale in integer steps; the X resource is fractional DPI.
	if v := os.Getenv("GDK_SCALE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	if api.resourceManagerString != nil {
		ptr := api.resourceManagerString(dpy)
		if ptr != 0 {
			res := cString(ptr)
			for _, line := range strings.Split(res, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToLower(line), "xft.dpi:") {
					val := strings.TrimSpace(line[len("xft.dpi:"):])
					if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
						return f / 96
					}
				}
			}
		}
	}
	return 1
}

func cString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); i < 1<<20; i++ {
		c := *(*byte)(unsafe.Pointer(ptr + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func (b *x11Backend) loadDisplays() {
	api := b.api
	screen := api.defaultScreen(b.dpy)
	scale := b.scale
	if scale <= 0 {
		scale = 1
	}
	if api.xineramaIsActive != nil && api.xineramaScreens != nil && api.xineramaIsActive(b.dpy) != 0 {
		var count int32
		ptr := api.xineramaScreens(b.dpy, &count)
		if ptr != 0 {
			defer api.free(ptr)
			if count > 0 && count < 64 {
				infos := unsafe.Slice((*xineramaScreenInfo)(unsafe.Pointer(ptr)), count)
				for i, info := range infos {
					if info.Width <= 0 || info.Height <= 0 {
						continue
					}
					x, y := int(info.XOrg), int(info.YOrg)
					w, h := int(info.Width), int(info.Height)
					b.displays = append(b.displays, Display{
						ID:   strconv.Itoa(int(info.ScreenNumber)),
						Name: "Screen " + strconv.Itoa(i+1),
						Bounds: image.Rect(
							int(math.Round(float64(x)/scale)), int(math.Round(float64(y)/scale)),
							int(math.Round(float64(x+w)/scale)), int(math.Round(float64(y+h)/scale))),
						NativeBounds: image.Rect(x, y, x+w, y+h),
						Scale:        scale,
						Primary:      i == 0,
					})
				}
			}
		}
	}
	if len(b.displays) == 0 {
		w := int(api.displayWidth(b.dpy, screen))
		h := int(api.displayHeight(b.dpy, screen))
		if w > 0 && h > 0 {
			b.displays = []Display{{
				ID:           strconv.Itoa(int(screen)),
				Name:         "Screen",
				Bounds:       image.Rect(0, 0, int(math.Round(float64(w)/scale)), int(math.Round(float64(h)/scale))),
				NativeBounds: image.Rect(0, 0, w, h),
				Scale:        scale,
				Primary:      true,
			}}
		}
	}
	// A single Xinerama screen is the normal case; keep the root window size
	// as NativeBounds so capture uses real pixels.
}

func (b *x11Backend) Capabilities() Capability {
	return Capability{
		PreCaptureOverlay: true,
		WindowDetection:   true,
		AllDisplays:       true,
		Cursor:            true,
		GlobalShortcut:    true,
	}
}

func (b *x11Backend) Permission() Permission { return PermissionNotRequired }

func (b *x11Backend) RequestPermission(context.Context) Permission { return PermissionNotRequired }

func (b *x11Backend) Displays() ([]Display, error) {
	if _, err := b.init(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.displays) == 0 {
		return nil, ErrNoDisplay
	}
	return append([]Display(nil), b.displays...), nil
}

func (b *x11Backend) Capture(ctx context.Context, opts Options) ([]Frame, error) {
	api, err := b.init()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	displays := append([]Display(nil), b.displays...)
	if len(displays) == 0 {
		return nil, ErrNoDisplay
	}
	root := api.rootWindow(b.dpy, api.defaultScreen(b.dpy))
	screen := api.defaultScreen(b.dpy)
	rootW := int(api.displayWidth(b.dpy, screen))
	rootH := int(api.displayHeight(b.dpy, screen))
	var frames []Frame
	var firstErr error
	selected := 0
	for _, d := range displays {
		if opts.AllDisplays || opts.DisplayID == "" || d.ID == opts.DisplayID {
			selected++
		}
	}
	for _, d := range displays {
		if !opts.AllDisplays && opts.DisplayID != "" && d.ID != opts.DisplayID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Capture the intersection of the display and the root window.
		r := d.NativeBounds.Intersect(image.Rect(0, 0, rootW, rootH))
		if r.Dx() < 1 || r.Dy() < 1 {
			continue
		}
		if int64(r.Dx())*int64(r.Dy()) > 80_000_000 {
			if firstErr == nil {
				firstErr = ErrTooLarge
			}
			continue
		}
		ximg := api.getImage(b.dpy, root, int32(r.Min.X), int32(r.Min.Y), uint32(r.Dx()), uint32(r.Dy()), xAllPlanes, xZPixmap)
		if ximg == 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: XGetImage", ErrCaptureFailed)
			}
			continue
		}
		img := (*xImage)(unsafe.Pointer(ximg))
		w, h := int(img.Width), int(img.Height)
		stride := int(img.BytesPerLine)
		bpp := int(img.BitsPerPixel) / 8
		var rgba *image.RGBA
		if img.Data != 0 && stride > 0 && bpp >= 2 && w > 0 && h > 0 {
			data := unsafe.Slice((*byte)(unsafe.Pointer(img.Data)), stride*h)
			rgba = RGBAFromPacked(data, stride, w, h, bpp, uint32(img.ByteOrder),
				uint32(img.RedMask), uint32(img.GreenMask), uint32(img.BlueMask))
			// Masks may be wider than 32 bits on a deep visual.
			if uint64(uint32(img.RedMask)) != img.RedMask {
				rgba = RGBAFromPacked(data, stride, w, h, bpp, uint32(img.ByteOrder),
					uint32(img.RedMask>>32), uint32(img.GreenMask>>32), uint32(img.BlueMask>>32))
			}
		}
		if api.destroyImage != nil {
			api.destroyImage(ximg)
		} else if img.Data != 0 {
			api.free(img.Data)
			api.free(ximg)
		}
		if rgba == nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: unsupported X11 visual", ErrCaptureFailed)
			}
			continue
		}
		frame := Frame{
			DisplayID: d.ID,
			Bounds: image.Rect(
				d.Bounds.Min.X+int(math.Round(float64(r.Min.X-d.NativeBounds.Min.X)/d.Scale)),
				d.Bounds.Min.Y+int(math.Round(float64(r.Min.Y-d.NativeBounds.Min.Y)/d.Scale)),
				d.Bounds.Min.X+int(math.Round(float64(r.Max.X-d.NativeBounds.Min.X)/d.Scale)),
				d.Bounds.Min.Y+int(math.Round(float64(r.Max.Y-d.NativeBounds.Min.Y)/d.Scale)),
			),
			Scale:      d.Scale,
			Image:      rgba,
			CapturedAt: time.Now(),
		}
		frame.Calibrate()
		frames = append(frames, frame)
	}
	_ = selected
	if len(frames) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrCaptureFailed
	}
	return frames, nil
}

func (b *x11Backend) Windows(ctx context.Context, displayID string) ([]WindowInfo, error) {
	api, err := b.init()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	root := api.rootWindow(b.dpy, api.defaultScreen(b.dpy))
	prop := api.internAtom(b.dpy, "_NET_CLIENT_LIST_STACKING", 1)
	if prop == 0 {
		prop = api.internAtom(b.dpy, "_NET_CLIENT_LIST", 1)
	}
	if prop == 0 {
		return nil, nil
	}
	ids, ok := b.readLongs(prop)
	if !ok {
		return nil, nil
	}
	nameAtom := api.internAtom(b.dpy, "_NET_WM_NAME", 1)
	visibleAtom := api.internAtom(b.dpy, "_NET_WM_VISIBLE_NAME", 1)
	utf8Atom := api.internAtom(b.dpy, "UTF8_STRING", 1)
	typeAtom := api.internAtom(b.dpy, "_NET_WM_WINDOW_TYPE", 1)
	dockAtom := api.internAtom(b.dpy, "_NET_WM_WINDOW_TYPE_DOCK", 1)
	desktopAtom := api.internAtom(b.dpy, "_NET_WM_WINDOW_TYPE_DESKTOP", 1)
	tooltipAtom := api.internAtom(b.dpy, "_NET_WM_WINDOW_TYPE_TOOLTIP", 1)
	hiddenAtom := api.internAtom(b.dpy, "_NET_WM_STATE_HIDDEN", 1)
	stateAtom := api.internAtom(b.dpy, "_NET_WM_STATE", 1)

	var windows []WindowInfo
	// The property lists windows bottom to top, so the last one is on top.
	for i := len(ids) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		id := uintptr(ids[i])
		if id == 0 {
			continue
		}
		if b.hidden(id, stateAtom, hiddenAtom) {
			continue
		}
		if b.windowTypeExcluded(id, typeAtom, dockAtom, desktopAtom, tooltipAtom) {
			continue
		}
		var x, y int32
		var child uintptr
		if api.translateCoords(b.dpy, id, root, 0, 0, &x, &y, &child) == 0 {
			continue
		}
		var w, h, border, depth uint32
		var groot uintptr
		var gx, gy int32
		if api.getGeometry(b.dpy, id, &groot, &gx, &gy, &w, &h, &border, &depth) == 0 {
			continue
		}
		if w < 2 || h < 2 {
			continue
		}
		title := ""
		if nameAtom != 0 {
			title = b.stringProperty(id, nameAtom)
		}
		if title == "" && visibleAtom != 0 {
			title = b.stringProperty(id, visibleAtom)
		}
		if title == "" {
			continue
		}
		_ = utf8Atom
		bounds := image.Rect(
			int(math.Round(float64(x)/b.scale)),
			int(math.Round(float64(y)/b.scale)),
			int(math.Round(float64(x+int32(w))/b.scale)),
			int(math.Round(float64(y+int32(h))/b.scale)),
		)
		windows = append(windows, WindowInfo{
			ID:     uint64(id),
			Title:  title,
			Bounds: bounds,
			ZOrder: len(windows),
		})
	}
	SortWindows(windows)
	return windows, nil
}

func (b *x11Backend) hidden(w uintptr, stateAtom, hiddenAtom uintptr) bool {
	if stateAtom == 0 || hiddenAtom == 0 {
		return false
	}
	vals, ok := b.readLongsFor(w, stateAtom)
	if !ok {
		return false
	}
	for _, v := range vals {
		if uintptr(v) == hiddenAtom {
			return true
		}
	}
	return false
}

func (b *x11Backend) windowTypeExcluded(w, typeAtom, dock, desktop, tooltip uintptr) bool {
	if typeAtom == 0 {
		return false
	}
	vals, ok := b.readLongsFor(w, typeAtom)
	if !ok {
		return false
	}
	for _, v := range vals {
		t := uintptr(v)
		if t != 0 && (t == dock || t == desktop || t == tooltip) {
			return true
		}
	}
	return false
}

func (b *x11Backend) readLongs(property uintptr) ([]uint64, bool) {
	return b.readLongsFor(b.api.rootWindow(b.dpy, b.api.defaultScreen(b.dpy)), property)
}

func (b *x11Backend) readLongsFor(w, property uintptr) ([]uint64, bool) {
	api := b.api
	var actualType uintptr
	var format int32
	var nitems, bytesAfter uint64
	var data uintptr
	if api.getWindowProperty(b.dpy, w, property, 0, 4096, 0, xAnyProperty,
		&actualType, &format, &nitems, &bytesAfter, &data) != xSuccess || data == 0 {
		return nil, false
	}
	defer api.free(data)
	if format != 32 || nitems == 0 {
		return nil, false
	}
	vals := unsafe.Slice((*uintptr)(unsafe.Pointer(data)), nitems)
	out := make([]uint64, nitems)
	for i, v := range vals {
		out[i] = uint64(v)
	}
	return out, true
}

func (b *x11Backend) stringProperty(w, property uintptr) string {
	api := b.api
	var actualType uintptr
	var format int32
	var nitems, bytesAfter uint64
	var data uintptr
	if api.getWindowProperty(b.dpy, w, property, 0, 4096, 0, xAnyProperty,
		&actualType, &format, &nitems, &bytesAfter, &data) != xSuccess || data == 0 {
		return ""
	}
	defer api.free(data)
	if format != 8 || nitems == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(data)), nitems))
}
