package ui

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/native"
	"github.com/kl09/mac-pulse/internal/netinspect"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/store"
)

//nolint:funlen // one table of whole-state expectations
func TestBuild(t *testing.T) {
	t.Parallel()

	self := Self{UID: 501, PID: 900}
	at := time.UnixMilli(1790777400000)
	chrome := "/Applications/Google Chrome.app"
	temp, remaining := 61.5, int64(5400)
	watts3 := 3.0
	freq, hot, watts, zero, unplugged := 2570.0, 71.5, 13.2, 0.0, at.Add(-time.Hour).UnixMilli()
	week, month := []uint64{70, 7}, []uint64{300, 30}
	// empty is the state of a zero snapshot: every slice present, every nullable null.
	empty := State{
		Time:          at.UnixMilli(),
		CPU:           CPU{Cores: []float64{}, Clusters: []Cluster{}},
		Memory:        Memory{Pressure: "normal"},
		Disk:          Disk{Volumes: []Volume{}},
		Sensors:       Sensors{Thermal: "nominal", Fans: []Fan{}, Temps: []Sensor{}, Bluetooth: []BluetoothDevice{}},
		SleepBlockers: []SleepBlocker{},
		Spark: Spark{
			CPU: []float64{}, Memory: []float64{}, GPU: []float64{}, Temp: []float64{}, Power: []float64{},
			DiskRead: []float64{}, DiskWrite: []float64{}, NetDown: []float64{}, NetUp: []float64{},
		},
		Apps:   Apps{Items: []App{}},
		Alerts: Alerts{Active: []Alert{}, Recent: []Alert{}},
		Settings: Settings{
			MenuBar: []string{}, AlertMuted: []string{}, AlertRules: []settings.Rule{},
			ClockZones: []string{}, TabOrder: []string{}, TileOrder: []string{},
		},
		Media: Media{Mic: MediaMic{Apps: []string{}}},
	}
	tests := []struct {
		name string
		in   Input
		want func(*State)
	}{
		{name: "empty snapshot has no null slice", in: Input{Snap: &collector.Snapshot{Time: at}}, want: func(*State) {}},
		{
			name: "dev tab: an ejectable volume, browser process kinds, the icon of a second mac-pulse and dev once docker has answered",
			in: Input{
				Self:      self,
				AllApps:   true,
				Home:      "/Users/alex",
				Docker:    &collector.DockerReport{Status: collector.DockerStopped},
				Listening: []netinspect.Listener{{App: "python3", PID: 12, Port: "8765", Dir: "~/proj"}},
				Snap: &collector.Snapshot{
					Time: at,
					Disk: collector.Disk{Volumes: []collector.Volume{{Name: "T7", Mount: "/Volumes/T7", Ejectable: true}}},
					Apps: []collector.App{{Name: "Google Chrome", BundlePath: chrome, PIDs: []int32{70, 71}, Processes: []collector.Process{
						{PID: 70, UID: 501, Name: "Google Chrome", Exe: chrome + "/Contents/MacOS/Google Chrome", Kind: "browser"},
						{PID: 71, UID: 501, Name: "Google Chrome Helper (Renderer)", Exe: chrome + "/Contents/Helper", Kind: "tab"},
					}}, {Name: "mac-pulse", PIDs: []int32{950}}},
				},
			},
			want: func(s *State) {
				s.Disk.Volumes = []Volume{{Name: "T7", Mount: "/Volumes/T7", Ejectable: true}}
				s.Apps.Items = []App{{Name: "Google Chrome", Icon: chrome, PIDCount: 2, Killable: true, Procs: []Proc{
					{PID: 70, Name: "Google Chrome", Killable: true, Kind: "browser"},
					{PID: 71, Name: "Google Chrome Helper (Renderer)", Killable: true, Kind: "tab"},
				}}, {Name: "mac-pulse", Icon: "self", PIDCount: 1, Procs: []Proc{}}}
				s.Dev = &Dev{
					Docker: "stopped", Containers: []DevContainer{}, Agents: []DevAgent{},
					Projects: []DevProject{{Name: "proj", Dir: "~/proj", Ports: []int{8765}, Apps: []string{"python3"}, PIDs: []int32{12}}},
				}
			},
		},
		{
			name: "sensors, gpu, battery on AC and sleep blockers; out-of-range readings clamp",
			in: Input{Snap: &collector.Snapshot{
				Time:    at,
				CPU:     collector.CPU{Total: 30, User: 20, System: 10, PerCore: []float64{40, 20}, Load1: 1, Load5: 2, Load15: 3, Uptime: time.Minute},
				CPUTemp: temp,
				Memory:  collector.Memory{Total: 100, Used: 60, App: 30, Wired: 20, Compressed: 10, Cached: 25, Free: 5, Pressure: "warning"},
				GPU:     collector.GPU{Present: true, Util: 140, Memory: 7},
				Battery: collector.Battery{
					Present: true, ExternalConnected: true, Power: -11.3, TimeRemaining: 90 * time.Minute, Percent: 250, Health: 583000,
				},
				SleepBlockers: []collector.SleepBlocker{
					{PID: 7, App: "caffeinate", Kind: "PreventUserIdleSystemSleep", Name: "caffeinate command-line tool"},
				},
			}},
			want: func(s *State) {
				s.CPU = CPU{
					Total: 30, User: 20, System: 10, Cores: []float64{40, 20}, Load: [3]float64{1, 2, 3}, Uptime: 60, Temp: &temp,
					Clusters: []Cluster{},
				}
				s.Memory = Memory{Total: 100, Used: 60, App: 30, Wired: 20, Compressed: 10, Cached: 25, Free: 5, Pressure: "warning"}
				s.GPU = &GPU{Util: 100, Memory: 7}
				s.Battery = &Battery{State: "plugged", Power: 11.3, TimeRemaining: &remaining, Percent: 100, Health: 100}
				s.SleepBlockers = []SleepBlocker{
					{PID: 7, App: "caffeinate", Kind: "PreventUserIdleSystemSleep", Name: "caffeinate command-line tool"},
				}
			},
		},
		{
			name: "detail sources: an idle cluster and a closed source are null, a stopped fan is 0, mV and mA become V and A",
			in: Input{NetTotals: store.NetTotals{Down7d: 70, Up7d: 7, Down30d: 300, Up30d: 30}, Snap: &collector.Snapshot{
				Time: at,
				CPU: collector.CPU{Model: "Apple M4 Pro", PerCore: []float64{1, 2, 3}, Clusters: []collector.Cluster{
					{Name: "Efficiency", Cores: 1, FreqMHz: freq, Temp: hot},
					{Name: "Performance", Cores: 2},
				}},
				Memory: collector.Memory{PageInRate: 1, PageOutRate: 2, SwapInRate: 3, SwapOutRate: 4, CompressRate: 5, DecompressRate: 6},
				Disk: collector.Disk{
					ReadBytes: 10, WriteBytes: 20, Model: "APPLE SSD", SMART: "verified",
					Volumes: []collector.Volume{{Name: "Macintosh HD", Mount: "/", FS: "apfs", Total: 100, Free: 40}},
				},
				GPU: collector.GPU{Present: true, Model: "Apple M4 Pro", Renderer: 120, Tiler: -3, MemoryAlloc: 9, Temp: hot},
				Battery: collector.Battery{
					Present: true, Voltage: 12610, Amperage: -896, DesignMAh: 6249, MaxMAh: 5831, UnpluggedAt: at.Add(-time.Hour),
				},
				Power:   collector.Power{SystemW: &watts, AdapterW: &zero},
				Thermal: 2,
				Fans:    []native.Fan{{RPM: 0, Min: 2317, Max: 7826}},
				Temps:   []collector.Sensor{{Group: "GPU", Key: "Tg0f", Temp: hot}},
				Bluetooth: []collector.BluetoothDevice{
					{Name: "Pods\u202e", Kind: "Headphones", Levels: []collector.BatteryLevel{{Part: "left", Percent: 130}}},
					{Name: "Keyboard"},
				},
				NetInfo: &collector.NetInfo{
					Interfaces: []collector.Interface{{Name: "en0", MAC: "aa:bb", IPv4: []string{"10.0.0.2"}, Primary: true}},
					Router:     "10.0.0.1",
					WiFi: &native.WiFi{
						Interface: "en0", PHY: "802.11ax", Security: "WPA3 Personal",
						RSSI: -52, Noise: -91, Channel: 44, WidthMHz: 80, BandGHz: 5, TxRateMbps: 864,
					},
				},
			}},
			want: func(s *State) {
				s.CPU = CPU{Cores: []float64{1, 2, 3}, Model: "Apple M4 Pro", Clusters: []Cluster{
					{Name: "Efficiency", Cores: 1, FreqMHz: &freq, Temp: &hot},
					{Name: "Performance", Cores: 2},
				}}
				s.Memory = Memory{
					Pressure: "normal", PageInRate: 1, PageOutRate: 2, SwapInRate: 3, SwapOutRate: 4, CompressRate: 5, DecompressRate: 6,
				}
				s.Disk = Disk{
					ReadTotal: 10, WriteTotal: 20, Model: "APPLE SSD", SMART: "verified",
					Volumes: []Volume{{Name: "Macintosh HD", Mount: "/", FS: "apfs", Total: 100, Free: 40}},
				}
				s.GPU = &GPU{Model: "Apple M4 Pro", Renderer: 100, MemoryAlloc: 9, Temp: &hot}
				s.Battery = &Battery{State: "battery", Voltage: 12.61, Amperage: 0.896, DesignMAh: 6249, MaxMAh: 5831, UnpluggedAt: &unplugged}
				s.Power = Power{System: &watts, Adapter: &zero}
				s.Sensors = Sensors{
					Thermal: "serious",
					Fans:    []Fan{{Min: 2317, Max: 7826}},
					Temps:   []Sensor{{Group: "GPU", Key: "Tg0f", Temp: hot}},
					Bluetooth: []BluetoothDevice{
						{Name: "Pods", Kind: "Headphones", Levels: []BatteryLevel{{Part: "left", Percent: 100}}},
						{Name: "Keyboard", Levels: []BatteryLevel{}},
					},
				}
				s.Network = Network{Down7d: &week[0], Up7d: &week[1], Down30d: &month[0], Up30d: &month[1]}
				s.NetInfo = &NetInfo{
					Interfaces: []Interface{{Name: "en0", MAC: "aa:bb", IPv4: []string{"10.0.0.2"}, IPv6: []string{}, Primary: true}},
					Router:     "10.0.0.1",
					DNS:        []string{},
					WiFi: &WiFi{
						Interface: "en0", RSSI: -52, Noise: -91, Channel: 44, BandGHz: 5, WidthMHz: 80, TxRate: 864,
						PHY: "802.11ax", Security: "WPA3 Personal",
					},
				}
			},
		},
		{
			name: "an adapter on AC, an unknown thermal state, rounded usage only for the user's own apps; bidi controls leave names",
			in: Input{Self: self, AllApps: true, Snap: &collector.Snapshot{
				Time:    at,
				Thermal: 9,
				Battery: collector.Battery{Present: true, ExternalConnected: true, Adapter: &collector.Adapter{Name: "70W USB-C", Watts: 68}},
				Apps: []collector.App{
					{
						Name: "gpj.exe\u202e", HasUsage: true, DiskReadRate: 3.4, EnergyMW: 13.2499,
						Processes: []collector.Process{{PID: 5, UID: 501, Name: "\u2066evil\u2069", Exe: "/opt/evil"}},
					},
					{Name: "mds", Processes: []collector.Process{{PID: 6, UID: 0, Name: "mds"}}},
				},
			}},
			want: func(s *State) {
				s.Sensors.Thermal = "critical"
				s.Battery = &Battery{State: "plugged", Adapter: &Adapter{Name: "70W USB-C", Watts: 68}}
				s.Apps = Apps{Items: []App{
					{
						Name: "gpj.exe", Killable: true, DiskReadRate: &watts3, DiskWriteRate: &zero, EnergyMW: &watts,
						Procs: []Proc{{PID: 5, Name: "evil", Killable: true}},
					},
					{Name: "mds", System: true, Procs: []Proc{{PID: 6, Name: "mds", System: true}}},
				}}
			},
		},
		{
			name: "killable is the user's own non-system process above pid 1; system is another uid, a system path off a tty, .appex, system bundle",
			in: Input{Self: self, AllApps: true, Snap: &collector.Snapshot{Time: at, HasProcRates: true, Apps: []collector.App{
				{
					Name: "Google Chrome", BundlePath: chrome, PIDs: []int32{2, 3}, CPU: 12.5, RSS: 1024,
					Processes: []collector.Process{
						{PID: 2, UID: 501, Name: "Google Chrome Helper", Exe: chrome + "/Contents/Helpers/Google Chrome Helper", CPU: 12.5, RSS: 1000},
						{PID: 3, UID: 0, Name: "updater", Exe: chrome + "/Contents/Helpers/updater", RSS: 24},
					},
				},
				{Name: "Safari", Processes: []collector.Process{{PID: 4, UID: 501, Exe: "/System/Applications/Safari.app/Contents/MacOS/Safari"}}},
				{Name: "yes", Processes: []collector.Process{{PID: 5, UID: 501, TTY: "ttys000", Exe: "/usr/bin/yes"}}},
				{Name: "node", Processes: []collector.Process{{PID: 6, UID: 501, Exe: "/usr/local/bin/node"}}},
				{Name: "gone", Processes: []collector.Process{{PID: 7, UID: 501}}},
				{Name: "launchd", Processes: []collector.Process{{PID: 1, UID: 0, Exe: "/sbin/launchd"}}},
				{Name: "mine at pid 1", Processes: []collector.Process{{PID: 1, UID: 501, Exe: "/opt/x"}}},
				{Name: "kernel_task", Processes: []collector.Process{{PID: 0, UID: 0}}},
				{Name: "mac-pulse", PIDs: []int32{900}, Processes: []collector.Process{{PID: 900, UID: 501, Exe: "/Users/alex/bin/mac-pulse"}}},
				{Name: "trustd", Processes: []collector.Process{{PID: 8, UID: 501, Exe: "/usr/libexec/trustd"}}},
				{Name: "loginwindow", Processes: []collector.Process{
					{PID: 9, UID: 501, Exe: "/System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow"},
				}},
				{Name: "Calculator", Processes: []collector.Process{
					{PID: 10, UID: 501, Exe: "/System/Applications/Calculator.app/Contents/MacOS/Calculator"},
				}},
				{Name: "zsh", Processes: []collector.Process{{PID: 11, UID: 501, TTY: "ttys000", Exe: "/bin/zsh"}}},
				{Name: "pmset", Processes: []collector.Process{{PID: 12, UID: 501, Exe: "/usr/bin/pmset"}}},
				{Name: "head", Processes: []collector.Process{{PID: 13, UID: 501, Exe: "/usr/bin/head"}}},
				{Name: "ssh-agent", Processes: []collector.Process{{PID: 14, UID: 501, Exe: "/usr/bin/ssh-agent"}}},
				{Name: "cloudphotod", Processes: []collector.Process{
					{PID: 15, UID: 501, Exe: "/System/Library/PrivateFrameworks/CloudPhotoServices.framework/Helpers/cloudphotod"},
				}},
				{Name: "XProtect", Processes: []collector.Process{
					{PID: 16, UID: 501, Exe: "/Library/Apple/System/Library/CoreServices/XProtect.app/Contents/MacOS/XProtect"},
				}},
				{Name: "Weather", Processes: []collector.Process{
					{PID: 17, UID: 501, Exe: "/System/Applications/Weather.app/Contents/PlugIns/WeatherWidget.appex/Contents/MacOS/WeatherWidget"},
				}},
				{Name: "Calendar", Processes: []collector.Process{
					{PID: 18, UID: 501, Exe: "/System/Applications/Calendar.app/Contents/MacOS/Calendar"},
					{PID: 19, UID: 501, Exe: "/System/Applications/Calendar.app/Contents/PlugIns/CalendarWidgetExtension.appex/Contents/MacOS/Widget"},
				}},
				{Name: "sh", Processes: []collector.Process{{PID: 20, UID: 501, Exe: "/bin/sh"}}},
				{Name: "empty group"},
			}}},
			want: func(s *State) {
				s.Apps = Apps{HasRates: true, Items: []App{
					{
						Name: "Google Chrome", Icon: chrome, PIDCount: 2, CPU: 12.5, Memory: 1024, Killable: true,
						Procs: []Proc{
							{PID: 2, Name: "Google Chrome Helper", CPU: 12.5, Memory: 1000, Killable: true},
							{PID: 3, Name: "updater", Memory: 24, System: true},
						},
					},
					{Name: "Safari", Killable: true, Procs: []Proc{{PID: 4, Killable: true}}},
					{Name: "yes", Killable: true, Procs: []Proc{{PID: 5, Killable: true}}},
					{Name: "node", Killable: true, Procs: []Proc{{PID: 6, Killable: true}}},
					{Name: "gone", Killable: true, Procs: []Proc{{PID: 7, Killable: true}}},
					{Name: "launchd", System: true, Procs: []Proc{{PID: 1, System: true}}},
					{Name: "mine at pid 1", Procs: []Proc{{PID: 1}}},
					{Name: "kernel_task", System: true, Procs: []Proc{{PID: 0, System: true}}},
					{Name: "mac-pulse", Icon: "self", PIDCount: 1, Procs: []Proc{{PID: 900}}},
					{Name: "trustd", System: true, Procs: []Proc{{PID: 8, System: true}}},
					{Name: "loginwindow", System: true, Procs: []Proc{{PID: 9, System: true}}},
					{Name: "Calculator", Killable: true, Procs: []Proc{{PID: 10, Killable: true}}},
					{Name: "zsh", Killable: true, Procs: []Proc{{PID: 11, Killable: true}}},
					{Name: "pmset", System: true, Procs: []Proc{{PID: 12, System: true}}},
					{Name: "head", System: true, Procs: []Proc{{PID: 13, System: true}}},
					{Name: "ssh-agent", System: true, Procs: []Proc{{PID: 14, System: true}}},
					{Name: "cloudphotod", System: true, Procs: []Proc{{PID: 15, System: true}}},
					{Name: "XProtect", System: true, Procs: []Proc{{PID: 16, System: true}}},
					{Name: "Weather", System: true, Procs: []Proc{{PID: 17, System: true}}},
					{Name: "Calendar", Killable: true, Procs: []Proc{{PID: 18, Killable: true}, {PID: 19, System: true}}},
					{Name: "sh", System: true, Procs: []Proc{{PID: 20, System: true}}},
					{Name: "empty group", Procs: []Proc{}},
				}}
			},
		},
		{
			name: "who records is named by the Apps group of the pid, else by the bundle id, once each; the camera has no names",
			in: Input{Self: self, Snap: &collector.Snapshot{
				Time: at,
				Apps: []collector.App{{Name: "Zoom\u202e", PIDs: []int32{40, 41}}},
				Media: collector.Media{
					Mic: true, Camera: true,
					MicPIDs:    []int32{41, 40, 77, 78, 79},
					MicBundles: []string{"us.zoom.xos", "us.zoom.xos", "com.example.rec\u202e", "", "com.example.rec"},
				},
			}},
			want: func(s *State) {
				s.Apps.Items = []App{{Name: "Zoom", PIDCount: 2, Procs: []Proc{}}}
				s.Media = Media{Mic: MediaMic{Active: true, Apps: []string{"Zoom", "com.example.rec"}}, Camera: MediaCamera{Active: true}}
			},
		},
		{
			name: "Low Power Mode reaches the battery",
			in:   Input{Snap: &collector.Snapshot{Time: at, Battery: collector.Battery{Present: true, Percent: 50, LowPower: true}}},
			want: func(s *State) {
				s.Battery = &Battery{Percent: 50, State: "battery", LowPower: true}
			},
		},
		{
			name: "a group that holds the mac-pulse pid under another name keeps its own icon",
			in: Input{
				Self: self, AllApps: true,
				Snap: &collector.Snapshot{Time: at, Apps: []collector.App{{
					Name: "codex", BundlePath: "/Applications/Codex.app", PIDs: []int32{40, 900},
					Processes: []collector.Process{
						{PID: 40, UID: 501, Name: "codex", TTY: "ttys001"}, {PID: 900, UID: 501, Name: "mac-pulse", TTY: "ttys001"},
					},
				}}},
			},
			want: func(s *State) {
				s.Apps.Items = []App{{Name: "codex", Icon: "/Applications/Codex.app", PIDCount: 2, Killable: true, Procs: []Proc{
					{PID: 40, Name: "codex", Killable: true}, {PID: 900, Name: "mac-pulse"},
				}}}
			},
		},
		{
			name: "network report carries icons, the system flag of the Apps tab and the traffic since midnight; " +
				"outside the Apps tab an app keeps its flags and carries no process rows",
			in: Input{
				Self: self,
				Snap: &collector.Snapshot{Time: at, Apps: []collector.App{
					{Name: "mDNSResponder", Processes: []collector.Process{{PID: 5, UID: 65, Exe: "/usr/sbin/mDNSResponder"}}},
				}},
				Net: &netinspect.Report{
					HasRates: true,
					Apps: []netinspect.AppTraffic{
						{App: "Google Chrome", BundlePath: chrome, PIDs: []int32{3}, DownRate: 10, Conns: 1},
						{App: "mac-pulse", PIDs: []int32{900}},
						{App: "mDNSResponder", PIDs: []int32{5}},
					},
					Talkers: []netinspect.Talker{{IP: "1.1.1.1", Conns: 2}},
					Listening: []netinspect.Listener{
						{App: "node", PID: 12, Proto: "tcp", Addr: "*", Port: "3000", Dir: "~/code/shop"},
						// A socket after a broken nettop frame belongs to nobody that can be named.
						{Proto: "tcp", Addr: "*", Port: "47812"},
					},
					Conns: []netinspect.Conn{{Proto: "tcp", RemoteIP: "1.1.1.1"}},
				},
				// Without the net info the totals stay out of the state.
				NetTotals: store.NetTotals{Down7d: 70, Up7d: 7, Down30d: 300, Up30d: 30},
				Today: store.Today{
					DiskWritten: 7, NetDown: 8, NetUp: 9,
					NetApps: []store.AppTotal{{Name: "Google Chrome", BundlePath: chrome, Down: 5, Up: 6}},
				},
			},
			want: func(s *State) {
				s.Disk.WrittenToday = 7
				s.Apps.Items = []App{{Name: "mDNSResponder", PIDCount: 0, System: true, Procs: []Proc{}}}
				s.Network = Network{DownToday: 8, UpToday: 9}
				s.Net = &NetReport{
					HasRates: true,
					Apps: []NetApp{
						{Name: "Google Chrome", Icon: chrome, PIDCount: 1, DownRate: 10, Connections: 1},
						{Name: "mac-pulse", Icon: "self", PIDCount: 1},
						{Name: "mDNSResponder", System: true, PIDCount: 1},
					},
					Connections: []NetConn{{App: "—", Proto: "tcp", RemoteIP: "1.1.1.1"}},
					Listening: []NetListener{
						{App: "node", PID: 12, Proto: "tcp", Addr: "*", Port: "3000", Dir: "~/code/shop"},
						{App: "—", Proto: "tcp", Addr: "*", Port: "47812"},
					},
					Talkers: []NetTalker{{IP: "1.1.1.1", Apps: []string{}, Connections: 2}},
					Today:   []NetAppTotal{{Name: "Google Chrome", Icon: chrome, Down: 5, Up: 6}},
				}
			},
		},
		{
			name: "sparklines, alerts and settings",
			in: Input{
				Snap:   &collector.Snapshot{Time: at},
				Spark:  store.Spark{CPU: []float64{1, 2}, NetUp: []float64{3}},
				Active: []alerts.Alert{{ID: "cpu::1", Kind: "cpu", Params: map[string]any{"value": 95.0}, Since: at}},
				Recent: []alerts.Alert{{
					ID: "app_cpu:yes\u202e:2", Kind: "app_cpu", App: "yes\u202e", Params: map[string]any{"app": "yes\u202e"},
					Since: at, Until: at.Add(time.Minute),
				}},
				Settings: settings.Settings{
					TempUnit: "F", NetUnit: "bits", MenuBar: []string{"mem", "cpu"}, Alerts: true,
					AlertCPU: 80, AlertTemp: 90, AlertDiskFree: 5, AppsShowSystem: true, Hotkey: true, Language: "de",
					Theme: "nord", Appearance: "dark", AlertBattery: 15, AlertMemory: 90, AlertSwap: 4, AlertHold: 30, AlertMuted: []string{"yes"},
					AlertRules:     []settings.Rule{{App: "yes", Metric: "cpu", Limit: 150, Minutes: 5}},
					MenuBarCompact: true, MenuBarGraph: true, WindowOnTop: true, HotkeyQuit: true,
					Clock: true, ClockDate: true, ClockSeconds: true, ClockHours: "24", ClockZones: []string{"Asia/Tokyo"},
					MenuBarSeparate: true, ShowInDock: true, TabOrder: []string{"storage", "overview"}, TileOrder: []string{"battery"},
				},
				SystemLang: "ru",
				LoginItem:  true,
				Pinned:     true,
			},
			want: func(s *State) {
				s.Spark.CPU, s.Spark.NetUp = []float64{1, 2}, []float64{3}
				s.Alerts = Alerts{
					Active: []Alert{{ID: "cpu::1", Kind: "cpu", Params: map[string]any{"value": 95.0}, Since: at.UnixMilli()}},
					Recent: []Alert{{
						ID: "app_cpu:yes\u202e:2", Kind: "app_cpu", App: "yes", Params: map[string]any{"app": "yes"},
						Since: at.UnixMilli(), Until: at.UnixMilli() + 60000,
					}},
				}
				s.Settings = Settings{
					TempUnit: "F", NetUnit: "bits", MenuBar: []string{"mem", "cpu"}, Alerts: true,
					AlertCPU: 80, AlertTemp: 90, AlertDiskFree: 5, AppsShowSystem: true, Hotkey: true, Language: "de", LaunchAtLogin: true,
					Theme: "nord", Appearance: "dark", AlertBattery: 15, AlertMemory: 90, AlertSwap: 4, AlertHold: 30, AlertMuted: []string{"yes"},
					AlertRules:     []settings.Rule{{App: "yes", Metric: "cpu", Limit: 150, Minutes: 5}},
					MenuBarCompact: true, MenuBarGraph: true, WindowOnTop: true, HotkeyQuit: true,
					Clock: true, ClockDate: true, ClockSeconds: true, ClockHours: "24", ClockZones: []string{"Asia/Tokyo"},
					MenuBarSeparate: true, ShowInDock: true, TabOrder: []string{"storage", "overview"}, TileOrder: []string{"battery"},
				}
				s.SystemLang, s.Pinned = "ru", true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			want := empty
			tt.want(&want)

			assert.Equal(t, want, Build(tt.in))
		})
	}
}

