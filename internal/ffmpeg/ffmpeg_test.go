package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testFiles(t *testing.T, n int) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	inputs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, "video", string(rune('a'+i))+".mp4")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, path)
	}
	return inputs, out
}

func baseOptions(t *testing.T, kind Kind) Options {
	t.Helper()
	inputs, out := testFiles(t, 1)
	return Options{
		Kind:          kind,
		Inputs:        inputs,
		OutputDir:     out,
		Format:        "mp4",
		CRF:           23,
		Preset:        "medium",
		AudioBitrate:  "128k",
		Start:         "00:00:00",
		FPS:           10,
		Width:         480,
		Position:      "topright",
		Margin:        20,
		FontSize:      32,
		FontColor:     "#ffffff",
		Opacity:       0.7,
		ImageWidth:    180,
		TransformMode: TransformResize,
		Height:        720,
		Rotation:      "90",
		Speed:         2,
		At:            "00:00:05",
		SubtitleMode:  SubtitleBurn,
		WatermarkMode: WatermarkText,
	}
}

func contains(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		ok := true
		for j := range want {
			if args[i+j] != want[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestBuildCompress(t *testing.T) {
	opt := baseOptions(t, KindCompress)
	opt.Resolution = "1080p"
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	step := plan.Steps[0]
	if !strings.HasSuffix(step.Out, "-compressed.mp4") {
		t.Fatalf("output = %s", step.Out)
	}
	if !contains(step.Args, "-crf", "23") || !contains(step.Args, "-preset", "medium") {
		t.Fatalf("encode args missing: %v", step.Args)
	}
	if !contains(step.Args, "-vf", "scale=-2:1080:flags=lanczos") {
		t.Fatalf("resolution filter missing: %v", step.Args)
	}
}

func TestBuildTrimUsesEndTime(t *testing.T) {
	opt := baseOptions(t, KindTrim)
	opt.Start = "10"
	opt.End = "40"
	opt.Copy = false
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if !contains(step.Args, "-ss", "10.000") || !contains(step.Args, "-t", "30.000") {
		t.Fatalf("trim args = %v", step.Args)
	}
	if step.Limit != 30 {
		t.Fatalf("limit = %v", step.Limit)
	}
}

func TestBuildMergeWritesConcatList(t *testing.T) {
	inputs, out := testFiles(t, 2)
	opt := baseOptions(t, KindMerge)
	opt.Inputs = inputs
	opt.OutputDir = out
	opt.Format = "keep"
	opt.Copy = true
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if len(step.Concat) != 2 {
		t.Fatalf("concat = %v", step.Concat)
	}
	if !contains(step.Args, "-f", "concat", "-safe", "0", "-i", "{list}") {
		t.Fatalf("concat args = %v", step.Args)
	}
	if !contains(step.Args, "-c", "copy") {
		t.Fatalf("copy missing: %v", step.Args)
	}
}

func TestBuildAudioMultipleInputs(t *testing.T) {
	inputs, out := testFiles(t, 2)
	opt := baseOptions(t, KindAudio)
	opt.Inputs = inputs
	opt.OutputDir = out
	opt.AudioFormat = "mp3"
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if !strings.HasSuffix(step.Out, ".mp3") {
			t.Fatalf("output = %s", step.Out)
		}
		if !contains(step.Args, "-vn", "-c:a", "libmp3lame") {
			t.Fatalf("audio args = %v", step.Args)
		}
	}
}

func TestBuildWatermarkImagePosition(t *testing.T) {
	opt := baseOptions(t, KindWatermark)
	opt.WatermarkMode = WatermarkImage
	opt.Image = filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(opt.Image, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	var filter string
	for i, arg := range step.Args {
		if arg == "-filter_complex" && i+1 < len(step.Args) {
			filter = step.Args[i+1]
		}
	}
	if !strings.Contains(filter, "overlay=W-w-20:20") {
		t.Fatalf("filter = %q", filter)
	}
	if !contains(step.Args, "-map", "[v]") || !contains(step.Args, "-map", "0:a?") {
		t.Fatalf("maps missing: %v", step.Args)
	}
}

func TestBuildCoverPattern(t *testing.T) {
	opt := baseOptions(t, KindCover)
	opt.Format = "jpg"
	opt.Interval = 10
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if !step.Pattern || !strings.Contains(step.Out, "%03d") {
		t.Fatalf("pattern = %q", step.Out)
	}
	if !contains(step.Args, "-vf", "fps=1/10") {
		t.Fatalf("cover args = %v", step.Args)
	}
}

func TestBuildBatch(t *testing.T) {
	inputs, out := testFiles(t, 3)
	opt := baseOptions(t, KindBatch)
	opt.Inputs = inputs
	opt.OutputDir = out
	opt.BatchMode = BatchCover
	opt.Format = "jpg"
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if !strings.HasSuffix(step.Out, "-cover.jpg") {
			t.Fatalf("output = %s", step.Out)
		}
	}
}

func TestUniqueOutputsForSameNames(t *testing.T) {
	dir := t.TempDir()
	inputs := []string{filepath.Join(dir, "a", "clip.mp4"), filepath.Join(dir, "b", "clip.mp4")}
	for _, path := range inputs {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opt := baseOptions(t, KindCompress)
	opt.Inputs = inputs
	opt.OutputDir = filepath.Join(dir, "out")
	plan, err := Build(Tool{Path: "ffmpeg"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Out == plan.Steps[1].Out {
		t.Fatalf("duplicate output %s", plan.Steps[0].Out)
	}
	if !strings.Contains(plan.Steps[1].Out, "-2.mp4") {
		t.Fatalf("second output = %s", plan.Steps[1].Out)
	}
}

func TestAtempoChain(t *testing.T) {
	if got := atempoChain(4); got != "atempo=2.0,atempo=2" {
		t.Fatalf("4x = %q", got)
	}
	if got := atempoChain(0.25); got != "atempo=0.5,atempo=0.5" {
		t.Fatalf("0.25x = %q", got)
	}
	if got := atempoChain(1.5); got != "atempo=1.5" {
		t.Fatalf("1.5x = %q", got)
	}
}

func TestValidateTrimNeedsDurationOrEnd(t *testing.T) {
	opt := baseOptions(t, KindTrim)
	opt.Start = "10"
	opt.Duration = ""
	opt.End = ""
	if err := Validate(opt); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseTime(t *testing.T) {
	cases := map[string]float64{
		"":          0,
		"30":        30,
		"01:30":     90,
		"00:01:30":  90,
		"1:02:03.5": 3723.5,
	}
	for input, want := range cases {
		got, ok := parseTime(input)
		if !ok || got != want {
			t.Fatalf("parseTime(%q) = %v, %v; want %v", input, got, ok, want)
		}
	}
	if _, ok := parseTime("a:b"); ok {
		t.Fatal("invalid time accepted")
	}
}

func TestEscapeDrawText(t *testing.T) {
	got := escapeDrawText("a:b'c,d%")
	if got != `a\:b\'c\,d\%` {
		t.Fatalf("escape = %q", got)
	}
}
