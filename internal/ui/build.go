package ui

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/store"
)

// topCandidates is twice what a top-five list shows, so an app hovering around fifth place
// stays in the state and keeps its sparkline.
const topCandidates = 10

// noApp stands for the app of a socket nobody can be named for: nettop's frame around it
// was broken, so its pid is unknown.
const noApp = "—"

type Input struct {
	Snap *collector.Snapshot
	// Net is nil while the inspector is stopped.
	Net   *netinspect.Report
	Spark store.Spark
	Today store.Today
	// NetTotals reaches the state only next to the net info, which detail:network asks for.
	NetTotals store.NetTotals
	Active    []alerts.Alert
	Recent    []alerts.Alert
	Settings  settings.Settings
	// SystemLang is settings.MatchLanguage of the macOS preferred languages.
	SystemLang string
	LoginItem  bool
	Self       Self
	Pinned     bool
	// Docker is nil unless a visible view is on the dev tab: with it the state carries dev,
	// built from Listening (the inspector's, nil before its first poll) and Home.
	Docker    *collector.DockerReport
	Listening []netinspect.Listener
	Home      string
	// AllApps sends every app, which only the Apps tab lists; without it the state carries
	// the leaders the top-five lists pick from, a tenth of the size on a busy Mac.
	AllApps bool
	// Storage is nil unless a visible view is on the storage tab: with it the state carries storage.
	Storage *StorageInput
}

// Self is the mac-pulse process: it may signal its own user's processes, never itself.
type Self struct {
	UID uint32
	PID int32
}

func Build(in Input) State {
	snap := in.Snap
	apps := buildApps(snap.Apps, in.Self)
	if !in.AllApps {
		apps = leaders(apps, topCandidates)
		// Only the Apps tab lists processes, and they are most of a leader's bytes.
		for i := range apps {
			apps[i].Procs = []Proc{}
		}
	}
	st := State{
		Time:     snap.Time.UnixMilli(),
		HasRates: snap.HasRates,
		CPU: CPU{
			Total:  snap.CPU.Total,
			User:   snap.CPU.User,
			System: snap.CPU.System,
			Cores:  append([]float64{}, snap.CPU.PerCore...),
			Load:   [3]float64{snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15},
			Uptime: int64(snap.CPU.Uptime.Seconds()),
			Model:  snap.CPU.Model,
		},
		Memory: Memory{
			Total:      snap.Memory.Total,
			Used:       snap.Memory.Used,
			App:        snap.Memory.App,
			Wired:      snap.Memory.Wired,
			Compressed: snap.Memory.Compressed,
			Cached:     snap.Memory.Cached,
			Free:       snap.Memory.Free,
			Pressure:   snap.Memory.Pressure,
			SwapUsed:   snap.Memory.SwapUsed,
			SwapTotal:  snap.Memory.SwapTotal,

			PageInRate: snap.Memory.PageInRate, PageOutRate: snap.Memory.PageOutRate,
			SwapInRate: snap.Memory.SwapInRate, SwapOutRate: snap.Memory.SwapOutRate,
			CompressRate: snap.Memory.CompressRate, DecompressRate: snap.Memory.DecompressRate,
		},
		Disk:    buildDisk(snap.Disk, in.Today.DiskWritten),
		Power:   Power{System: snap.Power.SystemW, Adapter: snap.Power.AdapterW, CPU: snap.Power.CPUW, GPU: snap.Power.GPUW},
		Sensors: buildSensors(snap),
		Network: Network{
			DownRate: snap.Network.DownRate, UpRate: snap.Network.UpRate,
			DownToday: in.Today.NetDown, UpToday: in.Today.NetUp,
		},
		SleepBlockers: make([]SleepBlocker, 0, len(snap.SleepBlockers)),
		Spark: Spark{
			CPU:       append([]float64{}, in.Spark.CPU...),
			Memory:    append([]float64{}, in.Spark.Memory...),
			GPU:       append([]float64{}, in.Spark.GPU...),
			Temp:      append([]float64{}, in.Spark.Temp...),
			Power:     append([]float64{}, in.Spark.Power...),
			DiskRead:  append([]float64{}, in.Spark.DiskRead...),
			DiskWrite: append([]float64{}, in.Spark.DiskWrite...),
			NetDown:   append([]float64{}, in.Spark.NetDown...),
			NetUp:     append([]float64{}, in.Spark.NetUp...),
		},
		Apps:       Apps{HasRates: snap.HasProcRates, Items: apps},
		Alerts:     Alerts{Active: buildAlerts(in.Active), Recent: buildAlerts(in.Recent)},
		Settings:   buildSettings(in.Settings, in.LoginItem),
		SystemLang: in.SystemLang,
		Pinned:     in.Pinned,
		Media:      buildMedia(snap.Media, snap.Apps),
	}
	// The slow collectors have not run yet, or vm_stat failed.
	if st.Memory.Pressure == "" {
		st.Memory.Pressure = collector.PressureNormal
	}
	if snap.CPUTemp > 0 {
		st.CPU.Temp = &snap.CPUTemp
	}
	st.CPU.Clusters = make([]Cluster, 0, len(snap.CPU.Clusters))
	for _, c := range snap.CPU.Clusters {
		st.CPU.Clusters = append(st.CPU.Clusters, Cluster{Name: c.Name, Cores: c.Cores, FreqMHz: known(c.FreqMHz), Temp: known(c.Temp)})
	}
	st.GPU = buildGPU(snap.GPU)
	st.Battery = buildBattery(snap.Battery)
	st.NetInfo = buildNetInfo(snap.NetInfo)
	if st.NetInfo != nil {
		t := in.NetTotals
		st.Network.Down7d, st.Network.Up7d, st.Network.Down30d, st.Network.Up30d = &t.Down7d, &t.Up7d, &t.Down30d, &t.Up30d
	}
	for _, b := range snap.SleepBlockers {
		st.SleepBlockers = append(st.SleepBlockers, SleepBlocker{PID: b.PID, App: displayText(b.App), Kind: b.Kind, Name: displayText(b.Name)})
	}
	if in.Net != nil {
		st.Net = buildNet(in.Net, in.Today.NetApps, SystemApps(snap.Apps, in.Self))
	}
	if in.Docker != nil {
		st.Dev = buildDev(snap, in.Listening, in.Docker, in.Home)
	}
	if in.Storage != nil {
		st.Storage = BuildStorage(*in.Storage, in.Home)
	}
	return st
}