func TestSystem(t *testing.T) {
	t.Parallel()

	webContent := "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/" +
		"com.apple.WebKit.WebContent.xpc/Contents/MacOS/com.apple.WebKit.WebContent"
	widget := "/System/Applications/Weather.app/Contents/PlugIns/WeatherWidget.appex/Contents/MacOS/WeatherWidget"
	tests := []struct {
		name string
		proc collector.Process
		want bool
	}{
		{name: "a folder named .appex elsewhere hides nothing", proc: collector.Process{UID: 501, Exe: "/Users/alex/Widget.appex/miner"}},
		{
			name: "an extension inside an installed app is the user's",
			proc: collector.Process{UID: 501, Exe: "/Applications/Xcode.app/Contents/PlugIns/Preview.appex/Contents/MacOS/Preview"},
		},
		{name: "an extension of macOS is a system process", proc: collector.Process{UID: 501, Exe: widget}, want: true},
		{name: "a helper under /System/Library is a system process", proc: collector.Process{UID: 501, Exe: webContent}, want: true},
		{name: "the same helper working for a browser is the user's", proc: collector.Process{UID: 501, Exe: webContent, Kind: "tab"}},
		{name: "another user's browser process stays a system one", proc: collector.Process{UID: 0, Exe: webContent, Kind: "tab"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, system(tt.proc, 501))
			tt.proc.PID = 7
			assert.Equal(t, !tt.want, killable(tt.proc, Self{UID: 501, PID: 900}))
		})
	}
}

