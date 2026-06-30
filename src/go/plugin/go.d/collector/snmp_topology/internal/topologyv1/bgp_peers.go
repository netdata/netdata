// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func buildSNMPTopologyV1BGPPeersTable(
	rows []topologyV1DynamicRow,
	actorIndex map[string]int,
	stringsDict *topologyapi.StringDictionary,
) topologyapi.Table {
	actorRefs := make([]any, len(rows))
	remoteActors := make([]any, len(rows))
	routingInstances := make([]any, len(rows))
	neighborIPs := make([]any, len(rows))
	remoteASes := make([]any, len(rows))
	states := make([]any, len(rows))
	adminStatuses := make([]any, len(rows))
	localIPs := make([]any, len(rows))
	localASes := make([]any, len(rows))
	localIdentifiers := make([]any, len(rows))
	peerIdentifiers := make([]any, len(rows))
	peerTypes := make([]any, len(rows))
	bgpVersions := make([]any, len(rows))
	descriptions := make([]any, len(rows))
	establishedUptimes := make([]any, len(rows))
	lastReceivedUpdateAges := make([]any, len(rows))
	sources := make([]any, len(rows))

	for i, row := range rows {
		actorRefs[i] = row.actorRef
		remoteActors[i] = nullableActorRef(actorIndex, row.values["remote_actor_id"])
		routingInstances[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(topologyV1ScalarLabelValue(row.values["routing_instance"]), "default"))
		neighborIPs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeBGPPeerAddress(topologyV1ScalarLabelValue(row.values["neighbor_ip"])))
		remoteASes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["remote_as"]))
		states[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["state"]))
		adminStatuses[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["admin_status"]))
		localIPs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeBGPPeerAddress(topologyV1ScalarLabelValue(row.values["local_ip"])))
		localASes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["local_as"]))
		localIdentifiers[i] = nullableStringRef(stringsDict, topologyutil.NormalizeBGPRouterID(topologyV1ScalarLabelValue(row.values["local_identifier"])))
		peerIdentifiers[i] = nullableStringRef(stringsDict, topologyutil.NormalizeBGPRouterID(topologyV1ScalarLabelValue(row.values["peer_identifier"])))
		peerTypes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["peer_type"]))
		bgpVersions[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["bgp_version"]))
		descriptions[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["description"]))
		establishedUptimes[i] = nullableUintValue(row.values["established_uptime"])
		lastReceivedUpdateAges[i] = nullableUintValue(row.values["last_received_update_age"])
		sources[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(topologyV1ScalarLabelValue(row.values["source"]), "bgp_mib"))
	}

	return topologyapi.MustTable(len(rows), snmpTopologyV1BGPPeersColumns(), []topologyapi.ColumnEncoding{
		topologyapi.Values(actorRefs...),
		topologyapi.Values(remoteActors...),
		topologyapi.Values(routingInstances...),
		topologyapi.Values(neighborIPs...),
		topologyapi.Values(remoteASes...),
		topologyapi.Values(states...),
		topologyapi.Values(adminStatuses...),
		topologyapi.Values(localIPs...),
		topologyapi.Values(localASes...),
		topologyapi.Values(localIdentifiers...),
		topologyapi.Values(peerIdentifiers...),
		topologyapi.Values(peerTypes...),
		topologyapi.Values(bgpVersions...),
		topologyapi.Values(descriptions...),
		topologyapi.Values(establishedUptimes...),
		topologyapi.Values(lastReceivedUpdateAges...),
		topologyapi.Values(sources...),
	})
}

func snmpTopologyV1BGPPeerValues(row topologymodel.BGPPeerDetailRow) map[string]any {
	out := map[string]any{
		"remote_actor_id":  row.RemoteActorID,
		"routing_instance": row.RoutingInstance,
		"neighbor_ip":      row.NeighborIP,
		"remote_as":        row.RemoteAS,
		"state":            row.State,
		"admin_status":     row.AdminStatus,
		"local_ip":         row.LocalIP,
		"local_as":         row.LocalAS,
		"local_identifier": row.LocalIdentifier,
		"peer_identifier":  row.PeerIdentifier,
		"peer_type":        row.PeerType,
		"bgp_version":      row.BGPVersion,
		"description":      row.Description,
		"source":           topologyutil.FirstNonEmptyString(row.Source, "bgp_mib"),
	}
	if row.EstablishedUptime != nil {
		out["established_uptime"] = *row.EstablishedUptime
	}
	if row.LastReceivedUpdateAge != nil {
		out["last_received_update_age"] = *row.LastReceivedUpdateAge
	}
	return pruneNilAttributes(out)
}
