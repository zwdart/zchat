package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/ffmpeg"
)

// ffmpegCapability describes one entry of the FFmpeg capability list.
type ffmpegCapability struct {
	Key     string
	Title   string
	Desc    string
	Command string
	Icon    *ui.SVG
}

func ffmpegCapabilities() []ffmpegCapability {
	return []ffmpegCapability{
		{
			Key:     string(ffmpeg.KindCompress),
			Title:   tr("app.ffmpeg.compress_video_2"),
			Desc:    tr("app.ffmpeg.reduce_size_and_standardize_resolution_with"),
			Command: "ffmpeg -i input.mp4 -c:v libx264 -crf 23 out.mp4",
			Icon:    iconFilm,
		},
		{
			Key:     string(ffmpeg.KindTrim),
			Title:   tr("app.ffmpeg.trim_video"),
			Desc:    tr("app.ffmpeg.cut_clips_and_remove_intros_or"),
			Command: "ffmpeg -ss 00:01:00 -t 30 -c copy out.mp4",
			Icon:    iconScissors,
		},
		{
			Key:     string(ffmpeg.KindMerge),
			Title:   tr("app.ffmpeg.merge_videos_2"),
			Desc:    tr("app.ffmpeg.merge_multiple_clips_in_order_with"),
			Command: "ffmpeg -f concat -i list.txt -c copy out.mp4",
			Icon:    iconLayers,
		},
		{
			Key:     string(ffmpeg.KindAudio),
			Title:   tr("app.ffmpeg.extract_audio"),
			Desc:    tr("app.ffmpeg.save_audio_from_video_as_mp3"),
			Command: "ffmpeg -i input.mp4 -vn -c:a libmp3lame out.mp3",
			Icon:    iconMusic,
		},
		{
			Key:     string(ffmpeg.KindGIF),
			Title:   tr("app.ffmpeg.video_to_gif"),
			Desc:    tr("app.ffmpeg.turn_clip_into_looping_gif_with"),
			Command: "ffmpeg -i input.mp4 -vf fps=10 out.gif",
			Icon:    iconImage,
		},
		{
			Key:     string(ffmpeg.KindWatermark),
			Title:   tr("app.ffmpeg.add_watermark"),
			Desc:    tr("app.ffmpeg.text_or_image_watermarks_with_position"),
			Command: "ffmpeg -i input.mp4 -vf drawtext=... out.mp4",
			Icon:    iconWatermark,
		},
		{
			Key:     string(ffmpeg.KindTransform),
			Title:   tr("app.ffmpeg.picture_adjustment"),
			Desc:    tr("app.ffmpeg.scale_crop_fit_vertical_or_horizontal"),
			Command: "ffmpeg -i input.mp4 -vf scale=1920:1080 out.mp4",
			Icon:    iconResize,
		},
		{
			Key:     string(ffmpeg.KindSubtitle),
			Title:   tr("app.ffmpeg.subtitles_and_music"),
			Desc:    tr("app.ffmpeg.burn_srt_subtitles_or_replace_audio"),
			Command: "ffmpeg -i input.mp4 -vf subtitles=sub.srt out.mp4",
			Icon:    iconSubtitles,
		},
		{
			Key:     string(ffmpeg.KindSpeed),
			Title:   tr("app.ffmpeg.speed_change"),
			Desc:    tr("app.ffmpeg.n_0_25_to_4_speed_changes"),
			Command: "ffmpeg -vf setpts=PTS/2 -af atempo=2 out.mp4",
			Icon:    iconGauge,
		},
		{
			Key:     string(ffmpeg.KindCover),
			Title:   tr("app.ffmpeg.video_snapshot"),
			Desc:    tr("app.ffmpeg.capture_single_cover_frame_or_export"),
			Command: "ffmpeg -ss 00:00:05 -frames:v 1 out.jpg",
			Icon:    iconScreenshot,
		},
		{
			Key:     string(ffmpeg.KindBatch),
			Title:   tr("app.common.batch_processing"),
			Desc:    tr("app.ffmpeg.compress_extract_audio_or_capture_covers"),
			Command: "for f in *.mp4; do ffmpeg -i \"$f\" ...; done",
			Icon:    iconLayers,
		},
	}
}

func findFFmpegCapability(key string) (ffmpegCapability, bool) {
	for _, cap := range ffmpegCapabilities() {
		if cap.Key == key {
			return cap, true
		}
	}
	return ffmpegCapability{}, false
}

// ffmpegState is the FFmpeg page's view state. Output folder, CRF, preset
// and audio bitrate are kept in settings so they survive a restart.
type ffmpegState struct {
	initialized bool
	cap         string
	inputs      []string

	format       string
	crf          float64
	preset       string
	resolution   string
	audioBitrate string

	start    string
	duration string
	end      string
	copy     bool

	audioFormat string

	fps      float64
	gifWidth float64
	hq       bool

	wmTab      int
	wmText     string
	wmImage    string
	position   string
	margin     float64
	fontSize   float64
	fontColor  string
	opacity    float64
	imageWidth float64

	transformMode string
	width         float64
	height        float64
	cropX         float64
	cropY         float64
	rotation      string

	subtitleMode int
	subtitleFile string
	bgmFile      string

	speed float64

	at       string
	interval float64

	batchMode int

	run    *ffmpegRun
	cancel context.CancelFunc
}

// ffmpegRun is one FFmpeg run's progress and result.
type ffmpegRun struct {
	running  bool
	finished bool
	total    int
	done     int
	percent  float64
	files    int
	current  string
	output   string
	started  time.Time
	elapsed  time.Duration
	log      []string
	errors   []string
	canceled bool
}

func (a *app) ffmpegDefaults() {
	f := &a.ffmpeg
	if f.initialized {
		return
	}
	f.initialized = true
	f.format = "mp4"
	f.crf = float64(a.settings.FFmpegCRF)
	if f.crf < 0 || f.crf > 51 {
		f.crf = 23
	}
	f.preset = a.settings.FFmpegPreset
	if f.preset == "" {
		f.preset = "medium"
	}
	f.resolution = "source"
	f.audioBitrate = a.settings.FFmpegAudioRate
	if f.audioBitrate == "" {
		f.audioBitrate = "128k"
	}
	f.start = "00:00:00"
	f.duration = ""
	f.end = ""
	f.copy = true
	f.audioFormat = "mp3"
	f.fps = 10
	f.gifWidth = 480
	f.hq = true
	f.wmTab = 0
	f.wmText = "© zchat"
	f.position = "topright"
	f.margin = 20
	f.fontSize = 32
	f.fontColor = "#ffffff"
	f.opacity = 0.7
	f.imageWidth = 180
	f.transformMode = ffmpeg.TransformResize
	f.width = 1280
	f.height = 720
	f.rotation = "90"
	f.subtitleMode = 0
	f.speed = 2
	f.at = "00:00:05"
	f.interval = 0
	f.batchMode = 0
}

