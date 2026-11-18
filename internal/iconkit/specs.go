// Package iconkit turns one square source image into the icon sizes and
// formats each desktop and mobile platform expects. The resizing is done by
// ImageMagick; this package only plans the commands, writes the small
// platform metadata files, and reports progress.
package iconkit

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"xin.dart.zchat/internal/i18n"
)

// Link is an official documentation link shown in the UI's requirements
// page.
type Link struct {
	Label string
	URL   string
}

// Icon is one raster image to produce.
type Icon struct {
	// Label names the output in progress messages, e.g. "mipmap-mdpi".
	Label string
	W, H  int
	// Rel is relative to the platform's directory and may contain {name}.
	Rel string
	// Inset is the margin on each side as a fraction of the canvas, used
	// by Android adaptive foregrounds (the system crops the outer ring).
	Inset float64
	// Opaque flattens transparency onto the chosen background color. iOS
	// app icons must not have an alpha channel.
	Opaque bool
}

// Bundle is a multi-resolution container written after its source icons.
// ImageMagick writes ICO; ICNS is attempted with ImageMagick and rebuilt by
// the package's own writer when that is unavailable.
type Bundle struct {
	Label string
	// Format is "ico" or "icns".
	Format string
	Rel    string
	// SourceSizes picks generated square icons used as ICNS entries.
	SourceSizes []int
}

// Static is a text file written as part of a platform's output. EN is the
// English text, used when the app language is English; an empty EN keeps
// Content in both languages (JSON and XML assets do not need translation).
type Static struct {
	Rel     string
	Content string
	EN      string
}

// DocRow is one row of the requirements table.
type DocRow struct {
	Item   string
	Sizes  string
	Format string
	Note   string
}

// Platform describes one target's files and the official requirements for
// them.
type Platform struct {
	Key     string
	Name    string
	Summary string
	Icons   []Icon
	Bundles []Bundle
	Static  []Static
	Notes   []string
	Links   []Link
	Docs    []DocRow
}

// FileCount is the number of files a platform writes.
func (p Platform) FileCount() int {
	return len(p.Icons) + len(p.Bundles) + len(p.Static)
}

// Markdown renders the platform's requirements as a small standalone
// document. The docs page shows it and the copy button puts it on the
// clipboard, so it stays readable outside the app.
func (p Platform) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n%s\n\n", p.Name, i18n.T(p.Summary))
	fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", i18n.T("项目"), i18n.T("尺寸"), i18n.T("格式"), i18n.T("说明"))
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, row := range p.Docs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			markdownCell(i18n.T(row.Item)),
			markdownCell(i18n.T(row.Sizes)),
			markdownCell(i18n.T(row.Format)),
			markdownCell(i18n.T(row.Note)))
	}
	b.WriteString("\n### " + i18n.T("备注") + "\n\n")
	for _, note := range p.Notes {
		b.WriteString("- " + i18n.T(note) + "\n")
	}
	b.WriteString("\n### " + i18n.T("官方文档") + "\n\n")
	for _, link := range p.Links {
		fmt.Fprintf(&b, "- [%s](%s)\n", i18n.T(link.Label), link.URL)
	}
	b.WriteString("\n")
	return b.String()
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// AllMarkdown is one document with every platform's requirements, for
// copying the whole reference at once.
func AllMarkdown() string {
	var b strings.Builder
	b.WriteString("# " + i18n.T("安装包图标要求") + "\n\n")
	for _, p := range Platforms {
		b.WriteString(p.Markdown())
		b.WriteString("\n")
	}
	return b.String()
}

// SafeName turns an app name into a file name safe on every target.
func SafeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "app"
	}
	if len(out) > 64 {
		out = strings.TrimRight(out[:64], "-.")
		if out == "" {
			out = "app"
		}
	}
	return out
}

// PlatformByKey finds a platform by its stable key.
func PlatformByKey(key string) (Platform, bool) {
	for _, p := range Platforms {
		if p.Key == key {
			return p, true
		}
	}
	return Platform{}, false
}

