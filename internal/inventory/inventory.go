// Package inventory remembers the devices seen on the local network, so
// MiddleLAN can tell which are new and keep the names the user gave them.
package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device is a remembered network device.
type Device struct {
	Key       string    `json:"key"` // MAC address, or IP when the MAC is unknown
	IP        string    `json:"ip"`
	MAC       string    `json:"mac,omitempty"`
	Hostname  string    `json:"host,omitempty"`
	Vendor    string    `json:"vendor,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Label     string    `json:"label,omitempty"` // the user's name for it
	OpenPorts []int     `json:"ports,omitempty"`
	FirstSeen time.Time `json:"first"`
	LastSeen  time.Time `json:"last"`
	Trusted   bool      `json:"trusted,omitempty"`
	IsGateway bool      `json:"gw,omitempty"`
	IsSelf    bool      `json:"self,omitempty"`
	Online    bool      `json:"-"`
	LatencyMs float64   `json:"-"`
}

// Name is the best name for the device.
func (d Device) Name() string {
	switch {
	case d.Label != "":
		return d.Label
	case d.Hostname != "":
		return d.Hostname
	case d.Vendor != "":
		return d.Vendor + " device"
	}
	return d.IP
}

// KeyFor returns the inventory key for a MAC and IP.
func KeyFor(mac, ip string) string {
	if mac != "" {
		return strings.ToLower(mac)
	}
	return ip
}

// Inventory is the set of known devices. Safe from any goroutine.
type Inventory struct {
	mu      sync.Mutex
	path    string
	devices map[string]*Device
	scanned time.Time
}

// Open loads the inventory from dir ("" keeps it in memory).
func Open(dir string) *Inventory {
	inv := &Inventory{devices: map[string]*Device{}}
	if dir == "" {
		return inv
	}
	inv.path = filepath.Join(dir, "devices.json")
	if b, err := os.ReadFile(inv.path); err == nil {
		var list []*Device
		if json.Unmarshal(b, &list) == nil {
			for _, d := range list {
				inv.devices[d.Key] = d
			}
		}
	}
	return inv
}

// Merge records the result of a full scan: devices found are online, the
// rest offline. It returns the devices never seen before. The first scan
// ever reports none as new, since everything would be.
func (inv *Inventory) Merge(found []Device, now time.Time) []Device {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	firstEver := len(inv.devices) == 0
	for _, d := range inv.devices {
		d.Online = false
	}
	var fresh []*Device
	for _, f := range found {
		key := KeyFor(f.MAC, f.IP)
		d, ok := inv.devices[key]
		if !ok {
			// A device first seen without a MAC is the same device now.
			if old, okIP := inv.devices[f.IP]; okIP && f.MAC != "" {
				delete(inv.devices, f.IP)
				d, ok = old, true
			}
		}
		if !ok {
			d = &Device{Key: key, FirstSeen: now}
			if !firstEver && !f.IsSelf {
				fresh = append(fresh, d)
			}
		}
		d.Key, d.IP, d.LastSeen, d.Online = key, f.IP, now, true
		if f.MAC != "" {
			d.MAC = f.MAC
		}
		if f.Hostname != "" {
			d.Hostname = f.Hostname
		}
		if f.Vendor != "" {
			d.Vendor = f.Vendor
		}
		if f.Kind != "" && f.Kind != "unknown" {
			d.Kind = f.Kind
		}
		if len(f.OpenPorts) > 0 {
			d.OpenPorts = f.OpenPorts
		}
		d.IsGateway, d.IsSelf, d.LatencyMs = f.IsGateway, f.IsSelf, f.LatencyMs
		if d.IsSelf {
			d.Trusted = true
		}
		inv.devices[key] = d
	}
	inv.scanned = now
	out := make([]Device, len(fresh))
	for i, d := range fresh {
		out[i] = *d
	}
	return out
}

// LastScan reports when the last scan was merged.
func (inv *Inventory) LastScan() time.Time {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.scanned
}

// List returns the devices: online first, then by IP.
func (inv *Inventory) List() []Device {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	out := make([]Device, 0, len(inv.devices))
	for _, d := range inv.devices {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return ipLess(out[i].IP, out[j].IP)
	})
	return out
}

// Update changes one device (label, trust) and saves.
func (inv *Inventory) Update(key string, fn func(*Device)) error {
	inv.mu.Lock()
	if d, ok := inv.devices[key]; ok {
		fn(d)
	}
	inv.mu.Unlock()
	return inv.Save()
}

// Forget removes a device.
func (inv *Inventory) Forget(key string) error {
	inv.mu.Lock()
	delete(inv.devices, key)
	inv.mu.Unlock()
	return inv.Save()
}

// Save writes the inventory.
func (inv *Inventory) Save() error {
	inv.mu.Lock()
	list := make([]*Device, 0, len(inv.devices))
	for _, d := range inv.devices {
		c := *d
		list = append(list, &c)
	}
	inv.mu.Unlock()
	if inv.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(inv.path), 0o755); err != nil {
		return err
	}
	tmp := inv.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, inv.path)
}

// ipLess orders dotted IPv4 addresses numerically, others as text.
func ipLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	if len(pa) == 4 && len(pb) == 4 {
		for i := range 4 {
			if len(pa[i]) != len(pb[i]) {
				return len(pa[i]) < len(pb[i])
			}
			if pa[i] != pb[i] {
				return pa[i] < pb[i]
			}
		}
		return false
	}
	return a < b
}
