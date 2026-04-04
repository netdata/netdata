// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataOpenBGPDNeighborsLiveActive []byte
	dataOpenBGPDRIBLiveActive       []byte
)

func TestParseOpenBGPDNeighborsLiveActiveFixture(t *testing.T) {
	families, neighbors, err := parseOpenBGPDNeighbors(dataOpenBGPDNeighborsLiveActive)
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Len(t, neighbors, 1)

	family := families[0]
	assert.Equal(t, "default_ipv4_unicast", family.ID)
	assert.Equal(t, backendOpenBGPD, family.Backend)
	assert.Equal(t, "default", family.VRF)
	assert.Equal(t, "ipv4", family.AFI)
	assert.Equal(t, "unicast", family.SAFI)
	assert.Equal(t, int64(0), family.PeersEstablished)
	assert.Equal(t, int64(0), family.PeersAdminDown)
	assert.Equal(t, int64(1), family.PeersDown)
	assert.Equal(t, int64(1), family.ConfiguredPeers)
	assert.Equal(t, int64(0), family.MessagesReceived)
	assert.Equal(t, int64(0), family.MessagesSent)
	assert.Equal(t, int64(0), family.PrefixesReceived)

	peer := family.Peers[0]
	assert.Equal(t, makePeerIDWithScope("default_ipv4_unicast", "192.0.2.2", "65020"), peer.ID)
	assert.Equal(t, makeNeighborIDWithScope("default", "192.0.2.2", "65020"), peer.NeighborID)
	assert.Equal(t, "192.0.2.2", peer.Address)
	assert.Equal(t, int64(65020), peer.RemoteAS)
	assert.Equal(t, "test-peer", peer.Desc)
	assert.Equal(t, peerStateDown, peer.State)
	assert.Equal(t, "Active", peer.StateText)
	assert.Equal(t, int64(0), peer.UptimeSecs)
	assert.Equal(t, int64(0), peer.MessagesReceived)
	assert.Equal(t, int64(0), peer.MessagesSent)
	assert.Equal(t, int64(0), peer.PrefixesReceived)
	assert.Equal(t, int64(0), peer.PrefixesSent)
	assert.True(t, peer.HasPrefixesSent)

	neighbor := neighbors[0]
	assert.Equal(t, makeNeighborIDWithScope("default", "192.0.2.2", "65020"), neighbor.ID)
	assert.Equal(t, backendOpenBGPD, neighbor.Backend)
	assert.Equal(t, "default", neighbor.VRF)
	assert.Equal(t, "192.0.2.2", neighbor.Address)
	assert.Equal(t, int64(65020), neighbor.RemoteAS)
	assert.Equal(t, "test-peer", neighbor.Desc)
	assert.True(t, neighbor.HasChurn)
	assert.True(t, neighbor.HasMessageTypes)
	assert.Equal(t, int64(0), neighbor.UpdatesReceived)
	assert.Equal(t, int64(0), neighbor.UpdatesSent)
	assert.Equal(t, int64(0), neighbor.WithdrawsReceived)
	assert.Equal(t, int64(0), neighbor.WithdrawsSent)
	assert.Equal(t, int64(0), neighbor.NotificationsReceived)
	assert.Equal(t, int64(0), neighbor.NotificationsSent)
	assert.Equal(t, int64(0), neighbor.KeepalivesReceived)
	assert.Equal(t, int64(0), neighbor.KeepalivesSent)
	assert.Equal(t, int64(0), neighbor.RouteRefreshReceived)
	assert.Equal(t, int64(0), neighbor.RouteRefreshSent)
}

func TestParseOpenBGPDRIBSummariesLiveFixture(t *testing.T) {
	summaries, err := parseOpenBGPDRIBSummaries(dataOpenBGPDRIBLiveActive)
	require.NoError(t, err)

	assert.Equal(t, openbgpdRIBSummary{RIBRoutes: 1, NotFound: 1}, summaries["default_ipv4_unicast"])
}