// buildMedia names who records by the Apps tab's group of each pid; a process the sample
// has not seen yet goes by its bundle id, which is its own word and is cut like one.
func buildMedia(m collector.Media, apps []collector.App) Media {
	out := Media{Mic: MediaMic{Active: m.Mic, Apps: []string{}}, Camera: MediaCamera{Active: m.Camera}}
	for i, pid := range m.MicPIDs {
		var name string
		if at := slices.IndexFunc(apps, func(a collector.App) bool { return slices.Contains(a.PIDs, pid) }); at >= 0 {
			name = displayText(apps[at].Name)
		} else if i < len(m.MicBundles) {
			name = collector.CleanText(m.MicBundles[i], maxNameBytes)
		}
		if name != "" && !slices.Contains(out.Mic.Apps, name) {
			out.Mic.Apps = append(out.Mic.Apps, name)
		}
	}
	return out
}

func buildSettings(cfg settings.Settings, loginItem bool) Settings {
	return Settings{
		TempUnit:       cfg.TempUnit,
		NetUnit:        cfg.NetUnit,
		MenuBar:        append([]string{}, cfg.MenuBar...),
		Alerts:         cfg.Alerts,
		AlertCPU:       cfg.AlertCPU,
		AlertTemp:      cfg.AlertTemp,
		AlertDiskFree:  cfg.AlertDiskFree,
		AppsShowSystem: cfg.AppsShowSystem,
		Hotkey:         cfg.Hotkey,
		Language:       cfg.Language,
		Theme:          cfg.Theme,
		Appearance:     cfg.Appearance,
		AlertBattery:   cfg.AlertBattery,
		AlertMemory:    cfg.AlertMemory,
		AlertSwap:      cfg.AlertSwap,
		AlertHold:      cfg.AlertHold,
		AlertMuted:     append([]string{}, cfg.AlertMuted...),
		AlertRules:     append([]settings.Rule{}, cfg.AlertRules...),
		MenuBarCompact: cfg.MenuBarCompact,
		MenuBarGraph:   cfg.MenuBarGraph,
		WindowOnTop:    cfg.WindowOnTop,
		HotkeyQuit:     cfg.HotkeyQuit,
		LaunchAtLogin:  loginItem,

		Clock: cfg.Clock, ClockDate: cfg.ClockDate, ClockSeconds: cfg.ClockSeconds, ClockHours: cfg.ClockHours,
		ClockZones:      append([]string{}, cfg.ClockZones...),
		MenuBarSeparate: cfg.MenuBarSeparate,
		ShowInDock:      cfg.ShowInDock,
		TabOrder:        append([]string{}, cfg.TabOrder...),
		TileOrder:       append([]string{}, cfg.TileOrder...),
	}
}

