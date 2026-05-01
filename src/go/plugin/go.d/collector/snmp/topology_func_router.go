// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func addTopologyFunctionHandler(handlers map[string]funcapi.MethodHandler) {
	if ddsnmp.TopologyHandler == nil || ddsnmp.TopologyMethodConfig == nil {
		return
	}
	handlers[ddsnmp.TopologyMethodConfig.ID] = ddsnmp.TopologyHandler
}

func appendTopologyMethodConfig(methods []funcapi.MethodConfig) []funcapi.MethodConfig {
	if ddsnmp.TopologyMethodConfig == nil {
		return methods
	}
	return append(methods, *ddsnmp.TopologyMethodConfig)
}
