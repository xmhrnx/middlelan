//go:build windows

package monitor

import (
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi                 = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable  = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable  = iphlpapi.NewProc("GetExtendedUdpTable")
	procSetPerTcpConnEStats  = iphlpapi.NewProc("SetPerTcpConnectionEStats")
	procGetPerTcpConnEStats  = iphlpapi.NewProc("GetPerTcpConnectionEStats")
	procSetPerTcp6ConnEStats = iphlpapi.NewProc("SetPerTcp6ConnectionEStats")
	procGetPerTcp6ConnEStats = iphlpapi.NewProc("GetPerTcp6ConnectionEStats")
)

const (
	tcpTableOwnerPIDAll = 5
	udpTableOwnerPID    = 1
	tcpEstatsData       = 1
)

var winTCPStates = map[uint32]string{
	1: "CLOSED", 2: "LISTEN", 3: "SYN_SENT", 4: "SYN_RECV", 5: "ESTABLISHED", 6: "FIN_WAIT1",
	7: "FIN_WAIT2", 8: "CLOSE_WAIT", 9: "CLOSING", 10: "LAST_ACK", 11: "TIME_WAIT", 12: "DELETE_TCB",
}

// windowsSource reads the owner-annotated TCP and UDP tables from the IP
// helper API. Per-connection byte counters (extended statistics) need
// administrator rights; without them only connections are listed.
type windowsSource struct {
	mu      sync.Mutex
	procs   map[int]procInfo
	estats  bool
	enabled map[string]bool
}

type procInfo struct{ name, exe string }

// NewSource returns the connection source for this platform.
func NewSource() Source {
	return &windowsSource{procs: map[int]procInfo{}, estats: windows.GetCurrentProcessToken().IsElevated(), enabled: map[string]bool{}}
}

func (s *windowsSource) Limits() string {
	if !s.estats {
		return "Per-app bandwidth needs administrator rights on Windows. Connections are still listed."
	}
	return ""
}

func (s *windowsSource) Sample() (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var conns []Conn
	for _, af := range []uint32{windows.AF_INET, windows.AF_INET6} {
		tcp, err := s.tcpTable(af)
		if err != nil {
			return Sample{}, err
		}
		conns = append(conns, tcp...)
		udp, err := s.udpTable(af)
		if err != nil {
			return Sample{}, err
		}
		conns = append(conns, udp...)
	}
	alive := map[int]bool{}
	keys := make(map[string]bool, len(conns))
	for i := range conns {
		keys[conns[i].Key] = true
		alive[conns[i].PID] = true
		info := s.proc(conns[i].PID)
		conns[i].Process, conns[i].Exe = info.name, info.exe
	}
	for pid := range s.procs {
		if !alive[pid] {
			delete(s.procs, pid)
		}
	}
	for k := range s.enabled {
		if !keys[k] {
			delete(s.enabled, k)
		}
	}
	iface, err := ifaceCounters()
	if err != nil {
		return Sample{}, err
	}
	return Sample{Conns: conns, Iface: iface, PerApp: s.estats}, nil
}

func getTable(proc *windows.LazyProc, af uint32, class uintptr) ([]byte, error) {
	size := uint32(16 * 1024)
	for tries := 0; tries < 5; tries++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(af), class, 0)
		switch windows.Errno(r) {
		case 0:
			return buf[:size], nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4096
			continue
		default:
			return nil, fmt.Errorf("%s: %w", proc.Name, windows.Errno(r))
		}
	}
	return nil, fmt.Errorf("%s: table keeps growing", proc.Name)
}

func (s *windowsSource) tcpTable(af uint32) ([]Conn, error) {
	buf, err := getTable(procGetExtendedTcpTable, af, tcpTableOwnerPIDAll)
	if err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint32(buf))
	rowLen, proto := 24, "tcp"
	if af == windows.AF_INET6 {
		rowLen, proto = 56, "tcp6"
	}
	var out []Conn
	for i := 0; i < n; i++ {
		off := 4 + i*rowLen
		if off+rowLen > len(buf) {
			break
		}
		r := buf[off : off+rowLen]
		var c Conn
		c.Proto, c.UID = proto, -1
		var raw []byte
		if af == windows.AF_INET {
			c.State = winTCPStates[binary.LittleEndian.Uint32(r[0:])]
			c.LocalIP = net.IP(r[4:8]).String()
			c.LocalPort = int(binary.BigEndian.Uint16(r[8:]))
			c.RemoteIP = net.IP(r[12:16]).String()
			c.RemotePort = int(binary.BigEndian.Uint16(r[16:]))
			c.PID = int(binary.LittleEndian.Uint32(r[20:]))
			raw = append([]byte(nil), r[:20]...) // MIB_TCPROW
		} else {
			c.LocalIP = net.IP(r[0:16]).String()
			c.LocalPort = int(binary.BigEndian.Uint16(r[20:]))
			c.RemoteIP = net.IP(r[24:40]).String()
			c.RemotePort = int(binary.BigEndian.Uint16(r[44:]))
			state := binary.LittleEndian.Uint32(r[48:])
			c.State = winTCPStates[state]
			c.PID = int(binary.LittleEndian.Uint32(r[52:]))
			// MIB_TCP6ROW: State, then the address fields as above.
			raw = make([]byte, 52)
			binary.LittleEndian.PutUint32(raw, state)
			copy(raw[4:], r[:48])
		}
		if c.State == "TIME_WAIT" || c.State == "DELETE_TCB" {
			continue
		}
		c.Key = fmt.Sprintf("%s:%s:%d-%s:%d", proto, c.LocalIP, c.LocalPort, c.RemoteIP, c.RemotePort)
		if s.estats && c.State == "ESTABLISHED" {
			s.connBytes(&c, raw, af == windows.AF_INET6)
		}
		out = append(out, c)
	}
	return out, nil
}

