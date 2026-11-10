package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

func (a *app) dialogs(c *ui.Context) {
	a.newGroupDialog(c)
	a.manualDialog(c)
	a.groupInfoDialog(c)
	a.groupAddDialog(c)
	a.aliasDialog(c)
	a.userInfoDialog(c)
	a.confirmDialog(c)
	// The chooser opens above the composer's screenshot button; only when
	// that button did not render this frame (another page, or no
	// conversation open) does its centered panel stand in.
	if !a.shotChooserAnchored {
		a.screenshotChooserDialog(c)
	}
}

func dialogPanel(backdrop, panel ui.Element, p palette, width float32) {
	backdrop.Background(ui.RGBA(0, 0, 0, 0.38))
	panel.Width(width).Padding(22).Radius(18).Gap(14).
		Background(p.bg).Border(1, p.hair).
		Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.42)).
		Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
}

func (a *app) openNewGroup() {
	a.groupName = ""
	a.groupPicked = map[string]bool{}
	a.newGroupOpen = true
	a.redraw()
}

func (a *app) newGroupDialog(c *ui.Context) {
	p := a.pal
	ui.DialogBase(c.Key("new-group"), &a.newGroupOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 460)
		ui.Text(c, tr("新建群聊")).FontSize(18).FontWeight(700)
		ui.Text(c, tr("群聊只在加入的成员之间发送，消息会分别送达每台在线设备。")).FontSize(12).TextColor(p.muted).LineHeight(1.45)
		name := ui.TextInput(c.Key("group-name"), &a.groupName).Label(tr("群聊名称")).Placeholder(tr("例如：三号会议室")).AutoFocus()
		if name.Submitted() {
			a.submitNewGroup()
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr("选择成员")).FontSize(12).FontWeight(650).TextColor(p.muted)
			ui.Text(c, itoa(len(a.groupPicked))+tr(" 人")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			if len(a.sortedPeers()) > 0 {
				if ui.Button(c.Key("pick-all"), tr("全选")).Clicked() {
					for _, peer := range a.sortedPeers() {
						a.groupPicked[peer.ID] = true
					}
				}
				if ui.Button(c.Key("pick-none"), tr("全不选")).Clicked() {
					a.groupPicked = map[string]bool{}
				}
			}
		})
		peers := a.sortedPeers()
		if len(peers) == 0 {
			ui.Row(c).Padding(14).Radius(12).Background(p.panel).Center().Children(func() {
				ui.Text(c, tr("没有发现其他设备，也可以先创建一个只有自己的群聊。")).FontSize(12).TextColor(p.muted).TextAlign(ui.Center)
			})
		} else {
			ui.Scroll(c).MaxHeight(280).Gap(2).Children(func() {
				for _, peer := range peers {
					a.pickerRow(c, peer, a.groupPicked)
				}
			})
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButtonKey(c, "cancel", nil, tr("取消"), p.hover.Over(p.bg), p.text).Clicked() {
				a.newGroupOpen = false
			}
			ok := pillButtonKey(c, "create", nil, tr("创建"), p.accent, p.onAcc)
			if strings.TrimSpace(a.groupName) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				a.submitNewGroup()
			}
		})
	})
}

func (a *app) submitNewGroup() {
	name := strings.TrimSpace(a.groupName)
	if name == "" {
		return
	}
	var picked []string
	for id, on := range a.groupPicked {
		if on {
			picked = append(picked, id)
		}
	}
	a.newGroupOpen = false
	a.createGroup(name, picked)
}

func (a *app) pickerRow(c *ui.Context, peer *model.Peer, picked map[string]bool) {
	p := a.pal
	on := picked[peer.ID]
	row := ui.ButtonBase(c).Key("pick-"+peer.ID).Padding(8, 8).Radius(10).Gap(10).AlignItems(ui.Center).FocusRing(false).Label(tr(peer.Title()))
	if on {
		row.Background(p.accent.Alpha(0.1))
	} else if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		ui.Avatar(c, tr(peer.Title()), a.avatarBitmap(peer.Avatar)).Size(32, 32)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, tr(peer.Title())).FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, presenceLabel(peer.Status)+" · "+peerAddr(peer)).FontSize(10.5).TextColor(p.muted).SingleLine()
		})
		check := ui.Box(c).Size(20, 20).Radius(999).Center().Border(1.5, p.hair)
		if on {
			check.Background(p.accent).BorderColor(p.accent).Children(func() {
				ui.Icon(c, iconCheck).Size(12, 12).TextColor(p.onAcc)
			})
		}
	})
	if row.Clicked() {
		picked[peer.ID] = !on
		a.redraw()
	}
}

