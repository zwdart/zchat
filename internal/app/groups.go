package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/proto"
)

func (a *app) groupForConv(c *model.Conversation) model.Group {
	if c == nil {
		return model.Group{}
	}
	return model.Group{
		ID:        c.ID,
		Name:      c.Title,
		Owner:     c.Owner,
		Members:   append([]string(nil), c.Members...),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// createGroup makes a group with the peers picked and tells each of them.
func (a *app) createGroup(name string, picked []string) *model.Conversation {
	name = strings.TrimSpace(name)
	if name == "" {
		name = tr("未命名群聊")
	}
	members := []string{a.profile.ID}
	seen := map[string]bool{a.profile.ID: true}
	for _, id := range picked {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		members = append(members, id)
	}
	now := time.Now()
	g := model.Group{
		ID:        model.NewID("g"),
		Name:      name,
		Owner:     a.profile.ID,
		Members:   members,
		CreatedAt: now,
		UpdatedAt: now,
	}
	c := g.Conversation()
	c.UpdatedAt = now
	a.convs[c.ID] = c
	a.msgs[c.ID] = nil
	a.appendSystem(c, tr("你创建了群聊「")+name+tr("」"))
	a.sendGroupAction(g, proto.GroupInvite, "")
	a.scheduleSave()
	a.openConv(c.ID)
	return c
}

func (a *app) sendGroupAction(g model.Group, action, intro string) {
	f := proto.NewFrame(a.profile.ID, proto.TypeGroup, proto.GroupAction{Action: action, Group: g, Intro: intro})
	for _, id := range g.Members {
		if id != a.profile.ID {
			a.deliver(id, f)
		}
	}
}

func (a *app) receiveGroup(from string, action proto.GroupAction) {
	g := action.Group
	if g.ID == "" || !strings.HasPrefix(g.ID, "g_") {
		return
	}
	switch action.Action {
	case proto.GroupInvite:
		a.receiveInvite(from, g, action.Intro)
	case proto.GroupUpdate:
		a.receiveGroupUpdate(from, g)
	case proto.GroupLeave:
		a.receiveGroupLeave(g, from)
	case proto.GroupDisband:
		if c := a.convs[g.ID]; c != nil {
			delete(a.convs, g.ID)
			delete(a.msgs, g.ID)
			if a.active == g.ID {
				a.active = ""
			}
			if a.store != nil {
				_ = a.store.DeleteConversation(g.ID)
				a.refreshDBStats()
			}
			a.scheduleSave()
			a.setToast(tr("群聊「") + c.Title + tr("」已解散"))
		}
	}
}

func (a *app) receiveInvite(from string, g model.Group, intro string) {
	if !g.HasMember(a.profile.ID) {
		return
	}
	if c := a.convs[g.ID]; c != nil {
		a.applyGroup(c, g)
		return
	}
	c := g.Conversation()
	if c.Title == "" {
		c.Title = tr("群聊")
	}
	c.UpdatedAt = time.Now()
	a.convs[c.ID] = c
	a.msgs[c.ID] = nil
	text := a.peerName(from) + tr(" 邀请你加入群聊")
	if intro != "" {
		text += tr("：") + intro
	}
	a.appendSystem(c, text)
	c.Unread = 1
	a.scheduleSave()
	a.updateBadge()
	a.notifyGroupInvite(c, from)
	a.redraw()
}

func (a *app) receiveGroupUpdate(from string, g model.Group) {
	c := a.convs[g.ID]
	if c == nil {
		if !g.HasMember(a.profile.ID) {
			return
		}
		c = g.Conversation()
		a.convs[g.ID] = c
		a.msgs[g.ID] = nil
		a.appendSystem(c, tr("你加入了群聊「")+c.Title+tr("」"))
	} else {
		if !g.HasMember(a.profile.ID) {
			delete(a.convs, g.ID)
			delete(a.msgs, g.ID)
			if a.active == g.ID {
				a.active = ""
			}
			if a.store != nil {
				_ = a.store.DeleteConversation(g.ID)
				a.refreshDBStats()
			}
			a.setToast(tr("你已被移出群聊「") + c.Title + tr("」"))
			a.scheduleSave()
			return
		}
		a.applyGroup(c, g)
	}
	a.scheduleSave()
	a.redraw()
}

func (a *app) receiveGroupLeave(g model.Group, from string) {
	c := a.convs[g.ID]
	if c == nil {
		return
	}
	removed := false
	members := c.Members[:0]
	for _, id := range c.Members {
		if id == from {
			removed = true
			continue
		}
		members = append(members, id)
	}
	if !removed {
		return
	}
	c.Members = members
	c.UpdatedAt = time.Now()
	a.appendSystem(c, a.peerName(from)+tr(" 退出了群聊"))
	a.scheduleSave()
	a.redraw()
}

func (a *app) applyGroup(c *model.Conversation, g model.Group) {
	c.Title = g.Name
	c.Owner = g.Owner
	c.Members = append([]string(nil), g.Members...)
	c.UpdatedAt = time.Now()
}

func (a *app) notifyGroupInvite(c *model.Conversation, from string) {
	if !a.settings.Notifications {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "chat:" + c.ID,
		Title: a.peerName(from) + tr(" 邀请你加入群聊"),
		Body:  c.Title,
		Group: c.ID,
	})
	if old := a.notifs[c.ID]; old != nil {
		old.Close()
	}
	a.notifs[c.ID] = n
	_ = n.Show()
}

func (a *app) renameGroup(c *model.Conversation, name string) {
	name = strings.TrimSpace(name)
	if c == nil || name == "" || name == c.Title {
		return
	}
	c.Title = name
	c.UpdatedAt = time.Now()
	a.appendSystem(c, tr("群聊改名为「")+name+tr("」"))
	a.sendGroupAction(a.groupForConv(c), proto.GroupUpdate, "")
	a.scheduleSave()
	a.redraw()
}

func (a *app) addGroupMembers(c *model.Conversation, ids []string) {
	if c == nil {
		return
	}
	seen := map[string]bool{}
	for _, id := range c.Members {
		seen[id] = true
	}
	added := 0
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		c.Members = append(c.Members, id)
		added++
	}
	if added == 0 {
		return
	}
	c.UpdatedAt = time.Now()
	a.appendSystem(c, tr("新增了 ")+itoa(added)+tr(" 位成员"))
	a.sendGroupAction(a.groupForConv(c), proto.GroupUpdate, "")
	a.scheduleSave()
	a.redraw()
}

