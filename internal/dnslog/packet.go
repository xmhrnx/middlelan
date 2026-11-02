package dnslog

import "encoding/binary"

const (
	protoUDP = 17
	dnsPort  = 53
)

// dnsPayload returns the UDP payload of an IPv4 or IPv6 packet sent from
// port 53, or false when pkt is anything else. IPv4 fragments other than
// the first and IPv6 packets with extension headers are not handled.
func dnsPayload(pkt []byte) ([]byte, bool) {
	if len(pkt) < 1 {
		return nil, false
	}
	var udp []byte
	switch pkt[0] >> 4 {
	case 4:
		if len(pkt) < 20 {
			return nil, false
		}
		ihl := int(pkt[0]&0x0f) * 4
		total := int(binary.BigEndian.Uint16(pkt[2:4]))
		if ihl < 20 || pkt[9] != protoUDP || binary.BigEndian.Uint16(pkt[6:8])&0x1fff != 0 {
			return nil, false
		}
		if total < len(pkt) && total >= ihl {
			pkt = pkt[:total] // drop link-layer padding
		}
		if len(pkt) < ihl {
			return nil, false
		}
		udp = pkt[ihl:]
	case 6:
		if len(pkt) < 40 || pkt[6] != protoUDP {
			return nil, false
		}
		if n := 40 + int(binary.BigEndian.Uint16(pkt[4:6])); n < len(pkt) {
			pkt = pkt[:n]
		}
		udp = pkt[40:]
	default:
		return nil, false
	}
	if len(udp) < 8 || binary.BigEndian.Uint16(udp[0:2]) != dnsPort {
		return nil, false
	}
	if n := int(binary.BigEndian.Uint16(udp[4:6])); n >= 8 && n < len(udp) {
		udp = udp[:n]
	}
	return udp[8:], true
}
