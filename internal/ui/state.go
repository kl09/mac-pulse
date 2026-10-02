// Package ui is the JSON contract between the Go core and the web frontend.
package ui

import "github.com/kl09/mac-pulse/internal/settings"

// Units everywhere: bytes, bytes per second (*_rate), percent 0–100, °C, watts,
// seconds (*_s), unix milliseconds (time, since, until, start). Slices are never null.

// IconSelf is the icon key of mac-pulse's own rows: the shell serves the embedded PNG for it.
const IconSelf = "self"

// State is pushed to window.mp.onState on every sampler tick.
type State struct {
	Time int64 `json:"time"`
	// HasRates is false on the first tick: every *_rate is 0 and renders as "—".
	HasRates      bool           `json:"has_rates"`
	CPU           CPU            `json:"cpu"`
	Memory        Memory         `json:"memory"`
	Disk          Disk           `json:"disk"`
	Network       Network        `json:"network"`
	GPU           *GPU           `json:"gpu"`
	Battery       *Battery       `json:"battery"`
	SleepBlockers []SleepBlocker `json:"sleep_blockers"`
	Spark         Spark          `json:"spark"`
	Apps          Apps           `json:"apps"`
	Power         Power          `json:"power"`
	Sensors       Sensors        `json:"sensors"`
	// NetInfo is null unless a visible view is on the network detail screen.
	NetInfo *NetInfo `json:"net_info"`
	// Net is null unless a visible view is on the network tab or the network detail screen.
	Net      *NetReport `json:"net"`
	Alerts   Alerts     `json:"alerts"`
	Settings Settings   `json:"settings"`
	// SystemLang is the language "system" stands for: the first macOS preferred language with a dictionary.
	SystemLang string `json:"system_lang"`
	// Pinned is true while the panel stays open when the user clicks elsewhere.
	Pinned bool `json:"pinned"`
	// Dev is null unless a visible view is on the dev tab, and until docker has first answered.
	Dev   *Dev  `json:"dev"`
	Media Media `json:"media"`
	// Storage is null unless a visible view is on the storage tab.
	Storage *Storage `json:"storage"`
}

// Media says whether the microphone and the camera are in use right now.
type Media struct {
	Mic    MediaMic    `json:"mic"`
	Camera MediaCamera `json:"camera"`
}

type MediaMic struct {
	Active bool `json:"active"`
	// Apps names who records: the Apps tab's name of the process, else its bundle id. Empty
	// on macOS 13, which only tells that the microphone is in use.
	Apps []string `json:"apps"`
}

// MediaCamera has no apps: macOS does not tell who uses the camera.
type MediaCamera struct {
	Active bool `json:"active"`
}

// Storage is the state of the two things the Storage tab does on request.
type Storage struct {
	Scan    StorageScan `json:"scan"`
	Cleanup Cleanup     `json:"cleanup"`
}

type StorageScan struct {
	// State is "idle" (nothing scanned), "running", "done", "cancelled" or "failed".
	State string `json:"state"`
	// Root is the folder scanned, with the home directory as "~"; "" while idle.
	Root string `json:"root"`
	// Files, Bytes and Denied count up while the scan runs and are its totals once done.
	Files  int   `json:"files"`
	Bytes  int64 `json:"bytes"`
	Denied int   `json:"denied"`
}

type Cleanup struct {
	// State is "idle" (never measured), "running" or "done".
	State string `json:"state"`
	// Categories is always the full list in display order; sizes are 0 until measured.
	Categories []CleanupCategory `json:"categories"`
}

type CleanupCategory struct {
	ID string `json:"id"`
	// Denied is true when macOS did not let the folder be read.
	Denied bool  `json:"denied"`
	Bytes  int64 `json:"bytes"`
	Items  int   `json:"items"`
	// Permanent is true for a category that cannot go to the Trash: its removal is final.
	Permanent bool `json:"permanent"`
}

// CleanupReview answers a cleanup_list: what a cleanup of the category would remove, for
// the user to pick from.
type CleanupReview struct {
	// ID names the listing this review shows. The cleanup message sends it back, and Go removes
	// nothing once the category has been listed or measured again.
	ID string `json:"id"`
	// Items are the largest entries of the category folder, largest first, at most 50.
	Items []CleanupItem `json:"items"`
	// Rest counts the smaller entries, which the page offers as one row, RestBytes their size.
	Rest      int   `json:"rest"`
	RestBytes int64 `json:"rest_bytes"`
}

