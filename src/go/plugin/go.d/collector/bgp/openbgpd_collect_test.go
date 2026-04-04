// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataOpenBGPDRIBIPv4Only  = []byte(`{"rib":[{"prefix":"23.42.1.0/24","ovs":"not-found"}]}`)
	dataOpenBGPDRIBIPv6Only  = []byte(`{"rib":[{"prefix":"fc29:bac0:167::/48","ovs":"valid"}]}`)
	dataOpenBGPDRIBVPNv4Only = []byte(`{"rib":[{"prefix":"rd 65010:1 198.51.100.0/24","ovs":"valid"}]}`)
	dataOpenBGPDRIBVPNv6Only = []byte(`{"rib":[{"prefix":"rd 65010:1 2001:db8::/64","ovs":"invalid"}]}`)
)

func TestCollector_CollectOpenBGPD(t *testing.T) {
	mock := &mockClient{
		neighbors: dataOpenBGPDNeighbors,
		ribFiltered: map[string][]byte{
			"ipv4": dataOpenBGPDRIBIPv4Only,
			"ipv6": dataOpenBGPDRIBIPv6Only,
		},
	}

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.SocketPath = ""
	collr.APIURL = "http://127.0.0.1:8080"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("default_ipv4_unicast", 1, 0, 0, 1, 1, 22161, 22613, 1, 1) {
		expected[k] = v
	}
	expected["family_default_ipv4_unicast_correctness_valid"] = 0
	expected["family_default_ipv4_unicast_correctness_invalid"] = 0
	expected["family_default_ipv4_unicast_correctness_not_found"] = 1

	for k, v := range familyMetricSet("default_ipv6_unicast", 1, 0, 0, 1, 1, 22160, 22289, 0, 1) {
		expected[k] = v
	}
	expected["family_default_ipv6_unicast_correctness_valid"] = 1
	expected["family_default_ipv6_unicast_correctness_invalid"] = 0
	expected["family_default_ipv6_unicast_correctness_not_found"] = 0

	peer4Scope := openbgpdPeerScope(openbgpdNeighbor{Group: "clients", BGPID: "206.100.25.7"}, 123)
	peer4ID := makePeerIDWithScope("default_ipv4_unicast", "200.100.25.7", peer4Scope)
	for k, v := range peerMetricSet(peer4ID, 22161, 22613, 1, 662400, peerStateUp) {
		expected[k] = v
	}
	expected["peer_"+peer4ID+"_prefixes_advertised"] = 864

	peer6Scope := openbgpdPeerScope(openbgpdNeighbor{Group: "clients", BGPID: "1.2.3.4"}, 12346)
	peer6ID := makePeerIDWithScope("default_ipv6_unicast", "2000:1:2::3", peer6Scope)
	for k, v := range peerMetricSet(peer6ID, 22160, 22289, 0, 662400, peerStateUp) {
		expected[k] = v
	}
	expected["peer_"+peer6ID+"_prefixes_advertised"] = 127

	neighbor4ID := makeNeighborIDWithScope("default", "200.100.25.7", peer4Scope)
	expected["neighbor_"+neighbor4ID+"_updates_received"] = 1
	expected["neighbor_"+neighbor4ID+"_updates_sent"] = 897
	expected["neighbor_"+neighbor4ID+"_notifications_received"] = 0
	expected["neighbor_"+neighbor4ID+"_notifications_sent"] = 0
	expected["neighbor_"+neighbor4ID+"_keepalives_received"] = 22158
	expected["neighbor_"+neighbor4ID+"_keepalives_sent"] = 22152
	expected["neighbor_"+neighbor4ID+"_route_refresh_received"] = 0
	expected["neighbor_"+neighbor4ID+"_route_refresh_sent"] = 0
	expected["neighbor_"+neighbor4ID+"_churn_updates_received"] = 1
	expected["neighbor_"+neighbor4ID+"_churn_updates_sent"] = 897
	expected["neighbor_"+neighbor4ID+"_churn_withdraws_received"] = 0
	expected["neighbor_"+neighbor4ID+"_churn_withdraws_sent"] = 7
	expected["neighbor_"+neighbor4ID+"_churn_notifications_received"] = 0
	expected["neighbor_"+neighbor4ID+"_churn_notifications_sent"] = 0
	expected["neighbor_"+neighbor4ID+"_churn_route_refresh_received"] = 0
	expected["neighbor_"+neighbor4ID+"_churn_route_refresh_sent"] = 0

	neighbor6ID := makeNeighborIDWithScope("default", "2000:1:2::3", peer6Scope)
	expected["neighbor_"+neighbor6ID+"_updates_received"] = 0
	expected["neighbor_"+neighbor6ID+"_updates_sent"] = 131
	expected["neighbor_"+neighbor6ID+"_notifications_received"] = 0
	expected["neighbor_"+neighbor6ID+"_notifications_sent"] = 0
	expected["neighbor_"+neighbor6ID+"_keepalives_received"] = 22158
	expected["neighbor_"+neighbor6ID+"_keepalives_sent"] = 22155
	expected["neighbor_"+neighbor6ID+"_route_refresh_received"] = 0
	expected["neighbor_"+neighbor6ID+"_route_refresh_sent"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_updates_received"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_updates_sent"] = 131
	expected["neighbor_"+neighbor6ID+"_churn_withdraws_received"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_withdraws_sent"] = 2
	expected["neighbor_"+neighbor6ID+"_churn_notifications_received"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_notifications_sent"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_route_refresh_received"] = 0
	expected["neighbor_"+neighbor6ID+"_churn_route_refresh_sent"] = 0

	assert.Equal(t, expected, mx)
	assert.Equal(t, []string{"ipv4", "ipv6"}, mock.ribFilteredCalls)
	assert.Equal(t, 0, mock.ribCalls)

	require.NotNil(t, collr.Charts().Get("family_default_ipv4_unicast_correctness"))
	peerChart := collr.Charts().Get("peer_" + peer4ID + "_messages")
	require.NotNil(t, peerChart)
	assert.Equal(t, "206.126.225.254", chartLabelValue(peerChart, "local_address"))

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, "http://127.0.0.1:8080", chartLabelValue(collectorChart, "target"))
}

