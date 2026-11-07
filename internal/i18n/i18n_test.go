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

func TestResolve(t *testing.T) {
	if Resolve(English) != English || Resolve(Chinese) != Chinese {
		t.Fatal("explicit languages did not resolve")
	}
	if got := Resolve(System); got != Chinese && got != English {
		t.Fatalf("system resolved to %q", got)
	}
}
