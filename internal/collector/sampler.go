package collector

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/kl09/mac-pulse/internal/native"
)

const (
	execTimeout = 3 * time.Second
	// The first sample, and the one after a pace change, do not wait out a whole interval:
	// the menu bar is empty until the first lands, and a panel that just opened wants fresh numbers.
	promptSample = time.Second
	// The ~400-process scan is the costliest collector (~20 ms CPU), so it
	// runs every 3rd tick and per-process CPU% averages over that window. Memory,
	// temperature and battery refresh on the same tick.
	processEvery = 3
	// pmset is ~15 ms CPU and sleep assertions change rarely, so they refresh every
	// 5th process scan (30 s at a 2 s interval).
	blockersEvery = 5 * processEvery
	// system_profiler is ~80 ms CPU and a battery level moves by a percent in minutes.
	bluetoothEvery = 30
	// Without Detail.Bluetooth the levels only feed the low-battery alert.
	bluetoothIdle = 5 * time.Minute
	// top is ~0.3 s CPU with its launch, the costliest read of all; kernel_task's share is
	// an average over the minute between two reads.
	kernelEvery = 2 * blockersEvery
	// diskutil is ~40 ms CPU and a SMART verdict changes once in a disk's life.
	smartEvery = 300
)

// Detail switches on the collectors only a detail screen shows; each costs too much to run
// behind a closed panel. A flag that goes on is served by the next sample.
type Detail struct {
	// Temps is every SMC temperature sensor, read on process-scan ticks.
	Temps bool
	// Bluetooth is the battery of connected devices, read every bluetoothEvery ticks instead
	// of every bluetoothIdle.
	Bluetooth bool
	// NetInfo is the Wi-Fi link on process-scan ticks; interfaces, router and DNS every blockersEvery ticks.
	NetInfo bool
	// SMART is the disk model and health, read every smartEvery ticks; the last answer stays in every snapshot.
	SMART bool
	// Dev is the agent sessions, summed from the last process scan.
	Dev bool
}

// Sampler owns the previous snapshot; rates are deltas against it. Only its Run
// goroutine touches its state; SetInterval and SetDetail reach it through channels.
type Sampler struct {
	interval   time.Duration
	intervals  chan time.Duration
	details    chan Detail
	detail     Detail
	onSample   func(*Snapshot)
	prev       *Snapshot
	prevCPU    cpu.TimesStat
	ticks      int
	prevProcAt time.Time
	// kernel is pid 0 as top last reported it; its CPU is the delta between two top reads.
	kernel   Process
	kernelAt time.Time

	model     string
	clusters  []Cluster
	prevUsage map[int32]native.Usage
	// The tick each detail collector last ran on; 0 is "not since its flag went on".
	tempsAt     int
	bluetoothAt int
	netInfoAt   int
	smartAt     int
	diskModel   string
	smart       string
	// bluetoothRead is the time of the last system_profiler call, whichever period asked for it.
	bluetoothRead time.Time
	// tags and agents cache what a pid never changes; both are rebuilt from the live pids.
	tags   map[int32]browserTag
	agents map[int32]agentMeta
}

func NewSampler(interval time.Duration, onSample func(*Snapshot)) *Sampler {
	return &Sampler{
		interval: interval, intervals: make(chan time.Duration, 1), details: make(chan Detail, 1), onSample: onSample,
	}
}

// SetInterval changes the pace and brings the next sample forward; it must not be called
// from two goroutines at once.
func (s *Sampler) SetInterval(interval time.Duration) {
	select {
	case <-s.intervals:
	default:
	}
	s.intervals <- interval
}

// SetDetail replaces the detail flags and brings the next sample forward; like SetInterval,
// it must not be called from two goroutines at once.
func (s *Sampler) SetDetail(d Detail) {
	select {
	case <-s.details:
	default:
	}
	s.details <- d
}

