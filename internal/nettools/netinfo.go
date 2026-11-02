package nettools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Interface describes a local network interface.
type Interface struct {
	// Name is the OS interface name and MAC its hardware address.
	Name, MAC string
	// Addrs lists the interface addresses in CIDR notation.
	Addrs []string
	// Up reports whether the interface is administratively up.
	Up bool
	// MTU is the interface MTU.
	MTU int
}

// NetInfo summarises the local network configuration.
type NetInfo struct {
	// Hostname is this machine's host name.
	Hostname string
	// LocalIP is the IPv4 address used for outbound traffic, Subnet its
	// network in CIDR notation and Gateway the default gateway.
	LocalIP, Subnet, Gateway string
	// DNSServers lists the configured resolvers (best effort).
	DNSServers []string
	// Interfaces lists all network interfaces.
	Interfaces []Interface
}

// LocalNetInfo gathers host name, primary local IP, subnet, default gateway,
// DNS servers and interfaces. Individual fields are best-effort and may be
// empty when the platform does not expose them (e.g. Android sandboxing);
// an error is returned only if nothing useful could be determined.
func LocalNetInfo() (NetInfo, error) {
	var info NetInfo
	var errs []error
	h, err := os.Hostname()
	if err != nil {
		errs = append(errs, fmt.Errorf("hostname: %w", err))
	} else {
		info.Hostname = h
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		errs = append(errs, fmt.Errorf("interfaces: %w", err))
	}
	local, lerr := outboundIP()
	if lerr != nil {
		errs = append(errs, lerr)
	}
	for _, ifc := range ifaces {
		it := Interface{Name: ifc.Name, MAC: ifc.HardwareAddr.String(), Up: ifc.Flags&net.FlagUp != 0, MTU: ifc.MTU}
		addrs, err := ifc.Addrs()
		if err != nil {
			errs = append(errs, fmt.Errorf("addrs %s: %w", ifc.Name, err))
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			it.Addrs = append(it.Addrs, ipn.String())
			if local != nil && ipn.IP.Equal(local) {
				info.Subnet = (&net.IPNet{IP: ipn.IP.Mask(ipn.Mask), Mask: ipn.Mask}).String()
			}
		}
		info.Interfaces = append(info.Interfaces, it)
	}
	if local == nil {
		local = firstPrivateIPv4(ifaces)
	}
	if local != nil {
		info.LocalIP = local.String()
		if info.Subnet == "" {
			info.Subnet = (&net.IPNet{IP: local.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
		}
	}
	info.Gateway, info.DNSServers = systemGatewayDNS(info.LocalIP)
	if info.LocalIP == "" && len(info.Interfaces) == 0 {
		return info, fmt.Errorf("local network info: %w", errors.Join(errs...))
	}
	return info, nil
}

// outboundIP finds the local IPv4 address used to reach the internet by
// "connecting" a UDP socket (no packets are sent).
func outboundIP() (net.IP, error) {
	c, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return nil, fmt.Errorf("local ip: %w", err)
	}
	ua, ok := c.LocalAddr().(*net.UDPAddr)
	if cerr := c.Close(); cerr != nil {
		return nil, fmt.Errorf("local ip: %w", cerr)
	}
	if !ok || ua.IP == nil || ua.IP.IsUnspecified() {
		return nil, errors.New("local ip: unknown")
	}
	return ua.IP.To4(), nil
}

// firstPrivateIPv4 returns the first private IPv4 address of an up,
// non-loopback interface.
func firstPrivateIPv4(ifaces []net.Interface) net.IP {
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && ipn.IP.IsPrivate() {
				return ipn.IP.To4()
			}
		}
	}
	return nil
}

// localIPNet returns the IPv4 network of the interface holding ip.
func localIPNet(ip net.IP) *net.IPNet {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(ip) {
				return ipn
			}
		}
	}
	return nil
}

// PublicInfo describes this network's public IP address and its owner.
type PublicInfo struct {
	// IP is the public address; ISP and Org name the operator; ASN is the
	// autonomous system ("AS13335").
	IP, ISP, Org, ASN string
	// City, Region, Country and CountryCode locate the address.
	City, Region, Country, CountryCode string
	// Lat and Lon are approximate coordinates.
	Lat, Lon float64
}

const publicInfoTimeout = 8 * time.Second

// PublicIPInfo looks up the public IP address and its geolocation via
// ipinfo.io, falling back to api.ipify.org (address only).
func PublicIPInfo(ctx context.Context) (PublicInfo, error) {
	info, err := ipinfoLookup(ctx)
	if err == nil {
		return info, nil
	}
	if ctx.Err() != nil {
		return PublicInfo{}, ctx.Err()
	}
	var ipify struct {
		IP string `json:"ip"`
	}
	if ferr := getJSON(ctx, "https://api.ipify.org?format=json", &ipify); ferr != nil {
		return PublicInfo{}, fmt.Errorf("public ip: %w", errors.Join(err, ferr))
	}
	if ipify.IP == "" {
		return PublicInfo{}, fmt.Errorf("public ip: empty response (%w)", err)
	}
	return PublicInfo{IP: ipify.IP}, nil
}

func ipinfoLookup(ctx context.Context) (PublicInfo, error) {
	var r struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
		Loc     string `json:"loc"`
		Org     string `json:"org"`
	}
	if err := getJSON(ctx, "https://ipinfo.io/json", &r); err != nil {
		return PublicInfo{}, err
	}
	if r.IP == "" {
		return PublicInfo{}, errors.New("ipinfo: empty response")
	}
	info := PublicInfo{IP: r.IP, City: r.City, Region: r.Region, CountryCode: r.Country, Country: countryName(r.Country), Org: r.Org}
	if strings.HasPrefix(r.Org, "AS") {
		asn, name, _ := strings.Cut(r.Org, " ")
		info.ASN, info.ISP = asn, name
	} else {
		info.ISP = r.Org
	}
	if lat, lon, ok := strings.Cut(r.Loc, ","); ok {
		info.Lat, _ = strconv.ParseFloat(strings.TrimSpace(lat), 64) // 0 when malformed
		info.Lon, _ = strconv.ParseFloat(strings.TrimSpace(lon), 64)
	}
	return info, nil
}

// getJSON fetches url and decodes a JSON body into v.
func getJSON(ctx context.Context, url string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, publicInfoTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MiddleLAN")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // body fully consumed below
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %s", url, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}
