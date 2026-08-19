// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortedAddrValuesUsesCanonicalNumericOrder(t *testing.T) {
	got := sortedAddrValues(map[string]netip.Addr{
		"mapped":  netip.MustParseAddr("::ffff:10.0.0.2"),
		"native":  netip.MustParseAddr("10.0.0.2"),
		"lexical": netip.MustParseAddr("10.0.0.10"),
		"invalid": {},
	})

	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("10.0.0.2"),
		netip.MustParseAddr("10.0.0.10"),
	}, got)
}
