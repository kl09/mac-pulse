package collector

import (
	"context"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/kl09/mac-pulse/internal/native"
)

const (
	familyChromium = "chromium"
	familySafari   = "safari"
	familyFirefox  = "firefox"
)

// browsers maps the bundle names whose processes get a Kind to the way their kind is told.
// A fixed list; Electron apps share Chromium's argv scheme but are not browsers.
var browsers = map[string]string{
	"Google Chrome": familyChromium, "Google Chrome Beta": familyChromium, "Google Chrome Canary": familyChromium,
	"Chromium": familyChromium, "Microsoft Edge": familyChromium, "Brave Browser": familyChromium, "Arc": familyChromium,
	"Vivaldi": familyChromium, "Opera": familyChromium, "Yandex": familyChromium,
	"Safari": familySafari, "Safari Technology Preview": familySafari,
	"Firefox": familyFirefox, "Firefox Developer Edition": familyFirefox, "Firefox Nightly": familyFirefox,
}

// browserIDs are the bundle ids of the browsers that answer AppleScript, by bundle name.
var browserIDs = map[string]string{
	"Google Chrome": "com.google.Chrome", "Google Chrome Beta": "com.google.Chrome.beta", "Google Chrome Canary": "com.google.Chrome.canary",
	"Chromium": "org.chromium.Chromium", "Microsoft Edge": "com.microsoft.edgemac", "Brave Browser": "com.brave.Browser",
	"Arc": "company.thebrowser.Browser", "Vivaldi": "com.vivaldi.Vivaldi", "Opera": "com.operasoftware.Opera",
	"Yandex": "ru.yandex.desktop.yandex-browser",
	"Safari": "com.apple.Safari", "Safari Technology Preview": "com.apple.SafariTechnologyPreview",
}

// browserTag is what a browser process never changes.
type browserTag struct {
	kind        string
	responsible int32
}

// browserFamily names the family of the browser whose bundle exe sits in, "" outside one.
func browserFamily(exe string) string {
	i := strings.Index(exe, ".app/")
	if i < 0 {
		return ""
	}
	return browsers[appName(exe[:i+len(".app")])]
}

// safariHelper reports a process launchd starts on behalf of an app that shows web content;
// which app is ResponsiblePID's to say.
func safariHelper(name string) bool {
	return strings.HasPrefix(name, "com.apple.WebKit.") || strings.HasPrefix(name, "com.apple.Safari.")
}

// browserKind tells the role of a browser process. Chromium and Firefox say it in the
// arguments, Safari in the process name. A Chromium renderer and a WebKit content process
// serve a site, not a tab, so "tab" counts processes rather than the tabs on screen.
func browserKind(family, name string, argv []string) string {
	switch family {
	case familySafari:
		switch {
		case !safariHelper(name):
			return "browser"
		case name == "com.apple.WebKit.WebContent":
			return "tab"
		case name == "com.apple.WebKit.GPU":
			return "gpu"
		}
		return "utility"
	case familyFirefox:
		// Taken from Mozilla's documentation, not from a live Firefox: a child is
		// started with -contentproc and its type is the last argument.
		switch {
		case !slices.Contains(argv, "-contentproc"):
			return "browser"
		case argv[len(argv)-1] == "tab" || argv[len(argv)-1] == "gpu":
			return argv[len(argv)-1]
		}
		return "utility"
	}
	i := slices.IndexFunc(argv, func(arg string) bool { return strings.HasPrefix(arg, "--type=") })
	switch {
	case i < 0:
		return "browser"
	case argv[i] == "--type=renderer" && slices.Contains(argv, "--extension-process"):
		return "extension"
	case argv[i] == "--type=renderer":
		return "tab"
	case argv[i] == "--type=gpu-process":
		return "gpu"
	default:
		return "utility"
	}
}

// tagBrowsers fills Kind and Responsible. Neither changes while a process lives, so the
// arguments (~8 µs) and the responsible pid are read once per pid.
// A pid reused by another browser process between two scans keeps the old tag.
func (s *Sampler) tagBrowsers(ctx context.Context, procs []Process) {
	tags := map[int32]browserTag{}
	for i := range procs {
		p := &procs[i]
		family := browserFamily(p.Exe)
		// A WebKit helper is a child of launchd and lives outside Safari's bundle; every app
		// with a web view has them, so only Safari's own are tagged.
		helper := family == "" && safariHelper(p.Name)
		if family == "" && !helper {
			continue
		}
		tag, ok := s.tags[p.PID]
		switch {
		case ok:
		case helper:
			responsible := native.ResponsiblePID(p.PID)
			j := slices.IndexFunc(procs, func(q Process) bool { return q.PID == responsible })
			if j >= 0 && browserFamily(procs[j].Exe) == familySafari {
				tag = browserTag{kind: browserKind(familySafari, p.Name, nil), responsible: responsible}
			}
		case family == familySafari:
			tag.kind = browserKind(family, p.Name, nil)
		default:
			// Another user's browser answers EPERM and stays untagged.
			argv, err := (&process.Process{Pid: p.PID}).CmdlineSliceWithContext(ctx)
			if err != nil {
				continue
			}
			tag.kind = browserKind(family, p.Name, argv)
		}
		tags[p.PID], p.Kind, p.Responsible = tag, tag.kind, tag.responsible
	}
	s.tags = tags
}
