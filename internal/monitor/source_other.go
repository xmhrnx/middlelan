//go:build !linux && !windows && !darwin && !android

package monitor

import "errors"

type unsupportedSource struct{}

// NewSource returns the connection source for this platform.
func NewSource() Source { return unsupportedSource{} }

func (unsupportedSource) Sample() (Sample, error) {
	return Sample{}, errors.New("connection monitoring isn't supported on this platform")
}

func (unsupportedSource) Limits() string {
	return "Connection monitoring isn't available on this platform."
}
