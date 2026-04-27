// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressStrings_DeduplicatesMappedAndUnmappedIPs(t *testing.T) {
	addresses := []netip.Addr{
		netip.MustParseAddr("::ffff:10.0.0.1"),
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("2001:db8::1"),
	}

	require.Equal(t, []string{"10.0.0.1", "2001:db8::1"}, addressStrings(addresses))
}

func TestSortedEndpointIPs_DeduplicatesMappedAndUnmappedIPs(t *testing.T) {
	values := map[string]netip.Addr{
		"mapped":   netip.MustParseAddr("::ffff:10.0.0.1"),
		"unmapped": netip.MustParseAddr("10.0.0.1"),
		"ipv6":     netip.MustParseAddr("2001:db8::1"),
	}

	require.Equal(t, []string{"10.0.0.1", "2001:db8::1"}, sortedEndpointIPs(values))
}
