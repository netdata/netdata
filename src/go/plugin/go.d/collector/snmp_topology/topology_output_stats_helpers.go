// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func normalizedMatchIPs(match topologyMatch) []string {
	if len(match.IPAddresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(match.IPAddresses))
	seen := make(map[string]struct{}, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		ip := normalizeIPAddress(value)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func topologyActorIsInferred(actor topologyActor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
		return true
	}
	if boolStatValue(actor.Attributes["inferred"]) {
		return true
	}
	if boolStatValue(actor.Labels["inferred"]) {
		return true
	}
	return false
}

func boolStatValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func intStatValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
		}
	}
	return 0
}

func topologyMetricValueString(metrics map[string]any, key string) string {
	if metrics == nil {
		return ""
	}
	value, ok := metrics[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
