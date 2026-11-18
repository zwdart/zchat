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
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					sectionTitle(c, p, tr("安装包图标"), tr("上传一张图片，一键生成 Android、iOS、Windows、Linux、macOS 的标准图标与目录"))
				})
				if run := a.iconRun; run != nil && run.running {
					ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(10, 12).Radius(999).Background(p.accent.Alpha(0.13)).Children(func() {
						ui.Spinner(c).Size(14, 14)
						ui.Text(c, tr("正在生成…")).FontSize(11.5).FontWeight(600).TextColor(p.accent)
					})
				}
			})
			ui.Segmented(c, &a.iconTab, tr("生成"), tr("平台要求")).Label(tr("视图")).Width(420)
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
			title := tr("正在检测 ImageMagick…")
			desc := tr("生成图标需要调用本机的 ImageMagick。")
			if a.iconToolChecked {
				if a.iconTool.Path != "" {
					bg, fg = ui.Oklch(0.72, 0.18, 145).Alpha(0.14), ui.Oklch(0.62, 0.17, 145)
					title = tr("ImageMagick 已就绪")
					desc = a.iconTool.Version
				} else {
					bg, fg = ui.Oklch(0.78, 0.16, 80).Alpha(0.16), ui.Oklch(0.58, 0.15, 65)
					title = tr("未找到 ImageMagick")
					desc = tr("请先安装 ImageMagick，再点击「重新检测」。")
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
			if pillButtonKey(c, "icon-tool-check", iconRefresh, tr("重新检测"), p.hover.Over(p.bg), p.text).Clicked() {
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
			ui.Text(c, tr("检测通过。开始生成前会再次确认 ImageMagick 仍可用，避免生成到一半才失败。")).FontSize(11).TextColor(p.faint)
		}
	})
}

func (a *app) iconSourceCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, tr("源图片")).FontSize(13).FontWeight(650)
			ui.Text(c, tr("建议 1024 × 1024 以上的正方形 PNG")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			if pillButtonKey(c, "icon-pick-source", iconUpload, tr("选择图片"), p.hover.Over(p.bg), p.text).Clicked() {
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
					ui.Text(c, tr("还没有选择图片")).FontSize(12.5).FontWeight(600)
					ui.Text(c, tr("支持 PNG、JPG、WebP、GIF。透明区域会在 iOS 图标中填充为你选择的背景色。")).FontSize(11).TextColor(p.muted).LineHeight(1.45)
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
				a.iconWarning(c, p, tr("源图片不存在或已被移动，请重新选择。"))
			} else {
				if a.iconSourceW > 0 && a.iconSourceW != a.iconSourceH {
					a.iconNote(c, p, tr("图片不是正方形：会按比例缩放，四周按平台要求填充透明或背景色。"))
				}
				if a.iconSourceW > 0 && min(a.iconSourceW, a.iconSourceH) < 1024 {
					a.iconNote(c, p, tr("源图短边小于 1024 像素，生成 1024 图标时可能变模糊，建议换更大的原图。"))
				}
			}
		}
	})
}

func (a *app) iconOutputCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Text(c, tr("输出设置")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
				fieldLabel(c, p, tr("应用名称"))
				ui.TextInput(c.Key("icon-name"), &a.settings.IconName).Placeholder("zchat")
			})
			ui.Column(c).Width(170).Gap(5).Children(func() {
				fieldLabel(c, p, tr("不透明背景色（iOS）"))
				ui.TextInput(c.Key("icon-bg"), &a.settings.IconBackground).Placeholder("#ffffff")
				if !iconBackgroundPattern.MatchString(strings.TrimSpace(a.settings.IconBackground)) {
					ui.Text(c, tr("请使用 #RGB 或 #RRGGBB")).FontSize(10.5).TextColor(ui.Oklch(0.64, 0.2, 28))
				}
			})
		})
		ui.Column(c).Gap(5).Children(func() {
			fieldLabel(c, p, tr("输出目录"))
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c.Key("icon-output"), &a.settings.IconOutputDir).Grow(1).Placeholder(tr("选择一个输出目录"))
				if ui.Button(c.Key("icon-pick-output"), tr("选择…")).Clicked() {
					a.pickIconOutput()
				}
			})
			ui.Text(c, tr("会按平台自动创建 android、ios、windows、linux、macos 子目录。已有文件会被覆盖，说明文件和图标会一起写出。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.4)
		})
	})
}