// ffmpegEnterCap applies the defaults that differ between capabilities
// without discarding the input list or the user's saved settings.
func (a *app) ffmpegEnterCap(key string) {
	f := &a.ffmpeg
	switch key {
	case string(ffmpeg.KindTrim), string(ffmpeg.KindMerge):
		f.format = "keep"
		f.copy = true
	case string(ffmpeg.KindGIF):
		f.start = "00:00:00"
		f.duration = "3"
		f.fps = 10
		f.gifWidth = 480
		f.hq = true
	case string(ffmpeg.KindCover):
		f.format = "jpg"
		f.at = "00:00:05"
		f.interval = 0
	case string(ffmpeg.KindAudio):
		f.audioFormat = "mp3"
	case string(ffmpeg.KindBatch):
		f.batchMode = 0
		if f.format == "keep" || !videoFormat(f.format) {
			f.format = "mp4"
		}
	default:
		if f.format == "keep" || !videoFormat(f.format) {
			f.format = "mp4"
		}
	}
}

func videoFormat(format string) bool {
	switch format {
	case "mp4", "mkv", "mov", "keep":
		return true
	}
	return false
}

func (a *app) ffmpegPage(c *ui.Context) {
	a.ffmpegDefaults()
	p := a.pal
	if !a.ffToolChecked && !a.ffToolChecking {
		a.detectFFmpegTool()
	}
	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(16).Children(func() {
			a.ffmpegHeader(c, p)
			if a.ffmpeg.cap == "" {
				a.ffmpegCapabilitiesView(c, p)
				return
			}
			a.ffmpegForm(c, p)
		})
	})
}

func (a *app) ffmpegHeader(c *ui.Context, p palette) {
	cap, selected := findFFmpegCapability(a.ffmpeg.cap)
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).DragWindow().Children(func() {
		if selected {
			if pillButtonKey(c, "ffmpeg-back", iconBack, tr("app.common.capabilities"), p.hover.Over(p.bg), p.text).Clicked() {
				a.ffmpegBack()
			}
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			if selected {
				sectionTitle(c, p, cap.Title, cap.Desc)
			} else {
				sectionTitle(c, p, "FFmpeg", tr("app.ffmpeg.compress_trim_merge_extract_audio_make"))
			}
		})
		if run := a.ffmpeg.run; run != nil && run.running {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(10, 12).Radius(999).Background(p.accent.Alpha(0.13)).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, tr("app.common.processing_2")).FontSize(11.5).FontWeight(600).TextColor(p.accent)
			})
		}
	})
}

func (a *app) ffmpegCapabilitiesView(c *ui.Context, p palette) {
	ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
		a.ffmpegToolCard(c, p)
		ui.Text(c, tr("app.common.choose_capability")).FontSize(13).FontWeight(650).Margin(2, 0, 0, 0)
		ui.Grid(c).FillWidth().Columns(2).Gap(12).Children(func() {
			for _, cap := range ffmpegCapabilities() {
				a.ffmpegCapabilityCard(c, p, cap)
			}
		})
		ui.Row(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, iconInfo).Size(16, 16).TextColor(p.faint)
			ui.Text(c, tr("app.ffmpeg.all_processing_runs_locally_with_ffmpeg")).FontSize(11).TextColor(p.muted).LineHeight(1.5)
		})
	})
}

func (a *app) ffmpegCapabilityCard(c *ui.Context, p palette, cap ffmpegCapability) {
	b := ui.ButtonBase(c).Key("ffmpeg-cap-"+cap.Key).FillWidth().Padding(15, 14).Radius(14).Gap(11).
		AlignItems(ui.Start).FocusRing(false).Label(cap.Title).Border(1, p.hair).Background(p.panel)
	if b.Hovered() {
		b.Background(p.hover.Over(p.panel))
	}
	b.Children(func() {
		ui.Box(c).Size(36, 36).Radius(10).Shrink(0).Center().Background(p.accent.Alpha(0.12)).Children(func() {
			ui.Icon(c, cap.Icon).Size(19, 19).TextColor(p.accent)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, cap.Title).FontSize(13.5).FontWeight(650)
			ui.Text(c, cap.Desc).FontSize(11).TextColor(p.muted).LineHeight(1.45)
			if cap.Command != "" {
				ui.Text(c, cap.Command).FontSize(10).TextColor(p.faint).SingleLine().Selectable()
			}
		})
	})
	if b.Clicked() {
		a.ffmpeg.cap = cap.Key
		a.ffmpeg.run = nil
		a.ffmpegEnterCap(cap.Key)
		if a.ffmpegOutputDir() == "" {
			a.settings.FFmpegOutputDir = defaultFFmpegOutputDir()
			_ = a.settings.save()
		}
		a.redraw()
	}
}

func (a *app) ffmpegToolCard(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Padding(16, 15).Radius(14).Background(p.panel).Border(1, p.hair).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			bg, fg := p.hover, p.muted
			title := tr("app.ffmpeg.detecting_ffmpeg")
			desc := tr("app.ffmpeg.these_tools_use_ffmpeg_installed_on")
			if a.ffToolChecked {
				if a.ffTool.Path != "" {
					bg, fg = ui.Oklch(0.72, 0.18, 145).Alpha(0.14), ui.Oklch(0.62, 0.17, 145)
					title = tr("app.ffmpeg.ffmpeg_is_ready")
					desc = a.ffTool.Version
				} else {
					bg, fg = ui.Oklch(0.78, 0.16, 80).Alpha(0.16), ui.Oklch(0.58, 0.15, 65)
					title = tr("app.ffmpeg.ffmpeg_not_found")
					desc = tr("app.ffmpeg.install_ffmpeg_first_then_click_check")
				}
			}
			ui.Box(c).Size(40, 40).Radius(12).Center().Background(bg).Children(func() {
				ui.Icon(c, iconFilm).Size(21, 21).TextColor(fg)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).FontSize(13.5).FontWeight(650)
					if a.ffToolChecking {
						ui.Spinner(c).Size(12, 12)
					}
				})
				ui.Text(c, desc).FontSize(11.5).TextColor(p.muted).SingleLine()
				if a.ffTool.Path != "" {
					ui.Text(c, a.ffTool.Path).FontSize(10.5).TextColor(p.faint).SingleLine()
				}
				if a.ffTool.Path != "" && a.ffTool.ProbePath == "" {
					ui.Text(c, tr("app.ffmpeg.ffprobe_was_not_found_progress_percentages")).FontSize(10.5).TextColor(p.faint).SingleLine()
				}
			})
			if pillButtonKey(c, "ffmpeg-tool-check", iconRefresh, tr("app.common.check_again"), p.hover.Over(p.bg), p.text).Clicked() {
				a.detectFFmpegTool()
			}
		})
		if a.ffToolChecked && a.ffTool.Path == "" {
			ui.Column(c).FillWidth().Padding(12).Radius(10).Background(ui.Oklch(0.78, 0.16, 80).Alpha(0.1)).Gap(4).Children(func() {
				ui.Text(c, ffmpegHint()).FontSize(11.5).TextColor(p.text).Selectable()
				if a.ffToolErr != "" {
					ui.Text(c, a.ffToolErr).FontSize(10.5).TextColor(p.muted).Selectable().SingleLine()
				}
			})
		}
	})
}

