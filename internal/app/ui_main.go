package app

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

const sidebarWidth = 292

// view builds the main window.
func (a *app) view(c *ui.Context) {
	// The screenshot chooser is a popover of the composer's button. Clear
	// the mark of the last frame so dialogs falls back to its own panel
	// when no composer button renders this frame.
	a.shotChooserAnchored = false
	t := *c.Theme()
	a.pal = paletteFor(&t)
	p := a.pal
	t.Accent, t.AccentText = p.accent, p.onAcc
	t.Selection = p.accent.Alpha(0.28)
	c.SetTheme(&t)

	if a.focusSearch {
		a.focusSearch = false
		a.searchBox.Focus(c)
	}
	if a.focusMsgQuery {
		a.focusMsgQuery = false
		a.msgSearch.Focus(c)
	}
	a.shortcuts(c)

	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Box(c).Fill().Children(func() {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			side := ui.Column(c).Width(sidebarWidth)
			if !c.Vibrancy() {
				side.Background(p.sidebar)
			}
			side.Children(func() { a.sidebar(c) })
			ui.Box(c).Key("content").Grow(1).Background(p.bg).BorderWidth(0, 0, 0, 1).BorderColor(p.hair).Children(func() {
				switch a.page {
				case pageDiscover:
					a.discoverPage(c)
				case pageShare:
					a.sharePage(c)
				default:
					if a.currentConv() != nil {
						a.chatPage(c)
					} else {
						a.welcomePage(c)
					}
				}
			})
		})
	})
	a.dialogs(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
}

func (a *app) shortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		a.focusSearch = true
	})
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyF, func() {
		if a.currentConv() != nil {
			a.msgSearchShown = true
			a.focusMsgQuery = true
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.openSettings)
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.msgSearchOpen() {
			a.msgQuery = ""
			a.clearMsgSearch()
			return
		}
		if a.newGroupOpen || a.manualOpen || a.groupInfoOpen || a.groupAddOpen || a.confirmOpen || a.shotChooserOpen {
			a.newGroupOpen = false
			a.manualOpen = false
			a.groupInfoOpen = false
			a.groupAddOpen = false
			a.confirmOpen = false
			a.shotChooserOpen = false
		}
	})
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyM, func() {
		if c := a.currentConv(); c != nil {
			c.Muted = !c.Muted
			a.scheduleSave()
			a.redraw()
		}
	})
}

func (a *app) sidebar(c *ui.Context) {
	p := a.pal
	top := float32(14)
	if c.TitleBar().Left > 0 {
		top = c.TitleBar().Height + 6
	}
	ui.Column(c).Fill().Padding(top, 14, 14, 14).Gap(4).DragWindow().Children(func() {
		a.profileRow(c)
		a.searchField(c)
		ui.Box(c).Height(6)
		a.navItem(c, iconChatFill, tr("聊天"), pageChat)
		a.navItem(c, iconCompass, tr("发现设备"), pageDiscover)
		a.navItem(c, iconFolder, tr("文件共享"), pageShare)
		ui.Row(c).Padding(10, 10, 6, 10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr("会话")).FontSize(11.5).FontWeight(700).TextColor(p.faint).LetterSpacing(0.6)
			ui.Spacer(c)
			b := iconButton(c, iconPlus, 15, p.muted, tr("新建群聊"))
			if b.Clicked() {
				a.openNewGroup()
			}
		})
		ui.Scroll(c).Key("conversations").Grow(1).Margin(0, -6).Padding(0, 6).Gap(2).Children(func() {
			if len(a.convs) == 0 {
				ui.Text(c, tr("还没有会话\n在「发现设备」里找人聊聊")).FontSize(12.5).TextColor(p.muted).LineHeight(1.6).Padding(10, 12)
			}
			for _, conv := range a.orderedConvs() {
				if !a.convMatches(conv) {
					continue
				}
				a.convRow(c, conv)
			}
		})
		a.statusRow(c)
	})
}

