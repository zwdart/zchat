package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDirForOS(t *testing.T) {
	cases := []struct {
		dir  string
		goos string
		want string
		ok   bool
	}{
		{"/home/me/Downloads", "windows", "", false},
		{`C:\Users\me\Downloads`, "windows", `C:\Users\me\Downloads`, true},
		{"C:/Users/me/Downloads", "windows", "C:/Users/me/Downloads", true},
		{`\\server\share`, "windows", `\\server\share`, true},
		{"//server/share", "windows", "//server/share", true},
		{`\\?\C:\long\path`, "windows", `\\?\C:\long\path`, true},
		{"", "windows", "", true},
		{"/home/me/Downloads", "linux", "/home/me/Downloads", true},
		{`C:\Users\me\Downloads`, "linux", "", false},
		{"C:/Users/me/Downloads", "darwin", "", false},
		{"relative/path", "windows", "", false},
		{"C:Downloads", "windows", "", false},
		{"relative/path", "linux", "relative/path", true},
	}
	for _, c := range cases {
		got, ok := normalizeDirForOS(c.dir, c.goos)
		if got != c.want || ok != c.ok {
			t.Errorf("normalizeDirForOS(%q, %q) = (%q, %v), want (%q, %v)", c.dir, c.goos, got, ok, c.want, c.ok)
		}
	}
}

func TestSafeFileNameFor(t *testing.T) {
	cases := []struct {
		name string
		goos string
		want string
	}{
		{"../../etc/passwd", "linux", "passwd"},
		{`..\..\Windows\System32\evil.dll`, "windows", "evil.dll"},
		{`C:\Users\me\photo.jpg`, "linux", "photo.jpg"},
		{"a:b?.txt", "linux", "a:b?.txt"},
		{"a:b?.txt", "windows", "a_b_.txt"},
		{"CON.txt", "windows", "_CON.txt"},
		{"CON .txt", "windows", "_CON .txt"},
		{"con", "windows", "_con"},
		{"trailing. ", "windows", "trailing"},
		{"...", "windows", "zchat-file"},
		{"", "windows", "zchat-file"},
		{"..", "linux", "zchat-file"},
		{"/", "linux", "zchat-file"},
		{"photo.jpg", "windows", "photo.jpg"},
		{"a\x00b.txt", "linux", "a_b.txt"},
	}
	for _, c := range cases {
		if got := safeFileNameFor(c.name, c.goos); got != c.want {
			t.Errorf("safeFileNameFor(%q, %q) = %q, want %q", c.name, c.goos, got, c.want)
		}
	}
}

func TestSafeFileNameKeepsExtensionWhenTrimming(t *testing.T) {
	name := strings.Repeat("a", 200) + ".mp3"
	got := safeFileNameFor(name, "windows")
	if !strings.HasSuffix(got, ".mp3") {
		t.Fatalf("safeFileNameFor long name = %q, want .mp3 suffix", got)
	}
	if len([]rune(got)) > 120 {
		t.Fatalf("safeFileNameFor long name = %d runes, want <= 120", len([]rune(got)))
	}
}

func TestEnsureDownloadDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "nested", "zchat")
	a := &app{settings: settings{DownloadDir: dir}}
	got, err := a.ensureDownloadDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("ensureDownloadDir = %q, want %q", got, dir)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", got)
	}
}

func TestDownloadDirCandidatesPrefersConfigured(t *testing.T) {
	dir := filepath.Join(t.TempDir(), appFolderName)
	got := downloadDirCandidates(dir)
	if len(got) == 0 || got[0] != dir {
		t.Fatalf("downloadDirCandidates = %v, want %q first", got, dir)
	}
	seen := make(map[string]bool, len(got))
	for _, d := range got {
		if seen[d] {
			t.Fatalf("downloadDirCandidates contains duplicate %q: %v", d, got)
		}
		seen[d] = true
	}
}

func TestMigrateDir(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "zchat")
	newDir := filepath.Join(root, appFolderName)
	if err := os.MkdirAll(filepath.Join(oldDir, "cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "settings.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "cache", "image.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old data folder still exists: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(newDir, "cache", "image.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "png" {
		t.Fatalf("migrated file = %q, want png", got)
	}
}

func TestMigrateDirKeepsExistingTarget(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "zchat")
	newDir := filepath.Join(root, appFolderName)
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "old.txt")); err != nil {
		t.Fatalf("old folder should be left alone: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(newDir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target file = %q, want new", got)
	}
}

func TestMigrateDownloadDir(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "zchat")
	newDir := filepath.Join(root, appFolderName)
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "song.mp3"), []byte("mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateDownloadDir(oldDir, newDir)
	got, err := os.ReadFile(filepath.Join(newDir, "song.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "mp3" {
		t.Fatalf("migrated download = %q, want mp3", got)
	}
}

func TestOldDownloadFolder(t *testing.T) {
	cases := []struct {
		dir       string
		downloads string
		want      string
	}{
		{"/home/me/Downloads/zchat", "/home/me/Downloads", "/home/me/Downloads/zchat"},
		{"/home/me/下载/zchat", "/home/me/Downloads", "/home/me/下载/zchat"},
		{"/home/me/Downloads/custom", "/home/me/Downloads", ""},
		{"/home/me/zchat", "/home/me/Downloads", ""},
		{"/home/me/Downloads/zchat2", "/home/me/Downloads", ""},
	}
	for _, c := range cases {
		if got := oldDownloadFolder(c.dir, c.downloads); got != c.want {
			t.Errorf("oldDownloadFolder(%q, %q) = %q, want %q", c.dir, c.downloads, got, c.want)
		}
	}
}

func TestDefaultDownloadDirUsesAppFolder(t *testing.T) {
	if dir := defaultSettings().DownloadDir; !strings.HasSuffix(dir, appFolderName) {
		t.Fatalf("default download folder = %q, want suffix %q", dir, appFolderName)
	}
}
