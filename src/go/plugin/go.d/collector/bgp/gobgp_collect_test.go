// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGoBGPMetrics(t *testing.T) {
	now := time.Now()
	families, neighbors := buildGoBGPMetrics(testGoBGPGlobal(), testGoBGPPeers(now))

	require.Len(t, families, 3)
	require.Len(t, neighbors, 3)

	byID := make(map[string]familyStats, len(families))
	for _, family := range families {
		byID[family.ID] = family
	}
	neighborsByAddress := make(map[string]neighborStats, len(neighbors))
	for _, neighbor := range neighbors {
		neighborsByAddress[neighbor.Address] = neighbor
	}

	def4 := byID["default_ipv4_unicast"]
	assert.Equal(t, "default", def4.VRF)
	assert.Equal(t, int64(1), def4.ConfiguredPeers)
	assert.Equal(t, int64(1), def4.PeersEstablished)
	assert.Equal(t, int64(100), def4.PrefixesReceived)
	assert.Equal(t, int64(15), def4.MessagesReceived)
	assert.Equal(t, int64(20), def4.MessagesSent)

	blue4 := byID["blue_ipv4_unicast"]
	assert.Equal(t, "blue", blue4.VRF)
	assert.Equal(t, int64(1), blue4.ConfiguredPeers)
	assert.Equal(t, int64(50), blue4.PrefixesReceived)
	assert.Equal(t, int64(0), blue4.MessagesReceived)
	assert.Equal(t, int64(0), blue4.MessagesSent)

	blue6 := byID["blue_ipv6_unicast"]
	assert.Equal(t, int64(2), blue6.ConfiguredPeers)
	assert.Equal(t, int64(1), blue6.PeersEstablished)
	assert.Equal(t, int64(1), blue6.PeersAdminDown)
	assert.Equal(t, int64(60), blue6.PrefixesReceived)

	assert.Equal(t, "Configured 198.51.100.2", neighborsByAddress["198.51.100.2"].Desc)
	assert.Equal(t, "blue", neighborsByAddress["198.51.100.2"].VRF)
	assert.Equal(t, int64(7), neighborsByAddress["198.51.100.2"].ConnectionsDropped)
}

