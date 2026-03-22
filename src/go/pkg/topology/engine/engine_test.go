// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopEngine_DiscoverByCIDRs(t *testing.T) {
	eng := NoopEngine{}
	_, err := eng.DiscoverByCIDRs(context.Background(), CIDRRequest{})
	require.True(t, errors.Is(err, ErrNotImplemented))
}

func TestNoopEngine_DiscoverByDevices(t *testing.T) {
	eng := NoopEngine{}
	_, err := eng.DiscoverByDevices(context.Background(), DeviceRequest{})
	require.True(t, errors.Is(err, ErrNotImplemented))
}
