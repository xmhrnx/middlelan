package nettools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device is a host discovered on the local network.
type Device struct {
	// IP is the IPv4 address; MAC the hardware address when known.
	IP, MAC string
	// Vendor is derived from the MAC's OUI; Hostname from reverse DNS,
	// NetBIOS or mDNS.
	Vendor, Hostname string
	// Kind is a best guess: "router", "phone", "computer", "printer", "tv",
	// "camera", "speaker", "iot" or "unknown".
	Kind string
	// Latency is the fastest liveness probe response.
	Latency time.Duration
	// OpenPorts lists the probed TCP ports that accepted a connection.
	OpenPorts []int
	// IsGateway and IsSelf mark the default gateway and this machine.
	IsGateway, IsSelf bool
}

// ScanOptions configures ScanLAN.
type ScanOptions struct {
	// Subnet to scan in CIDR notation; empty means the local /24.
	Subnet string
	// Timeout per probe (default 600ms).
	Timeout time.Duration
	// Concurrency is the number of hosts probed at once (default 32, max 64).
	Concurrency int
}

const (
	maxLANHosts       = 1024
	maxLANConcurrency = 64
	nameTimeout       = 700 * time.Millisecond
)

// lanProbePorts are tried on every host; a refused connection also proves
// the host is alive.
var lanProbePorts = []int{80, 443, 22, 445, 139, 62078, 8080, 554, 9100}

// ScanLAN discovers devices on a private subnet. Hosts are considered alive
// when they answer ICMP (where available) or any TCP probe (connected or
// refused); the ARP/neighbour table then adds MAC addresses and silent
// hosts. Names come from reverse DNS, NetBIOS and mDNS. onDevice is called
// once per device after enrichment, onProgress during the liveness sweep;
// both may be nil. Only RFC 1918, CGNAT and link-local subnets of at most
// 1024 hosts are accepted. The returned slice is sorted by IP.
func ScanLAN(ctx context.Context, opts ScanOptions, onDevice func(Device), onProgress func(done, total int)) ([]Device, error) {
	info, infoErr := LocalNetInfo()
	pfx, err := scanPrefix(opts.Subnet, info)
	if err != nil {
		if infoErr != nil {
			return nil, errors.Join(err, infoErr)
		}
		return nil, err
	}
	hosts := prefixHosts(pfx)
	if opts.Timeout <= 0 {
		opts.Timeout = 600 * time.Millisecond
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 32
	}
	if opts.Concurrency > maxLANConcurrency {
		opts.Concurrency = maxLANConcurrency
	}

	type liveInfo struct {
		latency time.Duration
		ports   []int
	}
	var mu sync.Mutex
	alive := make(map[string]*liveInfo)
	markAlive := func(ip string, lat time.Duration, port int) {
		mu.Lock()
		defer mu.Unlock()
		li := alive[ip]
		if li == nil {
			li = &liveInfo{latency: lat}
			alive[ip] = li
		}
		if lat > 0 && (li.latency == 0 || lat < li.latency) {
			li.latency = lat
		}
		if port > 0 {
			li.ports = append(li.ports, port)
		}
	}

	// 1. ICMP sweep over one shared socket (not on Windows, see nativeEcho).
	if runtime.GOOS != "windows" {
		for ip, rtt := range icmpSweep(ctx, hosts, opts.Timeout+300*time.Millisecond) {
			markAlive(ip, rtt, 0)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 2. TCP (and native ICMP on Windows) probes per host.
	total := len(hosts)
	done := 0
	jobs := make(chan net.IP)
	var wg sync.WaitGroup
	for i := 0; i < opts.Concurrency && i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				probeHost(ctx, ip, opts.Timeout, markAlive)
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				if onProgress != nil {
					onProgress(d, total)
				}
			}
		}()
	}
