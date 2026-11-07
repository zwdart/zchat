package app

import (
	"context"
	"errors"
	"testing"

	"xin.dart.zchat/internal/i18n"
)

// Sender-side progress moves from sending to sent, reports a speed, and
// can resume after an interrupted transfer.
func TestApplySendProgress(t *testing.T) {
	a := &app{
		sends: map[string]*sendState{},
		sendTokenMsg: map[string]string{
			"t_1":     "m_1",
			"t_empty": "m_empty",
		},
	}

	a.applySendProgress("t_1", "TCP", 0, 100, nil)
	st := a.sends["m_1"]
	if st == nil || !st.Working || st.Done || st.Err != "" || st.Sent != 0 || st.Total != 100 || st.Transport != "TCP" {
		t.Fatalf("initial state = %+v", st)
	}

	a.applySendProgress("t_1", "TCP", 50, 100, nil)
	if !st.Working || st.Done || st.Sent != 50 || st.Speed <= 0 {
		t.Fatalf("mid state = %+v", st)
	}

	a.applySendProgress("t_1", "HTTP", 100, 100, nil)
	if !st.Done || st.Working || st.Speed != 0 || st.Sent != 100 || st.Transport != "HTTP" {
		t.Fatalf("done state = %+v", st)
	}

	a.applySendProgress("t_1", "TCP", 40, 100, errors.New("boom"))
	if st.Err != "boom" || st.Working || st.Done || st.Sent != 40 || st.Transport != "TCP" {
		t.Fatalf("error state = %+v", st)
	}

	a.applySendProgress("t_1", "TCP", 40, 100, nil)
	if st.Err != "" || !st.Working || st.Done || st.Sent != 40 {
		t.Fatalf("resumed state = %+v", st)
	}

	a.applySendProgress("t_empty", "TCP", 0, 0, nil)
	if empty := a.sends["m_empty"]; empty == nil || !empty.Done || empty.Working {
		t.Fatalf("empty state = %+v", empty)
	}

	a.applySendProgress("unknown", "TCP", 1, 2, nil)
	if len(a.sends) != 2 {
		t.Fatalf("unknown token made state: %+v", a.sends)
	}
}

func TestTransferErrorText(t *testing.T) {
	old := i18n.Current()
	i18n.Set(i18n.English)
	defer i18n.Set(old)
	if got := transferErrorText(context.Canceled); got != "The other side cancelled" {
		t.Fatalf("cancel text = %q", got)
	}
	if got := transferErrorText(errors.New("write tcp: broken pipe")); got != "Connection lost" {
		t.Fatalf("broken pipe text = %q", got)
	}
	if got := transferErrorText(context.DeadlineExceeded); got != "Connection lost" {
		t.Fatalf("deadline text = %q", got)
	}
	if got := transferErrorText(errors.New("boom")); got != "boom" {
		t.Fatalf("unknown text = %q", got)
	}
}
