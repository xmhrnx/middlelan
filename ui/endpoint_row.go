package ui

import (
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/geo"
)

// endpointRow shows one remote address: who it is, where, and how much
// traffic went there.
func endpointRow(info geo.Info, port int, proto string, bytes uint64, active bool, lastSeen, now time.Time) giode.Element {
	title := info.Name()
	details := []string{}
	if info.Domain != "" && info.Host != "" && info.Host != info.Domain {
		details = append(details, info.Host)
	}
	if o := info.Owner(); o != "" {
		details = append(details, o)
	}
	if p := placeName(info.Place()); p != "" {
		details = append(details, p)
	}
	details = append(details, strings.ToUpper(strings.TrimSuffix(proto, "6"))+" "+serviceLabel(port))
	status := pill(tr("Active"), "theme.success")
	if !active {
		status = giode.Text(formatAgo(lastSeen, now), styleMuted)
	}
	right := []giode.Element{status}
	if bytes > 0 {
		right = append(right, giode.Text(formatBytes(bytes), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightMedium}))
	}
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(4, 8), Width: -1, Wrap: giode.WrapNoWrap},
		countryBadge(info.CountryCode, info.Scope),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Text(title, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(strings.Join(details, " · "), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(endpointLabel(info.IP, port), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, FontFamily: fontMono}),
		),
		giode.Stack(giode.Styles{Align: giode.AlignEnd, Gap: 4}, right...),
	)
}

// wellKnown names common ports.
var wellKnown = map[int]string{
	20: "FTP data", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 53: "DNS", 67: "DHCP", 68: "DHCP",
	80: "HTTP", 110: "POP3", 123: "NTP", 137: "NetBIOS", 138: "NetBIOS", 139: "NetBIOS", 143: "IMAP",
	161: "SNMP", 389: "LDAP", 443: "HTTPS", 445: "SMB", 465: "SMTPS", 500: "IKE", 514: "Syslog",
	554: "RTSP", 587: "SMTP", 631: "IPP", 853: "DNS over TLS", 993: "IMAPS", 995: "POP3S", 1194: "OpenVPN",
	1900: "SSDP", 3306: "MySQL", 3389: "RDP", 3478: "STUN", 4500: "IPsec", 5060: "SIP", 5222: "XMPP",
	5228: "Google Play", 5353: "mDNS", 5432: "PostgreSQL", 5900: "VNC", 6379: "Redis", 8080: "HTTP alt",
	8443: "HTTPS alt", 9000: "HTTP alt", 19302: "Google STUN", 27017: "MongoDB", 51820: "WireGuard",
}

// serviceLabel is "443 (HTTPS)" or "8123".
func serviceLabel(port int) string {
	if n, ok := wellKnown[port]; ok {
		return itoa(port) + " (" + n + ")"
	}
	return itoa(port)
}
