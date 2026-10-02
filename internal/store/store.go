// Package store keeps metric history, per-app rankings and the since-midnight counters,
// in memory and in history.gob.
package store

import (
	"bytes"
	"cmp"
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

const (
	fileName    = "history.gob"
	sparkPoints = 60
	// maxSampleGap is longer than any sampling interval: a wider gap is sleep, not a sample.
	maxSampleGap = 10 * time.Second
)

// Spark holds up to 60 points per series, oldest first, one per Add call.
type Spark struct {
	CPU       []float64
	Memory    []float64
	GPU       []float64
	Temp      []float64
	Power     []float64
	DiskRead  []float64
	DiskWrite []float64
	NetDown   []float64
	NetUp     []float64
}

// Today counts from local midnight and survives a restart.
type Today struct {
	DiskWritten uint64
	NetDown     uint64
	NetUp       uint64
	// NetApps is sorted by total traffic, busiest first.
	NetApps []AppTotal
}

type AppTotal struct {
	Name       string
	BundlePath string
	Down       uint64
	Up         uint64
}

// Store guards the history; any goroutine may call it. Nothing is written to disk
// until Flush.
type Store struct {
	path string
	now  func() time.Time
	// flushMu keeps two Flush calls from writing the same temp file.
	flushMu sync.Mutex

	mu    sync.Mutex
	st    state
	spark Spark
	// The previous readings of the cumulative counters, 0 until the first one.
	lastDiskWrite uint64
	lastNetRecv   uint64
	lastNetSent   uint64
	lastAdd       time.Time
}

// ErrUndecodable is OpenReadOnly's error for a history file it cannot read.
var ErrUndecodable = errors.New("history file does not decode")

// Open loads dir/history.gob. A missing file gives an empty store; a version 1 file is
// migrated and its original copied to history.gob.bak; a file that does not decode or
// has an unknown version is renamed to history.gob.bak and replaced. An earlier backup
// is never overwritten: the new one then carries the time in its name.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create history dir: %w", err)
	}
	return open(dir, false, time.Now)
}

// OpenReadOnly loads dir/history.gob for a reader beside the running app (-json, mcp): it
// creates, renames and writes nothing, since the file may belong to a binary of another
// version. A file that does not decode gives an empty store together with ErrUndecodable.
// The store must not be flushed.
func OpenReadOnly(dir string) (*Store, error) {
	return open(dir, true, time.Now)
}

func open(dir string, readOnly bool, now func() time.Time) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName), now: now}
	s.st = state{Version: version, Icons: map[string]string{}, NetApps: map[string]traffic{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	st, migrated, err := decodeState(raw)
	if err != nil && readOnly {
		return s, fmt.Errorf("%w: %w", ErrUndecodable, err)
	}
	if err != nil {
		slog.Warn("history file unreadable, starting empty", "path", s.path, "err", err)
		if err := os.Rename(s.path, s.backupPath()); err != nil {
			return nil, fmt.Errorf("set aside history: %w", err)
		}
		return s, nil
	}
	// The first Flush overwrites the old file; 30 days of history deserve one copy, and only
	// one however many starts end before that Flush.
	if old, _ := os.ReadFile(s.path + ".bak"); migrated && !readOnly && !bytes.Equal(old, raw) {
		if err := os.WriteFile(s.backupPath(), raw, 0o600); err != nil {
			return nil, fmt.Errorf("back up history before migration: %w", err)
		}
	}
	// gob leaves an empty map nil.
	if st.Icons == nil {
		st.Icons = map[string]string{}
	}
	if st.NetApps == nil {
		st.NetApps = map[string]traffic{}
	}
	for i := range st.Hours {
		if st.Hours[i].Apps == nil {
			st.Hours[i].Apps = map[string]appHour{}
		}
	}
	st.fillHourly()
	s.st = st
	return s, nil
}

// backupPath is history.gob.bak, or a name with the time in it when that one is taken.
func (s *Store) backupPath() string {
	if _, err := os.Stat(s.path + ".bak"); errors.Is(err, fs.ErrNotExist) {
		return s.path + ".bak"
	}
	return s.path + "." + s.now().Format("20060102-150405.000") + ".bak"
}

// Add records one sample: sparklines, the minute and hour averages, the today counters and the
// hour's per-app totals. The sample's own time is the clock.
func (s *Store) Add(snap *collector.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.rollover(snap.Time)
	s.st.DiskWritten += sinceLast(&s.lastDiskWrite, snap.Disk.WriteBytes)
	s.st.NetDown += sinceLast(&s.lastNetRecv, snap.Network.BytesRecv)
	s.st.NetUp += sinceLast(&s.lastNetSent, snap.Network.BytesSent)

	var memory float64
	if snap.Memory.Total > 0 {
		memory = float64(snap.Memory.Used) / float64(snap.Memory.Total) * 100
	}
	// Without the SMC only a battery knows the draw, and only its own.
	var system, chip, gpu float64
	power := math.Abs(snap.Battery.Power)
	if snap.Power.SystemW != nil {
		system, power = *snap.Power.SystemW, *snap.Power.SystemW
	}
	if snap.Power.CPUW != nil {
		chip = *snap.Power.CPUW
	}
	if snap.Power.GPUW != nil {
		gpu = *snap.Power.GPUW
	}
	for series, v := range map[*[]float64]float64{
		&s.spark.CPU:       snap.CPU.Total,
		&s.spark.Memory:    memory,
		&s.spark.GPU:       float64(snap.GPU.Util),
		&s.spark.Temp:      snap.CPUTemp,
		&s.spark.Power:     power,
		&s.spark.DiskRead:  snap.Disk.ReadRate,
		&s.spark.DiskWrite: snap.Disk.WriteRate,
		&s.spark.NetDown:   snap.Network.DownRate,
		&s.spark.NetUp:     snap.Network.UpRate,
	} {
		// The sparkline keeps its tick spacing, so a non-finite reading plots as 0.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		*series = append(*series, v)
		*series = (*series)[max(len(*series)-sparkPoints, 0):]
	}

	s.st.addSample(snap.Time, [seriesCount]float32{
		seriesCPU:       float32(snap.CPU.Total),
		seriesMemory:    float32(snap.Memory.Used),
		seriesNetDown:   float32(snap.Network.DownRate),
		seriesNetUp:     float32(snap.Network.UpRate),
		seriesDiskRead:  float32(snap.Disk.ReadRate),
		seriesDiskWrite: float32(snap.Disk.WriteRate),
		seriesGPU:       float32(snap.GPU.Util),
		seriesTemp:      float32(snap.CPUTemp),
		seriesBattery:   float32(snap.Battery.Percent),
		seriesPower:     float32(system),
		seriesCPUPower:  float32(chip),
		seriesGPUPower:  float32(gpu),
	})

	seconds := 1
	if gap := snap.Time.Sub(s.lastAdd); gap > time.Second && gap <= maxSampleGap {
		seconds = int(gap.Round(time.Second) / time.Second)
	}
	s.lastAdd = snap.Time
	h := s.st.openHour(snap.Time)
	h.Samples += seconds
	for _, app := range snap.Apps {
		a := h.Apps[app.Name]
		a.CPU += app.CPU * float64(seconds)
		a.Memory += float64(app.RSS) * float64(seconds)
		h.Apps[app.Name] = a
		if app.BundlePath != "" {
			s.st.Icons[app.Name] = app.BundlePath
		}
	}
}

// AddUsage records the traffic apps moved since the previous call.
func (s *Store) AddUsage(usage []netinspect.AppUsage) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.rollover(now)
	h := s.st.openHour(now)
	for _, u := range usage {
		t := s.st.NetApps[u.App]
		s.st.NetApps[u.App] = traffic{Down: t.Down + u.Down, Up: t.Up + u.Up}
		a := h.Apps[u.App]
		a.Net += u.Down + u.Up
		h.Apps[u.App] = a
		if u.BundlePath != "" {
			s.st.Icons[u.App] = u.BundlePath
		}
	}
}

