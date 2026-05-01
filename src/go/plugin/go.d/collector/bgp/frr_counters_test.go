// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataFRRNeighborsRich        []byte
	dataFRRIPv4SummaryDualstack []byte
	dataFRRIPv6SummaryDualstack []byte
	dataFRRNeighborsDualstack   []byte
)

func TestParseFRRNeighborsRichCounters(t *testing.T) {
	details, err := parseFRRNeighbors(dataFRRNeighborsRich)
	require.NoError(t, err)

	def := details["default"]["192.168.0.2"]
	assert.Equal(t, "edge-01", def.Desc)
	assert.Equal(t, "transit", def.PeerGroup)
	assert.Equal(t, int64(3), def.ConnectionsEstablished)
	assert.Equal(t, int64(2), def.ConnectionsDropped)
	assert.Equal(t, int64(12), def.UpdatesReceived)
	assert.Equal(t, int64(10), def.UpdatesSent)
	assert.Equal(t, int64(1), def.NotificationsSent)
	assert.Equal(t, int64(78), def.KeepalivesReceived)
	assert.Equal(t, int64(80), def.KeepalivesSent)
	assert.Equal(t, int64(0), def.RouteRefreshReceived)
	assert.Equal(t, int64(1), def.RouteRefreshSent)
	assert.Equal(t, neighborFamilyDetails{
		AcceptedPrefixCounter: 2,
		HasAcceptedPrefixes:   true,
		SentPrefixCounter:     4,
		HasSentPrefixes:       true,
	}, def.Families["default_ipv4_unicast"])

	red := details["red"]["192.168.1.2"]
	assert.Equal(t, "branch-west", red.Desc)
	assert.Equal(t, "branches", red.PeerGroup)
	assert.Equal(t, neighborFamilyDetails{
		AcceptedPrefixCounter: 1,
		HasAcceptedPrefixes:   true,
		SentPrefixCounter:     2,
		HasSentPrefixes:       true,
	}, red.Families["red_ipv4_unicast"])
}

func TestCollector_CollectCheapFRRDiagnostics(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDeep,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsRich,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range mergeMetricSets(
		familyMetricSet("default_ipv4_unicast", 1, 1, 1, 3, 3, 100, 100, 7, 1),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		familyMetricSet("red_ipv4_unicast", 1, 0, 1, 2, 2, 300, 300, 2, 0),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 100, 100, 3, 10, peerStateUp),
		peerAdvertisedMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 4),
		peerPolicyMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 2, 1),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.3"), 0, 0, 2, 0, peerStateDown),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.4"), 0, 0, 2, 0, peerStateAdminDown),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		peerMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.2"), 100, 100, 2, 20, peerStateUp),
		peerAdvertisedMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.2"), 2),
		peerPolicyMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.2"), 1, 1),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		peerMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.3"), 200, 200, 0, 0, peerStateDown),
	) {
		expected[k] = v
	}
	for k, v := range mergeMetricSets(
		neighborMetricSet(makeNeighborID("default", "192.168.0.2"), 3, 2, 12, 10, 0, 1, 78, 80, 0, 1),
		neighborChurnMetricSet(makeNeighborID("default", "192.168.0.2"), 12, 10, 0, 1, 0, 1),
		neighborResetMetricSet(makeNeighborID("default", "192.168.0.2"), 0, 0, 1, 42, 6, 6, 3),
		neighborMetricSet(makeNeighborID("default", "192.168.0.3"), 1, 1, 5, 4, 1, 0, 10, 12, 0, 0),
		neighborChurnMetricSet(makeNeighborID("default", "192.168.0.3"), 5, 4, 1, 0, 0, 0),
		neighborResetMetricSet(makeNeighborID("default", "192.168.0.3"), 0, 1, 0, 15, 4, 4, 0),
		neighborMetricSet(makeNeighborID("default", "192.168.0.4"), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		neighborChurnMetricSet(makeNeighborID("default", "192.168.0.4"), 0, 0, 0, 0, 0, 0),
		neighborResetMetricSet(makeNeighborID("default", "192.168.0.4"), 1, 0, 0, 0, 0, 0, 0),
		neighborMetricSet(makeNeighborID("red", "192.168.1.2"), 4, 1, 15, 14, 0, 0, 83, 84, 1, 0),
		neighborChurnMetricSet(makeNeighborID("red", "192.168.1.2"), 15, 14, 0, 0, 1, 0),
		neighborResetMetricSet(makeNeighborID("red", "192.168.1.2"), 0, 1, 0, 7, 5, 5, 1),
		neighborMetricSet(makeNeighborID("red", "192.168.1.3"), 2, 2, 6, 7, 0, 1, 15, 16, 0, 0),
		neighborChurnMetricSet(makeNeighborID("red", "192.168.1.3"), 6, 7, 0, 1, 0, 0),
		neighborResetMetricSet(makeNeighborID("red", "192.168.1.3"), 1, 0, 0, 0, 0, 0, 0),
	) {
		expected[k] = v
	}

	assert.Equal(t, expected, mx)
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(makePeerID("default_ipv4_unicast", "192.168.0.2"))))
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(makePeerID("red_ipv4_unicast", "192.168.1.2"))))
	require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(makePeerID("default_ipv4_unicast", "192.168.0.2"))))
	require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(makePeerID("red_ipv4_unicast", "192.168.1.2"))))
	require.NotNil(t, collr.Charts().Get("neighbor_"+makeNeighborID("default", "192.168.0.2")+"_transitions"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+makeNeighborID("default", "192.168.0.2")+"_churn"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+makeNeighborID("default", "192.168.0.2")+"_message_types"))
	require.NotNil(t, collr.Charts().Get(neighborLastResetStateChartID(makeNeighborID("default", "192.168.0.2"))))
	assert.Len(t, *collr.Charts(), len(collectorCharts)+len(familyChartsTmpl)*2+len(peerChartsTmpl)*5+3*5+3*5+4)
}

