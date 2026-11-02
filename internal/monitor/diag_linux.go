package monitor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

// Netlink sock_diag constants (linux/inet_diag.h, linux/sock_diag.h).
const (
	netlinkInetDiag   = 4
	sockDiagByFamily  = 20
	inetDiagInfo      = 2
	inetDiagMsgLen    = 72
	inetDiagReqV2Len  = 56
	tcpInfoBytesAcked = 120
	tcpInfoBytesRecv  = 128
	tcpInfoRTT        = 68
)

var tcpStates = map[uint8]string{
	1: "ESTABLISHED", 2: "SYN_SENT", 3: "SYN_RECV", 4: "FIN_WAIT1", 5: "FIN_WAIT2",
	6: "TIME_WAIT", 7: "CLOSE", 8: "CLOSE_WAIT", 9: "LAST_ACK", 10: "LISTEN", 11: "CLOSING",
}

// diagSocket is one socket from a sock_diag dump, before its owner is known.
type diagSocket struct {
	Conn
	inode uint32
}

// diagDump lists the sockets of one family and protocol through netlink
// sock_diag. For TCP it asks for tcp_info, which carries the socket's byte
// counters; no privileges are needed for that.
func diagDump(family, proto uint8) ([]diagSocket, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, netlinkInetDiag)
	if err != nil {
		return nil, fmt.Errorf("sock_diag socket: %w", err)
	}
	defer syscall.Close(fd)
	tv := syscall.NsecToTimeval(int64(2 * time.Second))
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("sock_diag bind: %w", err)
	}

	req := make([]byte, syscall.NLMSG_HDRLEN+inetDiagReqV2Len)
	binary.NativeEndian.PutUint32(req[0:], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:], sockDiagByFamily)
	binary.NativeEndian.PutUint16(req[6:], syscall.NLM_F_REQUEST|syscall.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], 1)
	body := req[syscall.NLMSG_HDRLEN:]
	body[0] = family
	body[1] = proto
	if proto == syscall.IPPROTO_TCP {
		body[2] = 1 << (inetDiagInfo - 1)
	}
	// Every state but TIME_WAIT, which is a closed connection lingering.
	binary.NativeEndian.PutUint32(body[4:], 0xffffffff&^(1<<6))
	if err := syscall.Sendto(fd, req, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("sock_diag send: %w", err)
	}

	var out []diagSocket
	buf := make([]byte, 64*1024)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, fmt.Errorf("sock_diag recv: %w", err)
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			switch m.Header.Type {
			case syscall.NLMSG_DONE:
				return out, nil
			case syscall.NLMSG_ERROR:
				if len(m.Data) >= 4 {
					if code := int32(binary.NativeEndian.Uint32(m.Data)); code != 0 {
						return nil, fmt.Errorf("sock_diag: %w", syscall.Errno(-code))
					}
				}
				return nil, errors.New("sock_diag: error reply")
			}
			if s, ok := parseDiagMsg(m.Data, proto); ok {
				out = append(out, s)
			}
		}
	}
}

func parseDiagMsg(b []byte, proto uint8) (diagSocket, bool) {
	if len(b) < inetDiagMsgLen {
		return diagSocket{}, false
	}
	family := b[0]
	var s diagSocket
	s.State = tcpStates[b[1]]
	s.LocalPort = int(binary.BigEndian.Uint16(b[4:]))
	s.RemotePort = int(binary.BigEndian.Uint16(b[6:]))
	if family == syscall.AF_INET {
		s.LocalIP = net.IP(b[8:12]).String()
		s.RemoteIP = net.IP(b[24:28]).String()
	} else {
		s.LocalIP = net.IP(b[8:24]).String()
		s.RemoteIP = net.IP(b[24:40]).String()
	}
	cookie := binary.NativeEndian.Uint64(b[44:])
	s.UID = int(binary.NativeEndian.Uint32(b[64:]))
	s.inode = binary.NativeEndian.Uint32(b[68:])
	s.Proto = protoName(proto, family == syscall.AF_INET6)
	if proto == syscall.IPPROTO_UDP {
		s.State = ""
		if s.RemotePort == 0 {
			s.State = "LISTEN"
		}
	}
	s.Key = fmt.Sprintf("%s:%x", s.Proto, cookie)

	for a := b[inetDiagMsgLen:]; len(a) >= 4; {
		l := int(binary.NativeEndian.Uint16(a))
		t := binary.NativeEndian.Uint16(a[2:])
		if l < 4 || l > len(a) {
			break
		}
		if t == inetDiagInfo {
			info := a[4:l]
			if len(info) >= tcpInfoBytesRecv+8 {
				s.Sent = binary.NativeEndian.Uint64(info[tcpInfoBytesAcked:])
				s.Recv = binary.NativeEndian.Uint64(info[tcpInfoBytesRecv:])
				s.HasBytes = true
			}
			if len(info) >= tcpInfoRTT+4 {
				s.RTT = time.Duration(binary.NativeEndian.Uint32(info[tcpInfoRTT:])) * time.Microsecond
			}
		}
		a = a[(l+3)&^3:]
	}
	return s, true
}

func protoName(proto uint8, v6 bool) string {
	name := "tcp"
	if proto == syscall.IPPROTO_UDP {
		name = "udp"
	}
	if v6 {
		name += "6"
	}
	return name
}
