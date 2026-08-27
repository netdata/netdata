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
	if limiter != nil {
		rows, err := worklimit.Sum(
			uint64(len(aggregate.L3Interfaces)), uint64(len(aggregate.OSPFNeighbors)), uint64(len(aggregate.BGPPeers)),
		)
		if err != nil {
			return err
		}
		actors := uint64(0)
		links := uint64(0)
		if data != nil {
			actors = uint64(len(data.Actors))
			links = uint64(len(data.Links))
		}
		items, err := worklimit.Sum(rows, actors, links, uint64(len(aggregate.Snapshots)))
		if err != nil {
			return err
		}
		if err := limiter.Charge(items); err != nil {
			return err
		}
		actorLookups, err := worklimit.Sum(actors, 1)
		if err != nil {
			return err
		}
		if err := limiter.ChargeProduct(rows, actorLookups); err != nil {
			return err
		}
		for _, size := range [...]uint64{
			uint64(len(aggregate.L3Interfaces)), uint64(len(aggregate.OSPFNeighbors)), uint64(len(aggregate.BGPPeers)),
			actors, items,
		} {
			if err := limiter.ChargeSort(size); err != nil {
				return err
			}
		}
	}
	resolver := newTopologyL3ActorResolverProvider(data, aggregate.Snapshots)
	applyL3SubnetWithResolver(data, aggregate, resolver)
	applyOSPFAdjacencyWithResolver(data, aggregate, resolver)
	// BGP extends IP identity after L3 and OSPF have consumed the shared base resolver.
	applyBGPAdjacencyWithResolver(data, aggregate, resolver)
	return nil
}
