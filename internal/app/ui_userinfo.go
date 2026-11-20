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
						ui.Text(c, tr("app.common.me")).FontSize(9.5).TextColor(p.accent).Padding(1, 5).Radius(999).Background(p.accent.Alpha(0.12))
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					dot(c, status, 7)
					if self {
						// Clicking the status opens the same menu as the
						// menu bar, so the profile dialog is where presence
						// changes now.
						ui.PopoverMenuButton(c, presenceLabel(status), func(m *ui.Menu) {
							for _, o := range model.PresenceOptions {
								if m.Item(tr(o.Label)).Checked(status == o.Value).Chosen() {
									a.setStatus(o.Value)
								}
							}
						}).Label(tr("app.chat.change_status"))
					} else {
						ui.Text(c, presenceLabel(status)).FontSize(12).TextColor(p.muted)
					}
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
			if peer != nil {
				detail(tr("app.common.status"), presenceLabel(status))
				detail(tr("app.common.name"), peer.Name)
				if peer.Alias != "" {
					detail(tr("app.common.alias"), peer.Alias)
				}
			}
			detail(tr("app.chat.user_id"), id)
			if peer != nil {
				detail(tr("app.common.ip_address"), peerAddr(peer))
				transfer := tr("app.chat.off")
				if peer.TransferPort > 0 {
					transfer = peer.IP + ":" + itoa(peer.TransferPort)
				}
				detail(tr("app.common.file_transfer_port"), transfer)
				if peer.Version != "" {
					detail(tr("app.common.version"), peer.Version)
				}
				if peer.Platform != "" {
					detail(tr("app.common.platform"), peer.Platform)
				}
				if !peer.LastSeen.IsZero() {
					detail(tr("app.chat.last_seen_2"), shortStamp(peer.LastSeen, time.Now()))
				}
			}
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if self {
				if pillButtonKey(c, "edit-profile", iconSettings, tr("app.chat.edit_profile"), p.accent, p.onAcc).Clicked() {
					a.userInfoOpen = false
					a.openSettings()
				}
			} else {
				if pillButtonKey(c, "message-peer", iconChat, tr("app.common.message"), p.accent, p.onAcc).Clicked() {
					a.userInfoOpen = false
					a.showMain()
					a.openPeer(peer)
				}
				if pillButtonKey(c, "alias-peer", nil, tr("app.common.set_alias"), p.hover.Over(p.bg), p.text).Clicked() {
					a.userInfoOpen = false
					a.openAlias(peer)
				}
				if peer.IP != "" {
					if pillButtonKey(c, "copy-ip", nil, tr("app.common.copy_ip_address"), p.hover.Over(p.bg), p.text).Clicked() {
						c.WriteClipboard(peer.IP)
						a.setToast(tr("app.common.copied_2") + peer.IP)
					}
				}
			}
			ui.Spacer(c)
			if pillButtonKey(c, "close-info", nil, tr("app.common.close"), p.hover.Over(p.bg), p.text).Clicked() {
				a.userInfoOpen = false
			}
		})
	})
}
