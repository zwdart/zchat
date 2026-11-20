package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

func (a *app) sharePage(c *ui.Context) {
	p := a.pal
	running := a.shareRunning()
	a.generateShareQR()
	subtitle := tr("app.share.open_it_in_browser_on_lan")
	if a.settings.UploadDir != "" {
		subtitle = tr("app.share.open_it_in_browser_on_lan_2")
	}

	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(18).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(16).DragWindow().Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					sectionTitle(c, p, tr("app.common.file_sharing"), subtitle)
				})
				if running {
					if pillButton(c, iconPause, tr("app.share.stop_sharing"), p.hover.Over(p.bg), p.text).Clicked() {
						a.stopShare()
					}
				} else {
					if pillButton(c, iconPlay, tr("app.share.start_sharing"), p.accent, p.onAcc).Clicked() {
						a.startShare(true)
					}
				}
			})

			a.shareStatusCard(c, running)

			ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
				a.shareDirCard(c, running)
				a.shareUploadDirCard(c)
				if running {
					a.shareAddresses(c)
					a.shareActions(c)
				}
				a.shareHint(c)
			})
		})
	})
}

func (a *app) shareStatusCard(c *ui.Context, running bool) {
	p := a.pal
	bg := p.panel
	iconBG := p.hover
	iconFG := p.muted
	title := tr("app.share.sharing_is_off")
	desc := tr("app.share.when_enabled_this_device_serves_file")
	status := tr("app.share.stopped")
	if running {
		iconBG = p.accent.Alpha(0.13)
		iconFG = p.accent
		title = tr("app.share.sharing")
		desc = tr("app.share.shared_folder_is_served_over_this")
		if a.shareUploadsActive() {
			desc = tr("app.share.shared_and_upload_folders_are_served")
		}
		status = tr("app.share.running")
		if port := a.share.Port(); port > 0 {
			status += tr("app.share.port") + itoa(port)
		}
	}
	ui.Row(c).FillWidth().Padding(16, 16).Radius(14).Background(bg).Border(1, p.hair).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(44, 44).Radius(13).Center().Background(iconBG).Children(func() {
			ui.Icon(c, iconFolder).Size(22, 22).TextColor(iconFG)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(14).FontWeight(650)
				dot(c, statusPresence(running), 7)
				ui.Text(c, status).FontSize(11).TextColor(p.muted)
			})
			ui.Text(c, desc).FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
		})
		if running {
			if a.shareURL() != "" {
				if pillButton(c, iconLink, tr("app.share.send_link"), p.accent, p.onAcc).Clicked() {
					a.sendShareLink()
				}
			}
		} else {
			if pillButton(c, iconPlay, tr("app.common.on"), p.accent, p.onAcc).Clicked() {
				a.startShare(true)
			}
		}
	})
}

func statusPresence(on bool) model.Presence {
	if on {
		return model.Online
	}
	return model.Offline
}

func (a *app) shareDirCard(c *ui.Context, running bool) {
	p := a.pal
	dir := a.settings.ShareDir
	ui.Column(c).Radius(14).Background(p.panel).Border(1, p.hair).Children(func() {
		ui.Row(c).Padding(14, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconFolder).Size(18, 18).TextColor(p.muted)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, tr("app.common.shared_folder")).FontSize(12).FontWeight(600).TextColor(p.muted)
				if dir == "" {
					ui.Text(c, tr("app.share.no_folder_selected_attachments_can_still")).FontSize(11.5).TextColor(p.faint).SingleLine()
				} else {
					ui.Text(c, dir).FontSize(12).SingleLine()
				}
			})
			if dir != "" {
				if ui.Button(c.Key("open-share-dir"), tr("app.common.open")).Clicked() {
					a.openPath(dir)
				}
				if ui.Button(c.Key("clear-share-dir"), tr("app.common.clear")).Clicked() {
					a.settings.ShareDir = ""
					_ = a.settings.save()
					if a.share != nil {
						a.share.SetDir("")
					}
					a.setToast(tr("app.share.shared_folder_cleared"))
					a.redraw()
				}
			}
			if ui.PrimaryButton(c.Key("choose-share-dir"), tr("app.share.choose_folder")).Clicked() {
				a.chooseShareDir()
			}
		})
		if running && dir != "" {
			access := tr("app.share.read_only_access")
			hint := tr("app.share.open_page_to_browse")
			if a.shareUploadsActive() {
				access = tr("app.share.browse_and_upload")
				hint = tr("app.share.open_page_to_browse_or_upload")
			}
			ui.Row(c).Padding(14, 10).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.hair).Children(func() {
				ui.Text(c, access).FontSize(11).TextColor(p.faint)
				ui.Spacer(c)
				ui.Text(c, hint).FontSize(11).TextColor(p.faint)
			})
		}
	})
}