func TestCollector_CollectGoBGP(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpPeers:  testGoBGPPeers(now),
		gobgpTables: map[string]uint64{
			"default_ipv4_unicast": 123,
			"blue_ipv4_unicast":    456,
			"blue_ipv6_unicast":    789,
		},
		gobgpValidation: map[string]gobgpValidationSummary{
			"default_ipv4_unicast": {HasCorrectness: true, Valid: 10, Invalid: 1, NotFound: 2},
			"blue_ipv6_unicast":    {HasCorrectness: true, Valid: 5, Invalid: 0, NotFound: 0},
		},
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = "127.0.0.1:50051"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 1, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("default_ipv4_unicast", 1, 0, 0, 1, 1, 15, 20, 100, 123) {
		expected[k] = v
	}
	expected["family_default_ipv4_unicast_correctness_valid"] = 10
	expected["family_default_ipv4_unicast_correctness_invalid"] = 1
	expected["family_default_ipv4_unicast_correctness_not_found"] = 2

	for k, v := range familyMetricSet("blue_ipv4_unicast", 1, 0, 0, 1, 1, 0, 0, 50, 456) {
		expected[k] = v
	}

	for k, v := range familyMetricSet("blue_ipv6_unicast", 1, 1, 0, 2, 2, 0, 0, 60, 789) {
		expected[k] = v
	}

	peer1 := testGoBGPPeers(now)[0].Peer
	peer1ID := makePeerIDWithScope("default_ipv4_unicast", peer1.GetState().GetNeighborAddress(), gobgpPeerScope(peer1))
	expected["peer_"+peer1ID+"_messages_received"] = 15
	expected["peer_"+peer1ID+"_messages_sent"] = 20
	expected["peer_"+peer1ID+"_prefixes_received"] = 100
	expected["peer_"+peer1ID+"_prefixes_accepted"] = 90
	expected["peer_"+peer1ID+"_prefixes_filtered"] = 10
	expected["peer_"+peer1ID+"_prefixes_advertised"] = 80
	expected["peer_"+peer1ID+"_state"] = peerStateUp

	peer2 := testGoBGPPeers(now)[1].Peer
	peer2Scope := gobgpPeerScope(peer2)
	peer2v4ID := makePeerIDWithScope("blue_ipv4_unicast", peer2.GetState().GetNeighborAddress(), peer2Scope)
	expected["peer_"+peer2v4ID+"_messages_received"] = 0
	expected["peer_"+peer2v4ID+"_messages_sent"] = 0
	expected["peer_"+peer2v4ID+"_prefixes_received"] = 50
	expected["peer_"+peer2v4ID+"_prefixes_accepted"] = 45
	expected["peer_"+peer2v4ID+"_prefixes_filtered"] = 5
	expected["peer_"+peer2v4ID+"_prefixes_advertised"] = 40
	expected["peer_"+peer2v4ID+"_state"] = peerStateUp

	peer2v6ID := makePeerIDWithScope("blue_ipv6_unicast", peer2.GetState().GetNeighborAddress(), peer2Scope)
	expected["peer_"+peer2v6ID+"_messages_received"] = 0
	expected["peer_"+peer2v6ID+"_messages_sent"] = 0
	expected["peer_"+peer2v6ID+"_prefixes_received"] = 60
	expected["peer_"+peer2v6ID+"_prefixes_accepted"] = 55
	expected["peer_"+peer2v6ID+"_prefixes_filtered"] = 5
	expected["peer_"+peer2v6ID+"_prefixes_advertised"] = 50
	expected["peer_"+peer2v6ID+"_state"] = peerStateUp

	peer3 := testGoBGPPeers(now)[2].Peer
	peer3ID := makePeerIDWithScope("blue_ipv6_unicast", peer3.GetState().GetNeighborAddress(), gobgpPeerScope(peer3))
	expected["peer_"+peer3ID+"_messages_received"] = 0
	expected["peer_"+peer3ID+"_messages_sent"] = 0
	expected["peer_"+peer3ID+"_prefixes_received"] = 0
	expected["peer_"+peer3ID+"_prefixes_accepted"] = 0
	expected["peer_"+peer3ID+"_prefixes_filtered"] = 0
	expected["peer_"+peer3ID+"_prefixes_advertised"] = 0
	expected["peer_"+peer3ID+"_state"] = peerStateAdminDown

	neighbor1ID := makeNeighborIDWithScope("default", peer1.GetState().GetNeighborAddress(), gobgpPeerScope(peer1))
	expected["neighbor_"+neighbor1ID+"_connections_established"] = 0
	expected["neighbor_"+neighbor1ID+"_connections_dropped"] = 2
	expected["neighbor_"+neighbor1ID+"_updates_received"] = 4
	expected["neighbor_"+neighbor1ID+"_updates_sent"] = 5
	expected["neighbor_"+neighbor1ID+"_notifications_received"] = 1
	expected["neighbor_"+neighbor1ID+"_notifications_sent"] = 0
	expected["neighbor_"+neighbor1ID+"_keepalives_received"] = 9
	expected["neighbor_"+neighbor1ID+"_keepalives_sent"] = 14
	expected["neighbor_"+neighbor1ID+"_route_refresh_received"] = 1
	expected["neighbor_"+neighbor1ID+"_route_refresh_sent"] = 1
	expected["neighbor_"+neighbor1ID+"_churn_updates_received"] = 4
	expected["neighbor_"+neighbor1ID+"_churn_updates_sent"] = 5
	expected["neighbor_"+neighbor1ID+"_churn_withdraws_received"] = 3
	expected["neighbor_"+neighbor1ID+"_churn_withdraws_sent"] = 1
	expected["neighbor_"+neighbor1ID+"_churn_notifications_received"] = 1
	expected["neighbor_"+neighbor1ID+"_churn_notifications_sent"] = 0
	expected["neighbor_"+neighbor1ID+"_churn_route_refresh_received"] = 1
	expected["neighbor_"+neighbor1ID+"_churn_route_refresh_sent"] = 1

	neighbor2ID := makeNeighborIDWithScope("blue", peer2.GetState().GetNeighborAddress(), peer2Scope)
	expected["neighbor_"+neighbor2ID+"_connections_established"] = 0
	expected["neighbor_"+neighbor2ID+"_connections_dropped"] = 4
	expected["neighbor_"+neighbor2ID+"_updates_received"] = 20
	expected["neighbor_"+neighbor2ID+"_updates_sent"] = 30
	expected["neighbor_"+neighbor2ID+"_notifications_received"] = 0
	expected["neighbor_"+neighbor2ID+"_notifications_sent"] = 1
	expected["neighbor_"+neighbor2ID+"_keepalives_received"] = 80
	expected["neighbor_"+neighbor2ID+"_keepalives_sent"] = 110
	expected["neighbor_"+neighbor2ID+"_route_refresh_received"] = 4
	expected["neighbor_"+neighbor2ID+"_route_refresh_sent"] = 5
	expected["neighbor_"+neighbor2ID+"_churn_updates_received"] = 20
	expected["neighbor_"+neighbor2ID+"_churn_updates_sent"] = 30
	expected["neighbor_"+neighbor2ID+"_churn_withdraws_received"] = 5
	expected["neighbor_"+neighbor2ID+"_churn_withdraws_sent"] = 5
	expected["neighbor_"+neighbor2ID+"_churn_notifications_received"] = 0
	expected["neighbor_"+neighbor2ID+"_churn_notifications_sent"] = 1
	expected["neighbor_"+neighbor2ID+"_churn_route_refresh_received"] = 4
	expected["neighbor_"+neighbor2ID+"_churn_route_refresh_sent"] = 5

	neighbor3ID := makeNeighborIDWithScope("blue", peer3.GetState().GetNeighborAddress(), gobgpPeerScope(peer3))
	expected["neighbor_"+neighbor3ID+"_connections_established"] = 0
	expected["neighbor_"+neighbor3ID+"_connections_dropped"] = 7
	expected["neighbor_"+neighbor3ID+"_updates_received"] = 0
	expected["neighbor_"+neighbor3ID+"_updates_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_notifications_received"] = 0
	expected["neighbor_"+neighbor3ID+"_notifications_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_keepalives_received"] = 0
	expected["neighbor_"+neighbor3ID+"_keepalives_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_route_refresh_received"] = 0
	expected["neighbor_"+neighbor3ID+"_route_refresh_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_updates_received"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_updates_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_withdraws_received"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_withdraws_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_notifications_received"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_notifications_sent"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_route_refresh_received"] = 0
	expected["neighbor_"+neighbor3ID+"_churn_route_refresh_sent"] = 0

	assertMetricRange(t, mx, "peer_"+peer1ID+"_uptime_seconds", 595, 605)
	assertMetricRange(t, mx, "peer_"+peer2v4ID+"_uptime_seconds", 1195, 1205)
	assertMetricRange(t, mx, "peer_"+peer2v6ID+"_uptime_seconds", 1195, 1205)
	assert.Equal(t, int64(0), mx["peer_"+peer3ID+"_uptime_seconds"])
	delete(mx, "peer_"+peer1ID+"_uptime_seconds")
	delete(mx, "peer_"+peer2v4ID+"_uptime_seconds")
	delete(mx, "peer_"+peer2v6ID+"_uptime_seconds")
	delete(mx, "peer_"+peer3ID+"_uptime_seconds")

	assert.Equal(t, expected, mx)

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, "127.0.0.1:50051", chartLabelValue(collectorChart, "target"))

	neighborChart := collr.Charts().Get("neighbor_" + neighbor2ID + "_message_types")
	require.NotNil(t, neighborChart)
	assert.Equal(t, "blue", chartLabelValue(neighborChart, "vrf"))
	assert.Equal(t, "2001:db8::254", chartLabelValue(neighborChart, "local_address"))
}

