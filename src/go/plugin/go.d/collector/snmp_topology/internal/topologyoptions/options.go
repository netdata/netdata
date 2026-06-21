// SPDX-License-Identifier: GPL-3.0-or-later

package topologyoptions

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
)

const (
	MapTypeLLDPCDPManaged                = snmptopologyfunc.MapTypeLLDPCDPManaged
	MapTypeHighConfidenceInferred        = snmptopologyfunc.MapTypeHighConfidenceInferred
	MapTypeAllDevicesLowConfidence       = snmptopologyfunc.MapTypeAllDevicesLowConfidence
	InferenceStrategyFDBMinimumKnowledge = snmptopologyfunc.InferenceStrategyFDBMinimumKnowledge
	InferenceStrategySTPParentTree       = snmptopologyfunc.InferenceStrategySTPParentTree
	InferenceStrategyFDBPairwise         = snmptopologyfunc.InferenceStrategyFDBPairwise
	InferenceStrategySTPFDBCorrelated    = snmptopologyfunc.InferenceStrategySTPFDBCorrelated
	InferenceStrategyCDPFDBHybrid        = snmptopologyfunc.InferenceStrategyCDPFDBHybrid
	ManagedFocusAllDevices               = snmptopologyfunc.ManagedFocusAllDevices
	ManagedFocusIPPrefix                 = snmptopologyfunc.ManagedFocusIPPrefix
	DepthAll                             = snmptopologyfunc.DepthAll
	DepthMin                             = snmptopologyfunc.DepthMin
	DepthMax                             = snmptopologyfunc.DepthMax
	DepthAllInternal                     = snmptopologyfunc.DepthAllInternal
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
	if options.Depth != DepthAllInternal {
		if options.Depth < DepthMin {
			options.Depth = DepthMin
		} else if options.Depth > DepthMax {
			options.Depth = DepthMax
		}
	}
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
