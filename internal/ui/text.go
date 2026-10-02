package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"strings"
	"sync"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/ui/web"
)

// A notification's title is one line: a process or device name longer than this is cut.
const notifyNameRunes = 60

// Text is a string of the web/i18n dictionaries: its key and the values of its {placeholders}.
// The frontend translates it; Translate does the same for text shown outside the web views.
type Text struct {
	Key    string         `json:"key"`
	Params map[string]any `json:"params,omitempty"`
}

var dictionaries = sync.OnceValue(func() map[string]map[string]any {
	all := make(map[string]map[string]any, len(settings.Languages))
	for _, lang := range settings.Languages {
		dict, err := readDictionary(web.FS, lang)
		if err != nil {
			slog.Error("dictionary skipped", "lang", lang, "err", err)
			continue
		}
		all[lang] = dict
	}
	return all
})

// readDictionary parses i18n/<lang>.js: one assignment whose right-hand side is strict JSON.
// A value is a string, or an object of plural forms keyed by CLDR category.
func readDictionary(assets fs.FS, lang string) (map[string]any, error) {
	raw, err := fs.ReadFile(assets, "i18n/"+lang+".js")
	if err != nil {
		return nil, fmt.Errorf("read dictionary: %w", err)
	}
	start, end := bytes.IndexByte(raw, '{'), bytes.LastIndexByte(raw, '}')
	if start < 0 || end < start {
		return nil, errors.New("parse dictionary: no object literal")
	}
	var dict map[string]any
	if err := json.Unmarshal(raw[start:end+1], &dict); err != nil {
		return nil, fmt.Errorf("parse dictionary: %w", err)
	}
	return dict, nil
}

// Translate renders a dictionary string for a notification or the status item's menu.
// A key the language lacks falls back to English, then to the key itself.
// Numbers print the Go way ("4.2", not "4,2") and plural forms are not resolved;
// no string used outside the web views has either. Port t() from app.js if one ever does.
func Translate(lang, key string, params map[string]any) string {
	all := dictionaries()
	text, ok := all[lang][key].(string)
	if !ok {
		text, ok = all["en"][key].(string)
	}
	if !ok {
		return key
	}
	pairs := make([]string, 0, 2*len(params))
	for name, value := range params {
		pairs = append(pairs, "{"+name+"}", fmt.Sprint(value))
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// AlertTexts renders the notification of an alert. The name of a process or a device is its
// own choice and lands in a system notification under mac-pulse's name, so every string
// value is cleaned and cut.
func AlertTexts(lang string, a alerts.Alert) (title, body string) {
	params := maps.Clone(a.Params)
	for name, value := range params {
		if text, ok := value.(string); ok {
			params[name] = collector.CleanText(text, notifyNameRunes)
		}
	}
	return Translate(lang, "alert."+a.Kind+".title", params), Translate(lang, "alert."+a.Kind+".detail", params)
}

// processKey is the dictionary key that describes a well-known macOS process, "" for a name
// the English dictionary has no "proc." string for. A family of processes shares one string.
func processKey(name string) string {
	switch {
	case strings.HasPrefix(name, "com.apple.WebKit."):
		name = "com.apple.WebKit"
	case strings.HasPrefix(name, "mdworker"):
		name = "mdworker"
	case strings.HasPrefix(strings.ToLower(name), "xprotect"):
		name = "XProtect"
	}
	if _, ok := dictionaries()["en"]["proc."+name].(string); !ok {
		return ""
	}
	return "proc." + name
}
