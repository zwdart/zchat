package app

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"xin.dart.zchat/internal/i18n"
	"xin.dart.zchat/internal/model"
	"xin.dart.zchat/internal/netutil"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.refreshDBStats()
	a.profileName = a.profile.Name
	a.profileSig = a.profile.Signature
	a.portEdit = float64(a.settings.Port)
	a.transferEdit = float64(a.settings.TransferPort)
	a.shareEdit = float64(a.settings.SharePort)
	a.shotIncludeEdit = a.settings.ScreenshotIncludeShortcut
	a.shotExcludeEdit = a.settings.ScreenshotExcludeShortcut
	a.updateSupported = mygo.Updater.Enabled()
	if a.settingsNav == "" {
		a.settingsNav = "profile"
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           tr("app.common.zchat_settings"),
		Width:           720,
		Height:          760,
		MinWidth:        620,
		MinHeight:       460,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f3f3f7, #1b1b1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
	})
}

// settingsSections are the menu items of the settings window, in page order.
var settingsSections = []struct {
	Key   string
	Label string
	Icon  *ui.SVG
}{
	{"profile", "个人资料", iconUsers},
	{"appearance", "外观", iconImage},
	{"notifications", "通知", iconBell},
	{"network", "网络", iconWifi},
	{"files", "文件", iconFolder},
	{"screenshot", "截图", iconScreenshot},
	{"history", "聊天记录", iconClock},
	{"update", "更新", iconRefresh},
	{"about", "关于", iconInfo},
}

// settingsMenu is the left menu of the settings window: a click scrolls the
// page to that section.
func (a *app) settingsMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(164).Padding(16, 12).Gap(2).
		BorderWidth(0, 1, 0, 0).BorderColor(p.hair).Children(func() {
		for _, sec := range settingsSections {
			sec := sec
			active := a.settingsNav == sec.Key
			b := ui.ButtonBase(c).Key("settings-nav-" + sec.Key).Height(32).PaddingX(9).Gap(9).Radius(8).
				AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(tr(sec.Label))
			bg := ui.Transparent
			switch {
			case active:
				bg = p.press
			case b.Hovered():
				bg = p.hover
			}
			b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
			color := p.text
			if active {
				color = p.accent
			}
			b.Children(func() {
				ui.Icon(c, sec.Icon).Size(15, 15).TextColor(color)
				ui.Text(c, tr(sec.Label)).FontSize(12.5).FontWeight(500).TextColor(color)
			})
			if b.Clicked() {
				a.settingsNav = sec.Key
				a.settingsSection = sec.Key
				a.redraw()
			}
		}
	})
}

// releaseLinkRow shows the releases page and copies its URL, for builds that
// cannot install updates themselves.
func (a *app) releaseLinkRow(c *ui.Context, p palette, key string) {
	url := releasePageURL()
	ui.Column(c).FillWidth().Padding(12, 15).Gap(8).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, tr("app.common.download_link")).FontSize(13.5).FontWeight(500)
			ui.Text(c, tr("app.settings.copy_it_into_browser_or_open")).FontSize(11.5).TextColor(p.muted)
		})
		ui.Text(c, url).FontSize(11.5).TextColor(p.text).Selectable()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key(key+"-copy-release"), tr("app.common.copy_link")).Clicked() {
				c.WriteClipboard(url)
				a.setToast(tr("app.common.download_link_copied"))
			}
			if ui.Button(c.Key(key+"-open-release"), tr("app.common.open_release_page")).Clicked() {
				a.openExternal(url)
			}
		})
	})
}

