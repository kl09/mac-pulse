package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupAgents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		procs []Process
		want  []Agent
	}{
		{name: "no processes", want: []Agent{}},
		{
			name: "no agent among the processes",
			procs: []Process{
				{PID: 1, Name: "launchd"}, {PID: 50, PPID: 1, Name: "zsh"}, {PID: 51, PPID: 50, Name: "node"},
			},
			want: []Agent{},
		},
		{
			name: "a session is the agent and everything below it; an agent started inside does not open a second one",
			procs: []Process{
				{PID: 1, Name: "launchd"},
				{PID: 50, PPID: 1, Name: "zsh", CPU: 1, RSS: 1},
				{PID: 100, PPID: 50, Name: "codex", CPU: 10, RSS: 300, HasUsage: true, EnergyMW: 40},
				{PID: 101, PPID: 100, Name: "zsh", CPU: 0.5, RSS: 10},
				{PID: 102, PPID: 101, Name: "go", CPU: 90, RSS: 500, HasUsage: true, EnergyMW: 900},
				{PID: 103, PPID: 101, Name: "codex", CPU: 5, RSS: 200},
				{PID: 104, PPID: 103, Name: "rg", CPU: 2, RSS: 20},
			},
			want: []Agent{{Name: "codex", PID: 100, Procs: 5, CPU: 107.5, RSS: 1030, EnergyMW: 940, HasUsage: true}},
		},
		{
			name: "two sessions side by side, busiest first; equal CPU orders by pid",
			procs: []Process{
				{PID: 200, PPID: 50, Name: "codex", CPU: 3},
				{PID: 100, PPID: 50, Name: "codex", CPU: 3},
				{PID: 300, PPID: 50, Name: "aider", CPU: 1},
				{PID: 301, PPID: 300, Name: "git", CPU: 30},
			},
			want: []Agent{
				{Name: "aider", PID: 300, Procs: 2, CPU: 31},
				{Name: "codex", PID: 100, Procs: 1, CPU: 3},
				{Name: "codex", PID: 200, Procs: 1, CPU: 3},
			},
		},
		{
			name: "a child whose parent is gone from the scan stays out; a name that only contains an agent's is no agent",
			procs: []Process{
				{PID: 100, PPID: 1, Name: "gemini"},
				{PID: 400, PPID: 999, Name: "node"},
				{PID: 500, PPID: 1, Name: "codex-helper"},
			},
			want: []Agent{{Name: "gemini", PID: 100, Procs: 1}},
		},
		{
			name:  "a parent cycle ends",
			procs: []Process{{PID: 10, PPID: 11, Name: "sh"}, {PID: 11, PPID: 10, Name: "opencode"}},
			want:  []Agent{{Name: "opencode", PID: 11, Procs: 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, groupAgents(tt.procs))
		})
	}
}

func TestTildePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		home string
		dir  string
		want string
	}{
		{name: "directory under home", home: "/Users/alex", dir: "/Users/alex/src/site", want: "~/src/site"},
		{name: "home itself", home: "/Users/alex", dir: "/Users/alex", want: "~"},
		{name: "a sibling whose name starts like home", home: "/Users/alex", dir: "/Users/alexander/src", want: "/Users/alexander/src"},
		{name: "outside home", home: "/Users/alex", dir: "/tmp/x", want: "/tmp/x"},
		{name: "unknown home leaves the path alone", home: "", dir: "/Users/alex", want: "/Users/alex"},
		{name: "unknown directory", home: "/Users/alex", dir: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, TildePath(tt.home, tt.dir))
		})
	}
}
