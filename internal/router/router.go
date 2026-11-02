// Package router reads the home router's traffic counters over UPnP
// (Internet Gateway Device), which shows how busy the whole network is,
// every device included, without touching anyone's traffic.
package router

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoIGD means no router answered as a UPnP Internet Gateway Device
// (UPnP may be off in its settings).
var ErrNoIGD = errors.New("no UPnP router found (UPnP may be turned off on the router)")

const wanCommon = "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1"

// Gateway is a router that reports its WAN counters.
type Gateway struct {
	Name       string // friendly name, e.g. "FRITZ!Box 7590"
	Model      string
	Location   string
	controlURL string
	client     *http.Client
}

// Counters are the router's cumulative internet traffic.
type Counters struct {
	Rx, Tx       uint64
	LinkDownMbps float64 // the line's maximum, as the router reports it
	LinkUpMbps   float64
	At           time.Time
}

var searchTargets = []string{
	"urn:schemas-upnp-org:device:InternetGatewayDevice:2",
	"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
	"urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
}

// Discover finds the router on the local network.
func Discover(ctx context.Context) (*Gateway, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	for _, st := range searchTargets {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + st + "\r\n\r\n"
		if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
			return nil, err
		}
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return nil, ErrNoIGD
		}
		loc := header(string(buf[:n]), "location")
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		if g, err := describe(ctx, client, loc); err == nil {
			return g, nil
		}
	}
}

func header(resp, name string) string {
	for _, line := range strings.Split(resp, "\r\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// device is the part of a UPnP description we read.
type device struct {
	FriendlyName string    `xml:"friendlyName"`
	ModelName    string    `xml:"modelName"`
	Services     []service `xml:"serviceList>service"`
	Devices      []device  `xml:"deviceList>device"`
}

type service struct {
	Type       string `xml:"serviceType"`
	ControlURL string `xml:"controlURL"`
}

type description struct {
	URLBase string `xml:"URLBase"`
	Device  device `xml:"device"`
}

func describe(ctx context.Context, client *http.Client, location string) (*Gateway, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseDescription(body, location)
}

func parseDescription(body []byte, location string) (*Gateway, error) {
	var d description
	if err := xml.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	ctrl := findService(d.Device, wanCommon)
	if ctrl == "" {
		return nil, ErrNoIGD
	}
	base := location
	if d.URLBase != "" {
		base = d.URLBase
	}
	b, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	c, err := url.Parse(ctrl)
	if err != nil {
		return nil, err
	}
	return &Gateway{
		Name: d.Device.FriendlyName, Model: d.Device.ModelName, Location: location,
		controlURL: b.ResolveReference(c).String(), client: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

func findService(d device, typ string) string {
	for _, s := range d.Services {
		if s.Type == typ {
			return s.ControlURL
		}
	}
	for _, sub := range d.Devices {
		if c := findService(sub, typ); c != "" {
			return c
		}
	}
	return ""
}

// Counters reads the router's totals.
func (g *Gateway) Counters(ctx context.Context) (Counters, error) {
	c := Counters{At: time.Now()}
	rx, err := g.call(ctx, "GetTotalBytesReceived", "NewTotalBytesReceived")
	if err != nil {
		return c, err
	}
	tx, err := g.call(ctx, "GetTotalBytesSent", "NewTotalBytesSent")
	if err != nil {
		return c, err
	}
	c.Rx, _ = strconv.ParseUint(rx, 10, 64)
	c.Tx, _ = strconv.ParseUint(tx, 10, 64)
	if down, err := g.call(ctx, "GetCommonLinkProperties", "NewLayer1DownstreamMaxBitRate"); err == nil {
		v, _ := strconv.ParseFloat(down, 64)
		c.LinkDownMbps = v / 1e6
	}
	if up, err := g.call(ctx, "GetCommonLinkProperties", "NewLayer1UpstreamMaxBitRate"); err == nil {
		v, _ := strconv.ParseFloat(up, 64)
		c.LinkUpMbps = v / 1e6
	}
	return c, nil
}

func (g *Gateway) call(ctx context.Context, action, field string) (string, error) {
	envelope := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` +
		action + ` xmlns:u="` + wanCommon + `"/></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.controlURL, strings.NewReader(envelope))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+wanCommon+"#"+action+`"`)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("router: %s: %s", action, resp.Status)
	}
	return soapField(body, field)
}

// soapField returns the text of the first element named field.
func soapField(body []byte, field string) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("router: no %s in reply", field)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == field {
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return "", err
			}
			return strings.TrimSpace(v), nil
		}
	}
}

// Delta returns the bytes moved between two readings. IGD v1 counters are
// 32-bit and wrap at 4 GiB; a reading that went backwards wrapped (or the
// router restarted, which then counts as zero).
func Delta(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	if prev <= 1<<32 {
		return cur + (1<<32 - prev)
	}
	return 0
}