type CleanupItem struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// StorageLevel answers a storage_open message through window.mp.onStorage: one folder of
// the scanned tree.
type StorageLevel struct {
	// Path is relative to the scan root, "" for the root itself.
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
	// Children are sorted by size, largest first, at most 200; the rest is summed in a last
	// entry with an empty Name.
	Children []StorageEntry `json:"children"`
}

type StorageEntry struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Files  int    `json:"files"`
	Dir    bool   `json:"dir"`
	Denied bool   `json:"denied"`
	// Category is the id of the cleanup category this folder is, "" for any other.
	Category string `json:"category"`
}

type Dev struct {
	// Docker is "ok", "missing" (no docker CLI), "stopped" (the daemon does not answer) or
	// "remote" (the CLI's context is not a local socket, so it is not polled).
	Docker     string         `json:"docker"`
	Containers []DevContainer `json:"containers"`
	// Projects and Agents are sorted by CPU, highest first.
	Projects []DevProject `json:"projects"`
	Agents   []DevAgent   `json:"agents"`
}

type DevContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
	// Project and Dir are the compose labels, "" outside compose.
	Project string `json:"project"`
	Dir     string `json:"dir"`
	// CPU, Memory and PIDs are null until the first `docker stats` answer.
	CPU    *float64 `json:"cpu"`
	Memory *uint64  `json:"memory"`
	PIDs   *int     `json:"pids"`
}

// DevProject is the user's listening processes that share a working directory, with the
// compose containers of that directory.
type DevProject struct {
	Name string `json:"name"`
	// Dir has the home directory as "~".
	Dir        string   `json:"dir"`
	Ports      []int    `json:"ports"`
	Apps       []string `json:"apps"`
	PIDs       []int32  `json:"pids"`
	CPU        float64  `json:"cpu"`
	Memory     uint64   `json:"memory"`
	Containers int      `json:"containers"`
}

type DevAgent struct {
	Name   string  `json:"name"`
	PID    int32   `json:"pid"`
	Dir    string  `json:"dir"`
	Since  int64   `json:"since"`
	Procs  int     `json:"procs"`
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	// EnergyMW is null when the session's counters need root.
	EnergyMW *float64 `json:"energy_mw"`
}

type CPU struct {
	Total  float64    `json:"total"`
	User   float64    `json:"user"`
	System float64    `json:"system"`
	Cores  []float64  `json:"cores"`
	Load   [3]float64 `json:"load"`
	Uptime int64      `json:"uptime_s"`
	Temp   *float64   `json:"temp_c"`
	Model  string     `json:"model"`
	// Clusters follow the order of Cores: the first Clusters[0].Cores entries belong to Clusters[0].
	Clusters []Cluster `json:"clusters"`
}

type Cluster struct {
	Name  string `json:"name"`
	Cores int    `json:"cores"`
	// FreqMHz is null while the cluster idles or the source is closed.
	FreqMHz *float64 `json:"freq_mhz"`
	// Temp is null unless a screen that lists the sensors is open.
	Temp *float64 `json:"temp_c"`
}

type Memory struct {
	Total uint64 `json:"total"`
	// Used is App + Wired + Compressed, the way Activity Monitor counts it.
	Used       uint64 `json:"used"`
	App        uint64 `json:"app"`
	Wired      uint64 `json:"wired"`
	Compressed uint64 `json:"compressed"`
	Cached     uint64 `json:"cached"`
	Free       uint64 `json:"free"`
	// Pressure is "normal", "warning" or "critical".
	Pressure  string `json:"pressure"`
	SwapUsed  uint64 `json:"swap_used"`
	SwapTotal uint64 `json:"swap_total"`
	// The six paging rates are bytes per second: pages times the page size.
	PageInRate     float64 `json:"page_in_rate"`
	PageOutRate    float64 `json:"page_out_rate"`
	SwapInRate     float64 `json:"swap_in_rate"`
	SwapOutRate    float64 `json:"swap_out_rate"`
	CompressRate   float64 `json:"compress_rate"`
	DecompressRate float64 `json:"decompress_rate"`
}

