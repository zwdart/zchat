package app

import (
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/netutil"
)

// sortedPeers lists the known peers with those reachable first.
func (a *app) sortedPeers() []*model.Peer {
	out := make([]*model.Peer, 0, len(a.peers))
	for id, p := range a.peers {
		if p == nil || id == a.profile.ID {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Status.Available() != b.Status.Available() {
			return a.Status.Available()
		}
		return strings.ToLower(a.Title()) < strings.ToLower(b.Title())
	})
	return out
}

func (a *app) discoverPage(c *ui.Context) {
	p := a.pal
	peers := a.sortedPeers()
	online := 0
	for _, peer := range peers {
		if peer.Connected && peer.Status.Available() {
			online++
		}
	}

	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(18).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(16).DragWindow().Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					sectionTitle(c, p, tr("发现设备"), tr("同一局域网里运行 zchat 的设备会自动出现"))
				})
				if pillButton(c, iconRefresh, tr("刷新"), p.hover.Over(p.bg), p.text).Clicked() {
					if a.node != nil {
						a.node.AnnounceNow()
					}
					a.setToast(tr("正在重新搜索局域网设备"))
				}
				if pillButton(c, iconPlus, tr("通过 IP 连接"), p.accent, p.onAcc).Clicked() {
					a.manualAddr = ""
					a.manualOpen = true
				}
			})

			a.thisDeviceCard(c, online, len(peers))

			if len(peers) == 0 {
				a.emptyDiscover(c)
				return
			}

			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, tr("设备")).FontSize(12).FontWeight(700).TextColor(p.muted).LetterSpacing(0.5)
				ui.Text(c, itoa(len(peers))).FontSize(11).TextColor(p.faint)
				ui.Spacer(c)
				ui.Text(c, tr("在线 ")+itoa(online)).FontSize(11.5).TextColor(p.muted)
			})
			ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(8).Children(func() {
				for _, peer := range peers {
					a.peerCard(c, peer)
				}
			})
		})
	})
}

func (a *app) thisDeviceCard(c *ui.Context, online, total int) {
	p := a.pal
	ui.Row(c).FillWidth().Padding(16, 16).Radius(14).Background(p.panel).Border(1, p.hair).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, a.profile.Name, a.avatarBitmap(a.profile.Avatar)).Size(46, 46).
			Label(tr("查看资料")).OnClick(func() { a.openUserInfo(a.profile.ID) })
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				dot(c, a.profile.Status, 8)
				ui.Text(c, a.profile.Name).FontSize(14).FontWeight(650).SingleLine()
				ui.Text(c, tr("本机")).FontSize(10).FontWeight(600).TextColor(p.accent).
					Padding(2, 7).Radius(999).Background(p.accent.Alpha(0.12))
			})
			addr := tr("端口 ") + itoa(a.nodePort())
			if ip := netutil.PrimaryIP(); ip != "" {
				addr = ip + " · " + addr
			}
			if !a.settings.Discovery {
				addr += tr(" · 自动发现已关闭")
			}
			ui.Text(c, addr).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
		ui.Column(c).AlignItems(ui.End).Gap(2).Children(func() {
			ui.Text(c, itoa(online)+tr(" 在线")).FontSize(12).FontWeight(600)
			ui.Text(c, itoa(total)+tr(" 台设备")).FontSize(10.5).TextColor(p.faint)
		})
		if b := iconButton(c, iconSettings, 15, p.muted, tr("网络设置")); b.Clicked() {
			a.openSettings()
		}
	})
}

func (a *app) emptyDiscover(c *ui.Context) {
	p := a.pal
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Box(c).Size(74, 74).Radius(22).Background(p.accent.Alpha(0.1)).Center().Children(func() {
			ui.Icon(c, iconWifi).Size(34, 34).TextColor(p.accent)
		})
		ui.Text(c, tr("还没有发现设备")).FontSize(17).FontWeight(650)
		ui.Text(c, tr("请确认对方也打开了 zchat，并且两台设备在同一个 Wi-Fi 或局域网内。\n如果网络屏蔽了广播，可以用 IP 地址直接连接。")).FontSize(12.5).
			TextColor(p.muted).TextAlign(ui.Center).LineHeight(1.6).MaxWidth(430)
		ui.Row(c).Gap(10).Margin(0, 8, 0, 0).Children(func() {
			if pillButton(c, iconRefresh, tr("重新搜索"), p.accent, p.onAcc).Clicked() {
				if a.node != nil {
					a.node.AnnounceNow()
				}
			}
			if pillButton(c, iconPlus, tr("通过 IP 连接"), p.hover.Over(p.bg), p.text).Clicked() {
				a.manualOpen = true
			}
		})
	})
}

