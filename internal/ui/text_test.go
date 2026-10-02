package ui

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/ui/web"
)

var placeholder = regexp.MustCompile(`\{\w+\}`)

// placeholders lists the {names} of a dictionary value: of the string, or of every plural form.
func placeholders(t *testing.T, value any) []string {
	t.Helper()
	var found []string
	switch v := value.(type) {
	case string:
		found = placeholder.FindAllString(v, -1)
	case map[string]any:
		require.Contains(t, v, "other", "a plural needs the form every language falls back to")
		for _, form := range v {
			found = append(found, placeholder.FindAllString(form.(string), -1)...)
		}
	default:
		require.Failf(t, "unexpected value", "%T", value)
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// Every language must be complete: a missing key shows English in the middle of a
// translated screen. A translation may drop a {placeholder} for want of room, but one
// the English string lacks is never filled and would show its braces.
func TestReadDictionary(t *testing.T) {
	t.Parallel()

	en, err := readDictionary(web.FS, "en")
	require.NoError(t, err)
	page, err := os.ReadFile("web/index.html")
	require.NoError(t, err)
	files, err := web.FS.ReadDir("i18n")
	require.NoError(t, err)
	require.Len(t, files, len(settings.Languages), "one dictionary per language of the settings")

	for _, lang := range settings.Languages {
		t.Run(lang, func(t *testing.T) {
			t.Parallel()

			dict, err := readDictionary(web.FS, lang)

			require.NoError(t, err)
			assert.ElementsMatch(t, slices.Collect(maps.Keys(en)), slices.Collect(maps.Keys(dict)))
			for key, value := range dict {
				assert.Subset(t, placeholders(t, en[key]), placeholders(t, value), key)
			}
			assert.NotEmpty(t, dict["lang.name"])
			assert.Contains(t, string(page), `<script src="i18n/`+lang+`.js"></script>`)
		})
	}
}

func TestReadDictionary_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lang string
		body string
	}{
		{name: "no such file", lang: "tlh"},
		{name: "no object literal", lang: "xx", body: `window.MP_I18N["xx"] = 1;`},
		{name: "not strict JSON", lang: "xx", body: `window.MP_I18N["xx"] = {key: 'value'};`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			require.NoError(t, os.Mkdir(dir+"/i18n", 0o700))
			require.NoError(t, os.WriteFile(dir+"/i18n/xx.js", []byte(tt.body), 0o600))

			_, err := readDictionary(os.DirFS(dir), tt.lang)

			require.Error(t, err)
		})
	}
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		lang   string
		key    string
		params map[string]any
		want   string
	}{
		{name: "plain string", lang: "en", key: "menu.open", want: "Open mac-pulse"},
		{name: "another language", lang: "ru", key: "menu.open", want: "Открыть mac-pulse"},
		{
			name: "placeholders, whole and fractional numbers", lang: "en", key: "alert.disk.detail",
			params: map[string]any{"value": 4.2, "limit": 10}, want: "4.2% free, threshold 10%",
		},
		{
			name: "a placeholder used twice", lang: "de", key: "alert.temp.detail",
			params: map[string]any{"value": 101.0, "limit": 95.0, "unit": "°C"}, want: "101 °C, Schwelle 95 °C",
		},
		{
			name: "app name", lang: "ja", key: "alert.app_cpu.title",
			params: map[string]any{"app": "yes"}, want: "yesがCPUを大量に使用しています",
		},
		{name: "unknown language falls back to English", lang: "tlh", key: "quit", want: "Quit"},
		{name: "unknown key comes back as it is", lang: "en", key: "no.such.key", want: "no.such.key"},
		{name: "a plural is not a string", lang: "en", key: "n.app", params: map[string]any{"n": 2}, want: "n.app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Translate(tt.lang, tt.key, tt.params))
		})
	}
}

