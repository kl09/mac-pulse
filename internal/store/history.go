package store

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const topApps = 10

var (
	ErrUnknownMetric = errors.New("unknown history metric")
	ErrUnknownRange  = errors.New("unknown history range")
)

var ranges = map[string]struct {
	points int
	step   time.Duration
	// hourly reads state.Hourly: the minutes end after 30 days.
	hourly bool
}{
	"1h":  {points: 60, step: time.Minute},
	"12h": {points: 144, step: 5 * time.Minute},
	"24h": {points: 144, step: 10 * time.Minute},
	"7d":  {points: 168, step: time.Hour},
	"30d": {points: 180, step: 4 * time.Hour},
	"90d": {points: 180, step: 12 * time.Hour, hourly: true},
	"1y":  {points: 183, step: 48 * time.Hour, hourly: true},
}

var metrics = map[string][]struct {
	name   string
	unit   string
	series int
	// zeroIsGap marks a series whose 0 means "the source was unavailable".
	// A minute that mixes readings and failures averages low; keep a per-series
	// sample count in minute if a power chart ever shows such dips.
	zeroIsGap bool
}{
	"cpu":     {{name: "total", unit: "percent", series: seriesCPU}},
	"memory":  {{name: "used", unit: "bytes", series: seriesMemory}},
	"network": {{name: "down", unit: "bytes_per_s", series: seriesNetDown}, {name: "up", unit: "bytes_per_s", series: seriesNetUp}},
	"disk":    {{name: "read", unit: "bytes_per_s", series: seriesDiskRead}, {name: "write", unit: "bytes_per_s", series: seriesDiskWrite}},
	"gpu":     {{name: "util", unit: "percent", series: seriesGPU}},
	"temp":    {{name: "cpu", unit: "celsius", series: seriesTemp}},
	"battery": {{name: "percent", unit: "percent", series: seriesBattery}},
	"power": {
		{name: "system", unit: "watts", series: seriesPower, zeroIsGap: true},
		{name: "cpu", unit: "watts", series: seriesCPUPower, zeroIsGap: true},
		{name: "gpu", unit: "watts", series: seriesGPUPower, zeroIsGap: true},
	},
}

// History is one metric over one range, downsampled to the range's fixed point count.
type History struct {
	Metric string
	Range  string
	// Start is the time of the first point; points are Step apart.
	Start  time.Time
	Step   time.Duration
	Series []Series
	// TopApps ranks at most 10 apps, highest first; only cpu, memory and network have it,
	// and only up to 30d: per-app totals are not kept longer.
	TopApps []AppValue
}

type Series struct {
	Name string
	// Unit is "percent", "bytes", "bytes_per_s", "celsius" or "watts".
	Unit string
	// Points are nil where mac-pulse was not running or, for watts, had no reading.
	Points []*float64
}

// AppValue is an app's average percent of one core for cpu, average RSS bytes for
// memory and total bytes for network.
type AppValue struct {
	Name       string
	BundlePath string
	Value      float64
}

// History validates metric and rng: both arrive from the web view.
func (s *Store) History(metric, rng string) (History, error) {
	series, ok := metrics[metric]
	if !ok {
		return History{}, fmt.Errorf("%w: %q", ErrUnknownMetric, metric)
	}
	r, ok := ranges[rng]
	if !ok {
		return History{}, fmt.Errorf("%w: %q", ErrUnknownRange, rng)
	}
	step := int64(r.step.Seconds())
	// Buckets sit on multiples of the step, so a point keeps its place between two queries.
	end := (s.now().Unix()/step + 1) * step
	start := end - int64(r.points)*step
	h := History{Metric: metric, Range: rng, Start: time.Unix(start, 0), Step: r.step}

	s.mu.Lock()
	defer s.mu.Unlock()
	// unit is the seconds one entry's At counts in.
	entries, unit := s.st.Minutes, int64(60)
	if r.hourly {
		entries, unit = s.st.Hourly, 3600
	}
	entries = entries[sort.Search(len(entries), func(i int) bool { return entries[i].At*unit >= start }):]
	for _, sr := range series {
		sums, counts := make([]float64, r.points), make([]int, r.points)
		for _, m := range entries {
			bucket := (m.At*unit - start) / step
			v := float64(m.V[sr.series])
			// A file with unsorted entries must not index outside the buckets, and a non-finite
			// value from an older file must not reach the JSON encoder.
			if bucket < 0 || bucket >= int64(r.points) || math.IsNaN(v) || math.IsInf(v, 0) || sr.zeroIsGap && v == 0 {
				continue
			}
			counts[bucket]++
			sums[bucket] += v
		}
		points := make([]*float64, r.points)
		for bucket, n := range counts {
			if n > 0 {
				avg := sums[bucket] / float64(n)
				points[bucket] = &avg
			}
		}
		h.Series = append(h.Series, Series{Name: sr.name, Unit: sr.unit, Points: points})
	}
	if !r.hourly {
		h.TopApps = s.st.topApps(metric, start, end)
	}
	return h, nil
}

// topApps ranks apps over the hours that overlap [start, end) in unix seconds.
// Whole hours, so the 1h range ranks over up to two hours; keep per-minute
// app buckets if that ever reads wrong.
func (st *state) topApps(metric string, start, end int64) []AppValue {
	if metric != "cpu" && metric != "memory" && metric != "network" {
		return nil
	}
	values := map[string]float64{}
	samples := 0
	for _, h := range st.Hours {
		if (h.At+1)*3600 <= start || h.At*3600 >= end {
			continue
		}
		samples += h.Samples
		for name, a := range h.Apps {
			switch metric {
			case "cpu":
				values[name] += a.CPU
			case "memory":
				values[name] += a.Memory
			default:
				values[name] += float64(a.Net)
			}
		}
	}
	var apps []AppValue
	for name, v := range values {
		// CPU and memory are sums over sampled seconds; network is already a total.
		if metric != "network" {
			v /= float64(max(samples, 1))
		}
		if v > 0 {
			apps = append(apps, AppValue{Name: name, BundlePath: st.Icons[name], Value: v})
		}
	}
	slices.SortFunc(apps, func(x, y AppValue) int {
		return cmp.Or(cmp.Compare(y.Value, x.Value), cmp.Compare(x.Name, y.Name))
	})
	return apps[:min(topApps, len(apps))]
}
