// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON           []byte
	dataConfigYAML           []byte
	dataFRRIPv4Summary       []byte
	dataFRRIPv4SummaryDeep   []byte
	dataFRRIPv4SummaryPfxSnt []byte
	dataFRRIPv6Summary       []byte
	dataFRREVPNSummary       []byte
	dataFRREVPNVNI           []byte
	dataFRRNeighbors         []byte
	dataFRRNeighborsEnriched []byte
	dataFRRPeerRoutesDefault []byte
	dataFRRPeerRoutesRed     []byte
	dataFRRPeerAdvDefault    []byte
	dataFRRPeerAdvRed        []byte
	dataFRREmptySummary      = []byte("{}")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataFRRIPv4Summary":       dataFRRIPv4Summary,
		"dataFRRIPv6Summary":       dataFRRIPv6Summary,
		"dataFRREVPNSummary":       dataFRREVPNSummary,
		"dataFRREVPNVNI":           dataFRREVPNVNI,
		"dataFRRNeighbors":         dataFRRNeighbors,
		"dataFRRPeerRoutesDefault": dataFRRPeerRoutesDefault,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		collr := New()
		collr.newClient = func(Config) (bgpClient, error) { return &mockClient{}, nil }
		require.NoError(t, collr.Init(context.Background()))
	})

	t.Run("invalid backend", func(t *testing.T) {
		collr := New()
		collr.Backend = "bogus"
		collr.newClient = func(Config) (bgpClient, error) { return &mockClient{}, nil }
		require.Error(t, collr.Init(context.Background()))
	})

	t.Run("invalid selector", func(t *testing.T) {
		collr := New()
		collr.SelectPeers.Includes = []string{"this-is-not-a-matcher"}
		collr.newClient = func(Config) (bgpClient, error) { return &mockClient{}, nil }
		require.Error(t, collr.Init(context.Background()))
	})

	t.Run("invalid family selector", func(t *testing.T) {
		collr := New()
		collr.SelectFamilies.Includes = []string{"this-is-not-a-matcher"}
		collr.newClient = func(Config) (bgpClient, error) { return &mockClient{}, nil }
		require.Error(t, collr.Init(context.Background()))
	})
}

func TestCollector_Check(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4Summary,
			"ipv6": dataFRREmptySummary,
		},
	})

	require.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckFailsWithoutBGPFamilies(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRREmptySummary,
			"ipv6": dataFRREmptySummary,
		},
	})

	require.Error(t, collr.Check(context.Background()))
}

func TestCollector_CheckError(t *testing.T) {
	collr := newTestCollector(t, &mockClient{err: errors.New("mock Summary() error")})
	require.Error(t, collr.Check(context.Background()))
}

func TestCollector_CollectAllPeersBelowLimit(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4Summary,
			"ipv6": dataFRREmptySummary,
		},
	})

	mx := collr.Collect(context.Background())
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("default_ipv4_unicast", 1, 1, 1, 3, 3, 100, 100, 4, 1) {
		expected[k] = v
	}
	for k, v := range familyMetricSet("red_ipv4_unicast", 1, 0, 1, 2, 2, 300, 300, 2, 0) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 100, 100, 0, 10, peerStateUp) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.3"), 0, 0, 2, 0, peerStateDown) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.4"), 0, 0, 2, 0, peerStateAdminDown) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.2"), 100, 100, 2, 20, peerStateUp) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("red_ipv4_unicast", "192.168.1.3"), 200, 200, 0, 0, peerStateDown) {
		expected[k] = v
	}
	assert.Equal(t, expected, mx)
	assert.Len(t, *collr.Charts(), len(collectorCharts)+len(familyChartsTmpl)*2+len(peerChartsTmpl)*5)
}

func TestCollector_SelectPeersWhenOverLimit(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4Summary,
			"ipv6": dataFRREmptySummary,
		},
	})
	collr.MaxPeers = 2
	collr.SelectPeers.Includes = []string{"=192.168.0.2"}
	collr.selectPeerMatcher = nil

	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("default_ipv4_unicast", 1, 1, 1, 3, 1, 100, 100, 4, 1) {
		expected[k] = v
	}
	for k, v := range familyMetricSet("red_ipv4_unicast", 1, 0, 1, 2, 0, 300, 300, 2, 0) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 100, 100, 0, 10, peerStateUp) {
		expected[k] = v
	}
	assert.Equal(t, expected, mx)
	assert.Len(t, *collr.Charts(), len(collectorCharts)+len(familyChartsTmpl)*2+len(peerChartsTmpl))
}

