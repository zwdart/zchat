package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"xin.dart.zchat/internal/iconkit"
)

// iconPicks are the platform check boxes. They start all on so a first
// click on "开始生成" produces the complete set.
type iconPicks struct {
	Android bool
	IOS     bool
	Windows bool
	Linux   bool
	MacOS   bool
}

// iconRun is one generation run's progress and result. It is only changed
// on the main thread; the worker goroutine posts updates through app.post.
type iconRun struct {
	running  bool
	finished bool
	total    int
	done     int
	files    int
	current  string
	output   string
	started  time.Time
	elapsed  time.Duration
	log      []string
	errors   []string
	warnings []string
	canceled bool
}

var iconBackgroundPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

func (a *app) iconKitPage(c *ui.Context) {
	p := a.pal
	if !a.iconToolChecked && !a.iconToolChecking {
		a.detectIconTool()
	}
	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(16).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(16).DragWindow().Children(func() {
				if a.magick.cap == magickIconCap {
					if b := iconButton(c, iconBack, 17, p.muted, tr("app.icon.back_to_capabilities")); b.Clicked() {
						a.magickBack()
					}
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					sectionTitle(c, p, tr("app.common.installer_icons"), tr("app.icon.upload_one_image_and_generate_standard"))
				})
				if run := a.iconRun; run != nil && run.running {
					ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(10, 12).Radius(999).Background(p.accent.Alpha(0.13)).Children(func() {
						ui.Spinner(c).Size(14, 14)
						ui.Text(c, tr("app.icon.generating")).FontSize(11.5).FontWeight(600).TextColor(p.accent)
					})
				}
			})
			ui.Segmented(c, &a.iconTab, tr("app.common.generate"), tr("app.icon.platform_requirements")).Label(tr("app.icon.view")).Width(420)
			ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
				if a.iconTab == 1 {
					a.iconDocsTab(c, p)
					return
				}
				a.iconToolCard(c, p)
				a.iconSourceCard(c, p)
				a.iconOutputCard(c, p)
				a.iconPlatformsCard(c, p)
				a.iconActions(c, p)
				if run := a.iconRun; run != nil {
					a.iconRunCard(c, p, run)
				}
				a.iconUsageHint(c, p)
			})
		})
	})
}

func (a *app) iconToolCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			bg, fg := p.hover, p.muted
			title := tr("app.common.checking_imagemagick")
			desc := tr("app.icon.generating_icons_uses_local_imagemagick_installation")
			if a.iconToolChecked {
				if a.iconTool.Path != "" {
					bg, fg = ui.Oklch(0.72, 0.18, 145).Alpha(0.14), ui.Oklch(0.62, 0.17, 145)
					title = tr("app.common.imagemagick_is_ready")
					desc = a.iconTool.Version
				} else {
					bg, fg = ui.Oklch(0.78, 0.16, 80).Alpha(0.16), ui.Oklch(0.58, 0.15, 65)
					title = tr("app.common.imagemagick_not_found")
					desc = tr("app.common.install_imagemagick_first_then_click_check")
				}
			}
			ui.Box(c).Size(40, 40).Radius(12).Center().Background(bg).Children(func() {
				ui.Icon(c, iconPackage).Size(21, 21).TextColor(fg)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).FontSize(13.5).FontWeight(650)
					if a.iconToolChecking {
						ui.Spinner(c).Size(12, 12)
					}
				})
				ui.Text(c, desc).FontSize(11.5).TextColor(p.muted).SingleLine()
				if a.iconTool.Path != "" {
					ui.Text(c, a.iconTool.Path).FontSize(10.5).TextColor(p.faint).SingleLine()
				}
			})
			if pillButtonKey(c, "icon-tool-check", iconRefresh, tr("app.common.check_again"), p.hover.Over(p.bg), p.text).Clicked() {
				a.detectIconTool()
			}
		})
		if a.iconToolChecked && a.iconTool.Path == "" {
			ui.Column(c).FillWidth().Padding(12).Radius(10).Background(ui.Oklch(0.78, 0.16, 80).Alpha(0.1)).Gap(4).Children(func() {
				ui.Text(c, imageMagickHint()).FontSize(11.5).TextColor(p.text).Selectable()
				if a.iconToolErr != "" {
					ui.Text(c, a.iconToolErr).FontSize(10.5).TextColor(p.muted).Selectable().SingleLine()
				}
			})
		} else if a.iconToolChecked {
			ui.Text(c, tr("app.icon.check_passed_imagemagick_is_checked_again")).FontSize(11).TextColor(p.faint)
		}
	})
}

