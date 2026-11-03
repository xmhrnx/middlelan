//go:build windows

package nettools

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Bindings for the IPv4 echo functions of iphlpapi.dll. These need no
// administrator rights.

var (
	modIphlpapi         = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = modIphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = modIphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = modIphlpapi.NewProc("IcmpSendEcho")
)

const (
	ipSuccess           = 0
	ipReqTimedOut       = 11010
	ipTTLExpiredTransit = 11013
	ipTTLExpiredReassem = 11014
)

// ipOptionInformation mirrors IP_OPTION_INFORMATION.
type ipOptionInformation struct {
	TTL         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

// icmpEchoReply mirrors ICMP_ECHO_REPLY.
type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

// icmpEchoResult is the decoded outcome of one IcmpSendEcho call.
type icmpEchoResult struct {
	From   net.IP
	Status uint32
	RTTms  uint32
	TTL    int
}

// icmpHandle is an open IcmpCreateFile handle.
type icmpHandle uintptr

func icmpOpen() (icmpHandle, error) {
	if err := procIcmpCreateFile.Find(); err != nil {
		return 0, err
	}
	r, _, err := procIcmpCreateFile.Call()
	if windows.Handle(r) == windows.InvalidHandle {
		return 0, fmt.Errorf("IcmpCreateFile: %w", err)
	}
	return icmpHandle(r), nil
}

func (h icmpHandle) close() error {
	r, _, err := procIcmpCloseHandle.Call(uintptr(h))
	if r == 0 {
		return fmt.Errorf("IcmpCloseHandle: %w", err)
	}
	return nil
}

// echo sends one IPv4 echo request with the given TTL (0 = default) and
// waits up to timeoutMs. It returns an error only for API failures; a
// timeout is reported as Status ipReqTimedOut.
func (h icmpHandle) echo(ip net.IP, ttl int, timeoutMs uint32) (icmpEchoResult, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return icmpEchoResult{}, errors.New("IcmpSendEcho: IPv4 only")
	}
	// IPAddr is the address in network byte order as laid out in memory.
	dst := *(*uint32)(unsafe.Pointer(&ip4[0]))
	payload := []byte("MiddleMan-ping-x")
	reply := make([]byte, int(unsafe.Sizeof(icmpEchoReply{}))+len(payload)+8+64)
	var opts *ipOptionInformation
	if ttl > 0 {
		opts = &ipOptionInformation{TTL: uint8(ttl)}
	}
	r, _, callErr := procIcmpSendEcho.Call(
		uintptr(h),
		uintptr(dst),
		uintptr(unsafe.Pointer(&payload[0])),
		uintptr(len(payload)),
		uintptr(unsafe.Pointer(opts)),
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(len(reply)),
		uintptr(timeoutMs),
	)
	er := (*icmpEchoReply)(unsafe.Pointer(&reply[0]))
	res := icmpEchoResult{Status: er.Status, RTTms: er.RoundTripTime, TTL: int(er.Options.TTL)}
	if er.Address != 0 {
		a := er.Address
		res.From = net.IPv4(byte(a), byte(a>>8), byte(a>>16), byte(a>>24))
	}
	if r != 0 {
		return res, nil
	}
	var errno syscall.Errno
	if errors.As(callErr, &errno) {
		switch errno {
		case ipReqTimedOut:
			res.Status = ipReqTimedOut
			return res, nil
		case ipTTLExpiredTransit, ipTTLExpiredReassem:
			res.Status = uint32(errno)
			return res, nil
		}
		if errno >= 11000 && errno < 11100 {
			// Other IP_* status codes (unreachable, ...): not an API failure.
			res.Status = uint32(errno)
			return res, nil
		}
	}
	if res.Status != ipSuccess && res.From != nil {
		return res, nil
	}
	return res, fmt.Errorf("IcmpSendEcho: %w", callErr)
}
