package magick

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xin.dart.zchat/internal/i18n"
)

// ImageInfo is the metadata the file-info dialog shows for an image.
type ImageInfo struct {
	Path        string
	Size        int64
	Modified    time.Time
	Format      string
	Width       int
	Height      int
	Depth       int
	Colorspace  string
	Channels    string
	Opaque      bool
	OpaqueKnown bool
	Frames      int
	Quality     int
	Type        string
	ResolutionX float64
	ResolutionY float64
	Units       string
}

const imageInfoFormat = "%m|%w|%h|%z|%[colorspace]|%[channels]|%x|%y|%U|%[opaque]|%n|%Q|%[type]"

// ImageInfo reads one image's dimensions and color details with identify.
// ImageMagick 7 is called as "magick identify"; ImageMagick 6 installs a
// separate identify program next to convert.
func (t Tool) ImageInfo(path string) (ImageInfo, error) {
	cmd, prefix, err := t.identifyCommand()
	if err != nil {
		return ImageInfo{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := append(append([]string{}, prefix...), "-format", imageInfoFormat, path)
	out, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	if err != nil {
		return ImageInfo{}, commandError("identify", out, err)
	}
	info, err := parseImageInfo(out)
	if err != nil {
		return ImageInfo{}, err
	}
	info.Path = path
	if st, statErr := os.Stat(path); statErr == nil {
		info.Modified = st.ModTime()
		info.Size = st.Size()
	}
	return info, nil
}

func (t Tool) identifyCommand() (string, []string, error) {
	base := strings.ToLower(filepath.Base(t.Path))
	if strings.HasPrefix(base, "magick") {
		return t.Path, []string{"identify"}, nil
	}
	for _, name := range []string{"identify", "identify.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil, nil
		}
	}
	return "", nil, errors.New(i18n.T("app.image.imagemagick_s_identify_command_was_not"))
}

func parseImageInfo(data []byte) (ImageInfo, error) {
	line := strings.TrimSpace(string(data))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	fields := strings.SplitN(strings.TrimSpace(line), "|", 13)
	if len(fields) < 10 {
		return ImageInfo{}, fmt.Errorf("identify: %s", i18n.T("app.image.could_not_read_image_details"))
	}
	info := ImageInfo{
		Format:      strings.TrimSpace(fields[0]),
		Width:       parseInt(fields[1]),
		Height:      parseInt(fields[2]),
		Depth:       parseInt(fields[3]),
		Colorspace:  strings.TrimSpace(fields[4]),
		Channels:    strings.TrimSpace(fields[5]),
		ResolutionX: parseFloat(fields[6]),
		ResolutionY: parseFloat(fields[7]),
		Units:       strings.TrimSpace(fields[8]),
		Frames:      1,
	}
	if len(fields) > 9 {
		quoted := strings.TrimSpace(fields[9])
		if quoted != "" {
			info.OpaqueKnown = true
			info.Opaque = strings.EqualFold(quoted, "true")
		}
	}
	if len(fields) > 10 {
		if n := parseInt(fields[10]); n > 0 {
			info.Frames = n
		}
	}
	if len(fields) > 11 {
		info.Quality = parseInt(fields[11])
	}
	if len(fields) > 12 {
		info.Type = strings.TrimSpace(fields[12])
	}
	if info.Width <= 0 || info.Height <= 0 {
		return ImageInfo{}, errors.New(i18n.T("app.image.could_not_read_image_details"))
	}
	return info, nil
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

func parseInt(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}
