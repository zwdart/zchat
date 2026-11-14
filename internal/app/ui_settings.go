package app

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/netutil"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.refreshDBStats()
	a.profileName = a.profile.Name
	a.profileSig = a.profile.Signature
	a.portEdit = float64(a.settings.Port)
	a.transferEdit = float64(a.settings.TransferPort)
	a.shareEdit = float64(a.settings.SharePort)
	a.shotIncludeEdit = a.settings.ScreenshotIncludeShortcut
	a.shotExcludeEdit = a.settings.ScreenshotExcludeShortcut
	a.updateSupported = mygo.Updater.Enabled()
	if a.settingsNav == "" {
		a.settingsNav = "profile"
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           tr("zchat 设置"),
		Width:           720,
		Height:          760,
		MinWidth:        620,
		MinHeight:       460,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f3f3f7, #1b1b1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
	})
}

// settingsSections are the menu items of the settings window, in page order.
var settingsSections = []struct {
	Key   string
	Label string
	Icon  *ui.SVG
}{
	{"profile", "个人资料", iconUsers},
	{"appearance", "外观", iconImage},
	{"notifications", "通知", iconBell},
	{"network", "网络", iconWifi},
	{"files", "文件", iconFolder},
	{"screenshot", "截图", iconScreenshot},
	{"history", "聊天记录", iconClock},
	{"update", "更新", iconRefresh},
	{"about", "关于", iconInfo},
}

// settingsMenu is the left menu of the settings window: a click scrolls the
// page to that section.
func (a *app) settingsMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(164).Padding(16, 12).Gap(2).
		BorderWidth(0, 1, 0, 0).BorderColor(p.hair).Children(func() {
		for _, sec := range settingsSections {
			sec := sec
			active := a.settingsNav == sec.Key
			b := ui.ButtonBase(c).Key("settings-nav-" + sec.Key).Height(32).PaddingX(9).Gap(9).Radius(8).
				AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(tr(sec.Label))
			bg := ui.Transparent
			switch {
			case active:
				bg = p.press
			case b.Hovered():
				bg = p.hover
			}
			b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
			color := p.text
			if active {
				color = p.accent
			}
			b.Children(func() {
				ui.Icon(c, sec.Icon).Size(15, 15).TextColor(color)
				ui.Text(c, tr(sec.Label)).FontSize(12.5).FontWeight(500).TextColor(color)
			})
			if b.Clicked() {
				a.settingsNav = sec.Key
				a.settingsSection = sec.Key
				a.redraw()
			}
		}
	})
}

// releaseLinkRow shows the releases page and copies its URL, for builds that
// cannot install updates themselves.
func (a *app) releaseLinkRow(c *ui.Context, p palette, key string) {
	url := releasePageURL()
	ui.Column(c).FillWidth().Padding(12, 15).Gap(8).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, tr("下载链接")).FontSize(13.5).FontWeight(500)
			ui.Text(c, tr("可以复制到浏览器，或直接打开发布页面。")).FontSize(11.5).TextColor(p.muted)
		})
		ui.Text(c, url).FontSize(11.5).TextColor(p.text).Selectable()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key(key+"-copy-release"), tr("复制链接")).Clicked() {
				c.WriteClipboard(url)
				a.setToast(tr("已复制下载链接"))
			}
			if ui.Button(c.Key(key+"-open-release"), tr("打开发布页面")).Clicked() {
				a.openExternal(url)
			}
		})
	})
}