func (a *app) ffmpegForm(c *ui.Context, p palette) {
	a.ffmpegToolCard(c, p)
	ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
		a.ffmpegInputsCard(c, p)
		a.ffmpegOutputCard(c, p)
		switch a.ffmpeg.cap {
		case string(ffmpeg.KindCompress):
			a.ffmpegCompressOptions(c, p)
		case string(ffmpeg.KindTrim):
			a.ffmpegTrimOptions(c, p)
		case string(ffmpeg.KindMerge):
			a.ffmpegMergeOptions(c, p)
		case string(ffmpeg.KindAudio):
			a.ffmpegAudioOptions(c, p)
		case string(ffmpeg.KindGIF):
			a.ffmpegGifOptions(c, p)
		case string(ffmpeg.KindWatermark):
			a.ffmpegWatermarkOptions(c, p)
		case string(ffmpeg.KindTransform):
			a.ffmpegTransformOptions(c, p)
		case string(ffmpeg.KindSubtitle):
			a.ffmpegSubtitleOptions(c, p)
		case string(ffmpeg.KindSpeed):
			a.ffmpegSpeedOptions(c, p)
		case string(ffmpeg.KindCover):
			a.ffmpegCoverOptions(c, p)
		case string(ffmpeg.KindBatch):
			a.ffmpegBatchOptions(c, p)
		}
		a.ffmpegActions(c, p)
		if run := a.ffmpeg.run; run != nil {
			a.ffmpegRunCard(c, p, run)
		}
	})
}

func (a *app) ffmpegInputsCard(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		audioOnly := a.ffmpeg.cap == string(ffmpeg.KindAudio)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			if audioOnly {
				ui.Text(c, tr("app.ffmpeg.input_audio_or_video")).FontSize(13).FontWeight(650)
			} else {
				ui.Text(c, tr("app.ffmpeg.input_video")).FontSize(13).FontWeight(650)
			}
			if len(a.ffmpeg.inputs) > 0 {
				ui.Text(c, fmt.Sprintf(tr("app.ffmpeg.files_selected"), len(a.ffmpeg.inputs))).FontSize(11).TextColor(p.faint)
			} else if audioOnly {
				ui.Text(c, tr("app.ffmpeg.supports_common_formats_such_as_mp4_2")).FontSize(11).TextColor(p.faint)
			} else {
				ui.Text(c, tr("app.ffmpeg.supports_common_formats_such_as_mp4")).FontSize(11).TextColor(p.faint)
			}
			ui.Spacer(c)
			if len(a.ffmpeg.inputs) > 0 && ui.Button(c.Key("ffmpeg-clear-inputs"), tr("app.common.clear_2")).Clicked() {
				a.ffmpeg.inputs = nil
				a.redraw()
			}
			label := tr("app.ffmpeg.add_videos")
			if audioOnly {
				label = tr("app.ffmpeg.add_files")
			}
			if pillButtonKey(c, "ffmpeg-pick-inputs", iconUpload, label, p.hover.Over(p.bg), p.text).Clicked() {
				a.pickFFmpegInputs()
			}
		})
		if len(a.ffmpeg.inputs) == 0 {
			ui.Row(c).FillWidth().Padding(18, 16).Radius(12).Background(p.hover.Over(p.panel)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconFilm).Size(20, 20).TextColor(p.faint)
				ui.Text(c, tr("app.ffmpeg.no_files_selected_yet_click_button")).FontSize(11.5).TextColor(p.muted)
			})
			return
		}
		merge := a.ffmpeg.cap == string(ffmpeg.KindMerge)
		max := len(a.ffmpeg.inputs)
		for i := 0; i < max; i++ {
			i := i
			path := a.ffmpeg.inputs[i]
			ui.Row(c).FillWidth().Padding(9, 9).Radius(10).Background(p.hover.Over(p.panel)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(34, 34).Radius(8).Shrink(0).Center().Background(p.panel).Children(func() {
					ui.Icon(c, iconFilm).Size(17, 17).TextColor(p.accent)
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, filepath.Base(path)).FontSize(12).FontWeight(600).SingleLine()
					meta := path
					if info, err := os.Stat(path); err == nil {
						meta = fmt.Sprintf("%s · %s", path, formatBytes(info.Size()))
					}
					ui.Text(c, meta).FontSize(10.5).TextColor(p.faint).SingleLine().Selectable()
				})
				if merge {
					if ui.Button(c.Key(fmt.Sprintf("ffmpeg-up-%d", i)), "↑").Clicked() && i > 0 {
						a.ffmpeg.inputs[i-1], a.ffmpeg.inputs[i] = a.ffmpeg.inputs[i], a.ffmpeg.inputs[i-1]
						a.redraw()
					}
					if ui.Button(c.Key(fmt.Sprintf("ffmpeg-down-%d", i)), "↓").Clicked() && i < len(a.ffmpeg.inputs)-1 {
						a.ffmpeg.inputs[i+1], a.ffmpeg.inputs[i] = a.ffmpeg.inputs[i], a.ffmpeg.inputs[i+1]
						a.redraw()
					}
				}
				if b := iconButton(c, iconInfo, 13, p.faint, tr("app.common.view_file_details")).Key(fmt.Sprintf("ffmpeg-info-%d", i)); b.Clicked() {
					a.openFileInfo(path, fileInfoMedia)
				}
				if b := iconButton(c, iconX, 12, p.faint, tr("app.common.remove")).Key(fmt.Sprintf("ffmpeg-remove-%d", i)); b.Clicked() {
					a.ffmpeg.inputs = append(a.ffmpeg.inputs[:i], a.ffmpeg.inputs[i+1:]...)
					a.redraw()
				}
			})
		}
		switch a.ffmpeg.cap {
		case string(ffmpeg.KindMerge):
			a.ffmpegNote(c, p, tr("app.ffmpeg.merging_follows_order_above_use_arrows"))
		case string(ffmpeg.KindBatch):
			a.ffmpegNote(c, p, tr("app.ffmpeg.batch_processing_runs_file_by_file"))
		}
	})
}

func (a *app) ffmpegOutputCard(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.common.output_folder")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("ffmpeg-output"), &a.settings.FFmpegOutputDir).Grow(1).Placeholder(tr("app.common.choose_output_folder_2"))
			if ui.Button(c.Key("ffmpeg-pick-output"), tr("app.common.select")).Clicked() {
				a.pickFFmpegOutputDir()
			}
			if dir := a.ffmpegOutputDir(); dir != "" {
				if ui.Button(c.Key("ffmpeg-open-output"), tr("app.common.open")).Clicked() {
					a.openPath(dir)
				}
			}
		})
		ui.Text(c, tr("app.common.results_are_written_to_this_folder")).FontSize(10.5).TextColor(p.faint).LineHeight(1.4)
	})
}

