// Package ffmpeg plans and runs the common FFmpeg tasks the app offers:
// compression, trimming, merging, audio extraction, GIF creation,
// watermarks, resizing, subtitles, speed changes, covers and batch
// processing. It only builds command lines and reports progress; the actual
// work is done by the installed FFmpeg.
package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"xin.dart.zchat/internal/i18n"
)

// Tool is a detected FFmpeg installation. FFprobe is used for durations
// and may be empty when only ffmpeg is installed.
type Tool struct {
	Path      string
	ProbePath string
	Version   string
}

// Detect finds ffmpeg and optionally ffprobe. The version line must mention
// FFmpeg so an unrelated program with the same name is rejected.
func Detect() (Tool, error) {
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		if runtime.GOOS == "windows" {
			path, err = exec.LookPath("ffmpeg")
		}
		if err != nil {
			return Tool{}, errors.New(i18n.T("app.ffmpeg.ffmpeg_not_found"))
		}
	}
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
	if !strings.Contains(first, "ffmpeg version") {
		return Tool{}, fmt.Errorf("%s%s", path, i18n.T("app.ffmpeg.is_not_ffmpeg"))
	}
	tool := Tool{Path: path, Version: strings.TrimSpace(first)}
	probe := "ffprobe"
	if runtime.GOOS == "windows" {
		probe = "ffprobe.exe"
	}
	if p, err := exec.LookPath(probe); err == nil {
		tool.ProbePath = p
	} else if p, err := exec.LookPath("ffprobe"); err == nil {
		tool.ProbePath = p
	}
	return tool, nil
}

// Kind names one toolkit capability.
type Kind string

const (
	KindCompress  Kind = "compress"
	KindTrim      Kind = "trim"
	KindMerge     Kind = "merge"
	KindAudio     Kind = "audio"
	KindGIF       Kind = "gif"
	KindWatermark Kind = "watermark"
	KindTransform Kind = "transform"
	KindSubtitle  Kind = "subtitle"
	KindSpeed     Kind = "speed"
	KindCover     Kind = "cover"
	KindBatch     Kind = "batch"
)

// Watermark modes.
const (
	WatermarkText  = "text"
	WatermarkImage = "image"
)

// Transform modes.
const (
	TransformResize = "resize"
	TransformCrop   = "crop"
	TransformPad    = "pad"
	TransformRotate = "rotate"
)

// Subtitle modes.
const (
	SubtitleBurn = "burn"
	SubtitleBGM  = "bgm"
)

// Batch modes.
const (
	BatchCompress = "compress"
	BatchAudio    = "audio"
	BatchCover    = "cover"
)

// Options are the choices for one FFmpeg run.
type Options struct {
	Kind      Kind
	Inputs    []string
	OutputDir string

	// Video compression.
	Format       string // mp4, mkv, mov; or keep
	CRF          int
	Preset       string
	Resolution   string // source, 1080p, 720p, 480p, vertical, horizontal, square
	AudioBitrate string // 96k, 128k, 192k

	// Trim.
	Start    string
	Duration string
	End      string
	Copy     bool

	// Audio extraction.
	AudioFormat string // mp3, m4a, aac, wav

	// GIF.
	FPS         int
	Width       int
	HighQuality bool

	// Watermark.
	WatermarkMode string
	Text          string
	Image         string
	Position      string
	Margin        int
	FontSize      int
	FontColor     string
	Opacity       float64
	ImageWidth    int

	// Transform.
	TransformMode string
	Height        int
	CropX         int
	CropY         int
	Rotation      string

	// Subtitles and background music.
	SubtitleMode string
	SubtitleFile string
	BGMFile      string

	// Speed.
	Speed float64

	// Cover.
	At       string
	Interval int

	// Batch.
	BatchMode string
}