func (a *app) settingsChanged() {
	_ = a.settings.save()
	a.settings.applyAppearance()
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAcc
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.settingsWin != nil {
			a.settingsWin.Close()
		}
	})
	s := &a.settings
	groups := make(map[string]ui.Element, len(settingsSections))

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.settingsMenu(c, p)
		ui.Scroll(c).Grow(1).Padding(24, 26, 30, 26).Gap(6).Children(func() {
			ui.Text(c, tr("设置")).FontSize(26).FontWeight(750).LetterSpacing(-0.4).Margin(0, 0, 6, 2)
			ui.Text(c, tr("这些设置只影响这台设备。")).FontSize(12).TextColor(p.muted).Margin(0, 0, 0, 8)

			group := func(title string, rows func()) ui.Element {
				ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(p.muted).Margin(14, 4, 4, 4)
				return ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hair).Dividers(1, p.hair).Children(rows)
			}
			row := func(label, desc string, control func()) {
				ui.Row(c).Padding(12, 15).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, label).FontSize(13.5).FontWeight(500)
						if desc != "" {
							ui.Text(c, desc).FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
						}
					})
					control()
				})
			}

			groups["profile"] = group(tr("个人资料"), func() {
				row(tr("名称"), tr("其他设备在局域网里看到的名字"), func() {
					ui.TextInput(c.Key("profile-name"), &a.profileName).Label(tr("名称")).Width(210).
						Placeholder(tr("我的 zchat")).OnSubmit(a.applyProfile)
				})
				row(tr("签名"), tr("一句介绍，显示在设备卡片里"), func() {
					ui.TextInput(c.Key("profile-signature"), &a.profileSig).Label(tr("签名")).Width(210).
						Placeholder(tr("可选")).OnSubmit(a.applyProfile)
				})
				ui.Row(c).Padding(12, 15).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Avatar(c, a.profileName, a.avatarBitmap(a.profile.Avatar)).Size(46, 46)
					ui.Column(c).Grow(1).Gap(3).Children(func() {
						ui.Text(c, tr("头像")).FontSize(13.5).FontWeight(500)
						ui.Text(c, tr("选择一张图片，会缩放到小尺寸后保存在本机")).FontSize(11.5).TextColor(p.muted)
					})
					if a.profile.Avatar != "" {
						if ui.Button(c.Key("remove-avatar"), tr("移除")).Clicked() {
							a.profile.Avatar = ""
							a.applyProfile()
						}
					}
					if ui.Button(c.Key("pick-avatar"), tr("选择…")).Clicked() {
						a.pickAvatar()
					}
					if ui.PrimaryButton(c.Key("save-profile"), tr("保存资料")).Clicked() {
						a.applyProfile()
					}
				})
				row(tr("状态"), tr("在线状态会广播给已连接的设备"), func() {
					labels := make([]string, len(model.PresenceOptions))
					chosen := presenceLabel(a.profile.Status)
					for i, o := range model.PresenceOptions {
						labels[i] = tr(o.Label)
						if o.Value == a.profile.Status {
							chosen = tr(o.Label)
						}
					}
					if ui.Select(c.Key("status"), &chosen, labels).Label(tr("状态")).Width(140).Changed() {
						for _, o := range model.PresenceOptions {
							if tr(o.Label) == chosen {
								a.setStatus(o.Value)
							}
						}
					}
				})
			})

			groups["appearance"] = group(tr("外观"), func() {
				row(tr("语言"), tr("界面语言，切换后立即生效"), func() {
					opts := i18n.Options()
					labels := make([]string, len(opts))
					chosen := ""
					for i, o := range opts {
						labels[i] = tr(o.Label)
						if o.Value == s.Language {
							chosen = labels[i]
						}
					}
					if chosen == "" {
						chosen = labels[0]
					}
					if ui.Select(c.Key("language"), &chosen, labels).Label(tr("语言")).Width(160).Changed() {
						for i, label := range labels {
							if label == chosen {
								s.Language = opts[i].Value
							}
						}
						a.settingsChanged()
						a.applyLanguage()
					}
				})
				row(tr("主题"), tr("跟随系统，或固定浅色、深色"), func() {
					labels := []string{tr("跟随系统"), tr("浅色"), tr("深色")}
					values := []string{"system", "light", "dark"}
					chosen := labels[0]
					for i, v := range values {
						if v == s.Appearance {
							chosen = labels[i]
						}
					}
					if ui.Select(c.Key("appearance"), &chosen, labels).Label(tr("主题")).Width(140).Changed() {
						for i, label := range labels {
							if label == chosen {
								s.Appearance = values[i]
							}
						}
						a.settingsChanged()
					}
				})
			})

			groups["notifications"] = group(tr("通知"), func() {
				row(tr("新消息通知"), tr("窗口不在前台时显示系统通知"), func() {
					if ui.Switch(c.Key("notifications"), &s.Notifications).Label(tr("新消息通知")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("自动下载图片"), tr("收到图片后自动缓存到本机，打开聊天即可查看"), func() {
					if ui.Switch(c.Key("auto-images"), &s.AutoDownloadImages).Label(tr("自动下载图片")).Changed() {
						a.settingsChanged()
						if s.AutoDownloadImages {
							a.queueActiveImages()
						}
					}
				})
				row(tr("关闭窗口时留在托盘"), tr("关闭窗口后继续接收消息，可从菜单栏或通知区重新打开"), func() {
					if ui.Switch(c.Key("close-tray"), &s.CloseToTray).Label(tr("关闭窗口时留在托盘")).Changed() {
						a.settingsChanged()
					}
				})
			})

			groups["network"] = group(tr("网络"), func() {
				row(tr("自动发现设备"), tr("通过 UDP 组播和广播在局域网里寻找其他 zchat"), func() {
					if ui.Switch(c.Key("discovery"), &s.Discovery).Label(tr("自动发现设备")).Changed() {
						a.settingsChanged()
						a.restartNetwork()
					}
				})
				row(tr("监听端口"), tr("TCP 端口，默认 42224；修改后重启网络生效"), func() {
					if ui.NumberInput(c.Key("tcp-port"), &a.portEdit, 1024, 65535, 1).Label(tr("监听端口")).Width(110).Changed() {
						s.Port = int(a.portEdit)
						_ = s.save()
					}
				})
				row(tr("文件传输端口"), tr("聊天附件通过这个 TCP 端口直连传输，默认 42225"), func() {
					if ui.NumberInput(c.Key("transfer-port"), &a.transferEdit, 1024, 65535, 1).Label(tr("文件传输端口")).Width(110).Changed() {
						s.TransferPort = int(a.transferEdit)
						_ = s.save()
					}
				})
				row(tr("本机地址"), tr("把这些地址告诉无法自动发现的设备"), func() {
					ip := netutil.PrimaryIP()
					if ip == "" {
						ip = tr("未连接局域网")
					}
					ui.Text(c, ip+":"+itoa(a.nodePort())).FontSize(12).TextColor(p.muted).FontFeatures("tnum")
				})
				row(tr("重新启动网络"), tr("让端口、文件传输或发现设置立即生效"), func() {
					if ui.Button(c.Key("restart-network"), tr("重启网络")).Clicked() {
						s.Port = int(a.portEdit)
						s.TransferPort = int(a.transferEdit)
						_ = s.save()
						a.restartNetwork()
						a.setToast(tr("网络已重启"))
					}
				})
			})

			groups["files"] = group(tr("文件"), func() {
				row(tr("下载目录"), a.settings.DownloadDir, func() {
					if ui.Button(c.Key("choose-downloads"), tr("选择…")).Clicked() {
						a.chooseDownloadDir()
					}
				})
				row(tr("共享目录"), tr("在「文件共享」页面开启后，访客可以浏览这个目录"), func() {
					if ui.Button(c.Key("open-share-page"), tr("打开共享")).Clicked() {
						a.showMain()
						a.setPage(pageShare)
					}
				})
				row(tr("上传目录"), tr("在「文件共享」页面开启后，访客可以通过网页上传文件到这个目录"), func() {
					if ui.Button(c.Key("open-upload-page"), tr("打开共享")).Clicked() {
						a.showMain()
						a.setPage(pageShare)
					}
				})
				row(tr("共享端口"), tr("HTTP 服务端口，默认 42230"), func() {
					if ui.NumberInput(c.Key("share-port"), &a.shareEdit, 1024, 65535, 1).Label(tr("共享端口")).Width(110).Changed() {
						s.SharePort = int(a.shareEdit)
						_ = s.save()
						// Do not turn the attachment fallback server into an
						// explicit share (and thereby enable uploads) just
						// because the port changed.
						if a.shareExplicit {
							a.startShare(false)
						}
					}
				})
				row(tr("启动时自动共享"), tr("打开 zchat 后自动开启上一个共享目录"), func() {
					if ui.Switch(c.Key("share-start"), &s.ShareOnStart).Label(tr("启动时自动共享")).Changed() {
						a.settingsChanged()
					}
				})
			})

			groups["screenshot"] = group(tr("截图"), func() {
				row(tr("启用截图"), tr("在聊天工具栏、菜单和快捷键中使用截图"), func() {
					if ui.Switch(c.Key("shot-enabled"), &s.ScreenshotEnabled).Label(tr("启用截图")).Changed() {
						a.settingsChanged()
						a.applyScreenshotShortcut()
						a.setMenu()
						a.rebuildTray()
					}
				})
				row(tr("默认包含聊天页面"), tr("点击聊天工具栏的截图按钮时，默认把当前聊天页面一起截入"), func() {
					if ui.Switch(c.Key("shot-include-chat"), &s.ScreenshotIncludeChat).Label(tr("默认包含聊天页面")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("包含聊天窗口的快捷键"), tr("连当前聊天窗口一起截取；留空或填 off 可关闭"), func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c.Key("shot-include-shortcut"), &a.shotIncludeEdit).Width(170).
							Placeholder("Ctrl+F1")
						if ui.Button(c.Key("shot-include-apply"), tr("应用")).Clicked() {
							if a.setScreenshotShortcut(a.shotIncludeEdit, true) {
								a.setToast(tr("截图快捷键已更新"))
							}
						}
					})
				})
				row(tr("不包含聊天窗口的快捷键"), tr("先隐藏 zchat 再截取；留空或填 off 可关闭"), func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c.Key("shot-exclude-shortcut"), &a.shotExcludeEdit).Width(170).
							Placeholder("Ctrl+F2")
						if ui.Button(c.Key("shot-exclude-apply"), tr("应用")).Clicked() {
							if a.setScreenshotShortcut(a.shotExcludeEdit, false) {
								a.setToast(tr("截图快捷键已更新"))
							}
						}
					})
				})
				row(tr("保存目录"), a.screenshotDir(), func() {
					if ui.Button(c.Key("choose-screenshot-dir"), tr("选择…")).Clicked() {
						a.chooseScreenshotDir()
					}
				})
				row(tr("捕获鼠标指针"), tr("截图时把鼠标指针一起截取下来"), func() {
					if ui.Switch(c.Key("shot-cursor"), &s.ScreenshotCursor).Label(tr("捕获鼠标指针")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("立即截图"), tr("选择区域后可以标注、复制、保存、贴图或发送到聊天"), func() {
					if ui.Button(c.Key("shot-now"), tr("截图")).Clicked() {
						a.openScreenshotChooser()
					}
				})
			})

			groups["history"] = group(tr("聊天记录"), func() {
				st := a.dbStats
				row(tr("SQLite 数据库"), tr("记录按会话和时间建立索引，保存在数据目录的 zchat.db 中"), func() {
					ui.Column(c).AlignItems(ui.End).Gap(2).Children(func() {
						ui.Text(c, itoa(st.Messages)+tr(" 条消息")).FontSize(12).TextColor(p.muted).FontFeatures("tnum")
						ui.Text(c, itoa(st.Conversations)+tr(" 个会话 · ")+model.HumanSize(st.Bytes)).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
					})
				})
				row(tr("打开数据库位置"), tr("可以在文件管理器里备份 zchat.db"), func() {
					if ui.Button(c.Key("open-db-dir"), tr("打开")).Clicked() {
						if a.store != nil {
							a.openPath(a.store.Dir())
						}
					}
				})
				row(tr("整理数据库"), tr("删除聊天记录后回收磁盘空间，记录很多时可能需要几秒"), func() {
					if ui.Button(c.Key("vacuum-db"), tr("整理")).Clicked() {
						if a.store != nil {
							if err := a.store.Vacuum(); err != nil {
								a.setToast(tr("整理失败：") + err.Error())
							} else {
								a.refreshDBStats()
								a.setToast(tr("数据库已整理"))
							}
							a.redraw()
						}
					}
				})
				row(tr("清空全部聊天记录"), tr("删除所有会话的消息，保留会话、资料和离线队列"), func() {
					if ui.Button(c.Key("clear-all-history"), tr("清空…")).Clicked() {
						a.confirm(tr("清空全部聊天记录"), tr("这会删除这台设备上所有会话的消息，且无法恢复。"), a.clearAllHistory)
					}
				})
			})

			groups["update"] = group(tr("更新"), func() {
				if !a.updateSupported {
					// Builds that cannot replace themselves, such as a .deb in
					// /opt, an AppImage or a development build, link to the
					// release page instead of pretending to update.
					row(tr("当前版本 ")+a.version, tr("这个版本无法自动更新，请从发布页面下载新版本。"), func() {
						ui.Text(c, tr("手动更新")).FontSize(11.5).TextColor(p.faint)
					})
					a.releaseLinkRow(c, p, "update")
					return
				}
				desc := tr("每天检查一次是否有新版本")
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += tr("；上次检查：") + shortStamp(t, time.Now())
				}
				row(tr("自动检查更新"), desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label(tr("自动检查更新")).Changed() {
						setUpdateChecks(on)
					}
				})
				row(tr("自动下载并安装"), tr("在后台下载新版本，下次打开时生效"), func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label(tr("自动下载并安装")).Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row(tr("当前版本 ")+a.version, tr("从 GitHub Releases 获取更新"), func() {
					if ui.Button(c.Key("check-updates"), tr("检查更新")).Clicked() {
						checkForUpdates()
					}
				})
			})

			groups["about"] = group(tr("关于"), func() {
				row("zchat", tr("局域网聊天 · 不经过服务器"), func() {
					ui.Text(c, "v"+a.version).FontSize(12).TextColor(p.muted)
				})
				row(tr("项目主页"), "github.com/zwdart/zchat", func() {
					if ui.Button(c.Key("project-home"), tr("打开")).Clicked() {
						a.openExternal("https://github.com/zwdart/zchat")
					}
				})
				row(tr("数据目录"), tr("资料、聊天记录和文件缓存都保存在这里"), func() {
					if ui.Button(c.Key("open-data"), tr("打开")).Clicked() {
						if a.store != nil {
							a.openPath(a.store.Dir())
						}
					}
				})
			})

			ui.Spacer(c).Height(20)
		})
	})

	if a.shotSettingsFocus {
		a.shotSettingsFocus = false
		a.settingsSection = "screenshot"
		a.settingsNav = "screenshot"
	}
	if a.settingsSection != "" {
		// The sections are built every frame; scroll as soon as the target
		// exists rather than keeping a reference across frames.
		if el, ok := groups[a.settingsSection]; ok {
			el.ScrollIntoView()
		}
		a.settingsSection = ""
	}
}