feed:
	for _, ip := range hosts {
		select {
		case jobs <- ip:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 3. Neighbour table: MACs plus hosts that ignored every probe.
	neigh := readNeighbors(ctx)
	for ipS := range neigh {
		if a, err := netip.ParseAddr(ipS); err == nil && pfx.Contains(a) {
			markAlive(ipS, 0, 0)
		}
	}
	selfMAC := ""
	if info.LocalIP != "" {
		if a, err := netip.ParseAddr(info.LocalIP); err == nil && pfx.Contains(a) {
			markAlive(info.LocalIP, 0, 0)
			selfMAC = interfaceMAC(info, info.LocalIP)
		}
	}

	// 4. Enrich and report, with bounded parallelism.
	devices := make([]Device, 0, len(alive))
	for ipS, li := range alive {
		sort.Ints(li.ports)
		d := Device{IP: ipS, MAC: neigh[ipS], Latency: li.latency, OpenPorts: li.ports}
		d.IsSelf = ipS == info.LocalIP
		d.IsGateway = ipS == info.Gateway
		if d.IsSelf && d.MAC == "" {
			d.MAC = selfMAC
		}
		devices = append(devices, d)
	}
	sortDevices(devices)
	sem := make(chan struct{}, 16)
	var ewg sync.WaitGroup
	for i := range devices {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		ewg.Add(1)
		go func(d *Device) {
			defer ewg.Done()
			defer func() { <-sem }()
			enrichDevice(ctx, d, info.Hostname)
			if onDevice != nil && ctx.Err() == nil {
				onDevice(*d)
			}
		}(&devices[i])
	}
	ewg.Wait()
	if err := ctx.Err(); err != nil {
		return devices, err
	}
	return devices, nil
}

// scanPrefix validates the requested subnet (or derives the local /24).
func scanPrefix(subnet string, info NetInfo) (netip.Prefix, error) {
	s := strings.TrimSpace(subnet)
	if s == "" {
		if info.LocalIP == "" {
			return netip.Prefix{}, errors.New("lan scan: no local IPv4 address")
		}
		a, err := netip.ParseAddr(info.LocalIP)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("lan scan: %w", err)
		}
		bits := 24
		if p, err := netip.ParsePrefix(info.Subnet); err == nil && p.Bits() > 24 {
			bits = p.Bits()
		}
		s = netip.PrefixFrom(a, bits).Masked().String()
	}
	if !strings.Contains(s, "/") {
		s += "/32"
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("lan scan: %w", err)
	}
	p = p.Masked()
	if !p.Addr().Is4() {
		return netip.Prefix{}, errors.New("lan scan: only IPv4 subnets are supported")
	}
	if !isPrivateAddr(p.Addr()) || p.Bits() < 8 {
		return netip.Prefix{}, fmt.Errorf("lan scan: %s is not a private subnet", p)
	}
	if p.Bits() < 22 {
		return netip.Prefix{}, fmt.Errorf("lan scan: %s has more than %d hosts", p, maxLANHosts)
	}
	return p, nil
}

// prefixHosts lists the host addresses of an IPv4 prefix (network and
// broadcast excluded for prefixes shorter than /31).
func prefixHosts(p netip.Prefix) []net.IP {
	var out []net.IP
	first := p.Addr()
	n := 1 << (32 - p.Bits())
	a := first
	for i := 0; i < n && len(out) < maxLANHosts; i++ {
		if p.Bits() >= 31 || (i != 0 && i != n-1) {
			b := a.As4()
			out = append(out, net.IPv4(b[0], b[1], b[2], b[3]).To4())
		}
		a = a.Next()
	}
	return out
}