func (s *Sampler) Run(ctx context.Context) {
	// Without a baseline the first sample would show the average since boot.
	if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) > 0 {
		s.prevCPU = times[0]
	}
	s.model, s.clusters = readTopology()
	timer := time.NewTimer(min(s.interval, promptSample))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case s.interval = <-s.intervals:
			timer.Reset(min(s.interval, promptSample))
		case s.detail = <-s.details:
			timer.Reset(min(s.interval, promptSample))
		case <-timer.C:
			snap := s.collect(ctx)
			// A result that arrives during shutdown is dropped: its consumer is already gone.
			if ctx.Err() != nil {
				return
			}
			s.onSample(snap)
			timer.Reset(s.interval)
		}
	}
}

func (s *Sampler) collect(ctx context.Context) *Snapshot {
	// Round(0) strips the monotonic clock, which stops during sleep: a rate over the wall
	// clock spreads the bytes of a sleep across its duration instead of one huge spike.
	snap := &Snapshot{Time: time.Now().Round(0), HasRates: s.prev != nil, Thermal: native.ThermalState()}
	// 0.2 ms CPU a read, so the microphone and camera state is as fresh as the sample.
	snap.Media.Mic, snap.Media.MicPIDs, snap.Media.MicBundles, snap.Media.Camera = native.MediaUse()
	var err error
	if snap.CPU, s.prevCPU, err = readCPU(ctx, s.prevCPU); err != nil {
		slog.Debug("collect cpu", "err", err)
	}
	if snap.Disk, err = readDisk(ctx); err != nil {
		slog.Debug("collect disk", "err", err)
	}
	if snap.Network, err = readNetwork(ctx); err != nil {
		slog.Debug("collect network", "err", err)
	}
	if g, err := native.GPU(); err == nil {
		snap.GPU = GPU{
			Present: true, Model: g.Model, Util: g.Util, Renderer: g.Renderer, Tiler: g.Tiler,
			Memory: g.Memory, MemoryAlloc: g.MemoryAlloc,
		}
	}
	var chip native.Power
	snap.Power, chip = readPower()
	snap.GPU.FreqMHz = chip.GPUMHz
	snap.CPU.Model, snap.CPU.Clusters = s.model, slices.Clone(s.clusters)
	for i, c := range snap.CPU.Clusters {
		snap.CPU.Clusters[i].FreqMHz = map[string]float64{"Efficiency": chip.EMHz, "Performance": chip.PMHz}[c.Name]
	}
	if s.prev != nil {
		dt := snap.Time.Sub(s.prev.Time).Seconds()
		snap.Disk.ReadRate = perSecond(snap.Disk.ReadBytes, s.prev.Disk.ReadBytes, dt)
		snap.Disk.WriteRate = perSecond(snap.Disk.WriteBytes, s.prev.Disk.WriteBytes, dt)
		snap.Network.DownRate = perSecond(snap.Network.BytesRecv, s.prev.Network.BytesRecv, dt)
		snap.Network.UpRate = perSecond(snap.Network.BytesSent, s.prev.Network.BytesSent, dt)
	}
	s.ticks++
	if s.ticks%processEvery == 1 {
		s.collectSlow(ctx, snap)
	} else {
		snap.Memory, snap.CPUTemp, snap.Battery, snap.Fans = s.prev.Memory, s.prev.CPUTemp, s.prev.Battery, s.prev.Fans
		snap.Processes, snap.Apps, snap.SleepBlockers = s.prev.Processes, s.prev.Apps, s.prev.SleepBlockers
		snap.HasProcRates, snap.Disk.Volumes = s.prev.HasProcRates, s.prev.Disk.Volumes
	}
	s.collectDetail(ctx, snap)
	s.prev = snap
	return snap
}

