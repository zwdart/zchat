package capture

import (
	"context"
	"image"
	"time"
)

// FakeBackend is a deterministic backend for tests and for the screenshot
// session without a desktop. It returns the configured displays, frames and
// windows. Error fields inject failures.
type FakeBackend struct {
	DisplayList  []Display
	FrameList    []Frame
	WindowList   []WindowInfo
	Cap          Capability
	Perm         Permission
	DisplayErr   error
	CaptureErr   error
	WindowErr    error
	RequestPerm  Permission
	CaptureCalls int
}

func (f *FakeBackend) Displays() ([]Display, error) {
	return f.DisplayList, f.DisplayErr
}

func (f *FakeBackend) Capabilities() Capability { return f.Cap }

func (f *FakeBackend) Capture(_ context.Context, opts Options) ([]Frame, error) {
	f.CaptureCalls++
	if f.CaptureErr != nil {
		return nil, f.CaptureErr
	}
	var out []Frame
	for _, frame := range f.FrameList {
		if opts.AllDisplays || opts.DisplayID == "" || frame.DisplayID == opts.DisplayID {
			frame.CapturedAt = time.Now()
			frame.Calibrate()
			out = append(out, frame)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoDisplay
	}
	return out, nil
}

func (f *FakeBackend) Windows(context.Context, string) ([]WindowInfo, error) {
	return append([]WindowInfo(nil), f.WindowList...), f.WindowErr
}

func (f *FakeBackend) Permission() Permission { return f.Perm }

func (f *FakeBackend) RequestPermission(context.Context) Permission {
	if f.RequestPerm != 0 {
		return f.RequestPerm
	}
	return f.Perm
}

// FakeFrame builds an opaque frame of width×height filled with c.
func FakeFrame(displayID string, bounds image.Rectangle, scale float64, width, height int) Frame {
	img := image.NewRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = 30
		img.Pix[i+1] = 60
		img.Pix[i+2] = 90
		img.Pix[i+3] = 255
	}
	return Frame{DisplayID: displayID, Bounds: bounds, Scale: scale, Image: img}
}