type Disk struct {
	Total     uint64  `json:"total"`
	Free      uint64  `json:"free"`
	ReadRate  float64 `json:"read_rate"`
	WriteRate float64 `json:"write_rate"`
	// WrittenToday counts from local midnight and survives restarts.
	WrittenToday uint64 `json:"written_today"`
	Model        string `json:"model"`
	// SMART is "verified", "failing" or "" while unread; it is read when the disk detail screen opens.
	SMART string `json:"smart"`
	// ReadTotal and WriteTotal count from boot.
	ReadTotal  uint64   `json:"read_total"`
	WriteTotal uint64   `json:"write_total"`
	Volumes    []Volume `json:"volumes"`
}

type Volume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
	FS    string `json:"fs"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
	// Ejectable is true for a mount under /Volumes/: the only ones an eject message may name.
	Ejectable bool `json:"ejectable"`
}

type Network struct {
	DownRate  float64 `json:"down_rate"`
	UpRate    float64 `json:"up_rate"`
	DownToday uint64  `json:"down_today"`
	UpToday   uint64  `json:"up_today"`
	// The 7 and 30 day totals include today; null outside detail:network.
	Down7d  *uint64 `json:"down_7d"`
	Up7d    *uint64 `json:"up_7d"`
	Down30d *uint64 `json:"down_30d"`
	Up30d   *uint64 `json:"up_30d"`
}

// GPU.Util is the Device utilization; Memory is in use, MemoryAlloc allocated.
type GPU struct {
	Util        float64  `json:"util"`
	Memory      uint64   `json:"memory"`
	Model       string   `json:"model"`
	Renderer    float64  `json:"renderer"`
	Tiler       float64  `json:"tiler"`
	MemoryAlloc uint64   `json:"memory_alloc"`
	FreqMHz     *float64 `json:"freq_mhz"`
	Temp        *float64 `json:"temp_c"`
}

type Battery struct {
	Percent int `json:"percent"`
	// State is "charging", "plugged" (on AC, not charging) or "battery".
	State string `json:"state"`
	// TimeRemaining is to full while charging and to empty on battery; null while macOS has no estimate.
	TimeRemaining *int64 `json:"time_remaining_s"`
	// Power is never negative; State gives the direction.
	Power     float64 `json:"power_w"`
	Health    int     `json:"health"`
	Cycles    int     `json:"cycles"`
	Temp      float64 `json:"temp_c"`
	DesignMAh int     `json:"design_mah"`
	MaxMAh    int     `json:"max_mah"`
	Voltage   float64 `json:"voltage_v"`
	// Amperage is never negative; State gives the direction.
	Amperage float64 `json:"amperage_a"`
	// Adapter is null on battery.
	Adapter *Adapter `json:"adapter"`
	// UnpluggedAt is null on AC and when mac-pulse started on battery.
	UnpluggedAt *int64 `json:"unplugged_at"`
	// LowPower is true while Low Power Mode is on.
	LowPower bool `json:"low_power"`
}

type Adapter struct {
	Name  string `json:"name"`
	Watts int    `json:"watts"`
}

// Power is in watts; a null field is a source that could not be read, never 0 W.
type Power struct {
	System  *float64 `json:"system_w"`
	Adapter *float64 `json:"adapter_w"`
	CPU     *float64 `json:"cpu_w"`
	GPU     *float64 `json:"gpu_w"`
}

type Sensors struct {
	// Thermal is "nominal", "fair", "serious" or "critical".
	Thermal string `json:"thermal"`
	// Fans is empty on a fanless Mac; RPM 0 is a stopped fan.
	Fans []Fan `json:"fans"`
	// Temps is filled only on the cpu, gpu and sensors detail screens.
	Temps []Sensor `json:"temps"`
	// Bluetooth is read every minute on the sensors detail screen and every five elsewhere.
	Bluetooth []BluetoothDevice `json:"bluetooth"`
}

type Fan struct {
	RPM float64 `json:"rpm"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type Sensor struct {
	Group string `json:"group"`
	// Key is the four-character SMC key.
	Key  string  `json:"key"`
	Temp float64 `json:"temp_c"`
}

