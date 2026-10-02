package collector

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/sensors"

	"github.com/kl09/mac-pulse/internal/native"
)

const (
	groupPerformance = "CPU performance"
	groupEfficiency  = "CPU efficiency"
	groupGPU         = "GPU"
	// groupPMU is the family Snapshot.CPUTemp belongs to: listed first, so that the headline's
	// number is the peak of the first group and the hotter cores come labelled after it.
	groupPMU = "Power management chip"

	// Some float keys under "T" are not temperatures (Tp01…Tp0v read 2–5 on an
	// M4 Pro). Nothing in a running Mac is colder than its 10 °C operating minimum; list
	// the bogus keys per chip if a real sensor is ever lost to this floor.
	minSensorTemp = 10
	// An idle Tp* key reads exactly 40.0, a placeholder rather than a measurement;
	// a live sensor lands on exactly 40 for a single tick at most. List the placeholder
	// keys per chip if that one tick ever matters.
	idleSensorTemp = 40
)

// sensorGroups maps the first two characters of an SMC key to its group, in display order.
// The prefixes were observed on an M4 Pro; a chip that names its sensors
// differently lands in "Other" until its prefixes are added.
var sensorGroups = []struct {
	prefix string
	name   string
}{
	{prefix: "TPD", name: groupPMU},
	{prefix: "Tp", name: groupPerformance},
	{prefix: "Te", name: groupEfficiency},
	{prefix: "Tg", name: groupGPU},
	{prefix: "TB", name: "Battery"},
	{prefix: "Ta", name: "Airflow"},
	{prefix: "Ts", name: "Palm rest"},
	{prefix: "TW", name: "Wireless"},
}

// readCPUTemp is the hottest die sensor of the power management chip (PMU), the one
// temperature that costs nothing to read on every sample. It is not a core: measured on an
// M4 Pro it reads what the SMC keys TPD* read, 51–54 °C idle and 62–74 °C with every core
// busy, while the cores (Tp*) go from 56–78 °C to 76–112 °C. tdev* report -9199 on M-series.
func readCPUTemp(ctx context.Context) (float64, error) {
	stats, err := sensors.TemperaturesWithContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("sensors: %w", err)
	}
	var maxTemp float64
	for _, s := range stats {
		if strings.HasPrefix(s.SensorKey, "PMU tdie") && s.Temperature > 0 && s.Temperature < 150 {
			maxTemp = max(maxTemp, s.Temperature)
		}
	}
	if maxTemp == 0 {
		return 0, errors.New("sensors: no PMU tdie reading")
	}
	return maxTemp, nil
}

// readTemps costs ~33 ms for the ~300 keys of an M4 Pro; the very first call walks the
// SMC key table and takes ~0.4 s.
func readTemps() ([]Sensor, error) {
	temps, err := native.Temps()
	if err != nil {
		return nil, fmt.Errorf("smc temperatures: %w", err)
	}
	return listTemps(temps), nil
}

// listTemps drops the keys that carry no measurement and orders the rest by group, then key.
func listTemps(temps map[string]float64) []Sensor {
	ranks := make(map[string]int, len(temps))
	list := make([]Sensor, 0, len(temps))
	for key, temp := range temps {
		if temp < minSensorTemp || temp == idleSensorTemp {
			continue
		}
		rank, group := sensorGroup(key)
		ranks[key] = rank
		list = append(list, Sensor{Group: group, Key: key, Temp: temp})
	}
	slices.SortFunc(list, func(x, y Sensor) int {
		return cmp.Or(cmp.Compare(ranks[x.Key], ranks[y.Key]), cmp.Compare(x.Key, y.Key))
	})
	return list
}

// sensorGroup returns the group's display rank and name; unknown keys rank last as "Other".
func sensorGroup(key string) (rank int, name string) {
	for i, g := range sensorGroups {
		if strings.HasPrefix(key, g.prefix) {
			return i, g.name
		}
	}
	return len(sensorGroups), "Other"
}
