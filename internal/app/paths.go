package app

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// appFolderName is the folder used for downloads and for the local data
// directory. It matches the app identifier in mygo.json.
const appFolderName = "xin.dart.zchat"

// setupDataDir points MyGo's user data path at the identifier folder and
// moves the old zchat folder there. It runs after the single-instance lock,
// so an older process cannot be writing to the folder while it is moved.
func setupDataDir() {
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	oldDir := filepath.Join(base, "zchat")
	newDir := filepath.Join(base, appFolderName)
	if oldDir != newDir {
		if err := migrateDir(oldDir, newDir); err != nil {
			// Keep the old location rather than starting with an empty
			// database when moving the data failed.
			log.Printf("zchat: cannot move data folder %s to %s: %v", oldDir, newDir, err)
			mygo.App.SetPath(mygo.PathUserData, oldDir)
			return
		}
	}
	mygo.App.SetPath(mygo.PathUserData, newDir)
}

// migrateDir moves oldDir to newDir. It first tries an atomic rename and
// falls back to copying into a temporary folder, so a partial copy is not
// mistaken for a completed migration on the next start.
func migrateDir(oldDir, newDir string) error {
	if oldDir == "" || newDir == "" || oldDir == newDir {
		return nil
	}
	info, err := os.Stat(oldDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	if entries, err := os.ReadDir(newDir); err == nil {
		if len(entries) > 0 {
			return nil // already migrated
		}
		_ = os.Remove(newDir)
	}
	if err := os.Rename(oldDir, newDir); err == nil {
		return nil
	}
	tmp := newDir + ".migrating"
	_ = os.RemoveAll(tmp)
	if err := copyDir(oldDir, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, newDir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// migrateDownloadDir renames the old default download folder to the new
// identifier folder when it is safe to do so. Unlike the data folder, it
// never copies large downloads on startup; if the rename fails, new files
// simply go to the new folder and the old files stay where they are.
func migrateDownloadDir(oldDir, newDir string) {
	if oldDir == "" || newDir == "" || oldDir == newDir {
		return
	}
	if _, err := os.Stat(newDir); err == nil {
		return
	}
	if info, err := os.Stat(oldDir); err != nil || !info.IsDir() {
		return
	}
	_ = os.Rename(oldDir, newDir)
}

// oldDownloadFolder reports whether dir is the old default download folder
// (a folder named zchat directly inside Downloads, including localized
// names). Custom folders are left alone.
func oldDownloadFolder(dir, downloads string) string {
	if dir == "" {
		return ""
	}
	if dir == filepath.Join(downloads, "zchat") {
		return dir
	}
	if filepath.Base(dir) != "zchat" {
		return ""
	}
	parent := filepath.Dir(dir)
	if parent == downloads {
		return dir
	}
	switch strings.ToLower(filepath.Base(parent)) {
	case "downloads", "下载":
		return dir
	}
	return ""
}

// normalizeDirForOS rejects a directory path that belongs to another OS.
// Settings copied between Linux, macOS, and Windows otherwise make the app
// try to create a path that cannot work on this machine.
func normalizeDirForOS(dir, goos string) (string, bool) {
	if dir == "" {
		return "", true
	}
	if goos == "windows" {
		// A UNC path (\\server\share or //server/share) is absolute.
		if strings.HasPrefix(dir, `\\`) || strings.HasPrefix(dir, "//") {
			return dir, true
		}
		// A drive path must include the separator after the colon. This
		// rejects POSIX paths such as /home/me/Downloads and drive-relative
		// paths such as C:Downloads, which cannot be created reliably.
		if len(dir) >= 3 && dir[1] == ':' && isASCIILetter(dir[0]) &&
			(dir[2] == '\\' || dir[2] == '/') {
			return dir, true
		}
		return "", false
	}
	// A Windows drive path copied from a Windows settings file.
	if len(dir) >= 2 && dir[1] == ':' && isASCIILetter(dir[0]) {
		return "", false
	}
	return dir, true
}

func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// safeFileName turns a name received from another device into one this OS
// can create. It also prevents a sender's path from escaping the download
// directory.
func safeFileName(name string) string {
	return safeFileNameFor(name, runtime.GOOS)
}

func safeFileNameFor(name, goos string) string {
	// Treat both separators as separators so a Windows path from the sender
	// cannot become part of a Linux filename.
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == ".." || name == "/" || name == "" {
		return "zchat-file"
	}
	if goos == "windows" {
		var b strings.Builder
		for _, r := range name {
			if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
		name = strings.TrimRight(b.String(), " .")
		if name == "" {
			return "zchat-file"
		}
		base := name
		if i := strings.IndexByte(base, '.'); i >= 0 {
			base = base[:i]
		}
		base = strings.TrimRight(base, " .")
		switch strings.ToUpper(base) {
		case "CON", "PRN", "AUX", "NUL",
			"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			name = "_" + name
		}
		name = trimNameRunes(name, 120)
	} else {
		// NUL is invalid on every platform; '/' was removed by path.Base.
		name = strings.ReplaceAll(name, "\x00", "_")
	}
	if name == "" || name == "." || name == ".." {
		return "zchat-file"
	}
	return name
}

// trimNameRunes keeps the extension while bounding the name so the complete
// Windows path stays below the classic MAX_PATH limit.
func trimNameRunes(name string, max int) string {
	runes := []rune(name)
	if len(runes) <= max {
		return name
	}
	ext := path.Ext(name)
	extRunes := []rune(ext)
	baseRunes := []rune(strings.TrimSuffix(name, ext))
	if len(extRunes) >= max {
		extRunes = extRunes[:max/2]
	}
	room := max - len(extRunes)
	if room < 1 {
		room = 1
	}
	if len(baseRunes) > room {
		baseRunes = baseRunes[:room]
	}
	return string(baseRunes) + string(extRunes)
}

func downloadDirCandidates(preferred string) []string {
	var out []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		d, ok := normalizeDirForOS(dir, runtime.GOOS)
		if !ok || d == "" {
			return
		}
		for _, x := range out {
			if x == d {
				return
			}
		}
		out = append(out, d)
	}
	add(preferred)
	add(defaultSettings().DownloadDir)
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "Downloads", appFolderName))
		add(filepath.Join(home, appFolderName))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		add(filepath.Join(dir, appFolderName, "downloads"))
	}
	add(filepath.Join(os.TempDir(), appFolderName))
	return out
}

// ensureDownloadDir returns a directory that exists and is writable. A path
// copied from another OS, a removed drive, or a read-only install directory
// falls back to the user's normal download folder.
func (a *app) ensureDownloadDir() (string, error) {
	var firstErr error
	for _, dir := range downloadDirCandidates(a.settings.DownloadDir) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if a.settings.DownloadDir != dir {
			a.settings.DownloadDir = dir
			_ = a.settings.save()
		}
		return dir, nil
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no download directory")
	}
	return "", firstErr
}
