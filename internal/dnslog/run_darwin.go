//go:build darwin && !ios

package dnslog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// bpfBufLen is the BPF read buffer size; reads must use exactly the
	// size the kernel settles on.
	bpfBufLen = 1 << 16
	// ifaceCheck is how often the default interface is looked up again,
	// so capture follows a switch between Wi-Fi, Ethernet and VPNs.
	ifaceCheck = time.Minute
	// noNetworkRetry is how long to wait when there is no default route.
	noNetworkRetry = 15 * time.Second
	// dltLoop is DLT_LOOP: like DLT_NULL, a 4-byte address family header.
	dltLoop = 108
)

// run captures DNS answers with a BPF device on the default interface. It
// needs root.
func (l *Log) run(ctx context.Context) error {
	if os.Geteuid() != 0 {
		l.setMethod("Unavailable: run as administrator", false)
		return nil
	}
	defer l.setMethod("Packet capture", false)
	for ctx.Err() == nil {
		iface, err := defaultInterface(ctx)
		if err != nil {
			l.setMethod("Packet capture (waiting for a network)", true)
			if !sleep(ctx, noNetworkRetry) {
				return nil
			}
			continue
		}
		dev, err := openBPF(iface)
		if err != nil {
			if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
				l.setMethod("Unavailable: run as administrator", false)
				return nil
			}
			l.setMethod("Packet capture (can't open "+iface+")", true)
			if !sleep(ctx, noNetworkRetry) {
				return nil
			}
			continue
		}
		l.setMethod("Packet capture", true)
		err = l.capture(ctx, dev, iface)
		if cerr := unix.Close(dev.fd); cerr != nil && err == nil {
			err = fmt.Errorf("dnslog: closing BPF device: %w", cerr)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// bpfDevice is an open BPF device bound to an interface.
type bpfDevice struct {
	fd      int
	bufLen  int
	linkLen int // bytes of link-layer header before the IP header
}

// capture reads from dev until ctx ends or the default interface changes.
func (l *Log) capture(ctx context.Context, dev bpfDevice, iface string) error {
	buf := make([]byte, dev.bufLen)
	nextCheck := time.Now().Add(ifaceCheck)
	for ctx.Err() == nil {
		n, err := unix.Read(dev.fd, buf)
		if err != nil {
			if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
				continue
			}
			return fmt.Errorf("dnslog: reading BPF device: %w", err)
		}
		now := time.Now()
		l.handleBPF(buf[:n], dev.linkLen, now)
		if now.After(nextCheck) {
			nextCheck = now.Add(ifaceCheck)
			if cur, err := defaultInterface(ctx); err != nil || cur != iface {
				return nil
			}
		}
	}
	return nil
}

// openBPF opens the first free /dev/bpfN, binds it to iface and installs
// the DNS response filter.
func openBPF(iface string) (bpfDevice, error) {
	fd := -1
	var err error
	for i := 0; i < 256; i++ {
		fd, err = unix.Open(fmt.Sprintf("/dev/bpf%d", i), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err == nil || !errors.Is(err, unix.EBUSY) {
			break
		}
	}
	if err != nil {
		return bpfDevice{}, fmt.Errorf("opening BPF device: %w", err)
	}
	dev, err := setupBPF(fd, iface)
	if err != nil {
		return bpfDevice{}, errors.Join(err, unix.Close(fd))
	}
	return dev, nil
}

// setupBPF configures an open BPF device.
func setupBPF(fd int, iface string) (bpfDevice, error) {
	dev := bpfDevice{fd: fd}
	// The buffer size must be set before binding to an interface.
	if err := unix.IoctlSetPointerInt(fd, unix.BIOCSBLEN, bpfBufLen); err != nil {
		return dev, fmt.Errorf("setting BPF buffer size: %w", err)
	}
	var ifr [unix.IFNAMSIZ + 16]byte
	if len(iface) >= unix.IFNAMSIZ {
		return dev, fmt.Errorf("interface name %q too long", iface)
	}
	copy(ifr[:], iface)
	if err := ioctl(fd, unix.BIOCSETIF, unsafe.Pointer(&ifr[0])); err != nil {
		return dev, fmt.Errorf("binding BPF device to %s: %w", iface, err)
	}
	dlt, err := unix.IoctlGetInt(fd, unix.BIOCGDLT)
	if err != nil {
		return dev, fmt.Errorf("reading link type: %w", err)
	}
	opts := filterOptions{}
	switch dlt {
	case unix.DLT_EN10MB:
		opts = filterOptions{offset: 14, ethernet: true}
	case unix.DLT_NULL, dltLoop:
		opts = filterOptions{offset: 4}
	case unix.DLT_RAW:
		opts = filterOptions{}
	default:
		return dev, fmt.Errorf("unsupported link type %d on %s", dlt, iface)
	}
	dev.linkLen = int(opts.offset)
	raw, err := dnsFilter(opts)
	if err != nil {
		return dev, err
	}
	insns := make([]unix.BpfInsn, len(raw))
	for i, r := range raw {
		insns[i] = unix.BpfInsn{Code: r.Op, Jt: r.Jt, Jf: r.Jf, K: r.K}
	}
	prog := unix.BpfProgram{Len: uint32(len(insns)), Insns: &insns[0]}
	if err := ioctl(fd, unix.BIOCSETF, unsafe.Pointer(&prog)); err != nil {
		return dev, fmt.Errorf("installing BPF filter: %w", err)
	}
	if err := unix.IoctlSetPointerInt(fd, unix.BIOCIMMEDIATE, 1); err != nil {
		return dev, fmt.Errorf("setting BPF immediate mode: %w", err)
	}
	// A read timeout lets the loop notice ctx ending on a quiet network.
	tv := unix.Timeval{Sec: 1}
	if err := ioctl(fd, unix.BIOCSRTIMEOUT, unsafe.Pointer(&tv)); err != nil {
		return dev, fmt.Errorf("setting BPF read timeout: %w", err)
	}
	if dev.bufLen, err = unix.IoctlGetInt(fd, unix.BIOCGBLEN); err != nil {
		return dev, fmt.Errorf("reading BPF buffer size: %w", err)
	}
	return dev, nil
}

// ioctl issues an ioctl whose argument is a pointer to a struct.
func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// defaultInterface names the interface of the default route.
func defaultInterface(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("finding the default route: %w", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if name, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "interface:"); ok {
			if name = strings.TrimSpace(name); name != "" {
				return name, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("reading the default route: %w", err)
	}
	return "", errors.New("no default route")
}

// sleep waits for d or until ctx ends; it reports whether ctx is still live.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
