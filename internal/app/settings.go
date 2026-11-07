package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/model"
)

// settings are the choices kept in settings.json in the user data
// directory. The profile and chats live in the store beside it.
type settings struct {
	Appearance         string         `json:"appearance"`
	Language           string         `json:"language,omitempty"`
	Status             model.Presence `json:"status"`
	Notifications      bool           `json:"notifications"`
	AutoDownloadImages bool           `json:"autoDownloadImages"`
	CloseToTray        bool           `json:"closeToTray"`
	Discovery          bool           `json:"discovery"`
	Port               int            `json:"port"`
	TransferPort       int            `json:"transferPort"`
	DownloadDir        string         `json:"downloadDir,omitempty"`
	ShareDir           string         `json:"shareDir,omitempty"`
	UploadDir          string         `json:"uploadDir,omitempty"`
	SharePort          int            `json:"sharePort"`
	ShareOnStart       bool           `json:"shareOnStart"`
	path               string
}

func defaultSettings() settings {
	downloads, _ := mygo.App.Path(mygo.PathDownloads)
	return settings{
		Appearance:         "system",
		Language:           "system",
		Status:             model.Online,
		Notifications:      true,
		AutoDownloadImages: true,
		CloseToTray:        true,
		Discovery:          true,
		Port:               42224,
		TransferPort:       42225,
		DownloadDir:        filepath.Join(downloads, appFolderName),
		SharePort:          42230,
		ShareOnStart:       false,
	}
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func loadSettings() settings {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &s)
		}
		s.path = path
	}
	if s.Port == 0 {
		s.Port = 42224
	}
	if s.SharePort == 0 {
		s.SharePort = 42230
	}
	if s.TransferPort == 0 {
		s.TransferPort = 42225
	}
	if s.Language == "" {
		s.Language = "system"
	}
	if s.Status == "" {
		s.Status = model.Online
	}
	if s.DownloadDir == "" {
		s.DownloadDir = defaultSettings().DownloadDir
	}
	// A settings file copied from another OS contains paths that cannot be
	// created here (for example /home/me/Downloads on Windows). Repair them
	// once so downloads do not fail with a "mkdir" error.
	changed := false
	// The default download folder used to be named "zchat". Move that one
	// folder to the app identifier and keep any custom download folder.
	if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
		if oldDir := oldDownloadFolder(s.DownloadDir, downloads); oldDir != "" {
			newDir := filepath.Join(filepath.Dir(oldDir), appFolderName)
			if oldDir != newDir {
				migrateDownloadDir(oldDir, newDir)
				s.DownloadDir = newDir
				changed = true
			}
		}
	}
	if d, ok := normalizeDirForOS(s.DownloadDir, runtime.GOOS); ok {
		s.DownloadDir = d
	} else {
		s.DownloadDir = defaultSettings().DownloadDir
		changed = true
	}
	if d, ok := normalizeDirForOS(s.ShareDir, runtime.GOOS); ok {
		s.ShareDir = d
	} else {
		s.ShareDir = ""
		changed = true
	}
	if d, ok := normalizeDirForOS(s.UploadDir, runtime.GOOS); ok {
		s.UploadDir = d
	} else {
		s.UploadDir = ""
		changed = true
	}
	if changed {
		_ = s.save()
	}
	return s
}

func (s settings) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s settings) applyAppearance() {
	switch s.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}
