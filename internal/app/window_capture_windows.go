//go:build windows

package app

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const dwmwaCloak = 13

// concealWindowForCapture takes win out of the desktop composition at once.
// Hiding a window with a Windows 11 material (Mica) can leave a translucent
// ghost for a frame or two, which a capture taken right after Hide records;
// DWM's cloak attribute removes the window from the composition immediately.
// It also keeps the window out of captures at the display-affinity level, so
// a frame that still reaches the compositor does not show in the shot. The
// returned function undoes both.
func concealWindowForCapture(win *mygo.Window) func() {
	if win == nil || win.IsDestroyed() {
		return nil
	}
	hwnd := win.NativeHandle()
	if hwnd == 0 {
		return nil
	}
	win.SetContentProtection(true)
	setWindowCloaked(hwnd, true)
	return func() {
		win.SetContentProtection(false)
		setWindowCloaked(hwnd, false)
	}
}

func setWindowCloaked(hwnd uintptr, cloaked bool) {
	v := int32(0)
	if cloaked {
		v = 1
	}
	procDwmSetWindowAttribute.Call(hwnd, dwmwaCloak, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}
