package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/ffmpeg"
)

// The FFmpeg pages build without a window, which catches layout and API
// mistakes that a plain compile cannot.
func TestFFmpegPageRenders(t *testing.T) {
	a := &app{
		settings: settings{
			FFmpegOutputDir: t.TempDir(),
			FFmpegCRF:       23,
			FFmpegPreset:    "medium",
			FFmpegAudioRate: "128k",
		},
		ffToolChecked: true,
		ffTool: ffmpeg.Tool{
			Path:      "/usr/bin/ffmpeg",
			ProbePath: "/usr/bin/ffprobe",
			Version:   "ffmpeg version 6.1",
		},
	}
	a.ffmpegDefaults()
	tt := ui.NewTester(a.ffmpegPage, 1100, 850)
	if !tt.HasText("FFmpeg") {
		t.Fatal("page title is missing")
	}
	for _, want := range []string{"视频压缩", "视频剪切", "视频拼接", "提取音频", "视频转 GIF", "添加水印", "画面调整", "字幕与音乐", "倍速播放", "视频截图", "批量处理"} {
		if !tt.HasText(want) {
			t.Fatalf("capability %q is missing", want)
		}
	}

	forms := []struct {
		cap  ffmpeg.Kind
		want string
	}{
		{ffmpeg.KindCompress, "压缩设置"},
		{ffmpeg.KindTrim, "剪切范围"},
		{ffmpeg.KindMerge, "拼接设置"},
		{ffmpeg.KindAudio, "音频设置"},
		{ffmpeg.KindGIF, "GIF 设置"},
		{ffmpeg.KindWatermark, "水印设置"},
		{ffmpeg.KindTransform, "画面调整"},
		{ffmpeg.KindSubtitle, "字幕与音乐"},
		{ffmpeg.KindSpeed, "倍速设置"},
		{ffmpeg.KindCover, "截图设置"},
		{ffmpeg.KindBatch, "批量设置"},
	}
	for _, form := range forms {
		a.ffmpeg.cap = string(form.cap)
		a.ffmpegEnterCap(a.ffmpeg.cap)
		tt.Frame()
		if !tt.HasText(form.want) {
			t.Fatalf("%s form is missing %q", form.cap, form.want)
		}
		if !tt.HasText("开始处理") {
			t.Fatalf("%s form has no run button", form.cap)
		}
	}
}

func TestFFmpegOptionsMapState(t *testing.T) {
	a := &app{
		settings: settings{
			FFmpegOutputDir: "/tmp/out",
			FFmpegCRF:       28,
			FFmpegPreset:    "fast",
			FFmpegAudioRate: "192k",
		},
	}
	a.ffmpegDefaults()
	a.ffmpeg.cap = string(ffmpeg.KindGIF)
	a.ffmpeg.gifWidth = 320
	a.ffmpeg.fps = 15
	opt := a.ffmpegOptions()
	if opt.Kind != ffmpeg.KindGIF || opt.Width != 320 || opt.FPS != 15 {
		t.Fatalf("gif options = %+v", opt)
	}
	if opt.CRF != 28 || opt.Preset != "fast" || opt.AudioBitrate != "192k" {
		t.Fatalf("saved settings were not applied: %+v", opt)
	}

	a.ffmpeg.cap = string(ffmpeg.KindSubtitle)
	a.ffmpeg.subtitleMode = 1
	if opt := a.ffmpegOptions(); opt.SubtitleMode != ffmpeg.SubtitleBGM {
		t.Fatalf("subtitle mode = %q", opt.SubtitleMode)
	}

	a.ffmpeg.cap = string(ffmpeg.KindBatch)
	a.ffmpeg.batchMode = 2
	if opt := a.ffmpegOptions(); opt.BatchMode != ffmpeg.BatchCover {
		t.Fatalf("batch mode = %q", opt.BatchMode)
	}
}
