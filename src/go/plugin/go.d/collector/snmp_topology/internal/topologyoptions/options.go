// SPDX-License-Identifier: GPL-3.0-or-later

package topologyoptions

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const (
	MapTypeManagedFabric                 = "managed_fabric"
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
	LookupVendorByMAC      func(mac string) (vendor string, prefix string)
	WorkLimiter            worklimit.Limiter
	prepared               bool
	managedFocus           preparedManagedFocus
}

type preparedManagedFocus struct {
	all bool
	ips []string
}

type ManagedFocusTarget struct {
	Value string
	Name  string
}

func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                MapTypeManagedFabric,
		InferenceStrategy:      InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     ManagedFocusAllDevices,
		Depth:                  DepthAllInternal,
	}
}

func NormalizeQueryOptions(options QueryOptions) QueryOptions {
	options, _ = PrepareQueryOptions(options)
	return options
}

// PrepareQueryOptions canonicalizes query input once. Repeated calls on a
// prepared value are no-ops so downstream stages consume the same focus truth.
func PrepareQueryOptions(options QueryOptions) (QueryOptions, error) {
	if options.prepared {
		return options, nil
	}
	options.MapType = NormalizeMapType(options.MapType)
	options.InferenceStrategy = NormalizeInferenceStrategy(options.InferenceStrategy)
	if options.InferenceStrategy == "" {
		options.InferenceStrategy = InferenceStrategyFDBMinimumKnowledge
	}
	focuses, err := normalizeManagedFocuses([]string{options.ManagedDeviceFocus}, options.WorkLimiter)
	if err != nil {
		return QueryOptions{}, err
	}
	if err := worklimit.ChargeStrings(options.WorkLimiter, focuses); err != nil {
		return QueryOptions{}, err
	}
	options.ManagedDeviceFocus = strings.Join(focuses, ",")
	options.managedFocus.all = len(focuses) == 1 && focuses[0] == ManagedFocusAllDevices
	if !options.managedFocus.all {
		if err := options.WorkLimiter.Charge(uint64(len(focuses))); err != nil {
			return QueryOptions{}, err
		}
		options.managedFocus.ips = make([]string, 0, len(focuses))
		for _, focus := range focuses {
			options.managedFocus.ips = append(options.managedFocus.ips, focus[len(ManagedFocusIPPrefix):])
		}
	}
	options.Depth = NormalizeDepth(options.Depth)
	options.prepared = true
	return options, nil
}

func (o QueryOptions) ManagedFocusIsAllDevices() bool {
	return o.managedFocus.all
}

func (o QueryOptions) ManagedFocusIPs() []string {
	return o.managedFocus.ips
}

func NormalizeMapType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", MapTypeManagedFabric:
		return MapTypeManagedFabric
	case MapTypeLLDPCDPManaged:
		return MapTypeLLDPCDPManaged
	case MapTypeHighConfidenceInferred:
		return MapTypeHighConfidenceInferred
	case MapTypeAllDevicesLowConfidence:
		return MapTypeAllDevicesLowConfidence
	default:
		return MapTypeManagedFabric
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
	return NormalizeMapType(v) == MapTypeAllDevicesLowConfidence
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
	normalized, _ := normalizeManagedFocuses(values, nil)
	return normalized
}

func normalizeManagedFocuses(values []string, limiter worklimit.Limiter) ([]string, error) {
	expanded, err := splitManagedFocusValues(values, limiter)
	if err != nil {
		return nil, err
	}
	if len(expanded) == 0 {
		return []string{ManagedFocusAllDevices}, nil
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, raw := range expanded {
		normalized := NormalizeManagedFocusValue(raw)
		if normalized == "" {
			continue
		}
		if normalized == ManagedFocusAllDevices {
			return []string{ManagedFocusAllDevices}, nil
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return []string{ManagedFocusAllDevices}, nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return nil, err
	}
	return out, nil
}

func SplitManagedFocusValues(values []string) []string {
	out, _ := splitManagedFocusValues(values, nil)
	return out
}

func splitManagedFocusValues(values []string, limiter worklimit.Limiter) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return nil, err
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
	return out, nil
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
