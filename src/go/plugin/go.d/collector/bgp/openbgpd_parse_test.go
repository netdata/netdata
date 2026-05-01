// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataOpenBGPDNeighbors            []byte
	dataOpenBGPDNeighborsMultifamily []byte
	dataOpenBGPDRIB                  []byte
)

func TestParseOpenBGPDNeighbors(t *testing.T) {
	families, neighbors, err := parseOpenBGPDNeighbors(dataOpenBGPDNeighbors)
	require.NoError(t, err)
	require.Len(t, families, 2)
	require.Len(t, neighbors, 2)

	family4 := families[0]
	assert.Equal(t, "default_ipv4_unicast", family4.ID)
	assert.Equal(t, backendOpenBGPD, family4.Backend)
	assert.Equal(t, "default", family4.VRF)
	assert.Equal(t, "ipv4", family4.AFI)
	assert.Equal(t, "unicast", family4.SAFI)
	assert.Equal(t, int64(1), family4.PeersEstablished)
	assert.Equal(t, int64(1), family4.ConfiguredPeers)
	assert.Equal(t, int64(22161), family4.MessagesReceived)
	assert.Equal(t, int64(22613), family4.MessagesSent)
	assert.Equal(t, int64(1), family4.PrefixesReceived)

	peer4 := family4.Peers[0]
	assert.Equal(t, "200.100.25.7", peer4.Address)
	assert.Equal(t, "206.126.225.254", peer4.LocalAddress)
	assert.Equal(t, int64(123), peer4.RemoteAS)
	assert.Equal(t, int64(22161), peer4.MessagesReceived)
	assert.Equal(t, int64(22613), peer4.MessagesSent)
	assert.Equal(t, int64(864), peer4.PrefixesSent)
	assert.True(t, peer4.HasPrefixesSent)
	assert.Equal(t, int64(7*24*3600+16*3600), peer4.UptimeSecs)
	assert.Equal(t, "clients", peer4.PeerGroup)

	family6 := families[1]
	assert.Equal(t, "default_ipv6_unicast", family6.ID)
	assert.Equal(t, int64(22160), family6.MessagesReceived)
	assert.Equal(t, int64(22289), family6.MessagesSent)
	assert.Equal(t, int64(0), family6.PrefixesReceived)

	neighbor4 := neighbors[0]
	assert.Equal(t, backendOpenBGPD, neighbor4.Backend)
	assert.Equal(t, "default", neighbor4.VRF)
	assert.Equal(t, "200.100.25.7", neighbor4.Address)
	assert.Equal(t, "206.126.225.254", neighbor4.LocalAddress)
	assert.Equal(t, int64(123), neighbor4.RemoteAS)
	assert.True(t, neighbor4.HasChurn)
	assert.True(t, neighbor4.HasMessageTypes)
	assert.Equal(t, int64(1), neighbor4.UpdatesReceived)
	assert.Equal(t, int64(897), neighbor4.UpdatesSent)
	assert.Equal(t, int64(0), neighbor4.WithdrawsReceived)
	assert.Equal(t, int64(7), neighbor4.WithdrawsSent)
	assert.Equal(t, int64(22158), neighbor4.KeepalivesReceived)
	assert.Equal(t, int64(22152), neighbor4.KeepalivesSent)
	assert.Equal(t, int64(0), neighbor4.RouteRefreshReceived)
	assert.Equal(t, int64(0), neighbor4.RouteRefreshSent)
}

func TestParseOpenBGPDNeighborsPrefersNegotiatedCapabilitiesOverRemoteOffer(t *testing.T) {
	families, _, err := parseOpenBGPDNeighbors(dataOpenBGPDNeighbors)
	require.NoError(t, err)

	ids := make([]string, 0, len(families))
	for _, family := range families {
		ids = append(ids, family.ID)
	}

	assert.Contains(t, ids, "default_ipv4_unicast")
	assert.Contains(t, ids, "default_ipv6_unicast")
	assert.NotContains(t, ids, "default_ipv4_vpn")
	assert.NotContains(t, ids, "default_ipv6_vpn")
}

func TestParseOpenBGPDNeighborsMultiFamilyDoesNotDuplicateSessionCounters(t *testing.T) {
	families, neighbors, err := parseOpenBGPDNeighbors(dataOpenBGPDNeighborsMultifamily)
	require.NoError(t, err)
	require.Len(t, families, 2)
	require.Len(t, neighbors, 1)

	for _, family := range families {
		assert.Equal(t, int64(1), family.PeersEstablished)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Equal(t, int64(0), family.MessagesReceived)
		assert.Equal(t, int64(0), family.MessagesSent)
		assert.Equal(t, int64(0), family.PrefixesReceived)

		peer := family.Peers[0]
		assert.Equal(t, int64(0), peer.MessagesReceived)
		assert.Equal(t, int64(0), peer.MessagesSent)
		assert.Equal(t, int64(0), peer.PrefixesReceived)
		assert.False(t, peer.HasPrefixesSent)
	}

	neighbor := neighbors[0]
	assert.Equal(t, int64(20), neighbor.UpdatesReceived)
	assert.Equal(t, int64(22), neighbor.UpdatesSent)
	assert.Equal(t, int64(1), neighbor.WithdrawsReceived)
	assert.Equal(t, int64(2), neighbor.WithdrawsSent)
	assert.Equal(t, int64(480), neighbor.KeepalivesReceived)
	assert.Equal(t, int64(500), neighbor.KeepalivesSent)
	assert.Equal(t, int64(3), neighbor.RouteRefreshReceived)
	assert.Equal(t, int64(4), neighbor.RouteRefreshSent)
}

func TestParseOpenBGPDRIBSummaries(t *testing.T) {
	summaries, err := parseOpenBGPDRIBSummaries(dataOpenBGPDRIB)
	require.NoError(t, err)

	assert.Equal(t, openbgpdRIBSummary{RIBRoutes: 1, NotFound: 1}, summaries["default_ipv4_unicast"])
	assert.Equal(t, openbgpdRIBSummary{RIBRoutes: 1, Valid: 1}, summaries["default_ipv6_unicast"])
}

func TestParseOpenBGPDUptime(t *testing.T) {
	t.Run("clock", func(t *testing.T) {
		secs, err := parseOpenBGPDUptime("10:11:12")
		require.NoError(t, err)
		assert.Equal(t, int64(10*time.Hour/time.Second+11*time.Minute/time.Second+12), secs)
	})

	t.Run("weeks_days_hours", func(t *testing.T) {
		secs, err := parseOpenBGPDUptime("01w2d15h")
		require.NoError(t, err)
		assert.Equal(t, int64(9*24*3600+15*3600), secs)
	})
}

func TestParseOpenBGPDMultiprotocolSingleTokenEVPN(t *testing.T) {
	afi, safi, ok := parseOpenBGPDMultiprotocol("EVPN")
	require.True(t, ok)
	assert.Equal(t, "l2vpn", afi)
	assert.Equal(t, "evpn", safi)
}