// tenth keeps one decimal: the apps and the sensors are hundreds of rows a tick, and
// seventeen digits per number would double the state.
func tenth(v float64) float64 {
	return math.Round(v*10) / 10
}

// leaders keeps the apps among the first n by CPU, by memory, by disk I/O or by energy, in
// the order they came. A top-five list sorted by any of the four reads the same from the
// result as from the full list; an app without disk and energy counters cannot lead in those.
func leaders(items []App, n int) []App {
	ranks := []func(App) float64{
		func(a App) float64 { return a.CPU },
		func(a App) float64 { return float64(a.Memory) },
		func(a App) float64 {
			if a.DiskReadRate == nil || a.DiskWriteRate == nil {
				return -1
			}
			return *a.DiskReadRate + *a.DiskWriteRate
		},
		func(a App) float64 {
			if a.EnergyMW == nil {
				return -1
			}
			return *a.EnergyMW
		},
	}
	keep := make([]bool, len(items))
	order := make([]int, len(items))
	for _, rank := range ranks {
		for i := range order {
			order[i] = i
		}
		// Stable, so equal values keep the incoming order, the way the frontend's sort does.
		slices.SortStableFunc(order, func(i, j int) int { return cmp.Compare(rank(items[j]), rank(items[i])) })
		for _, i := range order[:min(n, len(order))] {
			keep[i] = keep[i] || rank(items[i]) >= 0
		}
	}
	out := make([]App, 0, min(len(items), len(ranks)*n))
	for i, a := range items {
		if keep[i] {
			out = append(out, a)
		}
	}
	return out
}

// known turns the collector's "0 is unknown" into the contract's null.
func known(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	return &v
}

// displayText is a string the system supplied, made safe for a row: see collector.CleanText.
func displayText(s string) string {
	return collector.CleanText(s, 0)
}

func buildDisk(d collector.Disk, writtenToday uint64) Disk {
	out := Disk{
		Total:        d.Total,
		Free:         d.Free,
		ReadRate:     d.ReadRate,
		WriteRate:    d.WriteRate,
		WrittenToday: writtenToday,
		Model:        d.Model,
		SMART:        d.SMART,
		ReadTotal:    d.ReadBytes,
		WriteTotal:   d.WriteBytes,
		Volumes:      make([]Volume, 0, len(d.Volumes)),
	}
	for _, v := range d.Volumes {
		out.Volumes = append(out.Volumes, Volume{
			Name: displayText(v.Name), Mount: displayText(v.Mount), FS: v.FS, Total: v.Total, Free: v.Free, Ejectable: v.Ejectable,
		})
	}
	return out
}

func buildGPU(g collector.GPU) *GPU {
	if !g.Present {
		return nil
	}
	return &GPU{
		Util: float64(min(max(g.Util, 0), 100)), Memory: g.Memory, Model: g.Model,
		Renderer: float64(min(max(g.Renderer, 0), 100)), Tiler: float64(min(max(g.Tiler, 0), 100)),
		MemoryAlloc: g.MemoryAlloc, FreqMHz: known(g.FreqMHz), Temp: known(g.Temp),
	}
}