// Validate checks the options that do not need FFmpeg. The UI runs it
// before starting so mistakes are reported immediately.
func Validate(opt Options) error {
	inputs := cleanInputs(opt.Inputs)
	if len(inputs) == 0 {
		return errors.New(i18n.T("app.ffmpeg.choose_video_or_audio_file_first"))
	}
	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return fmt.Errorf("%s%v", i18n.T("app.ffmpeg.file_not_found"), err)
		}
		if info.IsDir() {
			return errors.New(i18n.T("app.common.selection_contains_folder"))
		}
	}
	switch opt.Kind {
	case KindCompress, KindTrim, KindAudio, KindGIF, KindWatermark, KindTransform, KindSubtitle, KindSpeed, KindCover, KindBatch:
	case KindMerge:
		if len(inputs) < 2 {
			return errors.New(i18n.T("app.ffmpeg.merging_needs_at_least_two_files"))
		}
	default:
		return errors.New(i18n.T("app.common.unknown_processing_type"))
	}
	if strings.TrimSpace(opt.OutputDir) == "" {
		return errors.New(i18n.T("app.common.choose_output_folder"))
	}
	switch opt.Kind {
	case KindTrim:
		start, okStart := parseTime(opt.Start)
		if !okStart {
			return errors.New(i18n.T("app.ffmpeg.start_time_format_is_invalid"))
		}
		if strings.TrimSpace(opt.Duration) != "" {
			d, ok := parseTime(opt.Duration)
			if !ok || d <= 0 {
				return errors.New(i18n.T("app.ffmpeg.duration_format_is_invalid"))
			}
		} else if strings.TrimSpace(opt.End) != "" {
			end, ok := parseTime(opt.End)
			if !ok || end <= start {
				return errors.New(i18n.T("app.ffmpeg.end_time_must_be_later_than"))
			}
		} else {
			return errors.New(i18n.T("app.ffmpeg.enter_duration_or_end_time"))
		}
	case KindGIF:
		if opt.FPS <= 0 || opt.FPS > 60 {
			return errors.New(i18n.T("app.ffmpeg.frame_rate_must_be_between_1"))
		}
		if opt.Width <= 0 {
			return errors.New(i18n.T("app.ffmpeg.enter_gif_width"))
		}
		if _, ok := parseTime(opt.Start); !ok {
			return errors.New(i18n.T("app.ffmpeg.start_time_format_is_invalid"))
		}
		if strings.TrimSpace(opt.Duration) != "" {
			if d, ok := parseTime(opt.Duration); !ok || d <= 0 {
				return errors.New(i18n.T("app.ffmpeg.duration_format_is_invalid"))
			}
		}
	case KindWatermark:
		if opt.WatermarkMode == WatermarkImage {
			if strings.TrimSpace(opt.Image) == "" {
				return errors.New(i18n.T("app.common.choose_watermark_image"))
			}
			if info, err := os.Stat(opt.Image); err != nil || info.IsDir() {
				return errors.New(i18n.T("app.common.watermark_image_not_found"))
			}
		} else if strings.TrimSpace(opt.Text) == "" {
			return errors.New(i18n.T("app.common.enter_watermark_text"))
		}
	case KindTransform:
		switch opt.TransformMode {
		case TransformCrop:
			if opt.Width <= 0 || opt.Height <= 0 {
				return errors.New(i18n.T("app.common.enter_width_and_height_to_crop"))
			}
		case TransformPad:
			if opt.Width <= 0 || opt.Height <= 0 {
				return errors.New(i18n.T("app.ffmpeg.aspect_fit_needs_both_width_and"))
			}
		case TransformResize:
			if opt.Width <= 0 && opt.Height <= 0 {
				return errors.New(i18n.T("app.ffmpeg.resize_needs_width_or_height"))
			}
		case TransformRotate:
			if !validRotation(opt.Rotation) {
				return errors.New(i18n.T("app.ffmpeg.choose_rotation_or_flip_mode"))
			}
		default:
			return errors.New(i18n.T("app.ffmpeg.unknown_picture_adjustment_mode"))
		}
	case KindSubtitle:
		if opt.SubtitleMode == SubtitleBurn {
			if strings.TrimSpace(opt.SubtitleFile) == "" {
				return errors.New(i18n.T("app.ffmpeg.choose_subtitle_file"))
			}
			if info, err := os.Stat(opt.SubtitleFile); err != nil || info.IsDir() {
				return errors.New(i18n.T("app.ffmpeg.subtitle_file_does_not_exist"))
			}
		} else {
			if strings.TrimSpace(opt.BGMFile) == "" {
				return errors.New(i18n.T("app.ffmpeg.choose_background_music_or_audio_file"))
			}
			if info, err := os.Stat(opt.BGMFile); err != nil || info.IsDir() {
				return errors.New(i18n.T("app.ffmpeg.background_music_does_not_exist"))
			}
		}
	case KindSpeed:
		if opt.Speed < 0.25 || opt.Speed > 4 {
			return errors.New(i18n.T("app.ffmpeg.speed_must_be_between_0_25"))
		}
	case KindCover:
		if _, ok := parseTime(opt.At); !ok {
			return errors.New(i18n.T("app.ffmpeg.capture_time_format_is_invalid"))
		}
		if opt.Interval < 0 {
			return errors.New(i18n.T("app.ffmpeg.interval_cannot_be_negative"))
		}
	case KindBatch:
		switch opt.BatchMode {
		case BatchCompress, BatchAudio, BatchCover:
		default:
			return errors.New(i18n.T("app.ffmpeg.choose_batch_mode"))
		}
	}
	return nil
}

