package collector

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserFamily(t *testing.T) {
	t.Parallel()

	const helpers = "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/154.0.8037.92/Helpers/"

	tests := []struct {
		name string
		exe  string
		want string
	}{
		{name: "the browser itself", exe: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", want: familyChromium},
		{
			name: "a helper bundle nested in the browser's",
			exe:  helpers + "Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)", want: familyChromium,
		},
		{name: "a bare helper binary of the browser", exe: helpers + "chrome_crashpad_handler", want: familyChromium},
		{
			name: "another Chromium browser, outside /Applications",
			exe:  "/Users/alex/Applications/Brave Browser.app/Contents/MacOS/Brave Browser", want: familyChromium,
		},
		{
			name: "an Electron app has the same helpers and is no browser",
			exe:  "/Applications/Slack.app/Contents/Frameworks/Slack Helper (Renderer).app/Contents/MacOS/Slack Helper (Renderer)",
		},
		{
			name: "Safari where macOS 15 keeps it",
			exe:  "/System/Volumes/Preboot/Cryptexes/App/System/Applications/Safari.app/Contents/MacOS/Safari", want: familySafari,
		},
		{
			name: "a Firefox child process",
			exe:  "/Applications/Firefox.app/Contents/MacOS/plugin-container.app/Contents/MacOS/plugin-container", want: familyFirefox,
		},
		{
			name: "a WebKit helper lives in a framework, outside any browser",
			exe:  "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.GPU.xpc/Contents/MacOS/com.apple.WebKit.GPU",
		},
		{name: "a binary outside any bundle", exe: "/usr/local/bin/Google Chrome"},
		{name: "no path", exe: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, browserFamily(tt.exe))
		})
	}
}

func TestBrowserKind(t *testing.T) {
	t.Parallel()

	// The arguments of six live Chrome 154 processes, as gopsutil returned them.
	raw, err := os.ReadFile("testdata/chrome_argv.json")
	require.NoError(t, err)
	var captured [][]string
	require.NoError(t, json.Unmarshal(raw, &captured))
	require.Len(t, captured, 6)

	// No Firefox capture: the arguments follow Mozilla's documentation.
	firefoxChild := func(kind string) []string {
		return []string{
			"/Applications/Firefox.app/Contents/MacOS/plugin-container.app/Contents/MacOS/plugin-container", "-contentproc",
			"-childID", "3", "-isForBrowser", "-prefsLen", "38291", "-appDir", "/Applications/Firefox.app/Contents/Resources/browser",
			"{0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0}", "4242", "gecko-crash-server-pipe.4242", "org.mozilla.machname.1804289383", "5", kind,
		}
	}

	tests := []struct {
		name   string
		family string
		// process is the process name: Safari's helpers tell their kind by it.
		process string
		argv    []string
		want    string
	}{
		{name: "captured: the main process has no --type", family: familyChromium, argv: captured[0], want: "browser"},
		{name: "captured: the crash handler has no --type either", family: familyChromium, argv: captured[1], want: "browser"},
		{name: "captured: gpu process", family: familyChromium, argv: captured[2], want: "gpu"},
		{name: "captured: network service is a utility", family: familyChromium, argv: captured[3], want: "utility"},
		{name: "captured: renderer", family: familyChromium, argv: captured[4], want: "tab"},
		{name: "captured: renderer of an extension", family: familyChromium, argv: captured[5], want: "extension"},
		{name: "a type this code has never seen is a utility", family: familyChromium, argv: []string{"Helper", "--type=ppapi"}, want: "utility"},
		{
			name: "--extension-process without a renderer type is not an extension", family: familyChromium,
			argv: []string{"Helper", "--type=utility", "--extension-process"}, want: "utility",
		},
		{name: "no arguments at all", family: familyChromium, want: "browser"},
		{name: "Safari itself", family: familySafari, process: "Safari", want: "browser"},
		{name: "Safari: web content", family: familySafari, process: "com.apple.WebKit.WebContent", want: "tab"},
		{name: "Safari: gpu", family: familySafari, process: "com.apple.WebKit.GPU", want: "gpu"},
		{name: "Safari: networking is a utility", family: familySafari, process: "com.apple.WebKit.Networking", want: "utility"},
		{name: "Safari: a helper of its own bundle", family: familySafari, process: "com.apple.Safari.SandboxBroker", want: "utility"},
		{
			name: "Firefox: the main process has no -contentproc", family: familyFirefox,
			argv: []string{"/Applications/Firefox.app/Contents/MacOS/firefox", "-foreground"}, want: "browser",
		},
		{name: "Firefox: content process", family: familyFirefox, argv: firefoxChild("tab"), want: "tab"},
		{name: "Firefox: gpu process", family: familyFirefox, argv: firefoxChild("gpu"), want: "gpu"},
		{name: "Firefox: socket process is a utility", family: familyFirefox, argv: firefoxChild("socket"), want: "utility"},
		{name: "Firefox: no arguments at all", family: familyFirefox, want: "browser"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, browserKind(tt.family, tt.process, tt.argv))
		})
	}
}