func (a *app) ffmpegCompressOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.compression_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, false)
			})
			ui.Column(c).Width(180).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.resolution"))
				a.ffmpegResolutionSelect(c, p)
			})
			ui.Column(c).Width(100).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.crf_quality"))
				ui.NumberInput(c.Key("ffmpeg-crf"), &a.ffmpeg.crf, 0, 51, 1)
			})
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.encoding_speed"))
				a.ffmpegPresetSelect(c, p)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.audio_bitrate"))
				a.ffmpegBitrateSelect(c, p)
			})
		})
		ui.Text(c, tr("app.ffmpeg.lower_crf_means_better_quality_and")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegTrimOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.trim_range")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(140).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.start_time"))
				ui.TextInput(c.Key("ffmpeg-start"), &a.ffmpeg.start).Placeholder("00:01:00")
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.duration"))
				ui.TextInput(c.Key("ffmpeg-duration"), &a.ffmpeg.duration).Placeholder("30")
			})
			ui.Column(c).Width(140).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.end_time_optional"))
				ui.TextInput(c.Key("ffmpeg-end"), &a.ffmpeg.end).Placeholder("00:01:30")
			})
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, true)
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c.Key("ffmpeg-copy"), &a.ffmpeg.copy).Label(tr("app.ffmpeg.fast_trim"))
			ui.Text(c, tr("app.ffmpeg.fast_trim_no_re_encoding")).FontSize(12)
		})
		ui.Text(c, tr("app.ffmpeg.time_accepts_seconds_mm_ss_or")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegMergeOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.merge_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, true)
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c.Key("ffmpeg-merge-copy"), &a.ffmpeg.copy).Label(tr("app.ffmpeg.direct_stream_copy"))
			ui.Text(c, tr("app.ffmpeg.direct_stream_copy_no_re_encoding")).FontSize(12)
		})
		ui.Text(c, tr("app.ffmpeg.direct_stream_copy_is_fastest_but")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegAudioOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.audio_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.audio_format"))
				a.ffmpegAudioFormatSelect(c, p)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.audio_bitrate"))
				a.ffmpegBitrateSelect(c, p)
			})
		})
		ui.Text(c, tr("app.ffmpeg.audio_extraction_removes_video_mp3_m4a")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegGifOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.gif_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.start_time"))
				ui.TextInput(c.Key("ffmpeg-gif-start"), &a.ffmpeg.start).Placeholder("00:00:00")
			})
			ui.Column(c).Width(110).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.duration_seconds"))
				ui.TextInput(c.Key("ffmpeg-gif-duration"), &a.ffmpeg.duration).Placeholder("3")
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.width_px"))
				ui.NumberInput(c.Key("ffmpeg-gif-width"), &a.ffmpeg.gifWidth, 16, 4096, 10)
			})
			ui.Column(c).Width(100).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.frame_rate"))
				ui.NumberInput(c.Key("ffmpeg-gif-fps"), &a.ffmpeg.fps, 1, 60, 1)
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c.Key("ffmpeg-gif-hq"), &a.ffmpeg.hq).Label(tr("app.ffmpeg.high_quality_palette"))
			ui.Text(c, tr("app.ffmpeg.high_quality_palette")).FontSize(12)
		})
		ui.Text(c, tr("app.ffmpeg.gifs_loop_automatically_higher_frame_rate")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegWatermarkOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.common.watermark_settings")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.ffmpeg.wmTab, tr("app.common.text_watermark"), tr("app.common.image_watermark")).Label(tr("app.common.type")).Width(280)
		if a.ffmpeg.wmTab == 0 {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.watermark_text"))
				ui.TextInput(c.Key("ffmpeg-wm-text"), &a.ffmpeg.wmText).Placeholder(tr("app.common.e_g_zchat"))
			})
		} else {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.watermark_image"))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("ffmpeg-wm-image"), &a.ffmpeg.wmImage).Grow(1).Placeholder(tr("app.common.png_or_jpg_transparent_background_works"))
					if ui.Button(c.Key("ffmpeg-pick-wm"), tr("app.common.select")).Clicked() {
						a.pickFFmpegWatermark()
					}
				})
			})
		}
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(140).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.position"))
				a.ffmpegPositionSelect(c, p)
			})
			ui.Column(c).Width(100).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.margin"))
				ui.NumberInput(c.Key("ffmpeg-wm-margin"), &a.ffmpeg.margin, 0, 1000, 2)
			})
			ui.Column(c).Width(110).Gap(5).Children(func() {
				if a.ffmpeg.wmTab == 1 {
					fieldLabel(c, p, tr("app.ffmpeg.watermark_width"))
					ui.NumberInput(c.Key("ffmpeg-wm-width"), &a.ffmpeg.imageWidth, 16, 4096, 10)
				} else {
					fieldLabel(c, p, tr("app.common.font_size"))
					ui.NumberInput(c.Key("ffmpeg-wm-font"), &a.ffmpeg.fontSize, 8, 400, 1)
				}
			})
			ui.Column(c).Width(110).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.opacity"))
				ui.NumberInput(c.Key("ffmpeg-wm-opacity"), &a.ffmpeg.opacity, 0.05, 1, 0.05)
			})
			if a.ffmpeg.wmTab == 0 {
				ui.Column(c).Width(110).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.text_color"))
					ui.TextInput(c.Key("ffmpeg-wm-color"), &a.ffmpeg.fontColor).Placeholder("#ffffff")
				})
			}
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, false)
			})
		})
		ui.Text(c, tr("app.ffmpeg.text_watermarks_use_system_fonts_install")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegTransformOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.picture_adjustment")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(190).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.adjustment"))
				values := []string{ffmpeg.TransformResize, ffmpeg.TransformCrop, ffmpeg.TransformPad, ffmpeg.TransformRotate}
				labels := []string{tr("app.ffmpeg.resize"), tr("app.common.crop_picture"), tr("app.ffmpeg.aspect_fit"), tr("app.ffmpeg.rotate_or_flip")}
				selectKeyed(c, "ffmpeg-transform-mode", &a.ffmpeg.transformMode, values, labels)
			})
			if a.ffmpeg.transformMode != ffmpeg.TransformRotate {
				ui.Column(c).Width(120).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.width_px"))
					ui.NumberInput(c.Key("ffmpeg-width"), &a.ffmpeg.width, 0, 20000, 10)
				})
				ui.Column(c).Width(120).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.height_px"))
					ui.NumberInput(c.Key("ffmpeg-height"), &a.ffmpeg.height, 0, 20000, 10)
				})
			}
			if a.ffmpeg.transformMode == ffmpeg.TransformCrop {
				ui.Column(c).Width(100).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.start_x"))
					ui.NumberInput(c.Key("ffmpeg-crop-x"), &a.ffmpeg.cropX, 0, 20000, 10)
				})
				ui.Column(c).Width(100).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.start_y"))
					ui.NumberInput(c.Key("ffmpeg-crop-y"), &a.ffmpeg.cropY, 0, 20000, 10)
				})
			}
			if a.ffmpeg.transformMode == ffmpeg.TransformRotate {
				ui.Column(c).Width(160).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.rotate_or_flip"))
					values := []string{"90", "180", "270", "flip-h", "flip-v"}
					labels := []string{tr("app.ffmpeg.n_90_clockwise"), tr("app.ffmpeg.rotate_180"), tr("app.ffmpeg.n_90_counterclockwise"), tr("app.ffmpeg.flip_horizontally"), tr("app.ffmpeg.flip_vertically")}
					selectKeyed(c, "ffmpeg-rotation", &a.ffmpeg.rotation, values, labels)
				})
			}
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, false)
			})
		})
		if a.ffmpeg.transformMode == ffmpeg.TransformPad {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr("app.ffmpeg.common_canvases")).FontSize(10.5).TextColor(p.faint)
				if ui.Button(c.Key("ffmpeg-preset-horizontal"), tr("app.ffmpeg.landscape_1920_1080")).Clicked() {
					a.ffmpeg.width, a.ffmpeg.height = 1920, 1080
					a.redraw()
				}
				if ui.Button(c.Key("ffmpeg-preset-vertical"), tr("app.ffmpeg.portrait_1080_1920")).Clicked() {
					a.ffmpeg.width, a.ffmpeg.height = 1080, 1920
					a.redraw()
				}
				if ui.Button(c.Key("ffmpeg-preset-square"), tr("app.ffmpeg.square_1080_1080")).Clicked() {
					a.ffmpeg.width, a.ffmpeg.height = 1080, 1080
					a.redraw()
				}
			})
		}
		note := tr("app.ffmpeg.resize_scales_to_width_and_height")
		switch a.ffmpeg.transformMode {
		case ffmpeg.TransformCrop:
			note = tr("app.ffmpeg.cropping_uses_top_left_corner_as")
		case ffmpeg.TransformPad:
			note = tr("app.ffmpeg.aspect_fit_scales_video_down_proportionally")
		case ffmpeg.TransformRotate:
			note = tr("app.ffmpeg.rotation_and_flipping_re_encode_video")
		}
		ui.Text(c, note).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegSubtitleOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.subtitles_and_music")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.ffmpeg.subtitleMode, tr("app.ffmpeg.burn_subtitles"), tr("app.ffmpeg.replace_background_music")).Label(tr("app.common.mode")).Width(300)
		if a.ffmpeg.subtitleMode == 0 {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.subtitle_file"))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("ffmpeg-subtitle-file"), &a.ffmpeg.subtitleFile).Grow(1).Placeholder(tr("app.ffmpeg.choose_srt_subtitle_file"))
					if ui.Button(c.Key("ffmpeg-pick-subtitle"), tr("app.common.select")).Clicked() {
						a.pickFFmpegSubtitle()
					}
				})
			})
		} else {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.background_music"))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("ffmpeg-bgm-file"), &a.ffmpeg.bgmFile).Grow(1).Placeholder(tr("app.ffmpeg.choose_mp3_m4a_wav_or_other"))
					if ui.Button(c.Key("ffmpeg-pick-bgm"), tr("app.common.select")).Clicked() {
						a.pickFFmpegBGM()
					}
				})
			})
		}
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, false)
			})
		})
		if a.ffmpeg.subtitleMode == 0 {
			ui.Text(c, tr("app.ffmpeg.subtitles_are_rendered_into_video_so")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
		} else {
			ui.Text(c, tr("app.ffmpeg.background_music_replaces_original_audio_and")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
		}
	})
}

