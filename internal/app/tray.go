package app

import (
	_ "embed"
	"log"
	"runtime"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/model"
)

//go:embed tray/tray.png
var trayColor []byte

//go:embed tray/tray-template.png
var trayTemplate []byte

// applyTray adds the icon in the menu bar or notification area.
func (a *app) applyTray() {
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
	icon, template := trayColor, false
	if runtime.GOOS == "darwin" {
		icon, template = trayTemplate, true
	}
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "zchat",
		Icon:           icon,
		IconIsTemplate: template,
		ToolTip:        "zchat",
		Menu:           a.trayMenu(),
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
			Label:   o.Label,
			Type:    mygo.MenuItemRadio,
			Checked: a.profile.Status == o.Value,
			Click:   run(func() { a.setStatus(o.Value) }),
		})
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 zchat", Click: run(a.showMain)},
		mygo.Separator(),
		{Label: "状态", Submenu: statusItems},
		mygo.Separator(),
		{Label: "新建群聊…", Click: run(func() {
			a.showMain()
			a.openNewGroup()
		})},
		{Label: "发现设备", Click: run(func() {
			a.showMain()
			a.setPage(pageDiscover)
		})},
		{Label: "文件共享", Click: run(func() {
			a.showMain()
			a.setPage(pageShare)
		})},
		mygo.Separator(),
		{Label: "设置…", Click: run(a.openSettings)},
		{Label: "检查更新…", Click: run(checkForUpdates)},
		mygo.Separator(),
		{Label: "退出 zchat", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}