func (a *app) applyProfile() {
	name := strings.TrimSpace(a.profileName)
	if name == "" {
		name = tr("我的 zchat")
		a.profileName = name
	}
	a.profile.Name = name
	a.profile.Signature = strings.TrimSpace(a.profileSig)
	// Keep the names on messages already sent by this profile current.
	for _, msgs := range a.msgs {
		for i := range msgs {
			if msgs[i].SenderID == a.profile.ID && msgs[i].Mine {
				msgs[i].SenderName = a.profile.Name
			}
		}
	}
	if a.node != nil {
		a.node.SetProfile(a.profile)
	}
	a.scheduleSave()
	a.setToast(tr("资料已保存"))
	a.redraw()
}

func (a *app) pickAvatar() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.settingsWin,
			Title:       tr("选择头像"),
			ButtonLabel: tr("使用"),
			Filters:     []mygo.FileFilter{{Name: tr("图片"), Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		encoded, err := avatarBase64(paths[0])
		if err != nil {
			a.post(func() { a.setToast(tr("无法读取图片：") + err.Error()) })
			return
		}
		a.post(func() {
			a.profile.Avatar = encoded
			a.applyProfile()
		})
	}()
}

// avatarBase64 reads an image, scales it small and returns a PNG in base64,
// as the profile and announcements carry it.
func avatarBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	const side = 192
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return "", image.ErrFormat
	}
	scale := min(float64(side)/float64(w), float64(side)/float64(h))
	if scale > 1 {
		scale = 1
	}
	tw := max(1, int(float64(w)*scale))
	th := max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