func (a *app) iconPlatformsCard(c *ui.Context, p palette) {
	a.iconCard(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, tr("目标平台")).FontSize(13).FontWeight(650)
			ui.Text(c, tr("可多选")).FontSize(11).TextColor(p.faint)
			ui.Spacer(c)
			if ui.Button(c.Key("icon-pick-all"), tr("全选")).Clicked() {
				a.iconPicks = iconPicks{Android: true, IOS: true, Windows: true, Linux: true, MacOS: true}
			}
			if ui.Button(c.Key("icon-pick-none"), tr("全不选")).Clicked() {
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
						ui.Text(c, fmt.Sprintf(tr("%d 个文件"), platform.FileCount())).FontSize(10.5).TextColor(p.faint)
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
			if pillButtonKey(c, "icon-cancel", iconPause, tr("取消生成"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelIconGeneration()
			}
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, tr("正在生成，已写入的文件会保留")).FontSize(11.5).TextColor(p.muted)
			return
		}
		label := tr("开始生成")
		if run := a.iconRun; run != nil && run.finished {
			label = tr("再次生成")
		}
		if pillButtonKey(c, "icon-generate", iconPackage, label, p.accent, p.onAcc).Clicked() {
			a.startIconGeneration()
		}
		ui.Text(c, tr("生成前会自动检查 ImageMagick、源图片和输出目录。")).FontSize(11).TextColor(p.faint)
	})
}

func (a *app) iconRunCard(c *ui.Context, p palette, run *iconRun) {
	status, color, icon := tr("生成中"), p.accent, iconPackage
	switch {
	case run.canceled:
		status, color, icon = tr("已取消"), ui.Oklch(0.78, 0.16, 80), iconPause
	case run.finished && len(run.errors) > 0 && run.files > 0:
		status, color, icon = tr("部分完成"), ui.Oklch(0.78, 0.16, 80), iconInfo
	case run.finished && len(run.errors) > 0:
		status, color, icon = tr("生成失败"), ui.Oklch(0.64, 0.2, 28), iconInfo
	case run.finished && run.files > 0:
		status, color, icon = tr("生成完成"), ui.Oklch(0.68, 0.17, 145), iconCheck
	case run.finished:
		status, color, icon = tr("生成失败"), ui.Oklch(0.64, 0.2, 28), iconInfo
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
					if pillButtonKey(c, "icon-open-output", iconFolder, tr("打开输出目录"), p.hover.Over(p.bg), p.text).Clicked() {
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
				ui.Text(c, fmt.Sprintf(tr("已写出 %d 个文件"), run.files)).FontSize(11).TextColor(p.muted)
			}
			if run.elapsed > 0 {
				ui.Text(c, tr("用时")+" "+formatRunDuration(run.elapsed)).FontSize(11).TextColor(p.muted)
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
				ui.Text(c, fmt.Sprintf(tr("有 %d 个文件失败"), len(run.errors))).FontSize(11.5).FontWeight(650)
				max := len(run.errors)
				if max > 6 {
					max = 6
				}
				for _, msg := range run.errors[:max] {
					ui.Text(c, "• "+msg).FontSize(10.5).TextColor(p.muted).Selectable()
				}
				if len(run.errors) > max {
					ui.Text(c, fmt.Sprintf(tr("还有 %d 个错误，详见日志。"), len(run.errors)-max)).FontSize(10.5).TextColor(p.faint)
				}
			})
		}
		if len(run.log) > 0 {
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				ui.Text(c, tr("生成日志")).FontSize(11.5).FontWeight(650).TextColor(p.muted)
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
		ui.Text(c, tr("小提示：生成的目录可以直接合并进工程；iOS 的 AppIcon.appiconset 拖进 Xcode，Windows 的 app.ico 用于安装包，Linux 的 .desktop 和 hicolor 图标放到对应系统目录即可。每个平台的 README.txt 写有具体步骤。")).FontSize(11).TextColor(p.muted).LineHeight(1.5)
	})
}

