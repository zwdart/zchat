// Package magick plans and runs the general ImageMagick tasks the app
// offers: format conversion, resizing, cropping, batch processing,
// watermarking, composition and quality adjustment. It only builds command
// lines and reports progress; the actual work is done by the installed
// ImageMagick.
package magick

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"xin.dart.zchat/internal/i18n"
)

// Tool is a detected ImageMagick installation.
type Tool struct {
	Path    string
	Version string
}

// Detect finds ImageMagick 7 ("magick") or ImageMagick 6 ("convert"). On
// Windows the system convert.exe (a FAT-to-NTFS tool) is rejected because it
// does not print an ImageMagick version.
func Detect() (Tool, error) {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{"magick.exe", "magick", "convert.exe", "convert"}
	} else {
		candidates = []string{"magick", "convert"}
	}
	var lastErr error
	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err != nil {
			lastErr = err
			continue
		}
		tool, err := probeTool(path)
		if err == nil {
			return tool, nil
		}
		lastErr = err
	}
	if lastErr == nil || errors.Is(lastErr, exec.ErrNotFound) {
		lastErr = errors.New(i18n.T("未找到 ImageMagick"))
	}
	return Tool{}, lastErr
}

func probeTool(path string) (Tool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
	if err != nil {
		return Tool{}, fmt.Errorf("%s -version: %w", path, err)
	}
	text := strings.TrimSpace(string(out))
	first := text
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if !strings.Contains(first, "ImageMagick") {
		return Tool{}, fmt.Errorf("%s%s", path, i18n.T(" 不是 ImageMagick"))
	}
	return Tool{Path: path, Version: strings.TrimSpace(first)}, nil
}

// Kind names one toolkit capability.
type Kind string

const (
	KindConvert   Kind = "convert"
	KindGeometry  Kind = "geometry"
	KindBatch     Kind = "batch"
	KindWatermark Kind = "watermark"
	KindCompose   Kind = "compose"
	KindOptimize  Kind = "optimize"
)

// Geometry modes.
const (
	GeometryResize = "resize"
	GeometryCrop   = "crop"
)

// Resize modes.
const (
	ResizeFit     = "fit"
	ResizeFill    = "fill"
	ResizeStretch = "stretch"
)

// Compose layouts.
const (
	ComposeHorizontal = "horizontal"
	ComposeVertical   = "vertical"
	ComposeAnimation  = "animation"
)

// Watermark modes.
const (
	WatermarkText  = "text"
	WatermarkImage = "image"
)

// Options are the choices for one toolkit run.
type Options struct {
	Kind      Kind
	Inputs    []string
	OutputDir string

	// Format is a file extension such as "png", or "keep" for the input's
	// own format. Quality only applies to lossy formats.
	Format  string
	Quality int

	// Geometry.
	GeometryMode string
	ResizeMode   string
	Width        int
	Height       int
	Gravity      string

	// Watermark.
	WatermarkMode  string
	WatermarkText  string
	WatermarkImage string
	Position       string
	Opacity        float64
	FontSize       int
	Color          string

	// Compose.
	Layout string
	Delay  int

	// Optimize and batch.
	AutoLevel bool
	Strip     bool
}