func (a *app) manualDialog(c *ui.Context) {
	p := a.pal
	ui.DialogBase(c.Key("manual-peer"), &a.manualOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 420)
		ui.Text(c, tr("通过 IP 连接")).FontSize(18).FontWeight(700)
		ui.Text(c, tr("自动发现被网络屏蔽时，输入对方的 IP 地址或 IP:端口。")).FontSize(12).TextColor(p.muted).LineHeight(1.45)
		in := ui.TextInput(c.Key("manual-addr"), &a.manualAddr).Label(tr("IP 地址")).Placeholder(tr("192.168.1.8 或 192.168.1.8:42224")).AutoFocus()
		if in.Submitted() {
			a.connectManual()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButtonKey(c, "cancel", nil, tr("取消"), p.hover.Over(p.bg), p.text).Clicked() {
				a.manualOpen = false
			}
			ok := pillButtonKey(c, "connect", nil, tr("连接"), p.accent, p.onAcc)
			if strings.TrimSpace(a.manualAddr) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				a.connectManual()
			}
		})
	})
}

func (a *app) connectManual() {
	addr := strings.TrimSpace(a.manualAddr)
	if addr == "" || a.node == nil {
		return
	}
	a.manualOpen = false
	a.setToast(tr("正在连接 ") + addr + "…")
	go func() {
		peer, err := a.node.ConnectAddress(addr)
		a.post(func() {
			if err != nil {
				a.setToast(tr("无法连接：") + err.Error())
				return
			}
			peer.Manual = true
			a.applyPeer(peer)
			a.setToast(tr("已连接 ") + tr(peer.Title()))
			a.openPeer(&peer)
		})
	}()
}

func (a *app) openGroupInfo(conv *model.Conversation) {
	if conv == nil || conv.Kind != model.KindGroup {
		return
	}
	a.groupInfoID = conv.ID
	a.groupRename = conv.Title
	a.groupInfoOpen = true
	a.redraw()
}

func (a *app) groupInfoDialog(c *ui.Context) {
	p := a.pal
	conv := a.convs[a.groupInfoID]
	if conv == nil || conv.Kind != model.KindGroup {
		a.groupInfoOpen = false
		return
	}
	ui.DialogBase(c.Key("group-info"), &a.groupInfoOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 480)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.groupAvatar(c, conv, 44)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr("群聊信息")).FontSize(18).FontWeight(700)
				ui.Text(c, itoa(len(conv.Members))+tr(" 位成员 · 群主 ")+a.peerName(conv.Owner)).FontSize(11.5).TextColor(p.muted)
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("group-name"), &a.groupRename).Label(tr("群聊名称")).Placeholder(tr("群聊名称")).Grow(1)
			save := ui.PrimaryButton(c.Key("group-rename"), tr("保存"))
			if strings.TrimSpace(a.groupRename) == "" || strings.TrimSpace(a.groupRename) == conv.Title {
				save.Disabled(true).Opacity(0.5)
			}
			if save.Clicked() || in.Submitted() {
				a.renameGroup(conv, a.groupRename)
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr("成员")).FontSize(12).FontWeight(650).TextColor(p.muted)
			ui.Spacer(c)
			if pillButtonKey(c, "add-members", iconPlus, tr("添加成员"), p.hover.Over(p.bg), p.text).Clicked() {
				a.openGroupAdd(conv)
			}
		})
		ui.Scroll(c).MaxHeight(280).Gap(2).Children(func() {
			for _, member := range a.groupMembers(conv) {
				member := member
				ui.Row(c).Padding(8, 8).Radius(10).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Avatar(c, tr(member.Title()), a.avatarBitmap(member.Avatar)).Size(32, 32)
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, tr(member.Title())).FontSize(13).FontWeight(600).SingleLine()
							if member.ID == a.profile.ID {
								ui.Text(c, tr("我")).FontSize(9.5).TextColor(p.accent).Padding(1, 5).Radius(999).Background(p.accent.Alpha(0.12))
							}
							if member.ID == conv.Owner {
								ui.Text(c, tr("群主")).FontSize(9.5).TextColor(p.faint).Padding(1, 5).Radius(999).Border(1, p.hair)
							}
						})
						ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
							dot(c, member.Status, 6)
							ui.Text(c, presenceLabel(member.Status)).FontSize(10.5).TextColor(p.muted)
						})
					})
					if conv.Owner == a.profile.ID && member.ID != a.profile.ID {
						if b := iconButton(c, iconX, 13, p.faint, tr("移出群聊")); b.Clicked() {
							id := member.ID
							a.confirm(tr("移出成员"), tr("确定把「")+tr(member.Title())+tr("」移出群聊吗？"), func() {
								a.removeGroupMember(conv, id)
							})
						}
					}
				})
			}
		})
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			label, prefix, dissolve := tr("退出群聊"), tr("确定退出群聊「"), false
			if conv.Owner == a.profile.ID && len(conv.Members) > 1 {
				label, prefix, dissolve = tr("解散群聊"), tr("确定解散群聊「"), true
			}
			if pillButtonKey(c, "leave", iconLeave, label, p.hover.Over(p.bg), p.text).Clicked() {
				a.confirm(label, prefix+conv.Title+tr("」吗？"), func() {
					if dissolve {
						a.disbandGroup(conv)
					} else {
						a.leaveGroup(conv)
					}
					a.groupInfoOpen = false
				})
			}
			if pillButtonKey(c, "done", nil, tr("完成"), p.accent, p.onAcc).Clicked() {
				a.groupInfoOpen = false
			}
		})
	})
}

