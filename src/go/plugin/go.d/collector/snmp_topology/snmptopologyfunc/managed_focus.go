// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func managedFocusParamOptions(deps Deps) []funcapi.ParamOption {
	if deps == nil {
		return nil
	}

	options := make([]funcapi.ParamOption, 0)
	for _, target := range deps.ManagedDeviceFocusTargets() {
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

func normalizeManagedFocuses(values []string) []string {
	expanded := splitManagedFocusValues(values)
	if len(expanded) == 0 {
		return []string{ManagedFocusAllDevices}
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, raw := range expanded {
		normalized := normalizeManagedFocusValue(raw)
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

func splitManagedFocusValues(values []string) []string {
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

func normalizeManagedFocusValue(v string) string {
	value := strings.TrimSpace(v)
	if strings.EqualFold(value, ManagedFocusAllDevices) {
		return ManagedFocusAllDevices
	}
	if len(value) <= len(ManagedFocusIPPrefix) ||
		!strings.EqualFold(value[:len(ManagedFocusIPPrefix)], ManagedFocusIPPrefix) {
		return ""
	}
	ip := strings.TrimSpace(value[len(ManagedFocusIPPrefix):])
	if ip == "" {
		return ""
	}
	return ManagedFocusIPPrefix + ip
}

func formatManagedFocuses(values []string) string {
	normalized := normalizeManagedFocuses(values)
	if len(normalized) == 0 {
		return ManagedFocusAllDevices
	}
	return strings.Join(normalized, ",")
}
