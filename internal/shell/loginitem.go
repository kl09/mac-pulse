package shell

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// bundleID is also CFBundleIdentifier in packaging/Info.plist.
const bundleID = "com.kl09.mac-pulse"

// Interactive lifts the CPU and I/O throttling launchd applies to background agents.
const launchAgentFormat = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`

// SetLoginItem writes or removes the LaunchAgent that starts the running executable at login.
//
// launchd is not told (no launchctl bootstrap), so the change takes effect at the
// next login and a moved app needs the switch flipped again. SMAppService fixes both but
// wants a properly signed bundle.
func SetLoginItem(enabled bool) error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove launch agent: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	// A bare binary (make run, bin/mac-pulse) has no stable path and no notification identity.
	if !strings.Contains(exe, ".app/") {
		return errors.New("run from mac-pulse.app (make install) to enable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create launch agents directory: %w", err)
	}
	// launchd must never read a half-written plist at login.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, launchAgentPlist(exe), 0o644); err != nil {
		return fmt.Errorf("write launch agent: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install launch agent: %w", err)
	}
	return nil
}

func LoginItem() bool {
	path, err := launchAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", bundleID+".plist"), nil
}

func launchAgentPlist(exe string) []byte {
	var escaped bytes.Buffer
	// A bytes.Buffer never fails a write.
	_ = xml.EscapeText(&escaped, []byte(exe))
	return fmt.Appendf(nil, launchAgentFormat, bundleID, escaped.String())
}