// Validate checks the options that do not need ImageMagick. The UI runs it
// before starting so mistakes are reported immediately.
func Validate(opt Options) error {
	inputs := cleanInputs(opt.Inputs)
	if len(inputs) == 0 {
		return errors.New(i18n.T("请先选择图片"))
	}
	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return fmt.Errorf("%s%v", i18n.T("图片不存在："), err)
		}
		if info.IsDir() {
			return errors.New(i18n.T("选择的项目里包含文件夹"))
		}
	}
	switch opt.Kind {
	case KindCompose:
		if len(inputs) < 2 {
			return errors.New(i18n.T("图像合成至少需要两张图片"))
		}
	case KindConvert, KindGeometry, KindBatch, KindWatermark, KindOptimize:
	default:
		return errors.New(i18n.T("未知的处理类型"))
	}
	if strings.TrimSpace(opt.OutputDir) == "" {
		return errors.New(i18n.T("请选择输出目录"))
	}
	switch opt.Kind {
	case KindGeometry:
		if opt.GeometryMode == GeometryCrop {
			if opt.Width <= 0 || opt.Height <= 0 {
				return errors.New(i18n.T("裁剪需要填写宽度和高度"))
			}
		} else if opt.Width <= 0 && opt.Height <= 0 {
			return errors.New(i18n.T("缩放需要填写宽度或高度"))
		}
		if opt.GeometryMode != GeometryCrop && opt.ResizeMode == ResizeFill && (opt.Width <= 0 || opt.Height <= 0) {
			return errors.New(i18n.T("填充并裁剪需要同时填写宽度和高度"))
		}
	case KindWatermark:
		if opt.WatermarkMode == WatermarkImage {
			if strings.TrimSpace(opt.WatermarkImage) == "" {
				return errors.New(i18n.T("请选择水印图片"))
			}
			if info, err := os.Stat(opt.WatermarkImage); err != nil || info.IsDir() {
				return errors.New(i18n.T("水印图片不存在"))
			}
		} else if strings.TrimSpace(opt.WatermarkText) == "" {
			return errors.New(i18n.T("请输入水印文字"))
		}
	}
	return nil
}

// Step is one ImageMagick command and the file it writes.
type Step struct {
	Label string
	Out   string
	Args  []string
}

// Plan is the ordered list of files to produce.
type Plan struct {
	Tool      Tool
	Inputs    []string
	OutputDir string
	Format    string
	Steps     []Step
}

