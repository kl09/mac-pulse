package settings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // one table over every key
func TestSettings_set(t *testing.T) {
	t.Parallel()

	const rule = `{"app":"Xcode","metric":"cpu","limit":150,"minutes":5}`
	tooManyRules := "[" + strings.Repeat(rule+",", maxRules) + rule + "]"

	tests := []struct {
		name    string
		key     string
		value   string
		want    func(*Settings)
		wantErr error
	}{
		{name: "temp unit", key: "temp_unit", value: `"F"`, want: func(s *Settings) { s.TempUnit = "F" }},
		{name: "temp unit outside the enum", key: "temp_unit", value: `"K"`, wantErr: ErrInvalidValue},
		{name: "temp unit of the wrong type", key: "temp_unit", value: `1`, wantErr: ErrInvalidValue},
		{name: "temp unit null", key: "temp_unit", value: `null`, wantErr: ErrInvalidValue},
		{name: "net unit", key: "net_unit", value: `"bits"`, want: func(s *Settings) { s.NetUnit = "bits" }},
		{name: "net unit outside the enum", key: "net_unit", value: `"nibbles"`, wantErr: ErrInvalidValue},
		{
			name: "menu bar keeps the given order", key: "menu_bar", value: `["net","cpu","battery"]`,
			want: func(s *Settings) { s.MenuBar = []string{"net", "cpu", "battery"} },
		},
		{name: "menu bar with an unknown item", key: "menu_bar", value: `["cpu","fans"]`, wantErr: ErrInvalidValue},
		{name: "menu bar with a repeated item", key: "menu_bar", value: `["cpu","cpu"]`, wantErr: ErrInvalidValue},
		{name: "menu bar empty", key: "menu_bar", value: `[]`, wantErr: ErrInvalidValue},
		{name: "menu bar null", key: "menu_bar", value: `null`, wantErr: ErrInvalidValue},
		{name: "menu bar of the wrong type", key: "menu_bar", value: `"cpu"`, wantErr: ErrInvalidValue},
		{name: "alerts off", key: "alerts", value: ` false `, want: func(s *Settings) { s.Alerts = false }},
		{name: "alerts null", key: "alerts", value: `null`, wantErr: ErrInvalidValue},
		{name: "alerts as a string", key: "alerts", value: `"true"`, wantErr: ErrInvalidValue},
		{name: "apps show system", key: "apps_show_system", value: `true`, want: func(s *Settings) { s.AppsShowSystem = true }},
		{name: "apps show system as a number", key: "apps_show_system", value: `1`, wantErr: ErrInvalidValue},
		{name: "hotkey on", key: "hotkey", value: `true`, want: func(s *Settings) { s.Hotkey = true }},
		{name: "hotkey as a string", key: "hotkey", value: `"true"`, wantErr: ErrInvalidValue},
		{name: "language", key: "language", value: `"pt-BR"`, want: func(s *Settings) { s.Language = "pt-BR" }},
		{name: "language back to system", key: "language", value: `"system"`, want: func(s *Settings) {}},
		{name: "language without a dictionary", key: "language", value: `"tlh"`, wantErr: ErrInvalidValue},
		{name: "language of the wrong type", key: "language", value: `1`, wantErr: ErrInvalidValue},
		{name: "alert cpu", key: "alert_cpu", value: `75`, want: func(s *Settings) { s.AlertCPU = 75 }},
		{name: "alert cpu lower bound", key: "alert_cpu", value: `1`, want: func(s *Settings) { s.AlertCPU = 1 }},
		{name: "alert cpu above the range", key: "alert_cpu", value: `101`, wantErr: ErrInvalidValue},
		{name: "alert cpu zero", key: "alert_cpu", value: `0`, wantErr: ErrInvalidValue},
		{name: "alert cpu fractional", key: "alert_cpu", value: `75.5`, wantErr: ErrInvalidValue},
		{name: "alert cpu as a string", key: "alert_cpu", value: `"75"`, wantErr: ErrInvalidValue},
		{name: "alert temp", key: "alert_temp", value: `110`, want: func(s *Settings) { s.AlertTemp = 110 }},
		{name: "alert temp below the range", key: "alert_temp", value: `39`, wantErr: ErrInvalidValue},
		{name: "alert disk free", key: "alert_disk_free", value: `5`, want: func(s *Settings) { s.AlertDiskFree = 5 }},
		{name: "alert disk free above the range", key: "alert_disk_free", value: `51`, wantErr: ErrInvalidValue},
		{name: "alert disk free negative", key: "alert_disk_free", value: `-1`, wantErr: ErrInvalidValue},
		{name: "value is not JSON", key: "alert_cpu", value: `{`, wantErr: ErrInvalidValue},
		{name: "launch at login is not settable", key: "launch_at_login", value: `true`, wantErr: ErrUnknownKey},
		{name: "unknown key", key: "accent", value: `"blue"`, wantErr: ErrUnknownKey},
		{name: "theme", key: "theme", value: `"terminal"`, want: func(s *Settings) { s.Theme = "terminal" }},
		{name: "appearance", key: "appearance", value: `"dark"`, want: func(s *Settings) { s.Appearance = "dark" }},
		{name: "appearance outside the three modes", key: "appearance", value: `"sepia"`, wantErr: ErrInvalidValue},
		{name: "theme that does not exist", key: "theme", value: `"dark"`, wantErr: ErrInvalidValue},
		{name: "menu bar with the disk item", key: "menu_bar", value: `["disk"]`, want: func(s *Settings) { s.MenuBar = []string{"disk"} }},
		{name: "alert battery off", key: "alert_battery", value: `0`, want: func(s *Settings) { s.AlertBattery = 0 }},
		{name: "alert battery above the range", key: "alert_battery", value: `51`, wantErr: ErrInvalidValue},
		{name: "alert memory", key: "alert_memory", value: `99`, want: func(s *Settings) { s.AlertMemory = 99 }},
		{name: "alert swap", key: "alert_swap", value: `64`, want: func(s *Settings) { s.AlertSwap = 64 }},
		{name: "alert hold", key: "alert_hold", value: `10`, want: func(s *Settings) { s.AlertHold = 10 }},
		{name: "alert hold below the range", key: "alert_hold", value: `9`, wantErr: ErrInvalidValue},
		{name: "muted apps", key: "alert_muted", value: `["Xcode"]`, want: func(s *Settings) { s.AlertMuted = []string{"Xcode"} }},
		{name: "muted app repeated", key: "alert_muted", value: `["Xcode","Xcode"]`, wantErr: ErrInvalidValue},
		{name: "muted app without a name", key: "alert_muted", value: `[""]`, wantErr: ErrInvalidValue},
		{name: "muted apps as null", key: "alert_muted", value: `null`, wantErr: ErrInvalidValue},
		{
			name: "alert rule", key: "alert_rules", value: `[{"app":"Xcode","metric":"memory","limit":8.5,"minutes":60}]`,
			want: func(s *Settings) { s.AlertRules = []Rule{{App: "Xcode", Metric: "memory", Limit: 8.5, Minutes: 60}} },
		},
		{name: "unknown rule metric", key: "alert_rules", value: `[{"app":"X","metric":"gpu","limit":1,"minutes":1}]`, wantErr: ErrInvalidValue},
		{name: "alert rule without a limit", key: "alert_rules", value: `[{"app":"Xcode","metric":"cpu","minutes":1}]`, wantErr: ErrInvalidValue},
		{name: "zero rule minutes", key: "alert_rules", value: `[{"app":"X","metric":"cpu","limit":1,"minutes":0}]`, wantErr: ErrInvalidValue},
		{name: "alert rules beyond the cap", key: "alert_rules", value: tooManyRules, wantErr: ErrInvalidValue},
		{name: "compact menu bar", key: "menu_bar_compact", value: `true`, want: func(s *Settings) { s.MenuBarCompact = true }},
		{name: "menu bar graphs", key: "menu_bar_graph", value: `true`, want: func(s *Settings) { s.MenuBarGraph = true }},
		{name: "window on top", key: "window_on_top", value: `true`, want: func(s *Settings) { s.WindowOnTop = true }},
		{name: "quit-heaviest shortcut", key: "hotkey_quit", value: `true`, want: func(s *Settings) { s.HotkeyQuit = true }},
		{name: "go field name is not a key", key: "TempUnit", value: `"F"`, wantErr: ErrUnknownKey},
		{name: "clock", key: "clock", value: `true`, want: func(s *Settings) { s.Clock = true }},
		{name: "clock date", key: "clock_date", value: `true`, want: func(s *Settings) { s.ClockDate = true }},
		{name: "clock seconds", key: "clock_seconds", value: `true`, want: func(s *Settings) { s.ClockSeconds = true }},
		{name: "clock as a number", key: "clock", value: `1`, wantErr: ErrInvalidValue},
		{name: "clock hours", key: "clock_hours", value: `"24"`, want: func(s *Settings) { s.ClockHours = "24" }},
		{name: "clock hours as a number", key: "clock_hours", value: `24`, wantErr: ErrInvalidValue},
		{name: "clock hours outside the enum", key: "clock_hours", value: `"13"`, wantErr: ErrInvalidValue},
		{
			name: "world clocks", key: "clock_zones", value: `["Asia/Tokyo","UTC"]`,
			want: func(s *Settings) { s.ClockZones = []string{"Asia/Tokyo", "UTC"} },
		},
		{name: "world clock in an unknown zone", key: "clock_zones", value: `["Mars/Olympus"]`, wantErr: ErrInvalidValue},
		{name: "world clock that climbs out of the zone database", key: "clock_zones", value: `["../../etc/passwd"]`, wantErr: ErrInvalidValue},
		{name: "world clock without a name", key: "clock_zones", value: `[""]`, wantErr: ErrInvalidValue},
		{name: "world clock named Local", key: "clock_zones", value: `["Local"]`, wantErr: ErrInvalidValue},
		{name: "world clock named by a zone file that is no zone", key: "clock_zones", value: `["Factory"]`, wantErr: ErrInvalidValue},
		{name: "world clock in a zone without a region", key: "clock_zones", value: `["EST5EDT"]`, wantErr: ErrInvalidValue},
		{name: "world clock with a doubled slash", key: "clock_zones", value: `["Europe//London"]`, wantErr: ErrInvalidValue},
		{name: "world clock repeated", key: "clock_zones", value: `["UTC","UTC"]`, wantErr: ErrInvalidValue},
		{
			name: "a ninth world clock", key: "clock_zones", wantErr: ErrInvalidValue,
			value: `["UTC","Asia/Tokyo","Europe/Berlin","Europe/Paris","Europe/Kyiv","America/New_York","Asia/Seoul","Asia/Dubai","Europe/Rome"]`,
		},
		{name: "world clocks as null", key: "clock_zones", value: `null`, wantErr: ErrInvalidValue},
		{name: "separate menu bar items", key: "menu_bar_separate", value: `true`, want: func(s *Settings) { s.MenuBarSeparate = true }},
		{name: "show in Dock", key: "show_in_dock", value: `true`, want: func(s *Settings) { s.ShowInDock = true }},
		{
			name: "tab order keeps the order and hides the rest", key: "tab_order", value: `["storage","overview"]`,
			want: func(s *Settings) { s.TabOrder = []string{"storage", "overview"} },
		},
		{name: "tab order empty", key: "tab_order", value: `[]`, wantErr: ErrInvalidValue},
		{name: "tab order with settings, which is always shown", key: "tab_order", value: `["settings"]`, wantErr: ErrInvalidValue},
		{name: "tab order with a repeated tab", key: "tab_order", value: `["dev","dev"]`, wantErr: ErrInvalidValue},
		{
			name: "tile order", key: "tile_order", value: `["battery","cpu"]`,
			want: func(s *Settings) { s.TileOrder = []string{"battery", "cpu"} },
		},
		{name: "tile order may hide every tile", key: "tile_order", value: `[]`, want: func(s *Settings) { s.TileOrder = []string{} }},
		{name: "tile order with a repeated tile", key: "tile_order", value: `["cpu","cpu"]`, wantErr: ErrInvalidValue},
		{name: "tile order with a tab name", key: "tile_order", value: `["storage"]`, wantErr: ErrInvalidValue},
		{name: "tile order as null", key: "tile_order", value: `null`, wantErr: ErrInvalidValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, want := Default(), Default()

			err := got.set(tt.key, json.RawMessage(tt.value))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, want, got)
				return
			}
			require.NoError(t, err)
			tt.want(&want)
			assert.Equal(t, want, got)
		})
	}
}

func TestMatchLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		preferred []string
		want      string
	}{
		{name: "no list", want: "en"},
		{name: "region is dropped", preferred: []string{"ru-RU", "en-US"}, want: "ru"},
		{name: "first supported wins", preferred: []string{"be-BY", "uk-UA", "ru-RU"}, want: "uk"},
		{name: "nothing supported", preferred: []string{"be-BY", "tlh"}, want: "en"},
		{name: "simplified chinese by script", preferred: []string{"zh-Hans-CN"}, want: "zh-Hans"},
		{name: "traditional chinese by script", preferred: []string{"zh-Hant-HK"}, want: "zh-Hant"},
		{name: "traditional chinese by region", preferred: []string{"zh-TW"}, want: "zh-Hant"},
		{name: "bare chinese", preferred: []string{"zh"}, want: "zh-Hans"},
		{name: "brazilian portuguese", preferred: []string{"pt-BR"}, want: "pt-BR"},
		{name: "european portuguese", preferred: []string{"pt-PT"}, want: "pt-BR"},
		{name: "english variant", preferred: []string{"en-AR", "es-AR"}, want: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, MatchLanguage(tt.preferred))
		})
	}
}