func (a *app) settingsChanged() {
	_ = a.settings.save()
	a.settings.applyAppearance()
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAcc
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.settingsWin != nil {
			a.settingsWin.Close()
		}
	})
	s := &a.settings
	groups := make(map[string]ui.Element, len(settingsSections))

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.settingsMenu(c, p)
		ui.Scroll(c).Grow(1).Padding(24, 26, 30, 26).Gap(6).Children(func() {
			ui.Text(c, tr("app.common.settings")).FontSize(26).FontWeight(750).LetterSpacing(-0.4).Margin(0, 0, 6, 2)
			ui.Text(c, tr("app.settings.these_settings_only_affect_this_device")).FontSize(12).TextColor(p.muted).Margin(0, 0, 0, 8)

			group := func(title string, rows func()) ui.Element {
				ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(p.muted).Margin(14, 4, 4, 4)
				return ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hair).Dividers(1, p.hair).Children(rows)
			}
			row := func(label, desc string, control func()) {
				ui.Row(c).Padding(12, 15).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, label).FontSize(13.5).FontWeight(500)
						if desc != "" {
							ui.Text(c, desc).FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
						}
					})
					control()
				})
			}

			groups["profile"] = group(tr("app.common.profile"), func() {
				row(tr("app.common.name"), tr("app.settings.name_other_devices_see_on_lan"), func() {
					ui.TextInput(c.Key("profile-name"), &a.profileName).Label(tr("app.common.name")).Width(210).
						Placeholder(tr("app.common.my_zchat")).OnSubmit(a.applyProfile)
				})
				row(tr("app.common.bio"), tr("app.settings.short_bio_shown_on_your_device"), func() {
					ui.TextInput(c.Key("profile-signature"), &a.profileSig).Label(tr("app.common.bio")).Width(210).
						Placeholder(tr("app.common.optional")).OnSubmit(a.applyProfile)
				})
				ui.Row(c).Padding(12, 15).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Avatar(c, a.profileName, a.avatarBitmap(a.profile.Avatar)).Size(46, 46)
					ui.Column(c).Grow(1).Gap(3).Children(func() {
						ui.Text(c, tr("app.settings.avatar")).FontSize(13.5).FontWeight(500)
						ui.Text(c, tr("app.settings.choose_image_it_is_scaled_down")).FontSize(11.5).TextColor(p.muted)
					})
					if a.profile.Avatar != "" {
						if ui.Button(c.Key("remove-avatar"), tr("app.common.remove")).Clicked() {
							a.profile.Avatar = ""
							a.applyProfile()
						}
					}
					if ui.Button(c.Key("pick-avatar"), tr("app.common.select")).Clicked() {
						a.pickAvatar()
					}
					if ui.PrimaryButton(c.Key("save-profile"), tr("app.settings.save_profile")).Clicked() {
						a.applyProfile()
					}
				})
				row(tr("app.common.status"), tr("app.settings.your_status_is_broadcast_to_connected"), func() {
					labels := make([]string, len(model.PresenceOptions))
					chosen := presenceLabel(a.profile.Status)
					for i, o := range model.PresenceOptions {
						labels[i] = tr(o.Label)
						if o.Value == a.profile.Status {
							chosen = tr(o.Label)
						}
					}
					if ui.Select(c.Key("status"), &chosen, labels).Label(tr("app.common.status")).Width(140).Changed() {
						for _, o := range model.PresenceOptions {
							if tr(o.Label) == chosen {
								a.setStatus(o.Value)
							}
						}
					}
				})
			})

			groups["appearance"] = group(tr("app.settings.appearance"), func() {
				row(tr("app.settings.language"), tr("app.settings.interface_language_changes_apply_immediately"), func() {
					opts := i18n.Options()
					labels := make([]string, len(opts))
					chosen := ""
					for i, o := range opts {
						labels[i] = tr(o.Label)
						if o.Value == s.Language {
							chosen = labels[i]
						}
					}
					if chosen == "" {
						chosen = labels[0]
					}
					if ui.Select(c.Key("language"), &chosen, labels).Label(tr("app.settings.language")).Width(160).Changed() {
						for i, label := range labels {
							if label == chosen {
								s.Language = opts[i].Value
							}
						}
						a.settingsChanged()
						a.applyLanguage()
					}
				})
				row(tr("app.common.theme"), tr("app.settings.follow_system_or_fix_light_or"), func() {
					labels := []string{tr("app.common.use_system_setting"), tr("app.settings.light"), tr("app.settings.dark")}
					values := []string{"system", "light", "dark"}
					chosen := labels[0]
					for i, v := range values {
						if v == s.Appearance {
							chosen = labels[i]
						}
					}
					if ui.Select(c.Key("appearance"), &chosen, labels).Label(tr("app.common.theme")).Width(140).Changed() {
						for i, label := range labels {
							if label == chosen {
								s.Appearance = values[i]
							}
						}
						a.settingsChanged()
					}
				})
			})

			groups["notifications"] = group(tr("app.settings.notifications"), func() {
				row(tr("app.settings.message_notifications"), tr("app.settings.show_system_notifications_when_window_is"), func() {
					if ui.Switch(c.Key("notifications"), &s.Notifications).Label(tr("app.settings.message_notifications")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("app.settings.auto_download_images"), tr("app.settings.cache_received_images_locally_open_chat"), func() {
					if ui.Switch(c.Key("auto-images"), &s.AutoDownloadImages).Label(tr("app.settings.auto_download_images")).Changed() {
						a.settingsChanged()
						if s.AutoDownloadImages {
							a.queueActiveImages()
						}
					}
				})
				row(tr("app.settings.keep_running_in_tray_when_window"), tr("app.settings.keep_receiving_messages_after_closing_window"), func() {
					if ui.Switch(c.Key("close-tray"), &s.CloseToTray).Label(tr("app.settings.keep_running_in_tray_when_window")).Changed() {
						a.settingsChanged()
					}
				})
			})

			groups["network"] = group(tr("app.common.network"), func() {
				row(tr("app.settings.discover_devices_automatically"), tr("app.settings.find_other_zchat_devices_on_lan"), func() {
					if ui.Switch(c.Key("discovery"), &s.Discovery).Label(tr("app.settings.discover_devices_automatically")).Changed() {
						a.settingsChanged()
						a.restartNetwork()
					}
				})
				row(tr("app.common.listen_port"), tr("app.settings.tcp_port_default_42224_restart_network"), func() {
					if ui.NumberInput(c.Key("tcp-port"), &a.portEdit, 1024, 65535, 1).Label(tr("app.common.listen_port")).Width(110).Changed() {
						s.Port = int(a.portEdit)
						_ = s.save()
					}
				})
				row(tr("app.common.file_transfer_port"), tr("app.settings.chat_attachments_transfer_directly_over_this"), func() {
					if ui.NumberInput(c.Key("transfer-port"), &a.transferEdit, 1024, 65535, 1).Label(tr("app.common.file_transfer_port")).Width(110).Changed() {
						s.TransferPort = int(a.transferEdit)
						_ = s.save()
					}
				})
				row(tr("app.settings.local_address"), tr("app.settings.give_these_addresses_to_devices_that"), func() {
					ip := netutil.PrimaryIP()
					if ip == "" {
						ip = tr("app.settings.not_connected_to_lan")
					}
					ui.Text(c, ip+":"+itoa(a.nodePort())).FontSize(12).TextColor(p.muted).FontFeatures("tnum")
				})
				row(tr("app.settings.restart_network_2"), tr("app.settings.apply_port_file_transfer_or_discovery"), func() {
					if ui.Button(c.Key("restart-network"), tr("app.settings.restart_network")).Clicked() {
						s.Port = int(a.portEdit)
						s.TransferPort = int(a.transferEdit)
						_ = s.save()
						a.restartNetwork()
						a.setToast(tr("app.settings.network_restarted"))
					}
				})
			})

			groups["files"] = group(tr("app.common.files"), func() {
				row(tr("app.common.downloads"), a.settings.DownloadDir, func() {
					if ui.Button(c.Key("choose-downloads"), tr("app.common.select")).Clicked() {
						a.chooseDownloadDir()
					}
				})
				row(tr("app.common.shared_folder"), tr("app.settings.enable_it_on_file_sharing_page"), func() {
					if ui.Button(c.Key("open-share-page"), tr("app.settings.open_sharing")).Clicked() {
						a.showMain()
						a.setPage(pageShare)
					}
				})
				row(tr("app.common.upload_folder"), tr("app.settings.after_sharing_is_enabled_on_file"), func() {
					if ui.Button(c.Key("open-upload-page"), tr("app.settings.open_sharing")).Clicked() {
						a.showMain()
						a.setPage(pageShare)
					}
				})
				row(tr("app.settings.share_port"), tr("app.settings.http_service_port_default_42230"), func() {
					if ui.NumberInput(c.Key("share-port"), &a.shareEdit, 1024, 65535, 1).Label(tr("app.settings.share_port")).Width(110).Changed() {
						s.SharePort = int(a.shareEdit)
						_ = s.save()
						// Do not turn the attachment fallback server into an
						// explicit share (and thereby enable uploads) just
						// because the port changed.
						if a.shareExplicit {
							a.startShare(false)
						}
					}
				})
				row(tr("app.settings.share_automatically_on_startup"), tr("app.settings.reopen_last_shared_folder_when_zchat"), func() {
					if ui.Switch(c.Key("share-start"), &s.ShareOnStart).Label(tr("app.settings.share_automatically_on_startup")).Changed() {
						a.settingsChanged()
					}
				})
			})

			groups["screenshot"] = group(tr("app.common.screenshot"), func() {
				row(tr("app.settings.enable_screenshots"), tr("app.settings.use_screenshots_from_chat_toolbar_menus"), func() {
					if ui.Switch(c.Key("shot-enabled"), &s.ScreenshotEnabled).Label(tr("app.settings.enable_screenshots")).Changed() {
						a.settingsChanged()
						a.applyScreenshotShortcut()
						a.setMenu()
						a.rebuildTray()
					}
				})
				row(tr("app.settings.include_chat_window_by_default"), tr("app.settings.include_current_chat_window_by_default"), func() {
					if ui.Switch(c.Key("shot-include-chat"), &s.ScreenshotIncludeChat).Label(tr("app.settings.include_chat_window_by_default")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("app.settings.shortcut_with_chat_window"), tr("app.settings.include_chat_window_leave_empty_or"), func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c.Key("shot-include-shortcut"), &a.shotIncludeEdit).Width(170).
							Placeholder("Ctrl+F1")
						if ui.Button(c.Key("shot-include-apply"), tr("app.common.apply")).Clicked() {
							if a.setScreenshotShortcut(a.shotIncludeEdit, true) {
								a.setToast(tr("app.settings.screenshot_shortcut_updated"))
							}
						}
					})
				})
				row(tr("app.settings.shortcut_without_chat_window"), tr("app.settings.hide_zchat_first_leave_empty_or"), func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c.Key("shot-exclude-shortcut"), &a.shotExcludeEdit).Width(170).
							Placeholder("Ctrl+F2")
						if ui.Button(c.Key("shot-exclude-apply"), tr("app.common.apply")).Clicked() {
							if a.setScreenshotShortcut(a.shotExcludeEdit, false) {
								a.setToast(tr("app.settings.screenshot_shortcut_updated"))
							}
						}
					})
				})
				row(tr("app.common.save_folder"), a.screenshotDir(), func() {
					if ui.Button(c.Key("choose-screenshot-dir"), tr("app.common.select")).Clicked() {
						a.chooseScreenshotDir()
					}
				})
				row(tr("app.settings.capture_mouse_pointer"), tr("app.settings.include_mouse_pointer_in_screenshot"), func() {
					if ui.Switch(c.Key("shot-cursor"), &s.ScreenshotCursor).Label(tr("app.settings.capture_mouse_pointer")).Changed() {
						a.settingsChanged()
					}
				})
				row(tr("app.settings.take_screenshot_now"), tr("app.settings.select_area_to_annotate_copy_save"), func() {
					if ui.Button(c.Key("shot-now"), tr("app.common.screenshot")).Clicked() {
						a.openScreenshotChooser()
					}
				})
			})

			groups["history"] = group(tr("app.common.chat_history"), func() {
				st := a.dbStats
				row(tr("app.settings.sqlite_database"), tr("app.settings.records_are_indexed_by_chat_and"), func() {
					ui.Column(c).AlignItems(ui.End).Gap(2).Children(func() {
						ui.Text(c, itoa(st.Messages)+tr("app.settings.messages")).FontSize(12).TextColor(p.muted).FontFeatures("tnum")
						ui.Text(c, itoa(st.Conversations)+tr("app.settings.conversations")+model.HumanSize(st.Bytes)).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
					})
				})
				row(tr("app.settings.open_database_location"), tr("app.settings.you_can_back_up_zchat_db"), func() {
					if ui.Button(c.Key("open-db-dir"), tr("app.common.open")).Clicked() {
						if a.store != nil {
							a.openPath(a.store.Dir())
						}
					}
				})
				row(tr("app.settings.compact_database"), tr("app.settings.reclaim_disk_space_after_deleting_chat"), func() {
					if ui.Button(c.Key("vacuum-db"), tr("app.common.compact")).Clicked() {
						if a.store != nil {
							if err := a.store.Vacuum(); err != nil {
								a.setToast(tr("app.settings.compaction_failed") + err.Error())
							} else {
								a.refreshDBStats()
								a.setToast(tr("app.settings.database_compacted"))
							}
							a.redraw()
						}
					}
				})
				row(tr("app.settings.clear_all_chat_history"), tr("app.settings.delete_messages_in_all_chats_keeping"), func() {
					if ui.Button(c.Key("clear-all-history"), tr("app.settings.clear")).Clicked() {
						a.confirm(tr("app.settings.clear_all_chat_history"), tr("app.settings.this_deletes_messages_in_every_chat"), a.clearAllHistory)
					}
				})
			})

			groups["update"] = group(tr("app.common.update"), func() {
				if !a.updateSupported {
					// Builds that cannot replace themselves, such as a .deb in
					// /opt, an AppImage or a development build, link to the
					// release page instead of pretending to update.
					row(tr("app.settings.current_version")+a.version, tr("app.common.this_build_cannot_update_itself_download"), func() {
						ui.Text(c, tr("app.settings.manual_update")).FontSize(11.5).TextColor(p.faint)
					})
					a.releaseLinkRow(c, p, "update")
					return
				}
				desc := tr("app.settings.check_for_updates_once_day")
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += tr("app.settings.last_checked") + shortStamp(t, time.Now())
				}
				row(tr("app.settings.check_for_updates_automatically"), desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label(tr("app.settings.check_for_updates_automatically")).Changed() {
						setUpdateChecks(on)
					}
				})
				row(tr("app.settings.download_and_install_automatically"), tr("app.settings.download_new_versions_in_background_they"), func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label(tr("app.settings.download_and_install_automatically")).Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row(tr("app.settings.current_version")+a.version, tr("app.settings.get_updates_from_github_releases"), func() {
					if ui.Button(c.Key("check-updates"), tr("app.common.check_for_updates")).Clicked() {
						checkForUpdates()
					}
				})
			})

			groups["about"] = group(tr("app.common.about"), func() {
				row("zchat", tr("app.settings.lan_chat_no_server_involved"), func() {
					ui.Text(c, "v"+a.version).FontSize(12).TextColor(p.muted)
				})
				row(tr("app.common.project_page"), "github.com/zwdart/zchat", func() {
					if ui.Button(c.Key("project-home"), tr("app.common.open")).Clicked() {
						a.openExternal("https://github.com/zwdart/zchat")
					}
				})
				row(tr("app.common.data_folder"), tr("app.settings.your_profile_chat_history_and_file"), func() {
					if ui.Button(c.Key("open-data"), tr("app.common.open")).Clicked() {
						if a.store != nil {
							a.openPath(a.store.Dir())
						}
					}
				})
			})

			ui.Spacer(c).Height(20)
		})
	})

	if a.shotSettingsFocus {
		a.shotSettingsFocus = false
		a.settingsSection = "screenshot"
		a.settingsNav = "screenshot"
	}
	if a.settingsSection != "" {
		// The sections are built every frame; scroll as soon as the target
		// exists rather than keeping a reference across frames.
		if el, ok := groups[a.settingsSection]; ok {
			el.ScrollIntoView()
		}
		a.settingsSection = ""
	}
}

