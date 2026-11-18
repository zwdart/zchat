package magick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempImage(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("not really an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func testTool() Tool {
	return Tool{Path: "/usr/bin/magick", Version: "ImageMagick 7.1"}
}

func argsContain(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestValidate(t *testing.T) {
	in := tempImage(t, "a.png")
	dir := t.TempDir()
	if err := Validate(Options{Kind: KindConvert, Inputs: []string{in}, OutputDir: dir}); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	if err := Validate(Options{Kind: KindConvert, OutputDir: dir}); err == nil {
		t.Fatal("missing inputs accepted")
	}
	if err := Validate(Options{Kind: KindConvert, Inputs: []string{in}}); err == nil {
		t.Fatal("missing output folder accepted")
	}
	if err := Validate(Options{Kind: KindCompose, Inputs: []string{in}, OutputDir: dir}); err == nil {
		t.Fatal("compose with one image accepted")
	}
	if err := Validate(Options{Kind: KindGeometry, Inputs: []string{in}, OutputDir: dir, GeometryMode: GeometryCrop}); err == nil {
		t.Fatal("crop without size accepted")
	}
	if err := Validate(Options{Kind: KindGeometry, Inputs: []string{in}, OutputDir: dir, GeometryMode: GeometryResize, ResizeMode: ResizeFill, Width: 800}); err == nil {
		t.Fatal("fill without a height accepted")
	}
	if err := Validate(Options{Kind: KindWatermark, Inputs: []string{in}, OutputDir: dir, WatermarkMode: WatermarkText}); err == nil {
		t.Fatal("text watermark without text accepted")
	}
}

func TestBuildConvert(t *testing.T) {
	in := tempImage(t, "photo.png")
	dir := t.TempDir()
	plan, err := Build(testTool(), Options{
		Kind: KindConvert, Inputs: []string{in}, OutputDir: dir,
		Format: "webp", Quality: 82,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(plan.Steps))
	}
	step := plan.Steps[0]
	if want := filepath.Join(dir, "photo.webp"); step.Out != want {
		t.Fatalf("output %q, want %q", step.Out, want)
	}
	if !argsContain(step.Args, "-quality", "82") {
		t.Fatalf("quality missing: %v", step.Args)
	}
	if !argsContain(step.Args, "-auto-orient") {
		t.Fatalf("auto-orient missing: %v", step.Args)
	}
	if step.Args[len(step.Args)-1] != step.Out {
		t.Fatal("output path is not the last argument")
	}
}

func TestBuildJPGFlattensTransparency(t *testing.T) {
	in := tempImage(t, "logo.png")
	plan, err := Build(testTool(), Options{
		Kind: KindConvert, Inputs: []string{in}, OutputDir: t.TempDir(), Format: "jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argsContain(plan.Steps[0].Args, "-alpha", "remove", "-alpha", "off") {
		t.Fatalf("transparency is not flattened: %v", plan.Steps[0].Args)
	}
}

func TestBuildGeometry(t *testing.T) {
	in := tempImage(t, "a.jpg")
	plan, err := Build(testTool(), Options{
		Kind: KindGeometry, Inputs: []string{in}, OutputDir: t.TempDir(),
		GeometryMode: GeometryResize, ResizeMode: ResizeFill,
		Width: 800, Height: 600, Format: "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if !argsContain(step.Args, "-resize", "800x600^") {
		t.Fatalf("resize spec missing: %v", step.Args)
	}
	if !argsContain(step.Args, "-extent", "800x600") {
		t.Fatalf("extent missing: %v", step.Args)
	}
	if filepath.Ext(step.Out) != ".jpg" {
		t.Fatalf("keep format lost: %s", step.Out)
	}

	plan, err = Build(testTool(), Options{
		Kind: KindGeometry, Inputs: []string{in}, OutputDir: t.TempDir(),
		GeometryMode: GeometryCrop, Width: 200, Height: 100, Gravity: "north", Format: "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	step = plan.Steps[0]
	if !argsContain(step.Args, "-crop", "200x100+0+0", "+repage") {
		t.Fatalf("crop spec missing: %v", step.Args)
	}
	if !argsContain(step.Args, "-gravity", "north") {
		t.Fatalf("gravity missing: %v", step.Args)
	}
}

func TestBuildBatch(t *testing.T) {
	a := tempImage(t, "a.jpg")
	plan, err := Build(testTool(), Options{
		Kind: KindBatch, Inputs: []string{a}, OutputDir: t.TempDir(),
		Format: "jpg", Quality: 70, Width: 1600,
	})
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if !argsContain(step.Args, "-resize", "1600x>") {
		t.Fatalf("max side missing: %v", step.Args)
	}
	if !strings.HasSuffix(step.Out, "-batch.jpg") {
		t.Fatalf("batch suffix missing: %s", step.Out)
	}
}

func TestBuildWatermark(t *testing.T) {
	in := tempImage(t, "a.png")
	plan, err := Build(testTool(), Options{
		Kind: KindWatermark, Inputs: []string{in}, OutputDir: t.TempDir(),
		WatermarkMode: WatermarkText, WatermarkText: "zchat", FontSize: 30,
		Position: "southeast", Opacity: 0.5, Color: "#ffffff",
	})
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	if !argsContain(step.Args, "-annotate", "+0+0", "zchat") {
		t.Fatalf("annotate missing: %v", step.Args)
	}
	if !argsContain(step.Args, "-fill", "#ffffff80") {
		t.Fatalf("alpha color missing: %v", step.Args)
	}

	wm := tempImage(t, "mark.png")
	plan, err = Build(testTool(), Options{
		Kind: KindWatermark, Inputs: []string{in}, OutputDir: t.TempDir(),
		WatermarkMode: WatermarkImage, WatermarkImage: wm, FontSize: 120, Opacity: 0.4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argsContain(plan.Steps[0].Args, "-composite") {
		t.Fatalf("composite missing: %v", plan.Steps[0].Args)
	}
}

func TestBuildCompose(t *testing.T) {
	a := tempImage(t, "a.png")
	b := tempImage(t, "b.png")
	dir := t.TempDir()
	plan, err := Build(testTool(), Options{
		Kind: KindCompose, Inputs: []string{a, b}, OutputDir: dir, Layout: ComposeHorizontal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argsContain(plan.Steps[0].Args, "+append") {
		t.Fatalf("horizontal append missing: %v", plan.Steps[0].Args)
	}
	if want := filepath.Join(dir, "a-merged.png"); plan.Steps[0].Out != want {
		t.Fatalf("output %q, want %q", plan.Steps[0].Out, want)
	}

	plan, err = Build(testTool(), Options{
		Kind: KindCompose, Inputs: []string{a, b}, OutputDir: dir,
		Layout: ComposeAnimation, Delay: 30, Format: "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argsContain(plan.Steps[0].Args, "-loop", "0", "-layers", "Optimize") {
		t.Fatalf("animation options missing: %v", plan.Steps[0].Args)
	}
	if filepath.Ext(plan.Steps[0].Out) != ".gif" {
		t.Fatalf("animation output is not a gif: %s", plan.Steps[0].Out)
	}
}

func TestBuildOptimize(t *testing.T) {
	in := tempImage(t, "a.png")
	plan, err := Build(testTool(), Options{
		Kind: KindOptimize, Inputs: []string{in}, OutputDir: t.TempDir(),
		Quality: 85, AutoLevel: true, Strip: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argsContain(plan.Steps[0].Args, "-auto-level") {
		t.Fatalf("auto-level missing: %v", plan.Steps[0].Args)
	}
	if !strings.HasSuffix(plan.Steps[0].Out, "-optimized.png") {
		t.Fatalf("optimize suffix missing: %s", plan.Steps[0].Out)
	}
}

func TestTargetPathDoesNotOverwriteInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.png")
	out := targetPath(dir, in, "", "keep")
	if out == in {
		t.Fatal("output would overwrite the input")
	}
}

func TestWithAlpha(t *testing.T) {
	if got := withAlpha("#fff", 0.5); got != "#ffffff80" {
		t.Fatalf("got %q", got)
	}
	if got := withAlpha("red", 0.5); got != "red" {
		t.Fatalf("non-hex color changed: %q", got)
	}
}