// Selected returns the platforms for keys, preserving the UI order.
func Selected(keys []string) []Platform {
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	out := make([]Platform, 0, len(keys))
	for _, p := range Platforms {
		if want[p.Key] {
			out = append(out, p)
		}
	}
	return out
}

// AllKeys returns every platform key in UI order.
func AllKeys() []string {
	out := make([]string, len(Platforms))
	for i, p := range Platforms {
		out[i] = p.Key
	}
	return out
}

// icon is a shorthand for the many square launcher icons below.
func icon(label string, size int, rel string) Icon {
	return Icon{Label: label, W: size, H: size, Rel: rel}
}

var androidAdaptiveXML = `<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@mipmap/ic_launcher_foreground" />
</adaptive-icon>
`

var androidColorXML = `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <color name="ic_launcher_background">{color}</color>
</resources>
`

var androidReadme = `Android 图标输出
================

1. 把 res/ 目录复制到你的 Android 工程 app/src/main/res/ 下（或合并同名目录）。
2. 清单里的 android:icon="@mipmap/ic_launcher" 和
   android:roundIcon="@mipmap/ic_launcher_round" 使用传统图标。
3. Android 8.0+ 会读取 res/mipmap-anydpi-v26/ic_launcher.xml 自适应图标。
   背景色在 res/values/ic_launcher_background.xml 中修改。
4. playstore/ic_launcher-playstore.png 用于 Google Play Console 的商店图标。

ImageMagick 已按各密度生成 PNG，直接覆盖即可。
`

var androidReadmeEN = `Android icon output
===================

1. Copy the res/ folder into your Android project's app/src/main/res/
   folder (or merge the matching folders).
2. android:icon="@mipmap/ic_launcher" and
   android:roundIcon="@mipmap/ic_launcher_round" use the legacy icons.
3. Android 8.0+ reads res/mipmap-anydpi-v26/ic_launcher.xml. Change the
   background color in res/values/ic_launcher_background.xml.
4. playstore/ic_launcher-playstore.png is the Google Play Console store icon.

ImageMagick has written every density, so you can copy the files directly.
`

var iosContentsJSON = `{
  "images" : [
    { "filename" : "Icon-20@2x.png", "idiom" : "iphone", "scale" : "2x", "size" : "20x20" },
    { "filename" : "Icon-20@3x.png", "idiom" : "iphone", "scale" : "3x", "size" : "20x20" },
    { "filename" : "Icon-29@2x.png", "idiom" : "iphone", "scale" : "2x", "size" : "29x29" },
    { "filename" : "Icon-29@3x.png", "idiom" : "iphone", "scale" : "3x", "size" : "29x29" },
    { "filename" : "Icon-40@2x.png", "idiom" : "iphone", "scale" : "2x", "size" : "40x40" },
    { "filename" : "Icon-40@3x.png", "idiom" : "iphone", "scale" : "3x", "size" : "40x40" },
    { "filename" : "Icon-60@2x.png", "idiom" : "iphone", "scale" : "2x", "size" : "60x60" },
    { "filename" : "Icon-60@3x.png", "idiom" : "iphone", "scale" : "3x", "size" : "60x60" },
    { "filename" : "Icon-20.png", "idiom" : "ipad", "scale" : "1x", "size" : "20x20" },
    { "filename" : "Icon-20@2x.png", "idiom" : "ipad", "scale" : "2x", "size" : "20x20" },
    { "filename" : "Icon-29.png", "idiom" : "ipad", "scale" : "1x", "size" : "29x29" },
    { "filename" : "Icon-29@2x.png", "idiom" : "ipad", "scale" : "2x", "size" : "29x29" },
    { "filename" : "Icon-40.png", "idiom" : "ipad", "scale" : "1x", "size" : "40x40" },
    { "filename" : "Icon-40@2x.png", "idiom" : "ipad", "scale" : "2x", "size" : "40x40" },
    { "filename" : "Icon-76.png", "idiom" : "ipad", "scale" : "1x", "size" : "76x76" },
    { "filename" : "Icon-76@2x.png", "idiom" : "ipad", "scale" : "2x", "size" : "76x76" },
    { "filename" : "Icon-83.5@2x.png", "idiom" : "ipad", "scale" : "2x", "size" : "83.5x83.5" },
    { "filename" : "Icon-1024.png", "idiom" : "ios-marketing", "scale" : "1x", "size" : "1024x1024" }
  ],
  "info" : {
    "author" : "xcode",
    "version" : 1
  }
}
`

