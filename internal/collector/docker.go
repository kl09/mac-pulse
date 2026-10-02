package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The values of DockerReport.Status.
const (
	DockerOK = "ok"
	// DockerMissing: no docker CLI in any of the known install paths.
	DockerMissing = "missing"
	// DockerStopped: the CLI is there and the daemon does not answer.
	DockerStopped = "stopped"
	// DockerRemote: the CLI's context is not a local socket (ssh://, tcp://). Polling it would
	// open network connections nobody clicked for, so it is never asked.
	DockerRemote = "remote"
)

const (
	// `docker stats --no-stream` samples for 1.5–2 s before it prints.
	dockerStatsTimeout = 10 * time.Second
	// The labels are asked for by name: in `{{json .}}` they are k=v pairs joined by commas,
	// and compose values hold commas themselves.
	dockerPSFormat = `{"id":{{json .ID}},"name":{{json .Names}},"image":{{json .Image}},"state":{{json .State}},` +
		`"status":{{json .Status}},"ports":{{json .Ports}},"project":{{json (.Label "com.docker.compose.project")}},` +
		`"dir":{{json (.Label "com.docker.compose.project.working_dir")}}}`
)

type Container struct {
	// ID is the 12-character short id.
	ID     string
	Name   string
	Image  string
	State  string
	Status string
	Ports  string
	// Project and Dir are the compose project and its working directory, "" outside compose.
	Project string
	Dir     string
	// HasStats is false until `docker stats` has answered for this container.
	HasStats bool
	// CPU is percent of one core.
	CPU    float64
	Memory uint64
	PIDs   int
}

type DockerReport struct {
	Status     string
	Containers []Container
}

// Docker polls the docker CLI on its own loop: `docker stats` takes over a second, which
// a sampler tick cannot wait for.
type Docker struct {
	interval time.Duration
	onReport func(*DockerReport)
}

func NewDocker(interval time.Duration, onReport func(*DockerReport)) *Docker {
	return &Docker{interval: interval, onReport: onReport}
}

// Run reports until ctx is cancelled: the list alone at once, its numbers as soon as
// `docker stats` answers, then both every interval.
func (d *Docker) Run(ctx context.Context) {
	cli := dockerCLI()
	rep := &DockerReport{Status: contextStatus(ctx, cli), Containers: []Container{}}
	local := rep.Status == DockerOK
	if local {
		rep = listContainers(ctx, cli)
	}
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for first := true; ; first = false {
		// A result that arrives during shutdown is dropped: its consumer is already gone.
		if ctx.Err() != nil {
			return
		}
		d.onReport(rep)
		if !first {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		if local {
			rep = readDocker(ctx, cli)
		}
	}
}

// ReadDocker is one pass of `docker ps` and `docker stats`, when the context is local.
func ReadDocker(ctx context.Context) *DockerReport {
	cli := dockerCLI()
	if status := contextStatus(ctx, cli); status != DockerOK {
		return &DockerReport{Status: status, Containers: []Container{}}
	}
	return readDocker(ctx, cli)
}

// contextStatus is DockerOK when the CLI's current context is a unix socket, the only kind
// that is polled. `docker context inspect` reads the CLI's own files and contacts nothing.
func contextStatus(ctx context.Context, cli string) string {
	if cli == "" {
		return DockerMissing
	}
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	switch {
	case err != nil:
		slog.Debug("docker context inspect", "err", err)
		return DockerStopped
	case !bytes.HasPrefix(out, []byte("unix://")):
		return DockerRemote
	}
	return DockerOK
}

func readDocker(ctx context.Context, cli string) *DockerReport {
	rep := listContainers(ctx, cli)
	if rep.Status != DockerOK || len(rep.Containers) == 0 {
		return rep
	}
	ctx, cancel := context.WithTimeout(ctx, dockerStatsTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "stats", "--no-stream", "--format", "{{json .}}").Output()
	if err != nil {
		slog.Debug("docker stats", "err", err)
		return rep
	}
	stats := parseDockerStats(bytes.NewReader(out))
	for i := range rep.Containers {
		c := &rep.Containers[i]
		if s, ok := stats[c.ID]; ok {
			c.HasStats, c.CPU, c.Memory, c.PIDs = true, s.CPU, s.Memory, s.PIDs
		}
	}
	return rep
}

// dockerCLI finds the docker binary, "" when none is installed. mac-pulse runs with the
// system PATH, which holds none of these.
func dockerCLI() string {
	paths := []string{"/usr/local/bin/docker", "/opt/homebrew/bin/docker"}
	// Without a home the two would be relative paths, looked up in the working directory.
	if home, _ := os.UserHomeDir(); home != "" {
		paths = append(paths, filepath.Join(home, ".docker/bin/docker"), filepath.Join(home, ".orbstack/bin/docker"))
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// listContainers is `docker ps`: running containers only. Any failure of the CLI, output
// it does not know included, reads as a stopped daemon.
func listContainers(ctx context.Context, cli string) *DockerReport {
	rep := &DockerReport{Status: DockerMissing, Containers: []Container{}}
	if cli == "" {
		return rep
	}
	rep.Status = DockerStopped
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "ps", "--format", dockerPSFormat).Output()
	if err != nil {
		slog.Debug("docker ps", "err", err)
		return rep
	}
	containers, err := parseDockerPS(bytes.NewReader(out))
	if err != nil {
		slog.Debug("docker ps", "err", err)
		return rep
	}
	rep.Status, rep.Containers = DockerOK, containers
	return rep
}

// parseDockerPS reads `docker ps --format dockerPSFormat`: one JSON object per line.
func parseDockerPS(r io.Reader) ([]Container, error) {
	containers := []Container{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var row struct {
			ID, Name, Image, State, Status, Ports, Project, Dir string
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("decode docker ps line: %w", err)
		}
		if row.ID == "" {
			return nil, fmt.Errorf("docker ps line without an id: %q", sc.Text())
		}
		containers = append(containers, Container{
			ID: row.ID, Name: row.Name, Image: row.Image, State: row.State, Status: row.Status, Ports: row.Ports,
			Project: row.Project, Dir: row.Dir,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read docker ps: %w", err)
	}
	return containers, nil
}

// parseDockerStats reads `docker stats --no-stream --format '{{json .}}'` into CPU, Memory
// and PIDs by container id. A line it cannot read is skipped: that container stays without stats.
func parseDockerStats(r io.Reader) map[string]Container {
	stats := map[string]Container{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var row struct{ ID, CPUPerc, MemUsage, PIDs string }
		if json.Unmarshal(sc.Bytes(), &row) != nil || row.ID == "" {
			continue
		}
		cpu, errCPU := strconv.ParseFloat(strings.TrimSuffix(row.CPUPerc, "%"), 64)
		pids, errPIDs := strconv.Atoi(row.PIDs)
		// "60MiB / 7.653GiB": usage, then the limit.
		used, _, _ := strings.Cut(row.MemUsage, " / ")
		number := strings.TrimRight(used, "BKMGTi")
		shift, known := map[string]uint{"B": 0, "KiB": 10, "MiB": 20, "GiB": 30, "TiB": 40}[used[len(number):]]
		mem, errMem := strconv.ParseFloat(number, 64)
		// ParseFloat takes "NaN" and "Inf", which the state's JSON cannot carry.
		if errCPU != nil || errPIDs != nil || errMem != nil || !known || !(cpu >= 0) || !(mem >= 0) || math.IsInf(cpu+mem, 1) {
			continue
		}
		stats[row.ID] = Container{CPU: cpu, Memory: uint64(mem * float64(uint64(1)<<shift)), PIDs: pids}
	}
	return stats
}