func buildBattery(b collector.Battery) *Battery {
	if !b.Present {
		return nil
	}
	out := &Battery{
		Percent:   min(max(b.Percent, 0), 100),
		State:     "battery",
		Power:     math.Abs(b.Power),
		Health:    min(b.Health, 100),
		Cycles:    b.CycleCount,
		Temp:      b.Temperature,
		DesignMAh: b.DesignMAh,
		MaxMAh:    b.MaxMAh,
		Voltage:   float64(b.Voltage) / 1000,
		Amperage:  math.Abs(float64(b.Amperage)) / 1000,
		LowPower:  b.LowPower,
	}
	switch {
	case b.IsCharging:
		out.State = "charging"
	case b.ExternalConnected:
		out.State = "plugged"
	}
	if b.TimeRemaining > 0 {
		remaining := int64(b.TimeRemaining.Seconds())
		out.TimeRemaining = &remaining
	}
	if b.Adapter != nil {
		out.Adapter = &Adapter{Name: displayText(b.Adapter.Name), Watts: b.Adapter.Watts}
	}
	if !b.UnpluggedAt.IsZero() {
		at := b.UnpluggedAt.UnixMilli()
		out.UnpluggedAt = &at
	}
	return out
}

func buildSensors(snap *collector.Snapshot) Sensors {
	out := Sensors{
		Thermal:   "nominal",
		Fans:      make([]Fan, 0, len(snap.Fans)),
		Temps:     make([]Sensor, 0, len(snap.Temps)),
		Bluetooth: make([]BluetoothDevice, 0, len(snap.Bluetooth)),
	}
	// NSProcessInfoThermalState, in its order.
	if names := []string{"nominal", "fair", "serious", "critical"}; snap.Thermal > 0 {
		out.Thermal = names[min(snap.Thermal, len(names)-1)]
	}
	for _, f := range snap.Fans {
		out.Fans = append(out.Fans, Fan{RPM: f.RPM, Min: f.Min, Max: f.Max})
	}
	for _, t := range snap.Temps {
		out.Temps = append(out.Temps, Sensor{Group: t.Group, Key: displayText(t.Key), Temp: tenth(t.Temp)})
	}
	for _, d := range snap.Bluetooth {
		dev := BluetoothDevice{Name: displayText(d.Name), Kind: displayText(d.Kind), Levels: make([]BatteryLevel, 0, len(d.Levels))}
		for _, l := range d.Levels {
			dev.Levels = append(dev.Levels, BatteryLevel{Part: l.Part, Percent: min(max(l.Percent, 0), 100)})
		}
		out.Bluetooth = append(out.Bluetooth, dev)
	}
	return out
}

func buildNetInfo(info *collector.NetInfo) *NetInfo {
	if info == nil {
		return nil
	}
	out := &NetInfo{
		Interfaces: make([]Interface, 0, len(info.Interfaces)),
		Router:     info.Router,
		DNS:        append([]string{}, info.DNS...),
	}
	for _, i := range info.Interfaces {
		out.Interfaces = append(out.Interfaces, Interface{
			Name: i.Name, MAC: i.MAC, IPv4: append([]string{}, i.IPv4...), IPv6: append([]string{}, i.IPv6...), Primary: i.Primary,
		})
	}
	if w := info.WiFi; w != nil {
		out.WiFi = &WiFi{
			Interface: w.Interface, RSSI: w.RSSI, Noise: w.Noise, Channel: w.Channel, BandGHz: w.BandGHz,
			WidthMHz: w.WidthMHz, TxRate: w.TxRateMbps, PHY: w.PHY, Security: w.Security,
		}
	}
	return out
}

func BuildHistory(h store.History, system map[string]bool) History {
	out := History{
		Metric:  h.Metric,
		Range:   h.Range,
		Start:   h.Start.UnixMilli(),
		Step:    int64(h.Step.Seconds()),
		Series:  make([]HistorySeries, 0, len(h.Series)),
		TopApps: make([]HistoryApp, 0, len(h.TopApps)),
	}
	for _, s := range h.Series {
		out.Series = append(out.Series, HistorySeries{Name: s.Name, Unit: s.Unit, Points: append([]*float64{}, s.Points...)})
	}
	for _, a := range h.TopApps {
		out.TopApps = append(out.TopApps, HistoryApp{
			Name: displayText(a.Name), Icon: selfIcon(a.Name, a.BundlePath), System: system[a.Name], Value: a.Value,
		})
	}
	return out
}