// probeHost runs the TCP probes (and native ICMP where available) for ip
// concurrently and records every positive result.
func probeHost(ctx context.Context, ip net.IP, timeout time.Duration, mark func(ip string, lat time.Duration, port int)) {
	var wg sync.WaitGroup
	ipS := ip.String()
	for _, port := range lanProbePorts {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			d := net.Dialer{Timeout: timeout}
			start := time.Now()
			c, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ipS, fmt.Sprint(port)))
			lat := time.Since(start)
			switch {
			case err == nil:
				_ = c.Close() // the connection was only a probe
				mark(ipS, lat, port)
			case isConnRefused(err):
				mark(ipS, lat, 0)
			}
		}(port)
	}
	if runtime.GOOS == "windows" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, rtt, err := nativeEcho(ip, timeout); err == nil && ok {
				if rtt == 0 {
					rtt = time.Millisecond / 2
				}
				mark(ipS, rtt, 0)
			}
		}()
	}
	wg.Wait()
}

// interfaceMAC returns the MAC of the interface holding ip.
func interfaceMAC(info NetInfo, ip string) string {
	for _, it := range info.Interfaces {
		for _, a := range it.Addrs {
			if strings.HasPrefix(a, ip+"/") {
				return it.MAC
			}
		}
	}
	return ""
}

// sortDevices orders devices by numeric IP.
func sortDevices(ds []Device) {
	sort.Slice(ds, func(i, j int) bool {
		a, b := net.ParseIP(ds[i].IP).To4(), net.ParseIP(ds[j].IP).To4()
		return bytes.Compare(a, b) < 0
	})
}

// enrichDevice fills vendor, host name and kind.
func enrichDevice(ctx context.Context, d *Device, selfHost string) {
	if d.MAC != "" {
		d.Vendor = Vendor(d.MAC)
	}
	ip := net.ParseIP(d.IP)
	if d.IsSelf {
		d.Hostname = selfHost
	}
	if d.Hostname == "" {
		d.Hostname = reverseLookup(ctx, d.IP)
	}
	if d.Hostname == "" {
		d.Hostname = netbiosName(ctx, ip, nameTimeout)
	}
	if d.Hostname == "" {
		d.Hostname = mdnsReverse(ctx, ip, nameTimeout)
	}
	d.Kind = guessKind(*d)
}

// guessKind classifies a device from its role, vendor, name and ports.
func guessKind(d Device) string {
	if d.IsGateway {
		return "router"
	}
	text := strings.ToLower(d.Vendor + " " + d.Hostname)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	port := func(p int) bool {
		for _, x := range d.OpenPorts {
			if x == p {
				return true
			}
		}
		return false
	}
	switch {
	case port(9100) || has("printer", "epson", "brother", "canon", "lexmark", "kyocera", "xerox", "ricoh", "hp-", "laserjet", "officejet"):
		return "printer"
	case port(62078) || has("iphone", "ipad", "android", "galaxy", "pixel", "phone", "xiaomi", "oneplus", "oppo", "vivo", "huawei device", "motorola"):
		return "phone"
	case port(554) || has("camera", "hikvision", "dahua", "axis comm", "reolink", "ipcam", "wyze", "arlo", "ezviz"):
		return "camera"
	case has("tv", "roku", "chromecast", "bravia", "webos", "tizen", "fire stick", "firetv", "apple tv", "appletv", "vizio", "tcl", "hisense"):
		return "tv"
	case has("sonos", "echo", "speaker", "homepod", "bose", "google home", "nest audio", "amazon technologies"):
		return "speaker"
	case has("router", "gateway", "tp-link", "netgear", "ubiquiti", "mikrotik", "linksys", "asustek", "zyxel", "fritz", "avm", "arris", "technicolor", "sagemcom"):
		return "router"
	case has("espressif", "tuya", "shelly", "sonoff", "philips lighting", "signify", "nest", "ecobee", "raspberry", "particle", "smart", "lifx", "wiz"):
		return "iot"
	case port(445) || port(139) || port(22) || has("desktop", "laptop", "macbook", "imac", "-pc", "dell", "lenovo", "intel corporate", "microsoft", "asus", "acer", "gigabyte", "msi"):
		return "computer"
	case d.IsSelf:
		return "computer"
	}
	if IsRandomMAC(d.MAC) {
		return "phone" // randomised MACs are typical of phones' private Wi-Fi addresses
	}
	return "unknown"
}
