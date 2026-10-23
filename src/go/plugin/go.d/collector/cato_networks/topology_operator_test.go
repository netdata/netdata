// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperatorSiteWANOverview(t *testing.T) {
	site := &siteState{
		ID:         "site",
		Name:       "Office",
		Devices:    []deviceState{{ID: "device", Name: "Primary", HaRole: "primary"}},
		Interfaces: map[string]*interfaceState{"wan": {ID: "wan", Name: "WAN", DeviceID: "device", PopName: "PoP", Connected: new(false)}},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	rows := topologyTableRows(t, data.Tables.Actor[catofunc.ActorTableInterfaces].Table, data.Dictionaries)
	require.Equal(t, 0, rows[0]["site_actor"])
	require.NotEqual(t, rows[0]["actor"], rows[0]["site_actor"])
	require.Nil(t, rows[0]["upstream_bandwidth"], "unknown capacity is not zero")
	modal := data.Types.ActorTypes[catofunc.ActorTypeSite].Presentation.Modal
	require.Equal(t, 1, modal.MiniTopology.Depth)
	require.Equal(t, "site_actor", modal.Sections[0].OwnerFilter.ActorColumn)
	links := topologyTableRows(t, data.Links, data.Dictionaries)
	tunnel := requireTopologyRow(t, links, "interface_id", "wan")
	require.Equal(t, "cato_tunnel_disconnected", tunnel["type"])
	require.Equal(t, 0, tunnel["site_actor"])
}

func TestOperatorSummaryIsNotObservedTunnel(t *testing.T) {
	site := &siteState{
		ID:      "site",
		PopName: "Site PoP",
		Devices: []deviceState{{ID: "device", Connected: new(true), LastPopName: "Old PoP"}},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
	rows := topologyTableRows(t, data.Links, data.Dictionaries)
	row := requireTopologyRow(t, rows, "protocol", "cato")
	require.Equal(t, "cato_pop_association", row["type"])
	require.Equal(t, "unknown", row["state"], "device connected does not prove this tunnel")
	require.Equal(t, "Device last reported PoP", row["observation"])
}

func TestOperatorCollectionCoverage(t *testing.T) {
	c := New()
	c.AccountID = "account"
	c.APIKey = "secret"
	c.now = fixedCatoTestNow
	fake := newFixtureAPIClient()
	fake.bgpErrSites = map[string]error{"1001": errors.New("private error detail")}
	fake.metricsErrSites = map[string]error{"1001": errors.New("private error detail")}
	c.client = fake
	initCollector(t, c)
	collectOnce(t, c)
	data, ok := c.topology.CurrentTopology()
	require.True(t, ok)
	validateCatoTopologyV1Data(t, data)
	labels := topologyTableRows(t, data.Tables.Actor["actor_labels"].Table, data.Dictionaries)
	found := map[string]string{}
	for _, r := range labels {
		if r["actor"] == 0 {
			found[r["key"].(string)] = r["value"].(string)
		}
	}
	require.Equal(t, "Unavailable", found["bgp_collection"])
	require.Equal(t, "Unavailable", found["metrics_collection"])
	require.NotEmpty(t, data.Notifications)
	for _, n := range data.Notifications {
		require.NotContains(t, n.Message, "private error detail")
	}
}

func TestOperatorCapacityAndDiagnostics(t *testing.T) {
	iface := normalizeSnapshotInterface(&accountSnapshotInterface{
		Info: &accountSnapshotInterfaceInfo{
			UpstreamBandwidth:   new(1.5),
			DownstreamBandwidth: new(0.0),
		},
	})
	iface.ID = "wan"
	iface.Name = "WAN"
	iface.PopName = "PoP"
	iface.Metrics = trafficMetrics{
		present:               trafficMetricLostDownstreamPercent | trafficMetricJitterUpstreamMS | trafficMetricLastMileLatencyMS,
		LostDownstreamPercent: 2.5,
		JitterUpstreamMS:      0,
		LastMileLatencyMS:     8,
	}
	site := &siteState{
		ID:         "site",
		Interfaces: map[string]*interfaceState{"wan": &iface},
		BGPPeers:   []bgpPeerState{{RemoteIP: "192.0.2.2", RemoteASN: "64512", LocalIP: "192.0.2.1", LocalASN: "64513", IncomingState: "established", OutgoingState: "idle"}},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	row := topologyTableRows(t, data.Tables.Actor[catofunc.ActorTableInterfaces].Table, data.Dictionaries)[0]
	require.Equal(t, 1.5, row["upstream_bandwidth"])
	require.Equal(t, 0.0, row["downstream_bandwidth"])
	require.Equal(t, 2.5, row["lost_downstream_percent"])
	require.Equal(t, 0.0, row["jitter_upstream_ms"])
	require.Nil(t, row["jitter_downstream_ms"])
	require.Equal(t, 8.0, row["last_mile_latency_ms"])
	require.Equal(t, "Unknown", row["tunnel_state"])
	bgp := requireTopologyRow(t, topologyTableRows(t, data.Links, data.Dictionaries), "protocol", "bgp")
	require.Equal(t, "192.0.2.1", bgp["local_ip"])
	require.Equal(t, "64513", bgp["local_asn"])
}

func TestOperatorBGPCoverageRecovery(t *testing.T) {
	c := New()
	c.AccountID = "account"
	c.APIKey = "secret"
	c.now = fixedCatoTestNow
	fake := newFixtureAPIClient()
	c.client = fake
	initCollector(t, c)
	sites := map[string]*siteState{"1001": {ID: "1001"}}
	fake.bgpErrSites = map[string]error{"1001": errors.New("failure")}
	require.Error(t, c.collectBGP(t.Context(), sites, []string{"1001"}))
	require.Equal(t, "unavailable", sites["1001"].BGPCollection)
	fake.bgpErrSites = nil
	require.NoError(t, c.collectBGP(t.Context(), sites, []string{"1001"}))
	require.Equal(t, "observed", sites["1001"].BGPCollection)
	fake.bgp["1001"] = nil
	require.NoError(t, c.collectBGP(t.Context(), sites, []string{"1001"}))
	require.Equal(t, "empty", sites["1001"].BGPCollection)
	require.NoError(t, c.collectBGP(t.Context(), sites, []string{"1001"}))
	require.Equal(t, "empty_cached", sites["1001"].BGPCollection)
}
