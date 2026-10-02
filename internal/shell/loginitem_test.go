package shell

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaunchAgentPlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		exe  string
	}{
		{name: "installed bundle", exe: "/Applications/mac-pulse.app/Contents/MacOS/mac-pulse"},
		{name: "spaces and non-ASCII", exe: "/Users/alex/Café Apps/mac-pulse.app/Contents/MacOS/mac-pulse"},
		{name: "XML metacharacters", exe: `/Users/alex/R&D <new> "x"/mac-pulse`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plist := launchAgentPlist(tt.exe)

			// plutil is the parser launchd's own tooling uses; reading each key back proves
			// the file is well-formed and carries the path unmangled.
			for keyPath, want := range map[string]string{
				"Label":              bundleID,
				"ProgramArguments.0": tt.exe,
				"RunAtLoad":          "true",
			} {
				cmd := exec.CommandContext(t.Context(), "plutil", "-extract", keyPath, "raw", "-o", "-", "-")
				cmd.Stdin = bytes.NewReader(plist)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, string(out))
				assert.Equal(t, want+"\n", string(out), keyPath)
			}
			assert.NotContains(t, string(plist), "KeepAlive", "quitting the app must not relaunch it")
		})
	}
}
