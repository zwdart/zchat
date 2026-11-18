package iconkit

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

// Options are the choices for one generation run.
type Options struct {
	Input      string
	OutputRoot string
	Name       string
	Background string
	Platforms  []string
}

// StepKind says how a step writes its output.
type StepKind int

const (
	StepIcon StepKind = iota + 1
	StepBundle
	StepStatic
)

// Source is a previously generated icon used to build a bundle.
type Source struct {
	Path string
	Size int
}

// Step is one unit of progress: one ImageMagick command or one small text
// file.
type Step struct {
	Kind     StepKind
	Platform string
	Label    string
	Out      string
	Args     []string
	Content  string
	Sources  []Source
}

// Plan is the complete, ordered list of files to produce.
type Plan struct {
	Tool       Tool
	Input      string
	OutputRoot string
	Name       string
	Background string
	Platforms  []Platform
	Steps      []Step
}

// Build validates the options and turns the platform specs into commands.
// It does not run anything, so it is cheap and safe to test.
func Build(tool Tool, opt Options) (*Plan, error) {
	if strings.TrimSpace(tool.Path) == "" {
		return nil, errors.New(i18n.T("未安装 ImageMagick"))
	}
	input := strings.TrimSpace(opt.Input)
	if input == "" {
		return nil, errors.New(i18n.T("请选择一张源图片"))
	}
	info, err := os.Stat(input)
	if err != nil {
		return nil, fmt.Errorf("%s%v", i18n.T("源图片不存在："), err)
	}
	if info.IsDir() {
		return nil, errors.New(i18n.T("源图片是一个目录"))
	}
	root := strings.TrimSpace(opt.OutputRoot)
	if root == "" {
		return nil, errors.New(i18n.T("请选择输出目录"))
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	name := SafeName(opt.Name)
	bg := strings.TrimSpace(opt.Background)
	if bg == "" {
		bg = "#ffffff"
	}
	plats := Selected(opt.Platforms)
	if len(plats) == 0 {
		return nil, errors.New(i18n.T("请至少选择一个平台"))
	}

	plan := &Plan{
		Tool:       tool,
		Input:      input,
		OutputRoot: root,
		Name:       name,
		Background: bg,
		Platforms:  plats,
	}
	seen := map[string]bool{}
	for _, platform := range plats {
		base := filepath.Join(root, platform.Key)
		sources := map[int]string{}
		for _, ic := range platform.Icons {
			rel := strings.ReplaceAll(ic.Rel, "{name}", name)
			out := filepath.Join(base, filepath.FromSlash(rel))
			if seen[out] {
				continue
			}
			seen[out] = true
			iw, ih := insetSize(ic.W, ic.H, ic.Inset)
			args := []string{
				input,
				"-auto-orient",
				"-resize", fmt.Sprintf("%dx%d", iw, ih),
				"-background", bg,
				"-gravity", "center",
				"-extent", fmt.Sprintf("%dx%d", ic.W, ic.H),
			}
			if ic.Opaque {
				args = append(args, "-alpha", "remove", "-alpha", "off")
			}
			args = append(args, "-strip", pngTarget(out, ic.Opaque))
			plan.Steps = append(plan.Steps, Step{
				Kind:     StepIcon,
				Platform: platform.Key,
				Label:    fmt.Sprintf("%s · %s %d×%d", platform.Name, ic.Label, ic.W, ic.H),
				Out:      out,
				Args:     args,
			})
			if ic.W == ic.H {
				if _, ok := sources[ic.W]; !ok {
					sources[ic.W] = out
				}
			}
		}
		for _, b := range platform.Bundles {
			rel := strings.ReplaceAll(b.Rel, "{name}", name)
			out := filepath.Join(base, filepath.FromSlash(rel))
			if seen[out] {
				continue
			}
			seen[out] = true
			step := Step{
				Kind:     StepBundle,
				Platform: platform.Key,
				Label:    fmt.Sprintf("%s · %s", platform.Name, b.Label),
				Out:      out,
			}
			switch b.Format {
			case "ico":
				step.Args = []string{
					input,
					"-auto-orient",
					"-resize", "256x256",
					"-background", "none",
					"-gravity", "center",
					"-extent", "256x256",
					"-define", "icon:auto-resize=256,128,64,48,32,24,16",
					"-strip",
					"ICO:" + out,
				}
			case "icns":
				for _, size := range b.SourceSizes {
					if path, ok := sources[size]; ok {
						step.Sources = append(step.Sources, Source{Path: path, Size: size})
					}
				}
				sort.Slice(step.Sources, func(i, j int) bool { return step.Sources[i].Size < step.Sources[j].Size })
				if len(step.Sources) == 0 {
					return nil, fmt.Errorf(i18n.T("无法为 %s 找到 ICNS 源图标"), platform.Name)
				}
				for _, src := range step.Sources {
					step.Args = append(step.Args, src.Path)
				}
				step.Args = append(step.Args, "-strip", "ICNS:"+out)
			default:
				return nil, fmt.Errorf("%s%s", i18n.T("未知的图标容器格式："), b.Format)
			}
			plan.Steps = append(plan.Steps, step)
		}
		for _, st := range platform.Static {
			rel := strings.ReplaceAll(st.Rel, "{name}", name)
			out := filepath.Join(base, filepath.FromSlash(rel))
			if seen[out] {
				continue
			}
			seen[out] = true
			content := st.Content
			if st.EN != "" && i18n.Current() == i18n.English {
				content = st.EN
			}
			content = strings.NewReplacer("{name}", name, "{color}", bg).Replace(content)
			plan.Steps = append(plan.Steps, Step{
				Kind:     StepStatic,
				Platform: platform.Key,
				Label:    fmt.Sprintf("%s · %s", platform.Name, filepath.ToSlash(rel)),
				Out:      out,
				Content:  content,
			})
		}
	}
	if len(plan.Steps) == 0 {
		return nil, errors.New(i18n.T("选择的平台没有可生成的文件"))
	}
	return plan, nil
}

func insetSize(w, h int, inset float64) (int, int) {
	if inset <= 0 {
		return w, h
	}
	iw := int(math.Round(float64(w) * (1 - 2*inset)))
	ih := int(math.Round(float64(h) * (1 - 2*inset)))
	if iw < 1 {
		iw = 1
	}
	if ih < 1 {
		ih = 1
	}
	return iw, ih
}

// pngTarget forces 32-bit RGBA for transparent icons and 24-bit RGB for
// icons that must not carry an alpha channel (iOS).
func pngTarget(out string, opaque bool) string {
	if opaque {
		return "PNG24:" + out
	}
	return "PNG32:" + out
}

// Progress is reported before and after each step.
type Progress struct {
	Done     int
	Total    int
	Label    string
	Platform string
	Err      error
	Running  bool
}

// Result summarizes a finished run.
type Result struct {
	Files    int
	Errors   []string
	Warnings []string
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
			report(Progress{Done: i, Total: total, Label: step.Label, Platform: step.Platform, Running: true})
		}
		var err error
		if err = os.MkdirAll(filepath.Dir(step.Out), 0o755); err == nil {
			switch step.Kind {
			case StepStatic:
				err = os.WriteFile(step.Out, []byte(step.Content), 0o644)
			case StepIcon, StepBundle:
				err = p.runMagick(ctx, step)
			default:
				err = errors.New(i18n.T("未知步骤"))
			}
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
			report(Progress{Done: i + 1, Total: total, Label: step.Label, Platform: step.Platform, Err: err})
		}
	}
	res.Duration = time.Since(started)
	return res
}