// Build validates the options and turns them into ImageMagick commands. It
// does not run anything, so it is cheap and safe to test.
func Build(tool Tool, opt Options) (*Plan, error) {
	if strings.TrimSpace(tool.Path) == "" {
		return nil, errors.New(i18n.T("未安装 ImageMagick"))
	}
	if err := Validate(opt); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(strings.TrimSpace(opt.OutputDir))
	if err != nil {
		return nil, err
	}
	format := normalizeFormat(opt.Format)
	quality := opt.Quality
	if quality <= 0 || quality > 100 {
		quality = 90
	}
	plan := &Plan{
		Tool:      tool,
		Inputs:    cleanInputs(opt.Inputs),
		OutputDir: root,
		Format:    format,
	}
	switch opt.Kind {
	case KindConvert:
		for _, in := range plan.Inputs {
			out := targetPath(root, in, "", format)
			args := []string{in, "-auto-orient"}
			args = append(args, formatArgs(format, quality)...)
			args = append(args, out)
			plan.Steps = append(plan.Steps, Step{
				Label: fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
				Out:   out,
				Args:  args,
			})
		}
	case KindGeometry:
		for _, in := range plan.Inputs {
			suffix := "-resized"
			args := []string{in, "-auto-orient"}
			if opt.GeometryMode == GeometryCrop {
				suffix = "-cropped"
				gravity := normalizeGravity(opt.Gravity)
				args = append(args,
					"-gravity", gravity,
					"-crop", fmt.Sprintf("%dx%d+0+0", opt.Width, opt.Height),
					"+repage",
				)
			} else {
				args = append(args, "-resize", resizeSpec(opt.Width, opt.Height, opt.ResizeMode))
				if opt.ResizeMode == ResizeFill {
					args = append(args,
						"-gravity", normalizeGravity(opt.Gravity),
						"-extent", fmt.Sprintf("%dx%d", opt.Width, opt.Height),
					)
				}
			}
			args = append(args, "-strip")
			args = append(args, formatArgs(format, quality)...)
			out := targetPath(root, in, suffix, format)
			args = append(args, out)
			plan.Steps = append(plan.Steps, Step{
				Label: fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
				Out:   out,
				Args:  args,
			})
		}
	case KindBatch:
		for _, in := range plan.Inputs {
			args := []string{in, "-auto-orient"}
			if opt.Width > 0 || opt.Height > 0 {
				args = append(args, "-resize", resizeSpec(opt.Width, opt.Height, ResizeFit)+">")
			}
			args = append(args, "-strip")
			args = append(args, formatArgs(format, quality)...)
			out := targetPath(root, in, "-batch", format)
			args = append(args, out)
			plan.Steps = append(plan.Steps, Step{
				Label: fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
				Out:   out,
				Args:  args,
			})
		}
	case KindWatermark:
		for _, in := range plan.Inputs {
			args := []string{in, "-auto-orient"}
			if opt.WatermarkMode == WatermarkImage {
				size := opt.FontSize
				if size <= 0 {
					size = 180
				}
				args = append(args,
					"(", opt.WatermarkImage,
					"-resize", fmt.Sprintf("%dx%d", size, size),
					"-alpha", "set",
				)
				opacity := clamp(opt.Opacity, 0.05, 1)
				if opacity < 1 {
					args = append(args,
						"-channel", "A",
						"-evaluate", "multiply", fmt.Sprintf("%.3f", opacity),
						"+channel",
					)
				}
				args = append(args,
					")",
					"-gravity", normalizeGravity(opt.Position),
					"-composite",
				)
			} else {
				size := opt.FontSize
				if size <= 0 {
					size = 36
				}
				color := strings.TrimSpace(opt.Color)
				if color == "" {
					color = "#ffffff"
				}
				args = append(args,
					"-gravity", normalizeGravity(opt.Position),
					"-fill", withAlpha(color, clamp(opt.Opacity, 0.05, 1)),
					"-pointsize", fmt.Sprintf("%d", size),
					"-annotate", "+0+0", opt.WatermarkText,
				)
			}
			args = append(args, "-strip")
			args = append(args, formatArgs(format, quality)...)
			out := targetPath(root, in, "-watermark", format)
			args = append(args, out)
			plan.Steps = append(plan.Steps, Step{
				Label: fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
				Out:   out,
				Args:  args,
			})
		}
	case KindCompose:
		outFormat := format
		if outFormat == "keep" {
			outFormat = "png"
		}
		suffix := "-merged"
		var args []string
		switch opt.Layout {
		case ComposeVertical:
			args = append(args, plan.Inputs...)
			args = append(args, "-append")
		case ComposeAnimation:
			suffix = "-animation"
			if outFormat != "gif" {
				outFormat = "gif"
			}
			delay := opt.Delay
			if delay <= 0 {
				delay = 20
			}
			args = append(args, "-delay", fmt.Sprintf("%d", delay))
			args = append(args, plan.Inputs...)
			args = append(args, "-loop", "0", "-layers", "Optimize")
		default:
			args = append(args, plan.Inputs...)
			args = append(args, "+append")
		}
		args = append(args, "-strip")
		out := targetPath(root, plan.Inputs[0], suffix, outFormat)
		args = append(args, out)
		plan.Steps = append(plan.Steps, Step{
			Label: fmt.Sprintf("%s → %s", i18n.T("合成图像"), filepath.Base(out)),
			Out:   out,
			Args:  args,
		})
	case KindOptimize:
		for _, in := range plan.Inputs {
			args := []string{in, "-auto-orient"}
			if opt.AutoLevel {
				args = append(args, "-auto-level")
			}
			if opt.Strip {
				args = append(args, "-strip")
			}
			args = append(args, formatArgs(format, quality)...)
			out := targetPath(root, in, "-optimized", format)
			args = append(args, out)
			plan.Steps = append(plan.Steps, Step{
				Label: fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
				Out:   out,
				Args:  args,
			})
		}
	}
	if len(plan.Steps) == 0 {
		return nil, errors.New(i18n.T("没有可处理的内容"))
	}
	return plan, nil
}

// Progress is reported before and after each step.
type Progress struct {
	Done    int
	Total   int
	Label   string
	Err     error
	Running bool
}

// Result summarizes a finished run.
type Result struct {
	Files    int
	Errors   []string
	Duration time.Duration
	Canceled bool
}

