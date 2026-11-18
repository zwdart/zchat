package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zchat/internal/magick"
)

const magickIconCap = "icons"

// magickCapability describes one entry of the toolkit's capability list.
type magickCapability struct {
	Key     string
	Title   string
	Desc    string
	Command string
	Icon    *ui.SVG
}

func magickCapabilities() []magickCapability {
	return []magickCapability{
		{
			Key:     string(magick.KindConvert),
			Title:   tr("格式转换"),
			Desc:    tr("在 PNG、JPG、WebP、GIF、BMP、TIFF、PDF 之间互相转换"),
			Command: "magick input.png output.webp",
			Icon:    iconImage,
		},
		{
			Key:     string(magick.KindGeometry),
			Title:   tr("缩放与裁剪"),
			Desc:    tr("按尺寸缩放、裁剪画面，适合缩略图与不同终端的图片"),
			Command: "magick input.jpg -resize 800x600 out.jpg",
			Icon:    iconResize,
		},
		{
			Key:     string(magick.KindBatch),
			Title:   tr("批量处理"),
			Desc:    tr("一次处理多张图片，统一格式、尺寸与压缩质量"),
			Command: "mogrify -quality 85 *.jpg",
			Icon:    iconLayers,
		},
		{
			Key:     string(magick.KindWatermark),
			Title:   tr("加水印"),
			Desc:    tr("添加文字或图片水印，可设置位置、大小和透明度"),
			Command: `magick input.jpg -annotate +0+0 "©"`,
			Icon:    iconWatermark,
		},
		{
			Key:     string(magick.KindCompose),
			Title:   tr("图像合成"),
			Desc:    tr("横向、纵向拼接多张图片，或合成 GIF 动画"),
			Command: "magick frame*.jpg animation.gif",
			Icon:    iconCompose,
		},
		{
			Key:     string(magick.KindOptimize),
			Title:   tr("颜色与质量"),
			Desc:    tr("调整压缩质量、自动色阶并清除元数据，减小图片体积"),
			Command: "magick input.jpg -quality 82 -strip out.jpg",
			Icon:    iconWand,
		},
		{
			Key:   magickIconCap,
			Title: tr("安装包图标"),
			Desc:  tr("上传一张图片，生成各平台安装包所需的标准图标与目录"),
			Icon:  iconPackage,
		},
	}
}

func findMagickCapability(key string) (magickCapability, bool) {
	for _, cap := range magickCapabilities() {
		if cap.Key == key {
			return cap, true
		}
	}
	return magickCapability{}, false
}

// magickState is the toolkit's view state. The output folder, format and
// quality also live in settings so they survive a restart.
type magickState struct {
	initialized bool
	cap         string
	inputs      []string

	format    string
	quality   float64
	geomTab   int
	resize    string
	width     float64
	height    float64
	gravity   string
	strip     bool
	autoLevel bool

	wmTab   int
	wmText  string
	wmImage string
	font    float64
	wmSize  float64
	opacity float64
	color   string
	layout  int
	delay   float64

	run    *magickRun
	cancel context.CancelFunc
}

// magickRun is one toolkit run's progress and result.
type magickRun struct {
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
	canceled bool
}

func (a *app) magickDefaults() {
	m := &a.magick
	if m.initialized {
		return
	}
	m.initialized = true
	m.format = a.settings.MagickFormat
	if m.format == "" {
		m.format = "keep"
	}
	m.quality = float64(a.settings.MagickQuality)
	if m.quality < 1 || m.quality > 100 {
		m.quality = 90
	}
	m.resize = magick.ResizeFit
	m.width, m.height = 800, 600
	m.gravity = "center"
	m.strip = true
	m.autoLevel = true
	m.wmTab = 0
	m.font = 36
	m.wmSize = 180
	m.opacity = 0.6
	m.color = "#ffffff"
	m.layout = 0
	m.delay = 20
}

func (a *app) magickPage(c *ui.Context) {
	if a.magick.cap == magickIconCap {
		a.iconKitPage(c)
		return
	}
	a.magickDefaults()
	p := a.pal
	if !a.iconToolChecked && !a.iconToolChecking {
		a.detectIconTool()
	}
	ui.Column(c).Fill().Children(func() {
		if inset := a.contentTop(c); inset > 0 {
			ui.Box(c).FillWidth().Height(inset).Shrink(0).DragWindow()
		}
		ui.Column(c).FillWidth().Grow(1).Padding(32, 30, 32, 24).Gap(16).Children(func() {
			a.magickHeader(c, p)
			if a.magick.cap == "" {
				a.magickCapabilitiesView(c, p)
				return
			}
			a.magickForm(c, p)
		})
	})
}

