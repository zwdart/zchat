// Package i18n keeps the app's translation catalogs. Each language lives in
// its own file under locales. Catalogs use dot-separated message IDs such as
// "app.ffmpeg.compress_video"; a missing entry falls back to Chinese and then
// to the ID itself instead of breaking the UI.
package i18n

import (
	"embed"
	"fmt"
	"os"
	"strings"
	"sync"
)

//go:embed locales/*.tsv
var localeFiles embed.FS

// Language settings accepted by Set.
const (
	System  = "system"
	Chinese = "zh"
	English = "en"
)

type catalog struct {
	keys       []string
	values     map[string]string
	duplicates []string
}

var catalogs = map[string]*catalog{
	Chinese: loadCatalog(Chinese),
	English: loadCatalog(English),
}

// sourceKeys indexes Chinese display labels that are stored in model/spec
// data and translated when they are rendered. It keeps those runtime labels
// working while the catalogs themselves use message IDs.
var sourceKeys = buildSourceKeys()

func buildSourceKeys() map[string]string {
	c := catalogs[Chinese]
	if c == nil {
		return nil
	}
	index := make(map[string]string, len(c.keys))
	for _, key := range c.keys {
		source := c.values[key]
		if source == "" {
			continue
		}
		if _, exists := index[source]; !exists {
			index[source] = key
		}
	}
	return index
}

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

// T translates a message ID. Missing entries fall back to Chinese and then to
// the input, so a new string never leaves a blank label. Chinese runtime
// labels are resolved through sourceKeys before the fallback.
func T(s string) string {
	mu.RLock()
	lang := current
	mu.RUnlock()
	key := s
	if c := catalogs[lang]; c != nil {
		if _, ok := c.values[key]; !ok {
			if resolved, ok := sourceKeys[s]; ok {
				key = resolved
			}
		}
	}
	if c := catalogs[lang]; c != nil {
		if v, ok := c.values[key]; ok {
			return v
		}
	}
	if lang != Chinese {
		if c := catalogs[Chinese]; c != nil {
			if v, ok := c.values[key]; ok {
				return v
			}
		}
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
		{System, "app.common.use_system_setting"},
		{Chinese, "app.common.simplified_chinese"},
		{English, "app.common.english"},
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

func loadCatalog(lang string) *catalog {
	data, err := localeFiles.ReadFile("locales/" + lang + ".tsv")
	if err != nil {
		return &catalog{values: map[string]string{}}
	}
	return parseCatalog(string(data))
}

func parseCatalog(s string) *catalog {
	c := &catalog{values: make(map[string]string)}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		key, value = unescape(key), unescape(value)
		if _, exists := c.values[key]; exists {
			c.duplicates = append(c.duplicates, key)
		} else {
			c.keys = append(c.keys, key)
		}
		c.values[key] = value
	}
	return c
}

func unescape(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}
