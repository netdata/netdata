// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

// ApplyLayer3 applies every logical layer-3 enrichment with one post-policy actor resolver.
func ApplyLayer3(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) {
	resolver := newTopologyL3ActorResolverProvider(data, aggregate.Snapshots)
	applyL3SubnetWithResolver(data, aggregate, resolver)
	applyOSPFAdjacencyWithResolver(data, aggregate, resolver)
	// BGP extends IP identity after L3 and OSPF have consumed the shared base resolver.
	applyBGPAdjacencyWithResolver(data, aggregate, resolver)
}