// SystemApps names the sample's apps that the Apps tab marks as system: the screens that
// list apps without their processes mark the same ones. An app that is not running is not in it.
func SystemApps(apps []collector.App, self Self) map[string]bool {
	names := map[string]bool{}
	for _, a := range apps {
		if len(a.Processes) > 0 && !slices.ContainsFunc(a.Processes, func(p collector.Process) bool { return !system(p, self.UID) }) {
			names[a.Name] = true
		}
	}
	return names
}

func buildApps(apps []collector.App, self Self) []App {
	items := make([]App, 0, len(apps))
	for _, a := range apps {
		app := App{
			Name: displayText(a.Name), Icon: selfIcon(a.Name, a.BundlePath), PIDCount: len(a.PIDs), CPU: tenth(a.CPU), Memory: a.RSS,
			System: len(a.Processes) > 0, Procs: make([]Proc, 0, len(a.Processes)),
		}
		if a.HasUsage {
			read, write, energy := math.Round(a.DiskReadRate), math.Round(a.DiskWriteRate), tenth(a.EnergyMW)
			app.DiskReadRate, app.DiskWriteRate, app.EnergyMW = &read, &write, &energy
		}
		for _, p := range a.Processes {
			proc := Proc{
				PID: p.PID, Name: displayText(p.Name), CPU: tenth(p.CPU), Memory: p.RSS,
				System: system(p, self.UID), Killable: killable(p, self), Kind: p.Kind,
			}
			app.System = app.System && proc.System
			app.Killable = app.Killable || proc.Killable
			app.Procs = append(app.Procs, proc)
		}
		items = append(items, app)
	}
	return items
}

// killable is the rule both the pushed state and the quit_app handler apply: without
// root only the user's own processes take a signal, and pid 0, launchd, system
// processes and mac-pulse itself are never a target.
func killable(p collector.Process, self Self) bool {
	return p.PID > 1 && p.PID != self.PID && p.UID == self.UID && !system(p, self.UID)
}

// system separates what macOS runs from what the user runs. An app the user started
// from /System/Applications (Safari, Calculator) lives in a bundle and stays a user app;
// the bundles under /System/Library and /Library/Apple (loginwindow, Dock, XProtect) do not,
// nor does an extension (.appex) of macOS: alone it is a widget, not its app. A tool from
// /usr/bin or /bin is the user's own only on a terminal (yes, python3, curl), not as a
// daemon (ssh-agent, pmset). A process of a browser (Kind) is the user's wherever it lives:
// Safari's helpers sit under /System/Library. Only a protected location makes a process of
// the user's own a system one: a folder name anybody can choose never hides it from Quit.
func system(p collector.Process, uid uint32) bool {
	switch {
	case p.UID != uid:
		return true
	case p.Kind != "":
		return false
	case strings.HasPrefix(p.Exe, "/System/Library/"), strings.HasPrefix(p.Exe, "/Library/Apple/"),
		strings.HasPrefix(p.Exe, "/System/") && strings.Contains(p.Exe, ".appex/"):
		return true
	case strings.Contains(p.Exe, ".app/"), p.TTY != "":
		return false
	}
	// Not all of /usr: /usr/local holds what the user installed.
	for _, dir := range []string{"/System/", "/usr/bin/", "/usr/libexec/", "/usr/sbin/", "/sbin/", "/bin/"} {
		if strings.HasPrefix(p.Exe, dir) {
			return true
		}
	}
	return false
}

// selfIcon substitutes the embedded icon for a mac-pulse row, whose bare binary has none:
// this process, another instance or a `mac-pulse mcp` an AI client started.
func selfIcon(name, bundlePath string) string {
	if name == "mac-pulse" {
		return IconSelf
	}
	return bundlePath
}

func buildAlerts(list []alerts.Alert) []Alert {
	out := make([]Alert, 0, len(list))
	for _, a := range list {
		alert := Alert{
			ID: a.ID, Kind: a.Kind, Params: maps.Clone(a.Params), App: displayText(a.App), Since: a.Since.UnixMilli(),
		}
		if a.App != "" && alert.Params != nil {
			alert.Params["app"] = alert.App
		}
		if !a.Until.IsZero() {
			alert.Until = a.Until.UnixMilli()
		}
		out = append(out, alert)
	}
	return out
}

