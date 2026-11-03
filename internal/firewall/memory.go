package firewall

import (
	"slices"
	"sync"
)

// memoryBackend only records the rules it is given; something else, such
// as MiddleMan's Android VPN through Firewall.Blocks, enforces them.
type memoryBackend struct {
	mu    sync.Mutex
	caps  Capabilities
	rules []Rule
}

func (b *memoryBackend) Name() string { return b.caps.Backend }

func (b *memoryBackend) Capabilities() Capabilities { return b.caps }

func (b *memoryBackend) Apply(rules []Rule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rules = slices.Clone(rules)
	return nil
}

func (b *memoryBackend) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rules = nil
	return nil
}

func (b *memoryBackend) SystemFirewallOff() bool { return false }
