package alerts

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
)

const (
	// seriesStep is twice the slowest sampling interval, so a bucket without a sample is a
	// time mac-pulse did not sample, not a gap between two samples.
	seriesStep = 10
	// maxPoints bounds the samples one alert keeps, 1 KB of float32: past it the buckets merge
	// in pairs and the step doubles, so an alert that lasts for days still fits.
	maxPoints = 240
	// preRollPoints is ten minutes of a system metric kept before any alert, to show what led to one.
	preRollPoints = 60
	contextRows   = 5
	// maxFacts bounds the facts of a stored alert: the longest list facts builds has six.
	maxFacts = 8
)

const (
	UnitPercent = "percent"
	UnitBytes   = "bytes"
	UnitCelsius = "celsius"
	// The units below only appear in a Context.
	UnitBytesPerS  = "bytes_per_s"
	UnitMilliwatts = "milliwatts"
	UnitWatts      = "watts"
	UnitSeconds    = "seconds"
	// UnitState is the macOS thermal state, 0 (nominal) to 3 (critical).
	UnitState = "state"
)

// systemKinds are the rules without an app or a device: their metrics are sampled all the
// time, so an alert of one starts with the minutes before it.
var systemKinds = []string{KindCPU, KindTemp, KindThermal, KindDisk, KindMemory, KindMemoryUsed, KindSwap, KindBatteryLow}

var kinds = append(slices.Clone(systemKinds), KindAppCPU, KindAppMemory, KindAppRule, KindBTBattery)

// Record is an alert together with what was recorded about it.
type Record struct {
	Alert
	// Detail is nil for an alert stored by a build that recorded none.
	Detail *Detail
}

// Detail is why an alert fired and what its metric did while it lasted.
type Detail struct {
	// Unit is UnitPercent, UnitBytes or UnitCelsius: the unit of every value here.
	Unit string
	// Limit is 0 for a rule without a number of its own (memory pressure, thermal state).
	// Below says the rule holds under the limit; Peak is then the lowest value.
	Limit float64
	Below bool
	// Hold is how long the rule had to hold before the alert fired, which it did at FiredAt.
	Hold    time.Duration
	FiredAt time.Time
	// Fired is the value at FiredAt and Last the newest one. Peak, Sum and N cover the samples
	// in which the rule held, from Since to Until.
	Fired   float64
	Peak    float64
	Last    float64
	Sum     float64
	N       int
	Series  Series
	Context Context
}

// Series is the samples of one metric in buckets Step seconds apart, the first at Start in
// unix seconds; a bucket nothing was sampled in is NaN.
type Series struct {
	Start int64
	Step  int64
	V     []float32
	// last counts the samples averaged into the newest bucket.
	last int
}

// Context is what the Mac was doing at the moment an alert fired, captured once.
type Context struct {
	// BundlePath is the .app of the alert's app, PIDs the number of its processes and Procs
	// the heaviest of them by the alert's metric.
	BundlePath string
	PIDs       int
	Procs      []Row
	// Top is the heaviest apps by the metric the alert is about, in TopUnit.
	Top     []Row
	TopUnit string
	Facts   []Fact
}

type Row struct {
	Name       string
	BundlePath string
	// PID is 0 for an app.
	PID   int32
	Value float64
}

// Fact is one reading next to the alert's own metric; Key names it for the page.
type Fact struct {
	Key   string
	Value float64
	Unit  string
}

// observe records one sample of the alert's metric. held says the rule holds in it: only
// those samples count into the peak and the average.
func (d *Detail) observe(at time.Time, v float64, held bool) {
	d.Series.add(at, v)
	d.Last = v
	if !held {
		return
	}
	if d.N == 0 || (d.Below && v < d.Peak) || (!d.Below && v > d.Peak) {
		d.Peak = v
	}
	d.Sum += v
	d.N++
}

// add averages v into the bucket of at. A sample from before the newest bucket, which a
// backward clock step makes, is dropped.
func (s *Series) add(at time.Time, v float64) {
	if len(s.V) == 0 {
		s.Start, s.Step = at.Unix()/seriesStep*seriesStep, seriesStep
	}
	if at.Unix() < s.Start {
		return
	}
	i := int((at.Unix() - s.Start) / s.Step)
	for i >= maxPoints {
		s.halve()
		i = int((at.Unix() - s.Start) / s.Step)
	}
	switch {
	case i < len(s.V)-1:
	case i == len(s.V)-1:
		s.last++
		s.V[i] += (float32(v) - s.V[i]) / float32(s.last)
	default:
		for len(s.V) < i {
			s.V = append(s.V, float32(math.NaN()))
		}
		s.V, s.last = append(s.V, float32(v)), 1
	}
}

