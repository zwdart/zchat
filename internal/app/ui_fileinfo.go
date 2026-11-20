package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/ffmpeg"
	"xin.dart.zchat/internal/magick"
)

const (
	fileInfoImage = "image"
	fileInfoMedia = "media"
)

type fileInfoEntry struct {
	Kind    string
	Path    string
	Loading bool
	Err     string
	Media   *ffmpeg.MediaInfo
	Image   *magick.ImageInfo
}

type fileInfoPair struct {
	Label string
	Value string
}

func (a *app) openFileInfo(path, kind string) {
	key := kind + "\x00" + path
	if a.fileInfoCache == nil {
		a.fileInfoCache = map[string]*fileInfoEntry{}
	}
	a.fileInfoOpen = true
	a.fileInfoKey = key
	if _, ok := a.fileInfoCache[key]; ok {
		a.redraw()
		return
	}
	entry := &fileInfoEntry{Kind: kind, Path: path, Loading: true}
	a.fileInfoCache[key] = entry
	videoTool, imageTool := a.ffTool, a.iconTool
	a.redraw()
	go func() {
		var (
			media *ffmpeg.MediaInfo
			image *magick.ImageInfo
			err   error
		)
		switch kind {
		case fileInfoMedia:
			tool := videoTool
			if tool.Path == "" {
				tool, err = ffmpeg.Detect()
			}
			if err == nil {
				var info ffmpeg.MediaInfo
				info, err = tool.MediaInfo(path)
				if err == nil {
					media = &info
				}
			}
		case fileInfoImage:
			tool := imageTool
			if tool.Path == "" {
				tool, err = magick.Detect()
			}
			if err == nil {
				var info magick.ImageInfo
				info, err = tool.ImageInfo(path)
				if err == nil {
					image = &info
				}
			}
		}
		a.post(func() {
			entry.Loading = false
			entry.Media = media
			entry.Image = image
			if err != nil {
				entry.Err = err.Error()
			}
			a.redraw()
		})
	}()
}

func (a *app) fileInfoDialog(c *ui.Context) {
	if !a.fileInfoOpen {
		return
	}
	entry := a.fileInfoCache[a.fileInfoKey]
	if entry == nil {
		a.fileInfoOpen = false
		return
	}
	p := a.pal
	ui.DialogBase(c.Key("file-info"), &a.fileInfoOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 520)
		icon, kind := iconFilm, tr("app.file.media_details")
		switch {
		case entry.Kind == fileInfoImage:
			icon, kind = iconImage, tr("app.common.image_details")
		case entry.Media != nil && entry.Media.Video != nil:
			kind = tr("app.file.video_details")
		case entry.Media != nil:
			kind = tr("app.file.audio_details")
		}
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(42, 42).Radius(12).Shrink(0).Center().Background(p.accent.Alpha(0.12)).Children(func() {
				ui.Icon(c, icon).Size(21, 21).TextColor(p.accent)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, filepath.Base(entry.Path)).FontSize(16).FontWeight(700).SingleLine()
				ui.Text(c, kind).FontSize(11.5).TextColor(p.faint)
			})
		})

		if entry.Loading {
			ui.Row(c).FillWidth().Padding(18, 16).Radius(12).Background(p.panel).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(15, 15)
				ui.Text(c, tr("app.file.reading_file_details")).FontSize(12).TextColor(p.muted)
			})
		} else {
			if highlights := fileInfoHighlights(entry); len(highlights) > 0 {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					for _, item := range highlights {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Padding(10, 11).Radius(10).
							Background(p.panel).Gap(3).Children(func() {
							ui.Text(c, item.Label).FontSize(10.5).TextColor(p.faint)
							ui.Text(c, item.Value).FontSize(14).FontWeight(650).SingleLine()
						})
					}
				})
			}
			if details := fileInfoDetails(entry); len(details) > 0 {
				ui.Scroll(c).MaxHeight(300).Children(func() {
					ui.Column(c).FillWidth().Padding(12, 13).Radius(12).Background(p.panel).Gap(7).Children(func() {
						for _, item := range details {
							ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
								ui.Text(c, item.Label).Width(104).Shrink(0).FontSize(11.5).TextColor(p.faint)
								ui.Text(c, item.Value).FontSize(12).Grow(1).Shrink(1).MinWidth(0).Selectable()
							})
						}
					})
				})
			}
			if entry.Err != "" {
				ui.Row(c).FillWidth().Padding(10, 11).Radius(10).
					Background(ui.Oklch(0.78, 0.16, 80).Alpha(0.12)).Gap(8).AlignItems(ui.Start).Children(func() {
					ui.Icon(c, iconInfo).Size(14, 14).TextColor(ui.Oklch(0.58, 0.15, 65)).Margin(1, 0, 0, 0)
					ui.Text(c, entry.Err).FontSize(10.5).TextColor(p.muted).LineHeight(1.4).Selectable()
				})
			}
		}

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if pillButtonKey(c, "file-info-open", iconFolder, tr("app.common.open"), p.hover.Over(p.bg), p.text).Clicked() {
				a.openPath(entry.Path)
			}
			if pillButtonKey(c, "file-info-reveal", nil, tr("app.common.show_in_folder"), p.hover.Over(p.bg), p.text).Clicked() {
				mygo.Shell.ShowItemInFolder(entry.Path)
			}
			if pillButtonKey(c, "file-info-copy", nil, tr("app.file.copy_path"), p.hover.Over(p.bg), p.text).Clicked() {
				c.WriteClipboard(entry.Path)
				a.setToast(tr("app.file.path_copied"))
			}
			ui.Spacer(c)
			if pillButtonKey(c, "file-info-close", nil, tr("app.common.close"), p.hover.Over(p.bg), p.text).Clicked() {
				a.fileInfoOpen = false
			}
		})
	})
}