func TestCollector_SelectFamiliesWhenOverLimit(t *testing.T) {
	collr := New()
	collr.MaxFamilies = 1
	collr.SelectFamilies.Includes = []string{"=default/ipv4/unicast"}
	collr.newClient = func(Config) (bgpClient, error) {
		return &mockClient{
			responses: map[string][]byte{
				"ipv4": dataFRRIPv4Summary,
				"ipv6": dataFRREmptySummary,
			},
		}, nil
	}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{}
	for k, v := range familyMetricSet("default_ipv4_unicast", 1, 1, 1, 3, 3, 100, 100, 4, 1) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.2"), 100, 100, 0, 10, peerStateUp) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.3"), 0, 0, 2, 0, peerStateDown) {
		expected[k] = v
	}
	for k, v := range peerMetricSet(makePeerID("default_ipv4_unicast", "192.168.0.4"), 0, 0, 2, 0, peerStateAdminDown) {
		expected[k] = v
	}
	assert.Equal(t, expected, mx)
	assert.Nil(t, collr.Charts().Get("family_red_ipv4_unicast_peer_states"))
}

func TestCollector_ObsoleteChartsAfterAbsence(t *testing.T) {
	mock := &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4Summary,
			"ipv6": dataFRREmptySummary,
		},
	}
	collr := newTestCollector(t, mock)
	collr.cleanupEvery = 0
	collr.obsoleteAfter = time.Minute

	require.NotNil(t, collr.Collect(context.Background()))
	require.NotEmpty(t, *collr.Charts())

	for id := range collr.familySeen {
		collr.familySeen[id] = time.Now().Add(-2 * time.Minute)
	}
	for id := range collr.peerSeen {
		collr.peerSeen[id] = time.Now().Add(-2 * time.Minute)
	}
	mock.responses["ipv4"] = dataFRREmptySummary
	mock.responses["ipv6"] = dataFRREmptySummary

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	_ = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)
	for _, chart := range *collr.Charts() {
		if chart.ID == "collector_status" || chart.ID == "collector_scrape_duration" || chart.ID == "collector_failures" || chart.ID == "collector_deep_queries" {
			assert.Falsef(t, chart.Obsolete, "collector chart %s should remain active", chart.ID)
			continue
		}
		assert.Truef(t, chart.Obsolete, "dynamic chart %s should be obsolete", chart.ID)
	}
}

func TestParseFRRSummary(t *testing.T) {
	families, err := parseFRRSummary(dataFRRIPv4Summary, "ipv4", "unicast", backendFRR, nil)
	require.NoError(t, err)
	require.Len(t, families, 2)

	byID := make(map[string]familyStats, len(families))
	for _, family := range families {
		byID[family.ID] = family
	}

	def := byID["default_ipv4_unicast"]
	assert.Equal(t, int64(64512), def.LocalAS)
	assert.Equal(t, int64(1), def.RIBRoutes)
	assert.Equal(t, int64(3), def.ConfiguredPeers)
	assert.Equal(t, int64(1), def.PeersEstablished)
	assert.Equal(t, int64(1), def.PeersAdminDown)
	assert.Equal(t, int64(1), def.PeersDown)
	assert.Equal(t, int64(100), def.MessagesReceived)
	assert.Equal(t, int64(100), def.MessagesSent)
	assert.Equal(t, int64(4), def.PrefixesReceived)
	require.Len(t, def.Peers, 3)
	assert.Equal(t, "192.168.0.4", def.Peers[2].Address)
	assert.Equal(t, peerStateAdminDown, def.Peers[2].State)

	red := byID["red_ipv4_unicast"]
	assert.Equal(t, int64(64612), red.LocalAS)
	assert.Equal(t, int64(0), red.RIBRoutes)
	assert.Equal(t, int64(2), red.ConfiguredPeers)
	assert.Equal(t, int64(300), red.MessagesReceived)
	assert.Equal(t, int64(300), red.MessagesSent)
	assert.Equal(t, int64(2), red.PrefixesReceived)
}

func TestParseFRRNeighborsBasicMetadata(t *testing.T) {
	details, err := parseFRRNeighbors(dataFRRNeighbors)
	require.NoError(t, err)

	require.Contains(t, details, "default")
	assert.Equal(t, "fw1", details["default"]["swp2"].Desc)
	assert.Equal(t, "rt1", details["default"]["10.1.1.10"].Desc)
	assert.Equal(t, "remote", details["vrf1"]["10.2.0.1"].Desc)
}

