//go:build !windows && !linux && !(darwin && !ios)

package wifi

import "context"

func current(context.Context) (Link, error) { return Link{}, ErrUnsupported }

func scan(context.Context) ([]Network, error) { return nil, ErrUnsupported }
