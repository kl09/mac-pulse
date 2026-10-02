package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// The owner name may itself hold parentheses, so it runs up to the "): [0x" that follows it.
var assertionLine = regexp.MustCompile(`(?m)^\s+pid (\d+)\((.*?)\): \[0x[0-9a-f]+\] [\d:]+ (\w+) named: "(.*)"`)

func readSleepBlockers(ctx context.Context) ([]SleepBlocker, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pmset", "-g", "assertions").Output()
	if err != nil {
		return nil, fmt.Errorf("pmset assertions: %w", err)
	}
	return parseAssertions(bytes.NewReader(out))
}

// parseAssertions keeps the per-process assertions that hold off system or display
// sleep (Prevent*Sleep, No*SleepAssertion) and drops the rest (UserIsActive, BackgroundTask).
func parseAssertions(r io.Reader) ([]SleepBlocker, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read pmset assertions: %w", err)
	}
	lines := assertionLine.FindAllStringSubmatch(string(raw), -1)
	blockers := make([]SleepBlocker, 0, len(lines))
	for _, m := range lines {
		// powerd holds its own assertion whenever the display is on; it blames nobody.
		if !strings.Contains(m[3], "Sleep") || m[2] == "powerd" {
			continue
		}
		// digits-only by the regexp; an overflow reads as the maximum.
		pid, _ := strconv.ParseInt(m[1], 10, 32)
		blockers = append(blockers, SleepBlocker{PID: int32(pid), App: m[2], Kind: m[3], Name: m[4]})
	}
	return blockers, nil
}
