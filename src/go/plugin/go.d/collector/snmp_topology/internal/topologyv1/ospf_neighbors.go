// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"strings"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func buildSNMPTopologyV1OSPFNeighborsTable(
	rows []topologyV1DynamicRow,
	actorIndex map[string]int,
	stringsDict *topologyapi.StringDictionary,
) topologyapi.Table {
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
		localRouterIDs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeTopologyRouterID(topologyV1ScalarLabelValue(row.values["local_router_id"])))
		neighborRouterIDs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeTopologyRouterID(topologyV1ScalarLabelValue(row.values["neighbor_router_id"])))
		neighborIPs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeNonUnspecifiedIPAddress(topologyV1ScalarLabelValue(row.values["neighbor_ip"])))
		states[i] = nullableStringRef(stringsDict, topologyutil.NormalizeOSPFNeighborState(topologyV1ScalarLabelValue(row.values["state"])))
		localIPs[i] = nullableStringRef(stringsDict, topologyutil.NormalizeNonUnspecifiedIPAddress(topologyV1ScalarLabelValue(row.values["local_ip"])))
		subnets[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["subnet"]))
		addresslessIndexes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["addressless_index"]))
		sources[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(topologyV1ScalarLabelValue(row.values["source"]), "ospf_mib"))
	}

	return topologyapi.MustTable(len(rows), snmpTopologyV1OSPFNeighborsColumns(), []topologyapi.ColumnEncoding{
		topologyapi.Values(actorRefs...),
		topologyapi.Values(remoteActors...),
		topologyapi.Values(localRouterIDs...),
		topologyapi.Values(neighborRouterIDs...),
		topologyapi.Values(neighborIPs...),
		topologyapi.Values(states...),
		topologyapi.Values(localIPs...),
		topologyapi.Values(subnets...),
		topologyapi.Values(addresslessIndexes...),
		topologyapi.Values(sources...),
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

func snmpTopologyV1OSPFNeighborValues(row topologymodel.OSPFNeighborDetailRow) map[string]any {
	return pruneNilAttributes(map[string]any{
		"remote_actor_id":    row.RemoteActorID,
		"local_router_id":    row.LocalRouterID,
		"neighbor_router_id": row.NeighborRouterID,
		"neighbor_ip":        row.NeighborIP,
		"state":              row.State,
		"local_ip":           row.LocalIP,
		"subnet":             row.Subnet,
		"addressless_index":  row.AddresslessIndex,
		"source":             topologyutil.FirstNonEmptyString(row.Source, "ospf_mib"),
	})
}
