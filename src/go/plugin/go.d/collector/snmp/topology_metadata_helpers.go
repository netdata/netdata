// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"
)

func pruneNilAttributes(attrs map[string]any) map[string]any {
	for k, v := range attrs {
		switch vv := v.(type) {
		case string:
			if vv == "" {
				delete(attrs, k)
			}
		case []string:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case []topologyManagementAddress:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case nil:
			delete(attrs, k)
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
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneTopologyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyCanonicalMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	key = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
	for strings.Contains(key, "__") {
		key = strings.ReplaceAll(key, "__", "_")
	}
	return strings.Trim(key, "_")
}

func topologyMetadataValue(labels map[string]string, aliases []string) string {
	if len(labels) == 0 || len(aliases) == 0 {
		return ""
	}
	byKey := make(map[string]string, len(labels))
	for key, value := range labels {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		canonical := topologyCanonicalMetadataKey(key)
		if canonical == "" {
			continue
		}
		if _, exists := byKey[canonical]; !exists {
			byKey[canonical] = value
		}
	}
	for _, alias := range aliases {
		alias = topologyCanonicalMetadataKey(alias)
		if alias == "" {
			continue
		}
		if value := strings.TrimSpace(byKey[alias]); value != "" {
			return value
		}
	}
	return ""
}

func setTopologyMetadataLabelIfMissing(labels map[string]string, key, value string) {
	if labels == nil {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if existing := strings.TrimSpace(labels[key]); existing == "" {
		labels[key] = value
	}
}

func ensureLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return make(map[string]string)
	}
	return labels
}

func deduplicateSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
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
