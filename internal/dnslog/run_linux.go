//go:build linux && !android

package dnslog

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// run captures DNS answers with an AF_PACKET socket on every interface. A
// classic BPF filter keeps everything but UDP from port 53 in the kernel,
// so the process only wakes for DNS responses. It needs root or
// CAP_NET_RAW.
func (l *Log) run(ctx context.Context) (err error) {
	f, oerr := openPacketSocket()
	if oerr != nil {
		if errors.Is(oerr, unix.EPERM) || errors.Is(oerr, unix.EACCES) {
			l.setMethod("Unavailable: run as administrator", false)
		} else {
			l.setMethod("Unavailable: "+oerr.Error(), false)
		}
		return nil
	}
	l.setMethod("Packet capture", true)
	defer l.setMethod("Packet capture", false)

	// Closing the socket when ctx ends unblocks the pending Read. Its
	// error is of no use then: the capture is over either way.
	stop := context.AfterFunc(ctx, func() { _ = f.Close() })
	defer func() {
		if stop() {
			if cerr := f.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("dnslog: closing packet socket: %w", cerr)
			}
		}
	}()

	buf := make([]byte, snapLen)
	for {
		n, rerr := f.Read(buf)
		if rerr != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(rerr, io.EOF) {
				continue // an empty datagram
			}
			return fmt.Errorf("dnslog: reading packets: %w", rerr)
		}
		l.handlePacket(buf[:n], time.Now())
	}
}

// openPacketSocket returns a non-blocking AF_PACKET socket, wrapped so the
// Go poller waits on it, that only receives DNS responses.
func openPacketSocket() (*os.File, error) {
	// Protocol 0 receives nothing until bound, so no unfiltered packet is
	// queued before the filter is attached.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("opening packet socket: %w", err)
	}
	if err := attachFilter(fd); err != nil {
		return nil, errors.Join(err, unix.Close(fd))
	}
	addr := &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL)}
	if err := unix.Bind(fd, addr); err != nil {
		return nil, errors.Join(fmt.Errorf("binding packet socket: %w", err), unix.Close(fd))
	}
	return os.NewFile(uintptr(fd), "dnslog-packet"), nil
}

// attachFilter installs the DNS response filter on fd.
func attachFilter(fd int) error {
	raw, err := dnsFilter(filterOptions{skipOutgoing: true})
	if err != nil {
		return err
	}
	prog := toSockFilter(raw)
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &fprog); err != nil {
		return fmt.Errorf("attaching packet filter: %w", err)
	}
	return nil
}

// toSockFilter converts assembled instructions to the kernel's layout.
func toSockFilter(raw []bpf.RawInstruction) []unix.SockFilter {
	out := make([]unix.SockFilter, len(raw))
	for i, r := range raw {
		out[i] = unix.SockFilter{Code: r.Op, Jt: r.Jt, Jf: r.Jf, K: r.K}
	}
	return out
}

// htons converts a 16-bit value to network byte order.
func htons(v uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return binary.NativeEndian.Uint16(b[:])
}
