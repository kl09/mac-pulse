package ui

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
)

//nolint:funlen // one table, a line per message shape
func TestParseMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    Message
		wantErr bool
	}{
		{name: "tab", raw: `{"type":"tab","name":"network","mode":"window"}`, want: Message{Type: "tab", Name: "network", Mode: "window"}},
		{
			name: "tab on a detail screen", raw: `{"type":"tab","name":"detail:sensors","mode":"popover"}`,
			want: Message{Type: "tab", Name: "detail:sensors", Mode: "popover"},
		},
		{name: "tab on a detail screen that does not exist", raw: `{"type":"tab","name":"detail:admin","mode":"popover"}`, wantErr: true},
		{name: "tab on a detail prefix alone", raw: `{"type":"tab","name":"detail:","mode":"popover"}`, wantErr: true},
		{name: "tab with an unknown name", raw: `{"type":"tab","name":"admin","mode":"popover"}`, wantErr: true},
		{name: "tab with an unknown mode", raw: `{"type":"tab","name":"network","mode":"fullscreen"}`, wantErr: true},
		{
			name: "quit app", raw: `{"type":"quit_app","app":"yes","pids":[5,6],"force":true}`,
			want: Message{Type: "quit_app", App: "yes", PIDs: []int32{5, 6}, Force: true},
		},
		{name: "quit app without the app name", raw: `{"type":"quit_app","pids":[5]}`, wantErr: true},
		{name: "quit app without pids", raw: `{"type":"quit_app","app":"yes","pids":[]}`, wantErr: true},
		{name: "quit app with a fractional pid", raw: `{"type":"quit_app","app":"yes","pids":[5.5]}`, wantErr: true},
		{name: "quit app with a pid beyond int32", raw: `{"type":"quit_app","app":"yes","pids":[4294967297]}`, wantErr: true},
		{name: "quit app with a string pid", raw: `{"type":"quit_app","app":"yes","pids":["5"]}`, wantErr: true},
		{
			name: "quit app with too many pids", wantErr: true,
			raw: `{"type":"quit_app","app":"yes","pids":[` + strings.Repeat("5,", maxQuitPIDs) + `5]}`,
		},
		{
			name: "set keeps the value raw", raw: `{"type":"set","key":"menu_bar","value":["cpu"]}`,
			want: Message{Type: "set", Key: "menu_bar", Value: json.RawMessage(`["cpu"]`)},
		},
		{name: "set without a value", raw: `{"type":"set","key":"alerts"}`, wantErr: true},
		{name: "set without a key", raw: `{"type":"set","value":true}`, wantErr: true},
		{name: "history", raw: `{"type":"history","metric":"cpu","range":"1h"}`, want: Message{Type: "history", Metric: "cpu", Range: "1h"}},
		{name: "login item", raw: `{"type":"login_item","enabled":true}`, want: Message{Type: "login_item", Enabled: true}},
		{name: "login item with a string flag", raw: `{"type":"login_item","enabled":"yes"}`, wantErr: true},
		{name: "visibility", raw: `{"type":"visibility","popover":true,"window":false}`, want: Message{Type: "visibility", Popover: true}},
		{name: "quit", raw: `{"type":"quit"}`, want: Message{Type: "quit"}},
		{name: "app info of an app", raw: `{"type":"app_info","name":"Safari"}`, want: Message{Type: "app_info", Name: "Safari"}},
		{
			name: "app info of a process", raw: `{"type":"app_info","name":"Safari","pid":7}`,
			want: Message{Type: "app_info", Name: "Safari", PID: 7},
		},
		{name: "app info without a name", raw: `{"type":"app_info","pid":7}`, wantErr: true},
		{name: "app info with a negative pid", raw: `{"type":"app_info","name":"Safari","pid":-1}`, wantErr: true},
		{name: "app info with a string pid", raw: `{"type":"app_info","name":"Safari","pid":"7"}`, wantErr: true},
		{
			name: "app info with a name no file can have", wantErr: true,
			raw: `{"type":"app_info","name":"` + strings.Repeat("x", maxNameBytes+1) + `"}`,
		},
		{name: "open window", raw: `{"type":"open_window"}`, want: Message{Type: "open_window"}},
		{name: "back to the panel", raw: `{"type":"open_panel"}`, want: Message{Type: "open_panel"}},
		{
			name: "log is cut to a bounded length", raw: `{"type":"log","message":"` + strings.Repeat("x", maxLogBytes+10) + `"}`,
			want: Message{Type: "log", Message: strings.Repeat("x", maxLogBytes)},
		},
		{
			name: "log cut inside a rune drops the broken tail", raw: `{"type":"log","message":"` + strings.Repeat("x", maxLogBytes-1) + `é"}`,
			want: Message{Type: "log", Message: strings.Repeat("x", maxLogBytes-1)},
		},
		{name: "public ip", raw: `{"type":"public_ip"}`, want: Message{Type: "public_ip"}},
		{name: "ping", raw: `{"type":"ping"}`, want: Message{Type: "ping"}},
		{name: "export", raw: `{"type":"export"}`, want: Message{Type: "export"}},
		{name: "copy", raw: `{"type":"copy"}`, want: Message{Type: "copy"}},
		{name: "tab dev", raw: `{"type":"tab","name":"dev","mode":"popover"}`, want: Message{Type: "tab", Name: "dev", Mode: "popover"}},
		{
			name: "visibility with a pinned panel", raw: `{"type":"visibility","popover":true,"window":false,"pinned":true}`,
			want: Message{Type: "visibility", Popover: true, Pinned: true},
		},
		{name: "eject", raw: `{"type":"eject","mount":"/Volumes/T7"}`, want: Message{Type: "eject", Mount: "/Volumes/T7"}},
		{name: "reveal a volume", raw: `{"type":"reveal","mount":"/"}`, want: Message{Type: "reveal", Mount: "/"}},
		{name: "reveal a process", raw: `{"type":"reveal","pid":77}`, want: Message{Type: "reveal", PID: 77}},
		{name: "export csv", raw: `{"type":"export_csv"}`, want: Message{Type: "export_csv"}},
		{name: "speed test", raw: `{"type":"speedtest"}`, want: Message{Type: "speedtest"}},
		{name: "pin", raw: `{"type":"pin","enabled":true}`, want: Message{Type: "pin", Enabled: true}},
		{
			name: "url names its tab", raw: `{"type":"url","url":"mac-pulse://open?tab=detail:cpu"}`,
			want: Message{Type: "url", URL: "mac-pulse://open?tab=detail:cpu", Name: "detail:cpu"},
		},
		{name: "hotkey", raw: `{"type":"hotkey","name":"quit"}`, want: Message{Type: "hotkey", Name: "quit"}},
		{name: "eject of the boot volume", raw: `{"type":"eject","mount":"/"}`, wantErr: true},
		{name: "eject outside /Volumes", raw: `{"type":"eject","mount":"/Users/alex"}`, wantErr: true},
		{name: "eject that climbs out of /Volumes", raw: `{"type":"eject","mount":"/Volumes/../"}`, wantErr: true},
		{name: "eject without a mount", raw: `{"type":"eject"}`, wantErr: true},
		{name: "reveal with both a mount and a pid", raw: `{"type":"reveal","mount":"/","pid":77}`, wantErr: true},
		{name: "reveal with neither", raw: `{"type":"reveal"}`, wantErr: true},
		{name: "reveal of a negative pid", raw: `{"type":"reveal","pid":-1}`, wantErr: true},
		{name: "reveal of a relative path", raw: `{"type":"reveal","mount":"Volumes/T7"}`, wantErr: true},
		{name: "reveal of an unclean path", raw: `{"type":"reveal","mount":"/Volumes/T7/../../etc"}`, wantErr: true},
		{
			name: "reveal of a path longer than any mount", wantErr: true,
			raw: `{"type":"reveal","mount":"/` + strings.Repeat("x", maxMountBytes) + `"}`,
		},
		{name: "url of another scheme", raw: `{"type":"url","url":"https://open?tab=dev"}`, wantErr: true},
		{name: "url of another action", raw: `{"type":"url","url":"mac-pulse://quit?tab=dev"}`, wantErr: true},
		{name: "url with a path", raw: `{"type":"url","url":"mac-pulse://open/x?tab=dev"}`, wantErr: true},
		{name: "url with an unknown tab", raw: `{"type":"url","url":"mac-pulse://open?tab=admin"}`, wantErr: true},
		{name: "url with script in the tab", raw: `{"type":"url","url":"mac-pulse://open?tab=dev');alert(1)//"}`, wantErr: true},
		{name: "url without a tab", raw: `{"type":"url","url":"mac-pulse://open"}`, wantErr: true},
		{name: "url that does not parse", raw: `{"type":"url","url":"mac-pulse://%zz"}`, wantErr: true},
		{name: "hotkey with an unknown name", raw: `{"type":"hotkey","name":"panel"}`, wantErr: true},
		{name: "an action whose type differs in case", raw: `{"type":"Ping"}`, wantErr: true},
		{
			name: "tab on storage", raw: `{"type":"tab","name":"storage","mode":"popover"}`,
			want: Message{Type: "tab", Name: "storage", Mode: "popover"},
		},
		{
			name: "tab on the clock screen", raw: `{"type":"tab","name":"detail:clock","mode":"popover"}`,
			want: Message{Type: "tab", Name: "detail:clock", Mode: "popover"},
		},
		{name: "storage scan of home", raw: `{"type":"storage_scan","root":"home"}`, want: Message{Type: "storage_scan", Root: "home"}},
		{
			name: "storage scan of a chosen folder", raw: `{"type":"storage_scan","root":"choose"}`,
			want: Message{Type: "storage_scan", Root: "choose"},
		},
		{name: "storage scan of a path from the page", raw: `{"type":"storage_scan","root":"/etc"}`, wantErr: true},
		{name: "storage scan without a root", raw: `{"type":"storage_scan"}`, wantErr: true},
		{name: "storage cancel", raw: `{"type":"storage_cancel"}`, want: Message{Type: "storage_cancel"}},
		{name: "storage clear", raw: `{"type":"storage_clear"}`, want: Message{Type: "storage_clear"}},
		{name: "storage open of the root", raw: `{"type":"storage_open","path":""}`, want: Message{Type: "storage_open"}},
		{
			name: "storage open of a folder", raw: `{"type":"storage_open","path":"Library/Caches"}`,
			want: Message{Type: "storage_open", Path: "Library/Caches"},
		},
		{name: "storage open that climbs out", raw: `{"type":"storage_open","path":"../x"}`, wantErr: true},
		{name: "storage open that climbs out and back", raw: `{"type":"storage_open","path":"a/../b"}`, wantErr: true},
		{name: "storage open of an absolute path", raw: `{"type":"storage_open","path":"/etc"}`, wantErr: true},
		{name: "storage open with a trailing slash", raw: `{"type":"storage_open","path":"a/"}`, wantErr: true},
		{
			name: "storage open of a path longer than any path", wantErr: true,
			raw: `{"type":"storage_open","path":"` + strings.Repeat("a/", maxMountBytes/2) + `a"}`,
		},
		{name: "reveal of a scanned path", raw: `{"type":"reveal","path":"Movies/film"}`, want: Message{Type: "reveal", Path: "Movies/film"}},
		{name: "reveal of a path that climbs out", raw: `{"type":"reveal","path":"../x"}`, wantErr: true},
		{name: "reveal with both a pid and a path", raw: `{"type":"reveal","pid":77,"path":"Movies"}`, wantErr: true},
		{name: "reveal with both a mount and a path", raw: `{"type":"reveal","mount":"/Volumes/T7","path":"Movies"}`, wantErr: true},
		{name: "cleanup scan", raw: `{"type":"cleanup_scan"}`, want: Message{Type: "cleanup_scan"}},
		{
			name: "cleanup list", raw: `{"type":"cleanup_list","category":"go_build"}`,
			want: Message{Type: "cleanup_list", Category: "go_build"},
		},
		{
			name: "listing the Trash only reads", raw: `{"type":"cleanup_list","category":"trash"}`,
			want: Message{Type: "cleanup_list", Category: "trash"},
		},
		{name: "cleanup list of an unknown category", raw: `{"type":"cleanup_list","category":"documents"}`, wantErr: true},
		{name: "cleanup list of a path", raw: `{"type":"cleanup_list","category":"../x"}`, wantErr: true},
		{
			name: "cleanup of named items to the Trash", raw: `{"type":"cleanup","category":"go_build","permanent":false,"items":["ab","trim.txt"]}`,
			want: Message{Type: "cleanup", Category: "go_build", Items: []string{"ab", "trim.txt"}},
		},
		{
			name: "cleanup names the review it confirms",
			raw:  `{"type":"cleanup","category":"go_build","review":"1790777400000000000","items":["ab"]}`,
			want: Message{Type: "cleanup", Category: "go_build", Review: "1790777400000000000", Items: []string{"ab"}},
		},
		{
			name: "cleanup of the review's last row alone", raw: `{"type":"cleanup","category":"go_build","items":[],"rest":true}`,
			want: Message{Type: "cleanup", Category: "go_build", Items: []string{}, Rest: true},
		},
		{
			name: "cleanup for good", raw: `{"type":"cleanup","category":"go_build","permanent":true,"items":["ab"]}`,
			want: Message{Type: "cleanup", Category: "go_build", Permanent: true, Items: []string{"ab"}},
		},
		{
			name: "emptying the Trash says it is for good", raw: `{"type":"cleanup","category":"trash","permanent":true,"items":["old.zip"]}`,
			want: Message{Type: "cleanup", Category: "trash", Permanent: true, Items: []string{"old.zip"}},
		},
		{name: "emptying the Trash without saying so", raw: `{"type":"cleanup","category":"trash","items":["old.zip"]}`, wantErr: true},
		{name: "cleanup of an unknown category", raw: `{"type":"cleanup","category":"documents","items":["a"]}`, wantErr: true},
		{name: "cleanup of a path", raw: `{"type":"cleanup","category":"../x","items":["a"]}`, wantErr: true},
		{name: "cleanup without a category", raw: `{"type":"cleanup","items":["a"]}`, wantErr: true},
		{name: "cleanup with nothing selected", raw: `{"type":"cleanup","category":"go_build"}`, wantErr: true},
		{name: "cleanup of an item that climbs out", raw: `{"type":"cleanup","category":"go_build","items":["../x"]}`, wantErr: true},
		{name: "cleanup of the parent folder", raw: `{"type":"cleanup","category":"go_build","items":[".."]}`, wantErr: true},
		{name: "cleanup of the category folder itself", raw: `{"type":"cleanup","category":"go_build","items":["."]}`, wantErr: true},
		{name: "cleanup of an absolute path", raw: `{"type":"cleanup","category":"go_build","items":["/abs"]}`, wantErr: true},
		{name: "cleanup of a path below an item", raw: `{"type":"cleanup","category":"go_build","items":["a/b"]}`, wantErr: true},
		{name: "cleanup of an empty name", raw: `{"type":"cleanup","category":"go_build","items":["ab",""]}`, wantErr: true},
		{name: "cleanup of a name with a NUL", raw: `{"type":"cleanup","category":"go_build","items":["a\u0000b"]}`, wantErr: true},
		{
			name: "cleanup of more names than a review lists",
			raw:  `{"type":"cleanup","category":"go_build","items":["` + strings.Repeat(`x","`, maxReviewItems) + `x"]}`, wantErr: true,
		},
		{name: "browser tabs", raw: `{"type":"browser_tabs","name":"Safari"}`, want: Message{Type: "browser_tabs", Name: "Safari"}},
		{name: "browser tabs without a name", raw: `{"type":"browser_tabs"}`, wantErr: true},
		{
			name: "process detail with the command line", raw: `{"type":"proc_detail","name":"iTerm","pid":812,"args":true}`,
			want: Message{Type: "proc_detail", Name: "iTerm", PID: 812, Args: true},
		},
		{name: "process detail of an app, not a process", raw: `{"type":"proc_detail","name":"iTerm","pid":0}`, wantErr: true},
		{name: "process detail without a name", raw: `{"type":"proc_detail","pid":812}`, wantErr: true},
		{
			name: "signal to one process", raw: `{"type":"quit_app","app":"yes","pids":[5],"signal":"STOP"}`,
			want: Message{Type: "quit_app", App: "yes", PIDs: []int32{5}, Signal: "STOP"},
		},
		{name: "signal outside the list", raw: `{"type":"quit_app","app":"yes","pids":[5],"signal":"USR1"}`, wantErr: true},
		{name: "signal by number", raw: `{"type":"quit_app","app":"yes","pids":[5],"signal":9}`, wantErr: true},
		{name: "signal to two processes", raw: `{"type":"quit_app","app":"yes","pids":[5,6],"signal":"STOP"}`, wantErr: true},
		{name: "signal together with force", raw: `{"type":"quit_app","app":"yes","pids":[5],"signal":"TERM","force":true}`, wantErr: true},
		{name: "update check", raw: `{"type":"update_check"}`, want: Message{Type: "update_check"}},
		{name: "open the battery settings", raw: `{"type":"open","target":"battery"}`, want: Message{Type: "open", Target: "battery"}},
		{name: "open of an address", raw: `{"type":"open","target":"https://example.com"}`, wantErr: true},
		{name: "open without a target", raw: `{"type":"open"}`, wantErr: true},
		{
			name: "tab on the alert screen", raw: `{"type":"tab","name":"detail:alert","mode":"popover"}`,
			want: Message{Type: "tab", Name: "detail:alert", Mode: "popover"},
		},
		{name: "tab with an alert id in its name", raw: `{"type":"tab","name":"detail:alert:cpu::5","mode":"popover"}`, wantErr: true},
		{
			name: "url names the alert screen", raw: `{"type":"url","url":"mac-pulse://open?tab=detail:alert"}`,
			want: Message{Type: "url", URL: "mac-pulse://open?tab=detail:alert", Name: "detail:alert"},
		},
		{name: "url with an alert id", raw: `{"type":"url","url":"mac-pulse://open?tab=detail:alert:cpu::5"}`, wantErr: true},
		{
			name: "tab on a section of the settings", raw: `{"type":"tab","name":"settings:alerts","mode":"window"}`,
			want: Message{Type: "tab", Name: "settings:alerts", Mode: "window"},
		},
		{name: "tab on an unknown section of the settings", raw: `{"type":"tab","name":"settings:nope","mode":"popover"}`, wantErr: true},
		{
			name: "url names a section of the settings", raw: `{"type":"url","url":"mac-pulse://open?tab=settings:appearance"}`,
			want: Message{Type: "url", URL: "mac-pulse://open?tab=settings:appearance", Name: "settings:appearance"},
		},
		{name: "url with an unknown section of the settings", raw: `{"type":"url","url":"mac-pulse://open?tab=settings:nope"}`, wantErr: true},
		{
			name: "alert detail of a system alert", raw: `{"type":"alert_detail","id":"cpu::1790777400"}`,
			want: Message{Type: "alert_detail", ID: "cpu::1790777400"},
		},
		{
			name: "alert detail of an app, whose name has a colon and a space",
			raw:  `{"type":"alert_detail","id":"app_cpu:My App: beta:1790777400"}`,
			want: Message{Type: "alert_detail", ID: "app_cpu:My App: beta:1790777400"},
		},
		{
			name: "alert detail of an own rule carries its metric", raw: `{"type":"alert_detail","id":"app_rulememory:Xcode:1790777400"}`,
			want: Message{Type: "alert_detail", ID: "app_rulememory:Xcode:1790777400"},
		},
		{
			name: "alert detail of an id no alert can have is passed on: the session answers that it keeps no such alert",
			raw:  `{"type":"alert_detail","id":"root::17e3"}`, want: Message{Type: "alert_detail", ID: "root::17e3"},
		},
		{name: "alert detail without an id", raw: `{"type":"alert_detail"}`, want: Message{Type: "alert_detail"}},
		{name: "alert detail with an id of the wrong type", raw: `{"type":"alert_detail","id":7}`, wantErr: true},
		{
			name: "alert detail with an id longer than any name is cut, on a rune",
			raw:  `{"type":"alert_detail","id":"app_cpu:` + strings.Repeat("a", 311) + `é:5"}`,
			want: Message{Type: "alert_detail", ID: "app_cpu:" + strings.Repeat("a", 311)},
		},
		{name: "an action with a field of the wrong type", raw: `{"type":"export","name":{"path":"/etc/passwd"}}`, wantErr: true},
		{name: "unknown type", raw: `{"type":"exec","message":"rm -rf"}`, wantErr: true},
		{name: "no type", raw: `{}`, wantErr: true},
		{name: "array instead of an object", raw: `[1]`, wantErr: true},
		{name: "not JSON", raw: `{"type":`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMessage([]byte(tt.raw))

			if tt.wantErr {
				require.ErrorIs(t, err, ErrBadMessage)
				assert.Equal(t, Message{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQuitTargets(t *testing.T) {
	t.Parallel()

	self := Self{UID: 501, PID: 900}
	apps := []collector.App{
		{Name: "yes", Processes: []collector.Process{{PID: 5, UID: 501, Name: "yes"}, {PID: 6, UID: 501, Name: "yes"}}},
		{Name: "Safari", Processes: []collector.Process{{PID: 7, UID: 501, Name: "Safari"}}},
		{Name: "Docker", Processes: []collector.Process{{PID: 8, UID: 501, Name: "Docker"}, {PID: 9, UID: 0, Name: "vmnetd"}}},
		{Name: "launchd", Processes: []collector.Process{{PID: 1, UID: 0, Name: "launchd"}}},
		{Name: "kernel_task", Processes: []collector.Process{{PID: 0, UID: 0, Name: "kernel_task"}}},
		{Name: "mac-pulse", Processes: []collector.Process{{PID: 900, UID: 501, Name: "mac-pulse"}}},
		{Name: "loginwindow", Processes: []collector.Process{
			{PID: 10, UID: 501, Name: "loginwindow", Exe: "/System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow"},
		}},
		{Name: "gpj.exe\u202e", Processes: []collector.Process{{PID: 11, UID: 501, Name: "gpj.exe\u202e"}}},
	}
	tests := []struct {
		name        string
		app         string
		pids        []int32
		signal      string
		wantTargets []int32
		wantRefused []Text
	}{
		{name: "every pid of the app", app: "yes", pids: []int32{5, 6}, wantTargets: []int32{5, 6}},
		{name: "a signal to the user's own process", app: "yes", pids: []int32{5}, signal: "STOP", wantTargets: []int32{5}},
		{
			name: "a signal to another user's process is refused", app: "Docker", pids: []int32{9}, signal: "STOP",
			wantRefused: []Text{{Key: "quit.not_owned", Params: map[string]any{"pid": int32(9), "name": "vmnetd"}}},
		},
		{
			name: "a signal to a system process is refused", app: "loginwindow", pids: []int32{10}, signal: "KILL",
			wantRefused: []Text{{Key: "quit.system", Params: map[string]any{"pid": int32(10), "name": "loginwindow"}}},
		},
		{
			name: "a signal to launchd is refused", app: "launchd", pids: []int32{1}, signal: "STOP",
			wantRefused: []Text{{Key: "quit.system_pid", Params: map[string]any{"pid": int32(1)}}},
		},
		{
			name: "a signal to mac-pulse itself is refused", app: "mac-pulse", pids: []int32{900}, signal: "TERM",
			wantRefused: []Text{{Key: "quit.self"}},
		},
		{name: "an app is matched by the name the state showed, bidi removed", app: "gpj.exe", pids: []int32{11}, wantTargets: []int32{11}},
		{name: "a repeated pid is signalled once", app: "yes", pids: []int32{5, 5}, wantTargets: []int32{5}},
		{name: "a pid that is gone is dropped silently", app: "yes", pids: []int32{5, 4242}, wantTargets: []int32{5}},
		{name: "an app that is gone has no targets", app: "curl", pids: []int32{4242}},
		{
			name: "a pid of another app is refused", app: "yes", pids: []int32{5, 7},
			wantTargets: []int32{5},
			wantRefused: []Text{{Key: "quit.moved", Params: map[string]any{"pid": int32(7), "owner": "Safari", "app": "yes"}}},
		},
		{
			name: "a root process inside the user's app is refused", app: "Docker", pids: []int32{8, 9},
			wantTargets: []int32{8}, wantRefused: []Text{{Key: "quit.not_owned", Params: map[string]any{"pid": int32(9), "name": "vmnetd"}}},
		},
		{
			name: "launchd is refused", app: "launchd", pids: []int32{1},
			wantRefused: []Text{{Key: "quit.system_pid", Params: map[string]any{"pid": int32(1)}}},
		},
		{
			name: "pid 0 is refused", app: "kernel_task", pids: []int32{0},
			wantRefused: []Text{{Key: "quit.system_pid", Params: map[string]any{"pid": int32(0)}}},
		},
		{
			name: "a negative pid would signal a process group", app: "yes", pids: []int32{-1, -5},
			wantRefused: []Text{
				{Key: "quit.system_pid", Params: map[string]any{"pid": int32(-1)}},
				{Key: "quit.system_pid", Params: map[string]any{"pid": int32(-5)}},
			},
		},
		{
			name: "mac-pulse does not quit itself", app: "mac-pulse", pids: []int32{900},
			wantRefused: []Text{{Key: "quit.self"}},
		},
		{
			name: "the user's own system process is refused", app: "loginwindow", pids: []int32{10},
			wantRefused: []Text{{Key: "quit.system", Params: map[string]any{"pid": int32(10), "name": "loginwindow"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targets, refused := QuitTargets(apps, Message{Type: "quit_app", App: tt.app, PIDs: tt.pids, Signal: tt.signal}, self)

			assert.Equal(t, tt.wantTargets, targets)
			assert.Equal(t, tt.wantRefused, refused)
		})
	}
}

func TestRevealPath(t *testing.T) {
	t.Parallel()

	self := Self{UID: 501, PID: 900}
	apps := []collector.App{
		{Name: "GoLand", BundlePath: "/Applications/GoLand.app", Processes: []collector.Process{
			{PID: 10, UID: 501, Exe: "/Applications/GoLand.app/Contents/MacOS/goland"},
			{PID: 11, UID: 501, Exe: "/Users/alex/go/bin/gopls"},
		}},
		{Name: "yes", Processes: []collector.Process{{PID: 20, UID: 501, Exe: "/usr/bin/yes"}}},
		{Name: "launchd", Processes: []collector.Process{{PID: 1, UID: 0, Exe: "/sbin/launchd"}}},
		{Name: "ghost", Processes: []collector.Process{{PID: 30, UID: 501}, {PID: 31, UID: 501, Exe: "-R"}}},
	}
	tests := []struct {
		name string
		pid  int32
		want string
	}{
		{name: "a process of an app shows the bundle", pid: 10, want: "/Applications/GoLand.app"},
		{name: "a helper outside the bundle shows its app too", pid: 11, want: "/Applications/GoLand.app"},
		{name: "a process without a bundle shows its executable", pid: 20, want: "/usr/bin/yes"},
		{name: "another user's process", pid: 1},
		{name: "a pid the sample does not have", pid: 77},
		{name: "a process whose path is unknown", pid: 30},
		{name: "a path that open would read as a flag", pid: 31},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, RevealPath(apps, tt.pid, self))
		})
	}
}

func TestInfoTarget(t *testing.T) {
	t.Parallel()

	self := Self{UID: 501, PID: 900}
	helper := "/Applications/Google Chrome.app/Contents/Frameworks/Helpers/Google Chrome Helper (GPU).app"
	apps := []collector.App{
		{Name: "Google Chrome", BundlePath: "/Applications/Google Chrome.app", PIDs: []int32{41, 40}, Processes: []collector.Process{
			{PID: 41, PPID: 40, UID: 501, Name: "Google Chrome Helper (GPU)", Exe: helper + "/Contents/MacOS/Google Chrome Helper (GPU)"},
			{PID: 40, PPID: 1, UID: 501, Name: "Google Chrome", Exe: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		}},
		{Name: "iTerm", BundlePath: "/Applications/iTerm.app", PIDs: []int32{50, 51, 52}, Processes: []collector.Process{
			{PID: 50, PPID: 1, UID: 501, Name: "iTerm2", Exe: "/Applications/iTerm.app/Contents/MacOS/iTerm2"},
			{PID: 51, PPID: 50, UID: 501, Name: "zsh", Exe: "/bin/zsh", TTY: "ttys001"},
			{PID: 52, PPID: 51, UID: 501, Name: "htop", Exe: "/opt/homebrew/bin/htop", TTY: "ttys001"},
		}},
		{Name: "WindowServer", PIDs: []int32{382}, Processes: []collector.Process{
			{PID: 382, PPID: 1, UID: 88, Name: "WindowServer", Exe: "/System/Library/PrivateFrameworks/SkyLight.framework/Resources/WindowServer"},
		}},
		{Name: "launchd", PIDs: []int32{1}, Processes: []collector.Process{{PID: 1, UID: 0, Name: "launchd", Exe: "/sbin/launchd"}}},
		{Name: "kernel_task", PIDs: []int32{0}, Processes: []collector.Process{{PID: 0, UID: 0, Name: "kernel_task"}}},
		{Name: "gpj.exe\u202e", PIDs: []int32{11}, Processes: []collector.Process{
			{PID: 11, PPID: 999, UID: 501, Name: "gpj.exe\u202e", Exe: "/tmp/gpj.exe\u202e"},
		}},
		{Name: "ghost"},
	}
	tests := []struct {
		name string
		app  string
		pid  int32
		want AppInfo
		// wantPath is the path Go reads, when it is not the one shown.
		wantPath string
		wantPID  int32
		wantOK   bool
	}{
		{
			name: "an app is its bundle and its root process", app: "Google Chrome", wantPID: 40, wantOK: true,
			want: AppInfo{Name: "Google Chrome", Title: "Google Chrome", Path: "/Applications/Google Chrome.app", Parent: "launchd", Killable: true},
		},
		{
			name: "a helper is a bundle of its own", app: "Google Chrome", pid: 41, wantPID: 41, wantOK: true,
			want: AppInfo{
				Name: "Google Chrome", PID: 41, Title: "Google Chrome Helper (GPU)", Parent: "Google Chrome", Killable: true,
				Path: helper,
			},
		},
		{
			name: "a tool run in a terminal is its executable, started by the shell of that app", app: "iTerm", pid: 52, wantPID: 52, wantOK: true,
			want: AppInfo{Name: "iTerm", PID: 52, Title: "htop", Path: "/opt/homebrew/bin/htop", Parent: "zsh (iTerm)", Killable: true},
		},
		{
			name: "a system process the dictionaries describe", app: "WindowServer", wantPID: 382, wantOK: true,
			want: AppInfo{
				Name: "WindowServer", Title: "WindowServer", DescKey: "proc.WindowServer", Parent: "launchd",
				Path: "/System/Library/PrivateFrameworks/SkyLight.framework/Resources/WindowServer",
			},
		},
		{
			name: "the kernel has no file and no parent", app: "kernel_task", wantOK: true,
			want: AppInfo{Name: "kernel_task", Title: "kernel_task", DescKey: "proc.kernel_task", Signing: collector.SignApple},
		},
		{
			name: "an app is matched by the name the state showed, and a parent that is gone is left out; the path shown loses its bidi controls",
			app:  "gpj.exe", wantPID: 11, wantOK: true, wantPath: "/tmp/gpj.exe\u202e",
			want: AppInfo{Name: "gpj.exe", Title: "gpj.exe", Path: "/tmp/gpj.exe", Killable: true},
		},
		{name: "a pid of another app", app: "iTerm", pid: 40},
		{name: "a pid that is gone", app: "iTerm", pid: 4242},
		{name: "an app that is gone", app: "curl"},
		{name: "an app without processes", app: "ghost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info, target, path, ok := InfoTarget(apps, Message{Type: "app_info", Name: tt.app, PID: tt.pid}, self)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, info)
			assert.Equal(t, cmp.Or(tt.wantPath, tt.want.Path), path)
			assert.Equal(t, tt.wantPID, target.PID)
		})
	}
}

func TestDetailTarget(t *testing.T) {
	t.Parallel()

	self := Self{UID: 501, PID: 900}
	apps := []collector.App{
		{Name: "iTerm", PIDs: []int32{50, 51}, Processes: []collector.Process{
			{PID: 50, PPID: 1, UID: 501, Name: "iTerm2"}, {PID: 51, PPID: 50, UID: 501, Name: "zsh"},
		}},
		{Name: "sudo", PIDs: []int32{60, 61}, Processes: []collector.Process{
			{PID: 60, PPID: 51, UID: 501, Name: "sudo"}, {PID: 61, PPID: 60, UID: 0, Name: "htop"},
		}},
		{Name: "launchd", PIDs: []int32{1}, Processes: []collector.Process{{PID: 1, UID: 0, Name: "launchd"}}},
	}
	tests := []struct {
		name   string
		app    string
		pid    int32
		wantOK bool
	}{
		{name: "a process of this user", app: "iTerm", pid: 51, wantOK: true},
		{name: "another user's process", app: "launchd", pid: 1},
		{name: "another user's process inside an app of this user", app: "sudo", pid: 61},
		{name: "a pid of another app", app: "iTerm", pid: 60},
		{name: "a pid that is gone", app: "iTerm", pid: 4242},
		{name: "an app that is gone", app: "curl", pid: 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target, ok := DetailTarget(apps, Message{Type: "proc_detail", Name: tt.app, PID: tt.pid}, self)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.pid, target.PID)
			}
		})
	}
}

func TestBrowserRunning(t *testing.T) {
	t.Parallel()

	apps := []collector.App{
		{Name: "Safari", Processes: []collector.Process{{PID: 70}}},
		{Name: "Google Chrome"},
	}
	tests := []struct {
		name string
		app  string
		want bool
	}{
		{name: "a browser of the last sample", app: "Safari", want: true},
		{name: "a browser that is not running", app: "Firefox"},
		{name: "a browser whose processes are gone", app: "Google Chrome"},
		{name: "no name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, BrowserRunning(apps, tt.app))
		})
	}
}

