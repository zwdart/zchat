package capture

import (
	"image"
	"sort"
	"strings"
)

// SortWindows orders windows from the top of the stack to the bottom. The
// platform backends fill ZOrder already, but a merged or filtered list may
// not be ordered.
func SortWindows(windows []WindowInfo) {
	sort.SliceStable(windows, func(i, j int) bool {
		return windows[i].ZOrder < windows[j].ZOrder
	})
}

// WindowAt returns the topmost window containing p in DIP coordinates.
func WindowAt(windows []WindowInfo, p image.Point) (WindowInfo, bool) {
	SortWindows(windows)
	for _, w := range windows {
		if p.In(w.Bounds) {
			return w, true
		}
	}
	return WindowInfo{}, false
}

// FilterWindows removes duplicate, empty, oversized and self windows, and
// clips the rest to the display. It is shared by the platform backends.
func FilterWindows(windows []WindowInfo, display image.Rectangle, selfNames ...string) []WindowInfo {
	self := make([]string, 0, len(selfNames))
	for _, name := range selfNames {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			self = append(self, name)
		}
	}
	seen := map[uint64]bool{}
	out := make([]WindowInfo, 0, len(windows))
	for _, w := range windows {
		if w.ID != 0 && seen[w.ID] {
			continue
		}
		seen[w.ID] = true
		if w.Bounds.Dx() < 2 || w.Bounds.Dy() < 2 {
			continue
		}
		r := w.Bounds.Intersect(display)
		if r.Dx() < 2 || r.Dy() < 2 {
			continue
		}
		w.Bounds = r
		skip := false
		if w.Title != "" {
			title := strings.ToLower(w.Title)
			for _, name := range self {
				if title == name || strings.Contains(title, name) {
					skip = true
					break
				}
			}
		}
		if skip {
			continue
		}
		out = append(out, w)
	}
	SortWindows(out)
	return out
}
