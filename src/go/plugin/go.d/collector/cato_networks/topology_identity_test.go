// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"encoding/json"
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catoPeerIdentityFixture(t *testing.T, account, remoteIP string) *topologyv1.Data {
	t.Helper()
	data := mustBuildTopology(t, account, map[string]*siteState{
		"site-a": {ID: "site-a", Name: "Site A", PopName: "PoP A", BGPPeers: []bgpPeerState{
			{RemoteIP: remoteIP, RemoteASN: "64512", BGPSession: "established", RoutesCount: 3},
		}},
		"site-b": {ID: "site-b", Name: "Site B", PopName: "PoP B", BGPPeers: []bgpPeerState{
			{RemoteIP: remoteIP, RemoteASN: "64512", BGPSession: "idle", RoutesCount: 7},
		}},
	}, []string{"site-a", "site-b"}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	return data
}

func catoDeclaredPeerKey(t *testing.T, data *topologyv1.Data, row map[string]any) string {
	t.Helper()
	columns := data.Types.ActorTypes[row["type"].(string)].MergeIdentity
	if len(columns) == 0 {
		return ""
	}
	values := make([]any, 0, len(columns))
	for _, column := range columns {
		value, exists := row[column]
		require.True(t, exists, "declared merge column %q must be present", column)
		values = append(values, value)
	}
	encoded, err := json.Marshal(values)
	require.NoError(t, err)
	return string(encoded)
}

func TestCatoPeerAccountIdentityAndIndependentSessionReports(t *testing.T) {
	data := catoPeerIdentityFixture(t, "account-a", "192.0.2.10")
	other := catoPeerIdentityFixture(t, "account-b", "192.0.2.10")
	definition := data.Types.ActorTypes[catofunc.ActorTypeBGPPeer]
	assert.Equal(t, []string{"account_id", "remote_ip", "remote_asn"}, definition.MergeIdentity)
	assert.Empty(t, definition.ParentIdentity, "a shared remote peer cannot live on one site")
	var peers []map[string]any
	for _, row := range topologyTableRows(t, data.Actors, data.Dictionaries) {
		if row["type"] == catofunc.ActorTypeBGPPeer {
			peers = append(peers, row)
		}
	}
	require.Len(t, peers, 2, "direct per-node view retains the existing source rows")
	assert.Equal(t, catoDeclaredPeerKey(t, data, peers[0]), catoDeclaredPeerKey(t, data, peers[1]))
	otherPeer := requireTopologyRow(t, topologyTableRows(t, other.Actors, other.Dictionaries), "type", catofunc.ActorTypeBGPPeer)
	assert.NotEqual(t, catoDeclaredPeerKey(t, data, peers[0]), catoDeclaredPeerKey(t, other, otherPeer))
	for index, site := range []string{"site-a", "site-b"} {
		assert.Equal(t, "account-a", peers[index]["account_id"])
		assert.Equal(t, catoBGPPeerActorID(site, "192.0.2.10", "64512"), peers[index]["id"])
		assert.Equal(t, site, peers[index]["site_id"])
		assert.Equal(t, "192.0.2.10", peers[index]["display_name"])
	}
	assert.Equal(t, "PoP A", peers[0]["pop_name"])
	assert.Equal(t, "PoP B", peers[1]["pop_name"])
	assert.Equal(t, "established", peers[0]["connectivity_status"])
	assert.Equal(t, "idle", peers[1]["connectivity_status"])
	links := topologyTableRows(t, data.Links, data.Dictionaries)
	var sessions []map[string]any
	for _, link := range links {
		if link["type"] == catofunc.LinkTypeBGP {
			sessions = append(sessions, link)
		}
	}
	require.Len(t, sessions, 2)
	for index, state := range []string{"established", "idle"} {
		assert.Equal(t, state, sessions[index]["state"])
		assert.Equal(t, peers[index]["_row"], sessions[index]["dst_actor"])
	}
	assert.EqualValues(t, 3, sessions[0]["routes"])
	assert.EqualValues(t, 7, sessions[1]["routes"])
	for _, notification := range data.Notifications {
		assert.NotEqual(t, "cato.bgp_peer_identity_unqualified", notification.Code)
	}
	for _, column := range data.Actors.Columns {
		if column.ID == "id" || column.ID == "site_id" || column.ID == "pop_name" || column.ID == "connectivity_status" {
			assert.Empty(t, column.Aggregation, "legacy fields need coherent CTS report selection, not blanket reducers")
		}
	}
}

func TestCatoUnqualifiedPeerIsSourceLocalWithoutChangingPresentation(t *testing.T) {
	for _, test := range []struct{ account, remoteIP, label string }{
		{"account-a", "", "64512"}, {"", "192.0.2.10", "192.0.2.10"},
	} {
		data := catoPeerIdentityFixture(t, test.account, test.remoteIP)
		known := data.Types.ActorTypes[catofunc.ActorTypeBGPPeer]
		fallback := data.Types.ActorTypes[catofunc.ActorTypeBGPPeerObservation]
		assert.Equal(t, known.Identity, fallback.Identity)
		assert.Empty(t, fallback.MergeIdentity)
		assert.Empty(t, fallback.ParentIdentity)
		assert.Equal(t, known.Presentation, fallback.Presentation)
		assert.Equal(t, known.Search, fallback.Search)
		assert.Equal(t, known.AggregationScopes, fallback.AggregationScopes)
		var peers []map[string]any
		for _, row := range topologyTableRows(t, data.Actors, data.Dictionaries) {
			if row["type"] == catofunc.ActorTypeBGPPeerObservation {
				peers = append(peers, row)
				assert.Empty(t, catoDeclaredPeerKey(t, data, row), "equal ASNs do not declare router identity")
				assert.Equal(t, test.label, row["display_name"])
				assert.Equal(t, test.remoteIP, row["remote_ip"])
				assert.Equal(t, "64512", row["remote_asn"])
			}
		}
		require.Len(t, peers, 2)
		assert.NotEqual(t, peers[0]["id"], peers[1]["id"], "preserve distinct local site observations")
		var identityWarnings []topologyv1.Notification
		var hasScopeNotification bool
		for _, notification := range data.Notifications {
			if notification.Code == "cato.bgp_peer_identity_unqualified" {
				identityWarnings = append(identityWarnings, notification)
			}
			hasScopeNotification = hasScopeNotification || notification.Code == "cato_access_scope"
		}
		assert.True(t, hasScopeNotification, "preserve existing producer notifications")
		require.Len(t, identityWarnings, 1, "one identity warning, not one per peer")
		assert.Equal(t, topologyv1.NotificationSeverityWarning, identityWarnings[0].Severity)
		assert.Contains(t, identityWarnings[0].Message, "may represent multiple peers")
		assert.Equal(t, catofunc.TopologyPresentation().Legend, data.Presentation.Legend)
		var peerLegends int
		for _, legend := range data.Presentation.Legend.Actors {
			if legend.Label == "BGP peer" {
				peerLegends++
			}
		}
		assert.Equal(t, 1, peerLegends)
	}
}
