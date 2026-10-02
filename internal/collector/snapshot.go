package collector

import (
	"time"

	"github.com/kl09/mac-pulse/internal/native"
)

type Snapshot struct {
	Time         time.Time
	HasRates     bool
	HasProcRates bool // false until the second process scan; per-process CPU is 0 before that
	CPU          CPU
	CPUTemp      float64
	Memory       Memory
	Disk         Disk
	Network      Network
	Battery      Battery
	GPU          GPU
	Processes    []Process
	Apps         []App
	// SleepBlockers refreshes every 5th process scan.
	SleepBlockers []SleepBlocker
	Power         Power
	// Thermal is 0 nominal, 1 fair, 2 serious, 3 critical.
	Thermal int
	// Fans is empty on a fanless Mac and when the SMC is closed; RPM 0 is a stopped fan.
	Fans []native.Fan
	// Temps and NetInfo are filled only while their Detail flag is on.
	Temps []Sensor
	// Bluetooth is always filled; Detail.Bluetooth only makes it fresher.
	Bluetooth []BluetoothDevice
	NetInfo   *NetInfo
	// Agents is filled only while Detail.Dev is on.
	Agents []Agent
	Media  Media
}

// Media says whether the microphone and the camera are in use, read without asking for
// either permission.
type Media struct {
	Mic bool
	// MicPIDs and MicBundles name the processes recording, index for index; a bundle id may
	// be "". Both are empty on macOS 13, which only tells that the microphone is in use.
	MicPIDs    []int32
	MicBundles []string
	// Camera has no list of processes: macOS does not tell who uses the camera.
	Camera bool
}

// CPU.User and CPU.System are percent of all cores and add up to Total.
type CPU struct {
	Total   float64
	User    float64
	System  float64
	PerCore []float64
	Load1   float64
	Load5   float64
	Load15  float64
	Uptime  time.Duration
	Model   string
	// Clusters follow the order of PerCore: the first Clusters[0].Cores entries belong to
	// Clusters[0]. Empty on a Mac without performance levels.
	Clusters []Cluster
}

type Cluster struct {
	// Name is "Efficiency" or "Performance", as sysctl reports it.
	Name  string
	Cores int
	// FreqMHz is 0 when the cluster idled through the whole tick or the source is closed.
	FreqMHz float64
	// Temp is the hottest sensor of the cluster, 0 unless Detail.Temps is on.
	Temp float64
}

// Memory.Used is App + Wired + Compressed, the way Activity Monitor counts it.
type Memory struct {
	Total      uint64
	Used       uint64
	App        uint64
	Wired      uint64
	Compressed uint64
	Cached     uint64
	Free       uint64
	// Pressure is PressureNormal, PressureWarning or PressureCritical.
	Pressure  string
	SwapUsed  uint64
	SwapTotal uint64
	// Cumulative bytes since boot, and their rates between two memory reads.
	PageIns        uint64
	PageOuts       uint64
	SwapIns        uint64
	SwapOuts       uint64
	Compressions   uint64
	Decompressions uint64
	PageInRate     float64
	PageOutRate    float64
	SwapInRate     float64
	SwapOutRate    float64
	CompressRate   float64
	DecompressRate float64
}

// Disk and Network carry cumulative byte counters; the rates are deltas
// against the previous snapshot in bytes per second.
type Disk struct {
	Total      uint64
	Free       uint64
	ReadBytes  uint64
	WriteBytes uint64
	ReadRate   float64
	WriteRate  float64
	// Model and SMART stay "" until Detail.SMART has been on once; SMART is then
	// SMARTVerified, SMARTFailing or "" for a disk that reports no status.
	Model string
	SMART string
	// Volumes lists "/" first, then /Volumes/* by name.
	Volumes []Volume
}

type Volume struct {
	Name  string
	Mount string
	FS    string
	Total uint64
	Free  uint64
	// Ejectable is true for a mount under /Volumes/; Eject accepts no other.
	Ejectable bool
}

type Network struct {
	BytesRecv uint64
	BytesSent uint64
	DownRate  float64
	UpRate    float64
}