func (a *app) iconSourceCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, tr("app.icon.source_image")).FontSize(13).FontWeight(650)
			ui.Text(c, tr("app.icon.square_png_of_at_least_1024")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			if pillButtonKey(c, "icon-pick-source", iconUpload, tr("app.common.choose_image"), p.hover.Over(p.bg), p.text).Clicked() {
				a.pickIconSource()
			}
		})
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			preview := ui.Box(c).Size(76, 76).Radius(14).Background(p.hover).Center()
			if bmp := a.loadImage(a.iconSource); bmp != nil {
				preview.Children(func() { ui.Image(c, bmp).Size(76, 76).Fit(ui.Contain).Radius(13) })
			} else {
				preview.Children(func() {
					ui.Icon(c, iconImage).Size(28, 28).TextColor(p.faint)
				})
			}
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				if a.iconSource == "" {
					ui.Text(c, tr("app.common.no_image_selected_yet")).FontSize(12.5).FontWeight(600)
					ui.Text(c, tr("app.icon.supports_png_jpg_webp_and_gif")).FontSize(11).TextColor(p.muted).LineHeight(1.45)
					return
				}
				ui.Text(c, filepath.Base(a.iconSource)).FontSize(12.5).FontWeight(600).SingleLine()
				ui.Text(c, a.iconSource).FontSize(10.5).TextColor(p.faint).SingleLine().Selectable()
				if a.iconSourceW > 0 && a.iconSourceH > 0 {
					ui.Text(c, fmt.Sprintf("%d × %d", a.iconSourceW, a.iconSourceH)).FontSize(11).TextColor(p.muted)
				}
			})
		})
		if a.iconSource != "" {
			if _, err := os.Stat(a.iconSource); err != nil {
				a.iconWarning(c, p, tr("app.icon.source_image_is_missing_or_was_2"))
			} else {
				if a.iconSourceW > 0 && a.iconSourceW != a.iconSourceH {
					a.iconNote(c, p, tr("app.icon.image_is_not_square_it_is"))
				}
				if a.iconSourceW > 0 && min(a.iconSourceW, a.iconSourceH) < 1024 {
					a.iconNote(c, p, tr("app.icon.short_side_is_below_1024_pixels"))
				}
			}
		}
	})
}

func (a *app) iconOutputCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Text(c, tr("app.icon.output_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.icon.app_name"))
				ui.TextInput(c.Key("icon-name"), &a.settings.IconName).Placeholder("zchat")
			})
			ui.Column(c).Width(170).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.icon.opaque_background_ios"))
				ui.TextInput(c.Key("icon-bg"), &a.settings.IconBackground).Placeholder("#ffffff")
				if !iconBackgroundPattern.MatchString(strings.TrimSpace(a.settings.IconBackground)) {
					ui.Text(c, tr("app.icon.use_rgb_or_rrggbb")).FontSize(10.5).TextColor(ui.Oklch(0.64, 0.2, 28))
				}
			})
		})
		ui.Column(c).Gap(5).Children(func() {
			fieldLabel(c, p, tr("app.common.output_folder"))
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c.Key("icon-output"), &a.settings.IconOutputDir).Grow(1).Placeholder(tr("app.common.choose_output_folder_2"))
				if ui.Button(c.Key("icon-pick-output"), tr("app.common.select")).Clicked() {
					a.pickIconOutput()
				}
			})
			ui.Text(c, tr("app.icon.creates_android_ios_windows_linux_and")).FontSize(10.5).TextColor(p.faint).LineHeight(1.4)
		})
	})
}