func TestCollector_OpenBGPDRIBCache(t *testing.T) {
	mock := &mockClient{
		neighbors: dataOpenBGPDNeighbors,
		ribFiltered: map[string][]byte{
			"ipv4": dataOpenBGPDRIBIPv4Only,
			"ipv6": dataOpenBGPDRIBIPv6Only,
		},
	}

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.SocketPath = ""
	collr.APIURL = "http://127.0.0.1:8080"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Hour)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	require.NotNil(t, collr.Collect(context.Background()))
	mock.ribFilteredErrs = map[string]error{
		"ipv4": assert.AnError,
		"ipv6": assert.AnError,
	}
	require.NotNil(t, collr.Collect(context.Background()))
	assert.Equal(t, []string{"ipv4", "ipv6"}, mock.ribFilteredCalls)
	assert.Equal(t, 0, mock.ribCalls)
}

func TestCollector_OpenBGPDSkipsRIBWhenNoFamiliesSelected(t *testing.T) {
	mock := &mockClient{
		neighbors: dataOpenBGPDNeighbors,
		rib:       dataOpenBGPDRIB,
	}

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.SocketPath = ""
	collr.APIURL = "http://127.0.0.1:8080"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.MaxFamilies = 1
	collr.SelectFamilies = matcher.SimpleExpr{Includes: []string{"=does-not-match/ipv4/unicast"}}
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	_ = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, 0, mock.ribCalls)
}

func TestCollector_OpenBGPDFallsBackToUnfilteredRIBWhenFilteredQueriesUnsupported(t *testing.T) {
	mock := &mockClient{
		neighbors: dataOpenBGPDNeighbors,
		rib:       dataOpenBGPDRIB,
	}

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.SocketPath = ""
	collr.APIURL = "http://127.0.0.1:8080"
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_rib_routes"])
	assert.Equal(t, int64(1), mx["family_default_ipv6_unicast_rib_routes"])
	assert.Equal(t, []string{"ipv4"}, mock.ribFilteredCalls)
	assert.Equal(t, 1, mock.ribCalls)
}

func TestShouldCollectOpenBGPDRIBSummaries(t *testing.T) {
	families := []familyStats{
		{ID: "default_ipv4_unicast", AFI: "ipv4", SAFI: "unicast"},
		{ID: "default_ipv4_vpn", AFI: "ipv4", SAFI: "vpn"},
		{ID: "default_ipv4_flowspec", AFI: "ipv4", SAFI: "flowspec"},
		{ID: "default_l2vpn_evpn", AFI: "l2vpn", SAFI: "evpn"},
	}

	assert.True(t, shouldCollectOpenBGPDRIBSummaries(families, map[string]bool{"default_ipv4_unicast": true}))
	assert.True(t, shouldCollectOpenBGPDRIBSummaries(families, map[string]bool{"default_ipv4_vpn": true}))
	assert.False(t, shouldCollectOpenBGPDRIBSummaries(families, map[string]bool{"default_ipv4_flowspec": true}))
	assert.False(t, shouldCollectOpenBGPDRIBSummaries(families, map[string]bool{"default_l2vpn_evpn": true}))
	assert.False(t, shouldCollectOpenBGPDRIBSummaries(families, map[string]bool{}))
}

func TestCollector_CollectOpenBGPDRIBSummariesVPNFamilies(t *testing.T) {
	mock := &mockClient{
		ribFiltered: map[string][]byte{
			"vpnv4": dataOpenBGPDRIBVPNv4Only,
			"vpnv6": dataOpenBGPDRIBVPNv6Only,
		},
	}

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)

	families := []familyStats{
		{ID: "default_ipv4_vpn", AFI: "ipv4", SAFI: "vpn"},
		{ID: "default_ipv6_vpn", AFI: "ipv6", SAFI: "vpn"},
		{ID: "default_l2vpn_evpn", AFI: "l2vpn", SAFI: "evpn"},
	}
	selectedFamilies := map[string]bool{
		"default_ipv4_vpn":   true,
		"default_ipv6_vpn":   true,
		"default_l2vpn_evpn": true,
	}

	summaries := collr.collectOpenBGPDRIBSummaries(mock, families, selectedFamilies, &scrapeMetrics{})

	assert.Equal(t, map[string]openbgpdRIBSummary{
		"default_ipv4_vpn": {
			RIBRoutes: 1,
			Valid:     1,
		},
		"default_ipv6_vpn": {
			RIBRoutes: 1,
			Invalid:   1,
		},
	}, summaries)
	assert.Equal(t, []string{"vpnv4", "vpnv6"}, mock.ribFilteredCalls)
	assert.Equal(t, 0, mock.ribCalls)
}
