package iconkit

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xin.dart.zchat/internal/i18n"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"zchat":            "zchat",
		"My App":           "My-App",
		"  微信 / WeChat  ":  "微信-WeChat",
		"../../etc/passwd": "etc-passwd",
		"":                 "app",
		"你好 World":         "你好-World",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlatformsHaveDocsAndOutputs(t *testing.T) {
	if len(Platforms) != 5 {
		t.Fatalf("got %d platforms, want 5", len(Platforms))
	}
	for _, p := range Platforms {
		if p.Name == "" || p.Summary == "" {
			t.Errorf("%s: missing name or summary", p.Key)
		}
		if len(p.Docs) == 0 || len(p.Links) == 0 {
			t.Errorf("%s: requirements page is incomplete", p.Key)
		}
		if p.FileCount() == 0 {
			t.Errorf("%s: no files", p.Key)
		}
		for _, ic := range p.Icons {
			if ic.W <= 0 || ic.H <= 0 || ic.Rel == "" {
				t.Errorf("%s: bad icon %+v", p.Key, ic)
			}
		}
		for _, b := range p.Bundles {
			if b.Format != "ico" && b.Format != "icns" {
				t.Errorf("%s: bad bundle format %q", p.Key, b.Format)
			}
		}
	}
}

func TestBuildAllPlatforms(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(input, []byte("not a real png"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "out")
	plan, err := Build(Tool{Path: "magick", Version: "ImageMagick 7"}, Options{
		Input:      input,
		OutputRoot: root,
		Name:       "My App",
		Background: "#ffffff",
		Platforms:  AllKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, p := range Platforms {
		want += p.FileCount()
	}
	if len(plan.Steps) != want {
		t.Fatalf("got %d steps, want %d", len(plan.Steps), want)
	}
	var ico, icns bool
	for _, step := range plan.Steps {
		if !strings.HasPrefix(step.Out, root) {
			t.Errorf("output outside root: %s", step.Out)
		}
		if strings.Contains(step.Out, "{") {
			t.Errorf("unsubstituted placeholder: %s", step.Out)
		}
		switch {
		case strings.HasSuffix(step.Out, ".ico"):
			ico = true
			if !contains(step.Args, "icon:auto-resize=256,128,64,48,32,24,16") {
				t.Errorf("ico step lacks auto-resize: %v", step.Args)
			}
		case strings.HasSuffix(step.Out, ".icns"):
			icns = true
			if len(step.Sources) < 5 {
				t.Errorf("icns has too few sources: %+v", step.Sources)
			}
		}
		if step.Platform == "ios" && strings.HasSuffix(step.Out, ".png") {
			if !strings.Contains(step.Args[len(step.Args)-1], "PNG24:") {
				t.Errorf("iOS icon must be opaque: %v", step.Args)
			}
		}
	}
	if !ico || !icns {
		t.Fatalf("missing bundle outputs: ico=%v icns=%v", ico, icns)
	}
	// The adaptive foreground is 108px with a 1/6 inset, so the artwork is
	// resized to 72px before the transparent extent.
	var found bool
	for _, step := range plan.Steps {
		if strings.Contains(step.Out, "mipmap-mdpi/ic_launcher_foreground.png") {
			found = true
			if !contains(step.Args, "72x72") || !contains(step.Args, "108x108") {
				t.Errorf("adaptive resize/extent wrong: %v", step.Args)
			}
		}
	}
	if !found {
		t.Fatal("adaptive foreground step missing")
	}
}

func TestBuildValidation(t *testing.T) {
	dir := t.TempDir()
	if _, err := Build(Tool{}, Options{}); err == nil {
		t.Fatal("want error without ImageMagick")
	}
	if _, err := Build(Tool{Path: "magick"}, Options{Input: filepath.Join(dir, "missing.png"), OutputRoot: dir, Platforms: []string{"ios"}}); err == nil {
		t.Fatal("want error for missing source")
	}
	input := filepath.Join(dir, "logo.png")
	_ = os.WriteFile(input, []byte("x"), 0o644)
	if _, err := Build(Tool{Path: "magick"}, Options{Input: input, OutputRoot: dir, Platforms: nil}); err == nil {
		t.Fatal("want error without platforms")
	}
}

func TestPlatformMarkdown(t *testing.T) {
	i18n.Set(i18n.English)
	defer i18n.Set(i18n.Chinese)
	md := Platforms[0].Markdown()
	if !strings.Contains(md, "## Android") {
		t.Fatalf("missing heading:\n%s", md)
	}
	if !strings.Contains(md, "| Item | Size | Format | Note |") {
		t.Fatalf("missing localized table header:\n%s", md)
	}
	if !strings.Contains(md, "https://developer.android.com") {
		t.Fatalf("missing official link:\n%s", md)
	}
	all := AllMarkdown()
	if !strings.Contains(all, "# Installer icon requirements") {
		t.Fatalf("missing all-docs title:\n%s", all)
	}
}

func TestWriteICNS(t *testing.T) {
	dir := t.TempDir()
	var sources []Source
	for _, size := range []int{32, 64, 128, 256, 512, 1024} {
		path := filepath.Join(dir, "icon.png")
		if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, Source{Path: path, Size: size})
	}
	out := filepath.Join(dir, "AppIcon.icns")
	if err := writeICNS(out, sources); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[:4]) != "icns" {
		t.Fatalf("bad header: %q", data[:min(4, len(data))])
	}
	if got := binary.BigEndian.Uint32(data[4:8]); int(got) != len(data) {
		t.Fatalf("header length %d, file length %d", got, len(data))
	}
	if !bytes.Contains(data, []byte("ic10")) || !bytes.Contains(data, []byte("ic11")) {
		t.Fatal("missing modern PNG entries")
	}
}

func TestDetectMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Detect(); err == nil {
		t.Fatal("want error when ImageMagick is not on PATH")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