func (a *app) iconDocsTab(c *ui.Context, p palette) {
	if a.iconDocOpts.OnLink == nil {
		a.iconDocOpts = markdown.Options{OnLink: a.openExternal, WrapCode: true}
	}
	ui.Column(c).FillWidth().Padding(14, 16).Radius(14).Background(p.panel).Border(1, p.hair).Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, tr("官方尺寸与格式要求")).FontSize(15).FontWeight(700)
				ui.Text(c, tr("下表按各平台当前官方规范整理。不同平台版本和打包工具可能略有差异，遇到审核或打包工具报错时，请以各平台官方文档为准。生成器会为每个平台建立独立目录，并附带 README.txt。")).FontSize(11.5).TextColor(p.muted).LineHeight(1.5)
				ui.Text(c, tr("ImageMagick 负责缩放和格式转换；所有图标都由同一张源图生成，不会上传到网络。")).FontSize(11).TextColor(p.faint)
			})
			if ui.Button(c.Key("icon-copy-all-docs"), tr("复制全部说明")).Clicked() {
				c.WriteClipboard(iconkit.AllMarkdown())
				a.setToast(tr("已复制全部平台说明"))
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
					ui.Text(c, fmt.Sprintf(tr("%d 个文件"), platform.FileCount())).FontSize(10.5).TextColor(p.muted)
				})
				if ui.Button(c.Key("icon-copy-doc-"+platform.Key), tr("复制说明")).Clicked() {
					c.WriteClipboard(platform.Markdown())
					a.setToast(platform.Name + tr(" 说明已复制"))
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
			Title:       tr("选择源图片"),
			ButtonLabel: tr("使用这张图片"),
			Message:     tr("建议选择 1024 × 1024 以上的 PNG"),
			Filters: []mygo.FileFilter{{
				Name:       tr("图片"),
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
				a.setToast(tr("无法读取这张图片，请换一张 PNG 或 JPG"))
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
			Title:             tr("选择输出目录"),
			ButtonLabel:       tr("使用这个目录"),
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
		a.setToast(tr("请先选择一张源图片"))
		return
	}
	if info, err := os.Stat(src); err != nil || info.IsDir() {
		a.setToast(tr("源图片不存在或已被移动，请重新选择"))
		return
	}
	keys := a.iconSelectedKeys()
	if len(keys) == 0 {
		a.setToast(tr("请至少选择一个平台"))
		return
	}
	out := strings.TrimSpace(a.settings.IconOutputDir)
	if out == "" {
		a.setToast(tr("请选择输出目录"))
		return
	}
	bg := strings.TrimSpace(a.settings.IconBackground)
	if !iconBackgroundPattern.MatchString(bg) {
		a.setToast(tr("背景色请使用 #RRGGBB 格式"))
		return
	}
	name := iconkit.SafeName(a.settings.IconName)
	a.settings.IconName = name
	_ = a.settings.save()

	run := &iconRun{
		running: true,
		current: tr("正在检测 ImageMagick…"),
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
				a.finishIconRun(run, iconkit.Result{}, tr("未找到 ImageMagick：")+err.Error())
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
				run.current = tr("正在创建目录并写入图标…")
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
		run.current = tr("已取消生成，已写入的文件会保留")
		a.setToast(tr("已取消生成，已写入的文件会保留"))
	} else if len(res.Errors) > 0 {
		run.current = tr("部分文件生成失败")
		a.setToast(fmt.Sprintf(tr("生成完成：%d 个文件，%d 个失败"), res.Files, len(res.Errors)))
	} else if res.Files == 0 {
		run.current = tr("没有写出任何文件")
		a.setToast(tr("生成失败：没有写出任何文件"))
	} else {
		run.current = tr("全部文件已生成")
		a.setToast(fmt.Sprintf(tr("生成完成：%d 个文件，%s"), res.Files, formatRunDuration(res.Duration)))
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
		run.current = tr("正在取消…")
	}
	a.redraw()
}

func iconDoneHint(run *iconRun) string {
	if run.canceled {
		return tr("生成已取消，成功写入的文件保留在输出目录中，可以重新生成覆盖。")
	}
	if len(run.errors) > 0 {
		return tr("成功生成的文件已保留；修复错误后可以再次生成，重复文件会被覆盖。")
	}
	if run.files > 0 {
		return tr("已完成，输出目录中的平台子目录可以直接合并进工程。")
	}
	return tr("没有生成文件，请检查 ImageMagick 和输出目录权限后重试。")
}

func formatRunDuration(d time.Duration) string {
	if d <= 0 {
		return "0 " + tr("秒")
	}
	if d < time.Second {
		return fmt.Sprintf("%d %s", d.Milliseconds(), tr("毫秒"))
	}
	return fmt.Sprintf("%.1f %s", d.Seconds(), tr("秒"))
}

func imageMagickHint() string {
	switch runtime.GOOS {
	case "windows":
		return tr("安装：winget install ImageMagick.ImageMagick  或  choco install imagemagick")
	case "darwin":
		return tr("安装：brew install imagemagick")
	default:
		return tr("安装：sudo apt install imagemagick  或  sudo dnf install ImageMagick")
	}
}

func appendTail(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}
