package collector

import (
	"cmp"
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// agentNames are the process names, as ps prints them, that open a session.
// A heuristic. An agent missing here is not shown, a CLI that runs as "node" is
// not recognised, and any process that happens to be called "codex" is.
var agentNames = []string{"codex", "aider", "gemini", "cursor-agent", "opencode"}

// agentMeta is what a session's root never changes, read once per pid.
type agentMeta struct {
	dir   string
	since time.Time
}

// TildePath writes dir with the home directory as "~".
func TildePath(home, dir string) string {
	if rest, ok := strings.CutPrefix(dir, home); ok && home != "" && (rest == "" || rest[0] == '/') {
		return "~" + rest
	}
	return dir
}

// groupAgents sums every session: its root is the topmost agent-named process of a parent
// chain, so an agent started inside a session belongs to that session. Dir and Since are
// left to the caller. Busiest first.
func groupAgents(procs []Process) []Agent {
	byPID := make(map[int32]Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	sessions := map[int32]*Agent{}
	for _, p := range procs {
		var root Process
		for cur, hops := p, 0; hops < maxParentHops; hops++ {
			if slices.Contains(agentNames, cur.Name) {
				root = cur
			}
			parent, ok := byPID[cur.PPID]
			if !ok || cur.PPID <= 1 {
				break
			}
			cur = parent
		}
		if root.PID == 0 {
			continue
		}
		a := sessions[root.PID]
		if a == nil {
			a = &Agent{Name: root.Name, PID: root.PID}
			sessions[root.PID] = a
		}
		a.Procs++
		a.CPU += p.CPU
		a.RSS += p.RSS
		a.EnergyMW += p.EnergyMW
		a.HasUsage = a.HasUsage || p.HasUsage
	}
	agents := make([]Agent, 0, len(sessions))
	for _, a := range sessions {
		agents = append(agents, *a)
	}
	slices.SortFunc(agents, func(x, y Agent) int { return cmp.Or(cmp.Compare(y.CPU, x.CPU), cmp.Compare(x.PID, y.PID)) })
	return agents
}

// collectAgents runs only while Detail.Dev is on.
func (s *Sampler) collectAgents(ctx context.Context, snap *Snapshot) {
	if !s.detail.Dev {
		s.agents = nil
		return
	}
	snap.Agents = groupAgents(snap.Processes)
	home, _ := os.UserHomeDir()
	meta := make(map[int32]agentMeta, len(snap.Agents))
	for i := range snap.Agents {
		a := &snap.Agents[i]
		m, ok := s.agents[a.PID]
		if !ok {
			// Another user's session answers EPERM: no directory, and it counts from now.
			p := &process.Process{Pid: a.PID}
			dir, _ := p.CwdWithContext(ctx)
			m = agentMeta{dir: TildePath(home, dir), since: snap.Time}
			if ms, err := p.CreateTimeWithContext(ctx); err == nil {
				m.since = time.UnixMilli(ms)
			}
		}
		meta[a.PID] = m
		a.Dir, a.Since = m.dir, m.since
	}
	s.agents = meta
}
