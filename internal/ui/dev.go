package ui

import (
	"cmp"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

// buildDev groups the user's listening processes into projects by working directory and
// counts the compose containers started from the same directory. home turns a compose
// label into the "~" form the listeners and the agents already carry.
func buildDev(snap *collector.Snapshot, listening []netinspect.Listener, docker *collector.DockerReport, home string) *Dev {
	dev := &Dev{
		Docker:     docker.Status,
		Containers: make([]DevContainer, 0, len(docker.Containers)),
		Agents:     make([]DevAgent, 0, len(snap.Agents)),
	}
	projects := devProjects(snap.Processes, listening)
	for _, c := range docker.Containers {
		dir := collector.TildePath(home, c.Dir)
		out := DevContainer{
			ID: c.ID, Name: displayText(c.Name), Image: displayText(c.Image), State: c.State, Status: displayText(c.Status),
			Ports: c.Ports, Project: displayText(c.Project), Dir: displayText(dir),
		}
		if c.HasStats {
			out.CPU, out.Memory, out.PIDs = &c.CPU, &c.Memory, &c.PIDs
		}
		dev.Containers = append(dev.Containers, out)
		if p := projects[dir]; p != nil {
			p.Containers++
		}
	}
	dev.Projects = make([]DevProject, 0, len(projects))
	for _, p := range projects {
		slices.Sort(p.Ports)
		p.CPU = tenth(p.CPU)
		dev.Projects = append(dev.Projects, *p)
	}
	slices.SortFunc(dev.Projects, func(x, y DevProject) int { return cmp.Or(cmp.Compare(y.CPU, x.CPU), cmp.Compare(x.Dir, y.Dir)) })
	for _, a := range snap.Agents {
		agent := DevAgent{
			Name: displayText(a.Name), PID: a.PID, Dir: displayText(a.Dir), Since: a.Since.UnixMilli(),
			Procs: a.Procs, CPU: tenth(a.CPU), Memory: a.RSS,
		}
		if a.HasUsage {
			energy := tenth(a.EnergyMW)
			agent.EnergyMW = &energy
		}
		dev.Agents = append(dev.Agents, agent)
	}
	return dev
}

// devProjects sums the listening processes of each working directory, keyed by it.
func devProjects(processes []collector.Process, listening []netinspect.Listener) map[string]*DevProject {
	procs := make(map[int32]collector.Process, len(processes))
	for _, p := range processes {
		procs[p.PID] = p
	}
	projects := map[string]*DevProject{}
	for _, l := range listening {
		// No directory: another user's process or a GUI app. The home itself is where every
		// tool started from a fresh terminal sits; an app's helper sits inside its bundle or
		// its Library container (Docker's backend, an IDE's embedded browser). None is a project.
		if l.Dir == "" || l.Dir == "~" || strings.Contains(l.Dir+"/", ".app/") || strings.Contains(l.Dir+"/", "/Library/") {
			continue
		}
		p := projects[l.Dir]
		if p == nil {
			p = &DevProject{Name: displayText(path.Base(l.Dir)), Dir: displayText(l.Dir), Ports: []int{}, Apps: []string{}, PIDs: []int32{}}
			projects[l.Dir] = p
		}
		if port, err := strconv.Atoi(l.Port); err == nil && !slices.Contains(p.Ports, port) {
			p.Ports = append(p.Ports, port)
		}
		if app := displayText(l.App); !slices.Contains(p.Apps, app) {
			p.Apps = append(p.Apps, app)
		}
		if !slices.Contains(p.PIDs, l.PID) {
			p.PIDs = append(p.PIDs, l.PID)
			p.CPU += procs[l.PID].CPU
			p.Memory += procs[l.PID].RSS
		}
	}
	return projects
}