func (a *app) ffmpegSpeedOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.speed_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.speed"))
				ui.NumberInput(c.Key("ffmpeg-speed"), &a.ffmpeg.speed, 0.25, 4, 0.25)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.ffmpegVideoFormatSelect(c, p, false)
			})
			ui.Text(c, tr("app.ffmpeg.for_example_2_means_2_speed")).FontSize(10.5).TextColor(p.faint).Margin(0, 0, 0, 4)
		})
		ui.Text(c, tr("app.ffmpeg.speed_changes_keep_video_and_audio")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegCoverOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.ffmpeg.capture_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.capture_time"))
				ui.TextInput(c.Key("ffmpeg-cover-at"), &a.ffmpeg.at).Placeholder("00:00:05")
			})
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.interval_in_seconds_0_for_one"))
				ui.NumberInput(c.Key("ffmpeg-cover-interval"), &a.ffmpeg.interval, 0, 3600, 1)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.ffmpeg.image_format"))
				a.ffmpegCoverFormatSelect(c, p)
			})
		})
		ui.Text(c, tr("app.ffmpeg.when_interval_is_0_one_cover")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegBatchOptions(c *ui.Context, p palette) {
	a.ffmpegCard(c, p, func() {
		ui.Text(c, tr("app.common.batch_settings")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.ffmpeg.batchMode, tr("app.ffmpeg.compress_video"), tr("app.ffmpeg.extract_audio"), tr("app.ffmpeg.capture_cover")).Label(tr("app.common.mode")).Width(340)
		switch a.ffmpeg.batchMode {
		case 1:
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.audio_format"))
					a.ffmpegAudioFormatSelect(c, p)
				})
				ui.Column(c).Width(130).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.audio_bitrate"))
					a.ffmpegBitrateSelect(c, p)
				})
			})
		case 2:
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.capture_time"))
					ui.TextInput(c.Key("ffmpeg-batch-at"), &a.ffmpeg.at).Placeholder("00:00:05")
				})
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.interval_in_seconds_0_for_one"))
					ui.NumberInput(c.Key("ffmpeg-batch-interval"), &a.ffmpeg.interval, 0, 3600, 1)
				})
				ui.Column(c).Width(120).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.image_format"))
					a.ffmpegCoverFormatSelect(c, p)
				})
			})
		default:
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
				ui.Column(c).Width(130).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.output_format"))
					a.ffmpegVideoFormatSelect(c, p, false)
				})
				ui.Column(c).Width(180).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.resolution"))
					a.ffmpegResolutionSelect(c, p)
				})
				ui.Column(c).Width(100).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.crf_quality"))
					ui.NumberInput(c.Key("ffmpeg-batch-crf"), &a.ffmpeg.crf, 0, 51, 1)
				})
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.encoding_speed"))
					a.ffmpegPresetSelect(c, p)
				})
				ui.Column(c).Width(120).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.ffmpeg.audio_bitrate"))
					a.ffmpegBitrateSelect(c, p)
				})
			})
		}
		ui.Text(c, tr("app.ffmpeg.batch_processing_produces_separate_result_for")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) ffmpegVideoFormatSelect(c *ui.Context, p palette, allowKeep bool) {
	values := []string{"mp4", "mkv", "mov"}
	labels := []string{"MP4", "MKV", "MOV"}
	if allowKeep {
		values = append([]string{"keep"}, values...)
		labels = append([]string{tr("app.common.keep_original_format")}, labels...)
	}
	selectKeyed(c, "ffmpeg-video-format", &a.ffmpeg.format, values, labels)
}