func (a *app) openGroupAdd(conv *model.Conversation) {
	if conv == nil {
		return
	}
	a.groupAddID = conv.ID
	a.groupAddPicked = map[string]bool{}
	a.groupAddOpen = true
	a.redraw()
}

func (a *app) groupAddDialog(c *ui.Context) {
	p := a.pal
	conv := a.convs[a.groupAddID]
	if conv == nil {
		a.groupAddOpen = false
		return
	}
	inGroup := map[string]bool{}
	for _, id := range conv.Members {
		inGroup[id] = true
	}
	var candidates []*model.Peer
	for _, peer := range a.sortedPeers() {
		if !inGroup[peer.ID] {
			candidates = append(candidates, peer)
		}
	}
	ui.DialogBase(c.Key("group-add"), &a.groupAddOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 440)
		ui.Text(c, tr("添加成员")).FontSize(18).FontWeight(700)
		ui.Text(c, tr("邀请后会把自己的群聊信息发送给新成员。")).FontSize(12).TextColor(p.muted)
		if len(candidates) == 0 {
			ui.Row(c).Padding(16).Radius(12).Background(p.panel).Center().Children(func() {
				ui.Text(c, tr("没有可以邀请的设备。")).FontSize(12).TextColor(p.muted)
			})
		} else {
			ui.Scroll(c).MaxHeight(300).Gap(2).Children(func() {
				for _, peer := range candidates {
					a.pickerRow(c, peer, a.groupAddPicked)
				}
			})
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButtonKey(c, "cancel", nil, tr("取消"), p.hover.Over(p.bg), p.text).Clicked() {
				a.groupAddOpen = false
			}
			ok := pillButtonKey(c, "add", nil, tr("添加"), p.accent, p.onAcc)
			if len(a.groupAddPicked) == 0 {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				var ids []string
				for id, on := range a.groupAddPicked {
					if on {
						ids = append(ids, id)
					}
				}
				a.addGroupMembers(conv, ids)
				a.groupAddOpen = false
			}
		})
	})
}

func (a *app) openAlias(peer *model.Peer) {
	if peer == nil {
		return
	}
	a.aliasPeer = peer.ID
	a.aliasName = peer.Alias
	a.aliasOpen = true
	a.redraw()
}

func (a *app) aliasDialog(c *ui.Context) {
	p := a.pal
	peer := a.peers[a.aliasPeer]
	if peer == nil {
		a.aliasOpen = false
		return
	}
	ui.DialogBase(c.Key("alias"), &a.aliasOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 400)
		ui.Text(c, tr("设置备注")).FontSize(18).FontWeight(700)
		ui.Text(c, tr("备注只保存在这台设备上，会替换设备原来的名称。")).FontSize(12).TextColor(p.muted).LineHeight(1.45)
		in := ui.TextInput(c.Key("alias-name"), &a.aliasName).Label(tr("备注")).Placeholder(peer.Name).AutoFocus()
		if in.Submitted() {
			a.submitAlias(peer)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if peer.Alias != "" {
				if pillButtonKey(c, "clear-alias", nil, tr("清除备注"), p.hover.Over(p.bg), p.text).Clicked() {
					peer.Alias = ""
					a.aliasOpen = false
					a.scheduleSave()
					a.redraw()
				}
			}
			if pillButtonKey(c, "cancel", nil, tr("取消"), p.hover.Over(p.bg), p.text).Clicked() {
				a.aliasOpen = false
			}
			if pillButtonKey(c, "save-alias", nil, tr("保存"), p.accent, p.onAcc).Clicked() {
				a.submitAlias(peer)
			}
		})
	})
}

