// Package settings holds the user preferences persisted in settings.json.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// LanguageSystem follows the macOS language list instead of naming a language.
const LanguageSystem = "system"

// Languages are the interface languages, one dictionary each in internal/ui/web/i18n.
var Languages = []string{"en", "ru", "es", "de", "fr", "it", "pt-BR", "zh-Hans", "zh-Hant", "ja", "ko", "uk"}

// Themes are the ids the frontend puts in data-skin; internal/ui/web/themes.css has a block for each but the first.
var Themes = []string{"aqua", "graphite", "nord", "catppuccin", "solarized", "gruvbox", "terminal", "paper", "contrast", "vivid"}

// ThemeMode names the themes drawn for one mode only; it mirrors SKIN_MODE in app.js.
var ThemeMode = map[string]string{"terminal": "dark", "paper": "light"}

// Tabs are the tabs of the panel besides Settings, in their default order.
var Tabs = []string{"overview", "processes", "network", "history", "dev", "storage"}

// Tiles are the blocks of the Overview, in their default order.
var Tiles = []string{"cpu", "memory", "network", "disk", "gpu", "battery", "sensors", "apps", "sleep"}

const (
	maxZones    = 8
	maxMuted    = 50
	maxRules    = 20
	maxAppBytes = 200
)

var (
	ErrUnknownKey   = errors.New("unknown settings key")
	ErrInvalidValue = errors.New("invalid settings value")
)

// Settings carries the keys of a {type:"set"} message as JSON names. Launch at login
// is not here: the LaunchAgent file is its only source of truth.
type Settings struct {
	// TempUnit is "C" or "F".
	TempUnit string `json:"temp_unit"`
	// NetUnit is "bytes" or "bits".
	NetUnit string `json:"net_unit"`
	// MenuBar is an ordered, non-empty subset of "cpu", "mem", "net", "temp", "battery", "disk".
	MenuBar []string `json:"menu_bar"`
	Alerts  bool     `json:"alerts"`
	// AlertCPU is percent of all cores, 1–100.
	AlertCPU int `json:"alert_cpu"`
	// AlertTemp is °C, 40–110.
	AlertTemp int `json:"alert_temp"`
	// AlertDiskFree is percent of the disk left free, 1–50.
	AlertDiskFree int `json:"alert_disk_free"`
	// AppsShowSystem lists system processes on the Apps tab.
	AppsShowSystem bool `json:"apps_show_system"`
	// Hotkey registers the global shortcut ⌃⌥P, which no other app then receives.
	Hotkey bool `json:"hotkey"`
	// Language is LanguageSystem or one of Languages.
	Language string `json:"language"`
	// Theme is one of Themes.
	Theme string `json:"theme"`
	// Appearance is "auto" (follow macOS), "light" or "dark".
	Appearance string `json:"appearance"`
	// AlertBattery is percent of charge, 1–50; 0 turns the alert off. So do AlertMemory
	// (percent of memory used, 1–99) and AlertSwap (GB of swap used, 1–64).
	AlertBattery int `json:"alert_battery"`
	AlertMemory  int `json:"alert_memory"`
	AlertSwap    int `json:"alert_swap"`
	// AlertHold is how many seconds a system-wide rule must hold before it alerts, 10–600.
	AlertHold int `json:"alert_hold"`
	// AlertMuted names the apps that never raise a per-app alert.
	AlertMuted []string `json:"alert_muted"`
	AlertRules []Rule   `json:"alert_rules"`
	// MenuBarCompact and MenuBarGraph are opt-in: with both off the status item has no graph and the regular font.
	MenuBarCompact bool `json:"menu_bar_compact"`
	MenuBarGraph   bool `json:"menu_bar_graph"`
	WindowOnTop    bool `json:"window_on_top"`
	// HotkeyQuit registers the global shortcut ⌃⌥K, which asks to quit the heaviest app.
	HotkeyQuit bool `json:"hotkey_quit"`
	// Clock shows the time as a status item of its own; ClockDate and ClockSeconds add to its text.
	Clock        bool `json:"clock"`
	ClockDate    bool `json:"clock_date"`
	ClockSeconds bool `json:"clock_seconds"`
	// ClockHours is "auto" (as macOS is set), "12" or "24".
	ClockHours string `json:"clock_hours"`
	// ClockZones are the IANA names of the world clocks, at most 8.
	ClockZones []string `json:"clock_zones"`
	// MenuBarSeparate gives every MenuBar entry a status item of its own in place of the shared one.
	MenuBarSeparate bool `json:"menu_bar_separate"`
	ShowInDock      bool `json:"show_in_dock"`
	// TabOrder is the tabs shown, in order: a non-empty subset of Tabs. TileOrder is the
	// same for the Overview, a subset of Tiles that may be empty.
	TabOrder  []string `json:"tab_order"`
	TileOrder []string `json:"tile_order"`
}

