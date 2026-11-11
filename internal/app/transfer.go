package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"xin.dart.zchat/internal/transfer"
)

// startTransfer starts the TCP service peers fetch chat attachments from.
func (a *app) startTransfer() {
	if a.transfer == nil {
		registry := ""
		if a.store != nil {
			registry = filepath.Join(a.store.Dir(), "transfers-tcp.json")
		}
		a.transfer = transfer.New(registry)
	}
	a.transfer.SetProgress(a.onTransferProgress)
	if a.transfer.Running() {
		return
	}
	if err := a.transfer.Start(a.settings.TransferPort); err != nil {
		a.setToast(tr("无法启动文件传输服务：") + err.Error())
		return
	}
	if port := a.transfer.Port(); port != a.settings.TransferPort {
		a.setToast(tr("文件传输端口 ") + itoa(a.settings.TransferPort) + tr(" 已被占用，改用 ") + itoa(port))
	}
}

// onTransferProgress shows how much of a sent file a receiver has read
// over TCP. It runs on a transfer goroutine.
func (a *app) onTransferProgress(token string, sent, total int64, err error) {
	a.post(func() { a.applySendProgress(token, "TCP", sent, total, err) })
}

// onShareProgress is the same for receivers using the HTTP fallback.
func (a *app) onShareProgress(token string, sent, total int64, err error) {
	a.post(func() { a.applySendProgress(token, "HTTP", sent, total, err) })
}

// applySendProgress updates the sender-side progress on the UI thread.
func (a *app) applySendProgress(token, transport string, sent, total int64, err error) {
	msgID := a.sendTokenMsg[token]
	if msgID == "" {
		return
	}
	if a.sends == nil {
		a.sends = map[string]*sendState{}
	}
	st := a.sends[msgID]
	if st == nil {
		st = &sendState{}
		a.sends[msgID] = st
	}
	now := time.Now()
	if err != nil {
		st.Sent, st.Total = sent, total
		st.Err = transferErrorText(err)
		st.Working = false
		st.Done = false
		st.Speed = 0
		st.Transport = transport
		st.lastAt = time.Time{}
		st.lastSent = sent
		a.redraw()
		return
	}
	resumed := st.Err != "" || (st.lastSent > 0 && sent < st.lastSent)
	if st.lastAt.IsZero() || resumed {
		st.lastAt, st.lastSent = now, sent
		st.Speed = 0
	} else if sent > st.lastSent {
		dt := now.Sub(st.lastAt).Seconds()
		if dt < 0.001 {
			// Two updates may arrive within one timer tick, especially on
			// Windows. A tiny floor keeps the first speed finite and above
			// zero instead of dividing by zero.
			dt = 0.001
		}
		inst := float64(sent-st.lastSent) / dt
		if st.Speed == 0 {
			st.Speed = inst
		} else {
			st.Speed = st.Speed*0.7 + inst*0.3
		}
		st.lastAt, st.lastSent = now, sent
	}
	st.Sent, st.Total = sent, total
	st.Err = ""
	st.Transport = transport
	st.Working = total > 0 && sent < total
	st.Done = sent >= total
	if st.Done {
		st.Working = false
		st.Speed = 0
	}
	a.redraw()
}

// transferErrorText keeps common network errors short and localised; a
// receiver that cancels is not an application failure.
func transferErrorText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return tr("对方已取消")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tr("连接已断开")
	}
	msg := err.Error()
	if strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF") || strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "deadline exceeded") {
		return tr("连接已断开")
	}
	return msg
}

// rebuildSendTokens maps both attachment tokens of outgoing messages back
// to their message, so progress from either transport reaches the card.
func (a *app) rebuildSendTokens() {
	if a.sendTokenMsg == nil {
		a.sendTokenMsg = map[string]string{}
	}
	for _, msgs := range a.msgs {
		for i := range msgs {
			m := &msgs[i]
			if !m.Mine || m.File == nil {
				continue
			}
			if m.File.Token != "" {
				a.sendTokenMsg[m.File.Token] = m.ID
			}
			if tok := shareToken(m.File.URL); tok != "" {
				a.sendTokenMsg[tok] = m.ID
			}
		}
	}
}

// restartTransfer recreates the server after its port setting changed.
func (a *app) restartTransfer() {
	if a.transfer != nil {
		a.transfer.Stop()
		a.transfer = nil
	}
	a.startTransfer()
}

func (a *app) stopTransfer() {
	if a.transfer != nil {
		a.transfer.Stop()
		a.transfer = nil
	}
}

// transferPort is the actual port peers are told about, or zero when the
// service is not running.
func (a *app) transferPort() int {
	if a.transfer == nil || !a.transfer.Running() {
		return 0
	}
	return a.transfer.Port()
}
