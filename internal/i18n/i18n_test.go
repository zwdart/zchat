package i18n

import (
	"strings"
	"testing"
)

func TestEnglishTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	if got := T("app.common.settings"); got != "Settings" {
		t.Fatalf("T(设置) = %q", got)
	}
	// Model/spec data still carries Chinese display labels. T must resolve
	// them through the source index rather than showing the raw label.
	if got := T("设置"); got != "Settings" {
		t.Fatalf("T(Chinese label) = %q", got)
	}
	if got := T("未知设备"); got != "Unknown device" {
		t.Fatalf("T(未知设备) = %q", got)
	}
	if got := TF("app.common.size_mismatch_received_expected", 3, 4); got != "Size mismatch: received 3, expected 4" {
		t.Fatalf("TF = %q", got)
	}
	if got := TF("app.common.could_not_create_download_folder", `C:\x`, "denied"); got != `Could not create download folder C:\x: denied` {
		t.Fatalf("TF download folder = %q", got)
	}
	if got := T("没有这条翻译"); got != "没有这条翻译" {
		t.Fatalf("missing key should fall back, got %q", got)
	}
	Set(Chinese)
	if got := T("app.common.settings"); got != "设置" {
		t.Fatalf("Chinese T = %q", got)
	}
}

func TestOptionsUseMessageIDs(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	want := map[string]string{
		System:  "Use system setting",
		Chinese: "Simplified Chinese",
		English: "English",
	}
	for _, opt := range Options() {
		if _, ok := want[opt.Value]; !ok {
			t.Fatalf("unexpected option value %q", opt.Value)
		}
		if opt.Label == "" {
			t.Fatalf("empty label for %q", opt.Value)
		}
		if got := T(opt.Label); got != want[opt.Value] {
			t.Errorf("T(%q) = %q, want %q", opt.Label, got, want[opt.Value])
		}
	}
}

// Every string the screenshot settings and its menu show has an English
// translation, so switching the language changes them too.
func TestScreenshotSettingsTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	for _, key := range []string{
		"app.common.screenshot",
		"app.common.screenshot_2",
		"app.chat.screenshot_options",
		"app.chat.include_window_in_screenshot",
		"app.chat.hide_window_in_screenshot",
		"app.common.settings_2",
		"app.settings.include_chat_window_by_default",
		"app.settings.include_current_chat_window_by_default",
		"app.common.include_current_chat_window",
		"app.common.do_not_include_current_chat_window",
		"app.settings.shortcut_with_chat_window",
		"app.settings.shortcut_without_chat_window",
		"app.settings.include_chat_window_leave_empty_or",
		"app.settings.hide_zchat_first_leave_empty_or",
		"app.common.shortcuts",
		"app.common.not_set",
		"app.common.send",
	} {
		if got := T(key); got == key {
			t.Errorf("T(%q) has no English translation", key)
		}
	}
}

// The FFmpeg toolkit's capability names and section headings need English
// text too, so switching the language does not leave half the page Chinese.
func TestFFmpegTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	for _, key := range []string{
		"app.ffmpeg.compress_video_2", "app.ffmpeg.trim_video", "app.ffmpeg.merge_videos_2", "app.ffmpeg.extract_audio", "app.ffmpeg.video_to_gif",
		"app.ffmpeg.add_watermark", "app.ffmpeg.picture_adjustment", "app.ffmpeg.subtitles_and_music", "app.ffmpeg.speed_change", "app.ffmpeg.video_snapshot",
		"app.common.batch_processing", "app.ffmpeg.compression_settings", "app.ffmpeg.trim_range", "app.ffmpeg.merge_settings", "app.ffmpeg.audio_settings",
		"app.ffmpeg.gif_settings", "app.common.watermark_settings", "app.ffmpeg.capture_settings", "app.common.batch_settings", "app.ffmpeg.restore_recommended",
		"app.common.start_processing", "app.common.output_folder", "app.common.choose_capability",
	} {
		if got := T(key); got == key {
			t.Errorf("T(%q) has no English translation", key)
		}
	}
}

// The shared file-info dialog is reachable from both toolkit pages, so its
// labels and formatting units need English text as well.
func TestFileInfoTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	for _, key := range []string{
		"app.common.view_file_details", "app.file.media_details", "app.file.video_details", "app.file.audio_details", "app.common.image_details", "app.file.reading_file_details",
		"app.common.file_size", "app.common.duration", "app.file.total_bitrate", "app.file.video_codec", "app.file.modified", "app.file.file_path",
		"app.common.transparent", "app.file.color_space", "app.file.color_channels", "app.file.bit_depth", "app.common.bit", "app.file.image_type",
		"app.file.print_resolution", "app.file.frames", "app.common.frames", "app.common.quality", "app.file.no_audio_track", "app.file.mono",
		"app.file.stereo", "app.file.channels", "app.file.h", "app.common.min", "app.common.yes", "app.common.no", "app.file.copy_path",
		"app.file.path_copied", "app.ffmpeg.ffprobe_was_not_found_so_video",
		"app.image.imagemagick_s_identify_command_was_not", "app.image.could_not_read_image_details",
		"app.file.bilevel", "app.file.grayscale", "app.file.palette", "app.file.truecolor", "app.file.color_separation", "app.common.optimized", "app.file.with_alpha", "app.common.unknown",
	} {
		if got := T(key); got == key {
			t.Errorf("T(%q) has no English translation", key)
		}
	}
}

// The catalogs are edited by hand, so keep their keys in the same order. A
// mismatch is caught here instead of showing up as a missing label.
func TestCatalogsMatch(t *testing.T) {
	zh, en := catalogs[Chinese], catalogs[English]
	if zh == nil || en == nil {
		t.Fatal("missing catalog")
	}
	if len(zh.duplicates) > 0 {
		t.Fatalf("zh catalog has duplicate keys: %v", zh.duplicates)
	}
	if len(en.duplicates) > 0 {
		t.Fatalf("en catalog has duplicate keys: %v", en.duplicates)
	}
	if len(zh.keys) != len(en.keys) {
		t.Fatalf("key counts differ: zh=%d en=%d", len(zh.keys), len(en.keys))
	}
	for i := range zh.keys {
		if zh.keys[i] != en.keys[i] {
			t.Fatalf("key %d differs: zh=%q en=%q", i+1, zh.keys[i], en.keys[i])
		}
	}
	for _, key := range en.keys {
		if _, ok := zh.values[key]; !ok {
			t.Fatalf("zh catalog is missing %q", key)
		}
	}
}

// Runtime labels in model/spec data are Chinese values. Every one must map
// back to its message ID so English never leaks a raw Chinese label.
func TestSourceLabelsResolve(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	for _, key := range catalogs[Chinese].keys {
		source := catalogs[Chinese].values[key]
		want := catalogs[English].values[key]
		if got := T(source); got != want {
			t.Errorf("T(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestKeysUseDotNotation(t *testing.T) {
	for _, key := range catalogs[Chinese].keys {
		parts := strings.Split(key, ".")
		if len(parts) < 3 {
			t.Fatalf("key %q is not dot-separated", key)
		}
		for _, part := range parts {
			if part == "" {
				t.Fatalf("key %q has an empty segment", key)
			}
			for _, r := range part {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
					t.Fatalf("key %q has an unsupported character %q", key, r)
				}
			}
		}
	}
}

func TestResolve(t *testing.T) {
	if Resolve(English) != English || Resolve(Chinese) != Chinese {
		t.Fatal("explicit languages did not resolve")
	}
	if got := Resolve(System); got != Chinese && got != English {
		t.Fatalf("system resolved to %q", got)
	}
}
