package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/model"
)

// palette holds the app's colors for the light and dark appearance.
type palette struct {
	dark    bool
	bg      ui.Color
	sidebar ui.Color
	panel   ui.Color
	text    ui.Color
	muted   ui.Color
	faint   ui.Color
	hair    ui.Color
	hover   ui.Color
	press   ui.Color
	accent  ui.Color
	onAcc   ui.Color
	mine    ui.Color
	onMine  ui.Color
	other   ui.Color
	shadow  ui.Color
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			dark:    true,
			bg:      ui.Hex("#141416"),
			sidebar: ui.Hex("#1b1b1f"),
			panel:   ui.Hex("#232327"),
			text:    ui.Hex("#f5f5f7"),
			muted:   ui.RGBA(235, 235, 245, 0.58),
			faint:   ui.RGBA(235, 235, 245, 0.3),
			hair:    ui.RGBA(255, 255, 255, 0.08),
			hover:   ui.RGBA(255, 255, 255, 0.07),
			press:   ui.RGBA(255, 255, 255, 0.12),
			accent:  ui.Oklch(0.68, 0.18, 245),
			onAcc:   ui.Hex("#ffffff"),
			mine:    ui.Oklch(0.58, 0.17, 248),
			onMine:  ui.Hex("#ffffff"),
			other:   ui.Hex("#26262b"),
			shadow:  ui.RGBA(0, 0, 0, 0.5),
		}
	}
	return palette{
		bg:      ui.Hex("#ffffff"),
		sidebar: ui.Hex("#f3f3f7"),
		panel:   ui.Hex("#f7f7fa"),
		text:    ui.Hex("#1d1d1f"),
		muted:   ui.RGBA(60, 60, 67, 0.62),
		faint:   ui.RGBA(60, 60, 67, 0.32),
		hair:    ui.RGBA(0, 0, 0, 0.08),
		hover:   ui.RGBA(0, 0, 0, 0.045),
		press:   ui.RGBA(0, 0, 0, 0.09),
		accent:  ui.Oklch(0.58, 0.19, 250),
		onAcc:   ui.Hex("#ffffff"),
		mine:    ui.Oklch(0.55, 0.19, 252),
		onMine:  ui.Hex("#ffffff"),
		other:   ui.Hex("#f1f1f5"),
		shadow:  ui.RGBA(0, 0, 0, 0.16),
	}
}

// statusColor is the dot beside a peer.
func statusColor(p model.Presence) ui.Color {
	switch p {
	case model.Online:
		return ui.Oklch(0.72, 0.18, 145)
	case model.Away:
		return ui.Oklch(0.78, 0.16, 80)
	case model.Busy, model.DND:
		return ui.Oklch(0.64, 0.2, 28)
	default:
		return ui.RGBA(128, 128, 128, 0.45)
	}
}

// iconButton is a borderless round button with an icon.
func iconButton(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(size+14, size+14).Radius(999).Center().Label(label).TextColor(color).FocusRing(false)
	bg := ui.Transparent
	if b.Hovered() {
		bg = ui.RGBA(128, 128, 128, 0.14)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.88
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

// pillButton is a rounded button with an icon and a label.
func pillButton(c *ui.Context, icon *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c).Height(34).PaddingX(16).Gap(7).Radius(999).AlignItems(ui.Center).TextColor(fg).FocusRing(false)
	fill := bg
	if b.Hovered() {
		fill = bg.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(b.AnimateColor("bg", fill, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.97
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).FontSize(13.5).FontWeight(600)
	})
	return b
}

// pillButtonKey is pillButton with a stable key for dialogs.
func pillButtonKey(c *ui.Context, key any, icon *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	return pillButton(c, icon, label, bg, fg).Key(key)
}

// dot is a small presence indicator.
func dot(c *ui.Context, p model.Presence, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Radius(999).Background(statusColor(p))
	if p == model.Offline || p == model.Invisible {
		box.Border(1.5, ui.RGBA(128, 128, 128, 0.45)).Background(ui.Transparent)
	}
	return box
}

// fieldLabel is a small heading above a settings control.
func fieldLabel(c *ui.Context, p palette, text string) {
	ui.Text(c, text).FontSize(12).FontWeight(600).TextColor(p.muted)
}

// sectionTitle is a heading in a page.
func sectionTitle(c *ui.Context, p palette, title, sub string) {
	ui.Column(c).Gap(3).Children(func() {
		ui.Text(c, title).FontSize(22).FontWeight(700).LetterSpacing(-0.3)
		if sub != "" {
			ui.Text(c, sub).FontSize(12.5).TextColor(p.muted)
		}
	})
}

// typingLabel is what a chat header says while a peer types.
func typingLabel(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " 正在输入…"
	case 2:
		return names[0] + "、" + names[1] + " 正在输入…"
	default:
		return "多人正在输入…"
	}
}