func TestLeaders(t *testing.T) {
	t.Parallel()

	zero, low, high := 0.0, 2.0, 900.0
	// Incoming order is CPU descending, the order the collector sorts by.
	busy := App{Name: "busy", CPU: 90, Memory: 10}
	daemon := App{Name: "root daemon", CPU: 50, Memory: 20}
	writer := App{Name: "writer", CPU: 5, Memory: 30, DiskReadRate: &low, DiskWriteRate: &high, EnergyMW: &low}
	hungry := App{Name: "hungry", CPU: 4, Memory: 40, DiskReadRate: &zero, DiskWriteRate: &zero, EnergyMW: &high}
	fat := App{Name: "fat", CPU: 1, Memory: 5000, DiskReadRate: &zero, DiskWriteRate: &zero, EnergyMW: &zero}
	idle := App{Name: "idle", DiskReadRate: &zero, DiskWriteRate: &zero, EnergyMW: &zero}
	items := []App{busy, daemon, writer, hungry, fat, idle}
	tests := []struct {
		name  string
		items []App
		n     int
		want  []App
	}{
		{name: "no apps", n: 10, want: []App{}},
		{name: "fewer apps than places keep them all", items: items, n: 10, want: items},
		{
			name:  "the leader of each ranking stays, in the incoming order; no counters, no place in disk and energy",
			items: items, n: 1, want: []App{busy, writer, hungry, fat},
		},
		{
			name:  "a tie goes to the app that came first, as the frontend's stable sort decides",
			items: items, n: 2, want: []App{busy, daemon, writer, hungry, fat},
		},
		{name: "no places", items: items, n: 0, want: []App{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, leaders(tt.items, tt.n))
		})
	}
}

