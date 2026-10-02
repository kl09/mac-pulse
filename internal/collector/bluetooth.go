package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// readBluetooth costs ~80 ms, so it runs once a minute on the sensors screen and every five otherwise.
func readBluetooth(ctx context.Context) ([]BluetoothDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "system_profiler", "SPBluetoothDataType", "-json").Output()
	if err != nil {
		return nil, fmt.Errorf("system_profiler bluetooth: %w", err)
	}
	return parseBluetooth(bytes.NewReader(out))
}

// parseBluetooth keeps the connected devices that report a battery level. Addresses and
// serial numbers in the input are never read.
func parseBluetooth(r io.Reader) ([]BluetoothDevice, error) {
	var doc struct {
		Controllers []struct {
			// Each element is a one-key object: the device name.
			Connected []map[string]map[string]any `json:"device_connected"`
		} `json:"SPBluetoothDataType"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode system_profiler bluetooth: %w", err)
	}
	var devices []BluetoothDevice
	for _, controller := range doc.Controllers {
		for _, entry := range controller.Connected {
			for name, props := range entry {
				d := BluetoothDevice{Name: name}
				d.Kind, _ = props["device_minorType"].(string)
				for _, part := range []string{"Left", "Right", "Case", "Main"} {
					level, _ := props["device_batteryLevel"+part].(string)
					// "99 %", with a no-break space before the sign.
					percent, err := strconv.Atoi(strings.TrimRight(level, " \u00a0%"))
					if err != nil {
						continue
					}
					d.Levels = append(d.Levels, BatteryLevel{Part: strings.ToLower(part), Percent: min(max(percent, 0), 100)})
				}
				if len(d.Levels) > 0 {
					devices = append(devices, d)
				}
			}
		}
	}
	return devices, nil
}
