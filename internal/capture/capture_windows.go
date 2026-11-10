//go:build windows

package capture

import (
	"context"
	"fmt"
	"image"
	"math"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procEnumDisplayMonitors      = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetCursorInfo            = user32.NewProc("GetCursorInfo")
	procGetIconInfo              = user32.NewProc("GetIconInfo")
	procDrawIconEx               = user32.NewProc("DrawIconEx")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection         = gdi32.NewProc("CreateDIBSection")
	procGdiFlush                 = gdi32.NewProc("GdiFlush")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procBitBlt                   = gdi32.NewProc("BitBlt")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
	procDwmGetWindowAttribute    = dwmapi.NewProc("DwmGetWindowAttribute")
	procGetDpiForMonitor         = shcore.NewProc("GetDpiForMonitor")
	procGetCurrentProcessID      = kernel32.NewProc("GetCurrentProcessId")
)

const (
	srcCopy                  = 0x00CC0020
	captureBlt               = 0x40000000
	dibRGBColors             = 0
	mdtEffectiveDPI          = 0
	dwmwaExtendedFrameBounds = 9
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

func (r winRect) dx() int { return int(r.Right - r.Left) }
func (r winRect) dy() int { return int(r.Bottom - r.Top) }

type monitorInfoEx struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
	SzDevice  [32]uint16
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type point struct{ X, Y int32 }

type cursorInfo struct {
	CbSize  uint32
	Flags   uint32
	HCursor uintptr
	Pt      point
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// windowsBackend uses GDI for pixels and User32/DWM for displays and
// windows. It needs no cgo or external runtime.
type windowsBackend struct {
	mu sync.Mutex
}

func platformBackend() Backend { return &windowsBackend{} }

func (b *windowsBackend) Capabilities() Capability {
	return Capability{
		PreCaptureOverlay: true,
		WindowDetection:   true,
		AllDisplays:       true,
		Cursor:            true,
		GlobalShortcut:    true,
	}
}

func (b *windowsBackend) Permission() Permission { return PermissionNotRequired }

func (b *windowsBackend) RequestPermission(context.Context) Permission {
	return PermissionNotRequired
}

func (b *windowsBackend) Displays() ([]Display, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Display
	cb := windows.NewCallback(func(hmon, _ uintptr, _ uintptr, _ uintptr) uintptr {
		var info monitorInfoEx
		info.CbSize = uint32(unsafe.Sizeof(info))
		if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&info))); r == 0 {
			return 1
		}
		scale := monitorScale(hmon)
		dip := func(v int32) int { return int(math.Round(float64(v) / scale)) }
		native := image.Rect(int(info.RcMonitor.Left), int(info.RcMonitor.Top), int(info.RcMonitor.Right), int(info.RcMonitor.Bottom))
		name := windows.UTF16ToString(info.SzDevice[:])
		out = append(out, Display{
			ID:           strconv.FormatUint(uint64(hmon), 10),
			Name:         name,
			Bounds:       image.Rect(dip(info.RcMonitor.Left), dip(info.RcMonitor.Top), dip(info.RcMonitor.Right), dip(info.RcMonitor.Bottom)),
			NativeBounds: native,
			Scale:        scale,
			Primary:      info.DwFlags&1 != 0,
		})
		return 1
	})
	if r, _, err := procEnumDisplayMonitors.Call(0, 0, cb, 0); r == 0 {
		return nil, captureError("EnumDisplayMonitors", err)
	}
	if len(out) == 0 {
		return nil, ErrNoDisplay
	}
	return out, nil
}

func monitorScale(hmon uintptr) float64 {
	var x, y uint32
	if r, _, _ := procGetDpiForMonitor.Call(hmon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); r == 0 && x > 0 {
		return float64(x) / 96
	}
	// GetDpiForMonitor is missing before Windows 8.1. A screen DC gives the
	// system DPI, which is better than assuming 96 for the common case.
	if dc, _, _ := procGetDC.Call(0); dc != 0 {
		defer procReleaseDC.Call(0, dc)
		// LOGPIXELSX = 88. GetDeviceCaps is in gdi32.
		if r, _, _ := gdi32.NewProc("GetDeviceCaps").Call(dc, 88); r > 0 {
			return float64(r) / 96
		}
	}
	return 1
}