func fileInfoHighlights(entry *fileInfoEntry) []fileInfoPair {
	if info := entry.Media; info != nil {
		if info.Video != nil {
			return []fileInfoPair{
				{tr("app.common.duration"), formatMediaDuration(info.Duration)},
				{tr("app.common.resolution"), formatResolution(info.Video.Width, info.Video.Height)},
				{tr("app.common.file_size"), formatBytes(info.Size)},
			}
		}
		out := []fileInfoPair{
			{tr("app.common.duration"), formatMediaDuration(info.Duration)},
			{tr("app.common.file_size"), formatBytes(info.Size)},
		}
		if info.Bitrate > 0 {
			out = append(out, fileInfoPair{tr("app.file.total_bitrate"), formatBitrate(info.Bitrate)})
		} else if info.FormatName != "" {
			out = append(out, fileInfoPair{tr("app.common.format"), containerLabel(info.FormatName, info.Path)})
		}
		return out
	}
	if info := entry.Image; info != nil {
		return []fileInfoPair{
			{tr("app.common.size"), formatResolution(info.Width, info.Height)},
			{tr("app.common.file_size"), formatBytes(info.Size)},
			{tr("app.common.format"), strings.ToUpper(info.Format)},
		}
	}
	return nil
}

func fileInfoDetails(entry *fileInfoEntry) []fileInfoPair {
	var out []fileInfoPair
	if info := entry.Media; info != nil {
		if info.Video != nil {
			out = append(out,
				fileInfoPair{tr("app.common.format"), containerLabel(info.FormatName, info.Path)},
				fileInfoPair{tr("app.file.video_codec"), friendlyCodec(info.Video.Codec)},
			)
			if info.Video.FPS > 0 {
				out = append(out, fileInfoPair{tr("app.common.frame_rate"), formatFPS(info.Video.FPS)})
			}
			if info.Bitrate > 0 {
				out = append(out, fileInfoPair{tr("app.file.total_bitrate"), formatBitrate(info.Bitrate)})
			}
		} else {
			out = append(out, fileInfoPair{tr("app.common.format"), containerLabel(info.FormatName, info.Path)})
		}
		out = append(out, fileInfoPair{tr("app.common.audio"), audioSummary(info.Audio)})
		if info.Modified.IsZero() {
			out = append(out, fileInfoPair{tr("app.file.modified"), "—"})
		} else {
			out = append(out, fileInfoPair{tr("app.file.modified"), info.Modified.Format("2006-01-02 15:04")})
		}
		return append(out, fileInfoPair{tr("app.file.file_path"), info.Path})
	}
	if info := entry.Image; info != nil {
		if info.OpaqueKnown {
			transparent := tr("app.common.no")
			if !info.Opaque {
				transparent = tr("app.common.yes")
			}
			out = append(out, fileInfoPair{tr("app.common.transparent"), transparent})
		}
		if info.Colorspace != "" {
			out = append(out, fileInfoPair{tr("app.file.color_space"), friendlyColorspace(info.Colorspace)})
		}
		if info.Channels != "" {
			out = append(out, fileInfoPair{tr("app.file.color_channels"), colorChannelLabel(info.Channels)})
		}
		if info.Depth > 0 {
			out = append(out, fileInfoPair{tr("app.file.bit_depth"), fmt.Sprintf("%d %s", info.Depth, tr("app.common.bit"))})
		}
		if info.Type != "" {
			out = append(out, fileInfoPair{tr("app.file.image_type"), imageTypeLabel(info.Type)})
		}
		if dpi := imageDPI(info); dpi != "" {
			out = append(out, fileInfoPair{tr("app.file.print_resolution"), dpi})
		}
		if info.Frames > 1 {
			out = append(out, fileInfoPair{tr("app.file.frames"), fmt.Sprintf("%d %s", info.Frames, tr("app.common.frames"))})
		}
		if info.Quality > 0 {
			out = append(out, fileInfoPair{tr("app.common.quality"), strconv.Itoa(info.Quality)})
		}
		if info.Modified.IsZero() {
			out = append(out, fileInfoPair{tr("app.file.modified"), "—"})
		} else {
			out = append(out, fileInfoPair{tr("app.file.modified"), info.Modified.Format("2006-01-02 15:04")})
		}
		return append(out, fileInfoPair{tr("app.file.file_path"), info.Path})
	}
	// Even when identify/ffprobe fails, the dialog still gives useful
	// filesystem details instead of an empty panel.
	info, err := os.Stat(entry.Path)
	if err == nil {
		out = append(out,
			fileInfoPair{tr("app.common.file_size"), formatBytes(info.Size())},
			fileInfoPair{tr("app.file.modified"), info.ModTime().Format("2006-01-02 15:04")},
		)
	}
	return append(out, fileInfoPair{tr("app.file.file_path"), entry.Path})
}

func formatMediaDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	seconds := int(math.Round(d.Seconds()))
	if seconds < 60 {
		if d < time.Second {
			return fmt.Sprintf("%d %s", d.Milliseconds(), tr("app.common.ms"))
		}
		return trimDecimal(d.Seconds()) + " " + tr("app.common.s")
	}
	h, m, s := seconds/3600, seconds%3600/60, seconds%60
	if h > 0 {
		return fmt.Sprintf("%d %s %02d %s %02d %s", h, tr("app.file.h"), m, tr("app.common.min"), s, tr("app.common.s"))
	}
	return fmt.Sprintf("%d %s %02d %s", m, tr("app.common.min"), s, tr("app.common.s"))
}

func formatResolution(width, height int) string {
	if width <= 0 || height <= 0 {
		return "—"
	}
	if divisor := gcd(width, height); divisor > 1 {
		return fmt.Sprintf("%d × %d (%d:%d)", width, height, width/divisor, height/divisor)
	}
	return fmt.Sprintf("%d × %d", width, height)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func formatFPS(fps float64) string {
	if math.Abs(fps-math.Round(fps)) < 0.01 {
		return fmt.Sprintf("%.0f fps", fps)
	}
	return fmt.Sprintf("%.2f fps", fps)
}

func formatBitrate(bps int64) string {
	switch {
	case bps >= 1_000_000:
		return trimDecimal(float64(bps)/1_000_000) + " Mbps"
	case bps >= 1_000:
		return trimDecimal(float64(bps)/1_000) + " kbps"
	case bps > 0:
		return strconv.FormatInt(bps, 10) + " bps"
	default:
		return "—"
	}
}

func trimDecimal(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

func formatSampleRate(hz int) string {
	if hz <= 0 {
		return ""
	}
	if hz >= 1000 {
		return trimDecimal(float64(hz)/1000) + " kHz"
	}
	return strconv.Itoa(hz) + " Hz"
}

func audioSummary(info *ffmpeg.AudioInfo) string {
	if info == nil {
		return tr("app.file.no_audio_track")
	}
	parts := []string{friendlyCodec(info.Codec)}
	if layout := channelLabel(info.ChannelLayout, info.Channels); layout != "" {
		parts = append(parts, layout)
	}
	if rate := formatSampleRate(info.SampleRate); rate != "" {
		parts = append(parts, rate)
	}
	if info.Bitrate > 0 {
		parts = append(parts, formatBitrate(info.Bitrate))
	}
	return strings.Join(parts, " · ")
}

func channelLabel(layout string, channels int) string {
	switch strings.ToLower(strings.TrimSpace(layout)) {
	case "mono":
		return tr("app.file.mono")
	case "stereo":
		return tr("app.file.stereo")
	case "quad":
		return "4 " + tr("app.file.channels")
	case "5.1", "5.1(side)":
		return "5.1 " + tr("app.file.channels")
	case "6.1":
		return "6.1 " + tr("app.file.channels")
	case "7.1":
		return "7.1 " + tr("app.file.channels")
	}
	switch channels {
	case 1:
		return tr("app.file.mono")
	case 2:
		return tr("app.file.stereo")
	case 0:
		return ""
	default:
		return fmt.Sprintf("%d %s", channels, tr("app.file.channels"))
	}
}

func containerLabel(formatName, path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "mp4", "m4v":
		return "MP4"
	case "mov":
		return "MOV"
	case "mkv":
		return "Matroska"
	case "webm":
		return "WebM"
	case "avi":
		return "AVI"
	case "flv":
		return "FLV"
	case "wmv":
		return "WMV"
	case "ts", "m2ts", "mts":
		return "MPEG-TS"
	case "mpg", "mpeg":
		return "MPEG"
	case "mp3":
		return "MP3"
	case "m4a":
		return "M4A"
	case "aac":
		return "AAC"
	case "wav":
		return "WAV"
	case "flac":
		return "FLAC"
	case "ogg", "opus":
		return "Ogg"
	}
	if ext != "" {
		return strings.ToUpper(ext)
	}
	if name := strings.TrimSpace(formatName); name != "" {
		if i := strings.IndexByte(name, ','); i >= 0 {
			name = name[:i]
		}
		return strings.ToUpper(name)
	}
	return "—"
}

func friendlyCodec(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "h264":
		return "H.264"
	case "hevc", "h265":
		return "H.265 / HEVC"
	case "mpeg4":
		return "MPEG-4"
	case "vp8":
		return "VP8"
	case "vp9":
		return "VP9"
	case "av1":
		return "AV1"
	case "prores":
		return "ProRes"
	case "aac":
		return "AAC"
	case "mp3":
		return "MP3"
	case "opus":
		return "Opus"
	case "vorbis":
		return "Vorbis"
	case "flac":
		return "FLAC"
	case "alac":
		return "ALAC"
	case "ac3":
		return "AC-3"
	case "eac3":
		return "E-AC-3"
	case "dts":
		return "DTS"
	case "truehd":
		return "TrueHD"
	case "pcm_s16le":
		return "PCM 16-bit"
	case "pcm_s24le":
		return "PCM 24-bit"
	case "pcm_s32le":
		return "PCM 32-bit"
	case "pcm_f32le":
		return "PCM 32-bit float"
	}
	if strings.TrimSpace(name) == "" {
		return "—"
	}
	return strings.ToUpper(name)
}

