package annotate

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

var (
	fontMu     sync.Mutex
	fontSource *sfnt.Font
	fontTried  bool
	faceCache  = map[int]font.Face{}
)

// fontCandidates lists fonts that include CJK glyphs before the Latin-only
// fallback, so annotation text in any of the app's languages draws.
func fontCandidates() []string {
	if env := os.Getenv("ZCHAT_ANNOTATE_FONT"); env != "" {
		return []string{env}
	}
	switch runtime.GOOS {
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		if dir == "Fonts" {
			dir = `C:\Windows\Fonts`
		}
		return []string{
			filepath.Join(dir, "msyh.ttc"),
			filepath.Join(dir, "msyhbd.ttc"),
			filepath.Join(dir, "simhei.ttf"),
			filepath.Join(dir, "simsun.ttc"),
			filepath.Join(dir, "segoeui.ttf"),
			filepath.Join(dir, "arial.ttf"),
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/Helvetica.ttc",
			"/System/Library/Fonts/SFNS.ttf",
			"/Library/Fonts/Arial Unicode.ttf",
		}
	default:
		return []string{
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
		}
	}
}

func loadFontSource() *sfnt.Font {
	fontMu.Lock()
	defer fontMu.Unlock()
	if fontTried {
		return fontSource
	}
	fontTried = true
	for _, path := range fontCandidates() {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		if coll, err := opentype.ParseCollection(data); err == nil && coll.NumFonts() > 0 {
			if f, err := coll.Font(0); err == nil {
				fontSource = f
				return fontSource
			}
		}
		if f, err := opentype.Parse(data); err == nil {
			fontSource = f
			return fontSource
		}
	}
	if f, err := opentype.Parse(goregular.TTF); err == nil {
		fontSource = f
	}
	return fontSource
}

func faceFor(size float32) font.Face {
	if size < 6 {
		size = 6
	}
	if size > 512 {
		size = 512
	}
	key := int(size * 4)
	fontMu.Lock()
	if f, ok := faceCache[key]; ok {
		fontMu.Unlock()
		return f
	}
	src := fontSource
	fontMu.Unlock()
	if src == nil {
		src = loadFontSource()
	}
	if src == nil {
		return nil
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	fontMu.Lock()
	faceCache[key] = face
	fontMu.Unlock()
	return face
}

// Measure returns the width and line height of text at the given pixel size.
func Measure(text string, size float32) (float32, float32) {
	face := faceFor(size)
	lines := strings.Split(text, "\n")
	if face == nil {
		w := float32(0)
		for _, line := range lines {
			if n := float32(len([]rune(line))) * size * 0.6; n > w {
				w = n
			}
		}
		return w, float32(len(lines)) * size * 1.25
	}
	var w float32
	for _, line := range lines {
		if adv := float32(font.MeasureString(face, line)); adv > w {
			w = adv
		}
	}
	return w, float32(len(lines)) * size * 1.25
}

// ascent returns the font's ascent for placing the first baseline.
func ascent(face font.Face, size float32) float32 {
	if face == nil {
		return size
	}
	return float32(face.Metrics().Ascent) / 64
}

func drawText(dst *image.RGBA, text string, at Point, size float32, c color.RGBA) {
	if text == "" {
		return
	}
	face := faceFor(size)
	if face == nil {
		return
	}
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
	}
	lineHeight := size * 1.25
	for i, line := range strings.Split(text, "\n") {
		baseline := at.Y + ascent(face, size) + float32(i)*lineHeight
		d.Dot = fixed.P(int(at.X), int(baseline))
		d.DrawString(line)
	}
}
