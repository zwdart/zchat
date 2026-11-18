package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/magick"
)

// The toolkit pages build without a window, which catches layout and API
// mistakes that a plain compile cannot.
func TestMagickPageRenders(t *testing.T) {
	a := &app{
		settings: settings{
			MagickOutputDir: t.TempDir(),
			MagickFormat:    "keep",
			MagickQuality:   90,
		},
		iconToolChecked: true,
		iconTool:        magick.Tool{Path: "/usr/bin/magick", Version: "ImageMagick 7.1"},
	}
	tt := ui.NewTester(a.magickPage, 1100, 850)
	if !tt.HasText("ImageMagick") {
		t.Fatal("page title is missing")
	}
	for _, want := range []string{"格式转换", "缩放与裁剪", "批量处理", "加水印", "图像合成", "颜色与质量", "安装包图标"} {
		if !tt.HasText(want) {
			t.Fatalf("capability %q is missing", want)
		}
	}

	a.magick.cap = string(magick.KindConvert)
	tt.Frame()
	for _, want := range []string{"输入图片", "添加图片", "输出目录", "目标格式", "开始处理"} {
		if !tt.HasText(want) {
			t.Fatalf("convert form is missing %q", want)
		}
	}

	a.magick.cap = string(magick.KindWatermark)
	tt.Frame()
	if !tt.HasText("文字水印") || !tt.HasText("水印文字") {
		t.Fatal("watermark form is missing")
	}

	a.magick.cap = string(magick.KindCompose)
	tt.Frame()
	if !tt.HasText("横向拼接") || !tt.HasText("GIF 动画") {
		t.Fatal("compose form is missing")
	}

	a.magick.cap = magickIconCap
	tt.Frame()
	if !tt.HasText("安装包图标") || !tt.HasText("开始生成") {
		t.Fatal("icon generator is not shown from the toolkit")
	}
}
