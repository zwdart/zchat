package app

import (
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/lan"
	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
)

// startNetwork starts discovery and the TCP listener. Peers reach the app
// only after the window exists, so their callbacks can post updates.
func (a *app) startNetwork() {
	p := a.profile
	p.Status = a.settings.Status
	a.profile.Status = p.Status
	n := lan.New(lan.Options{
		Profile: p,
		Port:    a.settings.Port,
		// Discovery off still leaves manual connections and file sharing.
		DisableDiscovery: !a.settings.Discovery,
		OnPeer: func(peer model.Peer) {
			a.post(func() { a.applyPeer(peer) })
		},
		OnConnect: func(peer model.Peer) {
			a.post(func() {
				a.applyPeer(peer)
				a.flushOutbox(peer.ID)
			})
		},
		OnDisconnect: func(id string) {
			a.post(func() {
				if p := a.peers[id]; p != nil {
					p.Connected = false
					a.scheduleSave()
				}
			})
		},
		OnFrame: func(id string, f proto.Frame) {
			a.post(func() { a.handleFrame(id, f) })
		},
		OnError: func(err error) {
			log.Println("zchat:", err)
		},
	})
	a.node = n
	if err := n.Start(); err != nil {
		a.setToast("无法监听端口：" + err.Error())
	}
}

// restartNetwork applies discovery and port settings by replacing the node.
func (a *app) restartNetwork() {
	if a.node != nil {
		a.node.Stop()
		a.node = nil
	}
	a.startNetwork()
}

func (a *app) applyPeer(p model.Peer) {
	if p.ID == "" || p.ID == a.profile.ID {
		return
	}
	old := a.peers[p.ID]
	if old != nil {
		p.Alias = old.Alias
		p.Manual = old.Manual
		if p.Avatar == "" {
			p.Avatar = old.Avatar
		}
	}
	cp := p
	a.peers[p.ID] = &cp
	a.scheduleSave()
}

func (a *app) queueFrame(peerID string, f proto.Frame) {
	if f.Type != proto.TypeChat && f.Type != proto.TypeGroup && f.Type != proto.TypeRecall {
		return
	}
	list := a.outbox[peerID]
	if len(list) >= 300 {
		list = list[len(list)-299:]
	}
	a.outbox[peerID] = append(list, f)
	a.scheduleSave()
}

// deliver sends a frame, queueing messages for a peer that is offline.
func (a *app) deliver(peerID string, f proto.Frame) {
	if peerID == "" || peerID == a.profile.ID {
		return
	}
	if a.node != nil && a.node.Send(peerID, f) {
		return
	}
	a.queueFrame(peerID, f)
}

// sendFrame sends an ephemeral frame, such as typing or a read receipt,
// without queueing it.
func (a *app) sendFrame(peerID string, f proto.Frame) {
	if a.node != nil {
		a.node.Send(peerID, f)
	}
}

func (a *app) flushOutbox(peerID string) {
	list := a.outbox[peerID]
	if len(list) == 0 {
		return
	}
	var rest []proto.Frame
	for _, f := range list {
		if !a.node.Send(peerID, f) {
			rest = append(rest, f)
		}
	}
	if len(rest) == 0 {
		delete(a.outbox, peerID)
	} else {
		a.outbox[peerID] = rest
	}
	a.scheduleSave()
}

func (a *app) handleFrame(id string, f proto.Frame) {
	switch f.Type {
	case proto.TypeChat:
		var chat proto.Chat
		if f.Decode(&chat) != nil {
			return
		}
		a.receiveMessage(id, chat.Message)
	case proto.TypeAck:
		var ack proto.Ack
		if f.Decode(&ack) != nil {
			return
		}
		a.receiveAck(ack)
	case proto.TypeRead:
		var read proto.Read
		if f.Decode(&read) != nil {
			return
		}
		a.receiveRead(id, read)
	case proto.TypeTyping:
		var t proto.Typing
		if f.Decode(&t) != nil {
			return
		}
		a.receiveTyping(id, t)
	case proto.TypeRecall:
		var r proto.Recall
		if f.Decode(&r) != nil {
			return
		}
		a.receiveRecall(id, r)
	case proto.TypeGroup:
		var g proto.GroupAction
		if f.Decode(&g) != nil {
			return
		}
		a.receiveGroup(id, g)
	}
}

func (a *app) peerName(id string) string {
	if id == a.profile.ID {
		return a.profile.Name
	}
	if p := a.peers[id]; p != nil {
		return p.Title()
	}
	return "未知设备"
}