// halve merges the buckets in pairs.
// A merged bucket is the mean of its halves, not of their samples; keep a count
// per bucket if a chart ever needs the exact mean.
func (s *Series) halve() {
	half := s.V[:0]
	for i := 0; i < len(s.V); i += 2 {
		v := s.V[i]
		if i+1 < len(s.V) {
			switch other := s.V[i+1]; {
			case math.IsNaN(float64(v)):
				v = other
			case !math.IsNaN(float64(other)):
				v = (v + other) / 2
			}
		}
		half = append(half, v)
	}
	s.V, s.Step, s.last = half, s.Step*2, 1
}

// slide is add for a pre-roll: it keeps the newest preRollPoints buckets and starts over
// after a longer gap or a backward clock step, so it never grows coarser.
func (s *Series) slide(at time.Time, v float64) {
	if end := s.Start + int64(len(s.V))*s.Step; at.Unix() < s.Start || at.Unix()-end > preRollPoints*seriesStep {
		*s = Series{}
	}
	s.add(at, v)
	if extra := len(s.V) - preRollPoints; extra > 0 {
		s.V = s.V[extra:]
		s.Start += int64(extra) * s.Step
	}
}

func (s Series) clone() Series {
	s.V = slices.Clone(s.V)
	return s
}

// clone copies what the engine goes on writing: the Context never changes once captured.
func (d *Detail) clone() *Detail {
	if d == nil {
		return nil
	}
	out := *d
	out.Series = d.Series.clone()
	return &out
}

// usesMemory says the rule of s judges an app's memory, not its CPU.
func usesMemory(s subject) bool {
	return s.kind == KindAppMemory || s.metric == "memory"
}

func unitOf(s subject) string {
	switch {
	case s.kind == KindTemp, s.kind == KindThermal:
		return UnitCelsius
	case s.kind == KindSwap, usesMemory(s):
		return UnitBytes
	}
	return UnitPercent
}

// value is the metric the rule of s is about, in unitOf(s); ok is false when the sample has
// no reading of it: no sensor, a failed collector, an app that quit. A thermal alert goes by
// the CPU temperature: the state macOS reports is four steps, not a curve.
func value(snap *collector.Snapshot, s subject) (v float64, ok bool) {
	switch s.kind {
	case KindCPU:
		return snap.CPU.Total, true
	case KindTemp, KindThermal:
		return snap.CPUTemp, snap.CPUTemp > 0
	case KindDisk:
		if snap.Disk.Total == 0 {
			return 0, false
		}
		return float64(snap.Disk.Free) / float64(snap.Disk.Total) * 100, true
	case KindMemory, KindMemoryUsed:
		if snap.Memory.Total == 0 {
			return 0, false
		}
		return float64(snap.Memory.Used) / float64(snap.Memory.Total) * 100, true
	case KindSwap:
		return float64(snap.Memory.SwapUsed), true
	case KindBatteryLow:
		return float64(snap.Battery.Percent), snap.Battery.Present
	case KindBTBattery:
		i := slices.IndexFunc(snap.Bluetooth, func(d collector.BluetoothDevice) bool { return d.Name == s.app })
		if i < 0 || len(snap.Bluetooth[i].Levels) == 0 {
			return 0, false
		}
		lowest := slices.MinFunc(snap.Bluetooth[i].Levels, func(a, b collector.BatteryLevel) int { return cmp.Compare(a.Percent, b.Percent) })
		return float64(lowest.Percent), true
	}
	i := slices.IndexFunc(snap.Apps, func(a collector.App) bool { return a.Name == s.app })
	switch {
	case i < 0:
		return 0, false
	case usesMemory(s):
		return float64(snap.Apps[i].RSS), true
	}
	return snap.Apps[i].CPU, true
}