func (a *app) applyProfile() {
	name := strings.TrimSpace(a.profileName)
	if name == "" {
		name = tr("app.common.my_zchat")
		a.profileName = name
	}
	a.profile.Name = name
	a.profile.Signature = strings.TrimSpace(a.profileSig)
	// Keep the names on messages already sent by this profile current.
	for _, msgs := range a.msgs {
		for i := range msgs {
			if msgs[i].SenderID == a.profile.ID && msgs[i].Mine {
				msgs[i].SenderName = a.profile.Name
			}
		}
	}
	if a.node != nil {
		a.node.SetProfile(a.profile)
	}
	a.scheduleSave()
	a.setToast(tr("app.settings.profile_saved"))
	a.redraw()
}

func (a *app) pickAvatar() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.settingsWin,
			Title:       tr("app.settings.choose_avatar"),
			ButtonLabel: tr("app.common.use"),
			Filters:     []mygo.FileFilter{{Name: tr("app.common.images"), Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		encoded, err := avatarBase64(paths[0])
		if err != nil {
			a.post(func() { a.setToast(tr("app.settings.could_not_read_image") + err.Error()) })
			return
		}
		a.post(func() {
			a.profile.Avatar = encoded
			a.applyProfile()
		})
	}()
}

// avatarBase64 reads an image, scales it small and returns a PNG in base64,
// as the profile and announcements carry it.
func avatarBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	const side = 192
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return "", image.ErrFormat
	}
	scale := min(float64(side)/float64(w), float64(side)/float64(h))
	if scale > 1 {
		scale = 1
	}
	tw := max(1, int(float64(w)*scale))
	th := max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
