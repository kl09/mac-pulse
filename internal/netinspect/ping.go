package netinspect

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var pingTime = regexp.MustCompile(`time=([\d.]+) ms`)

// ErrNoReply is a ping that left but was not answered within its 2 s.
var ErrNoReply = errors.New("no reply from 1.1.1.1")

// Ping sends one ICMP echo to 1.1.1.1 through /sbin/ping (setuid, so no root here) and
// returns the round trip. Like PublicIP it runs only on the user's click.
func Ping(ctx context.Context) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ping", "-c", "1", "-t", "2", "1.1.1.1").Output()
	// ping exits 2 when nothing came back; anything else is a failure to send.
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return 0, fmt.Errorf("ping: %w", err)
	}
	return parsePing(out)
}

func parsePing(out []byte) (time.Duration, error) {
	m := pingTime.FindSubmatch(out)
	if m == nil {
		return 0, ErrNoReply
	}
	ms, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ping time %q: %w", m[1], err)
	}
	return time.Duration(ms * float64(time.Millisecond)), nil
}
