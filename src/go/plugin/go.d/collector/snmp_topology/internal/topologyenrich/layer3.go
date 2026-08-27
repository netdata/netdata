// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

// ApplyLayer3 applies every logical layer-3 enrichment with one post-policy actor resolver.
func ApplyLayer3(
	data *topologymodel.Data,
	aggregate topologymodel.ObservationAggregate,
	limiter worklimit.Limiter,
) error {
	var work *enrichmentWork
	if limiter != nil {
		work = &enrichmentWork{limiter: limiter}
	}
	resolver := newTopologyL3ActorResolverProviderWithWork(work, data, aggregate.Snapshots)
	applyL3SubnetWithResolver(work, data, aggregate, resolver)
	if err := work.failure(); err != nil {
		return err
	}
	applyOSPFAdjacencyWithResolver(work, data, aggregate, resolver)
	if err := work.failure(); err != nil {
		return err
	}
	// BGP extends IP identity after L3 and OSPF have consumed the shared base resolver.
	applyBGPAdjacencyWithResolver(work, data, aggregate, resolver)
	return work.failure()
}