func (a *app) receiveMessage(from string, m model.Message) {
	m.Mine = false
	if m.ID == "" || m.SenderID == "" {
		m.SenderID = from
	}
	from = m.SenderID
	p := a.peers[from]
	if p == nil {
		if known, ok := a.node.Peer(from); ok {
			p = &known
			a.peers[from] = p
		} else {
			p = &model.Peer{ID: from, Name: m.SenderName, Status: model.Online, Connected: true, LastSeen: time.Now()}
			a.peers[from] = p
		}
	}
	// Path is the sender's copy; the receiver fetches its own.
	if m.File != nil {
		m.File.Path = ""
	}
	c := a.convs[m.ConvID]
	if strings.HasPrefix(m.ConvID, "g_") {
		// A group message needs the group it belongs to; do not show it
		// as a direct message when the invite has not arrived.
		if c == nil || c.Kind != model.KindGroup {
			return
		}
	} else if c == nil || c.Kind != model.KindDirect {
		// A direct message's conversation is the sender's peer ID on this
		// device, whichever ID the sender used.
		c = a.ensureDirect(p)
		m.ConvID = c.ID
	}
	if m.Kind == "" {
		m.Kind = model.KindText
	}
	if m.SenderName == "" {
		m.SenderName = p.Title()
	}
	if m.SentAt.IsZero() {
		m.SentAt = time.Now()
	}
	if m.Kind != model.KindSystem {
		m.Delivered = true
	}
	a.msgs[c.ID] = append(a.msgs[c.ID], m)
	c.UpdatedAt = m.SentAt
	a.saveMessages(c.ID)
	a.scheduleSave()

	a.sendFrame(from, proto.NewFrame(a.profile.ID, proto.TypeAck, proto.Ack{MessageID: m.ID, Time: time.Now().UnixMilli()}))

	if a.active == c.ID && a.activeVisible() {
		a.markActiveRead()
	} else {
		if !c.Muted {
			c.Unread++
		}
		a.notifyMessage(c, m)
	}
	a.updateBadge()
	if a.active == c.ID {
		a.msgList.ScrollToEnd()
	}
	if m.Kind == model.KindImage && a.settings.AutoDownloadImages {
		a.autoDownloadImage(c.ID, m.ID)
	}
	if m.Kind == model.KindSystem {
		return
	}
}

func (a *app) receiveAck(ack proto.Ack) {
	for convID, msgs := range a.msgs {
		for i := range msgs {
			if msgs[i].ID != ack.MessageID {
				continue
			}
			msgs[i].Delivered = true
			if msgs[i].Status != model.Read {
				msgs[i].Status = model.Delivered
			}
			a.saveMessages(convID)
			if a.active == convID {
				a.redraw()
			}
			return
		}
	}
}

func (a *app) receiveRead(from string, r proto.Read) {
	convID := r.ConvID
	if !strings.HasPrefix(convID, "g_") {
		if a.convs[from] != nil {
			convID = from
		}
	}
	msgs := a.msgs[convID]
	if len(msgs) == 0 {
		return
	}
	at := time.UnixMilli(r.Time)
	changed := false
	for i := range msgs {
		if msgs[i].Mine && !msgs[i].SentAt.After(at) {
			msgs[i].Read = true
			msgs[i].Delivered = true
			msgs[i].Status = model.Read
			changed = true
		}
	}
	if changed {
		a.saveMessages(convID)
		if a.active == convID {
			a.redraw()
		}
	}
}

func (a *app) receiveTyping(from string, t proto.Typing) {
	convID := t.ConvID
	if !strings.HasPrefix(convID, "g_") {
		convID = from
	}
	key := convID + "\x00" + from
	if !t.On {
		delete(a.typing, key)
		a.redraw()
		return
	}
	a.typing[key] = typingState{At: time.Now()}
	if a.active == convID {
		a.redraw()
	}
	time.AfterFunc(4*time.Second, func() {
		mygo.RunOnMain(func() {
			if st, ok := a.typing[key]; ok && time.Since(st.At) >= 3*time.Second {
				delete(a.typing, key)
				a.redraw()
			}
		})
	})
}

func (a *app) receiveRecall(from string, r proto.Recall) {
	for convID, msgs := range a.msgs {
		for i := range msgs {
			if msgs[i].ID != r.MessageID {
				continue
			}
			if msgs[i].SenderID != from && msgs[i].SenderID != a.profile.ID {
				return
			}
			msgs[i].Recalled = true
			a.saveMessages(convID)
			if a.active == convID {
				a.redraw()
			}
			return
		}
	}
}

func (a *app) sendText(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c := a.currentConv()
	if c == nil {
		return
	}
	a.sendTextTo(c.ID, text)
	a.input = ""
	c.Draft = ""
	a.sendTyping(false)
}

func (a *app) sendTextTo(convID, text string) {
	c := a.convs[convID]
	if c == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	m := model.Message{
		ID:         model.NewID("m"),
		ConvID:     c.ID,
		SenderID:   a.profile.ID,
		SenderName: a.profile.Name,
		Kind:       model.KindText,
		Text:       text,
		SentAt:     time.Now(),
		Status:     model.Sent,
		Mine:       true,
	}
	a.addLocalMessage(c, &m)
}