func TestParseFRRPrefixCounter(t *testing.T) {
	count, err := parseFRRPrefixCounter(dataFRRPeerAdvDefault)
	require.NoError(t, err)
	assert.Equal(t, int64(4), count)

	count, err = parseFRRPrefixCounter(dataFRRPeerRoutesRed)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestBuildFRRPeerCommand(t *testing.T) {
	assert.Equal(t,
		"show bgp ipv4 unicast neighbors 192.0.2.1 routes json",
		buildFRRPeerCommand("default", "ipv4", "unicast", "192.0.2.1", "routes"),
	)
	assert.Equal(t,
		"show bgp vrf red ipv4 unicast neighbors 192.0.2.1 advertised-routes json",
		buildFRRPeerCommand("red", "ipv4", "unicast", "192.0.2.1", "advertised-routes"),
	)
}

func TestParseFamilyKey(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		requestedAFI string
		wantAFI      string
		wantSAFI     string
	}{
		{name: "ipv4 unicast", key: "ipv4Unicast", requestedAFI: "ipv4", wantAFI: "ipv4", wantSAFI: "unicast"},
		{name: "ipv6 unicast", key: " ipv6Unicast ", requestedAFI: "ipv6", wantAFI: "ipv6", wantSAFI: "unicast"},
		{name: "flowspec", key: "ipv4FlowSpec", requestedAFI: "ipv4", wantAFI: "ipv4", wantSAFI: "flowspec"},
		{name: "evpn", key: "l2VpnEvpn", requestedAFI: "ipv4", wantAFI: "l2vpn", wantSAFI: "evpn"},
		{name: "fallback afi", key: "mysteryFamily", requestedAFI: "IPv6", wantAFI: "ipv6", wantSAFI: "mysteryfamily"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			afi, safi := parseFamilyKey(tc.key, tc.requestedAFI)
			assert.Equal(t, tc.wantAFI, afi)
			assert.Equal(t, tc.wantSAFI, safi)
		})
	}
}

func TestMapPeerState(t *testing.T) {
	tests := []struct {
		value string
		want  int64
	}{
		{value: "Established", want: peerStateUp},
		{value: " established ", want: peerStateUp},
		{value: "Idle (Admin)", want: peerStateAdminDown},
		{value: "Idle(Admin)", want: peerStateAdminDown},
		{value: "Administratively down", want: peerStateAdminDown},
		{value: "OpenConfirm", want: peerStateDown},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, mapPeerState(tc.value), tc.value)
	}
}

func TestCollector_CollectEnrichedPeerMetadataAndAdvertisedPrefixes(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryPfxSnt,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsEnriched,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	peerID := makePeerID("default_ipv4_unicast", "192.168.0.2")
	assert.Equal(t, int64(3), mx["peer_"+peerID+"_prefixes_advertised"])
	assert.Equal(t, int64(0), mx["peer_"+makePeerID("default_ipv4_unicast", "192.168.0.3")+"_prefixes_advertised"])

	msgChart := collr.Charts().Get("peer_" + peerID + "_messages")
	require.NotNil(t, msgChart)
	assert.Equal(t, "edge-01", chartLabelValue(msgChart, "peer_desc"))
	assert.Equal(t, "transit", chartLabelValue(msgChart, "peer_group"))

	advChart := collr.Charts().Get(peerAdvertisedPrefixesChartID(peerID))
	require.NotNil(t, advChart)
	assert.Equal(t, "edge-01", chartLabelValue(advChart, "peer_desc"))
	assert.Equal(t, "transit", chartLabelValue(advChart, "peer_group"))
	assert.Equal(t, "vrf default / IPv4 / unicast peer 192.168.0.2", advChart.Fam)
	require.NotNil(t, collr.Charts().Get(neighborLastResetStateChartID(makeNeighborID("default", "192.168.0.2"))))

	assert.Len(t, *collr.Charts(), len(collectorCharts)+len(familyChartsTmpl)*2+len(peerChartsTmpl)*5+3*5+3*5+5)
}