// Step is one FFmpeg command and the file it writes.
type Step struct {
	Label string
	Out   string
	Args  []string
	// Concat lists inputs for the concat demuxer. Run writes a temporary
	// list file and replaces {list} in Args with its path.
	Concat []string
	// ProbeInput is the file whose duration drives the progress bar.
	ProbeInput string
	// Limit is the expected output duration in seconds, when known.
	Limit float64
	// Speed divides the probed duration for the expected output length.
	Speed float64
	// Pattern marks outputs such as thumb-%03d.jpg that do not exist as a
	// single file.
	Pattern bool
}

// Plan is the ordered list of files to produce.
type Plan struct {
	Tool      Tool
	Inputs    []string
	OutputDir string
	Steps     []Step
}

// Build validates the options and turns them into FFmpeg commands. It does
// not run anything, so it is cheap and safe to test.
func Build(tool Tool, opt Options) (*Plan, error) {
	if strings.TrimSpace(tool.Path) == "" {
		return nil, errors.New(i18n.T("app.ffmpeg.ffmpeg_is_not_installed"))
	}
	if err := Validate(opt); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(strings.TrimSpace(opt.OutputDir))
	if err != nil {
		return nil, err
	}
	plan := &Plan{Tool: tool, Inputs: cleanInputs(opt.Inputs), OutputDir: root}
	b := &builder{plan: plan, opt: opt, root: root, used: map[string]bool{}}
	switch opt.Kind {
	case KindCompress:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.compressStep(in))
		}
	case KindTrim:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.trimStep(in))
		}
	case KindMerge:
		plan.Steps = append(plan.Steps, b.mergeStep())
	case KindAudio:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.audioStep(in))
		}
	case KindGIF:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.gifStep(in))
		}
	case KindWatermark:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.watermarkStep(in))
		}
	case KindTransform:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.transformStep(in))
		}
	case KindSubtitle:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.subtitleStep(in))
		}
	case KindSpeed:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.speedStep(in))
		}
	case KindCover:
		for _, in := range plan.Inputs {
			plan.Steps = append(plan.Steps, b.coverStep(in))
		}
	case KindBatch:
		for _, in := range plan.Inputs {
			switch opt.BatchMode {
			case BatchAudio:
				plan.Steps = append(plan.Steps, b.audioStep(in))
			case BatchCover:
				plan.Steps = append(plan.Steps, b.coverStep(in))
			default:
				plan.Steps = append(plan.Steps, b.compressStep(in))
			}
		}
	}
	if len(plan.Steps) == 0 {
		return nil, errors.New(i18n.T("app.common.nothing_to_process"))
	}
	return plan, nil
}

type builder struct {
	plan *Plan
	opt  Options
	root string
	used map[string]bool
}

