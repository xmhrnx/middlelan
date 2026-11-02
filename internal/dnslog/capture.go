package dnslog

import (
	"encoding/binary"
	"time"
)

// handlePacket records the DNS answers in an IP packet, if it carries any.
func (l *Log) handlePacket(pkt []byte, at time.Time) {
	payload, ok := dnsPayload(pkt)
	if !ok {
		return
	}
	for _, a := range ParseResponse(payload, at) {
		l.add(a)
	}
}

// handleBPF records the answers in one BPF read, which holds any number of
// packets, each behind a bpf_hdr and padded to a 4-byte boundary.
func (l *Log) handleBPF(buf []byte, linkLen int, at time.Time) {
	// bh_hdrlen may be 18, less than sizeof(struct bpf_hdr), when that
	// aligns the IP header; the fields read here end at byte 18.
	const hdrMin = 18
	for len(buf) >= hdrMin {
		caplen := int(binary.NativeEndian.Uint32(buf[8:12]))
		hdrlen := int(binary.NativeEndian.Uint16(buf[16:18]))
		end := hdrlen + caplen
		if hdrlen < hdrMin || end > len(buf) {
			return
		}
		if pkt := buf[hdrlen:end]; len(pkt) > linkLen {
			l.handlePacket(pkt[linkLen:], at)
		}
		next := (end + 3) &^ 3
		if next >= len(buf) {
			return
		}
		buf = buf[next:]
	}
}