func (a *app) iconPlatformsCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, tr("app.icon.target_platforms")).FontSize(13).FontWeight(650)
			ui.Text(c, tr("app.icon.multiple_choices")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			if ui.Button(c.Key("icon-pick-all"), tr("app.common.select_all")).Clicked() {
				a.iconPicks = iconPicks{Android: true, IOS: true, Windows: true, Linux: true, MacOS: true}
			}
			if ui.Button(c.Key("icon-pick-none"), tr("app.common.select_none")).Clicked() {
				a.iconPicks = iconPicks{}
			}
		})
		for _, platform := range iconkit.Platforms {
			platform := platform
			on := a.iconPicked(platform.Key)
			hover := p.hover
			row := ui.CheckboxBase(c.Key("icon-platform-"+platform.Key), on)
			row.FillWidth().Padding(12, 12).Radius(12).Gap(12).AlignItems(ui.Center)
			row.Border(1, p.hair)
			if *on {
				row.Background(p.accent.Alpha(0.07))
			} else if row.Hovered() {
				row.Background(hover)
			}
			row.Children(func() {
				box := ui.Box(c).Size(19, 19).Radius(5).Center().Border(1.5, p.faint)
				if *on {
					box.Background(p.accent).Border(1.5, p.accent).Children(func() {
						ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.onAcc)
					})
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, platform.Name).FontSize(12.5).FontWeight(650)
						ui.Text(c, fmt.Sprintf(tr("app.common.files_2"), platform.FileCount())).FontSize(10.5).TextColor(p.faint)
					})
					ui.Text(c, tr(platform.Summary)).FontSize(11).TextColor(p.muted).LineHeight(1.35)
				})
			})
		}
	})
}

func (a *app) iconActions(c *ui.Context, p palette) {
	running := a.iconRunning()
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		if running {
			if pillButtonKey(c, "icon-cancel", iconPause, tr("app.icon.cancel_generation"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelIconGeneration()
			}
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, tr("app.icon.generating_files_already_written_are_kept")).FontSize(11.5).TextColor(p.muted)
			return
		}
		label := tr("app.icon.generate")
		if run := a.iconRun; run != nil && run.finished {
			label = tr("app.icon.generate_again")
		}
		if pillButtonKey(c, "icon-generate", iconPackage, label, p.accent, p.onAcc).Clicked() {
			a.startIconGeneration()
		}
		ui.Text(c, tr("app.icon.imagemagick_source_image_and_output_folder")).FontSize(11).TextColor(p.faint)
	})
}

func (a *app) iconRunCard(c *ui.Context, p palette, run *iconRun) {
	status, color, icon := tr("app.icon.generating_2"), p.accent, iconPackage
	switch {
	case run.canceled:
		status, color, icon = tr("app.common.canceled"), ui.Oklch(0.78, 0.16, 80), iconPause
	case run.finished && len(run.errors) > 0 && run.files > 0:
		status, color, icon = tr("app.common.partially_complete"), ui.Oklch(0.78, 0.16, 80), iconInfo
	case run.finished && len(run.errors) > 0:
		status, color, icon = tr("app.icon.generation_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
	case run.finished && run.files > 0:
		status, color, icon = tr("app.icon.generation_complete"), ui.Oklch(0.68, 0.17, 145), iconCheck
	case run.finished:
		status, color, icon = tr("app.icon.generation_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
	}
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(38, 38).Radius(11).Center().Background(color.Alpha(0.14)).Children(func() {
				ui.Icon(c, icon).Size(20, 20).TextColor(color)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, status).FontSize(14).FontWeight(700).TextColor(color)
					if run.running {
						ui.Spinner(c).Size(12, 12)
					}
				})
				ui.Text(c, run.current).FontSize(11).TextColor(p.muted).SingleLine()
			})
			if run.output != "" {
				if _, err := os.Stat(run.output); err == nil {
					if pillButtonKey(c, "icon-open-output", iconFolder, tr("app.common.open_output_folder"), p.hover.Over(p.bg), p.text).Clicked() {
						a.openPath(run.output)
					}
				}
			}
		})
		ratio := float64(0)
		if run.total > 0 {
			ratio = float64(run.done) / float64(run.total)
		}
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		ui.Progress(c, ratio).Height(6).Radius(3)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d / %d", run.done, run.total)).FontSize(11).TextColor(p.muted)
			if run.files > 0 {
				ui.Text(c, fmt.Sprintf(tr("app.common.wrote_files"), run.files)).FontSize(11).TextColor(p.muted)
			}
			if run.elapsed > 0 {
				ui.Text(c, tr("app.common.time")+" "+formatRunDuration(run.elapsed)).FontSize(11).TextColor(p.muted)
			}
			if run.output != "" {
				ui.Text(c, run.output).FontSize(10.5).TextColor(p.faint).SingleLine().Grow(1).Shrink(1).MinWidth(0).Selectable()
			}
		})
		for _, warning := range run.warnings {
			a.iconNote(c, p, warning)
		}
		if len(run.errors) > 0 {
			ui.Column(c).FillWidth().Padding(10, 12).Radius(10).Background(ui.Oklch(0.64, 0.2, 28).Alpha(0.09)).Gap(4).Children(func() {
				ui.Text(c, fmt.Sprintf(tr("app.common.files_failed_2"), len(run.errors))).FontSize(11.5).FontWeight(650)
				max := len(run.errors)
				if max > 6 {
					max = 6
				}
				for _, msg := range run.errors[:max] {
					ui.Text(c, "• "+msg).FontSize(10.5).TextColor(p.muted).Selectable()
				}
				if len(run.errors) > max {
					ui.Text(c, fmt.Sprintf(tr("app.common.more_errors_see_log"), len(run.errors)-max)).FontSize(10.5).TextColor(p.faint)
				}
			})
		}
		if len(run.log) > 0 {
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				ui.Text(c, tr("app.icon.generation_log")).FontSize(11.5).FontWeight(650).TextColor(p.muted)
				lines := run.log
				if len(lines) > 10 {
					lines = lines[len(lines)-10:]
				}
				for _, line := range lines {
					ui.Text(c, line).FontSize(10.5).TextColor(p.muted).SingleLine().Selectable()
				}
			})
		}
		if run.finished {
			ui.Text(c, iconDoneHint(run)).FontSize(11).TextColor(p.faint)
		}
	})
}

