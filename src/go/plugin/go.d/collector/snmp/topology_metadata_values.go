// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "strings"

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