func (b *builder) compressStep(in string) Step {
	ext := b.videoExt("mp4")
	out := b.target(in, "-compressed", ext)
	args := baseArgs()
	args = append(args, "-i", in)
	if vf := resolutionFilter(b.opt.Resolution); vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args,
		"-c:v", "libx264",
		"-crf", itoa(clampInt(b.opt.CRF, 0, 51)),
		"-preset", presetName(b.opt.Preset),
		"-c:a", "aac",
		"-b:a", bitrate(b.opt.AudioBitrate),
	)
	if ext == "mp4" || ext == "mov" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) trimStep(in string) Step {
	start, _ := parseTime(b.opt.Start)
	duration := b.trimDuration()
	ext := b.videoExt("mp4")
	if b.opt.Copy {
		ext = extOrInput(b.opt.Format, in)
	}
	out := b.target(in, "-clip", ext)
	args := baseArgs()
	if b.opt.Copy {
		// Input seeking is fast and aligns to a keyframe, which is what
		// stream copy can use anyway.
		args = append(args, "-ss", secondsString(start), "-i", in, "-t", secondsString(duration),
			"-c", "copy", "-avoid_negative_ts", "make_zero")
	} else {
		// Output seeking after the input gives a precise cut when the
		// video is re-encoded.
		args = append(args, "-i", in, "-ss", secondsString(start), "-t", secondsString(duration))
		args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
		args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate))
	}
	args = append(args, out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
		Limit:      duration,
	}
}

func (b *builder) mergeStep() Step {
	first := b.plan.Inputs[0]
	ext := extOrInput(b.opt.Format, first)
	out := b.target(first, "-merged", ext)
	args := baseArgs()
	args = append(args, "-f", "concat", "-safe", "0", "-i", "{list}")
	if b.opt.Copy {
		args = append(args, "-c", "copy", "-avoid_negative_ts", "make_zero")
	} else {
		args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
		args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate))
	}
	args = append(args, out)
	return Step{
		Label:  fmt.Sprintf("%s → %s", i18n.T("app.ffmpeg.merge_videos"), filepath.Base(out)),
		Out:    out,
		Args:   args,
		Concat: append([]string(nil), b.plan.Inputs...),
	}
}