// Rule is the user's own alert for one app: Metric over Limit for Minutes.
type Rule struct {
	App string `json:"app"`
	// Metric is "cpu" (Limit is percent of one core) or "memory" (Limit is GB).
	Metric  string  `json:"metric"`
	Limit   float64 `json:"limit"`
	Minutes int     `json:"minutes"`
}

func Default() Settings {
	return Settings{
		TempUnit:      "C",
		NetUnit:       "bytes",
		MenuBar:       []string{"cpu", "mem"},
		Alerts:        true,
		AlertCPU:      90,
		AlertTemp:     95,
		AlertDiskFree: 10,
		Language:      LanguageSystem,
		Theme:         "catppuccin",
		Appearance:    "auto",
		AlertBattery:  20,
		AlertHold:     60,
		AlertMuted:    []string{},
		AlertRules:    []Rule{},
		ClockHours:    "auto",
		ClockZones:    []string{},
		TabOrder:      slices.Clone(Tabs),
		TileOrder:     slices.Clone(Tiles),
	}
}

// set is the trust boundary for values typed into the web view and for a hand-edited
// file: the field changes only when value has the right type and is in range.
func (s *Settings) set(key string, value json.RawMessage) error {
	flags := map[string]*bool{
		"alerts": &s.Alerts, "apps_show_system": &s.AppsShowSystem, "hotkey": &s.Hotkey, "hotkey_quit": &s.HotkeyQuit,
		"menu_bar_compact": &s.MenuBarCompact, "menu_bar_graph": &s.MenuBarGraph, "window_on_top": &s.WindowOnTop,
		"clock": &s.Clock, "clock_date": &s.ClockDate, "clock_seconds": &s.ClockSeconds,
		"menu_bar_separate": &s.MenuBarSeparate, "show_in_dock": &s.ShowInDock,
	}
	ranges := map[string]struct {
		field  *int
		lo, hi int
	}{
		"alert_cpu": {&s.AlertCPU, 1, 100}, "alert_temp": {&s.AlertTemp, 40, 110}, "alert_disk_free": {&s.AlertDiskFree, 1, 50},
		"alert_battery": {&s.AlertBattery, 0, 50}, "alert_memory": {&s.AlertMemory, 0, 99}, "alert_swap": {&s.AlertSwap, 0, 64},
		"alert_hold": {&s.AlertHold, 10, 600},
	}
	// An empty status item cannot be clicked and a panel without tabs leads nowhere: either
	// would lock the user out, so those two lists need an item.
	lists := map[string]struct {
		field   *[]string
		allowed []string
		least   int
	}{
		"menu_bar":  {&s.MenuBar, []string{"cpu", "mem", "net", "temp", "battery", "disk"}, 1},
		"tab_order": {&s.TabOrder, Tabs, 1}, "tile_order": {&s.TileOrder, Tiles, 0},
	}
	var err error
	if flag, ok := flags[key]; ok {
		err = setBool(flag, value)
	} else if r, ok := ranges[key]; ok {
		err = setInRange(r.field, value, r.lo, r.hi)
	} else if l, ok := lists[key]; ok {
		err = setSubset(l.field, value, l.allowed, l.least)
	} else {
		switch key {
		case "temp_unit":
			err = setOneOf(&s.TempUnit, value, "C", "F")
		case "net_unit":
			err = setOneOf(&s.NetUnit, value, "bytes", "bits")
		case "clock_hours":
			err = setOneOf(&s.ClockHours, value, "auto", "12", "24")
		case "clock_zones":
			err = setZones(&s.ClockZones, value)
		case "language":
			err = setOneOf(&s.Language, value, append([]string{LanguageSystem}, Languages...)...)
		case "theme":
			err = setOneOf(&s.Theme, value, Themes...)
		case "appearance":
			err = setOneOf(&s.Appearance, value, "auto", "light", "dark")
		case "alert_muted":
			err = setMuted(&s.AlertMuted, value)
		case "alert_rules":
			err = setRules(&s.AlertRules, value)
		default:
			return fmt.Errorf("%w: %q", ErrUnknownKey, key)
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidValue, key, err)
	}
	return nil
}