// BuildAlertDetail is the answer to an alert_detail.
func BuildAlertDetail(rec alerts.Record) *AlertDetail {
	out := &AlertDetail{
		Alert: buildAlerts([]alerts.Alert{rec.Alert})[0], Points: []*float64{}, Top: []AlertRow{}, Procs: []AlertRow{}, Facts: []AlertFact{},
	}
	d := rec.Detail
	if d == nil {
		return out
	}
	out.Recorded, out.Icon, out.Unit, out.Limit, out.Below = true, selfIcon(rec.App, d.Context.BundlePath), d.Unit, d.Limit, d.Below
	out.HoldS, out.FiredAt, out.TopUnit, out.PIDCount = int64(d.Hold.Seconds()), d.FiredAt.UnixMilli(), d.Context.TopUnit, d.Context.PIDs
	if d.N > 0 {
		avg := d.Sum / float64(d.N)
		out.Fired, out.Peak, out.Avg = &d.Fired, &d.Peak, &avg
		if rec.Until.IsZero() {
			out.Current = &d.Last
		}
	}
	out.Start, out.StepS = d.Series.Start*1000, d.Series.Step
	for _, v := range d.Series.V {
		var point *float64
		if v64 := float64(v); !math.IsNaN(v64) && !math.IsInf(v64, 0) {
			point = &v64
		}
		out.Points = append(out.Points, point)
	}
	for _, r := range d.Context.Top {
		out.Top = append(out.Top, AlertRow{Name: displayText(r.Name), Icon: selfIcon(r.Name, r.BundlePath), Value: r.Value})
	}
	for _, r := range d.Context.Procs {
		out.Procs = append(out.Procs, AlertRow{Name: displayText(r.Name), PID: r.PID, Value: r.Value})
	}
	for _, f := range d.Context.Facts {
		out.Facts = append(out.Facts, AlertFact{Key: f.Key, Value: f.Value, Unit: f.Unit})
	}
	return out
}

func buildNet(rep *netinspect.Report, today []store.AppTotal, system map[string]bool) *NetReport {
	net := &NetReport{
		HasRates:    rep.HasRates,
		Apps:        make([]NetApp, 0, len(rep.Apps)),
		Connections: make([]NetConn, 0, len(rep.Conns)),
		Listening:   make([]NetListener, 0, len(rep.Listening)),
		Talkers:     make([]NetTalker, 0, len(rep.Talkers)),
		Today:       make([]NetAppTotal, 0, len(today)),
	}
	for _, a := range rep.Apps {
		net.Apps = append(net.Apps, NetApp{
			Name: displayText(a.App), Icon: selfIcon(a.App, a.BundlePath), System: system[a.App], PIDCount: len(a.PIDs),
			DownRate: a.DownRate, UpRate: a.UpRate,
			DownTotal: a.DownTotal, UpTotal: a.UpTotal, Connections: a.Conns,
		})
	}
	for _, c := range rep.Conns {
		net.Connections = append(net.Connections, NetConn{
			App: cmp.Or(displayText(c.App), noApp), PID: c.PID, Proto: c.Proto, Local: c.Local, RemoteIP: c.RemoteIP, RemotePort: c.RemotePort,
			Host: displayText(c.Host), State: c.State, BytesIn: c.BytesIn, BytesOut: c.BytesOut, DownRate: c.DownRate, UpRate: c.UpRate,
		})
	}
	for _, l := range rep.Listening {
		net.Listening = append(net.Listening, NetListener{
			App: cmp.Or(displayText(l.App), noApp), PID: l.PID, Proto: l.Proto, Addr: l.Addr, Port: l.Port, Dir: displayText(l.Dir),
		})
	}
	for _, t := range rep.Talkers {
		net.Talkers = append(net.Talkers, NetTalker{
			IP: t.IP, Host: displayText(t.Host), Apps: append([]string{}, t.Apps...), DownRate: t.DownRate, UpRate: t.UpRate, Connections: t.Conns,
		})
	}
	for _, t := range today {
		net.Today = append(net.Today, NetAppTotal{Name: displayText(t.Name), Icon: selfIcon(t.Name, t.BundlePath), Down: t.Down, Up: t.Up})
	}
	return net
}
