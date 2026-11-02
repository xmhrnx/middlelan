//go:build darwin && !ios

package monitor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// darwinSource lists sockets with lsof and per-process traffic with
// nettop, both part of macOS. lsof only sees the user's own processes
// unless the app runs as administrator.
type darwinSource struct{}

// NewSource returns the connection source for this platform.
func NewSource() Source { return darwinSource{} }

func (darwinSource) Limits() string {
	if os.Geteuid() != 0 {
		return "Connections of system processes are hidden by macOS. Run as administrator to see them all."
	}
	return ""
}

func run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func (darwinSource) Sample() (Sample, error) {
	out, err := run("/usr/sbin/lsof", "-nP", "+c", "0", "-iTCP", "-iUDP", "-FpctPnT")
	// lsof exits 1 when some files couldn't be listed; its output is still good.
	if err != nil && len(out) == 0 {
		return Sample{}, fmt.Errorf("lsof: %w", err)
	}
	conns := parseLsof(out)
	smp := Sample{Conns: conns}
	if nt, err := run("/usr/bin/nettop", "-P", "-L", "1", "-x", "-J", "bytes_in,bytes_out"); err == nil {
		smp.Proc = parseNettop(nt)
		smp.PerApp = len(smp.Proc) > 0
	}
	if ns, err := run("/usr/sbin/netstat", "-ibn"); err == nil {
		smp.Iface = parseNetstatIB(ns)
	}
	return smp, nil
}

// parseLsof reads lsof's field output (-F pctPnT).
func parseLsof(out []byte) []Conn {
	var conns []Conn
	var pid int
	var cmd string
	var cur *Conn
	flush := func() {
		if cur != nil && cur.LocalPort != 0 {
			conns = append(conns, *cur)
		}
		cur = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		v := line[1:]
		switch line[0] {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(v)
		case 'c':
			cmd = v
		case 'f':
			flush()
			cur = &Conn{PID: pid, Process: cmd, UID: -1}
		case 't':
			if cur != nil && v == "IPv6" {
				cur.Proto = "6"
			}
		case 'P':
			if cur != nil {
				cur.Proto = strings.ToLower(v) + cur.Proto
			}
		case 'n':
			if cur != nil {
				local, remote, _ := strings.Cut(v, "->")
				cur.LocalIP, cur.LocalPort = splitHostPort(local)
				cur.RemoteIP, cur.RemotePort = splitHostPort(remote)
				if remote == "" {
					cur.State = "LISTEN"
				}
				cur.Key = fmt.Sprintf("%s:%d:%s", cur.Proto, cur.PID, v)
			}
		case 'T':
			if cur != nil && strings.HasPrefix(v, "ST=") {
				cur.State = strings.TrimPrefix(v, "ST=")
			}
		}
	}
	flush()
	return conns
}

func splitHostPort(s string) (string, int) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return s, 0
	}
	host := strings.Trim(s[:i], "[]")
	if host == "*" {
		host = "0.0.0.0"
	}
	port, _ := strconv.Atoi(s[i+1:])
	return host, port
}

// parseNettop reads "name.pid,bytes_in,bytes_out," lines.
func parseNettop(out []byte) map[int]ProcBytes {
	procs := map[int]ProcBytes{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(f) < 3 {
			continue
		}
		i := strings.LastIndex(f[0], ".")
		if i < 0 {
			continue
		}
		pid, err := strconv.Atoi(f[0][i+1:])
		if err != nil {
			continue
		}
		rx, _ := strconv.ParseUint(f[1], 10, 64)
		tx, _ := strconv.ParseUint(f[2], 10, 64)
		procs[pid] = ProcBytes{Name: f[0][:i], ByteCount: ByteCount{Rx: rx, Tx: tx}}
	}
	return procs
}

// parseNetstatIB sums the link-level rows of netstat -ibn, skipping loopback.
func parseNetstatIB(out []byte) ByteCount {
	var total ByteCount
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 || !strings.HasPrefix(f[2], "<Link#") || strings.HasPrefix(f[0], "lo") {
			continue
		}
		// Rows without an address have one field fewer.
		ib, ob := f[len(f)-5], f[len(f)-2]
		rx, err1 := strconv.ParseUint(ib, 10, 64)
		tx, err2 := strconv.ParseUint(ob, 10, 64)
		if err1 == nil && err2 == nil {
			total.Rx += rx
			total.Tx += tx
		}
	}
	return total
}