func (a *app) ffmpegCoverFormatSelect(c *ui.Context, p palette) {
	values := []string{"jpg", "png"}
	labels := []string{"JPG", "PNG"}
	selectKeyed(c, "ffmpeg-cover-format", &a.ffmpeg.format, values, labels)
}

func (a *app) ffmpegAudioFormatSelect(c *ui.Context, p palette) {
	values := []string{"mp3", "m4a", "aac", "wav"}
	labels := []string{"MP3", "M4A", "AAC", "WAV"}
	selectKeyed(c, "ffmpeg-audio-format", &a.ffmpeg.audioFormat, values, labels)
}

func (a *app) ffmpegResolutionSelect(c *ui.Context, p palette) {
	values := []string{"source", "1080p", "720p", "480p", "vertical", "horizontal", "square"}
	labels := []string{tr("app.ffmpeg.keep_source_resolution"), "1080p", "720p", "480p", tr("app.ffmpeg.portrait_1080_1920"), tr("app.ffmpeg.landscape_1920_1080"), tr("app.ffmpeg.square_1080_1080")}
	selectKeyed(c, "ffmpeg-resolution", &a.ffmpeg.resolution, values, labels)
}

func (a *app) ffmpegPresetSelect(c *ui.Context, p palette) {
	values := []string{"ultrafast", "veryfast", "fast", "medium", "slow"}
	labels := []string{tr("app.ffmpeg.ultra_fast"), tr("app.ffmpeg.very_fast"), tr("app.common.fast"), tr("app.ffmpeg.balanced"), tr("app.ffmpeg.slow_smaller")}
	selectKeyed(c, "ffmpeg-preset", &a.ffmpeg.preset, values, labels)
}

func (a *app) ffmpegBitrateSelect(c *ui.Context, p palette) {
	values := []string{"96k", "128k", "192k", "256k"}
	labels := []string{"96 kbps", "128 kbps", "192 kbps", "256 kbps"}
	selectKeyed(c, "ffmpeg-bitrate", &a.ffmpeg.audioBitrate, values, labels)
}

func (a *app) ffmpegPositionSelect(c *ui.Context, p palette) {
	values := []string{"topleft", "topright", "center", "bottomleft", "bottomright"}
	labels := []string{tr("app.ffmpeg.top_left"), tr("app.common.top_right_2"), tr("app.common.center"), tr("app.ffmpeg.bottom_left"), tr("app.ffmpeg.bottom_right")}
	selectKeyed(c, "ffmpeg-position", &a.ffmpeg.position, values, labels)
}

func (a *app) ffmpegActions(c *ui.Context, p palette) {
	if a.ffmpegRunning() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			if pillButtonKey(c, "ffmpeg-cancel", iconPause, tr("app.common.cancel_processing"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelFFmpegRun()
			}
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, tr("app.common.processing_files_already_written_are_kept")).FontSize(11.5).TextColor(p.muted)
		})
		return
	}
	label := tr("app.common.start_processing")
	if run := a.ffmpeg.run; run != nil && run.finished {
		label = tr("app.common.process_again")
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		if pillButtonKey(c, "ffmpeg-run", iconPlay, label, p.accent, p.onAcc).Clicked() {
			a.startFFmpegRun()
		}
		if pillButtonKey(c, "ffmpeg-reset", iconRefresh, tr("app.ffmpeg.restore_recommended"), p.hover.Over(p.bg), p.text).Clicked() {
			a.resetFFmpegDefaults()
		}
		ui.Text(c, tr("app.ffmpeg.input_files_output_folder_and_ffmpeg")).FontSize(11).TextColor(p.faint)
	})
}

func (a *app) ffmpegRunCard(c *ui.Context, p palette, run *ffmpegRun) {
	status, color, icon := tr("app.common.processing"), p.accent, iconFilm
	switch {
	case run.canceled:
		status, color, icon = tr("app.common.canceled"), ui.Oklch(0.78, 0.16, 80), iconPause
	case run.finished && len(run.errors) > 0 && run.files > 0:
		status, color, icon = tr("app.common.partially_complete"), ui.Oklch(0.78, 0.16, 80), iconInfo
	case run.finished && len(run.errors) > 0:
		status, color, icon = tr("app.common.processing_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
	case run.finished && run.files > 0:
		status, color, icon = tr("app.common.processing_complete"), ui.Oklch(0.68, 0.17, 145), iconCheck
	case run.finished:
		status, color, icon = tr("app.common.processing_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
	}
	a.ffmpegCard(c, p, func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(38, 38).Radius(11).Center().Background(color.Alpha(0.14)).Children(func() {
				ui.Icon(c, icon).Size(20, 20).TextColor(color)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, status).FontSize(14).FontWeight(700).TextColor(color)
					if run.running {
						ui.Spinner(c).Size(12, 12)
					}
				})
				ui.Text(c, run.current).FontSize(11).TextColor(p.muted).SingleLine()
			})
			if run.output != "" {
				if _, err := os.Stat(run.output); err == nil {
					if pillButtonKey(c, "ffmpeg-open-run-output", iconFolder, tr("app.common.open_output_folder"), p.hover.Over(p.bg), p.text).Clicked() {
						a.openPath(run.output)
					}
				}
			}
		})
		ratio := float64(0)
		if run.total > 0 {
			ratio = (float64(run.done) + run.percent) / float64(run.total)
		}
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		ui.Progress(c, ratio).Height(6).Radius(3)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d / %d", run.done, run.total)).FontSize(11).TextColor(p.muted)
			if run.running && run.percent > 0 {
				ui.Text(c, fmt.Sprintf("%.0f%%", ratio*100)).FontSize(11).TextColor(p.muted)
			}
			if run.files > 0 {
				ui.Text(c, fmt.Sprintf(tr("app.common.wrote_files"), run.files)).FontSize(11).TextColor(p.muted)
			}
			if run.elapsed > 0 {
				ui.Text(c, tr("app.common.time")+" "+formatRunDuration(run.elapsed)).FontSize(11).TextColor(p.muted)
			}
		})
		if len(run.errors) > 0 {
			ui.Column(c).FillWidth().Padding(10, 12).Radius(10).Background(ui.Oklch(0.64, 0.2, 28).Alpha(0.09)).Gap(4).Children(func() {
				ui.Text(c, fmt.Sprintf(tr("app.common.files_failed_2"), len(run.errors))).FontSize(11.5).FontWeight(650)
				max := len(run.errors)
				if max > 6 {
					max = 6
				}
				for _, msg := range run.errors[:max] {
					ui.Text(c, "• "+msg).FontSize(10.5).TextColor(p.muted).Selectable()
				}
				if len(run.errors) > max {
					ui.Text(c, fmt.Sprintf(tr("app.common.more_errors_see_log"), len(run.errors)-max)).FontSize(10.5).TextColor(p.faint)
				}
			})
		}
		if len(run.log) > 0 {
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				ui.Text(c, tr("app.common.processing_log")).FontSize(11.5).FontWeight(650).TextColor(p.muted)
				lines := run.log
				if len(lines) > 10 {
					lines = lines[len(lines)-10:]
				}
				for _, line := range lines {
					ui.Text(c, line).FontSize(10.5).TextColor(p.muted).SingleLine().Selectable()
				}
			})
		}
		if run.finished && run.files > 0 {
			ui.Text(c, tr("app.common.results_are_in_output_folder_source")).FontSize(11).TextColor(p.faint)
		}
	})
}

