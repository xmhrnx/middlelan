package nettools

//go:generate go run ./internal/ouigen -out oui_data.gz

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
)

//go:embed oui_data.gz
var ouiData []byte

var (
	ouiOnce  sync.Once
	ouiTable map[string]string
)

// loadOUI decompresses the embedded registry. A corrupt table simply yields
// no vendor names.
func loadOUI() {
	ouiTable = make(map[string]string, 60000)
	zr, err := gzip.NewReader(bytes.NewReader(ouiData))
	if err != nil {
		return
	}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "\t"); ok {
			ouiTable[k] = v
		}
	}
	// A truncated stream leaves the entries read so far; nothing else to do.
	_ = zr.Close()
}

// normalizeMAC returns the upper-case hex digits of mac without separators.
func normalizeMAC(mac string) string {
	var b strings.Builder
	for _, r := range mac {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r - 'a' + 'A')
		case r == ':' || r == '-' || r == '.' || r == ' ':
		default:
			return ""
		}
	}
	return b.String()
}

// Vendor returns the registered manufacturer for a MAC address (any common
// notation), or "" when unknown. Locally administered (randomised) addresses
// return "". The table is loaded lazily on first use.
func Vendor(mac string) string {
	h := normalizeMAC(mac)
	if len(h) < 6 || IsRandomMAC(h) {
		return ""
	}
	ouiOnce.Do(loadOUI)
	for _, n := range []int{9, 7, 6} {
		if len(h) >= n {
			if v, ok := ouiTable[h[:n]]; ok {
				return v
			}
		}
	}
	return ""
}

// IsRandomMAC reports whether mac is locally administered, as used by
// phones' private Wi-Fi addresses.
func IsRandomMAC(mac string) bool {
	h := normalizeMAC(mac)
	if len(h) < 2 {
		return false
	}
	return strings.ContainsRune("2367ABEF", rune(h[1]))
}