func (a *app) profileRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c).Key("profile").Padding(8, 8).Radius(12).Gap(10).AlignItems(ui.Center).FocusRing(false).Label(tr("个人资料"))
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Avatar(c, a.profile.Name, a.avatarBitmap(a.profile.Avatar)).Size(38, 38)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.profile.Name).FontSize(13.5).FontWeight(650).SingleLine()
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				dot(c, a.profile.Status, 7)
				ui.Text(c, presenceLabel(a.profile.Status)).FontSize(11.5).TextColor(p.muted).SingleLine()
			})
		})
		ui.Icon(c, iconSettings).Size(15, 15).TextColor(p.faint)
	})
	if b.Clicked() {
		a.openSettings()
	}
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item(tr("查看资料")).Chosen() {
			a.openUserInfo(a.profile.ID)
		}
		if m.Item(tr("设置…")).Chosen() {
			a.openSettings()
		}
	})
}

func (a *app) searchField(c *ui.Context) {
	in := ui.SearchField(c, &a.query).Bind(&a.searchBox).Label(tr("搜索会话")).Placeholder(tr("搜索会话或消息"))
	in.Margin(4, 8, 4, 8)
	in.Changed()
}

func (a *app) navItem(c *ui.Context, icon *ui.SVG, label string, page pageKind) {
	p := a.pal
	selected := a.page == page
	b := ui.ButtonBase(c).Key("nav-" + label).Height(34).PaddingX(10).Gap(10).Radius(9).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(label)
	bg := ui.Transparent
	switch {
	case selected:
		bg = p.press
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	color := p.text
	if selected {
		color = p.accent
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(color)
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(color)
	})
	if b.Clicked() {
		a.setPage(page)
	}
}

func (a *app) convMatches(conv *model.Conversation) bool {
	q := strings.ToLower(strings.TrimSpace(a.query))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(a.convTitle(conv)), q) {
		return true
	}
	if m := a.lastMessage(conv.ID); m != nil && strings.Contains(strings.ToLower(messagePreview(m)), q) {
		return true
	}
	return false
}

func (a *app) convRow(c *ui.Context, conv *model.Conversation) {
	p := a.pal
	active := a.page == pageChat && a.active == conv.ID
	b := ui.ButtonBase(c).Key("conv-"+conv.ID).Padding(8, 8).Radius(11).Gap(10).AlignItems(ui.Center).FocusRing(false).Label(a.convTitle(conv))
	bg := ui.Transparent
	switch {
	case active:
		bg = p.press
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	b.Children(func() {
		a.convAvatar(c, conv, 38)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.convTitle(conv)).FontSize(13.5).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				if t := a.convTime(conv); t != "" {
					ui.Text(c, t).FontSize(10.5).TextColor(p.faint).SingleLine()
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				preview := tr("还没有消息")
				if m := a.lastMessage(conv.ID); m != nil {
					preview = messagePreview(m)
					if m.Mine && m.Kind != model.KindSystem {
						preview = tr("我：") + preview
					}
				}
				ui.Text(c, preview).FontSize(11.5).TextColor(p.muted).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				if conv.Unread > 0 {
					ui.Box(c).MinWidth(17).Height(17).PaddingX(5).Radius(999).Center().Background(p.accent).Children(func() {
						ui.Text(c, shortCount(conv.Unread)).FontSize(10).FontWeight(700).TextColor(p.onAcc)
					})
				}
				if conv.Muted {
					ui.Icon(c, iconBell).Size(11, 11).TextColor(p.faint).Opacity(0.7)
				}
			})
		})
	})
	if b.Clicked() {
		a.openConv(conv.ID)
	}
	b.ContextMenu(func(m *ui.Menu) {
		if conv.Kind == model.KindGroup {
			if m.Item(tr("群聊信息")).Chosen() {
				a.openGroupInfo(conv)
			}
		} else if m.Item(tr("查看资料")).Chosen() {
			a.openUserInfo(conv.PeerID)
		}
		m.Separator()
		if m.Item(tr("置顶会话")).Checked(conv.Pinned).Chosen() {
			conv.Pinned = !conv.Pinned
			a.scheduleSave()
		}
		if m.Item(tr("消息免打扰")).Checked(conv.Muted).Chosen() {
			conv.Muted = !conv.Muted
			a.scheduleSave()
		}
		m.Separator()
		if m.Item(tr("清空聊天记录")).Chosen() {
			a.confirm(tr("清空聊天记录"), tr("只会删除这台设备上的记录。"), func() { a.clearHistory(conv.ID) })
		}
		if m.Item(tr("删除会话")).Chosen() {
			a.confirm(tr("删除会话"), tr("确定删除「")+a.convTitle(conv)+tr("」吗？"), func() { a.deleteConversation(conv.ID) })
		}
	})
}