func (p *Plan) runMagick(ctx context.Context, step Step) error {
	out, err := exec.CommandContext(ctx, p.Tool.Path, step.Args...).CombinedOutput()
	if err == nil {
		if _, statErr := os.Stat(step.Out); statErr == nil {
			if step.Kind == StepBundle && strings.HasSuffix(strings.ToLower(step.Out), ".icns") {
				// Some ImageMagick builds write only the first input as
				// ICNS. Prefer the built-in writer unless the container
				// really holds several sizes.
				if icnsEntries(step.Out) < 4 {
					if fallbackErr := writeICNS(step.Out, step.Sources); fallbackErr != nil {
						return fmt.Errorf("%s：%v", i18n.T("ICNS 尺寸不完整，内置写入器也失败"), fallbackErr)
					}
				}
			}
			return nil
		}
		err = errors.New(i18n.T("ImageMagick 没有写出文件"))
	}
	// ImageMagick builds are not always compiled with the ICNS writer.
	// The iconset PNGs are already on disk, so build the container here.
	if step.Kind == StepBundle && strings.HasSuffix(strings.ToLower(step.Out), ".icns") {
		if fallbackErr := writeICNS(step.Out, step.Sources); fallbackErr == nil {
			return nil
		}
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

// icnsEntries counts the images in an ICNS file, or 0 when the file is not
// a valid container.
func icnsEntries(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 8 || string(data[:4]) != "icns" {
		return 0
	}
	total := int(binary.BigEndian.Uint32(data[4:8]))
	if total > len(data) || total < 8 {
		total = len(data)
	}
	n := 0
	for off := 8; off+8 <= total; {
		size := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if size < 8 || off+size > total {
			return n
		}
		n++
		off += size
	}
	return n
}

// writeICNS builds a modern ICNS container from PNG files (32 to 1024 px).
// It is used only when ImageMagick cannot write ICNS, which is common on
// minimal Linux installs.
func writeICNS(path string, sources []Source) error {
	types := map[int]string{
		32:   "ic11",
		64:   "ic12",
		128:  "ic07",
		256:  "ic08",
		512:  "ic09",
		1024: "ic10",
	}
	seen := map[string]bool{}
	var body bytes.Buffer
	added := 0
	for _, src := range sources {
		typ, ok := types[src.Size]
		if !ok || seen[typ] {
			continue
		}
		data, err := os.ReadFile(src.Path)
		if err != nil || len(data) == 0 {
			continue
		}
		seen[typ] = true
		body.WriteString(typ)
		_ = binary.Write(&body, binary.BigEndian, uint32(len(data)+8))
		body.Write(data)
		added++
	}
	if added == 0 {
		return errors.New(i18n.T("没有可用于 ICNS 的 PNG"))
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
