// Package nettools implements the network diagnostics used by MiddleLAN:
// ping, traceroute, port scanning, DNS and WHOIS lookups, Wake-on-LAN,
// speed testing, subnet math, TLS inspection, local/public network
// information, MAC vendor lookup, LAN device discovery, UPnP/mDNS browsing
// and a gateway configuration health check.
//
// The package is pure Go (no cgo) and has no UI dependencies. Every
// long-running function honours context cancellation, uses short timeouts
// and bounded concurrency. Callbacks may be invoked from any goroutine; the
// caller is responsible for handing results over to its UI thread.
package nettools
