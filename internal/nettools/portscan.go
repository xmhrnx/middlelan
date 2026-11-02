package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// PortResult describes an open TCP port.
type PortResult struct {
	// Port is the TCP port number.
	Port int
	// Open is true when a TCP connection could be established.
	Open bool
	// Service is the well-known service name for Port, if any.
	Service string
	// Banner is the first line the service sent on connect, if any.
	Banner string
	// Latency is the TCP connect time.
	Latency time.Duration
}

// CommonPorts lists about a hundred frequently used TCP ports, suitable for a
// quick scan.
var CommonPorts = []int{
	7, 20, 21, 22, 23, 25, 37, 53, 69, 79, 80, 81, 82, 88, 106, 110, 111, 113,
	119, 135, 139, 143, 161, 179, 199, 389, 427, 443, 444, 445, 465, 513, 514,
	515, 543, 544, 548, 554, 587, 631, 646, 873, 990, 993, 995, 1025, 1026,
	1027, 1028, 1029, 1080, 1110, 1433, 1521, 1720, 1723, 1755, 1883, 1900,
	2000, 2001, 2049, 2121, 2717, 3000, 3128, 3306, 3389, 3986, 4899, 5000,
	5009, 5051, 5060, 5101, 5190, 5357, 5432, 5631, 5666, 5800, 5900, 6000,
	6001, 6646, 7070, 8000, 8008, 8009, 8080, 8081, 8443, 8888, 9000, 9100,
	9999, 10000, 32768, 49152, 49153, 49154, 62078,
}

const (
	maxScanConcurrency = 64
	bannerTimeout      = 500 * time.Millisecond
	maxPortSpecEntries = 65535
)

var serviceNames = map[int]string{
	7: "echo", 20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp",
	37: "time", 53: "dns", 67: "dhcp", 69: "tftp", 79: "finger", 80: "http",
	81: "http-alt", 82: "http-alt", 88: "kerberos", 106: "pop3pw", 110: "pop3",
	111: "rpcbind", 113: "ident", 119: "nntp", 123: "ntp", 135: "msrpc",
	137: "netbios-ns", 138: "netbios-dgm", 139: "netbios-ssn", 143: "imap",
	161: "snmp", 179: "bgp", 199: "smux", 389: "ldap", 427: "svrloc",
	443: "https", 444: "snpp", 445: "microsoft-ds", 465: "smtps", 513: "rlogin",
	514: "shell", 515: "printer", 543: "klogin", 544: "kshell", 548: "afp",
	554: "rtsp", 587: "submission", 631: "ipp", 636: "ldaps", 646: "ldp",
	853: "dns-over-tls", 873: "rsync", 990: "ftps", 993: "imaps", 995: "pop3s",
	1025: "nfs-or-iis", 1080: "socks", 1194: "openvpn", 1433: "mssql",
	1521: "oracle", 1701: "l2tp", 1720: "h323", 1723: "pptp", 1755: "wms",
	1883: "mqtt", 1900: "upnp", 2049: "nfs", 2121: "ftp-alt", 2375: "docker",
	3000: "http-dev", 3128: "squid", 3306: "mysql", 3389: "rdp", 3478: "stun",
	4899: "radmin", 5000: "upnp", 5060: "sip", 5222: "xmpp", 5353: "mdns",
	5357: "wsdapi", 5432: "postgresql", 5631: "pcanywhere", 5666: "nrpe",
	5800: "vnc-http", 5900: "vnc", 6000: "x11", 6379: "redis", 6646: "mcafee",
	7070: "realserver", 8000: "http-alt", 8008: "http-alt", 8009: "ajp13",
	8080: "http-proxy", 8081: "http-alt", 8443: "https-alt", 8883: "mqtt-tls",
	8888: "http-alt", 9000: "http-alt", 9100: "jetdirect", 9200: "elasticsearch",
	9999: "abyss", 10000: "webmin", 11211: "memcached", 27017: "mongodb",
	32400: "plex", 49152: "upnp", 62078: "iphone-sync",
}

// ServiceName returns the well-known service name for a TCP port, or "" if
// unknown.
func ServiceName(port int) string { return serviceNames[port] }

// ParsePortSpec parses a port list such as "22,80,8000-8100" into a sorted,
// de-duplicated slice. Ports must be in 1..65535.
func ParsePortSpec(s string) ([]int, error) {
	seen := make(map[int]bool)
	var ports []int
	add := func(p int) error {
		if !seen[p] {
			if len(ports) >= maxPortSpecEntries {
				return errors.New("port spec: too many ports")
			}
			seen[p] = true
			ports = append(ports, p)
		}
		return nil
	}
	parse := func(t string) (int, error) {
		p, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, fmt.Errorf("port spec: invalid port %q", strings.TrimSpace(t))
		}
		if p < 1 || p > 65535 {
			return 0, fmt.Errorf("port spec: port %d out of range", p)
		}
		return p, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) })
	if len(fields) == 0 {
		return nil, errors.New("port spec: empty")
	}
	for _, f := range fields {
		if lo, hi, ok := strings.Cut(f, "-"); ok {
			a, err := parse(lo)
			if err != nil {
				return nil, err
			}
			b, err := parse(hi)
			if err != nil {
				return nil, err
			}
			if a > b {
				return nil, fmt.Errorf("port spec: invalid range %q", f)
			}
			for p := a; p <= b; p++ {
				if err := add(p); err != nil {
					return nil, err
				}
			}
			continue
		}
		p, err := parse(f)
		if err != nil {
			return nil, err
		}
		if err := add(p); err != nil {
			return nil, err
		}
	}
	sort.Ints(ports)
	return ports, nil
}

// ScanPorts performs TCP connect checks of ports on host. concurrency is
// clamped to 1..64 (default 32) and timeout defaults to 1s per port. Only
// open ports are passed to onResult; onProgress receives the number of ports
// checked so far. Both callbacks may be nil and may run on any goroutine.
func ScanPorts(ctx context.Context, host string, ports []int, concurrency int, timeout time.Duration, onResult func(PortResult), onProgress func(done, total int)) error {
	ip, err := resolveIP(ctx, host)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		ports = CommonPorts
	}
	if concurrency <= 0 {
		concurrency = 32
	}
	if concurrency > maxScanConcurrency {
		concurrency = maxScanConcurrency
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	total := len(ports)
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i := 0; i < concurrency && i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				if r, ok := checkPort(ctx, ip, port, timeout); ok && onResult != nil {
					onResult(r)
				}
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
	for _, p := range ports {
		select {
		case jobs <- p:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}

// checkPort tries a TCP connection and, when open, grabs a short banner.
func checkPort(ctx context.Context, ip net.IP, port int, timeout time.Duration) (PortResult, bool) {
	if ctx.Err() != nil {
		return PortResult{}, false
	}
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return PortResult{}, false
	}
	r := PortResult{Port: port, Open: true, Service: ServiceName(port), Latency: time.Since(start)}
	r.Banner = readBanner(c, bannerTimeout)
	if err := c.Close(); err != nil {
		return r, true // the port was open regardless of close errors
	}
	return r, true
}

// readBanner reads what a server volunteers right after connecting and
// returns its first printable line.
func readBanner(c net.Conn, timeout time.Duration) string {
	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return ""
	}
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	if n == 0 || (err != nil && n == 0) {
		return ""
	}
	return sanitizeBanner(string(buf[:n]))
}

// sanitizeBanner keeps the first line and strips non-printable characters.
func sanitizeBanner(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	return strings.TrimSpace(s)
}
