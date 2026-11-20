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
			Title:   tr("app.common.format_conversion"),
			Desc:    tr("app.image.convert_between_png_jpg_webp_gif"),
			Command: "magick input.png output.webp",
			Icon:    iconImage,
		},
		{
			Key:     string(magick.KindGeometry),
			Title:   tr("app.image.resize_and_crop"),
			Desc:    tr("app.image.resize_or_crop_to_target_size"),
			Command: "magick input.jpg -resize 800x600 out.jpg",
			Icon:    iconResize,
		},
		{
			Key:     string(magick.KindBatch),
			Title:   tr("app.common.batch_processing"),
			Desc:    tr("app.image.process_many_images_at_once_with"),
			Command: "mogrify -quality 85 *.jpg",
			Icon:    iconLayers,
		},
		{
			Key:     string(magick.KindWatermark),
			Title:   tr("app.common.watermark"),
			Desc:    tr("app.image.add_text_or_image_watermark_with"),
			Command: `magick input.jpg -annotate +0+0 "©"`,
			Icon:    iconWatermark,
		},
		{
			Key:     string(magick.KindCompose),
			Title:   tr("app.image.composition_2"),
			Desc:    tr("app.image.stitch_images_horizontally_or_vertically_or"),
			Command: "magick frame*.jpg animation.gif",
			Icon:    iconCompose,
		},
		{
			Key:     string(magick.KindOptimize),
			Title:   tr("app.image.color_and_quality"),
			Desc:    tr("app.image.adjust_quality_auto_level_and_strip"),
			Command: "magick input.jpg -quality 82 -strip out.jpg",
			Icon:    iconWand,
		},
		{
			Key:   magickIconCap,
			Title: tr("app.common.installer_icons"),
			Desc:  tr("app.image.upload_one_image_to_generate_standard"),
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
			if pillButtonKey(c, "magick-back", iconBack, tr("app.common.capabilities"), p.hover.Over(p.bg), p.text).Clicked() {
				a.magickBack()
			}
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			if selected {
				sectionTitle(c, p, cap.Title, cap.Desc)
			} else {
				sectionTitle(c, p, "ImageMagick", tr("app.image.format_conversion_resizing_cropping_batch_processing"))
			}
		})
		if run := a.magick.run; run != nil && run.running {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(10, 12).Radius(999).Background(p.accent.Alpha(0.13)).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, tr("app.common.processing_2")).FontSize(11.5).FontWeight(600).TextColor(p.accent)
			})
		}
	})
}

func (a *app) magickCapabilitiesView(c *ui.Context, p palette) {
	ui.Scroll(c).Grow(1).Margin(0, -6).Padding(0, 6).Gap(14).Children(func() {
		a.magickToolCard(c, p)
		ui.Text(c, tr("app.common.choose_capability")).FontSize(13).FontWeight(650).Margin(2, 0, 0, 0)
		ui.Grid(c).FillWidth().Columns(2).Gap(12).Children(func() {
			for _, cap := range magickCapabilities() {
				a.magickCapabilityCard(c, p, cap)
			}
		})
		ui.Row(c).FillWidth().Padding(12, 14).Radius(12).Background(p.panel).Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, iconInfo).Size(16, 16).TextColor(p.faint)
			ui.Text(c, tr("app.image.everything_runs_locally_through_imagemagick_no")).FontSize(11).TextColor(p.muted).LineHeight(1.5)
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
			title := tr("app.common.checking_imagemagick")
			desc := tr("app.image.these_tools_use_imagemagick_installed_on")
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
			if pillButtonKey(c, "magick-tool-check", iconRefresh, tr("app.common.check_again"), p.hover.Over(p.bg), p.text).Clicked() {
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
			ui.Text(c, tr("app.image.input_images")).FontSize(13).FontWeight(650)
			if len(a.magick.inputs) > 0 {
				ui.Text(c, fmt.Sprintf(tr("app.image.selected"), len(a.magick.inputs))).FontSize(11).TextColor(p.faint)
			} else {
				ui.Text(c, tr("app.image.supports_png_jpg_webp_gif_bmp")).FontSize(11).TextColor(p.faint)
			}
			ui.Spacer(c)
			if len(a.magick.inputs) > 0 && ui.Button(c.Key("magick-clear-inputs"), tr("app.common.clear_2")).Clicked() {
				a.magick.inputs = nil
				a.redraw()
			}
			if pillButtonKey(c, "magick-pick-inputs", iconUpload, tr("app.image.add_images"), p.hover.Over(p.bg), p.text).Clicked() {
				a.pickMagickInputs()
			}
		})
		if len(a.magick.inputs) == 0 {
			ui.Row(c).FillWidth().Padding(18, 16).Radius(12).Background(p.hover.Over(p.panel)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconImage).Size(20, 20).TextColor(p.faint)
				ui.Text(c, tr("app.image.no_images_selected_yet_click_add")).FontSize(11.5).TextColor(p.muted)
			})
			return
		}
		max := len(a.magick.inputs)
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
				if b := iconButton(c, iconInfo, 13, p.faint, tr("app.common.view_file_details")).Key(fmt.Sprintf("magick-info-%d", i)); b.Clicked() {
					a.openFileInfo(path, fileInfoImage)
				}
				if b := iconButton(c, iconX, 12, p.faint, tr("app.common.remove")).Key(fmt.Sprintf("magick-remove-%d", i)); b.Clicked() {
					a.magick.inputs = append(a.magick.inputs[:i], a.magick.inputs[i+1:]...)
					a.redraw()
				}
			})
		}
		if a.magick.cap == string(magick.KindCompose) {
			a.magickNote(c, p, tr("app.image.stitching_and_gif_animation_follow_order"))
		}
	})
}

