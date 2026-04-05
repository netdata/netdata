// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func (f *funcTopology) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topologyMethodID {
		return nil, nil
	}

	return []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyMapTypeParamConfig(),
		topologyInferenceStrategyParamConfig(),
		topologyManagedFocusParamConfig(topologyManagedFocusParamOptions()),
		topologyDepthParamConfig(),
	}, nil
}

func (f *funcTopology) Cleanup(_ context.Context) {}

func (f *funcTopology) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if snmpTopologyRegistry == nil {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after topology refresh")
	}

	options := resolveTopologyQueryOptions(params)
	options.ResolveDNSName = resolveTopologyReverseDNSNameCached // never block on network I/O
	data, ok := snmpTopologyRegistry.snapshotWithOptions(options)
	if !ok {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after topology refresh")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "SNMP topology and neighbor discovery data",
		ResponseType: "topology",
		Data:         data,
	}
}

func topologyManagedFocusParamOptions() []funcapi.ParamOption {
	if snmpTopologyRegistry == nil {
		return nil
	}

	options := make([]funcapi.ParamOption, 0)
	for _, target := range snmpTopologyRegistry.managedDeviceFocusTargets() {
		if strings.TrimSpace(target.Value) == "" {
			continue
		}
		options = append(options, funcapi.ParamOption{
			ID:   target.Value,
			Name: target.Name,
		})
	}
	return options
}

func resolveTopologyQueryOptions(params funcapi.ResolvedParams) topologyQueryOptions {
	options := topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
	}

	if identity := normalizeTopologyNodesIdentity(params.GetOne(topologyParamNodesIdentity)); identity == topologyNodesIdentityMAC {
		options.CollapseActorsByIP = false
		options.EliminateNonIPInferred = false
	}
	if mapType := normalizeTopologyMapType(params.GetOne(topologyParamMapType)); mapType != "" {
		options.MapType = mapType
	}
	if strategy := normalizeTopologyInferenceStrategy(params.GetOne(topologyParamInferenceStrategy)); strategy != "" {
		options.InferenceStrategy = strategy
	}
	if focuses := normalizeTopologyManagedFocuses(params.Get(topologyParamManagedDeviceFocus)); len(focuses) > 0 {
		options.ManagedDeviceFocus = formatTopologyManagedFocuses(focuses)
	}
	options.Depth = normalizeTopologyDepth(params.GetOne(topologyParamDepth))

	return options
}
