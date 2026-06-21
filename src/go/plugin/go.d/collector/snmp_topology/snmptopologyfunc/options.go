// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func resolveQueryOptions(params funcapi.ResolvedParams) topologyoptions.QueryOptions {
	options := topologyoptions.QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyoptions.ManagedFocusAllDevices,
		Depth:                  topologyoptions.DepthAllInternal,
	}

	if identity := normalizeNodesIdentity(params.GetOne(ParamNodesIdentity)); identity == NodesIdentityMAC {
		options.CollapseActorsByIP = false
		options.EliminateNonIPInferred = false
	}
	if mapType := topologyoptions.NormalizeMapType(params.GetOne(ParamMapType)); mapType != "" {
		options.MapType = mapType
	}
	if strategy := topologyoptions.NormalizeInferenceStrategy(params.GetOne(ParamInferenceStrategy)); strategy != "" {
		options.InferenceStrategy = strategy
	}
	if focuses := topologyoptions.NormalizeManagedFocuses(params.Get(ParamManagedDeviceFocus)); len(focuses) > 0 {
		options.ManagedDeviceFocus = topologyoptions.FormatManagedFocuses(focuses)
	}
	options.Depth = topologyoptions.ParseDepth(params.GetOne(ParamDepth))

	return options
}

func normalizeNodesIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", NodesIdentityIP:
		return NodesIdentityIP
	case NodesIdentityMAC:
		return NodesIdentityMAC
	default:
		return ""
	}
}