var iosReadme = `iOS 图标输出
============

1. 把 AppIcon.appiconset 整个目录拖进 Xcode 的 Assets.xcassets。
2. 在 Target > General > App Icons and Launch Screen 选择 AppIcon。
3. 所有 PNG 都已去除 Alpha 通道（App Store 要求不透明图标）。
4. Xcode 14+ 也可以只使用 Icon-1024.png 作为单尺寸图标，其余尺寸
   由 Xcode 自动生成；本目录同时提供完整尺寸，兼容旧版 Xcode。

App Store Connect 的 1024 x 1024 图标就是 Icon-1024.png。
`

var iosReadmeEN = `iOS icon output
===============

1. Drag the whole AppIcon.appiconset folder into Assets.xcassets in Xcode.
2. Choose AppIcon under Target > General > App Icons and Launch Screen.
3. Every PNG has no alpha channel, as the App Store requires.
4. Xcode 14+ can use only Icon-1024.png and generate the other sizes; this
   folder also includes every size for older Xcode versions.

The 1024 x 1024 App Store Connect icon is Icon-1024.png.
`

var winReadme = `Windows 图标输出
================

1. app.ico 包含 16/24/32/48/64/128/256 多尺寸，可嵌入 exe 资源，
   也可用于 Inno Setup、NSIS、electron-builder 等安装包工具。
2. Assets/ 目录是 MSIX / WinUI 3 的 Package.appxmanifest 资源：
   - Square44x44Logo.png        应用列表
   - Square150x150Logo.png      开始菜单中等磁贴
   - Wide310x150Logo.png        宽磁贴
   - Square310x310Logo.png      大磁贴
   - StoreLogo.png              Microsoft Store
   - SplashScreen.png           启动画面
   - BadgeLogo.png              锁屏徽章（应为单色）
3. 每个资源同时提供 .scale-200.png 二倍图，可按需删除不需要的缩放级别。
   资源文件名要写进 Package.appxmanifest 的 uap:VisualElements / uap:SplashScreen。
`

var winReadmeEN = `Windows icon output
==================

1. app.ico contains 16/24/32/48/64/128/256 sizes. Embed it in the exe
   resources, or use it with Inno Setup, NSIS, electron-builder, and
   similar packaging tools.
2. Assets/ holds the MSIX / WinUI 3 Package.appxmanifest assets:
   - Square44x44Logo.png        app list
   - Square150x150Logo.png      medium Start tile
   - Wide310x150Logo.png        wide tile
   - Square310x310Logo.png      large tile
   - StoreLogo.png              Microsoft Store
   - SplashScreen.png           splash screen
   - BadgeLogo.png              lock screen badge (must be monochrome)
3. Every asset also has a .scale-200.png variant. Delete the scales you do
   not use. Reference the file names in Package.appxmanifest under
   uap:VisualElements / uap:SplashScreen.
`

var linuxReadme = `Linux 图标输出
==============

1. 把 share/icons/hicolor 复制到 /usr/share/icons/ 或
   ~/.local/share/icons/。
2. 把 {name}.desktop 复制到 /usr/share/applications/ 或
   ~/.local/share/applications/，按需修改 Exec= 和 Name=。
3. hicolor 主题包含 16 到 512 像素，桌面环境会挑选最合适的尺寸。
4. AppImage: 使用 256x256 或 512x512 的 PNG 作为 .DirIcon。
5. 安装后可以运行 update-desktop-database 和 gtk-update-icon-cache。
`

var linuxReadmeEN = `Linux icon output
=================

1. Copy share/icons/hicolor into /usr/share/icons/ or
   ~/.local/share/icons/.
2. Copy {name}.desktop into /usr/share/applications/ or
   ~/.local/share/applications/, and adjust Exec= and Name= if needed.
3. The hicolor theme includes 16 to 512 pixels; the desktop environment
   picks the best size.
4. AppImage: use the 256x256 or 512x512 PNG as .DirIcon.
5. After installing, run update-desktop-database and
   gtk-update-icon-cache.
`