func (b *builder) audioStep(in string) Step {
	ext := audioExt(b.opt.AudioFormat)
	out := b.target(in, "", ext)
	args := baseArgs()
	args = append(args, "-i", in, "-vn")
	switch ext {
	case "mp3":
		args = append(args, "-c:a", "libmp3lame")
	case "wav":
		args = append(args, "-c:a", "pcm_s16le")
	case "m4a":
		args = append(args, "-c:a", "aac", "-movflags", "+faststart")
	default:
		args = append(args, "-c:a", "aac")
	}
	if ext != "wav" {
		args = append(args, "-b:a", bitrate(b.opt.AudioBitrate))
	}
	args = append(args, out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) gifStep(in string) Step {
	start, _ := parseTime(b.opt.Start)
	duration := 3.0
	if d, ok := parseTime(b.opt.Duration); ok && d > 0 {
		duration = d
	}
	out := b.target(in, "", "gif")
	args := baseArgs()
	// Seeking after the input picks the exact requested start frame.
	args = append(args, "-i", in, "-ss", secondsString(start), "-t", secondsString(duration))
	vf := fmt.Sprintf("fps=%d,scale=%d:-1:flags=lanczos", clampInt(b.opt.FPS, 1, 60), clampInt(b.opt.Width, 16, 4096))
	if b.opt.HighQuality {
		vf += ",split[s0][s1];[s0]palettegen=stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
	}
	args = append(args, "-vf", vf, "-loop", "0", out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
		Limit:      duration,
	}
}

func (b *builder) watermarkStep(in string) Step {
	out := b.target(in, "-watermark", b.videoExt("mp4"))
	args := baseArgs()
	if b.opt.WatermarkMode == WatermarkImage {
		args = append(args, "-i", in, "-i", b.opt.Image)
		filter := "[1:v]"
		if b.opt.ImageWidth > 0 {
			filter += fmt.Sprintf("scale=%d:-1,", b.opt.ImageWidth)
		}
		filter += fmt.Sprintf("format=rgba,colorchannelmixer=aa=%.2f[wm];[0:v][wm]overlay=%s[v]",
			clampFloat(b.opt.Opacity, 0.05, 1), overlayPosition(b.opt.Position, b.opt.Margin))
		args = append(args, "-filter_complex", filter, "-map", "[v]", "-map", "0:a?")
	} else {
		args = append(args, "-i", in)
		filter := fmt.Sprintf("drawtext=text='%s':x=%s:y=%s:fontsize=%d:fontcolor=%s",
			escapeDrawText(b.opt.Text),
			positionX(b.opt.Position, b.opt.Margin), positionY(b.opt.Position, b.opt.Margin),
			clampInt(b.opt.FontSize, 8, 400),
			fontColor(b.opt.FontColor, b.opt.Opacity),
		)
		args = append(args, "-vf", filter)
	}
	args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
	args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate), out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) transformStep(in string) Step {
	suffix := "-resized"
	var vf string
	switch b.opt.TransformMode {
	case TransformCrop:
		suffix = "-cropped"
		vf = fmt.Sprintf("crop=%d:%d:%d:%d", b.opt.Width, b.opt.Height, b.opt.CropX, b.opt.CropY)
	case TransformPad:
		suffix = "-padded"
		vf = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
			b.opt.Width, b.opt.Height, b.opt.Width, b.opt.Height)
	case TransformRotate:
		suffix = "-rotated"
		vf = rotationFilter(b.opt.Rotation)
	default:
		if b.opt.Width > 0 && b.opt.Height > 0 {
			vf = fmt.Sprintf("scale=%d:%d", b.opt.Width, b.opt.Height)
		} else if b.opt.Width > 0 {
			vf = fmt.Sprintf("scale=%d:-2", b.opt.Width)
		} else {
			vf = fmt.Sprintf("scale=-2:%d", b.opt.Height)
		}
	}
	out := b.target(in, suffix, b.videoExt("mp4"))
	args := baseArgs()
	args = append(args, "-i", in, "-vf", vf)
	args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
	args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate), out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) subtitleStep(in string) Step {
	suffix := "-subtitled"
	args := baseArgs()
	if b.opt.SubtitleMode == SubtitleBGM {
		suffix = "-bgm"
		args = append(args, "-i", in, "-i", b.opt.BGMFile,
			"-map", "0:v:0", "-map", "1:a:0",
			"-c:v", "copy", "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate), "-shortest")
	} else {
		args = append(args, "-i", in,
			"-vf", "subtitles="+escapeFilterPath(b.opt.SubtitleFile))
		args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
		args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate))
	}
	out := b.target(in, suffix, b.videoExt("mp4"))
	args = append(args, out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) speedStep(in string) Step {
	speed := b.opt.Speed
	if speed <= 0 {
		speed = 2
	}
	out := b.target(in, fmt.Sprintf("-%gx", speed), b.videoExt("mp4"))
	args := baseArgs()
	args = append(args, "-i", in,
		"-vf", fmt.Sprintf("setpts=PTS/%.4f", speed),
		"-af", atempoChain(speed))
	args = append(args, videoEncodeArgs(b.opt.CRF, b.opt.Preset)...)
	args = append(args, "-c:a", "aac", "-b:a", bitrate(b.opt.AudioBitrate), out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
		Speed:      speed,
	}
}

func (b *builder) coverStep(in string) Step {
	ext := b.coverExt()
	at, _ := parseTime(b.opt.At)
	if at < 0 {
		at = 0
	}
	if b.opt.Interval > 0 {
		pattern := b.targetPattern(in, "-thumb", ext)
		args := baseArgs()
		args = append(args, "-i", in, "-vf", fmt.Sprintf("fps=1/%d", b.opt.Interval),
			"-q:v", "2", pattern)
		return Step{
			Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(pattern)),
			Out:        pattern,
			Args:       args,
			ProbeInput: in,
			Pattern:    true,
		}
	}
	out := b.target(in, "-cover", ext)
	args := baseArgs()
	// Seeking after the input is slower but picks the exact requested frame,
	// which matters for a cover image.
	args = append(args, "-i", in, "-ss", secondsString(at), "-frames:v", "1", "-q:v", "2", out)
	return Step{
		Label:      fmt.Sprintf("%s → %s", filepath.Base(in), filepath.Base(out)),
		Out:        out,
		Args:       args,
		ProbeInput: in,
	}
}

