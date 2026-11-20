package magick

import "testing"

func TestParseImageInfo(t *testing.T) {
	data := []byte("PNG|100|50|8|sRGB|srgba|72|72|PixelsPerInch|false|2|92|TrueColorAlpha\n")
	info, err := parseImageInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "PNG" || info.Width != 100 || info.Height != 50 {
		t.Fatalf("basic info = %+v", info)
	}
	if info.Depth != 8 || info.Colorspace != "sRGB" || info.Channels != "srgba" {
		t.Fatalf("color info = %+v", info)
	}
	if !info.OpaqueKnown || info.Opaque {
		t.Fatalf("transparency = known:%v opaque:%v", info.OpaqueKnown, info.Opaque)
	}
	if info.Frames != 2 || info.Quality != 92 || info.Type != "TrueColorAlpha" {
		t.Fatalf("extra info = %+v", info)
	}
	if info.ResolutionX != 72 || info.ResolutionY != 72 || info.Units != "PixelsPerInch" {
		t.Fatalf("resolution = %+v", info)
	}
}

func TestParseImageInfoRejectsBadOutput(t *testing.T) {
	if _, err := parseImageInfo([]byte("not an identify line")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestIdentifyCommandForMagick7(t *testing.T) {
	cmd, prefix, err := (Tool{Path: "/usr/bin/magick"}).identifyCommand()
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "/usr/bin/magick" || len(prefix) != 1 || prefix[0] != "identify" {
		t.Fatalf("command = %q %v", cmd, prefix)
	}
}
