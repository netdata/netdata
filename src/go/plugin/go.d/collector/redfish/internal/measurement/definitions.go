// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

// Definition is the publication contract of a metric. Empty States denotes a
// gauge; otherwise it lists the allowed enum states. Decoder catalogs stay private.
type Definition struct {
	Name   string
	States []string
	Float  bool
}

// Definitions returns fresh metric definitions for instrument registration.
func Definitions() []Definition {
	var result []Definition
	seen := make(map[string]bool)
	gauge := func(name string, float bool) {
		if !seen[name] {
			seen[name] = true
			result = append(result, Definition{
				Name:  name,
				Float: float,
			})
		}
	}
	states := func(name string, values []string) {
		if !seen[name] {
			seen[name] = true
			result = append(result, Definition{
				Name:   name,
				States: append([]string(nil), values...),
			})
		}
	}
	states("derived_health_status", []string{"ok", "warning", "critical"})
	for _, field := range scalarFields {
		gauge(field.Metric, field.Float || field.Algorithm != algorithmAbsolute)
	}
	for _, reading := range readingDescriptors {
		gauge(reading.Metric, true)
		if reading.AlarmMetric != "" {
			states(reading.AlarmMetric, alarmStates)
		}
	}
	for kind, status := range sourceStatusByKind {
		states(kind+"_acquisition_state", acquisitionStates)
		if status.Status {
			states(kind+"_health_status", healthStates)
			states(kind+"_health_rollup_status", healthStates)
			states(kind+"_state", resourceStates)
			for _, state := range healthStates {
				gauge(kind+"_conditions_"+state, false)
			}
		}
		if status.PowerState {
			states(kind+"_power_state", powerStates)
		}
		if status.FailurePredicted {
			states(kind+"_failure_predicted_state", failureStates)
		}
	}
	for _, source := range additionalStateSources {
		states(source.Metric, source.States)
	}
	for _, set := range sourceFlagSets {
		for _, member := range set.Members {
			gauge(set.Metric+"_"+member.Role, false)
		}
	}
	return result
}
