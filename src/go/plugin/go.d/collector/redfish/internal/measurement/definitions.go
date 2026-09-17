// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

// Definition is the publication contract of a metric. Empty States denotes a
// gauge; otherwise it lists the allowed enum states. Decoder catalogs stay private.
type Definition struct {
	Name   string
	States []string
}

// Definitions returns fresh metric definitions for instrument registration.
func Definitions() []Definition {
	var result []Definition
	seen := make(map[string]bool)
	gauge := func(name string) {
		if !seen[name] {
			seen[name] = true
			result = append(result, Definition{
				Name: name,
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
	for _, field := range scalarFields {
		gauge(field.Metric)
	}
	for _, reading := range readingDescriptors {
		gauge(reading.Metric)
		if reading.AlarmMetric != "" {
			states(reading.AlarmMetric, alarmStates)
		}
	}
	for kind, status := range sourceStatusByKind {
		states(kind+"_acquisition_state", acquisitionStates)
		if status.Status {
			states(kind+"_health", healthStates)
			states(kind+"_health_rollup", healthStates)
			states(kind+"_state", resourceStates)
			for _, state := range healthStates {
				gauge(kind + "_conditions_" + state)
			}
		}
		if status.PowerState {
			states(kind+"_power_state", powerStates)
		}
		if status.FailurePredicted {
			states(kind+"_failure_predicted", failureStates)
		}
	}
	for _, source := range additionalStateSources {
		states(source.Metric, source.States)
	}
	for _, set := range sourceFlagSets {
		for _, member := range set.Members {
			gauge(set.Metric + "_" + member.Role)
		}
	}
	return result
}
