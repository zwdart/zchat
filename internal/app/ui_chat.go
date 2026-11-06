package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

// chatRow is one row of the message list: a day heading, or a message by
// index in the conversation's messages.
type chatRow struct {
	index int // -1 for a day heading
	day   string
	at    time.Time
}

// rebuildRows lays the conversation's messages out with day headings,
// keeping only those matching the search when there is one.
func (a *app) rebuildRows(conv *model.Conversation) {
	if conv == nil {
		a.rows = nil
		return
	}
	msgs := a.msgs[conv.ID]
	if a.rowsConv == conv.ID && a.rowsLen == len(msgs) && a.rowsQuery == a.msgQuery {
		return
	}
	a.rowsConv, a.rowsLen, a.rowsQuery = conv.ID, len(msgs), a.msgQuery
	q := strings.ToLower(strings.TrimSpace(a.msgQuery))
	now := time.Now()
	rows := make([]chatRow, 0, len(msgs)+4)
	lastDay := ""
	for i := range msgs {
		m := &msgs[i]
		if q != "" && !messageMatches(m, q) {
			continue
		}
		day := model.DayLabel(m.SentAt, now)
		if day != lastDay {
			rows = append(rows, chatRow{index: -1, day: day, at: m.SentAt})
			lastDay = day
		}
		rows = append(rows, chatRow{index: i, at: m.SentAt})
	}
	a.rows = rows
}

func messageMatches(m *model.Message, q string) bool {
	if strings.Contains(strings.ToLower(m.Text), q) {
		return true
	}
	if strings.Contains(strings.ToLower(m.SenderName), q) {
		return true
	}
	return m.File != nil && strings.Contains(strings.ToLower(m.File.Name), q)
}

func (a *app) chatPage(c *ui.Context) {
	conv := a.currentConv()
	if conv == nil {
		a.welcomePage(c)
		return
	}
	a.rebuildRows(conv)

	ui.Column(c).Fill().Children(func() {
		a.chatHeader(c, conv)
		if a.msgSearchShown {
			a.messageSearch(c, conv)
		}
		a.messageList(c, conv)
		a.composer(c, conv)
	})
}

func (a *app) chatHeader(c *ui.Context, conv *model.Conversation) {
	p := a.pal
	ui.Row(c).Height(62).Padding(18, 14).Gap(12).AlignItems(ui.Center).BorderWidth(0, 0, 0, 1).BorderColor(p.hair).Children(func() {
		a.convAvatar(c, conv, 38)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.convTitle(conv)).FontSize(15).FontWeight(650).SingleLine().Shrink(1).MinWidth(0)
				if conv.Muted {
					ui.Icon(c, iconBell).Size(12, 12).TextColor(p.faint)
				}
			})
			sub := a.convSubtitle(conv)
			if names := a.typingNames(conv); len(names) > 0 {
				sub = typingLabel(names)
			}
			ui.Text(c, sub).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
		if b := iconButton(c, iconSearch, 16, p.muted, "搜索聊天记录"); b.Clicked() {
			a.msgSearchShown = !a.msgSearchShown
			if a.msgSearchShown {
				a.focusMsgQuery = true
			} else {
				a.clearMsgSearch()
			}
		}
		more := iconButton(c, iconMore, 16, p.muted, "更多")
		more.PopoverMenu(func(m *ui.Menu) { a.chatMenu(m, conv) })
	})
}

func (a *app) convSubtitle(conv *model.Conversation) string {
	if conv.Kind == model.KindGroup {
		n := len(conv.Members)
		online := 0
		for _, id := range conv.Members {
			if p := a.peers[id]; p != nil && p.Connected && p.Status.Available() {
				online++
			}
		}
		if n > 0 {
			return itoa(n) + " 位成员 · " + itoa(online) + " 人在线"
		}
		return "群聊"
	}
	p := a.peers[conv.PeerID]
	if p == nil {
		return "未知设备"
	}
	if p.Connected && p.Status.Available() {
		return p.Status.Label()
	}
	if !p.LastSeen.IsZero() {
		return p.Status.Label() + " · 最后在线 " + shortStamp(p.LastSeen, time.Now())
	}
	return p.Status.Label()
}

