// Package i18n keeps the app's small translation table. Chinese source
// strings are the keys; missing English entries fall back to Chinese, so
// adding a string never breaks the UI.
package i18n

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"
)

//go:embed en.tsv
var englishTable string

// Language settings accepted by Set.
const (
	System  = "system"
	Chinese = "zh"
	English = "en"
)

var english = parseTable(englishTable)
var (
	mu      sync.RWMutex
	current = Chinese
)

// Set applies a language setting: "system", "zh", or "en".
func Set(setting string) {
	lang := Chinese
	switch setting {
	case English:
		lang = English
	case Chinese:
		lang = Chinese
	default:
		lang = systemLanguage()
	}
	mu.Lock()
	current = lang
	mu.Unlock()
}

// Current returns the effective language, "zh" or "en".
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T translates a Chinese source string.
func T(s string) string {
	mu.RLock()
	lang := current
	mu.RUnlock()
	if lang != English {
		return s
	}
	if v, ok := english[s]; ok {
		return v
	}
	return s
}

// TF translates a format string and applies args.
func TF(format string, args ...any) string {
	return fmt.Sprintf(T(format), args...)
}

// Option is a language choice for settings.
type Option struct {
	Value string
	Label string
}

// Options lists the settings choices. Labels are translated at call time.
func Options() []Option {
	return []Option{
		{System, "跟随系统"},
		{Chinese, "简体中文"},
		{English, "English"},
	}
}

// Resolve turns a setting into a concrete language.
func Resolve(setting string) string {
	if setting == English {
		return English
	}
	if setting == Chinese {
		return Chinese
	}
	return systemLanguage()
}

func systemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.ToLower(os.Getenv(key))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "zh") {
			return Chinese
		}
		return English
	}
	// The app began as a Chinese-only client; use Chinese when the
	// environment gives no hint.
	return Chinese
}

func parseTable(s string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		out[unescape(key)] = unescape(value)
	}
	return out
}

func unescape(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}
