package collector

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// The first call waits for the user to read and answer the Automation prompt.
	tabsTimeout = 120 * time.Second
	maxTabs     = 500
	maxTabRunes = 200
	// The bundle id and the property are Go's own: nothing the page sent reaches the script.
	// A browser is addressed by its id, and only while it runs: a look-alike that merely
	// carries the name is not it, and the script never launches one.
	tabScript = `if application id "%[1]s" is running then
	tell application id "%[1]s"
		set out to ""
		repeat with w in windows
			repeat with t in tabs of w
				set out to out & (%[2]s of t) & linefeed
			end repeat
		end repeat
		return out
	end tell
end if`
)

var (
	// ErrNoScript means the app has no AppleScript dictionary to ask for tabs (Firefox, a non-browser).
	ErrNoScript = errors.New("app has no tab script")
	// ErrDenied means the user did not let mac-pulse control the browser.
	ErrDenied = errors.New("automation not permitted")
	// ErrNotRunning means no app with the browser's bundle id runs; the script asked nothing.
	ErrNotRunning = errors.New("browser is not running")
)

// BrowserTabs asks a running browser for the titles of its tabs, at most 500 of 200 runes
// each. The first call per browser makes macOS ask the user for the Automation permission,
// so it runs only on an explicit click; app must be running, or the script would launch it.
func BrowserTabs(ctx context.Context, app string) ([]string, error) {
	script := tabsScript(app)
	if script == "" {
		return nil, fmt.Errorf("tabs of %q: %w", app, ErrNoScript)
	}
	ctx, cancel := context.WithTimeout(ctx, tabsTimeout)
	defer cancel()
	// A tab title is a page's own text: both streams are read up to the cap.
	var out, problem capped
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = &out, &problem, time.Second
	err := cmd.Run()
	switch {
	// "execution error: Not authorized to send Apple events to Safari. (-1743)"
	case err != nil && strings.Contains(string(problem.buf), "(-1743)"):
		return nil, fmt.Errorf("tabs of %q: %w", app, ErrDenied)
	case err != nil:
		return nil, fmt.Errorf("tabs of %q: %w", app, err)
	// A browser with no window still answers with an empty line.
	case len(out.buf) == 0:
		return nil, fmt.Errorf("tabs of %q: %w", app, ErrNotRunning)
	}
	return parseTabs(out.buf), nil
}

// tabsScript is the AppleScript that lists the tab titles of the browser app, "" for an app
// that has none to ask.
func tabsScript(app string) string {
	property := map[string]string{familyChromium: "title", familySafari: "name"}[browsers[app]]
	if property == "" || browserIDs[app] == "" {
		return ""
	}
	return fmt.Sprintf(tabScript, browserIDs[app], property)
}

// parseTabs reads one title per line; a page picks its own title, so each is cleaned and cut.
func parseTabs(out []byte) []string {
	titles := []string{}
	for line := range strings.Lines(string(out)) {
		if title := strings.TrimSpace(CleanText(line, maxTabRunes)); title != "" && len(titles) < maxTabs {
			titles = append(titles, title)
		}
	}
	return titles
}
