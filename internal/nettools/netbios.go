package nettools

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"net"
	"strings"
	"time"
)

// netbiosName asks ip for its NetBIOS node status (UDP 137) and returns the
// workstation name, or "" when the host does not answer within timeout.
func netbiosName(ctx context.Context, ip net.IP, timeout time.Duration) string {
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: 137})
	if err != nil {
		return ""
	}
	defer conn.Close() //nolint:errcheck // UDP close cannot fail meaningfully
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return ""
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	id := uint16(rand.UintN(0xffff))
	if _, err := conn.Write(netbiosStatusRequest(id)); err != nil {
		return ""
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	name, err := parseNetbiosStatus(buf[:n], id)
	if err != nil {
		return ""
	}
	return name
}

// netbiosStatusRequest builds an NBSTAT query for the wildcard name "*".
func netbiosStatusRequest(id uint16) []byte {
	b := make([]byte, 12, 50)
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[4:], 1) // QDCOUNT
	b = append(b, 32)
	raw := make([]byte, 16)
	raw[0] = '*'
	for _, c := range raw {
		b = append(b, 'A'+(c>>4), 'A'+(c&0x0f))
	}
	b = append(b, 0)
	b = append(b, 0x00, 0x21, 0x00, 0x01) // NBSTAT, IN
	return b
}

// parseNetbiosStatus extracts the unique workstation (suffix 0x00) name from
// a node status response.
func parseNetbiosStatus(b []byte, id uint16) (string, error) {
	errShort := errors.New("netbios: short response")
	if len(b) < 12 || binary.BigEndian.Uint16(b) != id {
		return "", errors.New("netbios: unexpected response")
	}
	off := 12
	// Skip the answer name (labels or a compression pointer).
	for {
		if off >= len(b) {
			return "", errShort
		}
		l := int(b[off])
		if l&0xc0 == 0xc0 {
			off += 2
			break
		}
		off++
		if l == 0 {
			break
		}
		off += l
	}
	off += 2 + 2 + 4 + 2 // type, class, ttl, rdlength
	if off >= len(b) {
		return "", errShort
	}
	count := int(b[off])
	off++
	for i := 0; i < count; i++ {
		if off+18 > len(b) {
			break
		}
		name := strings.TrimRight(string(b[off:off+15]), " \x00")
		suffix := b[off+15]
		flags := binary.BigEndian.Uint16(b[off+16:])
		off += 18
		if suffix != 0x00 || name == "" {
			continue
		}
		if flags&0x8000 == 0 { // unique (non-group) name
			return name, nil
		}
	}
	return "", errors.New("netbios: no workstation name")
}
