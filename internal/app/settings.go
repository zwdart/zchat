package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/model"
)

// settings are the choices kept in settings.json in the user data
// directory. The profile and chats live in the store beside it.
type settings struct {
	Appearance         string         `json:"appearance"`
	Status             model.Presence `json:"status"`
	Notifications      bool           `json:"notifications"`
	AutoDownloadImages bool           `json:"autoDownloadImages"`
	CloseToTray        bool           `json:"closeToTray"`
	Discovery          bool           `json:"discovery"`
	Port               int            `json:"port"`
	DownloadDir        string         `json:"downloadDir,omitempty"`
	ShareDir           string         `json:"shareDir,omitempty"`
	SharePort          int            `json:"sharePort"`
	ShareOnStart       bool           `json:"shareOnStart"`
	path               string
}

func defaultSettings() settings {
	downloads, _ := mygo.App.Path(mygo.PathDownloads)
	return settings{
		Appearance:         "system",
		Status:             model.Online,
		Notifications:      true,
		AutoDownloadImages: true,
		CloseToTray:        true,
		Discovery:          true,
		Port:               47824,
		DownloadDir:        filepath.Join(downloads, "zchat"),
		SharePort:          47830,
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
		s.Port = 47824
	}
	if s.SharePort == 0 {
		s.SharePort = 47830
	}
	if s.Status == "" {
		s.Status = model.Online
	}
	if s.DownloadDir == "" {
		s.DownloadDir = defaultSettings().DownloadDir
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