func (a *app) shareUploadDirCard(c *ui.Context) {
	p := a.pal
	dir := a.settings.UploadDir
	ui.Column(c).Radius(14).Background(p.panel).Border(1, p.hair).Children(func() {
		ui.Row(c).Padding(14, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconUpload).Size(18, 18).TextColor(p.muted)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, tr("app.common.upload_folder")).FontSize(12).FontWeight(600).TextColor(p.muted)
				if dir == "" {
					ui.Text(c, tr("app.share.no_upload_folder_selected_visitors_can")).FontSize(11.5).TextColor(p.faint).SingleLine()
				} else {
					ui.Text(c, dir).FontSize(12).SingleLine()
				}
			})
			if dir != "" {
				if ui.Button(c.Key("open-upload-dir"), tr("app.common.open")).Clicked() {
					a.openPath(dir)
				}
				if ui.Button(c.Key("clear-upload-dir"), tr("app.common.clear")).Clicked() {
					a.settings.UploadDir = ""
					_ = a.settings.save()
					if a.share != nil && a.shareExplicit {
						a.share.SetUploadDir("")
					}
					a.setToast(tr("app.share.upload_folder_cleared"))
					a.redraw()
				}
			}
			if ui.PrimaryButton(c.Key("choose-upload-dir"), tr("app.share.choose_folder")).Clicked() {
				a.chooseUploadDir()
			}
		})
		ui.Row(c).Padding(14, 10).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.hair).Children(func() {
			ui.Text(c, tr("app.share.anyone_on_lan_with_address_can")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			status := tr("app.share.uploads_off")
			color := p.faint
			if dir != "" && a.shareUploadsActive() {
				status = tr("app.share.visitors_can_upload")
				color = p.accent
			} else if dir != "" {
				status = tr("app.share.takes_effect_when_sharing_starts")
			}
			ui.Text(c, status).FontSize(11).TextColor(color)
		})
	})
}

func (a *app) shareAddresses(c *ui.Context) {
	p := a.pal
	var urls []string
	if a.share != nil {
		urls = a.share.URLs()
	}
	ui.Column(c).Radius(14).Background(p.panel).Border(1, p.hair).Children(func() {
		ui.Row(c).Padding(14, 13).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconWifi).Size(17, 17).TextColor(p.muted)
			ui.Text(c, tr("app.share.access_address")).FontSize(13).FontWeight(600)
			ui.Spacer(c)
			ui.Text(c, tr("app.share.open_in_browser_on_same_lan")).FontSize(11).TextColor(p.faint)
		})
		if len(urls) == 0 {
			ui.Text(c, tr("app.share.no_usable_lan_address_found")).FontSize(12).TextColor(p.muted).Padding(14, 12)
			return
		}
		for _, url := range urls {
			url := url
			ui.Row(c).Padding(14, 10).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.hair).Children(func() {
				ui.Text(c, url).FontSize(12.5).FontFeatures("tnum").SingleLine().Grow(1).Shrink(1).MinWidth(0)
				if ui.Button(c.Key("open-"+url), tr("app.common.open")).Clicked() {
					a.openExternal(url)
				}
				if ui.Button(c.Key("copy-"+url), tr("app.common.copy")).Clicked() {
					mygo.Clipboard.WriteText(url)
					a.setToast(tr("app.share.link_copied"))
				}
			})
		}
	})
}

func (a *app) shareActions(c *ui.Context) {
	p := a.pal
	url := a.shareURL()
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Gap(10).Children(func() {
			ui.Column(c).Radius(14).Background(p.panel).Border(1, p.hair).Padding(16).Gap(8).Children(func() {
				ui.Text(c, tr("app.share.open_on_phone")).FontSize(13).FontWeight(650)
				ui.Text(c, tr("app.share.scan_qr_code_with_camera_or")).FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
				if a.shareQR != nil {
					ui.Box(c).Size(188, 188).Radius(12).Background(ui.RGB(255, 255, 255)).Padding(8).Children(func() {
						ui.Image(c, a.shareQR).Fill().Fit(ui.Contain)
					})
				}
				if url != "" {
					ui.Text(c, url).FontSize(10.5).TextColor(p.faint).SingleLine()
				}
			})
		})
		ui.Column(c).Width(260).Gap(10).Children(func() {
			ui.Column(c).Radius(14).Background(p.panel).Border(1, p.hair).Padding(16).Gap(7).Children(func() {
				ui.Text(c, tr("app.common.tip")).FontSize(13).FontWeight(650)
				hint := tr("app.share.shared_folder_is_read_only_visitors")
				if a.shareUploadsActive() {
					hint = tr("app.share.shared_folder_is_read_only_files")
				}
				ui.Text(c, hint).FontSize(11.5).
					TextColor(p.muted).LineHeight(1.55)
			})
		})
	})
}

func (a *app) shareHint(c *ui.Context) {
	p := a.pal
	ui.Row(c).Padding(14, 12).Radius(12).Background(p.accent.Alpha(0.07)).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconInfo).Size(15, 15).TextColor(p.accent)
		ui.Text(c, tr("app.share.if_you_cannot_connect_allow_zchat")).FontSize(11.5).TextColor(p.muted).LineHeight(1.45)
	})
}

func (a *app) chooseShareDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.share.choose_shared_folder"),
			Directory:   true,
			ButtonLabel: tr("app.common.share"),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.ShareDir = paths[0]
			_ = a.settings.save()
			if a.share != nil {
				a.share.SetDir(paths[0])
			}
			if a.shareRunning() {
				a.generateShareQR()
			}
			a.setToast(tr("app.share.shared_folder_updated"))
			a.redraw()
		})
	}()
}

func (a *app) chooseUploadDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.share.choose_upload_folder"),
			DefaultPath: a.settings.UploadDir,
			Directory:   true,
			ButtonLabel: tr("app.common.choose"),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.UploadDir = paths[0]
			_ = a.settings.save()
			if a.share != nil && a.shareExplicit {
				a.share.SetUploadDir(paths[0])
			}
			a.setToast(tr("app.share.upload_folder_updated"))
			a.redraw()
		})
	}()
}

func (a *app) chooseDownloadDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.share.choose_download_folder"),
			DefaultPath: a.settings.DownloadDir,
			Directory:   true,
			ButtonLabel: tr("app.common.choose"),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.DownloadDir = paths[0]
			_ = a.settings.save()
			a.setToast(tr("app.share.download_folder_updated"))
			a.redraw()
		})
	}()
}
