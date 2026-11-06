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
	a.profileName = a.profile.Name
	a.profileSig = a.profile.Signature
	a.portEdit = float64(a.settings.Port)
	a.shareEdit = float64(a.settings.SharePort)
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "zchat 设置",
		Width:           600,
		Height:          760,
		MinWidth:        500,
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

	ui.Scroll(c).Fill().Padding(24, 28, 30, 28).Gap(6).Children(func() {
		ui.Text(c, "设置").FontSize(26).FontWeight(750).LetterSpacing(-0.4).Margin(0, 0, 6, 2)
		ui.Text(c, "这些设置只影响这台设备。").FontSize(12).TextColor(p.muted).Margin(0, 0, 0, 8)

		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hair).Dividers(1, p.hair).Children(rows)
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

		group("个人资料", func() {
			row("名称", "其他设备在局域网里看到的名字", func() {
				ui.TextInput(c.Key("profile-name"), &a.profileName).Label("名称").Width(210).
					Placeholder("我的 zchat").OnSubmit(a.applyProfile)
			})
			row("签名", "一句介绍，显示在设备卡片里", func() {
				ui.TextInput(c.Key("profile-signature"), &a.profileSig).Label("签名").Width(210).
					Placeholder("可选").OnSubmit(a.applyProfile)
			})
			ui.Row(c).Padding(12, 15).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Avatar(c, a.profileName, a.avatarBitmap(a.profile.Avatar)).Size(46, 46)
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, "头像").FontSize(13.5).FontWeight(500)
					ui.Text(c, "选择一张图片，会缩放到小尺寸后保存在本机").FontSize(11.5).TextColor(p.muted)
				})
				if a.profile.Avatar != "" {
					if ui.Button(c.Key("remove-avatar"), "移除").Clicked() {
						a.profile.Avatar = ""
						a.applyProfile()
					}
				}
				if ui.Button(c.Key("pick-avatar"), "选择…").Clicked() {
					a.pickAvatar()
				}
				if ui.PrimaryButton(c.Key("save-profile"), "保存资料").Clicked() {
					a.applyProfile()
				}
			})
			row("状态", "在线状态会广播给已连接的设备", func() {
				labels := make([]string, len(model.PresenceOptions))
				chosen := a.profile.Status.Label()
				for i, o := range model.PresenceOptions {
					labels[i] = o.Label
					if o.Value == a.profile.Status {
						chosen = o.Label
					}
				}
				if ui.Select(c.Key("status"), &chosen, labels).Label("状态").Width(140).Changed() {
					for _, o := range model.PresenceOptions {
						if o.Label == chosen {
							a.setStatus(o.Value)
						}
					}
				}
			})
		})

		group("外观", func() {
			row("主题", "跟随系统，或固定浅色、深色", func() {
				labels := []string{"跟随系统", "浅色", "深色"}
				values := []string{"system", "light", "dark"}
				chosen := labels[0]
				for i, v := range values {
					if v == s.Appearance {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("appearance"), &chosen, labels).Label("主题").Width(140).Changed() {
					for i, label := range labels {
						if label == chosen {
							s.Appearance = values[i]
						}
					}
					a.settingsChanged()
				}
			})
		})

		group("通知", func() {
			row("新消息通知", "窗口不在前台时显示系统通知", func() {
				if ui.Switch(c.Key("notifications"), &s.Notifications).Label("新消息通知").Changed() {
					a.settingsChanged()
				}
			})
			row("自动下载图片", "收到图片后自动缓存到本机，打开聊天即可查看", func() {
				if ui.Switch(c.Key("auto-images"), &s.AutoDownloadImages).Label("自动下载图片").Changed() {
					a.settingsChanged()
					if s.AutoDownloadImages {
						a.queueActiveImages()
					}
				}
			})
			row("关闭窗口时留在托盘", "关闭窗口后继续接收消息，可从菜单栏或通知区重新打开", func() {
				if ui.Switch(c.Key("close-tray"), &s.CloseToTray).Label("关闭窗口时留在托盘").Changed() {
					a.settingsChanged()
				}
			})
		})

		group("网络", func() {
			row("自动发现设备", "通过 UDP 组播和广播在局域网里寻找其他 zchat", func() {
				if ui.Switch(c.Key("discovery"), &s.Discovery).Label("自动发现设备").Changed() {
					a.settingsChanged()
					a.restartNetwork()
				}
			})
			row("监听端口", "TCP 端口，默认 47824；修改后重启网络生效", func() {
				if ui.NumberInput(c.Key("tcp-port"), &a.portEdit, 1024, 65535, 1).Label("监听端口").Width(110).Changed() {
					s.Port = int(a.portEdit)
					_ = s.save()
				}
			})
			row("本机地址", "把这些地址告诉无法自动发现的设备", func() {
				ip := netutil.PrimaryIP()
				if ip == "" {
					ip = "未连接局域网"
				}
				ui.Text(c, ip+":"+itoa(a.nodePort())).FontSize(12).TextColor(p.muted).FontFeatures("tnum")
			})
			row("重新启动网络", "让端口或发现设置立即生效", func() {
				if ui.Button(c.Key("restart-network"), "重启网络").Clicked() {
					s.Port = int(a.portEdit)
					_ = s.save()
					a.restartNetwork()
					a.setToast("网络已重启")
				}
			})
		})

		group("文件", func() {
			row("下载目录", a.settings.DownloadDir, func() {
				if ui.Button(c.Key("choose-downloads"), "选择…").Clicked() {
					a.chooseDownloadDir()
				}
			})
			row("共享目录", "在「文件共享」页面开启后，访客可以浏览这个目录", func() {
				if ui.Button(c.Key("open-share-page"), "打开共享").Clicked() {
					a.showMain()
					a.setPage(pageShare)
				}
			})
			row("共享端口", "HTTP 服务端口，默认 47830", func() {
				if ui.NumberInput(c.Key("share-port"), &a.shareEdit, 1024, 65535, 1).Label("共享端口").Width(110).Changed() {
					s.SharePort = int(a.shareEdit)
					_ = s.save()
					if a.shareRunning() {
						a.startShare(false)
					}
				}
			})
			row("启动时自动共享", "打开 zchat 后自动开启上一个共享目录", func() {
				if ui.Switch(c.Key("share-start"), &s.ShareOnStart).Label("启动时自动共享").Changed() {
					a.settingsChanged()
				}
			})
		})

		group("更新", func() {
			desc := "每天检查一次是否有新版本"
			if t := lastUpdateCheck(); !t.IsZero() {
				desc += "；上次检查：" + shortStamp(t, time.Now())
			}
			row("自动检查更新", desc, func() {
				on := updateChecks()
				if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
					setUpdateChecks(on)
				}
			})
			row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
				on := updateDownloads()
				if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
					setUpdateDownloads(on)
				}
			})
			row("当前版本 "+a.version, "从 GitHub Releases 获取更新", func() {
				if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
					checkForUpdates()
				}
			})
		})

		group("关于", func() {
			row("zchat", "局域网聊天 · 不经过服务器", func() {
				ui.Text(c, "v"+a.version).FontSize(12).TextColor(p.muted)
			})
			row("项目主页", "github.com/zwdart/zchat", func() {
				if ui.Button(c.Key("project-home"), "打开").Clicked() {
					a.openExternal("https://github.com/zwdart/zchat")
				}
			})
			row("数据目录", "资料、聊天记录和文件缓存都保存在这里", func() {
				if ui.Button(c.Key("open-data"), "打开").Clicked() {
					if a.store != nil {
						a.openPath(a.store.Dir())
					}
				}
			})
		})

		ui.Spacer(c).Height(20)
	})
}

func (a *app) applyProfile() {
	name := strings.TrimSpace(a.profileName)
	if name == "" {
		name = "我的 zchat"
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
	a.setToast("资料已保存")
	a.redraw()
}

func (a *app) pickAvatar() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.settingsWin,
			Title:       "选择头像",
			ButtonLabel: "使用",
			Filters:     []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		encoded, err := avatarBase64(paths[0])
		if err != nil {
			a.post(func() { a.setToast("无法读取图片：" + err.Error()) })
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
