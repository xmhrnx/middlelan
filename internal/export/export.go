// Package export writes MiddleMan's history as CSV or JSON, for
// spreadsheets, reports or another tool.
package export

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/speedlog"
)

// Data is everything an export can contain.
type Data struct {
	Store   *monitor.Store
	Devices *inventory.Inventory
	Speed   *speedlog.Log
	// Describe names a destination IP (host, owner, country), optional.
	Describe func(ip string) (host, owner, country string)
}

// UsageRow is one app's traffic on one day.
type UsageRow struct {
	Day     string `json:"day"`
	App     string `json:"app"`
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

// DestinationRow is a remote address an app used.
type DestinationRow struct {
	App      string    `json:"app"`
	IP       string    `json:"ip"`
	Port     int       `json:"port"`
	Host     string    `json:"host,omitempty"`
	Owner    string    `json:"owner,omitempty"`
	Country  string    `json:"country,omitempty"`
	RxBytes  uint64    `json:"rxBytes"`
	TxBytes  uint64    `json:"txBytes"`
	Count    int       `json:"connections"`
	LastSeen time.Time `json:"lastSeen"`
}

func (d Data) usage() []UsageRow {
	var rows []UsageRow
	for _, day := range d.Store.Days() {
		t, err := time.ParseInLocation("2006-01-02", day, time.Local)
		if err != nil {
			continue
		}
		apps := d.Store.Day(t)
		names := make([]string, 0, len(apps))
		for a := range apps {
			names = append(names, a)
		}
		sort.Strings(names)
		for _, a := range names {
			rows = append(rows, UsageRow{Day: day, App: a, RxBytes: apps[a].Rx, TxBytes: apps[a].Tx})
		}
	}
	return rows
}

func (d Data) destinations() []DestinationRow {
	var rows []DestinationRow
	for _, app := range d.Store.Apps() {
		for _, dst := range d.Store.Destinations(app) {
			r := DestinationRow{App: app, IP: dst.IP, Port: dst.Port, RxBytes: dst.Rx, TxBytes: dst.Tx, Count: dst.Count, LastSeen: dst.LastSeen}
			if d.Describe != nil {
				r.Host, r.Owner, r.Country = d.Describe(dst.IP)
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// JSON writes everything as one JSON document.
func (d Data) JSON(w io.Writer) error {
	doc := struct {
		Exported     time.Time          `json:"exported"`
		Usage        []UsageRow         `json:"usage"`
		Destinations []DestinationRow   `json:"destinations"`
		Devices      []inventory.Device `json:"devices,omitempty"`
		Presence     []inventory.Event  `json:"presence,omitempty"`
		SpeedTests   []speedlog.Result  `json:"speedTests,omitempty"`
	}{Exported: time.Now(), Usage: d.usage(), Destinations: d.destinations()}
	if d.Devices != nil {
		doc.Devices, doc.Presence = d.Devices.List(), d.Devices.Events("")
	}
	if d.Speed != nil {
		doc.SpeedTests = d.Speed.Results(time.Time{})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// UsageCSV writes daily usage per app.
func (d Data) UsageCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"day", "app", "download_bytes", "upload_bytes"}); err != nil {
		return err
	}
	for _, r := range d.usage() {
		if err := cw.Write([]string{r.Day, r.App, u(r.RxBytes), u(r.TxBytes)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// DestinationsCSV writes every app's remembered destinations.
func (d Data) DestinationsCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"app", "ip", "port", "host", "owner", "country", "download_bytes", "upload_bytes", "connections", "last_seen"}); err != nil {
		return err
	}
	for _, r := range d.destinations() {
		rec := []string{r.App, r.IP, strconv.Itoa(r.Port), r.Host, r.Owner, r.Country, u(r.RxBytes), u(r.TxBytes), strconv.Itoa(r.Count), r.LastSeen.Format(time.RFC3339)}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func u(v uint64) string { return strconv.FormatUint(v, 10) }
