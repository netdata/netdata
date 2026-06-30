// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

type topologyMetricHandler func(*topologyCache, map[string]string)

var topologyMetricHandlers = make(map[ddsnmp.TopologyKind]topologyMetricHandler)

func registerTopologyMetricHandler(kind ddsnmp.TopologyKind, handler topologyMetricHandler) {
	if kind == "" {
		panic("empty topology metric kind")
	}
	if handler == nil {
		panic("nil topology metric handler")
	}
	if _, ok := topologyMetricHandlers[kind]; ok {
		panic("duplicate topology metric handler for kind " + string(kind))
	}
	topologyMetricHandlers[kind] = handler
}

func (c *topologyCache) ingestMetric(kind ddsnmp.TopologyKind, tags map[string]string) {
	if handler := topologyMetricHandlers[kind]; handler != nil {
		handler(c, tags)
	}
}