// collectSlow fills what moves slowly or costs too much for every tick: the SMC read
// is ~15 ms CPU and the vm_stat launch ~9 ms.
func (s *Sampler) collectSlow(ctx context.Context, snap *Snapshot) {
	var err error
	var dt float64
	if !s.prevProcAt.IsZero() {
		dt = snap.Time.Sub(s.prevProcAt).Seconds()
	}
	if snap.Memory, err = readMemory(ctx); err != nil {
		slog.Debug("collect memory", "err", err)
	}
	// A previous read without counters is a failed vm_stat, not a boot-to-now delta.
	if s.prev != nil && s.prev.Memory.PageIns > 0 {
		m, was := &snap.Memory, s.prev.Memory
		m.PageInRate, m.PageOutRate = perSecond(m.PageIns, was.PageIns, dt), perSecond(m.PageOuts, was.PageOuts, dt)
		m.SwapInRate, m.SwapOutRate = perSecond(m.SwapIns, was.SwapIns, dt), perSecond(m.SwapOuts, was.SwapOuts, dt)
		m.CompressRate = perSecond(m.Compressions, was.Compressions, dt)
		m.DecompressRate = perSecond(m.Decompressions, was.Decompressions, dt)
	}
	if snap.CPUTemp, err = readCPUTemp(ctx); err != nil {
		slog.Debug("collect cpu temp", "err", err)
	}
	if snap.Battery, err = readBattery(ctx); err != nil {
		slog.Debug("collect battery", "err", err)
	}
	snap.Battery.LowPower = native.LowPowerMode()
	if s.prev != nil && !snap.Battery.ExternalConnected {
		snap.Battery.UnpluggedAt = s.prev.Battery.UnpluggedAt
		if s.prev.Battery.ExternalConnected {
			snap.Battery.UnpluggedAt = snap.Time
		}
	}
	if snap.Fans, err = native.Fans(); err != nil && !errors.Is(err, native.ErrUnavailable) {
		slog.Debug("collect fans", "err", err)
	}
	if s.ticks%blockersEvery != 1 {
		snap.SleepBlockers, snap.Disk.Volumes = s.prev.SleepBlockers, s.prev.Disk.Volumes
	} else {
		s.collectSlowest(ctx, snap)
	}
	s.collectProcesses(ctx, snap, dt)
	s.prevProcAt = snap.Time
}

// collectProcesses scans the processes; dt is the time since the previous scan, 0 on the first.
func (s *Sampler) collectProcesses(ctx context.Context, snap *Snapshot, dt float64) {
	prevTimes := map[int32]float64{}
	if s.prev != nil {
		for _, p := range s.prev.Processes {
			prevTimes[p.PID] = p.CPUTime
		}
	}
	snap.HasProcRates = dt > 0
	var err error
	if snap.Processes, err = readProcesses(ctx, prevTimes, dt); err != nil {
		slog.Debug("collect processes", "err", err)
	}
	// Another user's pid answers EPERM without root, so only our own are asked.
	uid, usage := uint32(os.Getuid()), make(map[int32]native.Usage, len(snap.Processes))
	for i := range snap.Processes {
		p := &snap.Processes[i]
		if p.UID != uid {
			continue
		}
		cur, err := native.ProcUsage(p.PID)
		if err != nil {
			continue
		}
		usage[p.PID], p.HasUsage = cur, true
		if prev, ok := s.prevUsage[p.PID]; ok {
			p.DiskReadRate, p.DiskWriteRate, p.EnergyMW = usageRates(prev, cur, dt)
		}
	}
	s.prevUsage = usage
	s.tagBrowsers(ctx, snap.Processes)
	if !s.kernelAt.IsZero() {
		snap.Processes = append(snap.Processes, s.kernel)
	}
	snap.Apps = groupByApp(snap.Processes)
}

// collectSlowest runs every blockersEvery ticks: pmset is ~15 ms CPU.
func (s *Sampler) collectSlowest(ctx context.Context, snap *Snapshot) {
	var err error
	if snap.SleepBlockers, err = readSleepBlockers(ctx); err != nil {
		slog.Debug("collect sleep blockers", "err", err)
	}
	if snap.Disk.Volumes, err = readVolumes(ctx); err != nil {
		slog.Debug("collect volumes", "err", err)
	}
	if s.ticks%kernelEvery != 1 {
		return
	}
	k, err := readKernelTask(ctx)
	if err != nil {
		slog.Debug("collect kernel_task", "err", err)
		return
	}
	if !s.kernelAt.IsZero() && k.CPUTime >= s.kernel.CPUTime {
		k.CPU = (k.CPUTime - s.kernel.CPUTime) / snap.Time.Sub(s.kernelAt).Seconds() * 100
	}
	s.kernel, s.kernelAt = k, snap.Time
}