func (b *builder) trimDuration() float64 {
	if d, ok := parseTime(b.opt.Duration); ok && d > 0 {
		return d
	}
	start, _ := parseTime(b.opt.Start)
	if end, ok := parseTime(b.opt.End); ok && end > start {
		return end - start
	}
	return 0
}

func (b *builder) videoExt(fallback string) string {
	return normalizeExt(b.opt.Format, fallback)
}

func (b *builder) coverExt() string {
	ext := normalizeExt(b.opt.Format, "jpg")
	if ext != "jpg" && ext != "png" {
		return "jpg"
	}
	return ext
}

func baseArgs() []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostats", "-progress", "pipe:2", "-y"}
}

func videoEncodeArgs(crf int, preset string) []string {
	return []string{
		"-c:v", "libx264",
		"-crf", itoa(clampInt(crf, 0, 51)),
		"-preset", presetName(preset),
	}
}

func resolutionFilter(res string) string {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "2160p":
		return "scale=-2:2160:flags=lanczos"
	case "1080p":
		return "scale=-2:1080:flags=lanczos"
	case "720p":
		return "scale=-2:720:flags=lanczos"
	case "480p":
		return "scale=-2:480:flags=lanczos"
	case "vertical":
		return "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black"
	case "horizontal":
		return "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black"
	case "square":
		return "scale=1080:1080:force_original_aspect_ratio=decrease,pad=1080:1080:(ow-iw)/2:(oh-ih)/2:black"
	}
	return ""
}

func presetName(p string) string {
	switch p {
	case "ultrafast", "veryfast", "fast", "medium", "slow", "slower":
		return p
	}
	return "medium"
}

func bitrate(b string) string {
	b = strings.TrimSpace(b)
	if b == "" {
		return "128k"
	}
	return b
}

func audioExt(f string) string {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "m4a", "aac", "wav", "mp3":
		return strings.ToLower(strings.TrimSpace(f))
	}
	return "mp3"
}

func normalizeExt(format, fallback string) string {
	format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
	if format == "" || format == "keep" {
		return fallback
	}
	switch format {
	case "mp4", "mkv", "mov", "jpg", "jpeg", "png", "gif":
		if format == "jpeg" {
			return "jpg"
		}
		return format
	}
	return fallback
}

func extOrInput(format, input string) string {
	format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
	if format == "" || format == "keep" {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input)), ".")
		if ext == "" {
			return "mp4"
		}
		return ext
	}
	return normalizeExt(format, "mp4")
}

func validRotation(r string) bool {
	switch r {
	case "90", "180", "270", "flip-h", "flip-v":
		return true
	}
	return false
}

func rotationFilter(r string) string {
	switch r {
	case "180":
		return "hflip,vflip"
	case "270":
		return "transpose=2"
	case "flip-h":
		return "hflip"
	case "flip-v":
		return "vflip"
	default:
		return "transpose=1"
	}
}

func positionX(pos string, margin int) string {
	if margin < 0 {
		margin = 0
	}
	switch pos {
	case "topright", "bottomright":
		return fmt.Sprintf("w-text_w-%d", margin)
	case "center":
		return "(w-text_w)/2"
	default:
		return fmt.Sprintf("%d", margin)
	}
}

func positionY(pos string, margin int) string {
	if margin < 0 {
		margin = 0
	}
	switch pos {
	case "bottomleft", "bottomright":
		return fmt.Sprintf("h-text_h-%d", margin)
	case "center":
		return "(h-text_h)/2"
	default:
		return fmt.Sprintf("%d", margin)
	}
}

