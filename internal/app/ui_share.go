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
	subtitle := tr("在局域网里用浏览器打开，就能下载共享目录里的文件")
	if a.settings.UploadDir != "" {
		subtitle = tr("在局域网里用浏览器打开，就能浏览共享目录并上传文件")
	}

	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(18).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(16).DragWindow().Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					sectionTitle(c, p, tr("文件共享"), subtitle)
				})
				if running {
					if pillButton(c, iconPause, tr("停止共享"), p.hover.Over(p.bg), p.text).Clicked() {
						a.stopShare()
					}
				} else {
					if pillButton(c, iconPlay, tr("开启共享"), p.accent, p.onAcc).Clicked() {
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
	title := tr("共享未开启")
	desc := tr("开启后，本机会在局域网提供文件网页，手机和其他电脑都能访问。")
	status := tr("已停止")
	if running {
		iconBG = p.accent.Alpha(0.13)
		iconFG = p.accent
		title = tr("共享中")
		desc = tr("共享目录通过本机 HTTP 服务提供，不经过任何服务器。")
		if a.shareUploadsActive() {
			desc = tr("共享目录和上传目录通过本机 HTTP 服务提供，不经过任何服务器。")
		}
		status = tr("运行中")
		if port := a.share.Port(); port > 0 {
			status += tr(" · 端口 ") + itoa(port)
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
				if pillButton(c, iconLink, tr("发送链接"), p.accent, p.onAcc).Clicked() {
					a.sendShareLink()
				}
			}
		} else {
			if pillButton(c, iconPlay, tr("开启"), p.accent, p.onAcc).Clicked() {
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
				ui.Text(c, tr("共享目录")).FontSize(12).FontWeight(600).TextColor(p.muted)
				if dir == "" {
					ui.Text(c, tr("还没有选择目录，附件仍然可以发送")).FontSize(11.5).TextColor(p.faint).SingleLine()
				} else {
					ui.Text(c, dir).FontSize(12).SingleLine()
				}
			})
			if dir != "" {
				if ui.Button(c.Key("open-share-dir"), tr("打开")).Clicked() {
					a.openPath(dir)
				}
				if ui.Button(c.Key("clear-share-dir"), tr("清除")).Clicked() {
					a.settings.ShareDir = ""
					_ = a.settings.save()
					if a.share != nil {
						a.share.SetDir("")
					}
					a.setToast(tr("已清除共享目录"))
					a.redraw()
				}
			}
			if ui.PrimaryButton(c.Key("choose-share-dir"), tr("选择目录…")).Clicked() {
				a.chooseShareDir()
			}
		})
		if running && dir != "" {
			access := tr("只读访问")
			hint := tr("打开网页即可浏览")
			if a.shareUploadsActive() {
				access = tr("浏览与上传")
				hint = tr("打开网页即可浏览或上传")
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
				ui.Text(c, tr("上传目录")).FontSize(12).FontWeight(600).TextColor(p.muted)
				if dir == "" {
					ui.Text(c, tr("还没有选择上传目录，访客只能浏览和下载")).FontSize(11.5).TextColor(p.faint).SingleLine()
				} else {
					ui.Text(c, dir).FontSize(12).SingleLine()
				}
			})
			if dir != "" {
				if ui.Button(c.Key("open-upload-dir"), tr("打开")).Clicked() {
					a.openPath(dir)
				}
				if ui.Button(c.Key("clear-upload-dir"), tr("清除")).Clicked() {
					a.settings.UploadDir = ""
					_ = a.settings.save()
					if a.share != nil && a.shareExplicit {
						a.share.SetUploadDir("")
					}
					a.setToast(tr("已清除上传目录"))
					a.redraw()
				}
			}
			if ui.PrimaryButton(c.Key("choose-upload-dir"), tr("选择目录…")).Clicked() {
				a.chooseUploadDir()
			}
		})
		ui.Row(c).Padding(14, 10).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.hair).Children(func() {
			ui.Text(c, tr("同一局域网内知道地址的访客都可以上传")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			status := tr("未开启上传")
			color := p.faint
			if dir != "" && a.shareUploadsActive() {
				status = tr("允许访客上传")
				color = p.accent
			} else if dir != "" {
				status = tr("开启共享后生效")
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
			ui.Text(c, tr("访问地址")).FontSize(13).FontWeight(600)
			ui.Spacer(c)
			ui.Text(c, tr("在同一局域网的浏览器打开")).FontSize(11).TextColor(p.faint)
		})
		if len(urls) == 0 {
			ui.Text(c, tr("没有找到可用的局域网地址。")).FontSize(12).TextColor(p.muted).Padding(14, 12)
			return
		}
		for _, url := range urls {
			url := url
			ui.Row(c).Padding(14, 10).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.hair).Children(func() {
				ui.Text(c, url).FontSize(12.5).FontFeatures("tnum").SingleLine().Grow(1).Shrink(1).MinWidth(0)
				if ui.Button(c.Key("open-"+url), tr("打开")).Clicked() {
					a.openExternal(url)
				}
				if ui.Button(c.Key("copy-"+url), tr("复制")).Clicked() {
					mygo.Clipboard.WriteText(url)
					a.setToast(tr("已复制链接"))
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
				ui.Text(c, tr("在手机上打开")).FontSize(13).FontWeight(650)
				ui.Text(c, tr("用相机或浏览器扫描二维码，就能浏览共享目录。")).FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
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
				ui.Text(c, tr("小提示")).FontSize(13).FontWeight(650)
				hint := tr("共享目录是只读的，访客不能上传或修改文件。\n\n聊天附件通过独立的 TCP 端口直连发送，不依赖这个共享目录。\n\n关闭 zchat 后共享链接会失效。")
				if a.shareUploadsActive() {
					hint = tr("共享目录只读；访客上传的文件会保存到「上传目录」，不会修改共享目录。\n\n聊天附件通过独立的 TCP 端口直连发送，不依赖这个共享目录。\n\n关闭 zchat 后共享链接会失效。")
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
		ui.Text(c, tr("如果无法访问，请检查系统防火墙是否允许 zchat 接收入站连接，并确认访问设备和本机在同一个网段。")).FontSize(11.5).TextColor(p.muted).LineHeight(1.45)
	})
}

func (a *app) chooseShareDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("选择共享目录"),
			Directory:   true,
			ButtonLabel: tr("共享"),
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
			a.setToast(tr("共享目录已更新"))
			a.redraw()
		})
	}()
}

func (a *app) chooseUploadDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("选择上传目录"),
			DefaultPath: a.settings.UploadDir,
			Directory:   true,
			ButtonLabel: tr("选择"),
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
			a.setToast(tr("上传目录已更新"))
			a.redraw()
		})
	}()
}

func (a *app) chooseDownloadDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("选择下载目录"),
			DefaultPath: a.settings.DownloadDir,
			Directory:   true,
			ButtonLabel: tr("选择"),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.DownloadDir = paths[0]
			_ = a.settings.save()
			a.setToast(tr("下载目录已更新"))
			a.redraw()
		})
	}()
}