func (a *app) submitAlias(peer *model.Peer) {
	name := strings.TrimSpace(a.aliasName)
	if name == peer.Name {
		name = ""
	}
	peer.Alias = name
	a.aliasOpen = false
	a.scheduleSave()
	a.setToast(tr("备注已保存"))
	a.redraw()
}

func (a *app) confirmDialog(c *ui.Context) {
	p := a.pal
	ui.DialogBase(c.Key("confirm"), &a.confirmOpen, func(backdrop, panel ui.Element) {
		dialogPanel(backdrop, panel, p, 380)
		ui.Text(c, a.confirmTitle).FontSize(17).FontWeight(700)
		ui.Text(c, a.confirmText).FontSize(12.5).TextColor(p.muted).LineHeight(1.5)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if pillButtonKey(c, "cancel", nil, tr("取消"), p.hover.Over(p.bg), p.text).Clicked() {
				a.confirmOpen = false
				a.confirmAction = nil
			}
			if pillButtonKey(c, "ok", nil, tr("确定"), p.accent, p.onAcc).Clicked() {
				action := a.confirmAction
				a.confirmOpen = false
				a.confirmAction = nil
				if action != nil {
					action()
				}
			}
		})
	})
}

// screenshotChooserRows are the two choices of the screenshot chooser.
func (a *app) screenshotChooserRows(c *ui.Context, p palette) {
	if pillButtonKey(c, "shot-with-chat", nil, tr("包含当前聊天窗口"), p.accent, p.onAcc).FillWidth().Clicked() {
		a.shotChooserOpen = false
		a.redraw()
		a.startScreenshot(true)
	}
	if pillButtonKey(c, "shot-without-chat", nil, tr("不包含当前聊天窗口"), p.hover.Over(p.bg), p.text).FillWidth().Clicked() {
		a.shotChooserOpen = false
		a.redraw()
		a.startScreenshot(false)
	}
}

// screenshotChooserPopover asks whether the next screenshot includes the
// chat window, in a small panel above the composer's screenshot button.
func (a *app) screenshotChooserPopover(c *ui.Context, anchor ui.Element) {
	if !a.shotChooserOpen {
		return
	}
	a.shotChooserAnchored = true
	p := a.pal
	ui.PopoverBase(c, anchor, &a.shotChooserOpen, func(panel ui.Element) {
		panel.Width(232).Margin(6, 0, 0, 0).Padding(12).Radius(14).Gap(9).
			Background(p.bg).Border(1, p.hair).
			Shadow(0, 18, 44, -12, ui.RGBA(0, 0, 0, 0.4)).
			// The capture may follow this click within a frame or two, so
			// leave no exit fade that the shot could catch.
			Transition(ui.ElementTransition{})
		ui.Text(c, tr("截图")).FontSize(13).FontWeight(700)
		a.screenshotChooserRows(c, p)
		ui.Text(c, tr("快捷键")+"  "+shortcutLabel(a.settings.ScreenshotIncludeShortcut)+" / "+
			shortcutLabel(a.settings.ScreenshotExcludeShortcut)).FontSize(10.5).TextColor(p.faint)
	})
}

// screenshotChooserDialog is the chooser for entry points whose window has
// no composer button to anchor the popover to.
func (a *app) screenshotChooserDialog(c *ui.Context) {
	p := a.pal
	ui.DialogBase(c.Key("shot-mode"), &a.shotChooserOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		panel.Width(320).Padding(18).Radius(16).Gap(11).
			Background(p.bg).Border(1, p.hair).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.42)).
			// As the popover: no exit fade that the capture could catch.
			Transition(ui.ElementTransition{})
		ui.Text(c, tr("截图")).FontSize(15).FontWeight(700)
		a.screenshotChooserRows(c, p)
		ui.Text(c, tr("快捷键")+"  "+shortcutLabel(a.settings.ScreenshotIncludeShortcut)+" / "+
			shortcutLabel(a.settings.ScreenshotExcludeShortcut)).FontSize(10.5).TextColor(p.faint)
	})
}
