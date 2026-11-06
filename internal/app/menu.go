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
		{Label: "关于 zchat", Role: mygo.RoleAbout},
		{Label: "检查更新…", Click: run(checkForUpdates)},
		mygo.Separator(),
		{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: "服务"},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: "隐藏" + name},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: "全部显示"},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: "退出" + name})

	statusItems := make([]*mygo.MenuItem, 0, len(model.PresenceOptions))
	for _, o := range model.PresenceOptions {
		o := o
		statusItems = append(statusItems, &mygo.MenuItem{
			Label:   o.Label,
			Type:    mygo.MenuItemRadio,
			Checked: a.settings.Status == o.Value,
			Click:   run(func() { a.setStatus(o.Value) }),
		})
	}

	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "新建群聊…", Accelerator: "CmdOrCtrl+N", Click: run(func() {
				a.showMain()
				a.openNewGroup()
			})},
			{Label: "通过 IP 连接…", Accelerator: "CmdOrCtrl+Shift+N", Click: run(func() {
				a.showMain()
				a.manualOpen = true
			})},
			mygo.Separator(),
			{Label: "发送文件…", Accelerator: "CmdOrCtrl+O", Click: run(a.pickFiles)},
			{Label: "发送剪贴板图片", Click: run(a.sendClipboardImage)},
			mygo.Separator(),
			{Label: "文件共享…", Click: run(func() {
				a.showMain()
				a.setPage(pageShare)
			})},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: "撤销"},
			{Role: mygo.RoleRedo, Label: "重做"},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: "剪切"},
			{Role: mygo.RoleCopy, Label: "拷贝"},
			{Role: mygo.RolePaste, Label: "粘贴"},
			{Role: mygo.RoleSelectAll, Label: "全选"},
		}},
		{Label: "会话", Submenu: []*mygo.MenuItem{
			{Label: "搜索会话", Accelerator: "CmdOrCtrl+F", Click: run(func() {
				a.showMain()
				a.focusSearch = true
				a.redraw()
			})},
			{Label: "搜索聊天记录", Accelerator: "CmdOrCtrl+Shift+F", Click: run(func() {
				a.showMain()
				if a.currentConv() != nil {
					a.msgSearchShown = true
					a.focusMsgQuery = true
					a.redraw()
				}
			})},
			mygo.Separator(),
			{Label: "清空聊天记录", Click: run(func() {
				if c := a.currentConv(); c != nil {
					a.clearHistory(c.ID)
				}
			})},
			{Label: "删除当前会话", Click: run(func() {
				if c := a.currentConv(); c != nil {
					a.deleteConversation(c.ID)
				}
			})},
		}},
		{Label: "状态", Submenu: statusItems},
		{Label: "窗口", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: "最小化"},
			{Role: mygo.RoleZoom, Label: "缩放"},
			{Role: mygo.RoleClose, Label: "关闭"},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
			{Role: mygo.RoleFront, Label: "前置全部窗口"},
		}},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			{Label: "zchat 项目主页", Click: run(func() {
				a.openExternal("https://github.com/zwdart/zchat")
			})},
			{Label: "局域网聊天使用说明", Click: run(func() {
				a.showMain()
				a.setPage(pageDiscover)
			})},
		}},
	}))
}

func (a *app) openExternal(url string) {
	if err := mygo.Shell.OpenExternal(url); err != nil {
		a.setToast("无法打开链接：" + err.Error())
	}
}

func (a *app) pickFiles() {
	if a.currentConv() == nil {
		a.setToast("先打开一个聊天")
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "发送文件",
			Multiple:    true,
			ButtonLabel: "发送",
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.sendFiles(paths) })
	}()
}
