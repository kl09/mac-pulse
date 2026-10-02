package netinspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// With a broken interface networkQuality hangs past its own -M limit.
const speedTimeout = 40 * time.Second

// Speed is one networkQuality run.
type Speed struct {
	// Down and Up are bytes per second.
	Down float64
	Up   float64
	// RPM is the responsiveness in round trips per minute.
	RPM   float64
	RTTms float64
}

// SpeedTest runs the system's networkQuality against Apple's servers and moves about 200 MB.
// Like PublicIP it runs only on the user's click. A run cut off by the timeout wraps
// context.DeadlineExceeded.
func SpeedTest(ctx context.Context) (Speed, error) {
	ctx, cancel := context.WithTimeout(ctx, speedTimeout)
	defer cancel()
	// -M 20 caps each direction at 20 s; with less the download reads low.
	out, err := exec.CommandContext(ctx, "networkQuality", "-c", "-M", "20").Output()
	// The killed process reports "signal: killed", which says nothing about why.
	if ctx.Err() != nil {
		return Speed{}, fmt.Errorf("networkQuality: %w", ctx.Err())
	}
	if err != nil {
		return Speed{}, fmt.Errorf("networkQuality: %w", err)
	}
	return parseSpeed(bytes.NewReader(out))
}

// parseSpeed reads the JSON of `networkQuality -c`, whose throughputs are bits per second.
func parseSpeed(r io.Reader) (Speed, error) {
	var doc struct {
		Down *float64 `json:"dl_throughput"`
		Up   *float64 `json:"ul_throughput"`
		RPM  float64  `json:"responsiveness"`
		RTT  float64  `json:"base_rtt"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return Speed{}, fmt.Errorf("decode networkQuality: %w", err)
	}
	// Without the two keys the object is not a result, and 0 B/s would pass for one.
	if doc.Down == nil || doc.Up == nil {
		return Speed{}, errors.New("networkQuality gave no throughput")
	}
	return Speed{Down: *doc.Down / 8, Up: *doc.Up / 8, RPM: doc.RPM, RTTms: doc.RTT}, nil
}