func TestCollector_CollectDeepPeerPrefixMetrics(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDeep,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsEnriched,
		peerRoutes: map[string][]byte{
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "routes"): dataFRRPeerRoutesDefault,
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "routes"):     dataFRRPeerRoutesRed,
		},
		peerAdvertisedRoutes: map[string][]byte{
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "advertised-routes"): dataFRRPeerAdvDefault,
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "advertised-routes"):     dataFRRPeerAdvRed,
		},
	})
	collr.DeepPeerPrefixMetrics = true

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 4, 0, 0)

	defaultPeerID := makePeerID("default_ipv4_unicast", "192.168.0.2")
	assert.Equal(t, int64(2), mx["peer_"+defaultPeerID+"_prefixes_accepted"])
	assert.Equal(t, int64(1), mx["peer_"+defaultPeerID+"_prefixes_filtered"])
	assert.Equal(t, int64(4), mx["peer_"+defaultPeerID+"_prefixes_advertised"])

	redPeerID := makePeerID("red_ipv4_unicast", "192.168.1.2")
	assert.Equal(t, int64(1), mx["peer_"+redPeerID+"_prefixes_accepted"])
	assert.Equal(t, int64(1), mx["peer_"+redPeerID+"_prefixes_filtered"])
	assert.Equal(t, int64(2), mx["peer_"+redPeerID+"_prefixes_advertised"])

	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(defaultPeerID)))
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(redPeerID)))
	assert.Equal(t, "vrf default / IPv4 / unicast peer 192.168.0.2", collr.Charts().Get(peerPolicyChartID(defaultPeerID)).Fam)
	assert.Equal(t, "vrf red / IPv4 / unicast peer 192.168.1.2", collr.Charts().Get(peerPolicyChartID(redPeerID)).Fam)
}

func TestCollector_CollectDeepPeerPrefixMetricsRespectsBudget(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryDeep,
			"ipv6": dataFRREmptySummary,
		},
		neighbors: dataFRRNeighborsEnriched,
		peerRoutes: map[string][]byte{
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "routes"): dataFRRPeerRoutesDefault,
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "routes"):     dataFRRPeerRoutesRed,
		},
		peerAdvertisedRoutes: map[string][]byte{
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "advertised-routes"): dataFRRPeerAdvDefault,
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "advertised-routes"):     dataFRRPeerAdvRed,
		},
	})
	collr.DeepPeerPrefixMetrics = true
	collr.MaxDeepQueriesPerScrape = 1

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 1, 0, 2)

	defaultPeerID := makePeerID("default_ipv4_unicast", "192.168.0.2")
	assert.Equal(t, int64(2), mx["peer_"+defaultPeerID+"_prefixes_accepted"])
	assert.Equal(t, int64(1), mx["peer_"+defaultPeerID+"_prefixes_filtered"])
	_, ok := mx["peer_"+defaultPeerID+"_prefixes_advertised"]
	assert.False(t, ok)

	redPeerID := makePeerID("red_ipv4_unicast", "192.168.1.2")
	_, ok = mx["peer_"+redPeerID+"_prefixes_accepted"]
	assert.False(t, ok)
}

// TestCollector_CollectDeepPeerPrefixMetricsCoversColdScrapeNeighborFailure
// covers the realistic modern-FRR activation case for the deep fallback that
// the other tests in this file do not exercise: a cold scrape on any FRR
// version (including current 10.6) where the "show bgp vrf all neighbors json"
// query or parse fails before the collector has a warm neighbor cache. In
// that state detailsByVRF stays nil in collectFRRData, applyNeighborDetails
// is never called, HasPrefixPolicy stays false, and the prefix-policy deep
// query fires even though the daemon is modern. HasPrefixesSent is still
// satisfied by the summary pfxSnt path on FRR 8.0 or later, so the advertised-routes
// branch of the deep fallback must NOT fire. See the comment at
// collectDeepPeerPrefixMetrics for the full set of realistic activation
// situations for this code path.
func TestCollector_CollectDeepPeerPrefixMetricsCoversColdScrapeNeighborFailure(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4SummaryPfxSnt,
			"ipv6": dataFRREmptySummary,
		},
		// Simulate a cold-scrape Neighbors() failure: the cache is empty
		// (newTestCollector starts with a fresh collector) and the remote
		// query fails, so detailsByVRF stays nil for this scrape.
		neighborsErr: errors.New("mock Neighbors() error"),
		peerRoutes: map[string][]byte{
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "routes"): dataFRRPeerRoutesDefault,
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "routes"):     dataFRRPeerRoutesRed,
		},
		// Intentionally do NOT provide peerAdvertisedRoutes. If the deep
		// fallback incorrectly tries to fetch advertised routes (i.e. treats
		// HasPrefixesSent as false despite summary pfxSnt being present),
		// the mockClient will return an empty response and the assertions
		// on prefixes_advertised below would fail.
	})
	collr.DeepPeerPrefixMetrics = true

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	// query=1 because Neighbors() failed once (non-fatal, continues).
	// deep attempted=2: one policy query per Established peer
	// (192.168.0.2 in default, 192.168.1.2 in red). No advertised queries
	// are attempted because the summary pfxSnt path already set
	// HasPrefixesSent on both peers.
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 1, 0, 0, 2, 0, 0)

	defaultPeerID := makePeerID("default_ipv4_unicast", "192.168.0.2")
	redPeerID := makePeerID("red_ipv4_unicast", "192.168.1.2")

	// Accepted counts come from the deep policy query, proving the fallback
	// ran for both Established peers despite the missing neighbor metadata.
	assert.Equal(t, int64(2), mx["peer_"+defaultPeerID+"_prefixes_accepted"])
	assert.Equal(t, int64(1), mx["peer_"+redPeerID+"_prefixes_accepted"])

	// Advertised counts come from the summary pfxSnt field, NOT from any
	// deep advertised-routes query. dataFRRIPv4SummaryPfxSnt has pfxSnt=3
	// for 192.168.0.2 and pfxSnt=2 for 192.168.1.2.
	assert.Equal(t, int64(3), mx["peer_"+defaultPeerID+"_prefixes_advertised"])
	assert.Equal(t, int64(2), mx["peer_"+redPeerID+"_prefixes_advertised"])

	// The prefix-policy chart is instantiated for both peers because the
	// deep fallback successfully populated the policy metrics.
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(defaultPeerID)))
	require.NotNil(t, collr.Charts().Get(peerPolicyChartID(redPeerID)))
}