// connBytes reads a connection's data byte counters, enabling their
// collection the first time the connection is seen.
func (s *windowsSource) connBytes(c *Conn, row []byte, v6 bool) {
	set, get := procSetPerTcpConnEStats, procGetPerTcpConnEStats
	if v6 {
		set, get = procSetPerTcp6ConnEStats, procGetPerTcp6ConnEStats
	}
	if !s.enabled[c.Key] {
		rw := [1]byte{1} // TCP_ESTATS_DATA_RW_v0{EnableCollection: TRUE}
		r, _, _ := set.Call(uintptr(unsafe.Pointer(&row[0])), tcpEstatsData, uintptr(unsafe.Pointer(&rw[0])), 0, 1, 0)
		if r != 0 {
			return
		}
		s.enabled[c.Key] = true
	}
	var rod [128]byte // TCP_ESTATS_DATA_ROD_v0
	r, _, _ := get.Call(uintptr(unsafe.Pointer(&row[0])), tcpEstatsData, 0, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&rod[0])), 0, uintptr(len(rod)))
	if r != 0 {
		return
	}
	c.Sent = binary.LittleEndian.Uint64(rod[0:])
	c.Recv = binary.LittleEndian.Uint64(rod[16:])
	c.HasBytes = true
}

func (s *windowsSource) udpTable(af uint32) ([]Conn, error) {
	buf, err := getTable(procGetExtendedUdpTable, af, udpTableOwnerPID)
	if err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint32(buf))
	rowLen, proto := 12, "udp"
	if af == windows.AF_INET6 {
		rowLen, proto = 28, "udp6"
	}
	var out []Conn
	for i := 0; i < n; i++ {
		off := 4 + i*rowLen
		if off+rowLen > len(buf) {
			break
		}
		r := buf[off : off+rowLen]
		c := Conn{Proto: proto, UID: -1, State: "LISTEN"}
		if af == windows.AF_INET {
			c.LocalIP = net.IP(r[0:4]).String()
			c.LocalPort = int(binary.BigEndian.Uint16(r[4:]))
			c.PID = int(binary.LittleEndian.Uint32(r[8:]))
		} else {
			c.LocalIP = net.IP(r[0:16]).String()
			c.LocalPort = int(binary.BigEndian.Uint16(r[20:]))
			c.PID = int(binary.LittleEndian.Uint32(r[24:]))
		}
		c.Key = fmt.Sprintf("%s:%s:%d:%d", proto, c.LocalIP, c.LocalPort, c.PID)
		out = append(out, c)
	}
	return out, nil
}

func (s *windowsSource) proc(pid int) procInfo {
	if p, ok := s.procs[pid]; ok {
		return p
	}
	p := procInfo{name: fmt.Sprintf("pid %d", pid)}
	switch pid {
	case 0:
		p.name = "System Idle"
	case 4:
		p.name = "System"
	default:
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err == nil {
			buf := make([]uint16, windows.MAX_LONG_PATH)
			size := uint32(len(buf))
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil {
				p.exe = windows.UTF16ToString(buf[:size])
				p.name = strings.TrimSuffix(filepath.Base(p.exe), filepath.Ext(p.exe))
			}
			windows.CloseHandle(h)
		}
	}
	s.procs[pid] = p
	return p
}

func ifaceCounters() (ByteCount, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return ByteCount{}, err
	}
	var total ByteCount
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		row := windows.MibIfRow2{InterfaceIndex: uint32(ifc.Index)}
		if windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row) != nil {
			continue
		}
		total.Rx += row.InOctets
		total.Tx += row.OutOctets
	}
	return total, nil
}