func (a *app) convAvatar(c *ui.Context, conv *model.Conversation, size float32) {
	if conv.Kind == model.KindGroup {
		ui.Box(c).Size(size, size).Radius(size * 0.32).Center().Background(a.pal.accent.Alpha(0.16)).Children(func() {
			ui.Icon(c, iconUsers).Size(size*0.48, size*0.48).TextColor(a.pal.accent)
		})
		return
	}
	p := a.peers[conv.PeerID]
	name, avatar := "?", ""
	if p != nil {
		name, avatar = tr(p.Title()), p.Avatar
	}
	ui.Avatar(c, name, a.avatarBitmap(avatar)).Size(size, size)
}

func (a *app) convTime(conv *model.Conversation) string {
	if m := a.lastMessage(conv.ID); m != nil {
		return shortStamp(m.SentAt, time.Now())
	}
	return ""
}

func (a *app) lastMessage(convID string) *model.Message {
	msgs := a.msgs[convID]
	if len(msgs) == 0 {
		return nil
	}
	return &msgs[len(msgs)-1]
}

func shortStamp(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if model.SameDay(t, now) {
		return t.Format("15:04")
	}
	if model.SameDay(t, now.AddDate(0, 0, -1)) {
		return tr("昨天")
	}
	if t.Year() == now.Year() {
		return t.Format(tr("1月2日"))
	}
	return t.Format("2006/1/2")
}

func shortCount(n int) string {
	if n > 99 {
		return "99+"
	}
	return itoa(n)
}

func (a *app) statusRow(c *ui.Context) {
	ui.Row(c).Padding(8, 8).AlignItems(ui.Center).Gap(8).Children(func() {
		dot(c, a.profile.Status, 8)
		ui.PopoverMenuButton(c, presenceLabel(a.profile.Status), func(m *ui.Menu) {
			for _, o := range model.PresenceOptions {
				if m.Item(tr(o.Label)).Checked(a.profile.Status == o.Value).Chosen() {
					a.setStatus(o.Value)
				}
			}
			m.Separator()
			if m.Item(tr("设置…")).Chosen() {
				a.openSettings()
			}
		}).Grow(1).Justify(ui.Start)
	})
}

// contentTop is the room the native window controls take at the top of the
// content pane. The sidebar offsets itself when the controls are on its
// side; this keeps the chat, discover and share pages clear when they are
// over the content, as on Windows and most Linux desktops.
func (a *app) contentTop(c *ui.Context) float32 {
	bar := c.TitleBar()
	if bar.Height > 0 && bar.Right > 0 {
		return bar.Height
	}
	return 0
}

func (a *app) welcomePage(c *ui.Context) {
	p := a.pal
	ui.Column(c).Fill().Center().Gap(14).DragWindow().Children(func() {
		ui.Box(c).Size(84, 84).Radius(26).Background(p.accent.Alpha(0.12)).Center().Children(func() {
			ui.Icon(c, iconChatFill).Size(40, 40).TextColor(p.accent)
		})
		ui.Text(c, "zchat").FontSize(28).FontWeight(750).LetterSpacing(-0.5)
		ui.Text(c, tr("局域网聊天，不经过服务器")).FontSize(14).TextColor(p.muted)
		ui.Row(c).Gap(10).Margin(0, 18, 0, 0).Children(func() {
			if pillButton(c, iconCompass, tr("发现设备"), p.accent, p.onAcc).Clicked() {
				a.setPage(pageDiscover)
			}
			if pillButton(c, iconUsers, tr("新建群聊"), p.panel, p.text).Clicked() {
				a.openNewGroup()
			}
		})
	})
}

// groupAvatar is a group's avatar.
func (a *app) groupAvatar(c *ui.Context, conv *model.Conversation, size float32) {
	ui.Box(c).Size(size, size).Radius(size * 0.3).Center().Background(a.pal.accent.Alpha(0.16)).Children(func() {
		ui.Icon(c, iconUsers).Size(size*0.48, size*0.48).TextColor(a.pal.accent)
	})
}

func topInset(c *ui.Context) float32 {
	if runtime.GOOS == "darwin" && c.TitleBar().Height > 0 {
		return c.TitleBar().Height
	}
	return 0
}