var macContentsJSON = `{
  "images" : [
    { "filename" : "icon_16x16.png", "idiom" : "mac", "scale" : "1x", "size" : "16x16" },
    { "filename" : "icon_16x16@2x.png", "idiom" : "mac", "scale" : "2x", "size" : "16x16" },
    { "filename" : "icon_32x32.png", "idiom" : "mac", "scale" : "1x", "size" : "32x32" },
    { "filename" : "icon_32x32@2x.png", "idiom" : "mac", "scale" : "2x", "size" : "32x32" },
    { "filename" : "icon_128x128.png", "idiom" : "mac", "scale" : "1x", "size" : "128x128" },
    { "filename" : "icon_128x128@2x.png", "idiom" : "mac", "scale" : "2x", "size" : "128x128" },
    { "filename" : "icon_256x256.png", "idiom" : "mac", "scale" : "1x", "size" : "256x256" },
    { "filename" : "icon_256x256@2x.png", "idiom" : "mac", "scale" : "2x", "size" : "256x256" },
    { "filename" : "icon_512x512.png", "idiom" : "mac", "scale" : "1x", "size" : "512x512" },
    { "filename" : "icon_512x512@2x.png", "idiom" : "mac", "scale" : "2x", "size" : "512x512" }
  ],
  "info" : {
    "author" : "xcode",
    "version" : 1
  }
}
`

var macReadme = `macOS 图标输出
==============

1. AppIcon.appiconset 可以拖进 Xcode 的 Assets.xcassets，命名为 AppIcon。
2. AppIcon.icns 可直接用于 electron-builder、Tauri、PyInstaller 等
   使用 .icns 的打包工具。
3. 如果 ImageMagick 无法写入 ICNS，本工具会用内置写入器从 iconset PNG
   生成 .icns；也可以在 macOS 上运行：
      iconutil -c icns AppIcon.appiconset -o AppIcon.icns
4. App Store Connect 的 1024 图标是 icon_512x512@2x.png。若审核提示
   Alpha 通道问题，请将 1024 图导出为不带透明的 PNG。
`

var macReadmeEN = `macOS icon output
=================

1. Drag AppIcon.appiconset into Assets.xcassets in Xcode and name it AppIcon.
2. AppIcon.icns works directly with electron-builder, Tauri, PyInstaller and
   other tools that take .icns.
3. If ImageMagick cannot write ICNS, this tool builds it from the iconset
   PNGs with its built-in writer. You can also run this on macOS:
      iconutil -c icns AppIcon.appiconset -o AppIcon.icns
4. The 1024 App Store Connect icon is icon_512x512@2x.png. If review reports
   an alpha channel problem, export the 1024 image as an opaque PNG.
`

