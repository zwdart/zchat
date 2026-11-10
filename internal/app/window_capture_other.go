//go:build !windows

package app

import "github.com/egoist/mygo"

// concealWindowForCapture is unnecessary on platforms whose compositors
// remove a hidden window at once.
func concealWindowForCapture(*mygo.Window) func() { return nil }
