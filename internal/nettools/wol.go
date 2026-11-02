package nettools

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"
)

// WakeOnLAN sends a Wake-on-LAN magic packet for mac to UDP port 9 of
// broadcast (default 255.255.255.255; a "host:port" may override the port).
func WakeOnLAN(mac, broadcast string) error {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil {
		return fmt.Errorf("wol: %w", err)
	}
	if len(hw) != 6 {
		return fmt.Errorf("wol: %q is not a 48-bit MAC address", mac)
	}
	broadcast = strings.TrimSpace(broadcast)
	if broadcast == "" {
		broadcast = "255.255.255.255"
	}
	addr := broadcast
	if _, _, err := net.SplitHostPort(broadcast); err != nil {
		addr = net.JoinHostPort(broadcast, "9")
	}
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return fmt.Errorf("wol: %w", err)
	}
	packet := append(bytes.Repeat([]byte{0xff}, 6), bytes.Repeat(hw, 16)...)
	c, err := net.DialUDP("udp4", nil, ua)
	if err != nil {
		return fmt.Errorf("wol: %w", err)
	}
	if err := c.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		_ = c.Close()
		return fmt.Errorf("wol: %w", err)
	}
	if _, err := c.Write(packet); err != nil {
		_ = c.Close()
		return fmt.Errorf("wol: %w", err)
	}
	return c.Close()
}
