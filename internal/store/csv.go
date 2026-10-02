package store

import (
	"cmp"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// csvColumns are the columns of the system document after "time", in the order of minute.V.
var csvColumns = [seriesCount]struct {
	name     string
	decimals int
	// zeroIsEmpty marks a series whose 0 means "no reading": the cell stays empty.
	zeroIsEmpty bool
}{
	seriesCPU:       {name: "cpu_percent", decimals: 1},
	seriesMemory:    {name: "memory_bytes"},
	seriesNetDown:   {name: "net_down_bytes_per_s"},
	seriesNetUp:     {name: "net_up_bytes_per_s"},
	seriesDiskRead:  {name: "disk_read_bytes_per_s"},
	seriesDiskWrite: {name: "disk_write_bytes_per_s"},
	seriesGPU:       {name: "gpu_percent", decimals: 1},
	seriesTemp:      {name: "temp_c", decimals: 1, zeroIsEmpty: true},
	seriesBattery:   {name: "battery_percent"},
	seriesPower:     {name: "power_w", decimals: 2, zeroIsEmpty: true},
	seriesCPUPower:  {name: "cpu_w", decimals: 2, zeroIsEmpty: true},
	seriesGPUPower:  {name: "gpu_w", decimals: 2, zeroIsEmpty: true},
}

// WriteCSV exports the history as two CSV documents: the hourly averages of every system
// series, times in UTC, and each app's averages per local day over the last 30 days.
func (s *Store) WriteCSV(system, apps io.Writer) error {
	s.mu.Lock()
	systemRows, appRows := s.st.systemRows(), s.st.appRows()
	s.mu.Unlock()
	if err := csv.NewWriter(system).WriteAll(systemRows); err != nil {
		return fmt.Errorf("write system CSV: %w", err)
	}
	if err := csv.NewWriter(apps).WriteAll(appRows); err != nil {
		return fmt.Errorf("write apps CSV: %w", err)
	}
	return nil
}

func (st *state) systemRows() [][]string {
	header := []string{"time"}
	for _, c := range csvColumns {
		header = append(header, c.name)
	}
	rows := [][]string{header}
	for _, h := range st.Hourly {
		row := []string{time.Unix(h.At*3600, 0).UTC().Format(time.RFC3339)}
		for i, c := range csvColumns {
			cell := ""
			if !c.zeroIsEmpty || h.V[i] != 0 {
				cell = strconv.FormatFloat(float64(h.V[i]), 'f', c.decimals, 64)
			}
			row = append(row, cell)
		}
		rows = append(rows, row)
	}
	return rows
}

// appRows sums the hours of each local day; an app that did nothing all day has no row.
// An app that missed an hour's top counts as absent in that hour, so its daily
// averages read low.
func (st *state) appRows() [][]string {
	type key struct{ day, app string }
	totals, samples := map[key]appHour{}, map[string]int{}
	for _, h := range st.Hours {
		day := time.Unix(h.At*3600, 0).Format(time.DateOnly)
		samples[day] += h.Samples
		for name, a := range h.Apps {
			t := totals[key{day, name}]
			totals[key{day, name}] = appHour{CPU: t.CPU + a.CPU, Memory: t.Memory + a.Memory, Net: t.Net + a.Net}
		}
	}
	keys := make([]key, 0, len(totals))
	for k, t := range totals {
		if t != (appHour{}) {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(x, y key) int { return cmp.Or(cmp.Compare(x.day, y.day), cmp.Compare(x.app, y.app)) })
	rows := [][]string{{"date", "app", "cpu_percent_avg", "memory_bytes_avg", "network_bytes"}}
	for _, k := range keys {
		// An hour that only AddUsage touched has traffic and no samples.
		t, n := totals[k], float64(max(samples[k.day], 1))
		// A process picks its own name, and a spreadsheet runs a cell that starts like a formula.
		app := k.app
		if strings.ContainsAny(app[:min(len(app), 1)], "=+-@\t\r") {
			app = "'" + app
		}
		rows = append(rows, []string{
			k.day, app, strconv.FormatFloat(t.CPU/n, 'f', 1, 64), strconv.FormatFloat(t.Memory/n, 'f', 0, 64),
			strconv.FormatUint(t.Net, 10),
		})
	}
	return rows
}
