// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func buildSNMPTopologyV1OSPFNeighborsTable(
	rows []topologyV1DynamicRow,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
) topologyv1.Table {
	actorRefs := make([]any, len(rows))
	remoteActors := make([]any, len(rows))
	localRouterIDs := make([]any, len(rows))
	neighborRouterIDs := make([]any, len(rows))
	neighborIPs := make([]any, len(rows))
	states := make([]any, len(rows))
	localIPs := make([]any, len(rows))
	subnets := make([]any, len(rows))
	addresslessIndexes := make([]any, len(rows))
	sources := make([]any, len(rows))

	for i, row := range rows {
		actorRefs[i] = row.actorRef
		remoteActors[i] = nullableActorRef(actorIndex, row.values["remote_actor_id"])
		localRouterIDs[i] = nullableStringRef(stringsDict, normalizeTopologyRouterID(topologyV1ScalarLabelValue(row.values["local_router_id"])))
		neighborRouterIDs[i] = nullableStringRef(stringsDict, normalizeTopologyRouterID(topologyV1ScalarLabelValue(row.values["neighbor_router_id"])))
		neighborIPs[i] = nullableStringRef(stringsDict, normalizeNonUnspecifiedIPAddress(topologyV1ScalarLabelValue(row.values["neighbor_ip"])))
		states[i] = nullableStringRef(stringsDict, normalizeOSPFNeighborState(topologyV1ScalarLabelValue(row.values["state"])))
		localIPs[i] = nullableStringRef(stringsDict, normalizeNonUnspecifiedIPAddress(topologyV1ScalarLabelValue(row.values["local_ip"])))
		subnets[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["subnet"]))
		addresslessIndexes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["addressless_index"]))
		sources[i] = nullableStringRef(stringsDict, firstNonEmptyString(topologyV1ScalarLabelValue(row.values["source"]), "ospf_mib"))
	}

	return topologyv1.MustTable(len(rows), snmpTopologyV1OSPFNeighborsColumns(), []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(remoteActors...),
		topologyv1.Values(localRouterIDs...),
		topologyv1.Values(neighborRouterIDs...),
		topologyv1.Values(neighborIPs...),
		topologyv1.Values(states...),
		topologyv1.Values(localIPs...),
		topologyv1.Values(subnets...),
		topologyv1.Values(addresslessIndexes...),
		topologyv1.Values(sources...),
	})
}

func nullableActorRef(actorIndex map[string]int, value any) any {
	actorID := strings.TrimSpace(topologyV1ScalarLabelValue(value))
	if actorID == "" {
		return nil
	}
	ref, ok := actorIndex[actorID]
	if !ok {
		return nil
	}
	return ref
}

func snmpTopologyV1OSPFNeighborValues(row topologyOSPFNeighborDetailRow) map[string]any {
	return pruneNilAttributes(map[string]any{
		"remote_actor_id":    row.RemoteActorID,
		"local_router_id":    row.LocalRouterID,
		"neighbor_router_id": row.NeighborRouterID,
		"neighbor_ip":        row.NeighborIP,
		"state":              row.State,
		"local_ip":           row.LocalIP,
		"subnet":             row.Subnet,
		"addressless_index":  row.AddresslessIndex,
		"source":             firstNonEmptyString(row.Source, "ospf_mib"),
	})
}