func (a *app) magickHeader(c *ui.Context, p palette) {
	cap, selected := findMagickCapability(a.magick.cap)
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).DragWindow().Children(func() {
		if selected {
			if pillButtonKey(c, "magick-back", iconBack, tr("能力列表"), p.hover.Over(p.bg), p.text).Clicked() {
				a.magickBack()
			}
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			if selected {
				sectionTitle(c, p, cap.Title, cap.Desc)
			} else {
				sectionTitle(c, p, "ImageMagick", tr("格式转换、缩放裁剪、批量处理、水印、合成与质量优化，全部在本机完成"))
			}
		})
		if run := a.magick.run; run != nil && run.running {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(10, 12).Radius(999).Background(p.accent.Alpha(0.13)).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, tr("正在处理…")).FontSize(11.5).FontWeight(600).TextColor(p.accent)
			})
		}
	})
}

func (a *app) magickCapabilitiesView(c *ui.Context, p palette) {
	ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
		a.magickToolCard(c, p)
		ui.Text(c, tr("选择一项能力")).FontSize(13).FontWeight(650).Margin(2, 0, 0, 0)
		ui.Grid(c).FillWidth().Columns(2).Gap(12).Children(func() {
			for _, cap := range magickCapabilities() {
				a.magickCapabilityCard(c, p, cap)
			}
		})
		ui.Row(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, iconInfo).Size(16, 16).TextColor(p.faint)
			ui.Text(c, tr("所有处理都在本机调用 ImageMagick 完成，不会上传图片。源文件不会被修改，结果写入你选择的输出目录。")).FontSize(11).TextColor(p.muted).LineHeight(1.5)
		})
	})
}

func (a *app) magickCapabilityCard(c *ui.Context, p palette, cap magickCapability) {
	b := ui.ButtonBase(c).Key("magick-cap-"+cap.Key).FillWidth().Padding(15, 14).Radius(14).Gap(11).
		AlignItems(ui.Start).FocusRing(false).Label(cap.Title).Border(1, p.hair).Background(p.panel)
	if b.Hovered() {
		b.Background(p.hover.Over(p.panel))
	}
	b.Children(func() {
		ui.Box(c).Size(36, 36).Radius(10).Shrink(0).Center().Background(p.accent.Alpha(0.12)).Children(func() {
			ui.Icon(c, cap.Icon).Size(19, 19).TextColor(p.accent)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, cap.Title).FontSize(13.5).FontWeight(650)
			ui.Text(c, cap.Desc).FontSize(11).TextColor(p.muted).LineHeight(1.45)
			if cap.Command != "" {
				ui.Text(c, cap.Command).FontSize(10).TextColor(p.faint).SingleLine().Selectable()
			}
		})
	})
	if b.Clicked() {
		a.magick.cap = cap.Key
		a.magick.run = nil
		if a.magickOutputDir() == "" {
			a.settings.MagickOutputDir = defaultMagickOutputDir()
			_ = a.settings.save()
		}
		a.redraw()
	}
}

func (a *app) magickToolCard(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Padding(16, 15).Radius(14).Background(p.panel).Border(1, p.hair).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			bg, fg := p.hover, p.muted
			title := tr("正在检测 ImageMagick…")
			desc := tr("这些工具需要调用本机的 ImageMagick。")
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
				ui.Icon(c, iconWand).Size(21, 21).TextColor(fg)
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
			if pillButtonKey(c, "magick-tool-check", iconRefresh, tr("重新检测"), p.hover.Over(p.bg), p.text).Clicked() {
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
		}
	})
}

func (a *app) magickForm(c *ui.Context, p palette) {
	a.magickToolCard(c, p)
	ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
		a.magickInputsCard(c, p)
		a.magickOutputCard(c, p)
		switch a.magick.cap {
		case string(magick.KindConvert):
			a.magickConvertOptions(c, p)
		case string(magick.KindGeometry):
			a.magickGeometryOptions(c, p)
		case string(magick.KindBatch):
			a.magickBatchOptions(c, p)
		case string(magick.KindWatermark):
			a.magickWatermarkOptions(c, p)
		case string(magick.KindCompose):
			a.magickComposeOptions(c, p)
		case string(magick.KindOptimize):
			a.magickOptimizeOptions(c, p)
		}
		a.magickActions(c, p)
		if run := a.magick.run; run != nil {
			a.magickRunCard(c, p, run)
		}
	})
}