// Platforms is the ordered list shown in the UI. Sizes follow each
// platform's current official guidance; the Docs fields carry the details.
var Platforms = []Platform{
	{
		Key:     "android",
		Name:    "Android",
		Summary: "启动图标、自适应图标、圆形图标与 Google Play 商店图标",
		Icons: []Icon{
			icon("mipmap-mdpi", 48, "res/mipmap-mdpi/ic_launcher.png"),
			icon("mipmap-hdpi", 72, "res/mipmap-hdpi/ic_launcher.png"),
			icon("mipmap-xhdpi", 96, "res/mipmap-xhdpi/ic_launcher.png"),
			icon("mipmap-xxhdpi", 144, "res/mipmap-xxhdpi/ic_launcher.png"),
			icon("mipmap-xxxhdpi", 192, "res/mipmap-xxxhdpi/ic_launcher.png"),
			icon("mipmap-mdpi round", 48, "res/mipmap-mdpi/ic_launcher_round.png"),
			icon("mipmap-hdpi round", 72, "res/mipmap-hdpi/ic_launcher_round.png"),
			icon("mipmap-xhdpi round", 96, "res/mipmap-xhdpi/ic_launcher_round.png"),
			icon("mipmap-xxhdpi round", 144, "res/mipmap-xxhdpi/ic_launcher_round.png"),
			icon("mipmap-xxxhdpi round", 192, "res/mipmap-xxxhdpi/ic_launcher_round.png"),
			{Label: "adaptive foreground mdpi", W: 108, H: 108, Rel: "res/mipmap-mdpi/ic_launcher_foreground.png", Inset: 1.0 / 6},
			{Label: "adaptive foreground hdpi", W: 162, H: 162, Rel: "res/mipmap-hdpi/ic_launcher_foreground.png", Inset: 1.0 / 6},
			{Label: "adaptive foreground xhdpi", W: 216, H: 216, Rel: "res/mipmap-xhdpi/ic_launcher_foreground.png", Inset: 1.0 / 6},
			{Label: "adaptive foreground xxhdpi", W: 324, H: 324, Rel: "res/mipmap-xxhdpi/ic_launcher_foreground.png", Inset: 1.0 / 6},
			{Label: "adaptive foreground xxxhdpi", W: 432, H: 432, Rel: "res/mipmap-xxxhdpi/ic_launcher_foreground.png", Inset: 1.0 / 6},
			icon("Google Play store", 512, "playstore/ic_launcher-playstore.png"),
		},
		Static: []Static{
			{Rel: "res/mipmap-anydpi-v26/ic_launcher.xml", Content: androidAdaptiveXML},
			{Rel: "res/mipmap-anydpi-v26/ic_launcher_round.xml", Content: androidAdaptiveXML},
			{Rel: "res/values/ic_launcher_background.xml", Content: androidColorXML},
			{Rel: "README.txt", Content: androidReadme, EN: androidReadmeEN},
		},
		Notes: []string{
			"传统图标为 32-bit PNG，建议保留透明背景。",
			"自适应图标画布 108dp，中心 72dp 可见，内容放在直径 66dp 的安全区内；本工具已将前景缩进 1/6。",
			"mipmap-anydpi-v26 XML 会让 Android 8.0+ 自动使用自适应图标，背景色可在 res/values 中修改。",
			"Google Play 商店图标 512×512、32-bit PNG、不超过 1 MB。",
		},
		Links: []Link{
			{"Android 自适应图标", "https://developer.android.com/develop/ui/views/launch/icon_design_adaptive"},
			{"Google Play 图标规范", "https://support.google.com/googleplay/android-developer/answer/9866151"},
		},
		Docs: []DocRow{
			{"传统启动图标 ic_launcher", "48 / 72 / 96 / 144 / 192 px", "PNG 32-bit，可透明", "mdpi / hdpi / xhdpi / xxhdpi / xxxhdpi"},
			{"圆形图标 ic_launcher_round", "48 / 72 / 96 / 144 / 192 px", "PNG 32-bit，可透明", "Android 7.1+ 圆形启动器"},
			{"自适应前景 ic_launcher_foreground", "108 / 162 / 216 / 324 / 432 px", "PNG 32-bit，可透明", "108dp 画布，72dp 可见，66dp 安全区"},
			{"Google Play 商店图标", "512 × 512 px", "32-bit PNG，≤ 1 MB", "上传到 Play Console 商店信息"},
		},
	},
	{
		Key:     "ios",
		Name:    "iOS",
		Summary: "iPhone、iPad 与 App Store 的全套 AppIcon",
		Icons: []Icon{
			{Label: "iPhone 20pt @2x", W: 40, H: 40, Rel: "AppIcon.appiconset/Icon-20@2x.png", Opaque: true},
			{Label: "iPhone 20pt @3x", W: 60, H: 60, Rel: "AppIcon.appiconset/Icon-20@3x.png", Opaque: true},
			{Label: "iPhone 29pt @2x", W: 58, H: 58, Rel: "AppIcon.appiconset/Icon-29@2x.png", Opaque: true},
			{Label: "iPhone 29pt @3x", W: 87, H: 87, Rel: "AppIcon.appiconset/Icon-29@3x.png", Opaque: true},
			{Label: "iPhone 40pt @2x", W: 80, H: 80, Rel: "AppIcon.appiconset/Icon-40@2x.png", Opaque: true},
			{Label: "iPhone 40pt @3x", W: 120, H: 120, Rel: "AppIcon.appiconset/Icon-40@3x.png", Opaque: true},
			{Label: "iPhone 60pt @2x", W: 120, H: 120, Rel: "AppIcon.appiconset/Icon-60@2x.png", Opaque: true},
			{Label: "iPhone 60pt @3x", W: 180, H: 180, Rel: "AppIcon.appiconset/Icon-60@3x.png", Opaque: true},
			{Label: "iPad 20pt @1x", W: 20, H: 20, Rel: "AppIcon.appiconset/Icon-20.png", Opaque: true},
			{Label: "iPad 29pt @1x", W: 29, H: 29, Rel: "AppIcon.appiconset/Icon-29.png", Opaque: true},
			{Label: "iPad 40pt @1x", W: 40, H: 40, Rel: "AppIcon.appiconset/Icon-40.png", Opaque: true},
			{Label: "iPad 76pt @1x", W: 76, H: 76, Rel: "AppIcon.appiconset/Icon-76.png", Opaque: true},
			{Label: "iPad 76pt @2x", W: 152, H: 152, Rel: "AppIcon.appiconset/Icon-76@2x.png", Opaque: true},
			{Label: "iPad 83.5pt @2x", W: 167, H: 167, Rel: "AppIcon.appiconset/Icon-83.5@2x.png", Opaque: true},
			{Label: "App Store 1024", W: 1024, H: 1024, Rel: "AppIcon.appiconset/Icon-1024.png", Opaque: true},
		},
		Static: []Static{
			{Rel: "AppIcon.appiconset/Contents.json", Content: iosContentsJSON},
			{Rel: "README.txt", Content: iosReadme, EN: iosReadmeEN},
		},
		Notes: []string{
			"App Store 要求图标为 PNG，且不能带 Alpha 通道；本工具已按背景色压平透明区域。",
			"iOS 系统会自动裁切圆角，请提供满幅方形图，不要在图片里预先画圆角。",
			"Xcode 14+ 可用单张 1024×1024 图标，完整尺寸集兼容旧版 Xcode。",
		},
		Links: []Link{
			{"Apple 人机界面指南 · App 图标", "https://developer.apple.com/design/human-interface-guidelines/app-icons"},
			{"Xcode AppIcon 尺寸", "https://developer.apple.com/documentation/xcode/configuring-your-app-icon"},
		},
		Docs: []DocRow{
			{"iPhone 图标", "40, 60, 58, 87, 80, 120, 180 px", "PNG，无 Alpha", "20/29/40/60 pt 的 @2x、@3x"},
			{"iPad 图标", "20, 40, 29, 58, 76, 152, 167 px", "PNG，无 Alpha", "20/29/40/76/83.5 pt 的 @1x、@2x"},
			{"App Store 图标", "1024 × 1024 px", "PNG，无 Alpha", "App Store Connect 与 Xcode 营销图标"},
		},
	},
	{
		Key:     "windows",
		Name:    "Windows",
		Summary: "多尺寸 ICO 与 MSIX / WinUI 磁贴、启动画面资源",
		Bundles: []Bundle{
			{Label: "app.ico (16-256)", Format: "ico", Rel: "app.ico"},
		},
		Icons: []Icon{
			icon("Square44x44Logo", 44, "Assets/Square44x44Logo.png"),
			icon("Square44x44Logo scale-200", 88, "Assets/Square44x44Logo.scale-200.png"),
			icon("Square150x150Logo", 150, "Assets/Square150x150Logo.png"),
			icon("Square150x150Logo scale-200", 300, "Assets/Square150x150Logo.scale-200.png"),
			{Label: "Wide310x150Logo", W: 310, H: 150, Rel: "Assets/Wide310x150Logo.png"},
			{Label: "Wide310x150Logo scale-200", W: 620, H: 300, Rel: "Assets/Wide310x150Logo.scale-200.png"},
			icon("Square310x310Logo", 310, "Assets/Square310x310Logo.png"),
			icon("Square310x310Logo scale-200", 620, "Assets/Square310x310Logo.scale-200.png"),
			icon("StoreLogo", 50, "Assets/StoreLogo.png"),
			icon("StoreLogo scale-200", 100, "Assets/StoreLogo.scale-200.png"),
			{Label: "SplashScreen", W: 620, H: 300, Rel: "Assets/SplashScreen.png"},
			{Label: "SplashScreen scale-200", W: 1240, H: 600, Rel: "Assets/SplashScreen.scale-200.png"},
			icon("BadgeLogo", 24, "Assets/BadgeLogo.png"),
			icon("BadgeLogo scale-200", 48, "Assets/BadgeLogo.scale-200.png"),
		},
		Static: []Static{
			{Rel: "README.txt", Content: winReadme, EN: winReadmeEN},
		},
		Notes: []string{
			"ICO 内含 16/24/32/48/64/128/256 多个尺寸，Windows 会按 DPI 自动选择。",
			"MSIX 资源默认输出 100% 和 200% 两个缩放级别，可删掉不用的文件。",
			"BadgeLogo 必须是单色（通常为白色 + 透明），本工具只是缩放原图，请自行确认颜色。",
		},
		Links: []Link{
			{"Windows 应用图标指南", "https://learn.microsoft.com/windows/apps/design/style/iconography/app-icon-design"},
			{"MSIX 视觉资源", "https://learn.microsoft.com/windows/apps/design/style/iconography/app-icon-construction"},
		},
		Docs: []DocRow{
			{"应用图标 app.ico", "16 / 24 / 32 / 48 / 64 / 128 / 256 px", "ICO 多尺寸", "exe 资源、任务栏、开始菜单快捷方式"},
			{"Square44x44Logo", "44 px（+ 88 px @200%）", "PNG", "应用列表与任务栏"},
			{"Square150x150Logo", "150 px（+ 300 px @200%）", "PNG", "开始菜单中等磁贴"},
			{"Wide310x150Logo", "310 × 150 px（+ 620 × 300）", "PNG", "开始菜单宽磁贴"},
			{"Square310x310Logo", "310 px（+ 620 px @200%）", "PNG", "开始菜单大磁贴"},
			{"StoreLogo", "50 px（+ 100 px @200%）", "PNG", "Microsoft Store"},
			{"SplashScreen", "620 × 300 px（+ 1240 × 600）", "PNG", "应用启动画面"},
			{"BadgeLogo", "24 px（+ 48 px @200%）", "PNG，单色", "锁屏徽章"},
		},
	},
	{
		Key:     "linux",
		Name:    "Linux",
		Summary: "Freedesktop hicolor 图标主题与 .desktop 启动器",
		Icons: []Icon{
			icon("hicolor 16", 16, "share/icons/hicolor/16x16/apps/{name}.png"),
			icon("hicolor 22", 22, "share/icons/hicolor/22x22/apps/{name}.png"),
			icon("hicolor 24", 24, "share/icons/hicolor/24x24/apps/{name}.png"),
			icon("hicolor 32", 32, "share/icons/hicolor/32x32/apps/{name}.png"),
			icon("hicolor 48", 48, "share/icons/hicolor/48x48/apps/{name}.png"),
			icon("hicolor 64", 64, "share/icons/hicolor/64x64/apps/{name}.png"),
			icon("hicolor 128", 128, "share/icons/hicolor/128x128/apps/{name}.png"),
			icon("hicolor 256", 256, "share/icons/hicolor/256x256/apps/{name}.png"),
			icon("hicolor 512", 512, "share/icons/hicolor/512x512/apps/{name}.png"),
		},
		Static: []Static{
			{Rel: "{name}.desktop", Content: linuxDesktop},
			{Rel: "README.txt", Content: linuxReadme, EN: linuxReadmeEN},
		},
		Notes: []string{
			"hicolor 是所有桌面环境都支持的图标主题，16 到 512 像素按目录名区分尺寸。",
			"PNG 支持透明背景；系统会按面板/启动器需要的尺寸挑选最接近的一张。",
			".desktop 文件里的 Icon= 必须是图标文件的基础名（不含扩展名）。",
			"AppImage 通常使用 256×256 或 512×512 的 PNG 作为 .DirIcon。",
		},
		Links: []Link{
			{"Freedesktop 图标主题规范", "https://specifications.freedesktop.org/icon-theme-spec/latest/"},
			{"Desktop Entry 规范", "https://specifications.freedesktop.org/desktop-entry-spec/latest/"},
		},
		Docs: []DocRow{
			{"hicolor 图标", "16 / 22 / 24 / 32 / 48 / 64 / 128 / 256 / 512 px", "PNG，可透明", "share/icons/hicolor/<N>x<N>/apps/<name>.png"},
			{"桌面启动器", "—", ".desktop 文本文件", "Icon= 使用文件名去掉 .png 后的名字"},
			{"AppImage 图标", "256 × 256 或 512 × 512 px", "PNG", "作为 AppDir 的 .DirIcon"},
		},
	},
	{
		Key:     "macos",
		Name:    "macOS",
		Summary: "AppIcon.appiconset、ICNS 与 App Store 1024 图标",
		Icons: []Icon{
			icon("iconset 16", 16, "AppIcon.appiconset/icon_16x16.png"),
			icon("iconset 16@2x", 32, "AppIcon.appiconset/icon_16x16@2x.png"),
			icon("iconset 32", 32, "AppIcon.appiconset/icon_32x32.png"),
			icon("iconset 32@2x", 64, "AppIcon.appiconset/icon_32x32@2x.png"),
			icon("iconset 128", 128, "AppIcon.appiconset/icon_128x128.png"),
			icon("iconset 128@2x", 256, "AppIcon.appiconset/icon_128x128@2x.png"),
			icon("iconset 256", 256, "AppIcon.appiconset/icon_256x256.png"),
			icon("iconset 256@2x", 512, "AppIcon.appiconset/icon_256x256@2x.png"),
			icon("iconset 512", 512, "AppIcon.appiconset/icon_512x512.png"),
			icon("iconset 512@2x", 1024, "AppIcon.appiconset/icon_512x512@2x.png"),
		},
		Bundles: []Bundle{
			{Label: "AppIcon.icns", Format: "icns", Rel: "AppIcon.icns", SourceSizes: []int{16, 32, 64, 128, 256, 512, 1024}},
		},
		Static: []Static{
			{Rel: "AppIcon.appiconset/Contents.json", Content: macContentsJSON},
			{Rel: "README.txt", Content: macReadme, EN: macReadmeEN},
		},
		Notes: []string{
			"macOS 图标通常保留透明区域（圆角与阴影），本工具不会压平背景。",
			"App Store 的 1024×1024 营销图标不能带 Alpha；如审核提示，请单独导出不透明版本。",
			"AppIcon.icns 由 ImageMagick 生成；失败时自动改用内置 ICNS 写入器。",
		},
		Links: []Link{
			{"Apple 人机界面指南 · macOS 图标", "https://developer.apple.com/design/human-interface-guidelines/app-icons"},
			{"Xcode AppIcon 尺寸", "https://developer.apple.com/documentation/xcode/configuring-your-app-icon"},
		},
		Docs: []DocRow{
			{"AppIcon.appiconset", "16 / 32 / 64 / 128 / 256 / 512 / 1024 px", "PNG，可透明", "16/32/128/256/512 pt 的 @1x、@2x"},
			{"AppIcon.icns", "16 - 1024 px", "ICNS 多尺寸", "应用包 Contents/Resources 使用"},
			{"App Store 图标", "1024 × 1024 px", "PNG，无 Alpha", "App Store Connect 营销图标"},
		},
	},
}

var linuxDesktop = `[Desktop Entry]
Type=Application
Name={name}
Comment={name}
Exec={name}
Icon={name}
Terminal=false
Categories=Utility;
StartupNotify=true
`

// RelFor returns the output path of rel under a platform directory with the
// app name substituted.
func RelFor(platformKey, rel, name string) string {
	return filepath.Join(platformKey, strings.ReplaceAll(rel, "{name}", name))
}
