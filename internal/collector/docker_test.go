package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockerPS(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/docker_ps.txt")
	require.NoError(t, err)
	postgres := Container{
		ID: "a1b2c3d4e5f6", Name: "shop-postgres-1", Image: "postgres:17-alpine", State: "running", Status: "Up 7 days (healthy)",
		Ports: "0.0.0.0:5432->5432/tcp", Project: "shop", Dir: "/Users/alex/src/shop",
	}

	tests := []struct {
		name    string
		input   string
		want    []Container
		wantErr bool
	}{
		{name: "captured fixture: a compose container", input: string(fixture), want: []Container{postgres}},
		{
			name: "a container outside compose has no project, and a blank line is skipped",
			input: string(fixture) + "\n" +
				`{"id":"0f1e2d3c4b5a","name":"web","image":"nginx","state":"running","status":"Up 2 minutes","ports":"","project":"","dir":""}` + "\n",
			want: []Container{postgres, {ID: "0f1e2d3c4b5a", Name: "web", Image: "nginx", State: "running", Status: "Up 2 minutes"}},
		},
		{name: "no containers", input: "", want: []Container{}},
		{name: "output that is not the format", input: "WARNING: something changed\n", wantErr: true},
		{name: "a line without an id", input: `{"name":"web"}` + "\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDockerPS(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDockerStats(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/docker_stats.txt")
	require.NoError(t, err)
	line := func(cpu, mem, pids string) string {
		return `{"BlockIO":"0B / 0B","CPUPerc":"` + cpu + `","Container":"aaa","ID":"aaa","MemPerc":"1%","MemUsage":"` + mem +
			`","Name":"x","NetIO":"0B / 0B","PIDs":"` + pids + `"}` + "\n"
	}

	tests := []struct {
		name  string
		input string
		want  map[string]Container
	}{
		{
			name: "captured fixture", input: string(fixture),
			want: map[string]Container{"a1b2c3d4e5f6": {CPU: 0.9, Memory: 62946017, PIDs: 7}},
		},
		{
			name:  "MiB, and CPU above one core",
			input: line("0.16%", "60MiB / 7.653GiB", "7") + strings.ReplaceAll(line("212.5%", "1.5GiB / 8GiB", "40"), "aaa", "bbb"),
			want:  map[string]Container{"aaa": {CPU: 0.16, Memory: 62914560, PIDs: 7}, "bbb": {CPU: 212.5, Memory: 1610612736, PIDs: 40}},
		},
		{name: "KiB", input: line("0.00%", "512KiB / 1GiB", "1"), want: map[string]Container{"aaa": {Memory: 524288, PIDs: 1}}},
		{name: "bytes", input: line("0.00%", "0B / 0B", "0"), want: map[string]Container{"aaa": {}}},
		{name: "an unknown unit leaves the container without stats", input: line("1%", "5kB / 1GiB", "1"), want: map[string]Container{}},
		{name: "dashes for a container that just stopped", input: line("--", "-- / --", "--"), want: map[string]Container{}},
		{name: "NaN CPU", input: line("NaN%", "60MiB / 1GiB", "1"), want: map[string]Container{}},
		{name: "infinite memory", input: line("1%", "InfMiB / 1GiB", "1"), want: map[string]Container{}},
		{name: "negative CPU", input: line("-1%", "60MiB / 1GiB", "1"), want: map[string]Container{}},
		{name: "not JSON", input: "CONTAINER ID   NAME\n", want: map[string]Container{}},
		{name: "empty output", input: "", want: map[string]Container{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, parseDockerStats(strings.NewReader(tt.input)))
		})
	}
}

func TestListContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cli  string
		want string
	}{
		{name: "no CLI installed", cli: "", want: DockerMissing},
		{name: "CLI that exits 1, as with the daemon down", cli: "/usr/bin/false", want: DockerStopped},
		{name: "CLI whose output is not the format", cli: "/bin/echo", want: DockerStopped},
		{name: "CLI that prints nothing and exits 0: no containers", cli: "/usr/bin/true", want: DockerOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Never nil: the state's arrays are not null.
			assert.Equal(t, &DockerReport{Status: tt.want, Containers: []Container{}}, listContainers(t.Context(), tt.cli))
		})
	}
}

func TestContextStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// script is the body of a stand-in CLI; "" stands for no CLI installed.
		script string
		want   string
	}{
		{name: "no CLI installed", want: DockerMissing},
		{name: "a unix socket is local", script: "echo unix:///Users/alex/.docker/run/docker.sock", want: DockerOK},
		{name: "an ssh host is remote", script: "echo ssh://alex@build.example.com", want: DockerRemote},
		{name: "a tcp host is remote, loopback included", script: "echo tcp://127.0.0.1:2375", want: DockerRemote},
		{name: "no host in the answer", script: "echo", want: DockerRemote},
		{name: "a CLI that fails is not polled", script: "exit 1", want: DockerStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cli string
			if tt.script != "" {
				cli = filepath.Join(t.TempDir(), "docker")
				require.NoError(t, os.WriteFile(cli, []byte("#!/bin/sh\n"+tt.script+"\n"), 0o700))
			}

			assert.Equal(t, tt.want, contextStatus(t.Context(), cli))
		})
	}
}

func TestDocker_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		interval    time.Duration
		cancelAfter time.Duration
		wantMin     int
	}{
		// The list, then list and stats at once, whatever the interval.
		{name: "two reports arrive without waiting out the interval", interval: time.Hour, cancelAfter: 8 * time.Second, wantMin: 2},
		{name: "cancelled before it starts reports nothing", interval: time.Hour, cancelAfter: -time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), tt.cancelAfter)
			t.Cleanup(cancel)
			var mu sync.Mutex
			var reports []*DockerReport
			docker := NewDocker(tt.interval, func(r *DockerReport) {
				mu.Lock()
				defer mu.Unlock()
				if reports = append(reports, r); len(reports) == 2 {
					cancel()
				}
			})

			docker.Run(ctx)

			assert.GreaterOrEqual(t, len(reports), tt.wantMin)
			if tt.wantMin == 0 {
				assert.Empty(t, reports)
			}
			for _, r := range reports {
				assert.Contains(t, []string{DockerOK, DockerMissing, DockerStopped, DockerRemote}, r.Status)
				assert.NotNil(t, r.Containers)
			}
		})
	}
}
