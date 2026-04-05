// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

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