func (a *app) magickOutputCard(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.common.output_folder")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("magick-output"), &a.settings.MagickOutputDir).Grow(1).Placeholder(tr("app.common.choose_output_folder_2"))
			if ui.Button(c.Key("magick-pick-output"), tr("app.common.select")).Clicked() {
				a.pickMagickOutputDir()
			}
			if dir := a.magickOutputDir(); dir != "" {
				if ui.Button(c.Key("magick-open-output"), tr("app.common.open")).Clicked() {
					a.openPath(dir)
				}
			}
		})
		ui.Text(c, tr("app.common.results_are_written_to_this_folder")).FontSize(10.5).TextColor(p.faint).LineHeight(1.4)
	})
}

func (a *app) magickConvertOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.image.conversion_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(190).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.target_format"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(140).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.quality"))
				ui.NumberInput(c.Key("magick-convert-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Text(c, tr("app.image.jpg_does_not_support_transparency_transparent")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickGeometryOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.image.resize_or_crop")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.geomTab, tr("app.common.zoom"), tr("app.common.crop")).Label(tr("app.common.mode")).Width(260)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.width_px"))
				ui.NumberInput(c.Key("magick-width"), &a.magick.width, 0, 20000, 10)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.height_px"))
				ui.NumberInput(c.Key("magick-height"), &a.magick.height, 0, 20000, 10)
			})
			if a.magick.geomTab == 0 {
				ui.Column(c).Width(170).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.image.resize_mode"))
					labels := []string{tr("app.image.fit_keep_aspect_ratio"), tr("app.image.fill_and_crop"), tr("app.common.stretch")}
					values := []string{magick.ResizeFit, magick.ResizeFill, magick.ResizeStretch}
					selectKeyed(c, "magick-resize-mode", &a.magick.resize, values, labels)
				})
			} else {
				ui.Column(c).Width(150).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.image.crop_position"))
					a.magickGravitySelect(c, p, "magick-crop-gravity", &a.magick.gravity)
				})
			}
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.magickFormatSelect(c, p, true)
			})
		})
		if a.magick.geomTab == 0 {
			ui.Text(c, tr("app.image.set_width_or_height_to_0")).FontSize(10.5).TextColor(p.faint)
		} else {
			ui.Text(c, tr("app.image.crop_uses_chosen_position_as_its")).FontSize(10.5).TextColor(p.faint)
		}
	})
}