func TestBuildHistory(t *testing.T) {
	t.Parallel()

	at := time.UnixMilli(1790777400000)
	v := 12.5
	tests := []struct {
		name   string
		in     store.History
		system map[string]bool
		want   History
	}{
		{
			name: "no series and no apps marshal as empty arrays",
			in:   store.History{Metric: "gpu", Range: "1h", Start: at, Step: time.Minute},
			want: History{Metric: "gpu", Range: "1h", Start: at.UnixMilli(), Step: 60, Series: []HistorySeries{}, TopApps: []HistoryApp{}},
		},
		{
			name: "gaps stay null, the bundle path becomes the icon, mac-pulse gets its own and a system app its flag",
			in: store.History{
				Metric: "cpu", Range: "30d", Start: at, Step: 4 * time.Hour,
				Series: []store.Series{{Name: "total", Unit: "percent", Points: []*float64{nil, &v}}},
				TopApps: []store.AppValue{
					{Name: "Xcode", BundlePath: "/Applications/Xcode.app", Value: 80}, {Name: "mac-pulse", Value: 1}, {Name: "WindowServer", Value: 9},
				},
			},
			system: map[string]bool{"WindowServer": true},
			want: History{
				Metric: "cpu", Range: "30d", Start: at.UnixMilli(), Step: 14400,
				Series: []HistorySeries{{Name: "total", Unit: "percent", Points: []*float64{nil, &v}}},
				TopApps: []HistoryApp{
					{Name: "Xcode", Icon: "/Applications/Xcode.app", Value: 80},
					{Name: "mac-pulse", Icon: "self", Value: 1},
					{Name: "WindowServer", System: true, Value: 9},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, BuildHistory(tt.in, tt.system))
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestBuildAlertDetail(t *testing.T) {
	t.Parallel()

	since := time.UnixMilli(1790777400000)
	nan := float32(math.NaN())
	fired, peak, avg, last, low := 212.0, 240.0, 200.0, 181.0, 9.0
	p0, p1 := 200.0, 240.0
	tests := []struct {
		name string
		in   alerts.Record
		want *AlertDetail
	}{
		{
			name: "record of a build that kept no detail: the alert alone, every list empty",
			in: alerts.Record{Alert: alerts.Alert{
				ID: "disk::5", Kind: alerts.KindDisk, Params: map[string]any{"value": 9.4, "limit": 10}, Since: since, Until: since.Add(time.Hour),
			}},
			want: &AlertDetail{
				Alert: Alert{
					ID: "disk::5", Kind: "disk", Params: map[string]any{"value": 9.4, "limit": 10},
					Since: since.UnixMilli(), Until: since.Add(time.Hour).UnixMilli(),
				},
				Points: []*float64{}, Top: []AlertRow{}, Procs: []AlertRow{}, Facts: []AlertFact{},
			},
		},
		{
			name: "open app alert: the current value, the icon, names cleaned, a bucket without a sample null",
			in: alerts.Record{
				Alert: alerts.Alert{
					ID: "app_cpu:x\u202ez:5", Kind: alerts.KindAppCPU, App: "x\u202ez",
					Params: map[string]any{"app": "x\u202ez"}, Since: since,
				},
				Detail: &alerts.Detail{
					Unit: alerts.UnitPercent, Limit: 150, Hold: 5 * time.Minute, FiredAt: since.Add(5 * time.Minute),
					Fired: 212, Peak: 240, Last: 181, Sum: 600, N: 3,
					Series: alerts.Series{Start: since.Unix(), Step: 10, V: []float32{200, nan, 240}},
					Context: alerts.Context{
						BundlePath: "/Applications/xz.app", PIDs: 4, TopUnit: alerts.UnitPercent,
						Top:   []alerts.Row{{Name: "x\u202ez", BundlePath: "/Applications/xz.app", Value: 212}, {Name: "mac-pulse", Value: 3}},
						Procs: []alerts.Row{{Name: "x\u202ez helper", PID: 7, Value: 100}},
					},
				},
			},
			want: &AlertDetail{
				Alert:    Alert{ID: "app_cpu:x\u202ez:5", Kind: "app_cpu", App: "xz", Params: map[string]any{"app": "xz"}, Since: since.UnixMilli()},
				Recorded: true, Icon: "/Applications/xz.app", Unit: "percent", Limit: 150, HoldS: 300, FiredAt: since.Add(5 * time.Minute).UnixMilli(),
				Fired: &fired, Peak: &peak, Avg: &avg, Current: &last,
				Start: since.UnixMilli(), StepS: 10, Points: []*float64{&p0, nil, &p1},
				Top:     []AlertRow{{Name: "xz", Icon: "/Applications/xz.app", Value: 212}, {Name: "mac-pulse", Icon: "self", Value: 3}},
				TopUnit: "percent", PIDCount: 4, Procs: []AlertRow{{Name: "xz helper", PID: 7, Value: 100}}, Facts: []AlertFact{},
			},
		},
		{
			name: "ended system alert under its limit: no current value, the facts",
			in: alerts.Record{
				Alert: alerts.Alert{ID: "disk::5", Kind: alerts.KindDisk, Since: since, Until: since.Add(time.Hour)},
				Detail: &alerts.Detail{
					Unit: alerts.UnitPercent, Limit: 10, Below: true, Hold: time.Minute, FiredAt: since.Add(time.Minute),
					Fired: 9, Peak: 9, Last: 12, Sum: 9, N: 1,
					Context: alerts.Context{TopUnit: alerts.UnitBytesPerS, Facts: []alerts.Fact{{Key: "disk_free", Value: 42, Unit: alerts.UnitBytes}}},
				},
			},
			want: &AlertDetail{
				Alert:    Alert{ID: "disk::5", Kind: "disk", Since: since.UnixMilli(), Until: since.Add(time.Hour).UnixMilli()},
				Recorded: true, Unit: "percent", Limit: 10, Below: true, HoldS: 60, FiredAt: since.Add(time.Minute).UnixMilli(),
				Fired: &low, Peak: &low, Avg: &low,
				Points: []*float64{}, Top: []AlertRow{}, TopUnit: "bytes_per_s", Procs: []AlertRow{},
				Facts: []AlertFact{{Key: "disk_free", Value: 42, Unit: "bytes"}},
			},
		},
		{
			name: "metric without a reading (thermal state, no sensor): the four values are null",
			in: alerts.Record{
				Alert: alerts.Alert{ID: "thermal::5", Kind: alerts.KindThermal, Since: since},
				Detail: &alerts.Detail{
					Unit: alerts.UnitCelsius, Hold: time.Minute, FiredAt: since.Add(time.Minute),
					Context: alerts.Context{TopUnit: alerts.UnitPercent},
				},
			},
			want: &AlertDetail{
				Alert:    Alert{ID: "thermal::5", Kind: "thermal", Since: since.UnixMilli()},
				Recorded: true, Unit: "celsius", HoldS: 60, FiredAt: since.Add(time.Minute).UnixMilli(),
				Points: []*float64{}, Top: []AlertRow{}, TopUnit: "percent", Procs: []AlertRow{}, Facts: []AlertFact{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildAlertDetail(tt.in)

			assert.Equal(t, tt.want, got)
			_, err := json.Marshal(got)
			assert.NoError(t, err)
		})
	}
}
