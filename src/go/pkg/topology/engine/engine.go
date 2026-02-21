// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"context"
	"errors"
)

var (
	ErrNotImplemented = errors.New("topology engine not implemented")
	ErrInvalidRequest = errors.New("invalid topology discovery request")
)

// Engine executes topology discovery from different seed inputs.
type Engine interface {
	DiscoverByCIDRs(ctx context.Context, req CIDRRequest) (Result, error)
	DiscoverByDevices(ctx context.Context, req DeviceRequest) (Result, error)
}