func (a *app) magickBatchOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.common.batch_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.max_width"))
				ui.NumberInput(c.Key("magick-batch-width"), &a.magick.width, 0, 20000, 10)
			})
			ui.Column(c).Width(130).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.max_height"))
				ui.NumberInput(c.Key("magick-batch-height"), &a.magick.height, 0, 20000, 10)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.quality"))
				ui.NumberInput(c.Key("magick-batch-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Text(c, tr("app.image.only_images_larger_than_limit_are")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickWatermarkOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.common.watermark_settings")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.wmTab, tr("app.common.text_watermark"), tr("app.common.image_watermark")).Label(tr("app.common.type")).Width(280)
		if a.magick.wmTab == 0 {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.watermark_text"))
				ui.TextInput(c.Key("magick-wm-text"), &a.magick.wmText).Placeholder(tr("app.common.e_g_zchat"))
			})
		} else {
			ui.Column(c).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.watermark_image"))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("magick-wm-image"), &a.magick.wmImage).Grow(1).Placeholder(tr("app.common.png_or_jpg_transparent_background_works"))
					if ui.Button(c.Key("magick-pick-wm"), tr("app.common.select")).Clicked() {
						a.pickMagickWatermark()
					}
				})
			})
		}
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.position"))
				a.magickGravitySelect(c, p, "magick-wm-gravity", &a.magick.gravity)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				if a.magick.wmTab == 1 {
					fieldLabel(c, p, tr("app.image.max_watermark_side"))
					ui.NumberInput(c.Key("magick-wm-size"), &a.magick.wmSize, 1, 4000, 1)
				} else {
					fieldLabel(c, p, tr("app.common.font_size"))
					ui.NumberInput(c.Key("magick-wm-size"), &a.magick.font, 1, 4000, 1)
				}
			})
			ui.Column(c).Width(110).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.opacity"))
				ui.NumberInput(c.Key("magick-wm-opacity"), &a.magick.opacity, 0.05, 1, 0.05)
			})
			if a.magick.wmTab == 0 {
				ui.Column(c).Width(100).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.common.text_color"))
					ui.TextInput(c.Key("magick-wm-color"), &a.magick.color).Placeholder("#ffffff")
				})
			}
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.magickFormatSelect(c, p, true)
			})
		})
		ui.Text(c, tr("app.image.watermark_is_centered_by_default_corners")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickComposeOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.image.composition")).FontSize(13).FontWeight(650)
		ui.Segmented(c, &a.magick.layout, tr("app.image.horizontal_stitch"), tr("app.image.vertical_stitch"), tr("app.image.gif_animation")).Label(tr("app.common.mode")).Width(380)
		if a.magick.layout == 2 {
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
				ui.Column(c).Width(160).Gap(5).Children(func() {
					fieldLabel(c, p, tr("app.image.frame_delay_ms"))
					ui.NumberInput(c.Key("magick-delay"), &a.magick.delay, 10, 5000, 10)
				})
			})
			ui.Text(c, tr("app.image.gif_loops_forever_images_become_frames")).FontSize(10.5).TextColor(p.faint)
		} else {
			ui.Text(c, tr("app.image.stitched_images_are_saved_as_png")).FontSize(10.5).TextColor(p.faint)
		}
	})
}

func (a *app) magickOptimizeOptions(c *ui.Context, p palette) {
	a.magickCard(c, p, func() {
		ui.Text(c, tr("app.image.quality_settings")).FontSize(13).FontWeight(650)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(150).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.common.output_format"))
				a.magickFormatSelect(c, p, true)
			})
			ui.Column(c).Width(120).Gap(5).Children(func() {
				fieldLabel(c, p, tr("app.image.quality"))
				ui.NumberInput(c.Key("magick-opt-quality"), &a.magick.quality, 1, 100, 1)
			})
		})
		ui.Row(c).FillWidth().Gap(20).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Switch(c.Key("magick-auto-level"), &a.magick.autoLevel).Label(tr("app.image.auto_level"))
				ui.Text(c, tr("app.image.auto_level")).FontSize(12)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Switch(c.Key("magick-strip"), &a.magick.strip).Label(tr("app.image.strip_metadata"))
				ui.Text(c, tr("app.image.strip_metadata_exif_icc_etc")).FontSize(12)
			})
		})
		ui.Text(c, tr("app.image.auto_level_redistributes_tones_which_helps")).FontSize(10.5).TextColor(p.faint).LineHeight(1.45)
	})
}

func (a *app) magickFormatSelect(c *ui.Context, p palette, allowKeep bool) {
	values := []string{"png", "jpg", "webp", "gif", "bmp", "tiff", "pdf"}
	labels := []string{"PNG", "JPG", "WebP", "GIF", "BMP", "TIFF", "PDF"}
	if allowKeep {
		values = append([]string{"keep"}, values...)
		labels = append([]string{tr("app.common.keep_original_format")}, labels...)
	}
	selectKeyed(c, "magick-format", &a.magick.format, values, labels)
}

