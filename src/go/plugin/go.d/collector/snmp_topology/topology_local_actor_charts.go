// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func enrichLocalActorChartReferences(actor *topologymodel.Actor, interfaceCharts map[string]topologymodel.InterfaceChartRef) {
	if actor == nil || len(interfaceCharts) == 0 {
		return
	}

	lookup := topologyInterfaceChartLookup(interfaceCharts)
	if len(lookup) == 0 {
		return
	}

	enrichTopologyPortDetailsWithChartRefs(actor.Detail.L2.Device.Ports, lookup)
}

func topologyInterfaceChartLookup(interfaceCharts map[string]topologymodel.InterfaceChartRef) map[string]topologymodel.InterfaceChartRef {
	lookup := make(map[string]topologymodel.InterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = topologyutil.DeduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	return lookup
}

func enrichTopologyPortDetailsWithChartRefs(ports []topologyengine.ProjectionPortDetail, lookup map[string]topologymodel.InterfaceChartRef) {
	if len(lookup) == 0 || len(ports) == 0 {
		return
	}

	for i := range ports {
		name := strings.ToLower(strings.TrimSpace(topologyutil.FirstNonEmptyString(
			ports[i].IfName,
			ports[i].Name,
			ports[i].PortID,
		)))
		if name == "" {
			continue
		}
		ref, ok := lookup[name]
		if !ok {
			continue
		}
		ports[i].ChartIDSuffix = ref.ChartIDSuffix
		if len(ref.AvailableMetrics) > 0 {
			ports[i].AvailableMetrics = ref.AvailableMetrics
		}
	}
}