// collectDetail runs the collectors of the open detail screen: at once when a flag has
// just gone on, then at the collector's own period. Between runs the previous reading is
// carried over; a flag that is off leaves its fields empty. Bluetooth alone is also read
// with its flag off, every bluetoothIdle.
func (s *Sampler) collectDetail(ctx context.Context, snap *Snapshot) {
	var err error
	s.collectTemps(snap)
	s.collectNetInfo(ctx, snap)
	s.collectAgents(ctx, snap)
	// The first sample feeds the status item and must not wait for system_profiler, so behind
	// a closed panel the first read comes with the second sample and then every bluetoothIdle.
	idle := s.prev != nil && (s.bluetoothRead.IsZero() || snap.Time.Sub(s.bluetoothRead) >= bluetoothIdle)
	switch {
	case s.due(s.detail.Bluetooth, &s.bluetoothAt, bluetoothEvery) || idle:
		s.bluetoothRead = snap.Time
		if snap.Bluetooth, err = readBluetooth(ctx); err != nil {
			slog.Debug("collect bluetooth", "err", err)
		}
	case s.prev != nil:
		snap.Bluetooth = s.prev.Bluetooth
	}
	if s.due(s.detail.SMART, &s.smartAt, smartEvery) {
		if s.diskModel, s.smart, err = readSMART(ctx); err != nil {
			slog.Debug("collect smart", "err", err)
		}
	}
	snap.Disk.Model, snap.Disk.SMART = s.diskModel, s.smart
}

// due says whether a detail collector runs on this tick and stamps *at when it does;
// a flag that is off forgets the stamp, so the next switch-on reads at once.
func (s *Sampler) due(on bool, at *int, every int) bool {
	if !on {
		*at = 0
		return false
	}
	if *at != 0 && s.ticks%every != 1 {
		return false
	}
	*at = s.ticks
	return true
}

func (s *Sampler) collectTemps(snap *Snapshot) {
	switch {
	case s.due(s.detail.Temps, &s.tempsAt, processEvery):
		var err error
		if snap.Temps, err = readTemps(); err != nil && !errors.Is(err, native.ErrUnavailable) {
			slog.Debug("collect temps", "err", err)
		}
	case s.detail.Temps && s.prev != nil:
		snap.Temps = s.prev.Temps
	}
	clusterOf := map[string]string{groupEfficiency: "Efficiency", groupPerformance: "Performance"}
	for _, t := range snap.Temps {
		if t.Group == groupGPU {
			snap.GPU.Temp = max(snap.GPU.Temp, t.Temp)
		}
		for i, c := range snap.CPU.Clusters {
			if c.Name == clusterOf[t.Group] {
				snap.CPU.Clusters[i].Temp = max(c.Temp, t.Temp)
			}
		}
	}
}

// collectNetInfo refreshes the Wi-Fi link on process-scan ticks; a signal moves faster
// than an address.
func (s *Sampler) collectNetInfo(ctx context.Context, snap *Snapshot) {
	switch {
	case s.due(s.detail.NetInfo, &s.netInfoAt, blockersEvery):
		var err error
		if snap.NetInfo, err = readNetInfo(ctx); err != nil {
			slog.Debug("collect net info", "err", err)
		}
	case s.detail.NetInfo && s.prev != nil && s.prev.NetInfo != nil:
		info := *s.prev.NetInfo
		if s.ticks%processEvery == 1 {
			info.WiFi = nil
			if wifi, err := native.WiFiInfo(); err == nil {
				info.WiFi = &wifi
			}
		}
		snap.NetInfo = &info
	}
}

func perSecond(cur, prev uint64, dt float64) float64 {
	if cur < prev || dt <= 0 {
		return 0
	}
	return float64(cur-prev) / dt
}
