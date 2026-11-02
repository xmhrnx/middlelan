package nettools

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Severity ranks a router check finding.
type Severity int

const (
	// SevInfo is informational.
	SevInfo Severity = iota
	// SevLow is a minor hardening suggestion.
	SevLow
	// SevMedium should be reviewed.
	SevMedium
	// SevHigh should be fixed.
	SevHigh
)

// String returns the severity label.
func (s Severity) String() string {
	switch s {
	case SevInfo:
		return "info"
	case SevLow:
		return "low"
	case SevMedium:
		return "medium"
	case SevHigh:
		return "high"
	}
	return "unknown"
}

// Finding is one result of CheckRouter.
type Finding struct {
	// Severity ranks the finding.
	Severity Severity
	// Title is a one-line summary; Detail explains it and gives advice.
	Title, Detail string
}

// routerPorts are the management services checked on the gateway.
var routerPorts = []int{21, 22, 23, 53, 80, 139, 443, 445, 7547, 8080, 8443}

// CheckRouter runs a configuration health check of the user's own gateway
// (default: the detected default gateway): which management services it
// exposes on the LAN, whether UPnP IGD is enabled and whether DNS answers
// for non-existent names (a sign of DNS hijacking). Findings are sorted by
// severity, most severe first, and always end with an Info summary.
func CheckRouter(ctx context.Context, gateway string) ([]Finding, error) {
	gateway = cleanHost(gateway)
	if gateway == "" {
		info, err := LocalNetInfo()
		if err != nil {
			return nil, err
		}
		gateway = info.Gateway
	}
	ip := net.ParseIP(gateway)
	if ip == nil {
		return nil, fmt.Errorf("router check: %q is not an IP address", gateway)
	}
	if !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !isCGNAT(ip) {
		return nil, fmt.Errorf("router check: %s is not a private (LAN) address", ip)
	}

	open := make(map[int]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range routerPorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			d := net.Dialer{Timeout: 1200 * time.Millisecond}
			c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), fmt.Sprint(p)))
			if err != nil {
				return
			}
			_ = c.Close() // probe only
			mu.Lock()
			open[p] = true
			mu.Unlock()
		}(p)
	}
	var igd bool
	var igdServer string
	wg.Add(1)
	go func() {
		defer wg.Done()
		resps, err := ssdpSearch(ctx, "urn:schemas-upnp-org:device:InternetGatewayDevice:1", 2*time.Second)
		if err != nil && len(resps) == 0 {
			return
		}
		for _, r := range resps {
			if r.IP == ip.String() {
				igd, igdServer = true, r.Server
				return
			}
		}
	}()
	hijacked, hijackAddr, dnsErr := dnsHijackCheck(ctx)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var findings []Finding
	var fine []string
	add := func(s Severity, title, detail string) {
		findings = append(findings, Finding{Severity: s, Title: title, Detail: detail})
	}
	if open[23] {
		add(SevHigh, "Telnet is enabled (port 23)",
			"Telnet sends passwords in clear text. Disable Telnet in the router's administration settings and use the web interface or SSH instead.")
	} else {
		fine = append(fine, "Telnet is closed")
	}
	if open[21] {
		add(SevMedium, "FTP server is running (port 21)",
			"FTP is unencrypted and is often used for USB storage sharing. Disable it if you do not need it, or make sure it requires a strong password and is not reachable from the internet.")
	} else {
		fine = append(fine, "FTP is closed")
	}
	httpAdmin := open[80] || open[8080]
	httpsAdmin := open[443] || open[8443]
	switch {
	case httpAdmin && !httpsAdmin:
		add(SevMedium, "Admin page is only available over plain HTTP",
			"Your router login is sent unencrypted over the network. Enable HTTPS administration if the router supports it, use a strong admin password, and avoid logging in over guest or shared Wi-Fi.")
	case httpAdmin && httpsAdmin:
		add(SevLow, "Admin page is also reachable over plain HTTP",
			"HTTPS administration is available; prefer it (or enable HTTP-to-HTTPS redirection) so your password is never sent in clear text.")
	case httpsAdmin:
		fine = append(fine, "Admin page uses HTTPS")
	}
	if open[22] {
		add(SevLow, "SSH is reachable (port 22)",
			"Fine if you use it. Otherwise disable SSH, and make sure it uses key authentication or a strong password.")
	}
	if open[445] || open[139] {
		add(SevMedium, "File sharing (SMB) is enabled on the router",
			"Usually provided for attached USB storage. Disable it if unused, require a password, and make sure SMBv1 is turned off.")
	} else {
		fine = append(fine, "SMB file sharing is off")
	}
	if open[7547] {
		add(SevMedium, "TR-069 remote management port is open on the LAN (7547)",
			"This ISP remote-management service has a history of vulnerabilities. Keep the firmware up to date; ask your ISP whether it can be restricted.")
	}
	if igd {
		detail := "UPnP lets any device on your network open ports to the internet without asking. Disable UPnP unless an application (games, video calls) really needs it."
		if igdServer != "" {
			detail += " Responder: " + igdServer + "."
		}
		add(SevLow, "UPnP Internet Gateway Device is enabled", detail)
	} else {
		fine = append(fine, "UPnP IGD did not respond")
	}
	switch {
	case dnsErr != nil:
		add(SevInfo, "DNS check could not be completed", dnsErr.Error())
	case hijacked:
		add(SevHigh, "DNS returns addresses for non-existent domains",
			"A lookup for a random, non-existent name returned "+hijackAddr+". Your router or ISP may be redirecting DNS (for ads or after a compromise). Check the DNS servers configured in the router, change the admin password and update the firmware.")
	default:
		fine = append(fine, "DNS correctly reports non-existent domains")
	}
	if open[53] {
		fine = append(fine, "router provides DNS on the LAN")
	}

	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Severity > findings[j].Severity })
	summary := "No issues detected in the checked areas."
	if len(fine) > 0 {
		summary = "Looks good: " + strings.Join(fine, "; ") + "."
	}
	findings = append(findings, Finding{Severity: SevInfo, Title: "Checked " + ip.String(), Detail: summary})
	return findings, nil
}

// isCGNAT reports whether ip is in the 100.64.0.0/10 carrier-grade NAT range.
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

// dnsHijackCheck resolves a random name that cannot exist and reports
// whether the resolver nevertheless returned an address.
func dnsHijackCheck(ctx context.Context) (bool, string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 20)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	name := "mlan-" + string(b) + ".com"
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	if err != nil {
		var de *net.DNSError
		if errors.As(err, &de) && de.IsNotFound {
			return false, "", nil
		}
		return false, "", fmt.Errorf("lookup of a test name failed: %w", err)
	}
	if len(addrs) == 0 {
		return false, "", nil
	}
	return true, strings.Join(addrs, ", "), nil
}