func (b *windowsBackend) Capture(ctx context.Context, opts Options) ([]Frame, error) {
	displays, err := b.Displays()
	if err != nil {
		return nil, err
	}
	selected := make([]Display, 0, len(displays))
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		frame, err := captureWindowsDisplay(d, opts.IncludeCursor && (opts.AllDisplays || len(selected) == 1))
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

func captureWindowsDisplay(d Display, includeCursor bool) (Frame, error) {
	w, h := d.NativeBounds.Dx(), d.NativeBounds.Dy()
	if w <= 0 || h <= 0 {
		return Frame{}, ErrNoDisplay
	}
	// A 4K screenshot is 32MB; refuse absurd desktops rather than exhaust
	// memory on a broken virtual display.
	if int64(w)*int64(h) > 80_000_000 {
		return Frame{}, ErrTooLarge
	}
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return Frame{}, fmt.Errorf("%w: GetDC", ErrCaptureFailed)
	}
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return Frame{}, fmt.Errorf("%w: CreateCompatibleDC", ErrCaptureFailed)
	}
	defer procDeleteDC.Call(mem)
	// A DIB section owns its pixels, so the screenshot can be read back
	// without GetDIBits. GetDIBits fails on some systems for device-dependent
	// bitmaps even after a successful BitBlt (it returns 0 while the last
	// error is still ERROR_SUCCESS).
	bi := bitmapInfo{Header: bitmapInfoHeader{
		BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		BiWidth:       int32(w),
		BiHeight:      -int32(h),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: 0,
		BiSizeImage:   uint32(w * h * 4),
	}}
	var bits unsafe.Pointer
	bmp, _, err := procCreateDIBSection.Call(screen,
		uintptr(unsafe.Pointer(&bi)), dibRGBColors,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return Frame{}, captureError("CreateDIBSection", err)
	}
	defer procDeleteObject.Call(bmp)
	old, _, err := procSelectObject.Call(mem, bmp)
	if old == 0 || old == ^uintptr(0) {
		return Frame{}, captureError("SelectObject", err)
	}
	defer procSelectObject.Call(mem, old)
	if r, _, err := procBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen,
		uintptr(uint32(int32(d.NativeBounds.Min.X))), uintptr(uint32(int32(d.NativeBounds.Min.Y))), srcCopy|captureBlt); r == 0 {
		return Frame{}, captureError("BitBlt", err)
	}
	if includeCursor {
		drawCursor(mem, d.NativeBounds)
	}
	// GDI may batch drawing calls; flush before reading the DIB pixels.
	if r, _, _ := procGdiFlush.Call(); r == 0 {
		return Frame{}, fmt.Errorf("%w: GdiFlush", ErrCaptureFailed)
	}
	buf := make([]byte, w*h*4)
	copy(buf, unsafe.Slice((*byte)(bits), len(buf)))
	img := RGBAFromBGRA(buf, w*4, w, h)
	return Frame{DisplayID: d.ID, Bounds: d.Bounds, Scale: d.Scale, Image: img}, nil
}

// captureError wraps a failed Win32/GDI call. The syscall error is only
// included when it is real: GDI calls often leave ERROR_SUCCESS behind, and
// "The operation completed successfully" is not a useful failure reason.
func captureError(op string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%w: %s", ErrCaptureFailed, op)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrCaptureFailed, op, err)
	}
	return fmt.Errorf("%w: %s", ErrCaptureFailed, op)
}