func TestCollector_CollectWithoutNeighborMetadata(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": dataFRRIPv4Summary,
			"ipv6": dataFRREmptySummary,
		},
		neighborsErr: errors.New("mock Neighbors() error"),
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 1, 0, 0, 0, 0, 0)

	chart := collr.Charts().Get("peer_" + makePeerID("default_ipv4_unicast", "192.168.0.2") + "_messages")
	require.NotNil(t, chart)
	assert.Equal(t, "", chartLabelValue(chart, "peer_desc"))
	assert.Equal(t, "", chartLabelValue(chart, "peer_group"))
}

func TestCollector_CollectReportsCollectorStatusOnFatalQueryError(t *testing.T) {
	collr := newTestCollector(t, &mockClient{err: errors.New("mock Summary() error")})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusQueryError, 1, 0, 0, 0, 0, 0)

	assert.Empty(t, mx)
}

func TestCollector_CollectReportsCollectorStatusOnFatalParseError(t *testing.T) {
	collr := newTestCollector(t, &mockClient{
		responses: map[string][]byte{
			"ipv4": []byte("{"),
			"ipv6": dataFRREmptySummary,
		},
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusParseError, 0, 1, 0, 0, 0, 0)

	assert.Empty(t, mx)
}

func TestCollector_CollectUsingRealFRRReplaySocket(t *testing.T) {
	server := newFRRReplayServer(t, map[string][]byte{
		"show bgp vrf all neighbors json":                                                     dataFRRNeighborsEnriched,
		"show bgp vrf all ipv4 summary json":                                                  dataFRRIPv4SummaryDeep,
		"show bgp vrf all ipv6 summary json":                                                  dataFRREmptySummary,
		"show bgp vrf all l2vpn evpn summary json":                                            dataFRREmptySummary,
		"show rpki cache-server json":                                                         dataFRRRPKICacheServer,
		"show rpki cache-connection json":                                                     dataFRRRPKICacheConnection,
		"show rpki prefix-count json":                                                         dataFRRRPKIPrefixCount,
		buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "routes"):            dataFRRPeerRoutesDefault,
		buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "advertised-routes"): dataFRRPeerAdvDefault,
		buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "routes"):                dataFRRPeerRoutesRed,
		buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "advertised-routes"):     dataFRRPeerAdvRed,
	})

	collr := New()
	collr.SocketPath = server.socketPath
	collr.DeepPeerPrefixMetrics = true
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 4, 0, 0)

	assert.Equal(t,
		[]string{
			"show bgp vrf all neighbors json",
			"show bgp vrf all ipv4 summary json",
			"show bgp vrf all ipv6 summary json",
			"show bgp vrf all l2vpn evpn summary json",
			"show rpki cache-server json",
			"show rpki cache-connection json",
			"show rpki prefix-count json",
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "routes"),
			buildFRRPeerCommand("default", "ipv4", "unicast", "192.168.0.2", "advertised-routes"),
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "routes"),
			buildFRRPeerCommand("red", "ipv4", "unicast", "192.168.1.2", "advertised-routes"),
		},
		filterReplayCommands(server.commands()),
	)

	server.assertNoError(t)
}

