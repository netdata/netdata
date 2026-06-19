// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

func buildSNMPTopologyV1BGPPeersTable(
	rows []topologyV1DynamicRow,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
) topologyv1.Table {
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
		routingInstances[i] = nullableStringRef(stringsDict, firstNonEmptyString(topologyV1ScalarLabelValue(row.values["routing_instance"]), "default"))
		neighborIPs[i] = nullableStringRef(stringsDict, topologyBGPPeerAddressValue(topologyV1ScalarLabelValue(row.values["neighbor_ip"])))
		remoteASes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["remote_as"]))
		states[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["state"]))
		adminStatuses[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["admin_status"]))
		localIPs[i] = nullableStringRef(stringsDict, topologyBGPPeerAddressValue(topologyV1ScalarLabelValue(row.values["local_ip"])))
		localASes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["local_as"]))
		localIdentifiers[i] = nullableStringRef(stringsDict, normalizeBGPRouterID(topologyV1ScalarLabelValue(row.values["local_identifier"])))
		peerIdentifiers[i] = nullableStringRef(stringsDict, normalizeBGPRouterID(topologyV1ScalarLabelValue(row.values["peer_identifier"])))
		peerTypes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["peer_type"]))
		bgpVersions[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["bgp_version"]))
		descriptions[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["description"]))
		establishedUptimes[i] = nullableUintValue(row.values["established_uptime"])
		lastReceivedUpdateAges[i] = nullableUintValue(row.values["last_received_update_age"])
		sources[i] = nullableStringRef(stringsDict, firstNonEmptyString(topologyV1ScalarLabelValue(row.values["source"]), "bgp_mib"))
	}

	return topologyv1.MustTable(len(rows), snmpTopologyV1BGPPeersColumns(), []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(remoteActors...),
		topologyv1.Values(routingInstances...),
		topologyv1.Values(neighborIPs...),
		topologyv1.Values(remoteASes...),
		topologyv1.Values(states...),
		topologyv1.Values(adminStatuses...),
		topologyv1.Values(localIPs...),
		topologyv1.Values(localASes...),
		topologyv1.Values(localIdentifiers...),
		topologyv1.Values(peerIdentifiers...),
		topologyv1.Values(peerTypes...),
		topologyv1.Values(bgpVersions...),
		topologyv1.Values(descriptions...),
		topologyv1.Values(establishedUptimes...),
		topologyv1.Values(lastReceivedUpdateAges...),
		topologyv1.Values(sources...),
	})
}
