package iconkit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunWithFakeTool exercises folder creation, static files, command
// invocation and progress reporting without requiring ImageMagick.
func TestRunWithFakeTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake tool")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "magick")
	script := `#!/bin/sh
for last; do :; done
out="$last"
out="${out#PNG24:}"
out="${out#PNG32:}"
out="${out#ICO:}"
out="${out#ICNS:}"
mkdir -p "$(dirname "$out")"
printf 'fake' > "$out"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	plan, err := Build(Tool{Path: fake}, Options{
		Input:      src,
		OutputRoot: out,
		Name:       "demo",
		Platforms:  []string{"windows", "linux"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var reports []Progress
	res := plan.Run(context.Background(), func(p Progress) { reports = append(reports, p) })
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if res.Files != len(plan.Steps) {
		t.Fatalf("files = %d, steps = %d", res.Files, len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if _, err := os.Stat(step.Out); err != nil {
			t.Errorf("missing output %s: %v", step.Out, err)
		}
	}
	if len(reports) == 0 || reports[len(reports)-1].Done != len(plan.Steps) {
		t.Fatalf("last progress = %+v, total = %d", reports[len(reports)-1], len(plan.Steps))
	}
	for i := 1; i < len(reports); i++ {
		if reports[i].Done < reports[i-1].Done {
			t.Fatalf("progress went backwards: %+v", reports)
		}
	}
	// The desktop file must have both placeholders substituted.
	desktop, err := os.ReadFile(filepath.Join(out, "linux", "demo.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(desktop), "{name}") || !strings.Contains(string(desktop), "demo") {
		t.Fatalf("desktop file still has placeholders: %s", desktop)
	}
}