type BluetoothDevice struct {
	Name   string         `json:"name"`
	Kind   string         `json:"kind"`
	Levels []BatteryLevel `json:"levels"`
}

type BatteryLevel struct {
	// Part is "left", "right", "case" or "main".
	Part    string `json:"part"`
	Percent int    `json:"percent"`
}

type NetInfo struct {
	Interfaces []Interface `json:"interfaces"`
	// Router is "" without a default route.
	Router string   `json:"router"`
	DNS    []string `json:"dns"`
	// WiFi is null when Wi-Fi is off or not associated; it never carries the network name.
	WiFi *WiFi `json:"wifi"`
}

type Interface struct {
	Name    string   `json:"name"`
	MAC     string   `json:"mac"`
	IPv4    []string `json:"ipv4"`
	IPv6    []string `json:"ipv6"`
	Primary bool     `json:"primary"`
}

type WiFi struct {
	Interface string  `json:"interface"`
	RSSI      int     `json:"rssi"`
	Noise     int     `json:"noise"`
	Channel   int     `json:"channel"`
	BandGHz   float64 `json:"band_ghz"`
	WidthMHz  int     `json:"width_mhz"`
	TxRate    float64 `json:"tx_rate_mbps"`
	PHY       string  `json:"phy"`
	Security  string  `json:"security"`
}

