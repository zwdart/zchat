package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

func (a *app) openUserInfo(id string) {
	if id == "" {
		return
	}
	if id != a.profile.ID && a.peers[id] == nil {
		return
	}
	a.userInfoID = id
	a.userInfoOpen = true
	a.redraw()
}

// userInfoDialog shows the full details behind an avatar. It works for the
// local profile and for any peer the app knows.
func (a *app) userInfoDialog(c *ui.Context) {
	if !a.userInfoOpen {
		return
	}
	p := a.pal
	id := a.userInfoID
	self := id == a.profile.ID
	var peer *model.Peer
	if !self {
		peer = a.peers[id]
		if peer == nil {
			a.userInfoOpen = false
			return
		}
	}

	name, avatar := a.profile.Name, a.profile.Avatar
	status := a.profile.Status
	signature := a.profile.Signature
	if peer != nil {
		name, avatar = tr(peer.Title()), peer.Avatar
		status, signature = peer.Status, peer.Signature
	}

	ui.DialogBase(c.Key("user-info"), &a.userInfoOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 430)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Avatar(c, name, a.avatarBitmap(avatar)).Size(64, 64)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, name).FontSize(19).FontWeight(720).SingleLine().Shrink(1).MinWidth(0)
					if self {
						ui.Text(c, tr("我")).FontSize(9.5).TextColor(p.accent).Padding(1, 5).Radius(999).Background(p.accent.Alpha(0.12))
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					dot(c, status, 7)
					ui.Text(c, presenceLabel(status)).FontSize(12).TextColor(p.muted)
				})
			})
		})
		if signature != "" {
			ui.Text(c, signature).FontSize(12.5).TextColor(p.muted).LineHeight(1.5).Selectable()
		}

		detail := func(label, value string) {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Text(c, label).Width(100).Shrink(0).FontSize(11.5).TextColor(p.faint)
				ui.Text(c, value).FontSize(12).Grow(1).Shrink(1).MinWidth(0).Selectable()
			})
		}
		ui.Column(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(7).Children(func() {
			detail(tr("状态"), presenceLabel(status))
			if peer != nil {
				detail(tr("名称"), peer.Name)
				if peer.Alias != "" {
					detail(tr("备注"), peer.Alias)
				}
			}
			detail(tr("用户 ID"), id)
			if peer != nil {
				detail(tr("IP 地址"), peerAddr(peer))
				transfer := tr("未启用")
				if peer.TransferPort > 0 {
					transfer = peer.IP + ":" + itoa(peer.TransferPort)
				}
				detail(tr("文件传输端口"), transfer)
				if peer.Version != "" {
					detail(tr("版本"), peer.Version)
				}
				if peer.Platform != "" {
					detail(tr("平台"), peer.Platform)
				}
				if !peer.LastSeen.IsZero() {
					detail(tr("最后在线"), shortStamp(peer.LastSeen, time.Now()))
				}
			}
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if self {
				if pillButtonKey(c, "edit-profile", iconSettings, tr("编辑资料"), p.accent, p.onAcc).Clicked() {
					a.userInfoOpen = false
					a.openSettings()
				}
			} else {
				if pillButtonKey(c, "message-peer", iconChat, tr("发消息"), p.accent, p.onAcc).Clicked() {
					a.userInfoOpen = false
					a.showMain()
					a.openPeer(peer)
				}
				if pillButtonKey(c, "alias-peer", nil, tr("设置备注"), p.hover.Over(p.bg), p.text).Clicked() {
					a.userInfoOpen = false
					a.openAlias(peer)
				}
				if peer.IP != "" {
					if pillButtonKey(c, "copy-ip", nil, tr("复制 IP 地址"), p.hover.Over(p.bg), p.text).Clicked() {
						c.WriteClipboard(peer.IP)
						a.setToast(tr("已复制 ") + peer.IP)
					}
				}
			}
			ui.Spacer(c)
			if pillButtonKey(c, "close-info", nil, tr("关闭"), p.hover.Over(p.bg), p.text).Clicked() {
				a.userInfoOpen = false
			}
		})
	})
}
