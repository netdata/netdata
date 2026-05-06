// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"maps"
	"net/netip"
	"sort"
	"strings"
)

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func addressStrings(addresses []netip.Addr) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.Unmap().String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstAddress(addresses []netip.Addr) string {
	values := addressStrings(addresses)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueTopologyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedEndpointIPs(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		addr, ok := in[key]
		if !ok || !addr.IsValid() {
			continue
		}
		out = append(out, addr.Unmap().String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedTopologySet(in map[string]struct{}) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func csvToSet(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func labelsCSVToSlice(labels map[string]string, key string) []string {
	if len(labels) == 0 {
		return nil
	}
	return csvToSet(labels[key])
}

func pruneTopologyAttributes(attrs map[string]any) map[string]any {
	for key, value := range attrs {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(attrs, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]any:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case int:
			if typed == 0 {
				delete(attrs, key)
			}
		case nil:
			delete(attrs, key)
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func mapStringStringToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
