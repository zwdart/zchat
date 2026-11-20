package app

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// releasePageURL is where builds that cannot install updates themselves
// send people for a download.
func releasePageURL() string {
	return "https://github.com/zwdart/zchat/releases/latest"
}

func (a *app) openAbout() {
	a.aboutOpen = true
	a.redraw()
}

// aboutDialog shows the version, the project page and, for builds without
// self-updates, a copyable download URL.
func (a *app) aboutDialog(c *ui.Context) {
	if !a.aboutOpen {
		return
	}
	p := a.pal
	ui.DialogBase(c.Key("about"), &a.aboutOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 430)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(56, 56).Radius(16).Center().Background(p.accent.Alpha(0.12)).Children(func() {
				ui.Icon(c, iconChatFill).Size(30, 30).TextColor(p.accent)
			})
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "zchat").FontSize(20).FontWeight(750)
				ui.Text(c, tr("app.common.version")+" "+a.version).FontSize(12).TextColor(p.muted)
			})
		})
		ui.Text(c, tr("app.common.lan_chat_for_text_images_files")).
			FontSize(12.5).TextColor(p.muted).LineHeight(1.5)

		detail := func(label, value string, control func()) {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, label).FontSize(11.5).TextColor(p.faint)
					ui.Text(c, value).FontSize(12).TextColor(p.text).Selectable()
				})
				if control != nil {
					control()
				}
			})
		}
		dir := a.dataDir()
		if dir == "" {
			dir = "—"
		}
		ui.Column(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(10).Children(func() {
			detail(tr("app.common.platform"), runtime.GOOS+" / "+runtime.GOARCH, nil)
			detail(tr("app.common.project_page"), "github.com/zwdart/zchat", func() {
				if ui.Button(c.Key("about-home"), tr("app.common.open")).Clicked() {
					a.openExternal("https://github.com/zwdart/zchat")
				}
			})
			detail(tr("app.common.data_folder"), dir, func() {
				if ui.Button(c.Key("about-data"), tr("app.common.open")).Clicked() {
					if a.store != nil {
						a.openPath(a.store.Dir())
					}
				}
			})
		})

		if !a.updateSupported {
			ui.Column(c).FillWidth().Gap(7).Children(func() {
				ui.Text(c, tr("app.common.this_build_cannot_update_itself_download")).FontSize(11.5).TextColor(p.muted).LineHeight(1.45)
				ui.Text(c, releasePageURL()).FontSize(11.5).TextColor(p.text).Selectable()
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c.Key("about-copy-url"), tr("app.common.copy_link")).Clicked() {
						c.WriteClipboard(releasePageURL())
						a.setToast(tr("app.common.download_link_copied"))
					}
					if ui.Button(c.Key("about-release"), tr("app.common.open_release_page")).Clicked() {
						a.openExternal(releasePageURL())
					}
				})
			})
		}

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.updateSupported {
				if pillButtonKey(c, "about-update", iconRefresh, tr("app.common.check_for_updates"), p.accent, p.onAcc).Clicked() {
					a.aboutOpen = false
					checkForUpdates()
				}
			}
			ui.Spacer(c)
			if pillButtonKey(c, "about-close", nil, tr("app.common.close"), p.hover.Over(p.bg), p.text).Clicked() {
				a.aboutOpen = false
			}
		})
	})
}

// dataDir is the folder that holds settings, the database and caches, or ""
// when the store is not open yet.
func (a *app) dataDir() string {
	if a.store == nil {
		return ""
	}
	return a.store.Dir()
}
