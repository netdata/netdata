// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "context"

// NoopEngine is a temporary placeholder that satisfies Engine.
type NoopEngine struct{}

func (NoopEngine) DiscoverByCIDRs(context.Context, CIDRRequest) (Result, error) {
	return Result{}, ErrNotImplemented
}

func (NoopEngine) DiscoverByDevices(context.Context, DeviceRequest) (Result, error) {
	return Result{}, ErrNotImplemented
}