// setSubset takes an ordered list of at least least items out of allowed, none twice.
func setSubset(field *[]string, value json.RawMessage, allowed []string, least int) error {
	var items []string
	if err := json.Unmarshal(value, &items); err != nil {
		return err
	}
	// nil is JSON null: the state's arrays are never null.
	if items == nil || len(items) < least {
		return fmt.Errorf("need a list of at least %d items", least)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !slices.Contains(allowed, item) || seen[item] {
			return fmt.Errorf("item %q is unknown or repeated", item)
		}
		seen[item] = true
	}
	*field = items
	return nil
}

func setZones(field *[]string, value json.RawMessage) error {
	var zones []string
	if err := json.Unmarshal(value, &zones); err != nil {
		return err
	}
	if zones == nil || len(zones) > maxZones {
		return fmt.Errorf("need a list of at most %d time zones", maxZones)
	}
	seen := map[string]bool{}
	for _, zone := range zones {
		// LoadLocation takes "" and "Local" too, which name no zone a clock could be labelled with,
		// and "Factory", "EST5EDT" or "Europe//London", which the page's Intl refuses to format.
		named := zone == "UTC" || strings.Contains(zone, "/") && !strings.Contains(zone, "//")
		if _, err := time.LoadLocation(zone); err != nil || !named || seen[zone] {
			return fmt.Errorf("time zone %q is unknown or repeated", zone)
		}
		seen[zone] = true
	}
	*field = zones
	return nil
}

func setMuted(field *[]string, value json.RawMessage) error {
	// Not nil: the state's arrays are never null.
	apps := []string{}
	if err := json.Unmarshal(value, &apps); err != nil {
		return err
	}
	if apps == nil || len(apps) > maxMuted {
		return fmt.Errorf("need a list of at most %d apps", maxMuted)
	}
	seen := map[string]bool{}
	for _, app := range apps {
		if app == "" || len(app) > maxAppBytes || seen[app] {
			return fmt.Errorf("app %q is empty, too long or repeated", app)
		}
		seen[app] = true
	}
	*field = apps
	return nil
}

func setRules(field *[]Rule, value json.RawMessage) error {
	rules := []Rule{}
	if err := json.Unmarshal(value, &rules); err != nil {
		return err
	}
	if rules == nil || len(rules) > maxRules {
		return fmt.Errorf("need a list of at most %d rules", maxRules)
	}
	for _, r := range rules {
		if r.App == "" || len(r.App) > maxAppBytes || (r.Metric != "cpu" && r.Metric != "memory") ||
			!(r.Limit > 0) || r.Minutes < 1 || r.Minutes > 60 {
			return fmt.Errorf("rule %+v is out of range", r)
		}
	}
	*field = rules
	return nil
}

func setBool(field *bool, value json.RawMessage) error {
	// json.Unmarshal would take null as "leave it unchanged".
	switch string(bytes.TrimSpace(value)) {
	case "true":
		*field = true
	case "false":
		*field = false
	default:
		return errors.New("not a boolean")
	}
	return nil
}

func setOneOf(field *string, value json.RawMessage, allowed ...string) error {
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	if !slices.Contains(allowed, v) {
		return fmt.Errorf("%q is not one of %q", v, allowed)
	}
	*field = v
	return nil
}

func setInRange(field *int, value json.RawMessage, lo, hi int) error {
	var v int
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	if v < lo || v > hi {
		return fmt.Errorf("%d is outside %d–%d", v, lo, hi)
	}
	*field = v
	return nil
}

// MatchLanguage picks the interface language for the user's preferred languages (BCP 47
// tags, best first): the first one that has a dictionary, else English.
func MatchLanguage(preferred []string) string {
	for _, tag := range preferred {
		base, rest, _ := strings.Cut(tag, "-")
		switch {
		case base == "zh":
			if strings.HasPrefix(rest, "Hant") || slices.Contains([]string{"TW", "HK", "MO"}, rest) {
				return "zh-Hant"
			}
			return "zh-Hans"
		// European Portuguese readers are better served by the Brazilian text than by English.
		case base == "pt":
			return "pt-BR"
		case slices.Contains(Languages, base):
			return base
		}
	}
	return "en"
}
