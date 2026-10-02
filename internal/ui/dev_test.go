package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

//nolint:funlen // one table of whole-dev expectations
func TestBuildDev(t *testing.T) {
	t.Parallel()

	const home = "/Users/alex"
	since := time.UnixMilli(1790777400000)
	cpu, memory, pids, energy := 0.2, uint64(62914560), 7, 310.5
	procs := []collector.Process{
		{PID: 10, CPU: 12.34, RSS: 100},
		{PID: 11, CPU: 1, RSS: 20},
		{PID: 12, CPU: 40, RSS: 300},
	}
	tests := []struct {
		name      string
		snap      collector.Snapshot
		listening []netinspect.Listener
		docker    collector.DockerReport
		want      Dev
	}{
		{
			name:   "docker is not installed and nothing listens: no null slice",
			docker: collector.DockerReport{Status: collector.DockerMissing},
			want:   Dev{Docker: "missing", Containers: []DevContainer{}, Projects: []DevProject{}, Agents: []DevAgent{}},
		},
		{
			name: "listeners group by directory, busiest project first",
			snap: collector.Snapshot{Processes: procs},
			listening: []netinspect.Listener{
				{App: "node", PID: 10, Port: "5173", Dir: "~/src/shop"},
				{App: "node", PID: 10, Port: "3000", Dir: "~/src/shop"},
				{App: "node", PID: 10, Port: "3000", Dir: "~/src/shop"},
				{App: "go", PID: 11, Port: "8080", Dir: "~/src/shop"},
				{App: "python3", PID: 12, Port: "8765", Dir: "/tmp/proj"},
			},
			docker: collector.DockerReport{Status: collector.DockerOK},
			want: Dev{
				Docker: "ok", Containers: []DevContainer{}, Agents: []DevAgent{},
				Projects: []DevProject{
					{Name: "proj", Dir: "/tmp/proj", Ports: []int{8765}, Apps: []string{"python3"}, PIDs: []int32{12}, CPU: 40, Memory: 300},
					{
						Name: "shop", Dir: "~/src/shop", Ports: []int{3000, 5173, 8080}, Apps: []string{"node", "go"},
						PIDs: []int32{10, 11}, CPU: 13.3, Memory: 120,
					},
				},
			},
		},
		{
			name: "a GUI app, another user's process, a tool left in the home and an app's own helpers are not projects",
			listening: []netinspect.Listener{
				{App: "Dropbox", PID: 10, Port: "17500"},
				{App: "python3", PID: 12, Port: "8000", Dir: "~"},
				{App: "Docker", PID: 10, Port: "5432", Dir: "~/Library/Containers/com.docker.docker/Data"},
				{App: "GoLand", PID: 11, Port: "30000", Dir: "/Applications/GoLand.app/Contents/plugins/cef_server.app/Contents/MacOS"},
				{App: "Tool", PID: 11, Port: "30001", Dir: "/Applications/Tool.app"},
			},
			docker: collector.DockerReport{Status: collector.DockerStopped},
			want:   Dev{Docker: "stopped", Containers: []DevContainer{}, Projects: []DevProject{}, Agents: []DevAgent{}},
		},
		{
			name:      "a compose container joins the project of its working directory; stats are null until read",
			listening: []netinspect.Listener{{App: "go", PID: 11, Port: "*", Dir: "~/src/shop"}},
			docker: collector.DockerReport{Status: collector.DockerOK, Containers: []collector.Container{
				{
					ID: "a1b2c3d4e5f6", Name: "shop-postgres-1", Image: "postgres:17-alpine", State: "running", Status: "Up 7 days",
					Ports: "0.0.0.0:5432->5432/tcp", Project: "shop", Dir: "/Users/alex/src/shop", HasStats: true, CPU: 0.2, Memory: 62914560, PIDs: 7,
				},
				{ID: "0123456789ab", Name: "redis", Image: "redis:8", State: "running", Status: "Up 2 hours"},
				{ID: "ba9876543210", Name: "other-db-1", Project: "other", Dir: "/Users/alex/src/other"},
			}},
			want: Dev{
				Docker: "ok", Agents: []DevAgent{},
				Containers: []DevContainer{
					{
						ID: "a1b2c3d4e5f6", Name: "shop-postgres-1", Image: "postgres:17-alpine", State: "running", Status: "Up 7 days",
						Ports: "0.0.0.0:5432->5432/tcp", Project: "shop", Dir: "~/src/shop", CPU: &cpu, Memory: &memory, PIDs: &pids,
					},
					{ID: "0123456789ab", Name: "redis", Image: "redis:8", State: "running", Status: "Up 2 hours"},
					{ID: "ba9876543210", Name: "other-db-1", Project: "other", Dir: "~/src/other"},
				},
				Projects: []DevProject{
					{Name: "shop", Dir: "~/src/shop", Ports: []int{}, Apps: []string{"go"}, PIDs: []int32{11}, Containers: 1},
				},
			},
		},
		{
			name: "agents keep the sampler's order; energy is null without counters; bidi controls are dropped",
			snap: collector.Snapshot{Agents: []collector.Agent{
				{Name: "codex", PID: 40, Dir: "~/src/\u202eshop", Since: since, Procs: 5, CPU: 31.26, RSS: 900, EnergyMW: 310.52, HasUsage: true},
				{Name: "codex", PID: 50, Dir: "~/src/api", Since: since, Procs: 1, RSS: 10},
			}},
			docker: collector.DockerReport{Status: collector.DockerOK},
			want: Dev{
				Docker: "ok", Containers: []DevContainer{}, Projects: []DevProject{},
				Agents: []DevAgent{
					{Name: "codex", PID: 40, Dir: "~/src/shop", Since: since.UnixMilli(), Procs: 5, CPU: 31.3, Memory: 900, EnergyMW: &energy},
					{Name: "codex", PID: 50, Dir: "~/src/api", Since: since.UnixMilli(), Procs: 1, Memory: 10},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildDev(&tt.snap, tt.listening, &tt.docker, home)

			assert.Equal(t, tt.want, *got)
		})
	}
}