func (a *app) magickInputsCard(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, tr("输入图片")).FontSize(13).FontWeight(650)
			if len(a.magick.inputs) > 0 {
				ui.Text(c, fmt.Sprintf(tr("已选择 %d 张"), len(a.magick.inputs))).FontSize(11).TextColor(p.faint)
			} else {
				ui.Text(c, tr("支持 PNG、JPG、WebP、GIF、BMP、TIFF、SVG、PDF")).FontSize(11).TextColor(p.faint)
			}
			ui.Spacer(c)
			if len(a.magick.inputs) > 0 && ui.Button(c.Key("magick-clear-inputs"), tr("清空")).Clicked() {
				a.magick.inputs = nil
				a.redraw()
			}
			if pillButtonKey(c, "magick-pick-inputs", iconUpload, tr("添加图片"), p.hover.Over(p.bg), p.text).Clicked() {
				a.pickMagickInputs()
			}
		})
		if len(a.magick.inputs) == 0 {
			ui.Row(c).FillWidth().Padding(18, 16).Radius(12).Background(p.hover.Over(p.panel)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconImage).Size(20, 20).TextColor(p.faint)
				ui.Text(c, tr("还没有选择图片，点击右上角「添加图片」开始。")).FontSize(11.5).TextColor(p.muted)
			})
			return
		}
		max := len(a.magick.inputs)
		if max > 8 {
			max = 8
		}
		for i := 0; i < max; i++ {
			i := i
			path := a.magick.inputs[i]
			ui.Row(c).FillWidth().Padding(9, 9).Radius(10).Background(p.hover.Over(p.panel)).Gap(10).AlignItems(ui.Center).Children(func() {
				bmp := a.loadImage(path)
				ui.Box(c).Size(34, 34).Radius(8).Shrink(0).Center().Background(p.panel).Children(func() {
					if bmp != nil {
						ui.Image(c, bmp).Size(34, 34).Fit(ui.Cover).Radius(8)
					} else {
						ui.Icon(c, iconImage).Size(16, 16).TextColor(p.faint)
					}
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, filepath.Base(path)).FontSize(12).FontWeight(600).SingleLine()
					meta := path
					if info, err := os.Stat(path); err == nil {
						meta = fmt.Sprintf("%s · %s", path, formatBytes(info.Size()))
					}
					ui.Text(c, meta).FontSize(10.5).TextColor(p.faint).SingleLine().Selectable()
				})
				if b := iconButton(c, iconX, 12, p.faint, tr("移除")); b.Clicked() {
					a.magick.inputs = append(a.magick.inputs[:i], a.magick.inputs[i+1:]...)
					a.redraw()
				}
			})
		}
		if len(a.magick.inputs) > max {
			ui.Text(c, fmt.Sprintf(tr("还有 %d 张图片未显示"), len(a.magick.inputs)-max)).FontSize(10.5).TextColor(p.faint)
		}
		if a.magick.cap == string(magick.KindCompose) {
			a.magickNote(c, p, tr("拼接和 GIF 动画会按上面的顺序合成，可清空后重新按需要添加。"))
		}
	})
}

func (a *app) magickOutputCard(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("输出目录")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("magick-output"), &a.settings.MagickOutputDir).Grow(1).Placeholder(tr("选择一个输出目录"))
			if ui.Button(c.Key("magick-pick-output"), tr("选择…")).Clicked() {
				a.pickMagickOutputDir()
			}
			if dir := a.magickOutputDir(); dir != "" {
				if ui.Button(c.Key("magick-open-output"), tr("打开")).Clicked() {
					a.openPath(dir)
				}
			}
		})
		ui.Text(c, tr("处理结果会写入这个目录，源文件保持不变；同名文件会被覆盖。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.4)
	})
}

