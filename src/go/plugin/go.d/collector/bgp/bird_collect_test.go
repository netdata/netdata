// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_CollectBIRDProtocols(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	collr := newBIRDTestCollector(t, &mockClient{protocolsAll: dataBIRDProtocolsAllMultichannel})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("master4_ipv4_unicast", 1, 0, 0, 1, 1, 7, 33, 13, 10) {
		expected[k] = v
	}
	for k, v := range familyMetricSet("master6_ipv6_unicast", 1, 0, 0, 1, 1, 45, 73, 5, 2) {
		expected[k] = v
	}
	for k, v := range familyMetricSet("backup6_ipv6_unicast", 0, 0, 1, 1, 1, 3, 0, 1, 1) {
		expected[k] = v
	}

	edge4PeerID := makePeerIDWithScope("master4_ipv4_unicast", "192.0.2.2", "bgp_edge")
	edge6PeerID := makePeerIDWithScope("master6_ipv6_unicast", "192.0.2.2", "bgp_edge")
	backupPeerID := makePeerIDWithScope("backup6_ipv6_unicast", "2001:db8::2", "bgp_backup")
	for k, v := range mergeMetricSets(
		peerMetricSet(edge4PeerID, 7, 33, 13, 7200, peerStateUp),
		peerPolicyMetricSet(edge4PeerID, 12, 1),
		peerAdvertisedMetricSet(edge4PeerID, 34),
		peerMetricSet(edge6PeerID, 45, 73, 5, 7200, peerStateUp),
		peerPolicyMetricSet(edge6PeerID, 3, 2),
		peerAdvertisedMetricSet(edge6PeerID, 5),
		peerMetricSet(backupPeerID, 3, 0, 1, 0, peerStateDown),
		peerPolicyMetricSet(backupPeerID, 1, 0),
		peerAdvertisedMetricSet(backupPeerID, 0),
	) {
		expected[k] = v
	}

	edge4NeighborID := makeNeighborIDWithScope("master4", "192.0.2.2", "bgp_edge")
	edge6NeighborID := makeNeighborIDWithScope("master6", "192.0.2.2", "bgp_edge")
	backupNeighborID := makeNeighborIDWithScope("backup6", "2001:db8::2", "bgp_backup")
	for k, v := range mergeMetricSets(
		neighborChurnMetricSetWithWithdraws(edge4NeighborID, 1, 15, 6, 18),
		neighborChurnMetricSetWithWithdraws(edge6NeighborID, 20, 34, 25, 39),
		neighborChurnMetricSetWithWithdraws(backupNeighborID, 2, 0, 1, 0),
	) {
		expected[k] = v
	}

	assert.Equal(t, expected, mx)

	require.NotNil(t, collr.Charts().Get("family_master4_ipv4_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get("family_master6_ipv6_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get("family_backup6_ipv6_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get("peer_"+edge4PeerID+"_messages"))
	require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(edge4PeerID)))
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(edge4PeerID)))
	require.NotNil(t, collr.Charts().Get("neighbor_"+edge4NeighborID+"_churn"))
	assert.Nil(t, collr.Charts().Get("neighbor_"+edge4NeighborID+"_transitions"))
	assert.Nil(t, collr.Charts().Get("neighbor_"+edge4NeighborID+"_message_types"))

	peerChart := collr.Charts().Get("peer_" + edge4PeerID + "_messages")
	require.NotNil(t, peerChart)
	assert.Equal(t, "master4", chartLabelValue(peerChart, "table"))
	assert.Equal(t, "bgp_edge", chartLabelValue(peerChart, "protocol"))

	neighborChart := collr.Charts().Get("neighbor_" + edge4NeighborID + "_churn")
	require.NotNil(t, neighborChart)
	assert.Equal(t, "master4", chartLabelValue(neighborChart, "table"))
	assert.Equal(t, "bgp_edge", chartLabelValue(neighborChart, "protocol"))
}

