// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopology)(nil)

type funcTopology struct {
	router *funcRouter
}

func newFuncTopology(r *funcRouter) *funcTopology {
	return &funcTopology{router: r}
}

const topologyMethodID = "topology:snmp"

const (
	topologyParamNodesIdentity = "nodes_identity"
	topologyParamConnectivity  = "non_lldp_cdp_connectivity"

	topologyNodesIdentityIP  = "ip"
	topologyNodesIdentityMAC = "mac"

	topologyConnectivityStrict   = "strict"
	topologyConnectivityProbable = "probable"
)

func topologyNodesIdentityParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamNodesIdentity,
		Name:      "Nodes Identity",
		Help:      "Choose actor identity strategy: ip (collapse by IP, remove non-IP inferred) or mac",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: topologyNodesIdentityIP, Name: "IP", Default: true},
			{ID: topologyNodesIdentityMAC, Name: "MAC"},
		},
	}
}

func topologyConnectivityParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamConnectivity,
		Name:      "Non LLDP/CDP Connectivity",
		Help:      "Choose inferred non-LLDP/CDP connectivity mode: strict or probable",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: topologyConnectivityStrict, Name: "Strict"},
			{ID: topologyConnectivityProbable, Name: "Probable", Default: true},
		},
	}
}

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP Layer-2 topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: "topology",
		AgentWide:    true,
		RequiredParams: []funcapi.ParamConfig{
			topologyNodesIdentityParamConfig(),
			topologyConnectivityParamConfig(),
		},
	}
}

func (f *funcTopology) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topologyMethodID {
		return nil, nil
	}
	return []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyConnectivityParamConfig(),
	}, nil
}

func (f *funcTopology) Cleanup(_ context.Context) {}

func (f *funcTopology) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}

	options := topologyQueryOptions{
		CollapseActorsByIP:        true,
		EliminateNonIPInferred:    true,
		ProbabilisticConnectivity: true,
	}
	if identity := normalizeTopologyNodesIdentity(params.GetOne(topologyParamNodesIdentity)); identity != "" {
		if identity == topologyNodesIdentityMAC {
			options.CollapseActorsByIP = false
			options.EliminateNonIPInferred = false
		}
	}
	if mode := normalizeTopologyConnectivity(params.GetOne(topologyParamConnectivity)); mode != "" {
		options.ProbabilisticConnectivity = mode == topologyConnectivityProbable
	}

	if snmpTopologyRegistry == nil {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	data, ok := snmpTopologyRegistry.snapshotWithOptions(options)

	if !ok {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "SNMP topology and neighbor discovery data",
		ResponseType: "topology",
		Data:         data,
	}
}

func normalizeTopologyNodesIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyNodesIdentityIP:
		return topologyNodesIdentityIP
	case topologyNodesIdentityMAC:
		return topologyNodesIdentityMAC
	default:
		return ""
	}
}

func normalizeTopologyConnectivity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyConnectivityProbable:
		return topologyConnectivityProbable
	case topologyConnectivityStrict:
		return topologyConnectivityStrict
	default:
		return ""
	}
}