func TestCollector_CollectDeepPeerPrefixMetricsSkipsCheapFRRDiagnostics(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDeep,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsRich,
	})
	collr.DeepPeerPrefixMetrics = true

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(4), mx["peer_"+makePeerID("default_ipv4_unicast", "192.168.0.2")+"_prefixes_advertised"])
	assert.Equal(t, int64(2), mx["peer_"+makePeerID("red_ipv4_unicast", "192.168.1.2")+"_prefixes_advertised"])
}

func TestCollector_CollectSessionCountersOnceForDualStackPeer(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDualstack,
			"ipv6": dataFRRIPv6SummaryDualstack,
		},
		neighbors: dataFRRNeighborsDualstack,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	ipv4PeerID := makePeerID("default_ipv4_unicast", "192.0.2.2")
	ipv6PeerID := makePeerID("default_ipv6_unicast", "192.0.2.2")
	neighborID := makeNeighborID("default", "192.0.2.2")

	assert.Equal(t, int64(5), mx["family_default_ipv4_unicast_prefixes_received"])
	assert.Equal(t, int64(3), mx["family_default_ipv6_unicast_prefixes_received"])
	assert.Equal(t, int64(4), mx["neighbor_"+neighborID+"_connections_established"])
	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_connections_dropped"])
	assert.Equal(t, int64(13), mx["neighbor_"+neighborID+"_churn_updates_received"])
	assert.Equal(t, int64(12), mx["neighbor_"+neighborID+"_churn_updates_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_notifications_received"])
	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_churn_notifications_sent"])
	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_churn_route_refresh_received"])
	assert.Equal(t, int64(2), mx["neighbor_"+neighborID+"_churn_route_refresh_sent"])
	assert.Equal(t, int64(13), mx["neighbor_"+neighborID+"_updates_received"])
	assert.Equal(t, int64(12), mx["neighbor_"+neighborID+"_updates_sent"])
	assert.Equal(t, int64(81), mx["neighbor_"+neighborID+"_keepalives_received"])
	assert.Equal(t, int64(80), mx["neighbor_"+neighborID+"_keepalives_sent"])
	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_route_refresh_received"])
	assert.Equal(t, int64(2), mx["neighbor_"+neighborID+"_route_refresh_sent"])
	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_last_reset_never"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_reset_soft_or_unknown"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_reset_hard"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_reset_age_seconds"])
	assert.Equal(t, int64(6), mx["peer_"+ipv4PeerID+"_prefixes_advertised"])
	assert.Equal(t, int64(5), mx["peer_"+ipv6PeerID+"_prefixes_advertised"])
	assert.Equal(t, int64(4), mx["peer_"+ipv4PeerID+"_prefixes_accepted"])
	assert.Equal(t, int64(2), mx["peer_"+ipv6PeerID+"_prefixes_accepted"])

	_, ok := mx["family_default_ipv4_unicast_connections_established"]
	assert.False(t, ok)
	_, ok = mx["family_default_ipv6_unicast_connections_established"]
	assert.False(t, ok)
	_, ok = mx["peer_"+ipv4PeerID+"_connections_established"]
	assert.False(t, ok)
	_, ok = mx["peer_"+ipv6PeerID+"_connections_established"]
	assert.False(t, ok)

	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_transitions"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_churn"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_message_types"))
	require.NotNil(t, collr.Charts().Get(neighborLastResetStateChartID(neighborID)))
	require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(ipv4PeerID)))
	require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(ipv6PeerID)))
	assert.Len(t, *collr.Charts(), len(collectorCharts)+len(familyChartsTmpl)*2+len(peerChartsTmpl)*2+3+3+4)
}

