// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func recordTopologyFocusAllDevicesStats(data *topologymodel.Data, options topologyoptions.QueryOptions) {
	if data == nil {
		return
	}
	data.Stats.Focus.ManagedSNMPDeviceFocus = options.ManagedDeviceFocus
	data.Stats.Focus.Depth = topologymodel.FocusDepth{All: true}
	data.Stats.Focus.ActorsDepthFiltered = 0
	data.Stats.Focus.LinksDepthFiltered = 0
	data.Stats.HasFocus = true
	topologymodel.RecomputeLinkStats(data)
}

func recordTopologyFocusStats(data *topologymodel.Data, options topologyoptions.QueryOptions, beforeActors, beforeLinks int) {
	if data == nil {
		return
	}
	data.Stats.Focus.ManagedSNMPDeviceFocus = options.ManagedDeviceFocus
	if options.Depth == topologyoptions.DepthAllInternal {
		data.Stats.Focus.Depth = topologymodel.FocusDepth{All: true}
	} else {
		data.Stats.Focus.Depth = topologymodel.FocusDepth{Value: options.Depth}
	}
	data.Stats.Focus.ActorsDepthFiltered = beforeActors - len(data.Actors)
	data.Stats.Focus.LinksDepthFiltered = beforeLinks - len(data.Links)
	data.Stats.HasFocus = true
	topologymodel.RecomputeLinkStats(data)
}