func TestCollector_GoBGPValidationCache(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpPeers:  testGoBGPPeers(now),
		gobgpTables: map[string]uint64{
			"default_ipv4_unicast": 1,
			"blue_ipv4_unicast":    2,
			"blue_ipv6_unicast":    3,
		},
		gobgpValidation: map[string]gobgpValidationSummary{
			"default_ipv4_unicast": {HasCorrectness: true, Valid: 1},
			"blue_ipv4_unicast":    {HasCorrectness: true, NotFound: 1},
			"blue_ipv6_unicast":    {HasCorrectness: true, Invalid: 1},
		},
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = "127.0.0.1:50051"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Hour)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	require.NotNil(t, collr.Collect(context.Background()))
	mock.gobgpValidationErrs = map[string]error{
		"default_ipv4_unicast": assert.AnError,
		"blue_ipv4_unicast":    assert.AnError,
		"blue_ipv6_unicast":    assert.AnError,
	}
	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, 6, mock.gobgpTableCalls)
	assert.Equal(t, 1, mock.gobgpValidationCalls)
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_valid"])
	assert.NotContains(t, mx, "family_blue_ipv4_unicast_correctness_not_found")
	assert.NotContains(t, mx, "family_blue_ipv6_unicast_correctness_invalid")
}

func TestCollector_GoBGPSummariesHonorFamilySelection(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpPeers:  testGoBGPPeers(now),
		gobgpTables: map[string]uint64{
			"default_ipv4_unicast": 123,
			"blue_ipv4_unicast":    456,
			"blue_ipv6_unicast":    789,
		},
		gobgpValidation: map[string]gobgpValidationSummary{
			"default_ipv4_unicast": {HasCorrectness: true, Valid: 10},
			"blue_ipv4_unicast":    {HasCorrectness: true, NotFound: 1},
			"blue_ipv6_unicast":    {HasCorrectness: true, Invalid: 1},
		},
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = "127.0.0.1:50051"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.MaxFamilies = 1
	collr.SelectFamilies.Includes = []string{"=default/ipv4/unicast"}
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 1, 0, 0)

	assert.Equal(t, []string{"default_ipv4_unicast"}, mock.gobgpTableRefs)
	assert.Equal(t, []string{"default_ipv4_unicast"}, mock.gobgpValidationRefs)
	assert.Equal(t, 1, mock.gobgpTableCalls)
	assert.Equal(t, 1, mock.gobgpValidationCalls)
	assert.Contains(t, mx, "family_default_ipv4_unicast_rib_routes")
	assert.Contains(t, mx, "family_default_ipv4_unicast_correctness_valid")
	assert.NotContains(t, mx, "family_blue_ipv4_unicast_rib_routes")
	assert.NotContains(t, mx, "family_blue_ipv6_unicast_rib_routes")
	assert.NotContains(t, mx, "family_blue_ipv4_unicast_correctness_not_found")
	assert.NotContains(t, mx, "family_blue_ipv6_unicast_correctness_invalid")
}

func TestCollector_GoBGPValidationFailureCountsAttempt(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpPeers:  testGoBGPPeers(now),
		gobgpTables: map[string]uint64{
			"default_ipv4_unicast": 1,
			"blue_ipv4_unicast":    2,
			"blue_ipv6_unicast":    3,
		},
		gobgpValidationErrs: map[string]error{
			"default_ipv4_unicast": assert.AnError,
		},
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.Address = "127.0.0.1:50051"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 1, 1, 1, 0)

	assert.Equal(t, 1, mock.gobgpValidationCalls)
	assert.Equal(t, []string{"default_ipv4_unicast"}, mock.gobgpValidationRefs)
}

func assertMetricRange(t *testing.T, mx map[string]int64, key string, min, max int64) {
	t.Helper()
	value, ok := mx[key]
	require.Truef(t, ok, "metric %s must exist", key)
	assert.GreaterOrEqualf(t, value, min, "metric %s", key)
	assert.LessOrEqualf(t, value, max, "metric %s", key)
}