func (a *app) removeGroupMember(c *model.Conversation, id string) {
	if c == nil || id == a.profile.ID {
		return
	}
	var members []string
	for _, m := range c.Members {
		if m != id {
			members = append(members, m)
		}
	}
	c.Members = members
	c.UpdatedAt = time.Now()
	a.appendSystem(c, a.peerName(id)+tr(" 被移出了群聊"))
	a.sendGroupAction(a.groupForConv(c), proto.GroupUpdate, "")
	a.scheduleSave()
	a.redraw()
}

func (a *app) leaveGroup(c *model.Conversation) {
	if c == nil {
		return
	}
	g := a.groupForConv(c)
	f := proto.NewFrame(a.profile.ID, proto.TypeGroup, proto.GroupAction{Action: proto.GroupLeave, Group: g})
	for _, id := range c.Members {
		if id != a.profile.ID {
			a.deliver(id, f)
		}
	}
	a.forgetMessageFiles(a.msgs[c.ID])
	delete(a.convs, c.ID)
	delete(a.msgs, c.ID)
	if a.active == c.ID {
		a.active = ""
	}
	if a.store != nil {
		_ = a.store.DeleteConversation(c.ID)
		a.refreshDBStats()
	}
	a.scheduleSave()
	a.setToast(tr("已退出群聊「") + c.Title + tr("」"))
}

func (a *app) disbandGroup(c *model.Conversation) {
	if c == nil {
		return
	}
	g := a.groupForConv(c)
	f := proto.NewFrame(a.profile.ID, proto.TypeGroup, proto.GroupAction{Action: proto.GroupDisband, Group: g})
	for _, id := range c.Members {
		if id != a.profile.ID {
			a.deliver(id, f)
		}
	}
	a.forgetMessageFiles(a.msgs[c.ID])
	delete(a.convs, c.ID)
	delete(a.msgs, c.ID)
	if a.active == c.ID {
		a.active = ""
	}
	if a.store != nil {
		_ = a.store.DeleteConversation(c.ID)
		a.refreshDBStats()
	}
	a.scheduleSave()
	a.setToast(tr("群聊「") + c.Title + tr("」已解散"))
}

func (a *app) appendSystem(c *model.Conversation, text string) {
	if c == nil {
		return
	}
	m := model.Message{
		ID:         model.NewID("m"),
		ConvID:     c.ID,
		SenderID:   a.profile.ID,
		SenderName: a.profile.Name,
		Kind:       model.KindSystem,
		Text:       text,
		SentAt:     time.Now(),
	}
	a.msgs[c.ID] = append(a.msgs[c.ID], m)
	c.UpdatedAt = m.SentAt
	a.saveMessages(c.ID)
}

func (a *app) groupMembers(c *model.Conversation) []model.Peer {
	if c == nil {
		return nil
	}
	out := make([]model.Peer, 0, len(c.Members))
	for _, id := range c.Members {
		if id == a.profile.ID {
			out = append(out, a.profile.Peer("", 0))
			continue
		}
		if p := a.peers[id]; p != nil {
			out = append(out, *p)
			continue
		}
		out = append(out, model.Peer{ID: id, Name: tr("未知成员"), Status: model.Offline})
	}
	return out
}
