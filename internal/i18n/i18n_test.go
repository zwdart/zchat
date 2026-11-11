package i18n

import "testing"

func TestEnglishTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	if got := T("设置"); got != "Settings" {
		t.Fatalf("T(设置) = %q", got)
	}
	if got := TF("文件大小不一致：收到 %d，预期 %d", 3, 4); got != "Size mismatch: received 3, expected 4" {
		t.Fatalf("TF = %q", got)
	}
	if got := TF("无法创建下载目录 %s：%v", `C:\x`, "denied"); got != `Could not create download folder C:\x: denied` {
		t.Fatalf("TF download folder = %q", got)
	}
	if got := T("没有这条翻译"); got != "没有这条翻译" {
		t.Fatalf("missing key should fall back, got %q", got)
	}
	Set(Chinese)
	if got := T("设置"); got != "设置" {
		t.Fatalf("Chinese T = %q", got)
	}
}

// Every string the screenshot settings and its menu show has an English
// translation, so switching the language changes them too.
func TestScreenshotSettingsTranslations(t *testing.T) {
	old := Current()
	defer Set(old)
	Set(English)
	for _, key := range []string{
		"截图",
		"截图…",
		"截图选项",
		"显示窗口截图",
		"隐藏窗口截图",
		"设置…",
		"默认包含聊天页面",
		"点击聊天工具栏的截图按钮时，默认把当前聊天页面一起截入",
		"包含当前聊天窗口",
		"不包含当前聊天窗口",
		"包含聊天窗口的快捷键",
		"不包含聊天窗口的快捷键",
		"连当前聊天窗口一起截取；留空或填 off 可关闭",
		"先隐藏 zchat 再截取；留空或填 off 可关闭",
		"快捷键",
		"未设置",
		"发送",
	} {
		if got := T(key); got == key {
			t.Errorf("T(%q) has no English translation", key)
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
