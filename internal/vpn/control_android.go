//go:build android

package vpn

import (
	"errors"
	"sync"

	"golang.org/x/sys/unix"
)

// javaBridge is the Java side, com.middlelan.app.VpnBridge. jni_android.go
// provides it; it is the only file needing cgo, so everything here builds
// (and vets) without it.
type javaBridge interface {
	prepare() (bool, error)
	isPrepared() (bool, error)
	start() (bool, error)
	stop() error
	ownerUID(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (int, error)
	appLabel(uid int) (string, error)
}

var bridge javaBridge

var (
	errMu   sync.Mutex
	lastErr error

	ownersOnce sync.Once
	owners     *ownerLookup
)

func init() {
	// When the engine stops on its own (the tunnel failed or was
	// revoked), take the service and its notification down too.
	Default.onStop = func() {
		if bridge != nil {
			bridge.stop()
		}
	}
}

// Supported reports whether this platform has the on-device VPN.
func Supported() bool { return bridge != nil }

// Prepare reports whether the user already allowed MiddleLAN to run a
// VPN. If not, it shows the system consent dialog and returns false; call
// Granted (or Prepare again) once the user has answered.
func Prepare() (granted bool, err error) {
	if bridge == nil {
		return false, ErrUnsupported
	}
	return bridge.prepare()
}

// Granted reports whether the user already gave VPN consent, without
// asking.
func Granted() bool {
	if bridge == nil {
		return false
	}
	ok, err := bridge.isPrepared()
	return err == nil && ok
}

// Start asks the VPN service to start; the service establishes the tunnel
// and starts Default through nativeStart. It needs the user's consent
// (Prepare) first.
func Start() error {
	if bridge == nil {
		return ErrUnsupported
	}
	if !Granted() {
		return errors.New("vpn: MiddleLAN isn't allowed to run a VPN yet")
	}
	ok, err := bridge.start()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("vpn: the VPN service couldn't be started")
	}
	setLastError(nil)
	return nil
}

// Stop stops the VPN service and Default.
func Stop() error {
	var err error
	if bridge != nil {
		err = bridge.stop()
	}
	if serr := Default.Stop(); err == nil {
		err = serr
	}
	return err
}

// LastError returns why the VPN last failed to start, if it did.
func LastError() error {
	errMu.Lock()
	defer errMu.Unlock()
	return lastErr
}

func setLastError(err error) {
	errMu.Lock()
	lastErr = err
	errMu.Unlock()
}

func platformHooks(h Hooks) Hooks {
	if h.Owner == nil && bridge != nil {
		ownersOnce.Do(func() {
			owners = newOwnerLookup(func(proto int, srcIP string, srcPort int, dstIP string, dstPort int) int {
				uid, err := bridge.ownerUID(proto, srcIP, srcPort, dstIP, dstPort)
				if err != nil {
					return -1
				}
				return uid
			}, func(uid int) string {
				s, _ := bridge.appLabel(uid)
				return s
			})
		})
		h.Owner = owners.Owner
	}
	return h
}

// nativeStart runs Default on the tunnel the service established. The
// engine owns fd from here on: it is closed if starting fails.
func nativeStart(fd int, dns string) error {
	Default.SetHooks(platformHooks(*Default.hooks.Load()))
	servers := SplitDNS(dns)
	err := Default.Start(fd, 1500, servers)
	if errors.Is(err, ErrRunning) {
		// A new tunnel replaces the old one.
		Default.Stop()
		err = Default.Start(fd, 1500, servers)
	}
	if err != nil {
		unix.Close(fd)
	}
	setLastError(err)
	return err
}

func nativeStop() { Default.Stop() }

func nativeSetDNS(dns string) {
	if s := SplitDNS(dns); len(s) > 0 {
		Default.SetDNS(s)
	}
}