func (a *app) iconUsageHint(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconInfo).Size(16, 16).TextColor(p.faint)
		ui.Text(c, tr("app.icon.tip_generated_folders_can_be_merged")).FontSize(11).TextColor(p.muted).LineHeight(1.5)
	})
}

func (a *app) iconDocsTab(c *ui.Context, p palette) {
	if a.iconDocOpts.OnLink == nil {
		a.iconDocOpts = markdown.Options{OnLink: a.openExternal, WrapCode: true}
	}
	ui.Column(c).FillWidth().Padding(14, 16).Radius(14).Background(p.panel).Border(1, p.hair).Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, tr("app.icon.official_sizes_and_formats")).FontSize(15).FontWeight(700)
				ui.Text(c, tr("app.icon.table_follows_each_platform_s_current")).FontSize(11.5).TextColor(p.muted).LineHeight(1.5)
				ui.Text(c, tr("app.icon.imagemagick_handles_resizing_and_format_conversion")).FontSize(11).TextColor(p.faint)
			})
			if ui.Button(c.Key("icon-copy-all-docs"), tr("app.icon.copy_all_documentation")).Clicked() {
				c.WriteClipboard(iconkit.AllMarkdown())
				a.setToast(tr("app.icon.copied_all_platform_documentation"))
			}
		})
	})
	for _, platform := range iconkit.Platforms {
		platform := platform
		ui.Column(c).FillWidth().Padding(16, 16).Radius(14).Background(p.panel).Border(1, p.hair).Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(11).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(38, 38).Radius(11).Center().Background(p.accent.Alpha(0.12)).Children(func() {
					ui.Icon(c, iconPackage).Size(20, 20).TextColor(p.accent)
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, platform.Name).FontSize(14).FontWeight(700)
					ui.Text(c, tr(platform.Summary)).FontSize(11).TextColor(p.muted).LineHeight(1.4)
				})
				ui.Box(c).Padding(7, 5).Radius(999).Background(p.hover).Children(func() {
					ui.Text(c, fmt.Sprintf(tr("app.common.files_2"), platform.FileCount())).FontSize(10.5).TextColor(p.muted)
				})
				if ui.Button(c.Key("icon-copy-doc-"+platform.Key), tr("app.icon.copy_documentation")).Clicked() {
					c.WriteClipboard(platform.Markdown())
					a.setToast(platform.Name + tr("app.icon.documentation_copied"))
				}
			})
			markdown.View(c, platform.Markdown(), &a.iconDocOpts).FillWidth()
		})
	}
}

func (a *app) iconCard(c *ui.Context, p palette, fn func()) {
	ui.Column(c).FillWidth().Padding(16, 15).Radius(14).Background(p.panel).Border(1, p.hair).Gap(12).Children(fn)
}

