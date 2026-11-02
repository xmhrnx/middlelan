package wifi

import (
	"context"
	"errors"
	"time"
)

// Security is a simplified Wi-Fi security mode: "Open", "WEP", "WPA",
// "WPA2", "WPA3", "WPA2/WPA3", "Enterprise" or "Unknown".
type Security string

// The security modes a network can report.
const (
	SecOpen       Security = "Open"
	SecWEP        Security = "WEP"
	SecWPA        Security = "WPA"
	SecWPA2       Security = "WPA2"
	SecWPA3       Security = "WPA3"
	SecWPA2WPA3   Security = "WPA2/WPA3"
	SecEnterprise Security = "Enterprise"
	SecUnknown    Security = "Unknown"
)

// Network is one access point seen in a scan.
type Network struct {
	SSID         string // empty for hidden networks or when the OS withholds it
	BSSID        string // lower-case colon-separated MAC; empty when unknown
	SignalDBm    int    // received signal strength, estimated from Quality when the OS reports a percentage
	Quality      int    // signal quality 0-100
	Channel      int
	FrequencyMHz int
	Band         string // "2.4 GHz", "5 GHz", "6 GHz" or empty
	Security     Security
	Connected    bool // the device is associated with this access point
}

// Link is the Wi-Fi connection the device currently uses.
type Link struct {
	Interface    string
	SSID         string
	BSSID        string
	SignalDBm    int
	Quality      int
	Channel      int
	FrequencyMHz int
	Band         string
	TxRateMbps   float64
	RxRateMbps   float64
	Security     Security
	Connected    bool
}

var (
	// ErrNotWiFi means the device has no Wi-Fi adapter or isn't connected
	// to a Wi-Fi network (for example it is on Ethernet).
	ErrNotWiFi = errors.New("not connected to Wi-Fi")
	// ErrUnsupported means Wi-Fi information isn't available on this
	// platform, or the tools it needs are missing.
	ErrUnsupported = errors.New("Wi-Fi information isn't supported on this platform")
	// ErrNeedsLocation means the OS hides network names (and on some
	// systems scan results) until the app has location permission and
	// location services are on. Current returns the Link it could fill
	// together with this error.
	ErrNeedsLocation = errors.New("location permission is needed to see Wi-Fi network names")
)

const (
	currentTimeout = 8 * time.Second // system_profiler alone can take several seconds
	scanTimeout    = 10 * time.Second
)

// Current returns the Wi-Fi link the device is connected to. It returns
// ErrNotWiFi when there is no Wi-Fi connection, ErrUnsupported when the
// platform can't tell, and a partly filled Link with ErrNeedsLocation when
// the OS withholds the network name.
func Current(ctx context.Context) (Link, error) {
	ctx, cancel := context.WithTimeout(ctx, currentTimeout)
	defer cancel()
	l, err := current(ctx)
	if err == nil && l.SSID == "" && l.BSSID == "" && !l.Connected {
		return l, ErrNotWiFi
	}
	return l, err
}

// Scan returns the Wi-Fi networks around the device, strongest first. It
// may return the OS's cached scan results and never takes much more than
// ten seconds.
func Scan(ctx context.Context) ([]Network, error) {
	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()
	nets, err := scan(ctx)
	if err != nil {
		return nil, err
	}
	finishNetworks(nets)
	return nets, nil
}
