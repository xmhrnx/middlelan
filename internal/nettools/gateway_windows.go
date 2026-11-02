//go:build windows

package nettools

import (
	"context"
	"errors"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// systemGatewayDNS returns the default gateway and DNS servers on Windows
// using GetAdaptersAddresses, falling back to `route print`.
func systemGatewayDNS(localIP string) (string, []string) {
	gw, dns, err := adapterGatewayDNS(net.ParseIP(localIP))
	if err == nil && gw != "" {
		return gw, dns
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if out, rerr := runCmd(ctx, "route", "print", "0.0.0.0"); rerr == nil {
		gw = parseRoutePrint(string(out))
	}
	return gw, dns
}

// adapterGatewayDNS finds the adapter owning local (or the first adapter
// with a gateway) and returns its IPv4 gateway and DNS servers.
func adapterGatewayDNS(local net.IP) (string, []string, error) {
	size := uint32(15000)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) || i == 2 {
			return "", nil, err
		}
	}
	var fbGW string
	var fbDNS []string
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		gw := ""
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP(); ip != nil && ip.To4() != nil {
				gw = net.IP(append([]byte(nil), ip...)).String()
				break
			}
		}
		var dns []string
		for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
			if ip := d.Address.IP(); ip != nil {
				dns = appendUnique(dns, net.IP(append([]byte(nil), ip...)).String())
			}
		}
		owns := false
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip := u.Address.IP(); ip != nil && local != nil && ip.Equal(local) {
				owns = true
			}
		}
		if owns {
			return gw, dns, nil
		}
		if fbGW == "" && gw != "" {
			fbGW, fbDNS = gw, dns
		}
	}
	return fbGW, fbDNS, nil
}