func (a *app) magickConvertOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("转换设置")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(190).Gap(5).Children(func() {
				fieldLabel(c, p, tr("目标格式"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(140).Gap(5).Children(func() {
				fieldLabel(c, p, tr("压缩质量"))
				ui.NumberInput(c.Key("magick-convert-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Text(c, tr("JPG 不支持透明，透明区域会填充为白色。质量只影响 JPG、WebP 等有损格式。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickGeometryOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("缩放或裁剪")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.geomTab, tr("缩放"), tr("裁剪")).Label(tr("方式")).Width(260)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("宽度（像素）"))
				ui.NumberInput(c.Key("magick-width"), &a.magick.width, 0, 20000, 10)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("高度（像素）"))
				ui.NumberInput(c.Key("magick-height"), &a.magick.height, 0, 20000, 10)
			})
			if a.magick.geomTab == 0 {
				ui.Column(c).Width(170).Gap(5).Children(func() {
					fieldLabel(c, p, tr("缩放方式"))
					labels := []string{tr("适应（保持比例）"), tr("填充并裁剪"), tr("拉伸")}
					values := []string{magick.ResizeFit, magick.ResizeFill, magick.ResizeStretch}
					selectKeyed(c, "magick-resize-mode", &a.magick.resize, values, labels)
				})
			} else {
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("裁剪位置"))
					a.magickGravitySelect(c, p, "magick-crop-gravity", &a.magick.gravity)
				})
			}
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("输出格式"))
				a.magickFormatSelect(c, p, true)
			})
		})
		if a.magick.geomTab == 0 {
			ui.Text(c, tr("宽度或高度填 0 表示按比例自动计算。")).FontSize(10.5).TextColor(p.faint)
		} else {
			ui.Text(c, tr("裁剪以选定位置为基准，输出尺寸就是上面填写的宽高。")).FontSize(10.5).TextColor(p.faint)
		}
	})
}

func (a *app) magickBatchOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("批量设置")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("输出格式"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("最大宽度"))
				ui.NumberInput(c.Key("magick-batch-width"), &a.magick.width, 0, 20000, 10)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("最大高度"))
				ui.NumberInput(c.Key("magick-batch-height"), &a.magick.height, 0, 20000, 10)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("压缩质量"))
				ui.NumberInput(c.Key("magick-batch-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Text(c, tr("只会缩小超过限制的图片，不会放大；宽高填 0 表示不限制。每张图输出到「原文件名-batch」。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickWatermarkOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("水印设置")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.wmTab, tr("文字水印"), tr("图片水印")).Label(tr("类型")).Width(280)
		if a.magick.wmTab == 0 {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("水印文字"))
				ui.TextInput(c.Key("magick-wm-text"), &a.magick.wmText).Placeholder(tr("例如：© zchat"))
			})
		} else {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("水印图片"))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("magick-wm-image"), &a.magick.wmImage).Grow(1).Placeholder(tr("PNG 或 JPG，建议使用透明背景"))
					if ui.Button(c.Key("magick-pick-wm"), tr("选择…")).Clicked() {
						a.pickMagickWatermark()
					}
				})
			})
		}
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("位置"))
				a.magickGravitySelect(c, p, "magick-wm-gravity", &a.magick.gravity)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				if a.magick.wmTab == 1 {
					fieldLabel(c, p, tr("水印最大边长"))
					ui.NumberInput(c.Key("magick-wm-size"), &a.magick.wmSize, 1, 4000, 1)
				} else {
					fieldLabel(c, p, tr("字号"))
					ui.NumberInput(c.Key("magick-wm-size"), &a.magick.font, 1, 4000, 1)
				}
			})
			ui.Column(c).Width(110).Gap(5).Children(func() {
				fieldLabel(c, p, tr("不透明度"))
				ui.NumberInput(c.Key("magick-wm-opacity"), &a.magick.opacity, 0.05, 1, 0.05)
			})
			if a.magick.wmTab == 0 {
				ui.Column(c).Width(100).Gap(5).Children(func() {
					fieldLabel(c, p, tr("文字颜色"))
					ui.TextInput(c.Key("magick-wm-color"), &a.magick.color).Placeholder("#ffffff")
				})
			}
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("输出格式"))
				a.magickFormatSelect(c, p, true)
			})
		})
		ui.Text(c, tr("水印默认居中，可选择四角与边缘。建议先在一张图片上确认效果，再批量应用。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickComposeOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("合成方式")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.layout, tr("横向拼接"), tr("纵向拼接"), tr("GIF 动画")).Label(tr("方式")).Width(380)
		if a.magick.layout == 2 {
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
				ui.Column(c).Width(160).Gap(5).Children(func() {
					fieldLabel(c, p, tr("每帧间隔（毫秒）"))
					ui.NumberInput(c.Key("magick-delay"), &a.magick.delay, 10, 5000, 10)
				})
			})
			ui.Text(c, tr("GIF 会循环播放。图片会按列表顺序成为每一帧，尺寸不一致时以第一张为画布。")).FontSize(10.5).TextColor(p.faint)
		} else {
			ui.Text(c, tr("拼接后的图片默认保存为 PNG，可在「输出目录」中找到以 -merged 结尾的文件。")).FontSize(10.5).TextColor(p.faint)
		}
	})
}

