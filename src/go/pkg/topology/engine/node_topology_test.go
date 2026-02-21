// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInSameNetworkParity(t *testing.T) {
	t.Parallel()

	require.True(t, InSameNetwork(
		netip.MustParseAddr("192.168.0.1"),
		netip.MustParseAddr("192.168.0.2"),
		netip.MustParseAddr("255.255.255.252"),
	))
	require.False(t, InSameNetwork(
		netip.MustParseAddr("192.168.0.1"),
		netip.MustParseAddr("192.168.0.5"),
		netip.MustParseAddr("255.255.255.252"),
	))
	require.True(t, InSameNetwork(
		netip.MustParseAddr("10.10.0.1"),
		netip.MustParseAddr("10.168.0.5"),
		netip.MustParseAddr("255.0.0.0"),
	))
}

func TestMaskHelpers(t *testing.T) {
	t.Parallel()

	require.True(t, IsPointToPointMask(netip.MustParseAddr("255.255.255.252")))
	require.True(t, IsLoopbackMask(netip.MustParseAddr("255.255.255.255")))

	prefix, err := MaskToCIDRPrefix(netip.MustParseAddr("255.255.255.0"))
	require.NoError(t, err)
	require.Equal(t, 24, prefix)

	_, err = MaskToCIDRPrefix(netip.MustParseAddr("255.0.255.0"))
	require.Error(t, err)
}
