// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/funcapi"

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Aliases:      []string{topologyMethodID},
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP Layer-2 topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: "topology",
		AgentWide:    true,
		RequiredParams: []funcapi.ParamConfig{
			topologyNodesIdentityParamConfig(),
			topologyMapTypeParamConfig(),
			topologyInferenceStrategyParamConfig(),
			topologyManagedFocusParamConfig(nil),
			topologyDepthParamConfig(),
		},
	}
}