func (a *app) magickOptimizeOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("质量设置")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("输出格式"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("压缩质量"))
				ui.NumberInput(c.Key("magick-opt-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Row(c).FillWidth().Gap(20).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Switch(c.Key("magick-auto-level"), &a.magick.autoLevel).Label(tr("自动色阶"))
				ui.Text(c, tr("自动色阶")).FontSize(12)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Switch(c.Key("magick-strip"), &a.magick.strip).Label(tr("清除元数据"))
				ui.Text(c, tr("清除元数据（EXIF、ICC 等）")).FontSize(12)
			})
		})
		ui.Text(c, tr("自动色阶会重新分布明暗，常用于偏灰的照片；清除元数据可以减小体积并保护位置信息。")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickFormatSelect(c *ui.Context, p palette, allowKeep bool) {
	values := []string{"png", "jpg", "webp", "gif", "bmp", "tiff", "pdf"}
	labels := []string{"PNG", "JPG", "WebP", "GIF", "BMP", "TIFF", "PDF"}
	if allowKeep {
		values = append([]string{"keep"}, values...)
		labels = append([]string{tr("保持原格式")}, labels...)
	}
	selectKeyed(c, "magick-format", &a.magick.format, values, labels)
}

func (a *app) magickGravitySelect(c *ui.Context, p palette, key string, value *string) {
	values := []string{"center", "north", "south", "west", "east", "northwest", "northeast", "southwest", "southeast"}
	labels := []string{tr("居中"), tr("上方"), tr("下方"), tr("左侧"), tr("右侧"), tr("左上"), tr("右上"), tr("左下"), tr("右下")}
	selectKeyed(c, key, value, values, labels)
}

func selectKeyed(c *ui.Context, key any, value *string, values, labels []string) bool {
	chosen := ""
	for i, v := range values {
		if v == *value {
			chosen = labels[i]
			break
		}
	}
	if chosen == "" {
		chosen = labels[0]
	}
	if ui.Select(c.Key(key), &chosen, labels).Changed() {
		for i, label := range labels {
			if label == chosen {
				*value = values[i]
				return true
			}
		}
	}
	return false
}

func (a *app) magickActions(c *ui.Context, p palette) {
	if a.magickRunning() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			if pillButtonKey(c, "magick-cancel", iconPause, tr("取消处理"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelMagickRun()
			}
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, tr("正在处理，已写出的文件会保留")).FontSize(11.5).TextColor(p.muted)
		})
		return
	}
	label := tr("开始处理")
	if run := a.magick.run; run != nil && run.finished {
		label = tr("再次处理")
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		if pillButtonKey(c, "magick-run", iconPlay, label, p.accent, p.onAcc).Clicked() {
			a.startMagickRun()
		}
		ui.Text(c, tr("开始前会检查输入图片、输出目录和 ImageMagick。")).FontSize(11).TextColor(p.faint)
	})
}