func TestAlertTexts(t *testing.T) {
	t.Parallel()

	long := "Security alert: your Mac is locked, call +1-555-0100 now or lose your files \u202e"
	tests := []struct {
		name      string
		alert     alerts.Alert
		wantTitle string
		wantBody  string
	}{
		{
			name:      "numbers stay as they are",
			alert:     alerts.Alert{Kind: "disk", Params: map[string]any{"value": 4.2, "limit": 10}},
			wantTitle: Translate("en", "alert.disk.title", nil), wantBody: "4.2% free, threshold 10%",
		},
		{
			name:      "a name the process chose is cleaned and cut to one line",
			alert:     alerts.Alert{Kind: "app_cpu", Params: map[string]any{"app": long, "value": 250, "minutes": 2}},
			wantTitle: "Security alert: your Mac is locked, call +1-555-0100 now or  is using a lot of CPU",
			wantBody:  "250% of a core for over 2 min",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			params := maps.Clone(tt.alert.Params)

			title, body := AlertTexts("en", tt.alert)

			assert.Equal(t, tt.wantTitle, title)
			assert.Equal(t, tt.wantBody, body)
			assert.Equal(t, params, tt.alert.Params, "the alert keeps its own params: the state shows them too")
		})
	}
}

// The strings Go renders itself must exist as plain strings: Translate resolves no plural forms.
func TestTranslate_GoKeys(t *testing.T) {
	t.Parallel()

	keys := []string{
		"menu.open", "open_window", "menu.export", "menu.settings", "set.quit", "act.ejected",
		"act.cleaned", "err.no_trash", "err.stale", "err.automation", "act.signal", "upd.none", "upd.latest", "upd.newer", "err.update",
		"tab.storage", "alertd.gone", "err.busy", "err.timeout", "act.no_answer", "info.gone",
	}
	kinds := []string{
		"cpu", "memory", "disk", "temp", "app_cpu", "app_memory", "thermal",
		"battery_low", "bt_battery", "memory_used", "swap", "app_rule",
	}
	for _, kind := range kinds {
		keys = append(keys, "alert."+kind+".title", "alert."+kind+".detail")
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			assert.NotEqual(t, key, Translate("en", key, nil))
		})
	}
}

// Every process the Apps tab promises to explain has its sentence, in English at least;
// TestReadDictionary then holds the other languages to the same keys.
func TestProcessKey(t *testing.T) {
	t.Parallel()

	described := []string{
		"kernel_task", "WindowServer", "launchd", "mds", "mds_stores", "mdworker", "Spotlight", "coreaudiod", "bluetoothd", "cloudd",
		"bird", "nsurlsessiond", "trustd", "syspolicyd", "XProtect", "backupd", "photoanalysisd", "mediaanalysisd", "softwareupdated",
		"loginwindow", "Dock", "Finder", "SystemUIServer", "ControlCenter", "NotificationCenter", "powerd", "thermalmonitord", "configd",
		"mDNSResponder", "airportd", "WiFiAgent", "logd", "fseventsd", "distnoted", "cfprefsd", "securityd", "tccd", "corespotlightd",
		"suggestd", "sharingd", "rapportd", "universalaccessd", "hidd", "AirPlayXPCHelper", "com.apple.Virtualization.VirtualMachine",
		"VTDecoderXPCService",
	}
	tests := []struct {
		name string
		proc string
		want string
	}{
		{name: "a WebKit process of any role", proc: "com.apple.WebKit.WebContent", want: "proc.com.apple.WebKit"},
		{name: "a Spotlight worker of any kind", proc: "mdworker_shared", want: "proc.mdworker"},
		{name: "the XProtect scanner under its other spelling", proc: "XprotectService", want: "proc.XProtect"},
		{name: "a tool nobody described", proc: "htop"},
		{name: "a key of another family is not a process", proc: "x"},
		{name: "no name"},
	}
	for _, proc := range described {
		tests = append(tests, struct{ name, proc, want string }{name: proc, proc: proc, want: "proc." + proc})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, processKey(tt.proc))
		})
	}
}
