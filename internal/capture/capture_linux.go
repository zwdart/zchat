//go:build linux

package capture

import (
	"os"
	"strings"
)

// platformBackend picks the X11 or Wayland implementation from the session
// the app is running in. X11 can be captured directly with libX11; a
// Wayland session has no such protocol for ordinary clients, so the backend
// shells out to the desktop's screenshot tool and lets zchat edit afterwards.
func platformBackend() Backend {
	if isWaylandSession() {
		return &waylandBackend{}
	}
	return newX11Backend()
}

func isWaylandSession() bool {
	switch strings.ToLower(os.Getenv("XDG_SESSION_TYPE")) {
	case "wayland":
		return true
	case "x11":
		return false
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}