// Run creates the output folders and executes every step. Individual
// failures do not stop the run; they are collected so the UI can show
// exactly which files failed.
func (p *Plan) Run(ctx context.Context, report func(Progress)) Result {
	started := time.Now()
	res := Result{}
	total := len(p.Steps)
	if report != nil {
		report(Progress{Done: 0, Total: total, Label: i18n.T("准备输出目录"), Running: true})
	}
	for i, step := range p.Steps {
		if ctx.Err() != nil {
			res.Canceled = true
			break
		}
		if report != nil {
			report(Progress{Done: i, Total: total, Label: step.Label, Running: true})
		}
		var err error
		if err = os.MkdirAll(filepath.Dir(step.Out), 0o755); err == nil {
			err = p.runMagick(ctx, step)
		}
		if err != nil {
			if ctx.Err() != nil {
				res.Canceled = true
			} else {
				res.Errors = append(res.Errors, step.Label+"："+err.Error())
			}
		} else {
			res.Files++
		}
		if report != nil {
			report(Progress{Done: i + 1, Total: total, Label: step.Label, Err: err})
		}
	}
	res.Duration = time.Since(started)
	return res
}

func (p *Plan) runMagick(ctx context.Context, step Step) error {
	out, err := exec.CommandContext(ctx, p.Tool.Path, step.Args...).CombinedOutput()
	if err == nil {
		if _, statErr := os.Stat(step.Out); statErr == nil {
			return nil
		}
		err = errors.New(i18n.T("ImageMagick 没有写出文件"))
	}
	msg := strings.TrimSpace(string(out))
	if len(msg) > 400 {
		msg = msg[len(msg)-400:]
	}
	if msg == "" {
		return err
	}
	return fmt.Errorf("%v: %s", err, msg)
}

func cleanInputs(inputs []string) []string {
	out := make([]string, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		in = strings.TrimSpace(in)
		if in == "" || seen[in] {
			continue
		}
		seen[in] = true
		out = append(out, in)
	}
	return out
}

func normalizeFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	format = strings.TrimPrefix(format, ".")
	if format == "" {
		return "keep"
	}
	if format == "jpeg" {
		return "jpg"
	}
	if format == "tif" {
		return "tiff"
	}
	return format
}

func normalizeGravity(g string) string {
	switch g {
	case "northwest", "north", "northeast", "west", "center", "east", "southwest", "south", "southeast":
		return g
	}
	return "center"
}

func resizeSpec(w, h int, mode string) string {
	var spec string
	switch {
	case w > 0 && h > 0:
		spec = fmt.Sprintf("%dx%d", w, h)
	case w > 0:
		spec = fmt.Sprintf("%dx", w)
	default:
		spec = fmt.Sprintf("x%d", h)
	}
	switch mode {
	case ResizeFill:
		return spec + "^"
	case ResizeStretch:
		return spec + "!"
	default:
		return spec
	}
}

// formatArgs adds the options a target format needs. JPEG cannot store
// alpha, so transparent images are flattened onto white first.
func formatArgs(format string, quality int) []string {
	var args []string
	switch format {
	case "jpg", "jpeg":
		args = append(args,
			"-background", "#ffffff",
			"-alpha", "remove",
			"-alpha", "off",
		)
	}
	if format != "gif" && format != "png" && format != "bmp" {
		args = append(args, "-quality", fmt.Sprintf("%d", quality))
	}
	return args
}

func targetPath(root, input, suffix, format string) string {
	ext := normalizeFormat(format)
	if ext == "keep" {
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(input)), ".")
		if ext == "" {
			ext = "png"
		}
	}
	if ext == "jpeg" {
		ext = "jpg"
	}
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	out := filepath.Join(root, base+suffix+"."+ext)
	if samePath(out, input) {
		out = filepath.Join(root, base+suffix+"-out."+ext)
	}
	return out
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// withAlpha turns #rgb/#rrggbb into an 8-digit hex color understood by
// ImageMagick, keeping non-hex colors as they are.
func withAlpha(color string, opacity float64) string {
	if opacity >= 1 {
		return color
	}
	hex := color
	if strings.HasPrefix(hex, "#") {
		hex = hex[1:]
	}
	if !isHex(hex) {
		return color
	}
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	case 8:
		hex = hex[:6]
	default:
		return color
	}
	alpha := int(math.Round(clamp(opacity, 0, 1) * 255))
	return fmt.Sprintf("#%s%02x", hex, alpha)
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
