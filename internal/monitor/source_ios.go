//go:build ios

package monitor

import (
	"fmt"
	"net"
	"syscall"
)

// iosSource reads the phone's interface byte counters. iOS keeps an app
// from seeing other apps' sockets or traffic, so this reports Wi-Fi and
// cellular as the "apps", and their sum as the device's traffic.
type iosSource struct {
	counters map[string]*[2]counter32 // by interface name: rx, tx
}

// NewSource returns the connection source for this platform.
func NewSource() Source { return &iosSource{counters: map[string]*[2]counter32{}} }

func (s *iosSource) Limits() string {
	return "iOS doesn't let apps see other apps' connections. MiddleMan shows this iPhone's Wi-Fi and cellular traffic while it's open."
}

func (s *iosSource) Sample() (Sample, error) {
	rib, err := syscall.RouteRIB(syscall.NET_RT_IFLIST, 0)
	if err != nil {
		return Sample{}, fmt.Errorf("interface counters: %w", err)
	}
	msgs, err := syscall.ParseRoutingMessage(rib)
	if err != nil {
		return Sample{}, fmt.Errorf("interface counters: %w", err)
	}
	names := map[int]string{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, ifc := range ifs {
			names[ifc.Index] = ifc.Name
		}
	}
	smp := Sample{Proc: map[int]ProcBytes{}, PerApp: true}
	kinds := map[string]int{ifaceWiFi: 1, ifaceCellular: 2, ifaceWired: 3}
	for _, m := range msgs {
		im, ok := m.(*syscall.InterfaceMessage)
		if !ok {
			continue
		}
		name := names[int(im.Header.Index)]
		kind := ifaceKind(name)
		if kind == "" {
			continue
		}
		c := s.counters[name]
		if c == nil {
			c = &[2]counter32{}
			s.counters[name] = c
		}
		rx, tx := c[0].update(im.Header.Data.Ibytes), c[1].update(im.Header.Data.Obytes)
		pid := kinds[kind]
		p := smp.Proc[pid]
		p.Name = kind
		p.Rx += rx
		p.Tx += tx
		smp.Proc[pid] = p
		smp.Iface.Rx += rx
		smp.Iface.Tx += tx
	}
	return smp, nil
}