func (a *app) ffmpegCard(c *ui.Context, p palette, fn func()) {
	ui.Column(c).FillWidth().Padding(16, 15).Radius(14).Background(p.panel).Border(1, p.hair).Gap(12).Children(fn)
}

func (a *app) ffmpegNote(c *ui.Context, p palette, text string) {
	ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconInfo).Size(12, 12).TextColor(p.faint).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(10.5).TextColor(p.muted).LineHeight(1.45).Grow(1).Shrink(1).MinWidth(0)
	})
}

func (a *app) ffmpegOutputDir() string {
	return strings.TrimSpace(a.settings.FFmpegOutputDir)
}

func defaultFFmpegOutputDir() string {
	videos, err := mygo.App.Path(mygo.PathVideos)
	if err != nil || videos == "" {
		videos, _ = mygo.App.Path(mygo.PathDownloads)
	}
	if videos == "" {
		return ""
	}
	return filepath.Join(videos, appFolderName, "ffmpeg")
}

func ffmpegHint() string {
	switch runtime.GOOS {
	case "windows":
		return tr("app.ffmpeg.install_winget_install_gyan_ffmpeg_or")
	case "darwin":
		return tr("app.ffmpeg.install_brew_install_ffmpeg")
	default:
		return tr("app.ffmpeg.install_sudo_apt_install_ffmpeg_or")
	}
}

func (a *app) ffmpegBack() {
	if a.ffmpegRunning() {
		a.cancelFFmpegRun()
	}
	a.ffmpeg.run = nil
	a.ffmpeg.cap = ""
	a.redraw()
}

func (a *app) ffmpegRunning() bool {
	return a.ffmpeg.run != nil && a.ffmpeg.run.running
}

func (a *app) resetFFmpegDefaults() {
	if a.ffmpegRunning() {
		return
	}
	cap := a.ffmpeg.cap
	inputs := a.ffmpeg.inputs
	run := a.ffmpeg.run
	a.ffmpeg = ffmpegState{cap: cap, inputs: inputs, run: run}
	a.ffmpegDefaults()
	a.ffmpegEnterCap(cap)
	a.setToast(tr("app.ffmpeg.recommended_settings_restored"))
	a.redraw()
}

func (a *app) ffmpegOptions() ffmpeg.Options {
	f := &a.ffmpeg
	opt := ffmpeg.Options{
		Kind:          ffmpeg.Kind(f.cap),
		Inputs:        append([]string(nil), f.inputs...),
		OutputDir:     a.ffmpegOutputDir(),
		Format:        f.format,
		CRF:           int(f.crf),
		Preset:        f.preset,
		Resolution:    f.resolution,
		AudioBitrate:  f.audioBitrate,
		Start:         f.start,
		Duration:      f.duration,
		End:           f.end,
		Copy:          f.copy,
		AudioFormat:   f.audioFormat,
		FPS:           int(f.fps),
		Width:         int(f.width),
		HighQuality:   f.hq,
		Text:          f.wmText,
		Image:         strings.TrimSpace(f.wmImage),
		Position:      f.position,
		Margin:        int(f.margin),
		FontSize:      int(f.fontSize),
		FontColor:     f.fontColor,
		Opacity:       f.opacity,
		ImageWidth:    int(f.imageWidth),
		TransformMode: f.transformMode,
		Height:        int(f.height),
		CropX:         int(f.cropX),
		CropY:         int(f.cropY),
		Rotation:      f.rotation,
		SubtitleFile:  strings.TrimSpace(f.subtitleFile),
		BGMFile:       strings.TrimSpace(f.bgmFile),
		Speed:         f.speed,
		At:            f.at,
		Interval:      int(f.interval),
	}
	if f.cap == string(ffmpeg.KindGIF) {
		opt.Width = int(f.gifWidth)
	}
	switch f.cap {
	case string(ffmpeg.KindWatermark):
		opt.WatermarkMode = ffmpeg.WatermarkText
		if f.wmTab == 1 {
			opt.WatermarkMode = ffmpeg.WatermarkImage
		}
	case string(ffmpeg.KindSubtitle):
		opt.SubtitleMode = ffmpeg.SubtitleBurn
		if f.subtitleMode == 1 {
			opt.SubtitleMode = ffmpeg.SubtitleBGM
		}
	case string(ffmpeg.KindBatch):
		switch f.batchMode {
		case 1:
			opt.BatchMode = ffmpeg.BatchAudio
		case 2:
			opt.BatchMode = ffmpeg.BatchCover
		default:
			opt.BatchMode = ffmpeg.BatchCompress
		}
	}
	return opt
}

func (a *app) persistFFmpegSettings() {
	a.settings.FFmpegCRF = int(a.ffmpeg.crf)
	a.settings.FFmpegPreset = a.ffmpeg.preset
	a.settings.FFmpegAudioRate = a.ffmpeg.audioBitrate
	_ = a.settings.save()
}

func (a *app) detectFFmpegTool() {
	if a.ffToolChecking {
		return
	}
	a.ffToolChecking = true
	a.redraw()
	go func() {
		tool, err := ffmpeg.Detect()
		a.post(func() {
			a.ffToolChecking = false
			a.ffToolChecked = true
			if err != nil {
				a.ffTool = ffmpeg.Tool{}
				a.ffToolErr = err.Error()
			} else {
				a.ffTool = tool
				a.ffToolErr = ""
			}
			a.redraw()
		})
	}()
}