func (a *app) chatMenu(m *ui.Menu, conv *model.Conversation) {
	if conv.Kind == model.KindGroup {
		if m.Item("群聊信息").Chosen() {
			a.openGroupInfo(conv)
		}
		if m.Item("添加成员…").Chosen() {
			a.openGroupAdd(conv)
		}
		m.Separator()
	} else if p := a.peers[conv.PeerID]; p != nil {
		if m.Item("设置备注…").Chosen() {
			a.openAlias(p)
		}
		if p.IP != "" {
			if m.Item("复制 IP 地址").Chosen() {
				mygo.Clipboard.WriteText(p.IP)
				a.setToast("已复制 " + p.IP)
			}
		}
		m.Separator()
	}
	if m.Item("消息免打扰").Checked(conv.Muted).Chosen() {
		conv.Muted = !conv.Muted
		a.scheduleSave()
	}
	if m.Item("搜索聊天记录").Chosen() {
		a.msgSearchShown = true
		a.focusMsgQuery = true
		a.redraw()
	}
	m.Separator()
	if m.Item("清空聊天记录").Chosen() {
		a.confirm("清空聊天记录", "只会删除这台设备上的记录。", func() { a.clearHistory(conv.ID) })
	}
	if conv.Kind == model.KindGroup {
		if m.Item("退出群聊…").Chosen() {
			action := "退出"
			if conv.Owner == a.profile.ID && len(conv.Members) > 1 {
				action = "解散"
			}
			a.confirm(action+"群聊", "确定"+action+"「"+a.convTitle(conv)+"」吗？", func() {
				if action == "解散" {
					a.disbandGroup(conv)
				} else {
					a.leaveGroup(conv)
				}
			})
		}
	} else if m.Item("删除会话").Chosen() {
		a.confirm("删除会话", "确定删除「"+a.convTitle(conv)+"」吗？", func() { a.deleteConversation(conv.ID) })
	}
}

func (a *app) messageSearch(c *ui.Context, conv *model.Conversation) {
	p := a.pal
	ui.Row(c).Padding(18, 8).Gap(8).AlignItems(ui.Center).Background(p.panel).Children(func() {
		in := ui.SearchField(c, &a.msgQuery).Bind(&a.msgSearch).Label("搜索聊天记录").Placeholder("搜索聊天记录").Grow(1)
		in.Changed()
		count := 0
		for _, r := range a.rows {
			if r.index >= 0 {
				count++
			}
		}
		if strings.TrimSpace(a.msgQuery) != "" {
			ui.Text(c, itoa(count)+" 条结果").FontSize(11.5).TextColor(p.muted)
		}
		if b := iconButton(c, iconX, 14, p.muted, "关闭搜索"); b.Clicked() {
			a.clearMsgSearch()
		}
	})
}

func (a *app) messageList(c *ui.Context, conv *model.Conversation) {
	msgs := a.msgs[conv.ID]
	a.msgList.Key = func(row int) any {
		if row < 0 || row >= len(a.rows) {
			return row
		}
		r := a.rows[row]
		if r.index < 0 {
			return "day:" + r.day
		}
		if r.index < len(msgs) {
			return msgs[r.index].ID
		}
		return "stale:" + itoa(row)
	}
	a.msgList.FollowEnd = strings.TrimSpace(a.msgQuery) == ""
	if a.msgList.FollowEnd {
		a.msgList.FollowAnimation = &ui.Smooth
	} else {
		a.msgList.FollowAnimation = nil
	}
	ui.List(c.Key("messages"), &a.msgList, len(a.rows), func(i int) {
		a.messageRow(c, conv, i)
	}).Grow(1).Padding(16, 20, 12, 20).Gap(2)
}

