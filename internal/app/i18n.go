package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/model"
)

func tr(s string) string {
	return i18n.T(s)
}

// setLocale tells MyGo which language native windows and menus should use.
func setLocale() {
	if i18n.Current() == i18n.English {
		mygo.App.SetLocale("en")
	} else {
		mygo.App.SetLocale("zh-Hans")
	}
}

func tf(format string, args ...any) string {
	return i18n.TF(format, args...)
}

// applyLanguage switches the UI language and refreshes menus and windows.
func (a *app) applyLanguage() {
	i18n.Set(a.settings.Language)
	setLocale()
	a.setMenu()
	if a.settingsWin != nil {
		a.settingsWin.SetTitle(tr("app.common.zchat_settings"))
	}
	if a.tray != nil {
		a.applyTray()
	}
	a.redraw()
}

// presenceLabel translates the stable status labels kept in the model.
func presenceLabel(p model.Presence) string {
	return tr(p.Label())
}

// dayLabel names a message-list day in the current language.
func dayLabel(t, now time.Time) string {
	if model.SameDay(t, now) {
		return tr("app.common.today")
	}
	if model.SameDay(t, now.AddDate(0, 0, -1)) {
		return tr("app.common.yesterday")
	}
	if t.Year() == now.Year() {
		return t.Format(tr("app.common.date_format_md"))
	}
	return t.Format(tr("app.common.date_format_ymd"))
}

// messagePreview is model.Message.Preview with localized placeholders.
func messagePreview(m *model.Message) string {
	if m == nil {
		return ""
	}
	if m.Recalled {
		return tr("app.common.message_unsent")
	}
	switch m.Kind {
	case model.KindImage:
		if strings.TrimSpace(m.Text) != "" {
			return tr("app.common.image_2") + m.Text
		}
		return tr("app.common.image")
	case model.KindFile:
		if m.File != nil && m.File.Name != "" {
			return tr("app.common.file_2") + m.File.Name
		}
		return tr("app.common.file")
	case model.KindSystem:
		return m.Text
	default:
		s := strings.Join(strings.Fields(m.Text), " ")
		if len([]rune(s)) > 80 {
			s = string([]rune(s)[:80]) + "…"
		}
		return s
	}
}