func TestBuildBIRDFamiliesAdvancedChannels(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllAdvanced)
	require.NoError(t, err)

	families := buildBIRDFamilies(protocols)
	require.Len(t, families, 10)

	want := map[string]struct {
		afi      string
		safi     string
		table    string
		imported int64
	}{
		"flow4tab_ipv4_flowspec":       {afi: "ipv4", safi: "flowspec", table: "flow4tab", imported: 36},
		"flow6tab_ipv6_flowspec":       {afi: "ipv6", safi: "flowspec", table: "flow6tab", imported: 39},
		"label4_ipv4_label":            {afi: "ipv4", safi: "label", table: "label4", imported: 18},
		"label6_ipv6_label":            {afi: "ipv6", safi: "label", table: "label6", imported: 21},
		"mcast4_ipv4_multicast":        {afi: "ipv4", safi: "multicast", table: "mcast4", imported: 12},
		"mcast6_ipv6_multicast":        {afi: "ipv6", safi: "multicast", table: "mcast6", imported: 15},
		"mvpn4tab_ipv4_multicast__vpn": {afi: "ipv4", safi: "multicast_vpn", table: "mvpn4tab", imported: 30},
		"mvpn6tab_ipv6_multicast__vpn": {afi: "ipv6", safi: "multicast_vpn", table: "mvpn6tab", imported: 33},
		"vpn4tab_ipv4_vpn":             {afi: "ipv4", safi: "vpn", table: "vpn4tab", imported: 24},
		"vpn6tab_ipv6_vpn":             {afi: "ipv6", safi: "vpn", table: "vpn6tab", imported: 27},
	}

	for _, family := range families {
		expected, ok := want[family.ID]
		require.Truef(t, ok, "unexpected family id %q", family.ID)
		assert.Equal(t, expected.afi, family.AFI)
		assert.Equal(t, expected.safi, family.SAFI)
		assert.Equal(t, expected.table, family.Table)
		assert.Equal(t, int64(1), family.PeersEstablished)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Equal(t, expected.imported, family.MessagesReceived)
		require.Len(t, family.Peers, 1)
		assert.Equal(t, "198.51.100.2", family.Peers[0].Address)
	}
}

func TestCollector_CheckBIRDProtocols(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	collr := newBIRDTestCollector(t, &mockClient{protocolsAll: dataBIRDProtocolsAllMultichannel})
	require.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckFailsWithoutBIRDProtocols(t *testing.T) {
	collr := newBIRDTestCollector(t, &mockClient{protocolsAll: []byte("1002-Name Proto Table State Since Info\n0000 OK\n")})
	require.Error(t, collr.Check(context.Background()))
}

func TestCollector_CollectBIRDQueryError(t *testing.T) {
	collr := newBIRDTestCollector(t, &mockClient{err: assert.AnError})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusQueryError, 1, 0, 0, 0, 0, 0)
	assert.Empty(t, mx)
}

func newBIRDTestCollector(t *testing.T, mock *mockClient) *Collector {
	t.Helper()

	collr := New()
	collr.Backend = backendBIRD
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

func peerAdvertisedMetricSet(id string, advertised int64) map[string]int64 {
	return map[string]int64{
		"peer_" + id + "_prefixes_advertised": advertised,
	}
}

func neighborChurnMetricSetWithWithdraws(id string, updatesReceived, updatesSent, withdrawsReceived, withdrawsSent int64) map[string]int64 {
	prefix := "neighbor_" + id + "_churn_"
	return map[string]int64{
		prefix + "updates_received":       updatesReceived,
		prefix + "updates_sent":           updatesSent,
		prefix + "withdraws_received":     withdrawsReceived,
		prefix + "withdraws_sent":         withdrawsSent,
		prefix + "notifications_received": 0,
		prefix + "notifications_sent":     0,
		prefix + "route_refresh_received": 0,
		prefix + "route_refresh_sent":     0,
	}
}