func (a *app) iconWarning(c *ui.Context, p palette, text string) {
	ui.Row(c).FillWidth().Padding(9, 10).Radius(9).Background(ui.Oklch(0.64, 0.2, 28).Alpha(0.1)).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconInfo).Size(14, 14).TextColor(ui.Oklch(0.64, 0.2, 28))
		ui.Text(c, text).FontSize(10.5).TextColor(p.text).LineHeight(1.4)
	})
}

func (a *app) iconNote(c *ui.Context, p palette, text string) {
	ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconInfo).Size(12, 12).TextColor(p.faint).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(10.5).TextColor(p.muted).LineHeight(1.45).Grow(1).Shrink(1).MinWidth(0)
	})
}

func (a *app) iconPicked(key string) *bool {
	switch key {
	case "android":
		return &a.iconPicks.Android
	case "ios":
		return &a.iconPicks.IOS
	case "windows":
		return &a.iconPicks.Windows
	case "linux":
		return &a.iconPicks.Linux
	case "macos":
		return &a.iconPicks.MacOS
	}
	return nil
}

func (a *app) iconSelectedKeys() []string {
	var keys []string
	for _, key := range iconkit.AllKeys() {
		if on := a.iconPicked(key); on != nil && *on {
			keys = append(keys, key)
		}
	}
	return keys
}

func (a *app) iconRunning() bool {
	return a.iconRun != nil && a.iconRun.running
}

func (a *app) detectIconTool() {
	if a.iconToolChecking {
		return
	}
	a.iconToolChecking = true
	a.redraw()
	go func() {
		tool, err := iconkit.Detect()
		a.post(func() {
			a.iconToolChecking = false
			a.iconToolChecked = true
			a.iconTool = tool
			if err != nil {
				a.iconToolErr = err.Error()
			} else {
				a.iconToolErr = ""
			}
		})
	}()
}

func (a *app) pickIconSource() {
	if a.iconRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("app.icon.choose_source_image_2"),
			ButtonLabel: tr("app.common.use_this_image"),
			Message:     tr("app.icon.choose_png_of_at_least_1024"),
			Filters: []mygo.FileFilter{{
				Name:       tr("app.common.images"),
				Extensions: []string{"png", "jpg", "jpeg", "webp", "gif"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		w, h := imageSize(path)
		a.post(func() {
			if w == 0 || h == 0 {
				a.setToast(tr("app.icon.cannot_read_this_image_try_another"))
				return
			}
			a.iconSource = path
			a.iconSourceW = w
			a.iconSourceH = h
			_ = a.loadImage(path)
		})
	}()
}

func (a *app) pickIconOutput() {
	if a.iconRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            a.win,
			Title:             tr("app.common.choose_output_folder_3"),
			ButtonLabel:       tr("app.common.use_this_folder"),
			Directory:         true,
			CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.settings.IconOutputDir = paths[0]
			_ = a.settings.save()
		})
	}()
}

func (a *app) startIconGeneration() {
	if a.iconRunning() {
		return
	}
	src := strings.TrimSpace(a.iconSource)
	if src == "" {
		a.setToast(tr("app.icon.choose_source_image_first"))
		return
	}
	if info, err := os.Stat(src); err != nil || info.IsDir() {
		a.setToast(tr("app.icon.source_image_is_missing_or_was"))
		return
	}
	keys := a.iconSelectedKeys()
	if len(keys) == 0 {
		a.setToast(tr("app.icon.choose_at_least_one_platform"))
		return
	}
	out := strings.TrimSpace(a.settings.IconOutputDir)
	if out == "" {
		a.setToast(tr("app.common.choose_output_folder"))
		return
	}
	bg := strings.TrimSpace(a.settings.IconBackground)
	if !iconBackgroundPattern.MatchString(bg) {
		a.setToast(tr("app.icon.use_rrggbb_for_background_color"))
		return
	}
	name := iconkit.SafeName(a.settings.IconName)
	a.settings.IconName = name
	_ = a.settings.save()

	run := &iconRun{
		running: true,
		current: tr("app.common.checking_imagemagick"),
		output:  out,
		started: time.Now(),
	}
	a.iconRun = run
	ctx, cancel := context.WithCancel(context.Background())
	a.iconCancel = cancel
	a.redraw()

	go func() {
		tool, err := iconkit.Detect()
		if err != nil {
			a.post(func() {
				if a.iconRun != run {
					return
				}
				a.iconTool = iconkit.Tool{}
				a.iconToolErr = err.Error()
				a.iconToolChecked = true
				a.finishIconRun(run, iconkit.Result{}, tr("app.common.imagemagick_not_found_2")+err.Error())
			})
			return
		}
		a.post(func() {
			if a.iconRun == run {
				a.iconTool = tool
				a.iconToolErr = ""
				a.iconToolChecked = true
			}
		})
		plan, err := iconkit.Build(tool, iconkit.Options{
			Input:      src,
			OutputRoot: out,
			Name:       name,
			Background: bg,
			Platforms:  keys,
		})
		if err != nil {
			a.post(func() {
				if a.iconRun == run {
					a.finishIconRun(run, iconkit.Result{}, err.Error())
				}
			})
			return
		}
		a.post(func() {
			if a.iconRun == run {
				run.total = len(plan.Steps)
				run.current = tr("app.icon.creating_folders_and_writing_icons")
			}
		})
		res := plan.Run(ctx, func(ev iconkit.Progress) {
			a.post(func() {
				if a.iconRun == run {
					a.applyIconProgress(run, ev)
				}
			})
		})
		a.post(func() {
			if a.iconRun == run {
				a.finishIconRun(run, res, "")
			}
		})
	}()
}