func friendlyColorspace(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "srgb":
		return "sRGB"
	case "rgb":
		return "RGB"
	case "gray":
		return "Gray"
	case "cmyk":
		return "CMYK"
	case "lab":
		return "Lab"
	case "ycbcr":
		return "YCbCr"
	case "undefined":
		return tr("app.common.unknown")
	}
	return name
}

func colorChannelLabel(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "srgb", "rgb":
		return "RGB"
	case "srgba", "rgba":
		return "RGBA"
	case "gray", "grayscale":
		return "Gray"
	case "graya":
		return "Gray + Alpha"
	case "cmyk":
		return "CMYK"
	case "cmyka":
		return "CMYK + Alpha"
	case "undefined":
		return tr("app.common.unknown")
	}
	return strings.ToUpper(strings.TrimSpace(name))
}

func imageTypeLabel(name string) string {
	base := strings.TrimSpace(name)
	alpha := false
	switch strings.ToLower(base) {
	case "bilevel":
		base = tr("app.file.bilevel")
	case "grayscale":
		base = tr("app.file.grayscale")
	case "grayscalealpha", "grayscalematte":
		base, alpha = tr("app.file.grayscale"), true
	case "palette":
		base = tr("app.file.palette")
	case "palettealpha", "palettematte":
		base, alpha = tr("app.file.palette"), true
	case "truecolor":
		base = tr("app.file.truecolor")
	case "truecoloralpha", "truecolormatte":
		base, alpha = tr("app.file.truecolor"), true
	case "colorseparation":
		base = tr("app.file.color_separation")
	case "colorseparationalpha":
		base, alpha = tr("app.file.color_separation"), true
	case "optimize":
		base = tr("app.common.optimized")
	case "", "undefined":
		return tr("app.common.unknown")
	}
	if alpha {
		return base + " (" + tr("app.file.with_alpha") + ")"
	}
	return base
}

func imageDPI(info *magick.ImageInfo) string {
	if info.ResolutionX <= 0 || info.ResolutionY <= 0 {
		return ""
	}
	unit := ""
	switch info.Units {
	case "PixelsPerInch":
		unit = "DPI"
	case "PixelsPerCentimeter":
		unit = "DPCM"
	default:
		return ""
	}
	return fmt.Sprintf("%s × %s %s", trimDecimal(info.ResolutionX), trimDecimal(info.ResolutionY), unit)
}