func TestEjectVolume(t *testing.T) {
	t.Parallel()

	volumes := []collector.Volume{
		{Name: "Macintosh HD", Mount: "/"},
		{Name: "T7", Mount: "/Volumes/T7", Ejectable: true},
		{Name: "gpj\u202e", Mount: "/Volumes/gpj\u202e", Ejectable: true},
	}
	tests := []struct {
		name  string
		mount string
		want  collector.Volume
	}{
		{name: "an ejectable volume of the sample", mount: "/Volumes/T7", want: volumes[1]},
		{name: "a volume is matched by the mount the state showed, bidi removed", mount: "/Volumes/gpj", want: volumes[2]},
		{name: "the system volume is never ejected", mount: "/"},
		{name: "a volume that left since the sample", mount: "/Volumes/Gone"},
		{name: "a path that only resolves to a volume", mount: "/Volumes/../Volumes/T7"},
		{name: "no mount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, EjectVolume(volumes, tt.mount))
		})
	}
}

func TestBuildProcDetail(t *testing.T) {
	t.Parallel()

	apps := []collector.App{
		{Name: "iTerm", Processes: []collector.Process{
			{PID: 50, PPID: 1, Name: "iTerm2"}, {PID: 51, PPID: 50, Name: "zsh\u202e"}, {PID: 52, PPID: 51, Name: "htop"},
		}},
		{Name: "launchd", Processes: []collector.Process{{PID: 1, Name: "launchd"}}},
		{Name: "loop", Processes: []collector.Process{{PID: 60, PPID: 61, Name: "a"}, {PID: 61, PPID: 60, Name: "b"}}},
	}
	read := collector.ProcDetail{
		Threads: 3, Files: []string{"/dev/null"}, Sockets: []string{"IPv4 127.0.0.1:8799"}, More: 2, Args: []string{"htop"},
	}
	tests := []struct {
		name      string
		target    collector.Process
		wantChain []string
	}{
		{name: "parents, nearest first, up to launchd, cleaned", target: apps[0].Processes[2], wantChain: []string{"zsh", "iTerm2", "launchd"}},
		{name: "a parent the sample does not hold ends the chain", target: collector.Process{PID: 70, PPID: 4242}, wantChain: []string{}},
		{name: "launchd has no parent", target: apps[1].Processes[0], wantChain: []string{}},
		{name: "a parent cycle ends at the cap", target: apps[2].Processes[0], wantChain: slices.Repeat([]string{"b", "a"}, maxChain/2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildProcDetail(apps, tt.target, read)

			assert.Equal(t, &ProcDetail{
				Threads: 3, Chain: tt.wantChain, Files: read.Files, Sockets: read.Sockets, More: 2, Args: read.Args,
			}, got)
		})
	}
}