func (a *app) applyIconProgress(run *iconRun, ev iconkit.Progress) {
	if ev.Total > 0 {
		run.total = ev.Total
	}
	if ev.Label != "" {
		run.current = ev.Label
	}
	if ev.Done > run.done {
		line := "✓ " + ev.Label
		if ev.Err != nil {
			line = "✗ " + ev.Label + " — " + ev.Err.Error()
		}
		run.log = appendTail(run.log, line, 200)
		run.done = ev.Done
	}
}

func (a *app) finishIconRun(run *iconRun, res iconkit.Result, fatal string) {
	run.running = false
	run.finished = true
	run.elapsed = time.Since(run.started)
	run.files = res.Files
	run.errors = res.Errors
	run.warnings = res.Warnings
	run.canceled = res.Canceled
	if fatal != "" {
		run.errors = append(run.errors, fatal)
		run.current = fatal
		a.setToast(fatal)
	} else if res.Canceled {
		run.current = tr("app.icon.generation_canceled_files_already_written_are")
		a.setToast(tr("app.icon.generation_canceled_files_already_written_are"))
	} else if len(res.Errors) > 0 {
		run.current = tr("app.icon.some_files_failed")
		a.setToast(fmt.Sprintf(tr("app.icon.generated_files_failed"), res.Files, len(res.Errors)))
	} else if res.Files == 0 {
		run.current = tr("app.common.no_files_were_written")
		a.setToast(tr("app.icon.generation_failed_no_files_were_written"))
	} else {
		run.current = tr("app.icon.all_files_generated")
		a.setToast(fmt.Sprintf(tr("app.icon.generated_files_in"), res.Files, formatRunDuration(res.Duration)))
	}
	if a.iconCancel != nil {
		a.iconCancel()
		a.iconCancel = nil
	}
	a.redraw()
}

func (a *app) cancelIconGeneration() {
	if a.iconCancel != nil {
		a.iconCancel()
	}
	if run := a.iconRun; run != nil {
		run.current = tr("app.common.canceling")
	}
	a.redraw()
}

func iconDoneHint(run *iconRun) string {
	if run.canceled {
		return tr("app.icon.generation_canceled_successfully_written_files_stay")
	}
	if len(run.errors) > 0 {
		return tr("app.icon.successfully_generated_files_are_kept_fix")
	}
	if run.files > 0 {
		return tr("app.icon.done_platform_folders_in_output_directory")
	}
	return tr("app.icon.no_files_were_generated_check_imagemagick")
}

func formatRunDuration(d time.Duration) string {
	if d <= 0 {
		return "0 " + tr("app.common.s")
	}
	if d < time.Second {
		return fmt.Sprintf("%d %s", d.Milliseconds(), tr("app.common.ms"))
	}
	return fmt.Sprintf("%.1f %s", d.Seconds(), tr("app.common.s"))
}

func imageMagickHint() string {
	switch runtime.GOOS {
	case "windows":
		return tr("app.icon.install_winget_install_imagemagick_imagemagick_or")
	case "darwin":
		return tr("app.icon.install_brew_install_imagemagick")
	default:
		return tr("app.icon.install_sudo_apt_install_imagemagick_or")
	}
}

func appendTail(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}
