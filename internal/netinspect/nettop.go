package netinspect

import (
	"bufio"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type byteCount struct {
	In  uint64
	Out uint64
}

type byteRate struct {
	In  float64
	Out float64
}

// parseNettop reads `nettop -L 1 -x -J bytes_in,bytes_out,state`. A line with
// "<->" is a socket under the preceding "name.pid" line; the column order is
// state,bytes_in,bytes_out regardless of -J. Sockets without bytes (unbound or
// Listen) are not connections; Listen and bound UDP sockets are listeners,
// deduped so the v4 and v6 twins collapse into one row. nettop does not quote
// the process name, so the fixed columns are taken from the right and a comma in
// the name stays in the name. It does not escape a newline in the name either, so a
// line that fits neither shape ends the current process: the sockets after it would
// otherwise be counted for the process listed before.
//
// A process literally named "tcp4 a<->b" still reads as a socket.
//
//nolint:gocyclo // one pass over one line format; a split would spread the column layout over two functions
func parseNettop(r io.Reader) (map[int32]byteCount, []Conn, []Listener, error) {
	procs := map[int32]byteCount{}
	var conns []Conn
	var listening []Listener
	seen := map[Listener]struct{}{}
	var pid int32
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Scan()
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ",")
		n := len(fields)
		if n < 5 {
			pid = 0
			continue
		}
		name, state, in, out := strings.Join(fields[:n-4], ","), fields[n-4], fields[n-3], fields[n-2]
		bytesIn, _ := strconv.ParseUint(in, 10, 64)
		bytesOut, _ := strconv.ParseUint(out, 10, 64)
		socket := strings.Contains(name, "<->") &&
			(strings.HasPrefix(name, "tcp") || strings.HasPrefix(name, "udp") || strings.HasPrefix(name, "quic"))
		if !socket {
			p, err := strconv.ParseInt(name[strings.LastIndex(name, ".")+1:], 10, 64)
			if err != nil || p <= 0 || p > math.MaxInt32 {
				pid = 0
				continue
			}
			pid = int32(p)
			procs[pid] = byteCount{In: bytesIn, Out: bytesOut}
			continue
		}
		proto, endpoints, _ := strings.Cut(name, " ")
		local, remote, _ := strings.Cut(endpoints, "<->")
		if local == "" || remote == "" {
			pid = 0
			continue
		}
		v6 := strings.HasSuffix(proto, "6")
		localAddr, localPort := splitEndpoint(local, v6)
		remoteAddr, remotePort := splitEndpoint(remote, v6)
		if in != "" || out != "" {
			conns = append(conns, Conn{
				PID: pid, Proto: proto, Local: localAddr + ":" + localPort,
				RemoteIP: remoteAddr, RemotePort: remotePort, State: state,
				BytesIn: bytesIn, BytesOut: bytesOut,
			})
		}
		boundUDP := strings.HasPrefix(proto, "udp") && localPort != "*" && remoteAddr == "*" && remotePort == "*"
		if state == "Listen" || boundUDP {
			l := Listener{PID: pid, Proto: strings.TrimRight(proto, "46"), Addr: localAddr, Port: localPort}
			if _, dup := seen[l]; !dup {
				seen[l] = struct{}{}
				listening = append(listening, l)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, nil, err
	}
	return procs, conns, listening, nil
}

// v4 endpoints are addr:port, v6 are addr.port (the address itself holds colons
// and may carry a %zone).
func splitEndpoint(endpoint string, v6 bool) (addr, port string) {
	sep := ":"
	if v6 {
		sep = "."
	}
	i := strings.LastIndex(endpoint, sep)
	if i < 0 {
		return endpoint, ""
	}
	return endpoint[:i], endpoint[i+1:]
}

// rates gives bytes per second per key; a key absent from prev or a counter
// that went backwards (pid reuse, new socket on the same 4-tuple) reads 0.
func rates[K comparable](prev, cur map[K]byteCount, dt time.Duration) map[K]byteRate {
	out := make(map[K]byteRate, len(cur))
	for k, c := range cur {
		p, ok := prev[k]
		if !ok || dt <= 0 {
			out[k] = byteRate{}
			continue
		}
		var r byteRate
		if c.In >= p.In {
			r.In = float64(c.In-p.In) / dt.Seconds()
		}
		if c.Out >= p.Out {
			r.Out = float64(c.Out-p.Out) / dt.Seconds()
		}
		out[k] = r
	}
	return out
}

func (c Conn) key() string {
	return c.Proto + " " + c.Local + "<->" + c.RemoteIP + ":" + c.RemotePort
}
