// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func recordTopologyFocusAllDevicesStats(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	data.Stats["depth"] = topologyDepthAll
	data.Stats["actors_focus_depth_filtered"] = 0
	data.Stats["links_focus_depth_filtered"] = 0
	recomputeTopologyLinkStats(data)
}

func recordTopologyFocusStats(data *topologyData, options topologyQueryOptions, beforeActors, beforeLinks int) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	if options.Depth == topologyDepthAllInternal {
		data.Stats["depth"] = topologyDepthAll
	} else {
		data.Stats["depth"] = options.Depth
	}
	data.Stats["actors_focus_depth_filtered"] = beforeActors - len(data.Actors)
	data.Stats["links_focus_depth_filtered"] = beforeLinks - len(data.Links)
	recomputeTopologyLinkStats(data)
}