func (a *app) pickFFmpegInputs() {
	if a.ffmpegRunning() {
		return
	}
	audioOnly := a.ffmpeg.cap == string(ffmpeg.KindAudio)
	go func() {
		title := tr("app.ffmpeg.choose_videos")
		name := tr("app.common.video")
		exts := []string{"mp4", "mov", "mkv", "avi", "webm", "flv", "wmv", "m4v", "ts", "m2ts", "mpg", "mpeg", "3gp"}
		if audioOnly {
			title = tr("app.ffmpeg.choose_audio_or_video")
			name = tr("app.ffmpeg.audio_and_video")
			exts = append(exts, "mp3", "m4a", "aac", "wav", "flac", "ogg", "opus")
		}
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       title,
			ButtonLabel: tr("app.common.add"),
			Message:     tr("app.ffmpeg.you_can_select_multiple_files_merging"),
			Multiple:    true,
			Filters:     []mygo.FileFilter{{Name: name, Extensions: exts}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			seen := map[string]bool{}
			for _, in := range a.ffmpeg.inputs {
				seen[in] = true
			}
			for _, path := range paths {
				if !seen[path] {
					seen[path] = true
					a.ffmpeg.inputs = append(a.ffmpeg.inputs, path)
				}
			}
			a.redraw()
		})
	}()
}

func (a *app) pickFFmpegWatermark() {
	if a.ffmpegRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.common.choose_watermark_image_2"),
			ButtonLabel: tr("app.common.use_this_image"),
			Filters: []mygo.FileFilter{{
				Name:       tr("app.common.images"),
				Extensions: []string{"png", "jpg", "jpeg", "webp", "gif"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.ffmpeg.wmImage = paths[0]
			a.redraw()
		})
	}()
}

func (a *app) pickFFmpegSubtitle() {
	if a.ffmpegRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.ffmpeg.choose_subtitle_file_2"),
			ButtonLabel: tr("app.ffmpeg.use_this_subtitle"),
			Filters: []mygo.FileFilter{{
				Name:       tr("app.ffmpeg.subtitles"),
				Extensions: []string{"srt", "ass", "ssa", "vtt"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.ffmpeg.subtitleFile = paths[0]
			a.redraw()
		})
	}()
}

func (a *app) pickFFmpegBGM() {
	if a.ffmpegRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.ffmpeg.choose_background_music"),
			ButtonLabel: tr("app.ffmpeg.use_this_audio"),
			Filters: []mygo.FileFilter{{
				Name:       tr("app.common.audio"),
				Extensions: []string{"mp3", "m4a", "aac", "wav", "flac", "ogg", "opus"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.ffmpeg.bgmFile = paths[0]
			a.redraw()
		})
	}()
}

func (a *app) pickFFmpegOutputDir() {
	if a.ffmpegRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            a.win,
			Title:             tr("app.common.choose_output_folder_3"),
			ButtonLabel:       tr("app.common.use_this_folder"),
			Directory:         true,
			CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.FFmpegOutputDir = paths[0]
			_ = a.settings.save()
			a.redraw()
		})
	}()
}

func (a *app) startFFmpegRun() {
	if a.ffmpegRunning() {
		return
	}
	opt := a.ffmpegOptions()
	if err := ffmpeg.Validate(opt); err != nil {
		a.setToast(err.Error())
		return
	}
	a.persistFFmpegSettings()
	run := &ffmpegRun{
		running: true,
		current: tr("app.ffmpeg.detecting_ffmpeg"),
		output:  opt.OutputDir,
		started: time.Now(),
	}
	a.ffmpeg.run = run
	ctx, cancel := context.WithCancel(context.Background())
	a.ffmpeg.cancel = cancel
	a.redraw()
	go func() {
		tool, err := ffmpeg.Detect()
		if err != nil {
			a.post(func() {
				if a.ffmpeg.run != run {
					return
				}
				a.ffTool = ffmpeg.Tool{}
				a.ffToolErr = err.Error()
				a.ffToolChecked = true
				a.finishFFmpegRun(run, ffmpeg.Result{}, tr("app.ffmpeg.ffmpeg_not_found_2")+err.Error())
			})
			return
		}
		a.post(func() {
			if a.ffmpeg.run == run {
				a.ffTool = tool
				a.ffToolErr = ""
				a.ffToolChecked = true
			}
		})
		plan, err := ffmpeg.Build(tool, opt)
		if err != nil {
			a.post(func() {
				if a.ffmpeg.run == run {
					a.finishFFmpegRun(run, ffmpeg.Result{}, err.Error())
				}
			})
			return
		}
		a.post(func() {
			if a.ffmpeg.run == run {
				run.total = len(plan.Steps)
				run.current = tr("app.ffmpeg.processing_video")
			}
		})
		res := plan.Run(ctx, func(ev ffmpeg.Progress) {
			a.post(func() {
				if a.ffmpeg.run == run {
					a.applyFFmpegProgress(run, ev)
				}
			})
		})
		a.post(func() {
			if a.ffmpeg.run == run {
				a.finishFFmpegRun(run, res, "")
			}
		})
	}()
}

func (a *app) applyFFmpegProgress(run *ffmpegRun, ev ffmpeg.Progress) {
	if ev.Total > 0 {
		run.total = ev.Total
	}
	if ev.Label != "" {
		run.current = ev.Label
	}
	run.percent = ev.Percent
	if ev.Done > run.done {
		line := "✓ " + ev.Label
		if ev.Err != nil {
			line = "✗ " + ev.Label + " — " + ev.Err.Error()
		}
		run.log = appendTail(run.log, line, 200)
		run.done = ev.Done
		run.percent = 0
	}
}

func (a *app) finishFFmpegRun(run *ffmpegRun, res ffmpeg.Result, fatal string) {
	run.running = false
	run.finished = true
	run.elapsed = time.Since(run.started)
	run.files = res.Files
	run.errors = res.Errors
	run.canceled = res.Canceled
	if fatal != "" {
		run.errors = append(run.errors, fatal)
		run.current = fatal
		a.setToast(fatal)
	} else if res.Canceled {
		run.current = tr("app.common.processing_canceled_files_already_written_are")
		a.setToast(tr("app.common.processing_canceled_files_already_written_are"))
	} else if len(res.Errors) > 0 {
		run.current = tr("app.common.some_files_failed")
		a.setToast(fmt.Sprintf(tr("app.common.processed_files_failed"), res.Files, len(res.Errors)))
	} else if res.Files == 0 {
		run.current = tr("app.common.no_files_were_written")
		a.setToast(tr("app.common.processing_failed_no_files_were_written"))
	} else {
		run.current = tr("app.common.all_files_processed")
		a.setToast(fmt.Sprintf(tr("app.common.processed_files_in"), res.Files, formatRunDuration(res.Duration)))
	}
	if a.ffmpeg.cancel != nil {
		a.ffmpeg.cancel()
		a.ffmpeg.cancel = nil
	}
	a.redraw()
}

func (a *app) cancelFFmpegRun() {
	if a.ffmpeg.cancel != nil {
		a.ffmpeg.cancel()
	}
	if run := a.ffmpeg.run; run != nil {
		run.current = tr("app.common.canceling")
	}
	a.redraw()
}
