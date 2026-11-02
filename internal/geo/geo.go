// Package geo tells where an IP address is: its host name, owner, country
// and coordinates. Lookups are cached, batched and done in the background,
// so callers never wait on the network.
package geo

import (
	"net"
	"strings"
)

// Scope classifies an address.
type Scope string

// Address scopes.
const (
	ScopePublic    Scope = "Internet"
	ScopePrivate   Scope = "Local network"
	ScopeLoopback  Scope = "This device"
	ScopeLinkLocal Scope = "Link-local"
	ScopeMulticast Scope = "Multicast"
	ScopeInvalid   Scope = "Unknown"
)

// Info is what is known about an address.
type Info struct {
	IP string `json:"ip"`
	// Domain is the name an app looked up to reach this address (from
	// observed DNS answers); more telling than Host, the reverse-DNS name.
	Domain      string  `json:"-"`
	Host        string  `json:"host,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"cc,omitempty"`
	Region      string  `json:"region,omitempty"`
	City        string  `json:"city,omitempty"`
	Org         string  `json:"org,omitempty"`
	ASN         int     `json:"asn,omitempty"`
	Lat         float64 `json:"lat,omitempty"`
	Lon         float64 `json:"lon,omitempty"`
	Scope       Scope   `json:"scope"`
	Located     bool    `json:"located,omitempty"`
	HostDone    bool    `json:"hostDone,omitempty"`
	// Unlocatable is set when an online lookup was tried and failed (or
	// found nothing); the place stays unknown until a later retry.
	Unlocatable bool `json:"-"`
}

// Place is a short human description of where the address is.
func (i Info) Place() string {
	if i.Scope != ScopePublic {
		return string(i.Scope)
	}
	switch {
	case i.Unlocatable && i.Country == "":
		return "Location unknown"
	case i.City != "" && i.Country != "":
		return i.City + ", " + i.Country
	case i.Country != "":
		return i.Country
	}
	return ""
}

// Owner is the company or service behind the address, best effort.
func (i Info) Owner() string {
	if s := ServiceName(i.Domain, ""); s != "" {
		return s
	}
	if s := ServiceName(i.Host, i.Org); s != "" {
		return s
	}
	return i.Org
}

// Name is the best name for the address: the looked-up domain, else the
// reverse-DNS host, else the IP.
func (i Info) Name() string {
	switch {
	case i.Domain != "":
		return i.Domain
	case i.Host != "":
		return i.Host
	}
	return i.IP
}

// Classify returns the scope of an IP address.
func Classify(ip string) Scope {
	a := net.ParseIP(ip)
	switch {
	case a == nil || a.IsUnspecified():
		return ScopeInvalid
	case a.IsLoopback():
		return ScopeLoopback
	case a.IsMulticast() || a.Equal(net.IPv4bcast):
		return ScopeMulticast
	case a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast():
		return ScopeLinkLocal
	case a.IsPrivate() || isCGNAT(a):
		return ScopePrivate
	}
	return ScopePublic
}

func isCGNAT(a net.IP) bool {
	v4 := a.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

// services maps host name and owner fragments to friendly names.
var services = []struct{ match, name string }{
	{"1e100.net", "Google"}, {"google", "Google"}, {"youtube", "YouTube"}, {"googlevideo", "YouTube"},
	{"gstatic", "Google"}, {"amazonaws", "Amazon AWS"}, {"amazon", "Amazon"}, {"cloudfront", "Amazon CloudFront"},
	{"akamai", "Akamai"}, {"akam", "Akamai"}, {"edgekey", "Akamai"}, {"cloudflare", "Cloudflare"},
	{"fastly", "Fastly"}, {"fbcdn", "Meta"}, {"facebook", "Meta"}, {"instagram", "Instagram"},
	{"whatsapp", "WhatsApp"}, {"apple", "Apple"}, {"icloud", "Apple iCloud"}, {"aaplimg", "Apple"},
	{"microsoft", "Microsoft"}, {"msedge", "Microsoft"}, {"azure", "Microsoft Azure"}, {"windows", "Microsoft"},
	{"live.com", "Microsoft"}, {"office", "Microsoft 365"}, {"github", "GitHub"}, {"netflix", "Netflix"},
	{"nflx", "Netflix"}, {"spotify", "Spotify"}, {"discord", "Discord"}, {"slack", "Slack"},
	{"zoom", "Zoom"}, {"twitter", "X (Twitter)"}, {"twimg", "X (Twitter)"}, {"tiktok", "TikTok"},
	{"bytedance", "ByteDance"}, {"steam", "Steam"}, {"valve", "Valve"}, {"dropbox", "Dropbox"},
	{"digitalocean", "DigitalOcean"}, {"linode", "Akamai Linode"}, {"hetzner", "Hetzner"}, {"ovh", "OVH"},
	{"anthropic", "Anthropic"}, {"openai", "OpenAI"}, {"telegram", "Telegram"}, {"mozilla", "Mozilla"},
}

// ServiceName guesses the service behind a host name or network owner.
func ServiceName(host, org string) string {
	h, o := strings.ToLower(host), strings.ToLower(org)
	for _, s := range services {
		if strings.Contains(h, s.match) {
			return s.name
		}
	}
	for _, s := range services {
		if strings.Contains(o, s.match) {
			return s.name
		}
	}
	return ""
}