func TestCollector_CollectUsingRealFRRReplaySocketSkipsEVPNVNIWhenFamilyUnselected(t *testing.T) {
	server := newFRRReplayServer(t, map[string][]byte{
		"show bgp vrf all neighbors json":          dataFRRNeighborsEnriched,
		"show bgp vrf all ipv4 summary json":       dataFRRIPv4Summary,
		"show bgp vrf all ipv6 summary json":       dataFRREmptySummary,
		"show bgp vrf all l2vpn evpn summary json": dataFRREVPNSummary,
		"show rpki cache-server json":              dataFRRRPKICacheServer,
		"show rpki cache-connection json":          dataFRRRPKICacheConnection,
		"show rpki prefix-count json":              dataFRRRPKIPrefixCount,
	})

	collr := New()
	collr.SocketPath = server.socketPath
	collr.MaxFamilies = 1
	collr.SelectFamilies = matcher.SimpleExpr{Includes: []string{"=default/ipv4/unicast"}}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	_ = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t,
		[]string{
			"show bgp vrf all neighbors json",
			"show bgp vrf all ipv4 summary json",
			"show bgp vrf all ipv6 summary json",
			"show bgp vrf all l2vpn evpn summary json",
			"show rpki cache-server json",
			"show rpki cache-connection json",
			"show rpki prefix-count json",
		},
		filterReplayCommands(server.commands()),
	)

	server.assertNoError(t)
}

func TestCollector_Cleanup(t *testing.T) {
	mock := &mockClient{}
	collr := newTestCollector(t, mock)
	collr.Cleanup(context.Background())
	assert.True(t, mock.closed)
}

