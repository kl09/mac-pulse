package collector

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/native"
)

func TestAppOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pid      int32
		wantName bool
	}{
		// Who owns the test binary depends on what started go test: a shell, an IDE, CI.
		{name: "own pid has an owner", pid: int32(os.Getpid()), wantName: true},
		{name: "unknown pid gives nothing", pid: math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name, _ := AppOf(t.Context(), tt.pid)

			assert.Equal(t, tt.wantName, name != "")
		})
	}
}

func TestParsePS(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/ps.txt")
	require.NoError(t, err)
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	gpu := "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/140.0.0.0/Helpers/" +
		"Google Chrome Helper (GPU).app/Contents/MacOS/Google Chrome Helper (GPU)"

	tests := []struct {
		name  string
		input string
		want  []Process
	}{
		{
			name:  "captured fixture: root processes carry time and rss, ?? is no tty, comm keeps its spaces, garbage is skipped",
			input: string(fixture),
			want: []Process{
				{PID: 1, PPID: 0, UID: 0, Name: "launchd", Exe: "/sbin/launchd", CPUTime: 86*60 + 46.87, RSS: 17920 * 1024},
				{PID: 312, PPID: 1, UID: 0, Name: "logd", Exe: "/usr/libexec/logd", CPUTime: 85*60 + 51.98, RSS: 38912 * 1024},
				{
					PID: 382, PPID: 1, UID: 88, Name: "WindowServer", CPUTime: 1461*60 + 58.18, RSS: 98128 * 1024,
					Exe: "/System/Library/PrivateFrameworks/SkyLight.framework/Resources/WindowServer",
				},
				{PID: 4711, PPID: 712, UID: 501, Name: "Google Chrome", Exe: chrome, CPUTime: 3.25, RSS: 214112 * 1024},
				{PID: 4712, PPID: 4711, UID: 501, Name: "Google Chrome Helper (GPU)", Exe: gpu, CPUTime: 86400 + 2*3600 + 3*60 + 4, RSS: 99999 * 1024},
				{PID: 8000, PPID: 1, UID: 501, Name: "two colons", Exe: "/usr/local/bin/two colons", CPUTime: 12*3600 + 34*60 + 56, RSS: 1024 * 1024},
				{PID: 9000, PPID: 8000, UID: 501, TTY: "ttys003", Name: "yes", Exe: "/usr/bin/yes", CPUTime: 1.5, RSS: 512 * 1024},
			},
		},
		{name: "empty input", input: ""},
		{name: "line without comm is skipped", input: "  5  1  0  ??  0:00.10  100\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, parsePS(strings.NewReader(tt.input)))
		})
	}
}

func TestParseCPUTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want float64
	}{
		{name: "seconds", in: "1.50", want: 1.5},
		{name: "minutes and seconds", in: "86:46.87", want: 86*60 + 46.87},
		{name: "hours", in: "34:27:29", want: 34*3600 + 27*60 + 29},
		{name: "days", in: "1-02:03:04", want: 86400 + 2*3600 + 3*60 + 4},
		{name: "garbage", in: "n/a", want: 0},
		{name: "empty", in: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, parseCPUTime(tt.in), 1e-9)
		})
	}
}