func (a *app) peerCard(c *ui.Context, peer *model.Peer) {
	p := a.pal
	active := peer.Connected && peer.Status.Available()
	b := ui.ButtonBase(c).Key("peer-"+peer.ID).FillWidth().Padding(13, 14).Radius(13).Gap(13).
		AlignItems(ui.Center).FocusRing(false).Label(tr(peer.Title()))
	bg := p.panel
	if b.Hovered() {
		bg = p.hover.Over(p.bg)
	}
	if !active {
		b.Opacity(0.68)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		ui.Avatar(c, tr(peer.Title()), a.avatarBitmap(peer.Avatar)).Size(42, 42)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr(peer.Title())).FontSize(13.5).FontWeight(650).SingleLine().Shrink(1).MinWidth(0)
				dot(c, peer.Status, 7)
				ui.Text(c, presenceLabel(peer.Status)).FontSize(10.5).TextColor(p.muted)
				if peer.Manual {
					ui.Text(c, tr("手动")).FontSize(9.5).TextColor(p.faint).
						Padding(1, 5).Radius(999).Border(1, p.hair)
				}
			})
			meta := peerAddr(peer)
			if peer.Signature != "" {
				meta = peer.Signature + " · " + meta
			}
			ui.Text(c, meta).FontSize(11).TextColor(p.muted).SingleLine()
			info := strings.TrimSpace(strings.Join(nonEmpty(peer.Version, peer.Platform), " · "))
			if info != "" {
				ui.Text(c, info).FontSize(10).TextColor(p.faint).SingleLine()
			}
		})
		if active {
			if pillButton(c, iconChat, tr("发消息"), p.accent, p.onAcc).Clicked() {
				a.openPeer(peer)
			}
		} else {
			ui.Column(c).AlignItems(ui.End).Gap(3).Children(func() {
				ui.Text(c, tr("离线")).FontSize(11.5).TextColor(p.muted)
				if !peer.LastSeen.IsZero() {
					ui.Text(c, shortStamp(peer.LastSeen, time.Now())).FontSize(10).TextColor(p.faint)
				}
			})
		}
	})
	if b.Clicked() {
		a.openPeer(peer)
	}
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item(tr("查看资料")).Chosen() {
			a.openUserInfo(peer.ID)
		}
		m.Separator()
		if m.Item(tr("发送消息")).Chosen() {
			a.openPeer(peer)
		}
		if m.Item(tr("设置备注…")).Chosen() {
			a.openAlias(peer)
		}
		if peer.IP != "" {
			if m.Item(tr("复制 IP 地址")).Chosen() {
				mygo.Clipboard.WriteText(peer.IP)
				a.setToast(tr("已复制 ") + peer.IP)
			}
		}
		if peer.Signature != "" {
			if m.Item(tr("复制签名")).Chosen() {
				mygo.Clipboard.WriteText(peer.Signature)
				a.setToast(tr("已复制签名"))
			}
		}
		m.Separator()
		if peer.Manual || !peer.Connected {
			if m.Item(tr("从设备列表移除")).Chosen() {
				a.confirm(tr("移除设备"), tr("确定移除「")+tr(peer.Title())+tr("」吗？聊天记录会保留。"), func() {
					delete(a.peers, peer.ID)
					a.scheduleSave()
					a.redraw()
				})
			}
		}
	})
}

func (a *app) openPeer(peer *model.Peer) {
	if peer == nil {
		return
	}
	c := a.ensureDirect(peer)
	if c != nil {
		a.openConv(c.ID)
	}
}

func (a *app) nodePort() int {
	if a.node != nil {
		return a.node.Port()
	}
	return a.settings.Port
}

func peerAddr(p *model.Peer) string {
	if p.IP == "" {
		return tr("地址未知")
	}
	if p.Port == 0 {
		return p.IP
	}
	return p.IP + ":" + itoa(p.Port)
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