func (a *app) messageRow(c *ui.Context, conv *model.Conversation, row int) {
	if row < 0 || row >= len(a.rows) {
		return
	}
	r := a.rows[row]
	p := a.pal
	if r.index < 0 {
		ui.Row(c).Padding(0, 12).Center().Children(func() {
			ui.Text(c, r.day).FontSize(11).FontWeight(600).TextColor(p.faint).
				Padding(6, 12).Radius(999).Background(p.panel)
		})
		return
	}
	msgs := a.msgs[conv.ID]
	if r.index < 0 || r.index >= len(msgs) {
		return
	}
	m := &msgs[r.index]
	if m.Kind == model.KindSystem {
		ui.Row(c).Padding(0, 8).Center().Children(func() {
			ui.Text(c, m.Text).FontSize(11.5).TextColor(p.faint).TextAlign(ui.Center).MaxWidth(520)
		})
		return
	}
	if m.Recalled {
		who := m.SenderName
		if m.Mine {
			who = "你"
		}
		ui.Row(c).Padding(0, 8).Center().Children(func() {
			ui.Text(c, who+"撤回了一条消息").FontSize(11.5).TextColor(p.faint)
		})
		return
	}

	mine := m.Mine
	justify := ui.Start
	if mine {
		justify = ui.End
	}
	ui.Row(c).FillWidth().Justify(justify).AlignItems(ui.End).Gap(8).Padding(0, 3).Children(func() {
		if !mine && conv.Kind == model.KindGroup {
			a.memberAvatar(c, m.SenderID, 28)
		}
		ui.Column(c).MaxWidth(a.bubbleWidth(c)).AlignItems(justify).Gap(3).Children(func() {
			if !mine && conv.Kind == model.KindGroup {
				ui.Text(c, a.peerName(m.SenderID)).FontSize(10.5).TextColor(p.muted).PaddingX(4)
			}
			a.messageBody(c, conv, m)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				if mine {
					ui.Text(c, messageStatus(m)).FontSize(10).TextColor(p.faint)
				}
				ui.Text(c, model.ShortTime(m.SentAt)).FontSize(10).TextColor(p.faint)
			})
		})
	})
}

func (a *app) bubbleWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	w -= sidebarWidth
	if w < 420 {
		return w - 80
	}
	return min(560, w*0.68)
}

func (a *app) memberAvatar(c *ui.Context, id string, size float32) {
	name, avatar := "?", ""
	if id == a.profile.ID {
		name, avatar = a.profile.Name, a.profile.Avatar
	} else if p := a.peers[id]; p != nil {
		name, avatar = p.Title(), p.Avatar
	}
	ui.Avatar(c, name, a.avatarBitmap(avatar)).Size(size, size)
}

func (a *app) messageBody(c *ui.Context, conv *model.Conversation, m *model.Message) {
	switch m.Kind {
	case model.KindImage:
		a.imageBubble(c, conv, m)
	case model.KindFile:
		a.fileBubble(c, conv, m)
	default:
		a.textBubble(c, m)
	}
}

func (a *app) textBubble(c *ui.Context, m *model.Message) {
	p := a.pal
	bg, fg := p.other, p.text
	if m.Mine {
		bg, fg = p.mine, p.onMine
	}
	b := ui.Column(c).MaxWidth(a.bubbleWidth(c)).Padding(9, 13).Radius(15).Background(bg)
	b.Children(func() {
		ui.Text(c, m.Text).FontSize(14).TextColor(fg).LineHeight(1.45).Selectable()
	})
	a.messageMenu(b, m)
}

func (a *app) imageBubble(c *ui.Context, conv *model.Conversation, m *model.Message) {
	p := a.pal
	w := float32(300)
	h := float32(190)
	if m.File != nil && m.File.Width > 0 && m.File.Height > 0 {
		w = float32(min(m.File.Width, 380))
		h = w * float32(m.File.Height) / float32(m.File.Width)
		if h > 300 {
			h = 300
			w = h * float32(m.File.Width) / float32(m.File.Height)
		}
	}
	if w < 120 {
		w = 120
	}
	if h < 90 {
		h = 90
	}
	var bmp *ui.Bitmap
	if m.File != nil && m.File.Path != "" {
		bmp = a.loadImage(m.File.Path)
	}
	if bmp != nil {
		b := ui.ButtonBase(c).Radius(14).FocusRing(false).Label("[图片] " + fileNameOf(m)).
			BorderWidth(1).BorderColor(p.hair).Children(func() {
			ui.Image(c, bmp).Size(w, h).Fit(ui.Contain).Radius(13)
		})
		if b.Clicked() {
			a.openPath(m.File.Path)
		}
		a.messageMenu(b, m)
		return
	}
	// No local copy yet: offer to fetch it.
	label := "点击下载图片"
	progress := ""
	if st := a.transfers[m.ID]; st != nil && st.Working {
		label = "正在下载…"
		if st.Total > 0 {
			progress = model.HumanSize(st.Done) + " / " + model.HumanSize(st.Total)
		}
	}
	b := ui.ButtonBase(c).Size(w, h).Radius(14).FocusRing(false).Label(label).
		Background(p.other).Border(1, p.hair).Center().Children(func() {
		ui.Column(c).Center().Gap(8).Children(func() {
			ui.Icon(c, iconImage).Size(28, 28).TextColor(p.faint)
			ui.Text(c, label).FontSize(12).TextColor(p.muted)
			if progress != "" {
				ui.Text(c, progress).FontSize(10.5).TextColor(p.faint)
			}
		})
	})
	if b.Clicked() {
		a.downloadMessage(conv.ID, m.ID, "")
	}
	a.messageMenu(b, m)
}