func overlayPosition(pos string, margin int) string {
	if margin < 0 {
		margin = 0
	}
	switch pos {
	case "topleft":
		return fmt.Sprintf("%d:%d", margin, margin)
	case "topright":
		return fmt.Sprintf("W-w-%d:%d", margin, margin)
	case "bottomleft":
		return fmt.Sprintf("%d:H-h-%d", margin, margin)
	case "bottomright":
		return fmt.Sprintf("W-w-%d:H-h-%d", margin, margin)
	default:
		return "(W-w)/2:(H-h)/2"
	}
}

func fontColor(color string, opacity float64) string {
	color = strings.TrimSpace(color)
	if color == "" {
		color = "#ffffff"
	}
	alpha := clampFloat(opacity, 0.05, 1)
	if strings.HasPrefix(color, "#") {
		hex := strings.TrimPrefix(color, "#")
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		return fmt.Sprintf("0x%s@%.2f", hex, alpha)
	}
	return fmt.Sprintf("%s@%.2f", color, alpha)
}

// escapeDrawText escapes a user string for drawtext's text='...' option.
func escapeDrawText(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`:`, `\:`,
		`%`, `\%`,
		`,`, `\,`,
		`[`, `\[`,
		`]`, `\]`,
	)
	return r.Replace(s)
}

// escapeFilterPath escapes a file path used as a filter option value.
func escapeFilterPath(path string) string {
	path = filepath.ToSlash(path)
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`:`, `\:`,
		`,`, `\,`,
		`;`, `\;`,
		`[`, `\[`,
		`]`, `\]`,
	)
	return "'" + r.Replace(path) + "'"
}

// atempoChain builds one or more atempo filters, since a single atempo only
// accepts 0.5 to 2.0.
func atempoChain(speed float64) string {
	if speed <= 0 {
		speed = 1
	}
	var parts []string
	for speed > 2 {
		parts = append(parts, "atempo=2.0")
		speed /= 2
	}
	for speed < 0.5 {
		parts = append(parts, "atempo=0.5")
		speed /= 0.5
	}
	parts = append(parts, fmt.Sprintf("atempo=%g", speed))
	return strings.Join(parts, ",")
}

func (b *builder) target(input, suffix, ext string) string {
	out := targetPath(b.root, input, suffix, ext)
	base := strings.TrimSuffix(out, "."+ext)
	candidate := out
	for n := 2; b.used[strings.ToLower(candidate)]; n++ {
		candidate = fmt.Sprintf("%s-%d.%s", base, n, ext)
	}
	b.used[strings.ToLower(candidate)] = true
	return candidate
}

func (b *builder) targetPattern(input, suffix, ext string) string {
	pattern := targetPattern(b.root, input, suffix, ext)
	base := strings.TrimSuffix(pattern, "-%03d."+ext)
	candidate := pattern
	for n := 2; b.used[strings.ToLower(candidate)]; n++ {
		candidate = fmt.Sprintf("%s-%d-%%03d.%s", base, n, ext)
	}
	b.used[strings.ToLower(candidate)] = true
	return candidate
}

func targetPath(root, input, suffix, ext string) string {
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	out := filepath.Join(root, base+suffix+"."+ext)
	if samePath(out, input) {
		out = filepath.Join(root, base+suffix+"-out."+ext)
	}
	return out
}

func targetPattern(root, input, suffix, ext string) string {
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return filepath.Join(root, base+suffix+"-%03d."+ext)
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}

// parseTime accepts seconds, MM:SS, and HH:MM:SS with an optional fraction.
func parseTime(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	total := 0.0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, false
		}
		v, err := strconv.ParseFloat(part, 64)
		if err != nil || v < 0 {
			return 0, false
		}
		total = total*60 + v
	}
	return total, true
}