func (a *app) magickGravitySelect(c *ui.Context, p palette, key string, value *string) {
	values := []string{"center", "north", "south", "west", "east", "northwest", "northeast", "southwest", "southeast"}
	labels := []string{tr("app.common.center"), tr("app.image.top"), tr("app.image.bottom"), tr("app.image.left"), tr("app.image.right"), tr("app.common.top_left"), tr("app.common.top_right"), tr("app.common.bottom_left"), tr("app.common.bottom_right")}
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
			if pillButtonKey(c, "magick-cancel", iconPause, tr("app.common.cancel_processing"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelMagickRun()
			}
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, tr("app.common.processing_files_already_written_are_kept")).FontSize(11.5).TextColor(p.muted)
		})
		return
	}
	label := tr("app.common.start_processing")
	if run := a.magick.run; run != nil && run.finished {
		label = tr("app.common.process_again")
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		if pillButtonKey(c, "magick-run", iconPlay, label, p.accent, p.onAcc).Clicked() {
			a.startMagickRun()
		}
		ui.Text(c, tr("app.image.images_output_folder_and_imagemagick_are")).FontSize(11).TextColor(p.faint)
	})
}

func (a *app) magickRunCard(c *ui.Context, p palette, run *magickRun) {
	status, color, icon := tr("app.common.processing"), p.accent, iconWand
	switch {
	case run.canceled:
		status, color, icon = tr("app.common.canceled"), ui.Oklch(0.78, 0.16, 80), iconPause
	case run.finished && len(run.errors) > 0 && run.files > 0:
		status, color, icon = tr("app.common.partially_complete"), ui.Oklch(0.78, 0.16, 80), iconInfo
	case run.finished && len(run.errors) > 0:
		status, color, icon = tr("app.common.processing_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
	case run.finished && run.files > 0:
		status, color, icon = tr("app.common.processing_complete"), ui.Oklch(0.68, 0.17, 145), iconCheck
	case run.finished:
		status, color, icon = tr("app.common.processing_failed"), ui.Oklch(0.64, 0.2, 28), iconInfo
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
					if pillButtonKey(c, "magick-open-run-output", iconFolder, tr("app.common.open_output_folder"), p.hover.Over(p.bg), p.text).Clicked() {
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
		})
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
				ui.Text(c, tr("app.common.processing_log")).FontSize(11.5).FontWeight(650).TextColor(p.muted)
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
			ui.Text(c, tr("app.common.results_are_in_output_folder_source")).FontSize(11).TextColor(p.faint)
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
			Title:       tr("app.common.choose_image"),
			ButtonLabel: tr("app.common.add"),
			Message:     tr("app.image.select_several_images_at_once_composition"),
			Multiple:    true,
			Filters: []mygo.FileFilter{{
				Name:       tr("app.common.images"),
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
			Title:       tr("app.common.choose_watermark_image_2"),
			ButtonLabel: tr("app.common.use_this_image"),
			Filters: []mygo.FileFilter{{
				Name:       tr("app.common.images"),
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
			Title:             tr("app.common.choose_output_folder_3"),
			ButtonLabel:       tr("app.common.use_this_folder"),
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
		current: tr("app.common.checking_imagemagick"),
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
				a.finishMagickRun(run, magick.Result{}, tr("app.common.imagemagick_not_found_2")+err.Error())
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
				run.current = tr("app.image.processing_images")
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
		run.current = tr("app.common.processing_canceled_files_already_written_are")
		a.setToast(tr("app.common.processing_canceled_files_already_written_are"))
	} else if len(res.Errors) > 0 {
		run.current = tr("app.common.some_files_failed")
		a.setToast(fmt.Sprintf(tr("app.common.processed_files_failed"), res.Files, len(res.Errors)))
	} else if res.Files == 0 {
		run.current = tr("app.common.no_files_were_written")
		a.setToast(tr("app.common.processing_failed_no_files_were_written"))
	} else {
		run.current = tr("app.common.all_files_processed")
		a.setToast(fmt.Sprintf(tr("app.common.processed_files_in"), res.Files, formatRunDuration(res.Duration)))
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
		run.current = tr("app.common.canceling")
	}
	a.redraw()
}
