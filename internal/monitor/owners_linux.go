package monitor

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// procInfo names a process.
type procInfo struct {
	name, exe string
}

// owners maps socket inodes to the processes holding them. Walking every
// process's file descriptors is the costly part of a sample, so it's done
// only when unknown sockets appear, and at most every few seconds.
type owners struct {
	inodes   map[uint32]int
	procs    map[int]procInfo
	users    map[int]string
	lastScan time.Time
}

func newOwners() *owners {
	return &owners{inodes: map[uint32]int{}, procs: map[int]procInfo{}, users: map[int]string{}}
}

const ownerRescanEvery = 4 * time.Second

// resolve fills in PID, Process and Exe for each socket it can attribute.
func (o *owners) resolve(socks []diagSocket) []Conn {
	unknown := false
	for _, s := range socks {
		if _, ok := o.inodes[s.inode]; !ok && s.inode != 0 {
			unknown = true
			break
		}
	}
	if unknown && time.Since(o.lastScan) >= ownerRescanEvery {
		o.scan()
	}
	seen := make(map[uint32]bool, len(socks))
	conns := make([]Conn, 0, len(socks))
	for _, s := range socks {
		c := s.Conn
		seen[s.inode] = true
		if pid, ok := o.inodes[s.inode]; ok && pid > 0 {
			info := o.proc(pid)
			c.PID, c.Process, c.Exe = pid, info.name, info.exe
		}
		if c.Process == "" {
			c.Process = o.userLabel(c.UID)
		}
		conns = append(conns, c)
	}
	for ino := range o.inodes {
		if !seen[ino] {
			delete(o.inodes, ino)
		}
	}
	return conns
}

func (o *owners) scan() {
	o.lastScan = time.Now()
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	alive := map[int]bool{}
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		alive[pid] = true
		fdDir := filepath.Join("/proc", d.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // another user's process
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), 10, 32)
			if err == nil {
				o.inodes[uint32(ino)] = pid
			}
		}
	}
	for pid := range o.procs {
		if !alive[pid] {
			delete(o.procs, pid)
		}
	}
}

func (o *owners) proc(pid int) procInfo {
	if p, ok := o.procs[pid]; ok {
		return p
	}
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	var p procInfo
	if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
		p.name = strings.TrimSpace(string(b))
	}
	if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
		p.exe = strings.TrimSuffix(exe, " (deleted)")
		// comm is cut at 15 characters; the executable's name isn't.
		if base := filepath.Base(p.exe); len(p.name) >= 15 && strings.HasPrefix(base, p.name) {
			p.name = base
		}
	}
	if p.name == "" {
		p.name = "pid " + strconv.Itoa(pid)
	}
	o.procs[pid] = p
	return p
}

func (o *owners) userLabel(uid int) string {
	if uid < 0 {
		return "Unknown"
	}
	if n, ok := o.users[uid]; ok {
		return n
	}
	label := "System"
	if uid != 0 {
		label = "uid " + strconv.Itoa(uid)
		if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
			label = u.Username
		}
	}
	label += " (hidden)"
	o.users[uid] = label
	return label
}