func (s *Store) Spark() Spark {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Spark{
		CPU:       slices.Clone(s.spark.CPU),
		Memory:    slices.Clone(s.spark.Memory),
		GPU:       slices.Clone(s.spark.GPU),
		Temp:      slices.Clone(s.spark.Temp),
		Power:     slices.Clone(s.spark.Power),
		DiskRead:  slices.Clone(s.spark.DiskRead),
		DiskWrite: slices.Clone(s.spark.DiskWrite),
		NetDown:   slices.Clone(s.spark.NetDown),
		NetUp:     slices.Clone(s.spark.NetUp),
	}
}

func (s *Store) Today() Today {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Without this a store reopened the next morning would show yesterday until the first Add.
	s.st.rollover(now)
	today := Today{DiskWritten: s.st.DiskWritten, NetDown: s.st.NetDown, NetUp: s.st.NetUp}
	for name, t := range s.st.NetApps {
		today.NetApps = append(today.NetApps, AppTotal{Name: name, BundlePath: s.st.Icons[name], Down: t.Down, Up: t.Up})
	}
	slices.SortFunc(today.NetApps, func(x, y AppTotal) int {
		return cmp.Or(cmp.Compare(y.Down+y.Up, x.Down+x.Up), cmp.Compare(x.Name, y.Name))
	})
	return today
}

// NetTotals is the traffic of the last 7 and 30 days, today included.
type NetTotals struct {
	Down7d, Up7d, Down30d, Up30d uint64
}

// NetTotals sums the minute averages the network history is drawn from.
// A minute counts as 60 s at its average rate, so the minutes mac-pulse started
// or stopped in are overcounted; keep per-day byte counters if the totals must be exact.
func (s *Store) NetTotals() NetTotals {
	week := s.now().Unix()/60 - 7*24*60
	s.mu.Lock()
	defer s.mu.Unlock()
	var down7, up7, down30, up30 float64
	for _, m := range s.st.Minutes {
		down, up := float64(m.V[seriesNetDown])*60, float64(m.V[seriesNetUp])*60
		down30, up30 = down30+down, up30+up
		if m.At > week {
			down7, up7 = down7+down, up7+up
		}
	}
	return NetTotals{Down7d: uint64(down7), Up7d: uint64(up7), Down30d: uint64(down30), Up30d: uint64(up30)}
}

// Alerts returns the closed alerts the file held, or the ones of the last SetAlerts.
func (s *Store) Alerts() []alerts.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.st.Alerts)
}

// SetAlerts replaces the closed alerts the next Flush writes.
func (s *Store) SetAlerts(records []alerts.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Alerts = records
}

func (s *Store) Flush() error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	var buf bytes.Buffer
	s.mu.Lock()
	err := gob.NewEncoder(&buf).Encode(&s.st)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	// Write, sync, rename: a crash or power cut must not cost 30 days of history.
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create history temp file: %w", err)
	}
	_, err = f.Write(buf.Bytes())
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace history: %w", err)
	}
	return nil
}

// sinceLast returns how far a cumulative counter moved and remembers cur. A zero
// reading is a failed collector and a smaller one a counter reset; neither counts.
// The network counter is a sum over interfaces, so one that reappears with
// its old count is added again; diff per interface in the collector if totals ever jump.
func sinceLast(last *uint64, cur uint64) uint64 {
	if cur == 0 {
		return 0
	}
	var moved uint64
	if *last != 0 && cur > *last {
		moved = cur - *last
	}
	*last = cur
	return moved
}