func (a *app) fileBubble(c *ui.Context, conv *model.Conversation, m *model.Message) {
	p := a.pal
	if m.File == nil {
		a.textBubble(c, m)
		return
	}
	file := m.File
	st := a.transfers[m.ID]
	bg, fg := p.other, p.text
	if m.Mine {
		bg, fg = p.mine, p.onMine
	}
	iconBG := p.accent.Alpha(0.16)
	iconFG := p.accent
	if m.Mine {
		iconBG = ui.RGBA(255, 255, 255, 0.2)
		iconFG = p.onMine
	}
	b := ui.ButtonBase(c).Width(min(360, a.bubbleWidth(c))).Padding(12, 13).Radius(14).
		FocusRing(false).Label("[文件] " + file.Name).Background(bg).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(38, 38).Radius(10).Center().Background(iconBG).Children(func() {
				ui.Icon(c, iconFile).Size(19, 19).TextColor(iconFG)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, file.Name).FontSize(13).FontWeight(600).TextColor(fg).SingleLine()
				meta := model.HumanSize(file.Size)
				if file.MIME != "" && !strings.HasPrefix(file.MIME, "application/octet-stream") {
					meta += " · " + shortMIME(file.MIME)
				}
				if st != nil && st.Working && st.Total > 0 {
					meta = model.HumanSize(st.Done) + " / " + model.HumanSize(st.Total)
				} else if st != nil && st.Err != "" {
					meta = "下载失败：" + st.Err
				} else if file.Path != "" {
					meta += " · 已下载"
				}
				ui.Text(c, meta).FontSize(10.5).TextColor(fg.Alpha(0.7)).SingleLine()
				if st != nil && st.Working && st.Total > 0 {
					ratio := float64(st.Done) / float64(st.Total)
					ui.Progress(c, ratio).Height(3).Radius(2).Margin(2, 0, 0, 0)
				}
			})
			ui.Icon(c, iconDownload).Size(17, 17).TextColor(fg.Alpha(0.75))
		})
	})
	if b.Clicked() {
		if file.Path != "" {
			a.openPath(file.Path)
		} else {
			a.downloadMessage(conv.ID, m.ID, "")
		}
	}
	a.messageMenu(b, m)
}

func shortMIME(mimeType string) string {
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = mimeType[:i]
	}
	parts := strings.Split(mimeType, "/")
	if len(parts) == 2 {
		return strings.ToUpper(parts[1])
	}
	return mimeType
}

