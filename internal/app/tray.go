package app

import (
	"bytes"
	"image"
	"image/png"
	"log"

	"github.com/egoist/mygo"
	xdraw "golang.org/x/image/draw"

	"xin.dart.zchat/internal/model"
)

var (
	trayIconPNG    []byte
	trayIconDevPNG []byte
)

// SetTrayIcons supplies the app icons used in the tray. The root package
// embeds them from resources/, outside this package.
func SetTrayIcons(icon, dev []byte) {
	trayIconPNG, trayIconDevPNG = icon, dev
}

// trayIcon returns the app icon at the size tray areas expect. The
// production icon and the development icon are the only tray icons; there
// is no separate tray artwork.
func trayIcon() []byte {
	src := trayIconPNG
	if mygo.IsDev() && len(trayIconDevPNG) > 0 {
		src = trayIconDevPNG
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return src
	}
	return buf.Bytes()
}

// applyTray adds the icon in the menu bar or notification area.
func (a *app) applyTray() {
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "zchat",
		Icon:    trayIcon(),
		ToolTip: "zchat",
		Menu:    a.trayMenu(),
	})
	if err != nil {
		log.Println("zchat: tray:", err)
		return
	}
	t.OnClick(a.showMain)
	a.tray = t
}

func (a *app) rebuildTray() {
	a.applyTray()
	a.redraw()
}

func (a *app) trayMenu() *mygo.Menu {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.Focus()
			fn()
			a.redraw()
		}
	}
	statusItems := make([]*mygo.MenuItem, 0, len(model.PresenceOptions))
	for _, o := range model.PresenceOptions {
		o := o
		statusItems = append(statusItems, &mygo.MenuItem{
			Label:   tr(o.Label),
			Type:    mygo.MenuItemRadio,
			Checked: a.profile.Status == o.Value,
			Click:   run(func() { a.setStatus(o.Value) }),
		})
	}
	shots := []*mygo.MenuItem{}
	if a.settings.ScreenshotEnabled {
		shots = append(shots, &mygo.MenuItem{Label: tr("app.common.screenshot_2"), Click: run(a.openScreenshotChooser)})
	}
	shots = append(shots,
		&mygo.MenuItem{Label: tr("app.common.show_all_pins"), Click: run(a.showAllPins)},
		&mygo.MenuItem{Label: tr("app.common.restore_pin_interaction"), Click: run(func() { a.setPinsInteractive(true) })},
		&mygo.MenuItem{Label: tr("app.common.close_all_pins"), Click: run(a.closeAllPins)},
	)

	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: tr("app.common.open_zchat"), Click: run(a.showMain)},
		mygo.Separator(),
		{Label: tr("app.common.status"), Submenu: statusItems},
		mygo.Separator(),
		{Label: tr("app.common.new_group_2"), Click: run(func() {
			a.showMain()
			a.openNewGroup()
		})},
		{Label: tr("app.common.discover"), Click: run(func() {
			a.showMain()
			a.setPage(pageDiscover)
		})},
		{Label: tr("app.common.file_sharing"), Click: run(func() {
			a.showMain()
			a.setPage(pageShare)
		})},
		{Label: tr("app.common.screenshots_and_pins"), Submenu: shots},
		mygo.Separator(),
		{Label: tr("app.common.settings_2"), Click: run(a.openSettings)},
		{Label: tr("app.common.check_for_updates_2"), Click: run(checkForUpdates)},
		mygo.Separator(),
		{Label: tr("app.common.quit_zchat"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}
