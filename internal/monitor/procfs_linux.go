package monitor

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readProcNet parses /proc/net/{tcp,tcp6,udp,udp6}: the fallback when
// sock_diag isn't allowed. It has no byte counters.
func readProcNet(name string) ([]diagSocket, error) {
	f, err := os.Open(filepath.Join("/proc/net", name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	udp := strings.HasPrefix(name, "udp")
	var out []diagSocket
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		lip, lport, err1 := parseHexAddr(fields[1])
		rip, rport, err2 := parseHexAddr(fields[2])
		if err1 != nil || err2 != nil {
			continue
		}
		st, _ := strconv.ParseUint(fields[3], 16, 8)
		if st == 6 { // TIME_WAIT
			continue
		}
		uid, _ := strconv.Atoi(fields[7])
		inode, _ := strconv.ParseUint(fields[9], 10, 32)
		s := diagSocket{inode: uint32(inode)}
		s.Proto = name
		s.LocalIP, s.LocalPort, s.RemoteIP, s.RemotePort = lip, lport, rip, rport
		s.UID = uid
		if udp {
			if rport == 0 {
				s.State = "LISTEN"
			}
		} else {
			s.State = tcpStates[uint8(st)]
		}
		s.Key = fmt.Sprintf("%s:%s:%d-%s:%d:%d", name, lip, lport, rip, rport, inode)
		out = append(out, s)
	}
	return out, sc.Err()
}

// parseHexAddr decodes "0100007F:0035" (IPv4) or the 32-digit IPv6 form;
// each 32-bit word is in host (little endian) order.
func parseHexAddr(s string) (string, int, error) {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return "", 0, fmt.Errorf("bad address %q", s)
	}
	raw, err := hex.DecodeString(host)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return "", 0, fmt.Errorf("bad address %q", s)
	}
	for i := 0; i < len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return "", 0, err
	}
	ip := net.IP(raw)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.String(), int(p), nil
}

// readIfaceCounters sums the byte counters of every interface but loopback
// from /proc/net/dev.
func readIfaceCounters() (ByteCount, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return ByteCount{}, err
	}
	defer f.Close()
	var total ByteCount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		total.Rx += rx
		total.Tx += tx
	}
	return total, sc.Err()
}