// capture takes the Context of an alert that fires on snap.
func capture(snap *collector.Snapshot, s subject) Context {
	c := Context{TopUnit: UnitPercent, Facts: facts(snap, s)}
	rank := func(a collector.App) float64 { return a.CPU }
	switch {
	case s.kind == KindBTBattery:
		return c
	case s.kind == KindMemory, s.kind == KindMemoryUsed, s.kind == KindSwap, usesMemory(s):
		c.TopUnit, rank = UnitBytes, func(a collector.App) float64 { return float64(a.RSS) }
	// The disk and the battery have no per-app share; who writes and who draws power is the nearest thing.
	case s.kind == KindDisk:
		c.TopUnit, rank = UnitBytesPerS, func(a collector.App) float64 { return a.DiskWriteRate }
	case s.kind == KindBatteryLow:
		c.TopUnit, rank = UnitMilliwatts, func(a collector.App) float64 { return a.EnergyMW }
	}
	for _, a := range snap.Apps {
		if rank(a) > 0 {
			c.Top = append(c.Top, Row{Name: a.Name, BundlePath: a.BundlePath, Value: rank(a)})
		}
		if s.app != a.Name {
			continue
		}
		c.BundlePath, c.PIDs = a.BundlePath, len(a.PIDs)
		for _, p := range a.Processes {
			v := p.CPU
			if usesMemory(s) {
				v = float64(p.RSS)
			}
			c.Procs = append(c.Procs, Row{Name: p.Name, PID: p.PID, Value: v})
		}
	}
	c.Top, c.Procs = heaviest(c.Top), heaviest(c.Procs)
	return c
}

// heaviest keeps the contextRows largest rows, largest first.
func heaviest(rows []Row) []Row {
	slices.SortStableFunc(rows, func(a, b Row) int { return cmp.Compare(b.Value, a.Value) })
	return slices.Clip(rows[:min(len(rows), contextRows)])
}

// facts are the readings a Context carries next to its lists, by the kind of the alert.
func facts(snap *collector.Snapshot, s subject) []Fact {
	switch s.kind {
	case KindCPU:
		return []Fact{{Key: "cpu_user", Value: snap.CPU.User, Unit: UnitPercent}, {Key: "cpu_system", Value: snap.CPU.System, Unit: UnitPercent}}
	case KindTemp, KindThermal:
		out := []Fact{{Key: "thermal", Value: float64(snap.Thermal), Unit: UnitState}}
		if snap.CPUTemp > 0 {
			out = append(out, Fact{Key: "cpu_temp", Value: snap.CPUTemp, Unit: UnitCelsius})
		}
		return out
	case KindMemory, KindMemoryUsed, KindSwap:
		m := snap.Memory
		return []Fact{
			{Key: "mem_app", Value: float64(m.App), Unit: UnitBytes},
			{Key: "mem_wired", Value: float64(m.Wired), Unit: UnitBytes},
			{Key: "mem_compressed", Value: float64(m.Compressed), Unit: UnitBytes},
			{Key: "mem_cached", Value: float64(m.Cached), Unit: UnitBytes},
			{Key: "mem_free", Value: float64(m.Free), Unit: UnitBytes},
			{Key: "mem_swap", Value: float64(m.SwapUsed), Unit: UnitBytes},
		}
	case KindDisk:
		return []Fact{
			{Key: "disk_free", Value: float64(snap.Disk.Free), Unit: UnitBytes},
			{Key: "disk_total", Value: float64(snap.Disk.Total), Unit: UnitBytes},
			{Key: "disk_write", Value: snap.Disk.WriteRate, Unit: UnitBytesPerS},
		}
	case KindBatteryLow:
		b := snap.Battery
		out := []Fact{
			{Key: "bat_level", Value: float64(b.Percent), Unit: UnitPercent},
			{Key: "bat_draw", Value: math.Abs(b.Power), Unit: UnitWatts},
		}
		if b.TimeRemaining > 0 {
			out = append(out, Fact{Key: "bat_left", Value: b.TimeRemaining.Seconds(), Unit: UnitSeconds})
		}
		return out
	case KindBTBattery:
		var out []Fact
		for _, d := range snap.Bluetooth {
			if d.Name != s.app {
				continue
			}
			for _, l := range d.Levels[:min(len(d.Levels), contextRows)] {
				out = append(out, Fact{Key: "bt_" + l.Part, Value: float64(l.Percent), Unit: UnitPercent})
			}
		}
		return out
	}
	return nil
}