func (a *app) addLocalMessage(c *model.Conversation, m *model.Message) {
	m.Mine = true
	m.ConvID = c.ID
	a.msgs[c.ID] = append(a.msgs[c.ID], *m)
	c.UpdatedAt = m.SentAt
	c.Draft = ""
	a.saveMessages(c.ID)
	a.scheduleSave()
	a.dispatchMessage(c, *m)
	if a.active == c.ID {
		a.msgList.ScrollToEnd()
		a.markActiveRead()
	}
	a.redraw()
}

func (a *app) dispatchMessage(c *model.Conversation, m model.Message) {
	// Path is this device's copy: it means nothing to a receiver, and a
	// receiver that saw one would skip the download.
	if m.File != nil {
		file := *m.File
		file.Path = ""
		m.File = &file
	}
	f := proto.NewFrame(a.profile.ID, proto.TypeChat, proto.Chat{Message: m})
	switch c.Kind {
	case model.KindGroup:
		for _, id := range c.Members {
			if id != a.profile.ID {
				a.deliver(id, f)
			}
		}
	case model.KindDirect:
		if c.PeerID != "" {
			a.deliver(c.PeerID, f)
		}
	}
}

func (a *app) sendTyping(on bool) {
	c := a.currentConv()
	if c == nil {
		return
	}
	if on {
		if time.Since(a.lastTyping) < 2*time.Second {
			return
		}
		a.lastTyping = time.Now()
	}
	f := proto.NewFrame(a.profile.ID, proto.TypeTyping, proto.Typing{ConvID: c.ID, On: on})
	switch c.Kind {
	case model.KindDirect:
		a.sendFrame(c.PeerID, f)
	case model.KindGroup:
		for _, id := range c.Members {
			if id != a.profile.ID {
				a.sendFrame(id, f)
			}
		}
	}
}

func (a *app) stopTyping(convID string) {
	c := a.convs[convID]
	if c == nil {
		return
	}
	f := proto.NewFrame(a.profile.ID, proto.TypeTyping, proto.Typing{ConvID: c.ID, On: false})
	switch c.Kind {
	case model.KindDirect:
		a.sendFrame(c.PeerID, f)
	case model.KindGroup:
		for _, id := range c.Members {
			if id != a.profile.ID {
				a.sendFrame(id, f)
			}
		}
	}
}

func (a *app) typingNames(c *model.Conversation) []string {
	var out []string
	for key := range a.typing {
		conv, peer, ok := strings.Cut(key, "\x00")
		if !ok || conv != c.ID || peer == a.profile.ID {
			continue
		}
		out = append(out, a.peerName(peer))
	}
	return out
}

func (a *app) recallMessage(convID, msgID string) {
	c := a.convs[convID]
	if c == nil {
		return
	}
	msgs := a.msgs[convID]
	for i := range msgs {
		if msgs[i].ID != msgID || !msgs[i].Mine {
			continue
		}
		if time.Since(msgs[i].SentAt) > 2*time.Minute {
			a.setToast("超过 2 分钟的消息不能撤回")
			return
		}
		msgs[i].Recalled = true
		a.saveMessages(convID)
		f := proto.NewFrame(a.profile.ID, proto.TypeRecall, proto.Recall{MessageID: msgID})
		switch c.Kind {
		case model.KindDirect:
			a.deliver(c.PeerID, f)
		case model.KindGroup:
			for _, id := range c.Members {
				if id != a.profile.ID {
					a.deliver(id, f)
				}
			}
		}
		a.redraw()
		return
	}
}

func (a *app) deleteMessage(convID, msgID string) {
	msgs := a.msgs[convID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			a.msgs[convID] = append(msgs[:i], msgs[i+1:]...)
			a.saveMessages(convID)
			a.redraw()
			return
		}
	}
}

func (a *app) clearHistory(convID string) {
	if a.msgs[convID] == nil {
		return
	}
	a.msgs[convID] = nil
	a.saveMessages(convID)
	if a.store != nil {
		_ = a.store.DeleteConversation(convID)
	}
	a.redraw()
}

func (a *app) deleteConversation(convID string) {
	if c := a.convs[convID]; c != nil && c.Kind == model.KindGroup {
		a.leaveGroup(c)
		return
	}
	delete(a.convs, convID)
	delete(a.msgs, convID)
	if a.active == convID {
		a.active = ""
	}
	if a.store != nil {
		_ = a.store.DeleteConversation(convID)
	}
	a.scheduleSave()
	a.redraw()
}

func (a *app) notifyMessage(c *model.Conversation, m model.Message) {
	if !a.settings.Notifications || c.Muted {
		return
	}
	title := a.peerName(m.SenderID)
	body := m.Preview()
	if c.Kind == model.KindGroup {
		title = a.convTitle(c) + " · " + title
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "chat:" + c.ID,
		Title: title,
		Body:  body,
		Group: c.ID,
	})
	if old := a.notifs[c.ID]; old != nil {
		old.Close()
	}
	a.notifs[c.ID] = n
	if err := n.Show(); err != nil {
		delete(a.notifs, c.ID)
	}
}
