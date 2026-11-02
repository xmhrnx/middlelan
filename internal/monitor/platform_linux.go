//go:build linux && !android

package monitor

// NewSource returns the connection source for this platform.
func NewSource() Source { return newLinuxSource() }
