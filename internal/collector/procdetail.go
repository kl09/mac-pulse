package collector

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

const (
	// maxOpenRows bounds the files and sockets of one answer together.
	maxOpenRows = 200
	// A command line is cut to maxArgs arguments of maxArgRunes each: it is shown, not replayed.
	maxArgs     = 200
	maxArgRunes = 1000
)

// ProcDetail is what one process has open right now. Its start time is StartedAt's.
type ProcDetail struct {
	Threads int
	// Files are the working directory and the regular files, folders and devices open on a
	// descriptor; Sockets the IPv4, IPv6 and unix sockets. Together at most 200, More counts the rest.
	Files   []string
	Sockets []string
	More    int
	// Args is the command line, read only when asked for and kept nowhere.
	Args []string
}

// ReadProcDetail reads a process of the caller's own user; another user's process is an
// error, not an empty answer. The environment is never read.
func ReadProcDetail(ctx context.Context, pid int32, withArgs bool) (ProcDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	// lsof prints nothing and exits 1 for a pid that is gone or belongs to another user.
	var out capped
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-p", strconv.Itoa(int(pid)), "-F", "ftn")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ProcDetail{}, fmt.Errorf("lsof pid %d: %w", pid, err)
	}
	d := parseLsof(out.buf)
	p := &process.Process{Pid: pid}
	threads, err := p.NumThreadsWithContext(ctx)
	if err != nil {
		return ProcDetail{}, fmt.Errorf("threads of pid %d: %w", pid, err)
	}
	d.Threads, d.Args = int(threads), []string{}
	if !withArgs {
		return d, nil
	}
	argv, err := p.CmdlineSliceWithContext(ctx)
	if err != nil {
		return ProcDetail{}, fmt.Errorf("command line of pid %d: %w", pid, err)
	}
	for _, arg := range argv[:min(len(argv), maxArgs)] {
		d.Args = append(d.Args, CleanText(arg, maxArgRunes))
	}
	return d, nil
}

// parseLsof reads `lsof -F ftn`: one field per line, its first byte the field's letter, and
// every open file as f (descriptor), t (type), n (name) in that order. "txt" rows are the
// executable and every library it loaded, which is most of the output and not what the
// process opened.
func parseLsof(out []byte) ProcDetail {
	d := ProcDetail{Files: []string{}, Sockets: []string{}}
	var fd, kind string
	// unnamed counts the unix sockets lsof knows only by a kernel address ("->0x933565f8"),
	// which tells the reader nothing: they share the one row at d.Sockets[unnamedAt].
	unnamed, unnamedAt := 0, 0
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		switch value := line[1:]; line[0] {
		case 'f':
			fd, kind = value, ""
		case 't':
			kind = value
		case 'n':
			file, socket := opened(fd, kind)
			addressOnly := socket && strings.HasPrefix(kind+" "+value, "unix ->0x")
			switch {
			case !file && !socket:
			case addressOnly && unnamed > 0:
				unnamed++
			case len(d.Files)+len(d.Sockets) >= maxOpenRows:
				d.More++
			case file:
				d.Files = append(d.Files, CleanText(value, 0))
			case addressOnly:
				unnamed, unnamedAt = 1, len(d.Sockets)
				d.Sockets = append(d.Sockets, "unix socket")
			default:
				d.Sockets = append(d.Sockets, kind+" "+CleanText(value, 0))
			}
		}
	}
	if unnamed > 1 {
		d.Sockets[unnamedAt] = fmt.Sprintf("unix socket × %d", unnamed)
	}
	return d
}

// opened says whether a row of lsof is a file or a socket the process holds: one on a
// numbered descriptor, or for a file the working directory.
func opened(fd, kind string) (file, socket bool) {
	_, err := strconv.Atoi(fd)
	file = (fd == "cwd" || err == nil) && slices.Contains([]string{"REG", "DIR", "CHR"}, kind)
	socket = err == nil && slices.Contains([]string{"IPv4", "IPv6", "unix"}, kind)
	return file, socket
}