func TestCollector_ReusesNeighborDetailsCacheOnNeighborQueryError(t *testing.T) {
	mock := &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDeep,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsRich,
	}
	collr := newTestCollector(t, mock)

	first := collr.Collect(context.Background())
	require.NotNil(t, first)
	first = assertAndStripCollectorMetrics(t, first, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	neighborID := makeNeighborID("default", "192.168.0.2")
	assert.Equal(t, int64(3), first["neighbor_"+neighborID+"_connections_established"])
	assert.Equal(t, int64(12), first["neighbor_"+neighborID+"_updates_received"])

	mock.neighbors = nil
	mock.neighborsErr = errors.New("mock neighbors query failed")

	second := collr.Collect(context.Background())
	require.NotNil(t, second)
	second = assertAndStripCollectorMetrics(t, second, collectorStatusOK, 1, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(3), second["neighbor_"+neighborID+"_connections_established"])
	assert.Equal(t, int64(12), second["neighbor_"+neighborID+"_updates_received"])
	assert.Equal(t, int64(1), second["neighbor_"+neighborID+"_last_reset_hard"])
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_transitions"))
}

func mergeMetricSets(sets ...map[string]int64) map[string]int64 {
	merged := make(map[string]int64)
	for _, set := range sets {
		for key, value := range set {
			merged[key] = value
		}
	}
	return merged
}

func neighborMetricSet(id string, established, dropped, updatesReceived, updatesSent, notificationsReceived, notificationsSent, keepalivesReceived, keepalivesSent, routeRefreshReceived, routeRefreshSent int64) map[string]int64 {
	prefix := "neighbor_" + id + "_"
	return map[string]int64{
		prefix + "connections_established": established,
		prefix + "connections_dropped":     dropped,
		prefix + "updates_received":        updatesReceived,
		prefix + "updates_sent":            updatesSent,
		prefix + "notifications_received":  notificationsReceived,
		prefix + "notifications_sent":      notificationsSent,
		prefix + "keepalives_received":     keepalivesReceived,
		prefix + "keepalives_sent":         keepalivesSent,
		prefix + "route_refresh_received":  routeRefreshReceived,
		prefix + "route_refresh_sent":      routeRefreshSent,
	}
}

func neighborResetMetricSet(id string, never, softOrUnknown, hard, ageSecs, resetCode, errorCode, errorSubcode int64) map[string]int64 {
	prefix := "neighbor_" + id + "_"
	return map[string]int64{
		prefix + "last_reset_never":           never,
		prefix + "last_reset_soft_or_unknown": softOrUnknown,
		prefix + "last_reset_hard":            hard,
		prefix + "last_reset_age_seconds":     ageSecs,
		prefix + "last_reset_code":            resetCode,
		prefix + "last_error_code":            errorCode,
		prefix + "last_error_subcode":         errorSubcode,
	}
}

func neighborChurnMetricSet(id string, updatesReceived, updatesSent, notificationsReceived, notificationsSent, routeRefreshReceived, routeRefreshSent int64) map[string]int64 {
	prefix := "neighbor_" + id + "_churn_"
	return map[string]int64{
		prefix + "updates_received":       updatesReceived,
		prefix + "updates_sent":           updatesSent,
		prefix + "withdraws_received":     0,
		prefix + "withdraws_sent":         0,
		prefix + "notifications_received": notificationsReceived,
		prefix + "notifications_sent":     notificationsSent,
		prefix + "route_refresh_received": routeRefreshReceived,
		prefix + "route_refresh_sent":     routeRefreshSent,
	}
}

func peerPolicyMetricSet(id string, accepted, filtered int64) map[string]int64 {
	prefix := "peer_" + id + "_"
	return map[string]int64{
		prefix + "prefixes_accepted": accepted,
		prefix + "prefixes_filtered": filtered,
	}
}
