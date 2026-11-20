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
	"sort"
	"strings"
	"time"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/magick"
)

// Tool is a detected ImageMagick installation.
type Tool = magick.Tool

// Detect finds the installed ImageMagick.
func Detect() (Tool, error) {
	return magick.Detect()
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
		return nil, errors.New(i18n.T("app.common.imagemagick_is_not_installed"))
	}
	input := strings.TrimSpace(opt.Input)
	if input == "" {
		return nil, errors.New(i18n.T("app.icon.choose_source_image"))
	}
	info, err := os.Stat(input)
	if err != nil {
		return nil, fmt.Errorf("%s%v", i18n.T("app.icon.source_image_not_found"), err)
	}
	if info.IsDir() {
		return nil, errors.New(i18n.T("app.icon.source_image_is_folder"))
	}
	root := strings.TrimSpace(opt.OutputRoot)
	if root == "" {
		return nil, errors.New(i18n.T("app.common.choose_output_folder"))
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
		return nil, errors.New(i18n.T("app.icon.choose_at_least_one_platform"))
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
					return nil, fmt.Errorf(i18n.T("app.icon.cannot_find_icns_source_icon_for"), platform.Name)
				}
				for _, src := range step.Sources {
					step.Args = append(step.Args, src.Path)
				}
				step.Args = append(step.Args, "-strip", "ICNS:"+out)
			default:
				return nil, fmt.Errorf("%s%s", i18n.T("app.icon.unknown_icon_container_format"), b.Format)
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
		return nil, errors.New(i18n.T("app.icon.selected_platforms_have_no_files_to"))
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
		report(Progress{Done: 0, Total: total, Label: i18n.T("app.common.preparing_output_folders"), Running: true})
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
				err = errors.New(i18n.T("app.icon.unknown_step"))
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
						return fmt.Errorf("%s：%v", i18n.T("app.icon.icns_sizes_are_incomplete_and_built"), fallbackErr)
					}
				}
			}
			return nil
		}
		err = errors.New(i18n.T("app.common.imagemagick_did_not_write_output_file"))
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
		return errors.New(i18n.T("app.icon.no_pngs_available_for_icns"))
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