type SleepBlocker struct {
	PID  int32  `json:"pid"`
	App  string `json:"app"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Spark holds up to 60 points per series, oldest first, one per sampler tick: 2 s apart
// while a view is visible, 5 s apart while none is.
type Spark struct {
	CPU       []float64 `json:"cpu"`
	Memory    []float64 `json:"memory"`
	GPU       []float64 `json:"gpu"`
	Temp      []float64 `json:"temp"`
	Power     []float64 `json:"power"`
	DiskRead  []float64 `json:"disk_read"`
	DiskWrite []float64 `json:"disk_write"`
	NetDown   []float64 `json:"net_down"`
	NetUp     []float64 `json:"net_up"`
}

type Apps struct {
	// HasRates is false until the second process scan: every cpu renders as "—".
	HasRates bool  `json:"has_rates"`
	Items    []App `json:"items"`
}

type App struct {
	Name string `json:"name"`
	// Icon is the bundle path, loaded as icon?path=<Icon>; "" for an app without a bundle,
	// IconSelf for mac-pulse itself.
	Icon     string `json:"icon"`
	PIDCount int    `json:"pid_count"`
	// CPU is percent of one core and exceeds 100 on a multi-threaded load.
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	// System is true when every process is a system one: the Apps tab hides these by default.
	System bool `json:"system"`
	// Killable is true when at least one process is.
	Killable bool `json:"killable"`
	// The disk rates and EnergyMW are null when every process belongs to another user:
	// their counters need root.
	DiskReadRate  *float64 `json:"disk_read_rate"`
	DiskWriteRate *float64 `json:"disk_write_rate"`
	EnergyMW      *float64 `json:"energy_mw"`
	Procs         []Proc   `json:"procs"`
}

type Proc struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	// System is true for another user's process and for a binary outside any bundle
	// under /System, /usr (except /usr/local), /sbin, /bin or /Library/Apple.
	System bool `json:"system"`
	// Killable is true for the user's own process other than launchd and mac-pulse itself.
	Killable bool `json:"killable"`
	// Kind is "browser", "tab", "extension", "gpu" or "utility" for a process of a
	// Chromium-family browser, Safari or Firefox, "" for any other.
	Kind string `json:"kind"`
}

type NetReport struct {
	// HasRates is false on the first poll after the tab opens.
	HasRates    bool          `json:"has_rates"`
	Apps        []NetApp      `json:"apps"`
	Connections []NetConn     `json:"connections"`
	Listening   []NetListener `json:"listening"`
	Talkers     []NetTalker   `json:"talkers"`
	Today       []NetAppTotal `json:"today"`
}

type NetApp struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	// System is the Apps tab's flag for the app of the same name.
	System   bool    `json:"system"`
	PIDCount int     `json:"pid_count"`
	DownRate float64 `json:"down_rate"`
	UpRate   float64 `json:"up_rate"`
	// DownTotal and UpTotal count from the first poll that saw the app.
	DownTotal   uint64 `json:"down_total"`
	UpTotal     uint64 `json:"up_total"`
	Connections int    `json:"connections"`
}

type NetConn struct {
	App   string `json:"app"`
	PID   int32  `json:"pid"`
	Proto string `json:"proto"`
	Local string `json:"local"`
	// RemoteIP and RemotePort are "*" for an unconnected socket.
	RemoteIP   string `json:"remote_ip"`
	RemotePort string `json:"remote_port"`
	// Host is the reverse DNS name, "" when there is none.
	Host string `json:"host"`
	// State is the TCP state, "" for UDP.
	State    string  `json:"state"`
	BytesIn  uint64  `json:"bytes_in"`
	BytesOut uint64  `json:"bytes_out"`
	DownRate float64 `json:"down_rate"`
	UpRate   float64 `json:"up_rate"`
}

type NetListener struct {
	App   string `json:"app"`
	PID   int32  `json:"pid"`
	Proto string `json:"proto"`
	Addr  string `json:"addr"`
	Port  string `json:"port"`
	// Dir is the working directory with the home as "~"; "" for another user's process and for "/".
	Dir string `json:"dir"`
}

type NetTalker struct {
	IP          string   `json:"ip"`
	Host        string   `json:"host"`
	Apps        []string `json:"apps"`
	DownRate    float64  `json:"down_rate"`
	UpRate      float64  `json:"up_rate"`
	Connections int      `json:"connections"`
}

// NetAppTotal is one app's traffic since local midnight; it outlives the app's processes.
type NetAppTotal struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Down uint64 `json:"down"`
	Up   uint64 `json:"up"`
}

type Alerts struct {
	Active []Alert `json:"active"`
	// Recent holds resolved alerts, newest first, at most 20.
	Recent []Alert `json:"recent"`
}

type Alert struct {
	// ID is stable for as long as the condition lasts.
	ID string `json:"id"`
	// Kind is "cpu", "memory", "disk", "temp", "thermal", "app_cpu", "app_memory",
	// "battery_low", "bt_battery", "memory_used", "swap" or "app_rule".
	Kind string `json:"kind"`
	// Params fill the dictionary strings alert.<kind>.title and alert.<kind>.detail.
	Params map[string]any `json:"params"`
	// App is "" for a system-wide alert; for "bt_battery" it is the device name.
	App   string `json:"app"`
	Since int64  `json:"since"`
	// Until is 0 while the alert is active.
	Until int64 `json:"until"`
}

// AlertDetail answers an alert_detail message: the alert as the lists carry it, why it fired
// and what was measured while it lasted. The lists stay light; this is fetched for one alert.
type AlertDetail struct {
	Alert
	// Recorded is false for an alert stored by a build that recorded no details: every field
	// below is then empty.
	Recorded bool `json:"recorded"`
	// Icon is the .app of the alert's app, "" for a system alert and for a process outside a bundle.
	Icon string `json:"icon"`
	// Unit is "percent", "bytes" or "celsius": the unit of Limit, the four values and Points.
	Unit string `json:"unit"`
	// Limit is 0 for a rule without a number of its own; Below says the rule holds under it.
	Limit   float64 `json:"limit"`
	Below   bool    `json:"below"`
	HoldS   int64   `json:"hold_s"`
	FiredAt int64   `json:"fired_at"`
	// Fired, Peak (the lowest value when Below) and Avg are null when the metric had no
	// reading; Current is null once the alert has ended.
	Fired   *float64 `json:"fired"`
	Peak    *float64 `json:"peak"`
	Avg     *float64 `json:"avg"`
	Current *float64 `json:"current"`
	// Points are StepS apart from Start, at most 240, null where nothing was sampled; for a
	// system alert they begin up to ten minutes before Since.
	Start  int64      `json:"start"`
	StepS  int64      `json:"step_s"`
	Points []*float64 `json:"points"`
	// Top is the five heaviest apps when the alert fired, in TopUnit: "percent", "bytes",
	// "bytes_per_s" (disk writes) or "milliwatts".
	Top     []AlertRow `json:"top"`
	TopUnit string     `json:"top_unit"`
	// PIDCount and Procs belong to an alert about an app: its processes when the alert fired
	// and the five heaviest of them, in Unit.
	PIDCount int         `json:"pid_count"`
	Procs    []AlertRow  `json:"procs"`
	Facts    []AlertFact `json:"facts"`
}

type AlertRow struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	// PID is 0 for an app.
	PID   int32   `json:"pid"`
	Value float64 `json:"value"`
}

// AlertFact is one reading captured when the alert fired. Unit is one of AlertDetail's, or
// "bytes_per_s", "watts", "seconds" or "state" (the thermal state, 0 nominal to 3 critical).
type AlertFact struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Settings keys are the values of the "key" field in a {type:"set"} message.
type Settings struct {
	// TempUnit is "C" or "F"; the state always carries °C.
	TempUnit string `json:"temp_unit"`
	// NetUnit is "bytes" or "bits"; the state always carries bytes.
	NetUnit string `json:"net_unit"`
	// MenuBar is an ordered subset of "cpu", "mem", "net", "temp", "battery", "disk".
	MenuBar       []string `json:"menu_bar"`
	Alerts        bool     `json:"alerts"`
	AlertCPU      int      `json:"alert_cpu"`
	AlertTemp     int      `json:"alert_temp"`
	AlertDiskFree int      `json:"alert_disk_free"`
	// AppsShowSystem lists system apps on the Apps tab.
	AppsShowSystem bool `json:"apps_show_system"`
	// Hotkey is the global shortcut ⌃⌥P that toggles the panel.
	Hotkey bool `json:"hotkey"`
	// Language is "system" or a language code of web/i18n.
	Language string `json:"language"`
	// Theme is one of settings.Themes; the frontend puts it in data-skin.
	Theme string `json:"theme"`
	// Appearance is "auto", "light" or "dark": the mode of a theme that has both.
	Appearance string `json:"appearance"`
	// A 0 in AlertBattery (percent), AlertMemory (percent) or AlertSwap (GB) turns that alert off.
	AlertBattery int `json:"alert_battery"`
	AlertMemory  int `json:"alert_memory"`
	AlertSwap    int `json:"alert_swap"`
	// AlertHold is seconds.
	AlertHold      int             `json:"alert_hold"`
	AlertMuted     []string        `json:"alert_muted"`
	AlertRules     []settings.Rule `json:"alert_rules"`
	MenuBarCompact bool            `json:"menu_bar_compact"`
	MenuBarGraph   bool            `json:"menu_bar_graph"`
	WindowOnTop    bool            `json:"window_on_top"`
	// HotkeyQuit is the global shortcut ⌃⌥K that asks to quit the heaviest app.
	HotkeyQuit bool `json:"hotkey_quit"`
	// Clock shows the time as a status item of its own; ClockHours is "auto", "12" or "24"
	// and ClockZones the IANA names of the world clocks.
	Clock        bool     `json:"clock"`
	ClockDate    bool     `json:"clock_date"`
	ClockSeconds bool     `json:"clock_seconds"`
	ClockHours   string   `json:"clock_hours"`
	ClockZones   []string `json:"clock_zones"`
	// MenuBarSeparate gives every MenuBar entry a status item of its own.
	MenuBarSeparate bool `json:"menu_bar_separate"`
	ShowInDock      bool `json:"show_in_dock"`
	// TabOrder and TileOrder are the tabs and the Overview blocks shown, in order; one left out is hidden.
	TabOrder  []string `json:"tab_order"`
	TileOrder []string `json:"tile_order"`
	// LaunchAtLogin changes through a {type:"login_item"} message, not {type:"set"}.
	LaunchAtLogin bool `json:"launch_at_login"`
}

// History answers a {type:"history"} message through window.mp.onHistory.
type History struct {
	Metric string `json:"metric"`
	Range  string `json:"range"`
	Start  int64  `json:"start"`
	Step   int64  `json:"step_s"`
	// Series points are null where mac-pulse was not running.
	Series  []HistorySeries `json:"series"`
	TopApps []HistoryApp    `json:"top_apps"`
}

type HistorySeries struct {
	Name string `json:"name"`
	// Unit is "percent", "bytes", "bytes_per_s", "celsius" or "watts".
	Unit   string     `json:"unit"`
	Points []*float64 `json:"points"`
}

// HistoryApp ranks an app over the range: average percent of one core for cpu,
// average bytes for memory, total bytes for network.
type HistoryApp struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	// System is the Apps tab's flag for the app of the same name, false once it no longer runs.
	System bool    `json:"system"`
	Value  float64 `json:"value"`
}

// Notice is a one-off message for the user, pushed to window.mp.onNotice.
type Notice struct {
	// Level is "info" or "error".
	Level string `json:"level"`
	// Texts are sentences the frontend translates and joins.
	Texts []Text `json:"texts"`
}

// Action answers a button's message (public_ip, ping, export, copy, eject, export_csv,
// speedtest, app_info, storage_scan, cleanup_scan, cleanup_list, cleanup, browser_tabs, proc_detail,
// update_check, alert_detail, quit_app with a signal) through window.mp.onAction.
type Action struct {
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	// Text is the result or the reason it failed. With Key it is the {text} of that dictionary string.
	Text string `json:"text"`
	Key  string `json:"key"`
	// Values is the result of a speedtest (down and up in bytes per second, rpm, rtt_ms) or
	// of a cleanup (bytes, items, failed).
	Values map[string]float64 `json:"values,omitempty"`
	// Info is the result of an app_info; a refusal carries only its Name and PID.
	Info *AppInfo `json:"info,omitempty"`
	// List is the result of a browser_tabs, the tab titles, or of a cleanup that moved entries
	// to the Trash: the name of the folder made there and the folder they came from.
	List []string `json:"list,omitempty"`
	// Detail is the result of a proc_detail.
	Detail *ProcDetail `json:"detail,omitempty"`
	// Review is the result of a cleanup_list.
	Review *CleanupReview `json:"review,omitempty"`
	// Alert is the result of an alert_detail; a refusal carries the id it was asked for in Text.
	Alert *AlertDetail `json:"alert,omitempty"`
}

// ProcDetail is what one process has open, as collector.ProcDetail reads it, plus its parents.
type ProcDetail struct {
	Threads int `json:"threads"`
	// Chain names the parents, from the nearest one to launchd.
	Chain   []string `json:"chain"`
	Files   []string `json:"files"`
	Sockets []string `json:"sockets"`
	// More counts the files and sockets beyond the 200 listed.
	More int `json:"more"`
	// Args is the command line, empty unless the message asked for it.
	Args []string `json:"args"`
}

// AppInfo says what an app or a process is. Every string but Name and Title may be "":
// the page leaves that line out.
type AppInfo struct {
	// Name and PID repeat the message, so the page knows which row the answer is for.
	Name string `json:"name"`
	PID  int32  `json:"pid"`
	// Title is the name of the row: the bundle's folder or the process. BundleName is what the
	// bundle calls itself (CFBundleName), which is the program's own claim.
	Title      string `json:"title"`
	BundleName string `json:"bundle_name"`
	Version    string `json:"version"`
	BundleID   string `json:"bundle_id"`
	Developer  string `json:"developer"`
	// Category is a dictionary string "cat.<Category>" when the dictionaries have it.
	Category string `json:"category"`
	// Path is the .app directory of a bundle, the executable of anything else.
	Path string `json:"path"`
	// Signing is "apple", "developer" (Signer then names who), "adhoc", "none" or "unverified"
	// (signed, but the signature does not verify against Apple's anchors).
	Signing string `json:"signing"`
	Signer  string `json:"signer"`
	// ExecutableOnly is true for a bundle: Signing was checked for its program file alone, not
	// for the resources beside it.
	ExecutableOnly bool `json:"executable_only"`
	// DescKey names the dictionary's description of a well-known macOS process; Manual is
	// the summary of the tool's man page, in English.
	DescKey string `json:"desc_key"`
	Manual  string `json:"manual"`
	// Parent is the process that started this one, with its app when that has another name.
	Parent string `json:"parent"`
	// Since is the launch time in Unix milliseconds, 0 when unknown.
	Since    int64  `json:"since"`
	User     string `json:"user"`
	Killable bool   `json:"killable"`
}
