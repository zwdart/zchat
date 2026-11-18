package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/iconkit"
)

// The icon page builds in both tabs without a window. This catches layout
// or API mistakes that a plain compile cannot.
func TestIconKitPageRenders(t *testing.T) {
	a := &app{
		settings: settings{
			IconName:       "zchat",
			IconOutputDir:  t.TempDir(),
			IconBackground: "#ffffff",
		},
		iconPicks:       iconPicks{Android: true, IOS: true, Windows: true, Linux: true, MacOS: true},
		iconToolChecked: true,
		iconTool:        iconkit.Tool{Path: "/usr/bin/magick", Version: "ImageMagick 7.1"},
	}
	tt := ui.NewTester(a.iconKitPage, 1000, 800)
	if !tt.HasText("安装包图标") {
		t.Fatal("page title is missing")
	}
	if !tt.HasText("开始生成") {
		t.Fatal("generate button is missing")
	}

	a.iconTab = 1
	tt.Frame()
	if !tt.HasText("官方尺寸与格式要求") {
		t.Fatal("requirements heading is missing")
	}
	if !tt.HasText("复制全部说明") {
		t.Fatal("copy-all button is missing")
	}
	if !tt.HasText("Android") || !tt.HasText("Google Play") {
		t.Fatal("platform documentation is missing")
	}
}