func (a *app) magickRunCard(c *ui.Context, p palette, run *magickRun) {
	status, color, icon := tr("处理中"), p.accent, iconWand
	switch {
	case run.canceled:
		status, color, icon = tr("已取消"), ui.Oklch(0.78, 0.16, 80), iconPause
	case run.finished && len(run.errors) > 0 && run.files > 0:
		status, color, icon = tr("部分完成"), ui.Oklch(0.78, 0.16, 80), iconInfo
	case run.finished && len(run.errors) > 0:
		status, color, icon = tr("处理失败"), ui.Oklch(0.64, 0.2, 28), iconInfo
	case run.finished && run.files > 0:
		status, color, icon = tr("处理完成"), ui.Oklch(0.68, 0.17, 145), iconCheck
	case run.finished:
		status, color, icon = tr("处理失败"), ui.Oklch(0.64, 0.2, 28), iconInfo
	}
	a.magickCard(c, p, func() {
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
					if pillButtonKey(c, "magick-open-run-output", iconFolder, tr("打开输出目录"), p.hover.Over(p.bg), p.text).Clicked() {
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
		})
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
				ui.Text(c, tr("处理日志")).FontSize(11.5).FontWeight(650).TextColor(p.muted)
				lines := run.log
				if len(lines) > 10 {
					lines = lines[len(lines)-10:]
				}
				for _, line := range lines {
					ui.Text(c, line).FontSize(10.5).TextColor(p.muted).SingleLine().Selectable()
				}
			})
		}
		if run.finished && run.files > 0 {
			ui.Text(c, tr("结果已写入输出目录，源文件保持不变。")).FontSize(11).TextColor(p.faint)
		}
	})
}

func (a *app) magickCard(c *ui.Context, p palette, fn func()) {
	ui.Column(c).FillWidth().Padding(16, 15).Radius(14).Background(p.panel).Border(1, p.hair).Gap(12).Children(fn)
}

func (a *app) magickNote(c *ui.Context, p palette, text string) {
	ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconInfo).Size(12, 12).TextColor(p.faint).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(10.5).TextColor(p.muted).LineHeight(1.45).Grow(1).Shrink(1).MinWidth(0)
	})
}

func (a *app) magickOutputDir() string {
	return strings.TrimSpace(a.settings.MagickOutputDir)
}

func defaultMagickOutputDir() string {
	pictures, err := mygo.App.Path(mygo.PathPictures)
	if err != nil || pictures == "" {
		pictures, _ = mygo.App.Path(mygo.PathDownloads)
	}
	if pictures == "" {
		return ""
	}
	return filepath.Join(pictures, appFolderName, "magick")
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func (a *app) magickBack() {
	if a.magickRunning() {
		a.cancelMagickRun()
	}
	a.magick.run = nil
	a.magick.cap = ""
	a.redraw()
}

func (a *app) magickRunning() bool {
	return a.magick.run != nil && a.magick.run.running
}

func (a *app) magickOptions() magick.Options {
	m := &a.magick
	opt := magick.Options{
		Kind:      magick.Kind(m.cap),
		Inputs:    append([]string(nil), m.inputs...),
		OutputDir: a.magickOutputDir(),
		Format:    m.format,
		Quality:   int(m.quality),
		Width:     int(m.width),
		Height:    int(m.height),
		Gravity:   m.gravity,
		Position:  m.gravity,
		FontSize:  int(m.font),
		Opacity:   m.opacity,
		Color:     m.color,
		Delay:     int(m.delay),
		Strip:     m.strip,
		AutoLevel: m.autoLevel,
	}
	if m.cap == string(magick.KindGeometry) {
		opt.GeometryMode = magick.GeometryResize
		if m.geomTab == 1 {
			opt.GeometryMode = magick.GeometryCrop
		}
		opt.ResizeMode = m.resize
	}
	if m.cap == string(magick.KindWatermark) {
		opt.WatermarkMode = magick.WatermarkText
		if m.wmTab == 1 {
			opt.WatermarkMode = magick.WatermarkImage
			opt.FontSize = int(m.wmSize)
		}
		opt.WatermarkText = m.wmText
		opt.WatermarkImage = strings.TrimSpace(m.wmImage)
	}
	if m.cap == string(magick.KindCompose) {
		switch m.layout {
		case 1:
			opt.Layout = magick.ComposeVertical
		case 2:
			opt.Layout = magick.ComposeAnimation
		default:
			opt.Layout = magick.ComposeHorizontal
		}
	}
	return opt
}

func (a *app) persistMagickSettings() {
	a.settings.MagickFormat = a.magick.format
	a.settings.MagickQuality = int(a.magick.quality)
	_ = a.settings.save()
}

func (a *app) pickMagickInputs() {
	if a.magickRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("选择图片"),
			ButtonLabel: tr("添加"),
			Message:     tr("可以一次选择多张图片，合成时按选择顺序排列"),
			Multiple:    true,
			Filters: []mygo.FileFilter{{
				Name:       tr("图片"),
				Extensions: []string{"png", "jpg", "jpeg", "webp", "gif", "bmp", "tif", "tiff", "svg", "pdf"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			seen := map[string]bool{}
			for _, in := range a.magick.inputs {
				seen[in] = true
			}
			for _, path := range paths {
				if !seen[path] {
					seen[path] = true
					a.magick.inputs = append(a.magick.inputs, path)
				}
			}
			_ = a.loadImage(paths[0])
			a.redraw()
		})
	}()
}

func (a *app) pickMagickWatermark() {
	if a.magickRunning() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       tr("选择水印图片"),
			ButtonLabel: tr("使用这张图片"),
			Filters: []mygo.FileFilter{{
				Name:       tr("图片"),
				Extensions: []string{"png", "jpg", "jpeg", "webp", "gif"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() {
			a.magick.wmImage = paths[0]
			_ = a.loadImage(paths[0])
		})
	}()
}

func (a *app) pickMagickOutputDir() {
	if a.magickRunning() {
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
			a.settings.MagickOutputDir = paths[0]
			_ = a.settings.save()
		})
	}()
}