func secondsString(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%.3f", seconds)
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

func itoa(v int) string { return strconv.Itoa(v) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Progress is reported before, during and after each step.
type Progress struct {
	Done    int
	Total   int
	Percent float64
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
	durations := map[string]float64{}
	if report != nil {
		report(Progress{Done: 0, Total: total, Label: i18n.T("app.common.preparing_output_folders"), Running: true})
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
		listPath := ""
		if err = os.MkdirAll(filepath.Dir(step.Out), 0o755); err == nil {
			if len(step.Concat) > 0 {
				listPath, err = writeConcatList(step.Concat)
			}
			if err == nil {
				limit := step.Limit
				if limit <= 0 && step.ProbeInput != "" && !step.Pattern {
					d := durations[step.ProbeInput]
					if d == 0 {
						d = p.probeDuration(ctx, step.ProbeInput)
						durations[step.ProbeInput] = d
					}
					if step.Speed > 0 && d > 0 {
						limit = d / step.Speed
					} else {
						limit = d
					}
				}
				err = p.runFFmpeg(ctx, step, listPath, limit, func(frac float64) {
					if report != nil {
						report(Progress{Done: i, Total: total, Percent: frac, Label: step.Label, Running: true})
					}
				})
			}
			if listPath != "" {
				_ = os.Remove(listPath)
			}
		}
		written := 0
		if err == nil {
			if step.Pattern {
				base := filepath.Base(step.Out)
				parts := strings.SplitN(base, "%03d", 2)
				prefix, suffix := parts[0], ""
				if len(parts) == 2 {
					suffix = parts[1]
				}
				entries, _ := os.ReadDir(filepath.Dir(step.Out))
				for _, entry := range entries {
					name := entry.Name()
					if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
						written++
					}
				}
				if written == 0 {
					err = errors.New(i18n.T("app.ffmpeg.ffmpeg_did_not_write_file"))
				}
			} else {
				if _, statErr := os.Stat(step.Out); statErr != nil {
					err = errors.New(i18n.T("app.ffmpeg.ffmpeg_did_not_write_file"))
				} else {
					written = 1
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				res.Canceled = true
			} else {
				res.Errors = append(res.Errors, step.Label+"："+err.Error())
			}
		} else {
			res.Files += written
		}
		if report != nil {
			report(Progress{Done: i + 1, Total: total, Percent: 1, Label: step.Label, Err: err})
		}
	}
	res.Duration = time.Since(started)
	return res
}

func (p *Plan) probeDuration(ctx context.Context, input string) float64 {
	if p.Tool.ProbePath == "" || input == "" {
		return 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, p.Tool.ProbePath,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", input).Output()
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func (p *Plan) runFFmpeg(ctx context.Context, step Step, listPath string, limit float64, progress func(float64)) error {
	args := make([]string, len(step.Args))
	copy(args, step.Args)
	for i, arg := range args {
		if arg == "{list}" {
			args[i] = listPath
		}
	}
	cmd := exec.CommandContext(ctx, p.Tool.Path, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var tail []string
	var outTime float64
	lastReport := time.Time{}
	for scanner.Scan() {
		line := scanner.Text()
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
			if us, err := strconv.ParseFloat(v, 64); err == nil && us >= 0 {
				outTime = us / 1_000_000
			}
		} else if v, ok := strings.CutPrefix(line, "out_time_ms="); ok {
			if us, err := strconv.ParseFloat(v, 64); err == nil && us >= 0 {
				outTime = us / 1_000_000
			}
		} else {
			tail = append(tail, line)
			if len(tail) > 20 {
				tail = tail[len(tail)-20:]
			}
		}
		if limit > 0 && progress != nil && time.Since(lastReport) > 150*time.Millisecond {
			frac := outTime / limit
			if frac > 1 {
				frac = 1
			}
			progress(frac)
			lastReport = time.Now()
		}
	}
	err = cmd.Wait()
	if err == nil {
		return nil
	}
	msg := strings.Join(tail, "\n")
	if len(msg) > 600 {
		msg = msg[len(msg)-600:]
	}
	if msg == "" {
		return err
	}
	return fmt.Errorf("%v: %s", err, msg)
}

func writeConcatList(files []string) (string, error) {
	f, err := os.CreateTemp("", "zchat-concat-*.txt")
	if err != nil {
		return "", err
	}
	name := f.Name()
	for _, path := range files {
		line := "file '" + escapeConcatPath(path) + "'\n"
		if _, err := f.WriteString(line); err != nil {
			f.Close()
			os.Remove(name)
			return "", err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func escapeConcatPath(path string) string {
	path = filepath.ToSlash(path)
	return strings.ReplaceAll(path, "'", `'\''`)
}
