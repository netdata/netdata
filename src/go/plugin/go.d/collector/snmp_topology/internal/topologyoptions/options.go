// SPDX-License-Identifier: GPL-3.0-or-later

package topologyoptions

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const (
	MapTypeLLDPCDPManaged                = "lldp_cdp_managed"
	MapTypeHighConfidenceInferred        = "high_confidence_inferred"
	MapTypeAllDevicesLowConfidence       = "all_devices_low_confidence"
	InferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	InferenceStrategySTPParentTree       = "stp_parent_tree"
	InferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	InferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	InferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
	ManagedFocusAllDevices               = "all_devices"
	ManagedFocusIPPrefix                 = "ip:"
	DepthAll                             = "all"
	DepthMin                             = 0
	DepthMax                             = 10
	DepthAllInternal                     = -1
)

type QueryOptions struct {
	CollapseActorsByIP     bool
	EliminateNonIPInferred bool
	MapType                string
	InferenceStrategy      string
	ManagedDeviceFocus     string
	Depth                  int
	ResolveDNSName         func(ip string) string
}

type ManagedFocusTarget struct {
	Value string
	Name  string
}

func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                MapTypeLLDPCDPManaged,
		InferenceStrategy:      InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     ManagedFocusAllDevices,
		Depth:                  DepthAllInternal,
	}
}

func NormalizeQueryOptions(options QueryOptions) QueryOptions {
	options.MapType = NormalizeMapType(options.MapType)
	if options.MapType == "" {
		options.MapType = MapTypeLLDPCDPManaged
	}
	options.InferenceStrategy = NormalizeInferenceStrategy(options.InferenceStrategy)
	if options.InferenceStrategy == "" {
		options.InferenceStrategy = InferenceStrategyFDBMinimumKnowledge
	}
	options.ManagedDeviceFocus = FormatManagedFocuses(ParseManagedFocuses(options.ManagedDeviceFocus))
	options.Depth = NormalizeDepth(options.Depth)
	return options
}

func NormalizeMapType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", MapTypeLLDPCDPManaged:
		return MapTypeLLDPCDPManaged
	case MapTypeHighConfidenceInferred:
		return MapTypeHighConfidenceInferred
	case MapTypeAllDevicesLowConfidence:
		return MapTypeAllDevicesLowConfidence
	default:
		return ""
	}
}

func NormalizeInferenceStrategy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", InferenceStrategyFDBMinimumKnowledge:
		return InferenceStrategyFDBMinimumKnowledge
	case InferenceStrategySTPParentTree:
		return InferenceStrategySTPParentTree
	case InferenceStrategyFDBPairwise:
		return InferenceStrategyFDBPairwise
	case InferenceStrategySTPFDBCorrelated:
		return InferenceStrategySTPFDBCorrelated
	case InferenceStrategyCDPFDBHybrid:
		return InferenceStrategyCDPFDBHybrid
	default:
		return ""
	}
}

func NormalizeDepth(depth int) int {
	if depth == DepthAllInternal {
		return DepthAllInternal
	}
	if depth < DepthMin {
		return DepthMin
	}
	if depth > DepthMax {
		return DepthMax
	}
	return depth
}

func ParseDepth(v string) int {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" || value == DepthAll {
		return DepthAllInternal
	}
	depth, err := strconv.Atoi(value)
	if err != nil {
		return DepthAllInternal
	}
	return NormalizeDepth(depth)
}

func IsMapTypeProbable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", MapTypeAllDevicesLowConfidence:
		return true
	default:
		return false
	}
}

func NormalizeManagedFocusValue(v string) string {
	value := strings.TrimSpace(v)
	switch strings.ToLower(value) {
	case ManagedFocusAllDevices:
		return ManagedFocusAllDevices
	}
	if len(value) > len(ManagedFocusIPPrefix) &&
		strings.EqualFold(value[:len(ManagedFocusIPPrefix)], ManagedFocusIPPrefix) {
		ip := topologyutil.NormalizeIPAddress(strings.TrimSpace(value[len(ManagedFocusIPPrefix):]))
		if ip == "" {
			return ""
		}
		return ManagedFocusIPPrefix + ip
	}
	return ""
}

func NormalizeManagedFocuses(values []string) []string {
	expanded := SplitManagedFocusValues(values)
	if len(expanded) == 0 {
		return []string{ManagedFocusAllDevices}
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, raw := range expanded {
		normalized := NormalizeManagedFocusValue(raw)
		if normalized == "" {
			continue
		}
		if normalized == ManagedFocusAllDevices {
			return []string{ManagedFocusAllDevices}
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return []string{ManagedFocusAllDevices}
	}
	sort.Strings(out)
	return out
}

func SplitManagedFocusValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, raw := range values {
		for token := range strings.SplitSeq(raw, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

func ParseManagedFocuses(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{ManagedFocusAllDevices}
	}
	return NormalizeManagedFocuses(strings.Split(value, ","))
}

func FormatManagedFocuses(values []string) string {
	normalized := NormalizeManagedFocuses(values)
	if len(normalized) == 0 {
		return ManagedFocusAllDevices
	}
	return strings.Join(normalized, ",")
}

func IsManagedFocusAllDevices(value string) bool {
	normalized := ParseManagedFocuses(value)
	return len(normalized) == 1 && normalized[0] == ManagedFocusAllDevices
}

func ManagedFocusSelectedIPs(value string) []string {
	normalized := ParseManagedFocuses(value)
	if len(normalized) == 1 && normalized[0] == ManagedFocusAllDevices {
		return nil
	}

	out := make([]string, 0, len(normalized))
	for _, focus := range normalized {
		if len(focus) <= len(ManagedFocusIPPrefix) {
			continue
		}
		if !strings.EqualFold(focus[:len(ManagedFocusIPPrefix)], ManagedFocusIPPrefix) {
			continue
		}
		ip := topologyutil.NormalizeIPAddress(strings.TrimSpace(focus[len(ManagedFocusIPPrefix):]))
		if ip == "" {
			continue
		}
		out = append(out, ip)
	}
	return out
}