func (a *app) startMagickRun() {
	if a.magickRunning() {
		return
	}
	opt := a.magickOptions()
	if err := magick.Validate(opt); err != nil {
		a.setToast(err.Error())
		return
	}
	a.persistMagickSettings()
	run := &magickRun{
		running: true,
		current: tr("正在检测 ImageMagick…"),
		output:  opt.OutputDir,
		started: time.Now(),
	}
	a.magick.run = run
	ctx, cancel := context.WithCancel(context.Background())
	a.magick.cancel = cancel
	a.redraw()
	go func() {
		tool, err := magick.Detect()
		if err != nil {
			a.post(func() {
				if a.magick.run != run {
					return
				}
				a.iconTool = magick.Tool{}
				a.iconToolErr = err.Error()
				a.iconToolChecked = true
				a.finishMagickRun(run, magick.Result{}, tr("未找到 ImageMagick：")+err.Error())
			})
			return
		}
		a.post(func() {
			if a.magick.run == run {
				a.iconTool = tool
				a.iconToolErr = ""
				a.iconToolChecked = true
			}
		})
		plan, err := magick.Build(tool, opt)
		if err != nil {
			a.post(func() {
				if a.magick.run == run {
					a.finishMagickRun(run, magick.Result{}, err.Error())
				}
			})
			return
		}
		a.post(func() {
			if a.magick.run == run {
				run.total = len(plan.Steps)
				run.current = tr("正在处理图片…")
			}
		})
		res := plan.Run(ctx, func(ev magick.Progress) {
			a.post(func() {
				if a.magick.run == run {
					a.applyMagickProgress(run, ev)
				}
			})
		})
		a.post(func() {
			if a.magick.run == run {
				a.finishMagickRun(run, res, "")
			}
		})
	}()
}

func (a *app) applyMagickProgress(run *magickRun, ev magick.Progress) {
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

func (a *app) finishMagickRun(run *magickRun, res magick.Result, fatal string) {
	run.running = false
	run.finished = true
	run.elapsed = time.Since(run.started)
	run.files = res.Files
	run.errors = res.Errors
	run.canceled = res.Canceled
	if fatal != "" {
		run.errors = append(run.errors, fatal)
		run.current = fatal
		a.setToast(fatal)
	} else if res.Canceled {
		run.current = tr("已取消处理，已写出的文件会保留")
		a.setToast(tr("已取消处理，已写出的文件会保留"))
	} else if len(res.Errors) > 0 {
		run.current = tr("部分文件处理失败")
		a.setToast(fmt.Sprintf(tr("处理完成：%d 个文件，%d 个失败"), res.Files, len(res.Errors)))
	} else if res.Files == 0 {
		run.current = tr("没有写出任何文件")
		a.setToast(tr("处理失败：没有写出任何文件"))
	} else {
		run.current = tr("全部文件已处理")
		a.setToast(fmt.Sprintf(tr("处理完成：%d 个文件，%s"), res.Files, formatRunDuration(res.Duration)))
	}
	if a.magick.cancel != nil {
		a.magick.cancel()
		a.magick.cancel = nil
	}
	a.redraw()
}

func (a *app) cancelMagickRun() {
	if a.magick.cancel != nil {
		a.magick.cancel()
	}
	if run := a.magick.run; run != nil {
		run.current = tr("正在取消…")
	}
	a.redraw()
}
