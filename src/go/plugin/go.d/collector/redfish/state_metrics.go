// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type flagValue struct {
	Set     flagSet
	Member  flagMember
	Value   bool
	Present bool
}

func flagValues(node *graphNode) []flagValue {
	var result []flagValue
	for _, set := range sourceFlagSets {
		if string(set.Kind) != node.Kind {
			continue
		}
		document := node.Data
		if set.Document != "" {
			document = findEnrichment(node, string(set.Document))
		}
		if document == nil {
			continue
		}
		values := make([]flagValue, 0, len(set.Members))
		for _, member := range set.Members {
			raw, present := jsonPath(document, member.Path)
			value, valid := raw.(bool)
			if present && valid && member.Invert {
				value = !value
			}
			values = append(values, flagValue{
				Set:     set,
				Member:  member,
				Value:   value,
				Present: present && valid,
			})
		}
		result = append(result, values...)
	}
	return result
}

func (c *protocolClient) flagObservations(node *graphNode, values []flagValue) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	result := make([]hardwareObservation, 0, len(values))
	for _, value := range values {
		if !value.Present {
			continue
		}
		result = append(result, hardwareObservation{
			Metric: value.Set.Metric + "_" + value.Member.Role,
			Value:  boolFloat(value.Value),
			Labels: labels,
		})
	}
	return result
}

func (c *protocolClient) statusObservations(node *graphNode) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	prefix := strings.ReplaceAll(node.Kind, "-", "_")
	var result []hardwareObservation
	status := statusForKind(node.Kind)
	if node.AcquisitionState != "" {
		result = append(result, hardwareObservation{
			Metric: prefix + "_acquisition_state",
			State:  normalizedEnum(node.AcquisitionState, acquisitionStates),
			Labels: labels,
		})
	}
	if status.Status {
		if state, present, _ := categoricalStringState(node.Data, "Status.Health", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health", state, labels))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.HealthRollup", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health_rollup", state, labels))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.State", normalizeResourceState); present {
			result = append(result, stateObservation(prefix+"_state", state, labels))
		}
	}
	if status.PowerState {
		if state, present, _ := categoricalStringState(node.Data, "PowerState", func(value string) string {
			return normalizedEnum(value, powerStates)
		}); present {
			result = append(result, stateObservation(prefix+"_power_state", state, labels))
		}
	}
	if status.FailurePredicted {
		if raw, exists := jsonPath(node.Data, "FailurePredicted"); exists && raw != nil {
			state := "unknown"
			if value, ok := raw.(bool); ok {
				if value {
					state = "predicted"
				} else {
					state = "clear"
				}
			}
			result = append(result, stateObservation(prefix+"_failure_predicted", state, labels))
		}
	}
	if status.Status {
		if counts, present, readable := conditionCountsForNode(node); present && readable {
			for role, value := range map[string]int{
				"ok": counts.OK, "warning": counts.Warning, "critical": counts.Critical, "unknown": counts.Unknown,
			} {
				result = append(result, hardwareObservation{
					Metric: prefix + "_conditions_" + role,
					Value:  float64(value),
					Labels: labels,
				})
			}
		}
	}
	result = append(result, c.additionalStateObservations(node, labels)...)
	return result
}

func statusForKind(kind string) statusDescriptor { return sourceStatusByKind[kind] }

func stateObservation(
	metric, state string,
	labels []metrix.Label,
) hardwareObservation {
	return hardwareObservation{
		Metric: metric,
		State:  state,
		Labels: labels,
	}
}

func normalizeHealth(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok":
		return "ok"
	case "warning":
		return "warning"
	case "critical":
		return "critical"
	default:
		return "unknown"
	}
}

func normalizeResourceState(value string) string {
	return normalizedEnum(value, resourceStates)
}

func normalizedEnum(value string, allowed []string) string {
	value = snakeCase(value)
	if slices.Contains(allowed, value) {
		return value
	}
	return "unknown"
}

func (c *protocolClient) additionalStateObservations(
	node *graphNode,
	labels []metrix.Label,
) []hardwareObservation {
	var result []hardwareObservation
	for _, source := range additionalStateSources {
		if source.Kind != node.Kind {
			continue
		}
		document := node.Data
		if source.Document != "" {
			document = findEnrichment(node, source.Document)
		}
		raw, ok := jsonPath(document, source.Path)
		if (!ok || raw == nil) && source.FallbackPath != "" {
			raw, ok = jsonPath(document, source.FallbackPath)
		}
		if !ok || raw == nil {
			continue
		}
		var state string
		if source.BooleanFalse != "" || source.BooleanTrue != "" {
			value, ok := raw.(bool)
			if !ok {
				state = "unknown"
			} else if value {
				state = source.BooleanTrue
			} else {
				state = source.BooleanFalse
			}
		} else if value, ok := raw.(string); ok {
			if source.Metric == "redundancy_mode" {
				// Redundancy.v1 defines these spellings for legacy Mode and modern RedundancyType.
				switch strings.TrimSpace(value) {
				case "N+m", "NPlusM":
					value = "n_plus_m"
				}
			}
			state = normalizedEnum(value, source.States)
		} else {
			state = "unknown"
		}
		result = append(result, stateObservation(source.Metric, state, labels))
	}
	return result
}

func snakeCase(value string) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	for i, r := range value {
		if r == '-' || r == ' ' {
			if result.Len() > 0 {
				result.WriteByte('_')
			}
			continue
		}
		if r >= 'A' && r <= 'Z' {
			if i > 0 && result.Len() > 0 {
				previous := value[i-1]
				if previous >= 'a' && previous <= 'z' {
					result.WriteByte('_')
				}
			}
			result.WriteRune(r + ('a' - 'A'))
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func categoricalStringState(
	document map[string]any,
	path string,
	normalize func(string) string,
) (state string, present, readable bool) {
	raw, present := jsonPath(document, path)
	if !present || raw == nil {
		return "", false, false
	}
	value, ok := raw.(string)
	if !ok {
		return "unknown", true, true
	}
	return normalize(value), true, true
}

func conditionCountsForNode(node *graphNode) (conditionCounts, bool, bool) {
	if node == nil {
		return conditionCounts{}, false, false
	}
	raw, present := jsonPath(node.Data, "Status.Conditions")
	if !present || raw == nil {
		return conditionCounts{}, false, false
	}
	conditions, ok := raw.([]any)
	if !ok {
		return conditionCounts{}, true, false
	}
	for _, condition := range conditions {
		// Partial envelope decoding leaves zero-valued records for malformed members.
		// Those are unreadable conditions, not evidence of a healthy empty list.
		if _, ok := condition.(map[string]any); !ok {
			return conditionCounts{}, true, false
		}
	}
	return conditionCountsFrom(node.Doc.Status.Conditions), true, true
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
