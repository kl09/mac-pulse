package store

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/kl09/mac-pulse/internal/alerts"
)

const (
	// version changes whenever state or a type inside it changes shape; Open migrates the
	// versions it knows and sets any other file aside.
	version       = 2
	retainMinutes = 30 * 24 * 60
	retainHours   = 30 * 24
	// retainHourly is a leap year of hourly averages: ~0.5 MB.
	retainHourly   = 366 * 24
	topAppsPerHour = 10
)

// Indexes into minute.V.
const (
	seriesCPU = iota
	seriesMemory
	seriesNetDown
	seriesNetUp
	seriesDiskRead
	seriesDiskWrite
	seriesGPU
	seriesTemp
	seriesBattery
	// The power series are watts; 0 is "no reading", which History turns into a gap.
	seriesPower
	seriesCPUPower
	seriesGPUPower
	seriesCount
)

// state is everything that survives a restart; history.gob is its gob encoding.
type state struct {
	Version int
	// Minutes holds one entry per minute that had samples, oldest first; a missing
	// minute is a time mac-pulse was not running.
	Minutes []minute
	// Hourly is Minutes at a coarser grain kept for a year: At is unix time in hours. A
	// field gob adds without a new version; a file written without it is filled from Minutes.
	Hourly []minute
	// Hours holds per-app totals, oldest first. The last one is still filling and keeps
	// every app; a closed hour keeps only its top apps.
	Hours []hour
	// Icons maps an app name to its .app directory.
	// Never pruned; it grows by one entry per distinct bundle ever seen.
	Icons map[string]string

	// Day is the local date the counters below belong to.
	Day         string
	DiskWritten uint64
	NetDown     uint64
	NetUp       uint64
	NetApps     map[string]traffic

	// Alerts are the closed alerts as of the last flush, newest first. Like Hourly, a field
	// added without a new version: a file written without it has none.
	Alerts []alerts.Record
}

type minute struct {
	// At is unix time in minutes, in hours inside state.Hourly.
	At int64
	// N is the number of samples averaged into V; in state.Hourly, of closed minutes.
	N int32
	V [seriesCount]float32
}

type hour struct {
	// At is unix time in hours.
	At int64
	// Samples is the seconds the Add calls stand for; every app's CPU and Memory are sums
	// weighted by them, so a faster sampling rate does not weigh more.
	Samples int
	Apps    map[string]appHour
}

type appHour struct {
	// CPU sums percent of one core, Memory sums RSS bytes, Net is bytes down plus up.
	CPU    float64
	Memory float64
	Net    uint64
}

type traffic struct {
	Down uint64
	Up   uint64
}

// rollover zeroes the today counters when now falls on another local date.
func (st *state) rollover(now time.Time) {
	day := now.Local().Format(time.DateOnly)
	if st.Day == day {
		return
	}
	st.Day, st.DiskWritten, st.NetDown, st.NetUp, st.NetApps = day, 0, 0, 0, map[string]traffic{}
}

// addSample folds one sample into the running average of its minute, and that minute into
// its hour with a weight of one whatever its sample count: the sampling rate follows the
// panel, and an hour averaged over samples would weigh the time the panel was open more.
func (st *state) addSample(now time.Time, values [seriesCount]float32) {
	at := now.Unix() / 60
	var before minute
	if i := sort.Search(len(st.Minutes), func(i int) bool { return st.Minutes[i].At >= at }); i < len(st.Minutes) && st.Minutes[i].At == at {
		before = st.Minutes[i]
	}
	st.Minutes = fold(st.Minutes, at, retainMinutes, values)
	after := st.Minutes[len(st.Minutes)-1].V
	st.Hourly = st.Hourly[:sort.Search(len(st.Hourly), func(i int) bool { return st.Hourly[i].At > at/60 })]
	if n := len(st.Hourly); before.N <= 0 || n == 0 || st.Hourly[n-1].At != at/60 {
		st.Hourly = fold(st.Hourly, at/60, retainHourly, after)
		return
	}
	// The minute is in its hour already: only its share moves.
	h := &st.Hourly[len(st.Hourly)-1]
	for i := range h.V {
		h.V[i] += (after[i] - before.V[i]) / float32(h.N)
	}
}

// fillHourly rebuilds the hourly averages from the minutes when a file has none.
func (st *state) fillHourly() {
	if len(st.Hourly) > 0 {
		return
	}
	for _, m := range st.Minutes {
		// A minute without samples is a damaged entry: its values mean nothing.
		if m.N > 0 {
			st.Hourly = fold(st.Hourly, m.At/60, retainHourly, m.V)
		}
	}
}

// fold averages values into the entry at, the last of entries, and drops what is older
// than retain. Entries after at are what a forward clock jump left behind; they go once
// the clock is corrected.
//
// A forward jump past the retention still wipes history.
func fold(entries []minute, at, retain int64, values [seriesCount]float32) []minute {
	entries = entries[:sort.Search(len(entries), func(i int) bool { return entries[i].At > at })]
	if last := len(entries) - 1; last < 0 || entries[last].At < at {
		entries = append(entries, minute{At: at})
		expired := sort.Search(len(entries), func(i int) bool { return entries[i].At > at-retain })
		entries = slices.Delete(entries, 0, expired)
	}
	last := &entries[len(entries)-1]
	// A count below zero comes from a damaged file and would turn the average into Inf.
	last.N = max(last.N, 0) + 1
	for i, v := range values {
		// NaN or an overflow to Inf would poison the average for good.
		if v64 := float64(v); math.IsNaN(v64) || math.IsInf(v64, 0) {
			continue
		}
		last.V[i] += (v - last.V[i]) / float32(last.N)
	}
	return entries
}

// openHour returns the hour at, closing the previous one on a change; hours after
// now are dropped as in fold.
func (st *state) openHour(now time.Time) *hour {
	at := now.Unix() / 3600
	st.Hours = st.Hours[:sort.Search(len(st.Hours), func(i int) bool { return st.Hours[i].At > at })]
	if n := len(st.Hours); n == 0 || st.Hours[n-1].At < at {
		if n > 0 {
			st.Hours[n-1].prune()
		}
		st.Hours = append(st.Hours, hour{At: at, Apps: map[string]appHour{}})
		expired := sort.Search(len(st.Hours), func(i int) bool { return st.Hours[i].At > at-retainHours })
		st.Hours = slices.Delete(st.Hours, 0, expired)
	}
	return &st.Hours[len(st.Hours)-1]
}

// prune keeps the apps that rank in the hour's top by CPU, by memory or by network.
func (h *hour) prune() {
	names := slices.Collect(maps.Keys(h.Apps))
	keep := map[string]bool{}
	for _, value := range []func(appHour) float64{
		func(a appHour) float64 { return a.CPU },
		func(a appHour) float64 { return a.Memory },
		func(a appHour) float64 { return float64(a.Net) },
	} {
		slices.SortFunc(names, func(x, y string) int {
			return cmp.Or(cmp.Compare(value(h.Apps[y]), value(h.Apps[x])), cmp.Compare(x, y))
		})
		for _, name := range names[:min(topAppsPerHour, len(names))] {
			if value(h.Apps[name]) > 0 {
				keep[name] = true
			}
		}
	}
	maps.DeleteFunc(h.Apps, func(name string, _ appHour) bool { return !keep[name] })
}