type Battery struct {
	Present           bool
	Percent           int
	IsCharging        bool
	ExternalConnected bool
	TimeRemaining     time.Duration
	CycleCount        int
	Health            int
	Temperature       float64
	// Voltage is mV and Amperage mA, negative while discharging; Power is their product in W.
	Voltage   int64
	Amperage  int64
	Power     float64
	DesignMAh int
	MaxMAh    int
	// Adapter is nil on battery.
	Adapter *Adapter
	// UnpluggedAt is when the sampler saw the adapter go; zero on AC and when it started on battery.
	UnpluggedAt time.Time
	// LowPower is true while Low Power Mode is on.
	LowPower bool
}

type Adapter struct {
	Name  string
	Watts int
}

// GPU.Util is the Device utilization; Memory is in use, MemoryAlloc allocated.
type GPU struct {
	Present     bool
	Util        int
	Memory      uint64
	Model       string
	Renderer    int
	Tiler       int
	MemoryAlloc uint64
	// FreqMHz and Temp are 0 when unknown, like Cluster's.
	FreqMHz float64
	Temp    float64
}

// Power is in watts; a nil field is a source that could not be read, never 0 W.
type Power struct {
	SystemW  *float64
	AdapterW *float64
	CPUW     *float64
	GPUW     *float64
}

type Sensor struct {
	// Group is one of the sensorGroups names or "Other".
	Group string
	// Key is the four-character SMC key.
	Key  string
	Temp float64
}

type BluetoothDevice struct {
	Name string
	// Kind is the device type system_profiler reports ("Headphones"), "" when it gives none.
	Kind   string
	Levels []BatteryLevel
}

type BatteryLevel struct {
	// Part is "left", "right", "case" or "main".
	Part    string
	Percent int
}

type NetInfo struct {
	// Interfaces are up and have an address; the default route's one comes first.
	Interfaces []Interface
	// Router is "" without a default route.
	Router string
	DNS    []string
	// WiFi is nil when Wi-Fi is off or not associated.
	WiFi *native.WiFi
}

type Interface struct {
	Name    string
	MAC     string
	IPv4    []string
	IPv6    []string
	Primary bool
}

type Process struct {
	PID  int32
	PPID int32
	// Responsible is the Safari process a WebKit or Safari helper works for, which launchd
	// starts on Safari's behalf; 0 for every other process. Grouping follows it in place of PPID.
	Responsible int32
	// UID is the effective owner; 0 (root) when it could not be read.
	UID uint32
	// TTY is the controlling terminal, "" for a process without one.
	TTY  string
	Name string
	Exe  string
	// Kind is the role of a browser process (Chromium family, Safari, Firefox): "browser",
	// "tab", "extension", "gpu" or "utility"; "" for every other process.
	Kind    string
	CPU     float64
	CPUTime float64
	RSS     uint64
	// HasUsage is false for another user's process: its disk and energy counters need root.
	HasUsage      bool
	DiskReadRate  float64
	DiskWriteRate float64
	// EnergyMW is the average power over the scan window, not Activity Monitor's Energy Impact.
	EnergyMW float64
}

// App groups processes under the outermost .app bundle in their parent chain.
type App struct {
	Name string
	// BundlePath is the .app directory, "" for a process outside any bundle.
	BundlePath string
	PIDs       []int32
	CPU        float64
	RSS        uint64
	Processes  []Process
	// HasUsage is true when at least one process has it; the three sums cover those.
	HasUsage      bool
	DiskReadRate  float64
	DiskWriteRate float64
	EnergyMW      float64
}

// Agent is one AI coding agent session: a process named like an agent CLI plus its descendants.
type Agent struct {
	Name string
	// PID is the session's root process.
	PID int32
	// Dir is the root's working directory with the home directory as "~".
	Dir   string
	Since time.Time
	Procs int
	// CPU is percent of one core and RSS bytes, summed over the session.
	CPU float64
	RSS uint64
	// EnergyMW is valid only with HasUsage, like Process's.
	EnergyMW float64
	HasUsage bool
}

// SleepBlocker is one power assertion that holds off system or display sleep.
type SleepBlocker struct {
	PID int32
	// App is the owning process name as pmset prints it.
	App string
	// Kind is the assertion type, e.g. PreventUserIdleSystemSleep.
	Kind string
	Name string
}
