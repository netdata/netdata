// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func Methods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		methodConfig(),
	}
}

func methodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           MethodID,
		FunctionName: FunctionName,
		Aliases:      []string{MethodID},
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP Layer-2 topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: topologyv1.ResponseType,
		Scope:        funcapi.MethodScopeAgent,
		RequiredParams: []funcapi.ParamConfig{
			nodesIdentityParamConfig(),
			mapTypeParamConfig(),
			inferenceStrategyParamConfig(),
			managedFocusParamConfig(nil),
			depthParamConfig(),
		},
	}
}
