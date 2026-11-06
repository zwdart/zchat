package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// The update plugin's check and preferences, which tests can swap.
var (
	checkForUpdates    = updater.CheckForUpdates
	updateChecks       = updater.AutomaticChecks
	setUpdateChecks    = updater.SetAutomaticChecks
	updateDownloads    = updater.AutomaticDownloads
	setUpdateDownloads = updater.SetAutomaticDownloads
	lastUpdateCheck    = updater.LastCheck
)

// useUpdater checks GitHub's releases for a new version soon after launch
// and then once a day, and offers it in an update window of native UI, in
// Chinese. It must run before App.Run.
func (a *app) useUpdater() {
	mygo.Use(native.New(updater.Options{Language: "zh-Hans"}))
	updater.OnChange(func() {
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}