func newTestCollector(t *testing.T, mock *mockClient) *Collector {
	t.Helper()

	collr := New()
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

type mockClient struct {
	responses                  map[string][]byte
	neighbors                  []byte
	frrRPKICacheServers        []byte
	frrRPKICacheConnections    []byte
	frrRPKIPrefixCount         []byte
	rib                        []byte
	ribFiltered                map[string][]byte
	evpnVNI                    []byte
	protocolsAll               []byte
	peerRoutes                 map[string][]byte
	peerAdvertisedRoutes       map[string][]byte
	gobgpGlobal                *gobgpGlobalInfo
	gobgpPeers                 []*gobgpPeerInfo
	gobgpRpki                  []*gobgpRpkiInfo
	gobgpTables                map[string]uint64
	gobgpTableErrs             map[string]error
	gobgpValidation            map[string]gobgpValidationSummary
	gobgpValidationErrs        map[string]error
	gobgpTableRefs             []string
	gobgpValidationRefs        []string
	err                        error
	neighborsErr               error
	frrRPKICacheServersErr     error
	frrRPKICacheConnectionsErr error
	frrRPKIPrefixCountErr      error
	ribErr                     error
	ribFilteredErrs            map[string]error
	closed                     bool
	ribCalls                   int
	ribFilteredCalls           []string
	evpnVNICalls               int
	gobgpTableCalls            int
	gobgpValidationCalls       int
}

func (m *mockClient) Summary(afi, safi string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if data, ok := m.responses[summaryResponseKey(afi, safi)]; ok {
		return data, nil
	}
	if data, ok := m.responses[afi]; ok {
		return data, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) Neighbors() ([]byte, error) {
	if m.neighborsErr != nil {
		return nil, m.neighborsErr
	}
	if len(m.neighbors) > 0 {
		return m.neighbors, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) RPKICacheServers() ([]byte, error) {
	if m.frrRPKICacheServersErr != nil {
		return nil, m.frrRPKICacheServersErr
	}
	if len(m.frrRPKICacheServers) > 0 {
		return m.frrRPKICacheServers, nil
	}
	return []byte(`{"servers":[]}`), nil
}

func (m *mockClient) RPKICacheConnections() ([]byte, error) {
	if m.frrRPKICacheConnectionsErr != nil {
		return nil, m.frrRPKICacheConnectionsErr
	}
	if len(m.frrRPKICacheConnections) > 0 {
		return m.frrRPKICacheConnections, nil
	}
	return []byte(`{"error":"No connection to RPKI cache server."}`), nil
}

func (m *mockClient) RPKIPrefixCount() ([]byte, error) {
	if m.frrRPKIPrefixCountErr != nil {
		return nil, m.frrRPKIPrefixCountErr
	}
	if len(m.frrRPKIPrefixCount) > 0 {
		return m.frrRPKIPrefixCount, nil
	}
	return nil, errFeatureUnsupported
}

func (m *mockClient) RIB() ([]byte, error) {
	m.ribCalls++
	if m.ribErr != nil {
		return nil, m.ribErr
	}
	if len(m.rib) > 0 {
		return m.rib, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) RIBFiltered(family string) ([]byte, error) {
	m.ribFilteredCalls = append(m.ribFilteredCalls, family)
	if err, ok := m.ribFilteredErrs[family]; ok && err != nil {
		return nil, err
	}
	if data, ok := m.ribFiltered[family]; ok {
		return data, nil
	}
	return nil, errOpenBGPDFilteredRIBUnsupported
}

func (m *mockClient) EVPNVNI() ([]byte, error) {
	m.evpnVNICalls++
	if len(m.evpnVNI) > 0 {
		return m.evpnVNI, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) ProtocolsAll() ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.protocolsAll) > 0 {
		return m.protocolsAll, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) PeerRoutes(vrf, afi, safi, neighbor string) ([]byte, error) {
	if data, ok := m.peerRoutes[buildFRRPeerCommand(vrf, afi, safi, neighbor, "routes")]; ok {
		return data, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) PeerAdvertisedRoutes(vrf, afi, safi, neighbor string) ([]byte, error) {
	if data, ok := m.peerAdvertisedRoutes[buildFRRPeerCommand(vrf, afi, safi, neighbor, "advertised-routes")]; ok {
		return data, nil
	}
	return dataFRREmptySummary, nil
}

func (m *mockClient) Close() error {
	m.closed = true
	return nil
}

func (m *mockClient) GetBgp() (*gobgpGlobalInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.gobgpGlobal != nil {
		return m.gobgpGlobal, nil
	}
	return &gobgpGlobalInfo{}, nil
}

func (m *mockClient) ListPeers() ([]*gobgpPeerInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.gobgpPeers != nil {
		return m.gobgpPeers, nil
	}
	return nil, nil
}

func (m *mockClient) ListRpki() ([]*gobgpRpkiInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.gobgpRpki != nil {
		return m.gobgpRpki, nil
	}
	return nil, nil
}

func (m *mockClient) GetTable(ref *gobgpFamilyRef) (uint64, error) {
	m.gobgpTableCalls++
	if ref != nil {
		m.gobgpTableRefs = append(m.gobgpTableRefs, ref.ID)
	}
	if ref != nil && m.gobgpTableErrs != nil {
		if err, ok := m.gobgpTableErrs[ref.ID]; ok {
			return 0, err
		}
	}
	if ref != nil && m.gobgpTables != nil {
		if value, ok := m.gobgpTables[ref.ID]; ok {
			return value, nil
		}
	}
	return 0, nil
}

func (m *mockClient) ListPathValidation(ref *gobgpFamilyRef) (gobgpValidationSummary, error) {
	m.gobgpValidationCalls++
	if ref != nil {
		m.gobgpValidationRefs = append(m.gobgpValidationRefs, ref.ID)
	}
	if ref != nil && m.gobgpValidationErrs != nil {
		if err, ok := m.gobgpValidationErrs[ref.ID]; ok {
			return gobgpValidationSummary{}, err
		}
	}
	if ref != nil && m.gobgpValidation != nil {
		if value, ok := m.gobgpValidation[ref.ID]; ok {
			return value, nil
		}
	}
	return gobgpValidationSummary{}, nil
}

func summaryResponseKey(afi, safi string) string {
	if safi == "" || safi == "unicast" {
		return afi
	}
	return afi + "/" + safi
}

func familyMetricSet(id string, established, adminDown, down, configured, charted, received, sent, prefixes, rib int64) map[string]int64 {
	prefix := "family_" + id + "_"
	return map[string]int64{
		prefix + "peers_established": established,
		prefix + "peers_admin_down":  adminDown,
		prefix + "peers_down":        down,
		prefix + "peers_configured":  configured,
		prefix + "peers_charted":     charted,
		prefix + "messages_received": received,
		prefix + "messages_sent":     sent,
		prefix + "prefixes_received": prefixes,
		prefix + "rib_routes":        rib,
	}
}

func peerMetricSet(id string, received, sent, prefixes, uptime, state int64) map[string]int64 {
	prefix := "peer_" + id + "_"
	return map[string]int64{
		prefix + "messages_received": received,
		prefix + "messages_sent":     sent,
		prefix + "prefixes_received": prefixes,
		prefix + "uptime_seconds":    uptime,
		prefix + "state":             state,
	}
}

func collectorMetricSet(status string, query, parse, deep, attempted, failed, skipped int64) map[string]int64 {
	return map[string]int64{
		"collector_status_ok":               boolToInt(status == collectorStatusOK),
		"collector_status_permission_error": boolToInt(status == collectorStatusPermissionError),
		"collector_status_timeout":          boolToInt(status == collectorStatusTimeout),
		"collector_status_query_error":      boolToInt(status == collectorStatusQueryError),
		"collector_status_parse_error":      boolToInt(status == collectorStatusParseError),
		"collector_failures_query":          query,
		"collector_failures_parse":          parse,
		"collector_failures_deep_query":     deep,
		"collector_deep_queries_attempted":  attempted,
		"collector_deep_queries_failed":     failed,
		"collector_deep_queries_skipped":    skipped,
	}
}

func assertAndStripCollectorMetrics(t *testing.T, mx map[string]int64, status string, query, parse, deep, attempted, failed, skipped int64) map[string]int64 {
	t.Helper()

	duration, ok := mx["collector_scrape_duration_ms"]
	require.True(t, ok)
	assert.GreaterOrEqual(t, duration, int64(0))
	delete(mx, "collector_scrape_duration_ms")

	for key, value := range collectorMetricSet(status, query, parse, deep, attempted, failed, skipped) {
		assert.Equalf(t, value, mx[key], "metric %s", key)
		delete(mx, key)
	}

	return mx
}

func chartLabelValue(chart *Chart, key string) string {
	for _, label := range chart.Labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
}

type frrReplayServer struct {
	socketPath         string
	listener           net.Listener
	responses          map[string][]byte
	closeAfterResponse bool

	mu          sync.Mutex
	seen        []string
	connections int

	errCh chan error
}

func newFRRReplayServer(t *testing.T, responses map[string][]byte) *frrReplayServer {
	return newFRRReplayServerWithOptions(t, responses, false)
}

func newFRRReplayServerClosing(t *testing.T, responses map[string][]byte) *frrReplayServer {
	return newFRRReplayServerWithOptions(t, responses, true)
}

func newFRRReplayServerWithOptions(t *testing.T, responses map[string][]byte, closeAfterResponse bool) *frrReplayServer {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "bgpd.vty")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	srv := &frrReplayServer{
		socketPath:         socketPath,
		listener:           ln,
		responses:          responses,
		closeAfterResponse: closeAfterResponse,
		errCh:              make(chan error, 1),
	}

	go srv.serve()

	t.Cleanup(func() {
		_ = ln.Close()
		srv.assertNoError(t)
	})

	return srv
}

func (s *frrReplayServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.recordErr(err)
			return
		}
		s.recordConnection()
		go s.handleConn(conn)
	}
}

func (s *frrReplayServer) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		cmd, err := readNullTerminated(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			s.recordErr(err)
			return
		}

		s.recordCommand(cmd)

		resp, ok := s.response(cmd)
		if !ok {
			s.recordErr(fmt.Errorf("unexpected command %q", cmd))
			return
		}

		if _, err := conn.Write(append(resp, 0)); err != nil {
			s.recordErr(err)
			return
		}
		if s.closeAfterResponse && cmd != "enable" {
			return
		}
	}
}

func (s *frrReplayServer) response(cmd string) ([]byte, bool) {
	if cmd == "enable" {
		return []byte("ok"), true
	}

	resp, ok := s.responses[cmd]
	return resp, ok
}

func (s *frrReplayServer) recordCommand(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, cmd)
}

func (s *frrReplayServer) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

func (s *frrReplayServer) recordConnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections++
}

func (s *frrReplayServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

func (s *frrReplayServer) recordErr(err error) {
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *frrReplayServer) assertNoError(t *testing.T) {
	t.Helper()
	select {
	case err := <-s.errCh:
		require.NoError(t, err)
	default:
	}
}

func readNullTerminated(conn net.Conn) (string, error) {
	var data []byte
	buf := make([]byte, 1)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		if buf[0] == 0 {
			return string(data), nil
		}
		data = append(data, buf[0])
	}
}

func filterReplayCommands(commands []string) []string {
	var filtered []string
	for _, cmd := range commands {
		if cmd != "enable" {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}
