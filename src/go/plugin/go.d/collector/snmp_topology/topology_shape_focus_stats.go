// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

func recordTopologyFocusAllDevicesStats(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	data.Stats.Focus.ManagedSNMPDeviceFocus = options.ManagedDeviceFocus
	data.Stats.Focus.Depth = topologyFocusDepth{All: true}
	data.Stats.Focus.ActorsDepthFiltered = 0
	data.Stats.Focus.LinksDepthFiltered = 0
	data.Stats.HasFocus = true
	topologymodel.RecomputeLinkStats(data)
}

func recordTopologyFocusStats(data *topologyData, options topologyQueryOptions, beforeActors, beforeLinks int) {
	if data == nil {
		return
	}
	data.Stats.Focus.ManagedSNMPDeviceFocus = options.ManagedDeviceFocus
	if options.Depth == topologyDepthAllInternal {
		data.Stats.Focus.Depth = topologyFocusDepth{All: true}
	} else {
		data.Stats.Focus.Depth = topologyFocusDepth{Value: options.Depth}
	}
	data.Stats.Focus.ActorsDepthFiltered = beforeActors - len(data.Actors)
	data.Stats.Focus.LinksDepthFiltered = beforeLinks - len(data.Links)
	data.Stats.HasFocus = true
	topologymodel.RecomputeLinkStats(data)
}
