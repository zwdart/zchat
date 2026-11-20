package app

import (
	"runtime"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/model"
)

// setMenu gives the app its menu bar. The statuses are check items so the
// menu shows which one is in use.
func (a *app) setMenu() {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	name := mygo.App.Name()
	appMenu := []*mygo.MenuItem{
		{Label: tr("app.common.about_zchat"), Role: mygo.RoleAbout},
		{Label: tr("app.common.check_for_updates_2"), Click: run(checkForUpdates)},
		mygo.Separator(),
		{Label: tr("app.common.settings_2"), Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: tr("app.common.services")},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: tr("app.common.hide") + name},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: tr("app.common.hide_others")},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: tr("app.common.show_all")},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: tr("app.common.quit") + name})

	statusItems := make([]*mygo.MenuItem, 0, len(model.PresenceOptions))
	for _, o := range model.PresenceOptions {
		o := o
		statusItems = append(statusItems, &mygo.MenuItem{
			Label:   tr(o.Label),
			Type:    mygo.MenuItemRadio,
			Checked: a.settings.Status == o.Value,
			Click:   run(func() { a.setStatus(o.Value) }),
		})
	}

	fileItems := []*mygo.MenuItem{
		{Label: tr("app.common.new_group_2"), Accelerator: "CmdOrCtrl+N", Click: run(func() {
			a.showMain()
			a.openNewGroup()
		})},
		{Label: tr("app.common.connect_by_ip_2"), Accelerator: "CmdOrCtrl+Shift+N", Click: run(func() {
			a.showMain()
			a.manualOpen = true
		})},
		mygo.Separator(),
	}
	if a.settings.ScreenshotEnabled {
		fileItems = append(fileItems, &mygo.MenuItem{Label: tr("app.common.screenshot_2"), Click: run(a.openScreenshotChooser)})
	}
	fileItems = append(fileItems,
		&mygo.MenuItem{Label: tr("app.common.send_file_2"), Accelerator: "CmdOrCtrl+O", Click: run(a.pickFiles)},
		&mygo.MenuItem{Label: tr("app.common.send_clipboard_image"), Click: run(a.sendClipboardImage)},
		mygo.Separator(),
		&mygo.MenuItem{Label: tr("app.common.file_sharing_2"), Click: run(func() {
			a.showMain()
			a.setPage(pageShare)
		})},
		&mygo.MenuItem{Label: tr("app.common.imagemagick_tools"), Click: run(func() {
			a.showMain()
			a.setPage(pageMagick)
		})},
		&mygo.MenuItem{Label: tr("app.common.ffmpeg_tools"), Click: run(func() {
			a.showMain()
			a.setPage(pageFFmpeg)
		})},
	)

	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: tr("app.common.files"), Submenu: fileItems},
		{Label: tr("app.common.edit"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: tr("app.common.undo")},
			{Role: mygo.RoleRedo, Label: tr("app.common.redo")},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: tr("app.common.cut")},
			{Role: mygo.RoleCopy, Label: tr("app.common.copy_2")},
			{Role: mygo.RolePaste, Label: tr("app.common.paste")},
			{Role: mygo.RoleSelectAll, Label: tr("app.common.select_all")},
		}},
		{Label: tr("app.common.chats"), Submenu: []*mygo.MenuItem{
			{Label: tr("app.common.search_chats"), Accelerator: "CmdOrCtrl+F", Click: run(func() {
				a.showMain()
				a.focusSearch = true
				a.redraw()
			})},
			{Label: tr("app.common.search_chat_history"), Accelerator: "CmdOrCtrl+Shift+F", Click: run(func() {
				a.showMain()
				if a.currentConv() != nil {
					a.msgSearchShown = true
					a.focusMsgQuery = true
					a.redraw()
				}
			})},
			mygo.Separator(),
			{Label: tr("app.common.clear_chat_history"), Click: run(func() {
				if c := a.currentConv(); c != nil {
					a.clearHistory(c.ID)
				}
			})},
			{Label: tr("app.common.delete_current_chat"), Click: run(func() {
				if c := a.currentConv(); c != nil {
					a.deleteConversation(c.ID)
				}
			})},
		}},
		{Label: tr("app.common.status"), Submenu: statusItems},
		{Label: tr("app.common.window"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: tr("app.common.minimize")},
			{Role: mygo.RoleZoom, Label: tr("app.common.zoom")},
			{Role: mygo.RoleClose, Label: tr("app.common.close")},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
			{Role: mygo.RoleFront, Label: tr("app.common.bring_all_to_front")},
		}},
		{Label: tr("app.common.help"), Submenu: []*mygo.MenuItem{
			{Label: tr("app.common.zchat_project_page"), Click: run(func() {
				a.openExternal("https://github.com/zwdart/zchat")
			})},
			{Label: tr("app.common.lan_chat_guide"), Click: run(func() {
				a.showMain()
				a.setPage(pageDiscover)
			})},
		}},
	}))
}

func (a *app) openExternal(url string) {
	if err := mygo.Shell.OpenExternal(url); err != nil {
		a.setToast(tr("app.common.could_not_open_link") + err.Error())
	}
}

func (a *app) pickFiles() {
	if a.currentConv() == nil {
		a.setToast(tr("app.common.open_chat_first"))
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.common.send_file"),
			Multiple:    true,
			ButtonLabel: tr("app.common.send"),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.sendFiles(paths) })
	}()
}
