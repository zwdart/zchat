package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"xin.dart.zchat/internal/i18n"
)

// MediaInfo is the subset of ffprobe metadata the file-info dialog shows.
// Codec and container names stay in their ffprobe form so the UI can decide
// how friendly to make them.
type MediaInfo struct {
	Path       string
	Size       int64
	Modified   time.Time
	Duration   time.Duration
	FormatName string
	FormatLong string
	Bitrate    int64
	Video      *VideoInfo
	Audio      *AudioInfo
}

type VideoInfo struct {
	Codec       string
	Width       int
	Height      int
	FPS         float64
	Bitrate     int64
	PixelFormat string
}

type AudioInfo struct {
	Codec         string
	Channels      int
	ChannelLayout string
	SampleRate    int
	Bitrate       int64
}

type probeDocument struct {
	Format struct {
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		Bitrate    string `json:"bit_rate"`
		FormatName string `json:"format_name"`
		FormatLong string `json:"format_long_name"`
	} `json:"format"`
	Streams []struct {
		CodecType     string `json:"codec_type"`
		CodecName     string `json:"codec_name"`
		Width         int    `json:"width"`
		Height        int    `json:"height"`
		AvgFrameRate  string `json:"avg_frame_rate"`
		RFrameRate    string `json:"r_frame_rate"`
		Duration      string `json:"duration"`
		Bitrate       string `json:"bit_rate"`
		Channels      int    `json:"channels"`
		ChannelLayout string `json:"channel_layout"`
		SampleRate    string `json:"sample_rate"`
		PixelFormat   string `json:"pix_fmt"`
		Disposition   struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

// MediaInfo reads a video or audio file with ffprobe. ffprobe is optional
// for processing but required for this detailed view.
func (t Tool) MediaInfo(path string) (MediaInfo, error) {
	if strings.TrimSpace(t.ProbePath) == "" {
		return MediaInfo{}, errors.New(i18n.T("app.ffmpeg.ffprobe_was_not_found_so_video"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, t.ProbePath,
		"-v", "error", "-show_format", "-show_streams", "-of", "json", path).CombinedOutput()
	if err != nil {
		return MediaInfo{}, commandError("ffprobe", out, err)
	}
	info, err := parseMediaInfo(out)
	if err != nil {
		return MediaInfo{}, err
	}
	info.Path = path
	if st, statErr := os.Stat(path); statErr == nil {
		info.Modified = st.ModTime()
		if info.Size == 0 {
			info.Size = st.Size()
		}
	}
	return info, nil
}

func parseMediaInfo(data []byte) (MediaInfo, error) {
	var doc probeDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe JSON: %w", err)
	}
	info := MediaInfo{
		Size:       parseInt64(doc.Format.Size),
		Bitrate:    parseInt64(doc.Format.Bitrate),
		FormatName: strings.TrimSpace(doc.Format.FormatName),
		FormatLong: strings.TrimSpace(doc.Format.FormatLong),
	}
	var videoDuration, audioDuration time.Duration
	for _, st := range doc.Streams {
		if st.CodecType != "video" || st.Width <= 0 || st.Height <= 0 || st.Disposition.AttachedPic != 0 {
			continue
		}
		fps := parseRate(st.AvgFrameRate)
		if fps <= 0 {
			fps = parseRate(st.RFrameRate)
		}
		info.Video = &VideoInfo{
			Codec:       st.CodecName,
			Width:       st.Width,
			Height:      st.Height,
			FPS:         fps,
			Bitrate:     parseInt64(st.Bitrate),
			PixelFormat: st.PixelFormat,
		}
		videoDuration = parseSeconds(st.Duration)
		break
	}
	for _, st := range doc.Streams {
		if st.CodecType != "audio" {
			continue
		}
		info.Audio = &AudioInfo{
			Codec:         st.CodecName,
			Channels:      st.Channels,
			ChannelLayout: st.ChannelLayout,
			SampleRate:    int(parseInt64(st.SampleRate)),
			Bitrate:       parseInt64(st.Bitrate),
		}
		audioDuration = parseSeconds(st.Duration)
		break
	}
	info.Duration = parseSeconds(doc.Format.Duration)
	if info.Duration <= 0 {
		info.Duration = videoDuration
	}
	if info.Duration <= 0 {
		info.Duration = audioDuration
	}
	return info, nil
}

func parseSeconds(s string) time.Duration {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0
	}
	return time.Duration(v * float64(time.Second))
}

func parseRate(s string) float64 {
	num, den, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return v
	}
	n, err1 := strconv.ParseFloat(strings.TrimSpace(num), 64)
	d, err2 := strconv.ParseFloat(strings.TrimSpace(den), 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

func parseInt64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func commandError(name string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	if msg == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %s", name, msg)
}