func drawCursor(dc uintptr, native image.Rectangle) {
	var ci cursorInfo
	ci.CbSize = uint32(unsafe.Sizeof(ci))
	if r, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci))); r == 0 || ci.Flags&1 == 0 {
		return
	}
	x := int(ci.Pt.X) - native.Min.X
	y := int(ci.Pt.Y) - native.Min.Y
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(ci.HCursor, uintptr(unsafe.Pointer(&ii))); r != 0 {
		x -= int(ii.XHotspot)
		y -= int(ii.YHotspot)
		defer procDeleteObject.Call(ii.HbmMask)
		if ii.HbmColor != 0 {
			defer procDeleteObject.Call(ii.HbmColor)
		}
	}
	procDrawIconEx.Call(dc, uintptr(uint32(int32(x))), uintptr(uint32(int32(y))), ci.HCursor, 0, 0, 0, 0, 3)
}

func (b *windowsBackend) Windows(ctx context.Context, displayID string) ([]WindowInfo, error) {
	displays, err := b.Displays()
	if err != nil {
		return nil, err
	}
	var target *Display
	for i := range displays {
		if displays[i].ID == displayID {
			target = &displays[i]
			break
		}
	}
	pid, _, _ := procGetCurrentProcessID.Call()
	z := 0
	var out []WindowInfo
	cb := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		if ctx.Err() != nil {
			return 0
		}
		if r, _, _ := procIsWindowVisible.Call(hwnd); r == 0 {
			return 1
		}
		if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
			return 1
		}
		n, _, _ := procGetWindowTextLengthW.Call(hwnd)
		if n == 0 {
			return 1
		}
		titleBuf := make([]uint16, n+1)
		if r, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), uintptr(len(titleBuf))); r == 0 {
			return 1
		}
		var wpid uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if uintptr(wpid) == pid {
			return 1
		}
		var nr winRect
		if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&nr))); r == 0 {
			return 1
		}
		// DWM's extended bounds leave out the invisible resize border and
		// drop shadow that GetWindowRect includes on Windows 10/11.
		var dr winRect
		if r, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&dr)), uintptr(unsafe.Sizeof(dr))); r == 0 {
			nr = dr
		}
		if nr.dx() < 2 || nr.dy() < 2 {
			return 1
		}
		center := image.Pt((int(nr.Left)+int(nr.Right))/2, (int(nr.Top)+int(nr.Bottom))/2)
		d, ok := DisplayAt(displays, pixelPointToDIP(displays, center))
		if !ok {
			return 1
		}
		if target != nil && d.ID != target.ID {
			return 1
		}
		dip := pixelRectToDIP(displays, image.Rect(int(nr.Left), int(nr.Top), int(nr.Right), int(nr.Bottom)))
		out = append(out, WindowInfo{
			ID:     uint64(hwnd),
			Title:  windows.UTF16ToString(titleBuf),
			Bounds: dip,
			ZOrder: z,
		})
		z++
		return 1
	})
	if r, _, err := procEnumWindows.Call(cb, 0); r == 0 && len(out) == 0 {
		return nil, captureError("EnumWindows", err)
	}
	return out, nil
}

func pixelPointToDIP(displays []Display, p image.Point) image.Point {
	for _, d := range displays {
		if p.In(d.NativeBounds) {
			return image.Pt(
				d.Bounds.Min.X+int(math.Round(float64(p.X-d.NativeBounds.Min.X)/d.Scale)),
				d.Bounds.Min.Y+int(math.Round(float64(p.Y-d.NativeBounds.Min.Y)/d.Scale)),
			)
		}
	}
	d, ok := DisplayAt(displays, p)
	if !ok {
		return p
	}
	return image.Pt(
		d.Bounds.Min.X+int(math.Round(float64(p.X-d.NativeBounds.Min.X)/d.Scale)),
		d.Bounds.Min.Y+int(math.Round(float64(p.Y-d.NativeBounds.Min.Y)/d.Scale)),
	)
}

func pixelRectToDIP(displays []Display, r image.Rectangle) image.Rectangle {
	a := pixelPointToDIP(displays, r.Min)
	b := pixelPointToDIP(displays, r.Max)
	return image.Rect(a.X, a.Y, b.X, b.Y)
}