func (a *app) messageMenu(b ui.Element, m *model.Message) {
	b.ContextMenu(func(menu *ui.Menu) {
		if m.Kind == model.KindText && m.Text != "" {
			if menu.Item("复制文本").Chosen() {
				mygo.Clipboard.WriteText(m.Text)
				a.setToast("已复制")
			}
		}
		if m.File != nil {
			if m.File.Path != "" {
				if menu.Item("打开").Chosen() {
					a.openPath(m.File.Path)
				}
				if menu.Item("在文件夹中显示").Chosen() {
					mygo.Shell.ShowItemInFolder(m.File.Path)
				}
			} else if m.File.URL != "" {
				if menu.Item("下载").Chosen() {
					a.downloadMessage(m.ConvID, m.ID, "")
				}
			}
			if m.File.URL != "" {
				if menu.Item("复制下载链接").Chosen() {
					mygo.Clipboard.WriteText(m.File.URL)
					a.setToast("已复制下载链接")
				}
			}
			if !m.Mine && m.File.URL != "" {
				if menu.Item("另存为…").Chosen() {
					a.saveMessageAs(m.ConvID, m.ID)
				}
			}
			menu.Separator()
		}
		if m.Mine {
			if time.Since(m.SentAt) <= 2*time.Minute {
				if menu.Item("撤回").Chosen() {
					a.recallMessage(m.ConvID, m.ID)
				}
			}
			if menu.Item("删除").Chosen() {
				a.deleteMessage(m.ConvID, m.ID)
			}
		}
	})
}

func messageStatus(m *model.Message) string {
	if m.Status == model.Failed {
		return "发送失败"
	}
	if m.Read {
		return "已读"
	}
	if m.Delivered || m.Status == model.Delivered {
		return "已送达"
	}
	return "已发送"
}

func fileNameOf(m *model.Message) string {
	if m.File == nil {
		return ""
	}
	return m.File.Name
}

func (a *app) composer(c *ui.Context, conv *model.Conversation) {
	p := a.pal
	ui.Column(c).FillWidth().Padding(16, 12, 16, 16).Gap(7).Children(func() {
		row := ui.Row(c).FillWidth().Radius(16).Background(p.panel).Border(1, p.hair).Padding(6, 8).Gap(2).AlignItems(ui.End)
		row.Children(func() {
			if b := iconButton(c, iconImage, 16, p.muted, "发送图片"); b.Clicked() {
				a.pickImages()
			}
			if b := iconButton(c, iconClip, 16, p.muted, "发送文件"); b.Clicked() {
				a.pickFiles()
			}
			if b := iconButton(c, iconLink, 16, p.muted, "发送共享链接"); b.Clicked() {
				a.sendShareLink()
			}
			in := ui.TextArea(c, &a.input).Placeholder("输入消息，Enter 发送，Shift+Enter 换行").
				FontSize(14).MinHeight(38).MaxHeight(132).Grow(1).MinWidth(0).
				Padding(4, 8).FocusRing(false).Label("消息")
			in.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods&ui.Shift != 0 || k.Mods&ui.Cmd != 0 {
					return false
				}
				text := k.Text()
				if strings.TrimSpace(text) == "" {
					return true
				}
				a.sendText(text)
				k.SetText("")
				return true
			})
			in.OnPaste(func(text string) bool {
				if text != "" {
					return false
				}
				if len(mygo.Clipboard.ReadImage()) == 0 {
					return false
				}
				a.sendClipboardImage()
				return true
			})
			in.OnChange(func() {
				conv.Draft = a.input
				if strings.TrimSpace(a.input) == "" {
					a.sendTyping(false)
				} else {
					a.sendTyping(true)
				}
				a.scheduleSave()
			})
			send := ui.ButtonBase(c).Size(38, 38).Radius(999).Center().FocusRing(false).Label("发送")
			fill := p.accent
			if strings.TrimSpace(a.input) == "" {
				fill = p.press
			}
			send.Background(fill)
			pose := ui.Pose{}
			if send.Pressed() && fill != p.press {
				pose.Scale = 0.9
			}
			send.AnimatePose(pose, ui.Snappy)
			send.Children(func() {
				color := p.onAcc
				if strings.TrimSpace(a.input) == "" {
					color = p.muted
				}
				ui.Icon(c, iconSend).Size(17, 17).TextColor(color)
			})
			if send.Clicked() && strings.TrimSpace(a.input) != "" {
				a.sendText(a.input)
			}
		})
		ui.Row(c).Gap(6).PaddingX(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "局域网内点对点发送 · 文件经本机 HTTP 共享").FontSize(10).TextColor(p.faint)
			ui.Spacer(c)
			if a.shareRunning() {
				ui.Icon(c, iconWifi).Size(10, 10).TextColor(p.faint)
				ui.Text(c, "文件共享已开启").FontSize(10).TextColor(p.faint)
			}
		})
	})
}