func TestParseTop(t *testing.T) {
	t.Parallel()

	header := "Processes: 600 total\n\nPID TIME     MEM\n"
	tests := []struct {
		name    string
		input   string
		want    Process
		wantErr string
	}{
		{
			name: "hours and megabytes", input: header + "0   34:27:29 39M\n",
			want: Process{Name: "kernel_task", CPUTime: 34*3600 + 27*60 + 29, RSS: 39 << 20},
		},
		{name: "growing kilobytes", input: header + "0   00:01.23 512K+\n", want: Process{Name: "kernel_task", CPUTime: 1.23, RSS: 512 << 10}},
		{name: "no unit", input: header + "0   00:01.23 7\n", want: Process{Name: "kernel_task", CPUTime: 1.23, RSS: 7}},
		{name: "no row", input: header, wantErr: "no row for pid 0"},
		{name: "unreadable memory", input: header + "0   00:01.23 lots\n", wantErr: "kernel_task mem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTop(strings.NewReader(tt.input))

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestGroupByApp(t *testing.T) {
	t.Parallel()

	foo := Process{PID: 10, PPID: 1, Name: "Foo", Exe: "/Applications/Foo.app/Contents/MacOS/Foo", CPU: 5, RSS: 100}
	helper := Process{PID: 11, PPID: 10, Name: "Foo Helper", Exe: "/Applications/Foo.app/Contents/Helpers/Foo Helper", CPU: 2, RSS: 50}
	grandchild := Process{PID: 12, PPID: 11, Name: "python3", Exe: "/usr/bin/python3", CPU: 7, RSS: 10}
	orphan := Process{PID: 20, PPID: 1, Name: "sshd", Exe: "/usr/sbin/sshd", CPU: 0.5, RSS: 20}
	terminal := Process{PID: 30, PPID: 1, Name: "Terminal", Exe: "/Applications/Terminal.app/Contents/MacOS/Terminal", CPU: 1, RSS: 10}
	zsh := Process{PID: 31, PPID: 30, Name: "zsh", Exe: "/bin/zsh", CPU: 0, RSS: 5}
	curl := Process{PID: 32, PPID: 31, Name: "curl", Exe: "/usr/bin/curl", CPU: 3, RSS: 40}
	bar := Process{
		PID: 50, PPID: 1, Name: "Bar", Exe: "/Applications/Bar.app/Contents/MacOS/Bar",
		HasUsage: true, DiskReadRate: 10, DiskWriteRate: 100, EnergyMW: 7,
	}
	barHelper := Process{
		PID: 51, PPID: 50, Name: "helper", Exe: "/Applications/Bar.app/Contents/MacOS/helper",
		HasUsage: true, DiskReadRate: 1, DiskWriteRate: 2, EnergyMW: 3,
	}
	goland := Process{PID: 60, PPID: 1, Name: "goland", Exe: "/Applications/GoLand.app/Contents/MacOS/goland"}
	junie := Process{PID: 61, PPID: 60, Name: "junie", Exe: "/Users/alex/Library/Caches/JetBrains/junie.app/Contents/MacOS/junie"}
	copilot := Process{PID: 62, PPID: 60, Name: "copilot-language-server", Exe: "/Users/alex/Library/copilot-language-server"}
	ideShell := Process{PID: 63, PPID: 60, TTY: "ttys000", Name: "zsh", Exe: "/bin/zsh"}
	promptShell := Process{PID: 64, PPID: 1, TTY: "ttys000", Name: "zsh", Exe: "/bin/zsh"}
	gitstatusd := Process{
		PID: 65, PPID: 64, TTY: "ttys000", Name: "gitstatusd-darwin-arm64", Exe: "/Users/alex/.cache/gitstatus/gitstatusd-darwin-arm64",
	}
	codex := Process{PID: 66, PPID: 63, TTY: "ttys000", Name: "codex", Exe: "/Users/alex/.local/bin/codex"}
	gopls := Process{PID: 67, PPID: 66, TTY: "ttys000", Name: "gopls", Exe: "/Users/alex/go/bin/gopls"}
	toolShell := Process{PID: 68, PPID: 66, Name: "zsh", Exe: "/bin/zsh"}
	caffeinate := Process{PID: 69, PPID: 68, Name: "caffeinate", Exe: "/usr/bin/caffeinate"}
	chrome := Process{PID: 70, PPID: 1, Name: "Google Chrome", Exe: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	chromeCodex := Process{PID: 71, PPID: 70, Name: "codex", Exe: "/Users/alex/.local/bin/codex"}
	login := Process{PID: 33, PPID: 30, TTY: "ttys001", Name: "login", Exe: "/usr/bin/login"}
	loginShell := Process{PID: 34, PPID: 33, TTY: "ttys001", Name: "-zsh", Exe: "/bin/zsh"}
	loginCurl := Process{PID: 35, PPID: 34, TTY: "ttys001", Name: "curl", Exe: "/usr/bin/curl", CPU: 3, RSS: 40}
	agent := Process{PID: 80, PPID: 1, Name: "python3", Exe: "/opt/homebrew/bin/python3"}
	const safariApp = "/System/Volumes/Preboot/Cryptexes/App/System/Applications/Safari.app"
	safari := Process{PID: 90, PPID: 1, Name: "Safari", Exe: safariApp + "/Contents/MacOS/Safari"}
	webContent := Process{
		PID: 91, PPID: 1, Responsible: 90, Name: "com.apple.WebKit.WebContent",
		Exe: "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.WebContent.xpc" +
			"/Contents/MacOS/com.apple.WebKit.WebContent",
	}
	mailContent := webContent
	mailContent.PID, mailContent.Responsible = 92, 0
	barRoot := Process{PID: 52, PPID: 50, Name: "root-helper", Exe: "/Applications/Bar.app/Contents/MacOS/root-helper"}

	tests := []struct {
		name  string
		procs []Process
		want  []App
	}{
		{
			name:  "helper and grandchild fold into the bundle, sorted by cpu",
			procs: []Process{orphan, foo, helper, grandchild},
			want: []App{
				{
					Name: "Foo", BundlePath: "/Applications/Foo.app", PIDs: []int32{10, 11, 12}, CPU: 14, RSS: 160,
					Processes: []Process{grandchild, foo, helper},
				},
				{Name: "sshd", PIDs: []int32{20}, CPU: 0.5, RSS: 20, Processes: []Process{orphan}},
			},
		},
		{
			name:  "orphan with ppid 1 keeps its own name",
			procs: []Process{orphan},
			want:  []App{{Name: "sshd", PIDs: []int32{20}, CPU: 0.5, RSS: 20, Processes: []Process{orphan}}},
		},
		{
			name:  "child of a non-bundle parent keeps its own name",
			procs: []Process{orphan, {PID: 21, PPID: 20, Name: "zsh", Exe: "/bin/zsh", CPU: 1, RSS: 5}},
			want: []App{
				{Name: "zsh", PIDs: []int32{21}, CPU: 1, RSS: 5, Processes: []Process{
					{PID: 21, PPID: 20, Name: "zsh", Exe: "/bin/zsh", CPU: 1, RSS: 5},
				}},
				{Name: "sshd", PIDs: []int32{20}, CPU: 0.5, RSS: 20, Processes: []Process{orphan}},
			},
		},
		{
			name:  "cli child of shell keeps its own name",
			procs: []Process{terminal, zsh, curl},
			want: []App{
				{Name: "curl", PIDs: []int32{32}, CPU: 3, RSS: 40, Processes: []Process{curl}},
				{
					Name: "Terminal", BundlePath: "/Applications/Terminal.app", PIDs: []int32{30, 31}, CPU: 1, RSS: 15,
					Processes: []Process{terminal, zsh},
				},
			},
		},
		{
			name:  "equal cpu sorts by rss desc before name",
			procs: []Process{orphan, {PID: 22, PPID: 1, Name: "zzz", Exe: "/usr/bin/zzz", CPU: 0.5, RSS: 40}},
			want: []App{
				{Name: "zzz", PIDs: []int32{22}, CPU: 0.5, RSS: 40, Processes: []Process{
					{PID: 22, PPID: 1, Name: "zzz", Exe: "/usr/bin/zzz", CPU: 0.5, RSS: 40},
				}},
				{Name: "sshd", PIDs: []int32{20}, CPU: 0.5, RSS: 20, Processes: []Process{orphan}},
			},
		},
		{
			name: "bare binary named like a bundle joins it and the group keeps the bundle path",
			procs: []Process{
				{PID: 40, PPID: 1, Name: "Foo", Exe: "/usr/local/bin/Foo", CPU: 9, RSS: 1},
				foo,
			},
			want: []App{{
				Name: "Foo", BundlePath: "/Applications/Foo.app", PIDs: []int32{40, 10}, CPU: 14, RSS: 101,
				Processes: []Process{{PID: 40, PPID: 1, Name: "Foo", Exe: "/usr/local/bin/Foo", CPU: 9, RSS: 1}, foo},
			}},
		},
		{
			name:  "usage sums over the processes that have it",
			procs: []Process{bar, barHelper, barRoot},
			want: []App{{
				Name: "Bar", BundlePath: "/Applications/Bar.app", PIDs: []int32{50, 51, 52},
				HasUsage: true, DiskReadRate: 11, DiskWriteRate: 102, EnergyMW: 10,
				Processes: []Process{bar, barHelper, barRoot},
			}},
		},
		{
			name:  "shells, the prompt's orphaned shell and gitstatusd fold into the terminal app on their tty",
			procs: []Process{goland, ideShell, promptShell, gitstatusd},
			want: []App{{
				Name: "GoLand", BundlePath: "/Applications/GoLand.app", PIDs: []int32{60, 63, 64, 65},
				Processes: []Process{goland, ideShell, promptShell, gitstatusd},
			}},
		},
		{
			name:  "login and a login shell fold into the terminal, the typed command does not",
			procs: []Process{terminal, login, loginShell, loginCurl},
			want: []App{
				{Name: "curl", PIDs: []int32{35}, CPU: 3, RSS: 40, Processes: []Process{loginCurl}},
				{
					Name: "Terminal", BundlePath: "/Applications/Terminal.app", PIDs: []int32{30, 33, 34}, CPU: 1, RSS: 10,
					Processes: []Process{terminal, login, loginShell},
				},
			},
		},
		{
			name:  "orphaned shell without a terminal on its tty keeps its own name",
			procs: []Process{promptShell, gitstatusd},
			want: []App{
				{Name: "gitstatusd-darwin-arm64", PIDs: []int32{65}, Processes: []Process{gitstatusd}},
				{Name: "zsh", PIDs: []int32{64}, Processes: []Process{promptShell}},
			},
		},
		{
			name:  "app's helper outside its bundle or in a bundle of its own belongs to the app",
			procs: []Process{goland, junie, copilot},
			want: []App{{
				Name: "GoLand", BundlePath: "/Applications/GoLand.app", PIDs: []int32{60, 61, 62},
				Processes: []Process{goland, junie, copilot},
			}},
		},
		{
			name:  "typed command owns what it starts, through its own shells too",
			procs: []Process{goland, ideShell, codex, gopls, toolShell, caffeinate},
			want: []App{
				{Name: "GoLand", BundlePath: "/Applications/GoLand.app", PIDs: []int32{60, 63}, Processes: []Process{goland, ideShell}},
				{Name: "codex", PIDs: []int32{66, 67, 68, 69}, Processes: []Process{codex, gopls, toolShell, caffeinate}},
			},
		},
		{
			name:  "executable an app started joins the group of the same command typed in a shell",
			procs: []Process{goland, ideShell, codex, chrome, chromeCodex},
			want: []App{
				{Name: "GoLand", BundlePath: "/Applications/GoLand.app", PIDs: []int32{60, 63}, Processes: []Process{goland, ideShell}},
				{Name: "Google Chrome", BundlePath: "/Applications/Google Chrome.app", PIDs: []int32{70}, Processes: []Process{chrome}},
				{Name: "codex", PIDs: []int32{66, 71}, Processes: []Process{codex, chromeCodex}},
			},
		},
		{
			name:  "executable an app started stays with the app when nobody typed it",
			procs: []Process{chrome, chromeCodex},
			want: []App{{
				Name: "Google Chrome", BundlePath: "/Applications/Google Chrome.app", PIDs: []int32{70, 71},
				Processes: []Process{chrome, chromeCodex},
			}},
		},
		{
			name:  "daemon outside any bundle is nobody's typed command: an app's child of its name stays with the app",
			procs: []Process{agent, foo, helper, grandchild},
			want: []App{
				{
					Name: "Foo", BundlePath: "/Applications/Foo.app", PIDs: []int32{10, 11, 12}, CPU: 14, RSS: 160,
					Processes: []Process{grandchild, foo, helper},
				},
				{Name: "python3", PIDs: []int32{80}, Processes: []Process{agent}},
			},
		},
		{
			name:  "helper launchd started for Safari follows its responsible process, another app's helper keeps its own name",
			procs: []Process{safari, webContent, mailContent},
			want: []App{
				{
					Name: "Safari", BundlePath: safariApp, PIDs: []int32{90, 91}, Processes: []Process{safari, webContent},
				},
				{Name: "com.apple.WebKit.WebContent", PIDs: []int32{92}, Processes: []Process{mailContent}},
			},
		},
		{name: "empty list", procs: nil, want: []App{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, groupByApp(tt.procs))
		})
	}
}

func TestUsageRates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prev       native.Usage
		cur        native.Usage
		dt         float64
		wantRead   float64
		wantWrite  float64
		wantEnergy float64
	}{
		{
			name: "deltas over the window",
			prev: native.Usage{DiskRead: 1000, DiskWrite: 5000, EnergyNJ: 1e9},
			cur:  native.Usage{DiskRead: 4000, DiskWrite: 5600, EnergyNJ: 7e9},
			dt:   6, wantRead: 500, wantWrite: 100, wantEnergy: 1000,
		},
		{name: "idle process", prev: native.Usage{DiskRead: 10, EnergyNJ: 10}, cur: native.Usage{DiskRead: 10, EnergyNJ: 10}, dt: 6},
		{
			name: "reused pid: one counter went down, so none is trusted",
			prev: native.Usage{DiskRead: 9000, DiskWrite: 100, EnergyNJ: 5e9},
			cur:  native.Usage{DiskRead: 10, DiskWrite: 900, EnergyNJ: 6e9},
			dt:   6,
		},
		{name: "energy counter went down", prev: native.Usage{EnergyNJ: 5e9}, cur: native.Usage{DiskWrite: 600, EnergyNJ: 1e9}, dt: 6},
		{name: "no time between the scans", prev: native.Usage{}, cur: native.Usage{DiskRead: 600}, dt: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			read, write, energy := usageRates(tt.prev, tt.cur, tt.dt)

			assert.InDelta(t, tt.wantRead, read, 1e-9)
			assert.InDelta(t, tt.wantWrite, write, 1e-9)
			assert.InDelta(t, tt.wantEnergy, energy, 1e-9)
		})
	}
}
