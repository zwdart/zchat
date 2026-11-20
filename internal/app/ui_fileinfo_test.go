package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/ffmpeg"
	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/magick"
)

func TestFileInfoDialogRendersMedia(t *testing.T) {
	key := fileInfoMedia + "\x00/tmp/demo.mp4"
	a := &app{
		fileInfoOpen: true,
		fileInfoKey:  key,
		fileInfoCache: map[string]*fileInfoEntry{
			key: {
				Kind: fileInfoMedia,
				Path: "/tmp/demo.mp4",
				Media: &ffmpeg.MediaInfo{
					Path:       "/tmp/demo.mp4",
					Size:       12 << 20,
					Duration:   2*time.Minute + 35*time.Second,
					FormatName: "mov,mp4,m4a,3gp,3g2,mj2",
					Bitrate:    8_400_000,
					Video: &ffmpeg.VideoInfo{
						Codec: "h264",
						Width: 1920, Height: 1080,
						FPS: 29.97,
					},
					Audio: &ffmpeg.AudioInfo{
						Codec: "aac", Channels: 2, ChannelLayout: "stereo", SampleRate: 48000,
					},
				},
			},
		},
	}
	tt := ui.NewTester(a.fileInfoDialog, 900, 800)
	for _, want := range []string{"视频信息", "时长", "2 分 35 秒", "1920 × 1080 (16:9)", "12.0 MB", "视频编码", "H.264", "帧率", "29.97 fps", "AAC · 立体声 · 48 kHz", "文件路径"} {
		if !tt.HasText(want) {
			t.Fatalf("dialog is missing %q", want)
		}
	}
}

func TestFileInfoDialogRendersImage(t *testing.T) {
	key := fileInfoImage + "\x00/tmp/demo.png"
	a := &app{
		fileInfoOpen: true,
		fileInfoKey:  key,
		fileInfoCache: map[string]*fileInfoEntry{
			key: {
				Kind: fileInfoImage,
				Path: "/tmp/demo.png",
				Image: &magick.ImageInfo{
					Path: "/tmp/demo.png", Size: 2 << 20,
					Format: "PNG", Width: 1080, Height: 1920,
					Depth: 8, Colorspace: "sRGB", Channels: "srgba", Type: "TrueColorAlpha",
					Opaque: false, OpaqueKnown: true, Frames: 1,
					ResolutionX: 72, ResolutionY: 72, Units: "PixelsPerInch",
				},
			},
		},
	}
	tt := ui.NewTester(a.fileInfoDialog, 900, 800)
	for _, want := range []string{"图片信息", "尺寸", "1080 × 1920 (9:16)", "2.0 MB", "PNG", "透明", "是", "色彩空间", "sRGB", "位深", "72 × 72 DPI", "图片类型", "真彩色 (含透明通道)"} {
		if !tt.HasText(want) {
			t.Fatalf("dialog is missing %q", want)
		}
	}
}

func TestFileInfoDialogRendersEnglish(t *testing.T) {
	old := i18n.Current()
	defer i18n.Set(old)
	i18n.Set(i18n.English)
	key := fileInfoMedia + "\x00/tmp/audio.mp3"
	a := &app{
		fileInfoOpen: true,
		fileInfoKey:  key,
		fileInfoCache: map[string]*fileInfoEntry{
			key: {
				Kind: fileInfoMedia,
				Path: "/tmp/audio.mp3",
				Media: &ffmpeg.MediaInfo{
					Path:       "/tmp/audio.mp3",
					Size:       3 << 20,
					Duration:   2*time.Minute + 35*time.Second,
					FormatName: "mp3",
					Bitrate:    128_000,
					Audio: &ffmpeg.AudioInfo{
						Codec: "mp3", Channels: 2, ChannelLayout: "stereo", SampleRate: 44100,
						Bitrate: 128_000,
					},
				},
			},
		},
	}
	tt := ui.NewTester(a.fileInfoDialog, 900, 800)
	for _, want := range []string{
		"Audio details", "Duration", "2 min 35 s", "File size", "3.0 MB",
		"Total bitrate", "128 kbps", "MP3", "Stereo", "44.1 kHz",
	} {
		if !tt.HasText(want) {
			t.Fatalf("dialog is missing %q", want)
		}
	}
}

func TestFileInfoFormatting(t *testing.T) {
	if got := formatResolution(1920, 1080); got != "1920 × 1080 (16:9)" {
		t.Fatalf("resolution = %q", got)
	}
	if got := formatBitrate(8_400_000); got != "8.4 Mbps" {
		t.Fatalf("bitrate = %q", got)
	}
	if got := formatFPS(30); got != "30 fps" {
		t.Fatalf("fps = %q", got)
	}
	if got := formatSampleRate(48000); got != "48 kHz" {
		t.Fatalf("sample rate = %q", got)
	}
	if got := heightRatio(1080, 1920); got != "9:16" {
		t.Fatalf("ratio = %q", got)
	}
}

func heightRatio(width, height int) string {
	divisor := gcd(width, height)
	return itoa(width/divisor) + ":" + itoa(height/divisor)
}
