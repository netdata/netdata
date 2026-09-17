// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

func normalizeReading(node *Resource, raw rawReading) normalizedReading {
	identitySource := raw.IdentitySource
	if identitySource == "" {
		identitySource = raw.Path
	}
	reading := normalizedReading{
		IdentitySource:     identitySource,
		SourcePath:         raw.Path,
		SourceType:         raw.Type,
		SourceUnits:        raw.Units,
		SourceBasis:        raw.Basis,
		Role:               raw.Role,
		Primary:            raw.Primary,
		PhysicalContext:    raw.PhysicalContext,
		PhysicalSubcontext: raw.PhysicalSubcontext,
		ImplementationType: raw.ImplementationType,
	}
	if reading.Role == "" {
		reading.Role = "input"
	}
	reading.Key = identity.Key("netdata:redfish:reading:v1", node.Key+"\x00"+identitySource, 32)
	spec, fixed := readingType(raw.Type, raw.Units, raw.FixedFamily)
	if !fixed {
		return reading
	}
	basis, ok := readingBasis(raw.Basis)
	if !ok {
		return reading
	}
	reading.Family = spec.Family
	reading.Units = spec.Units
	reading.Basis = basis
	reading.DataSourceURI = raw.DataSourceURI
	reading.SensorResetTime = readingTimestamp(raw.SensorResetTime)
	reading.LifetimeStartDateTime = readingTimestamp(raw.LifetimeStartDateTime)
	reading.SemanticSourceClass = readingSemanticClass(node, reading)
	surface, ok := matchReading(
		reading.Family,
		reading.Basis,
		reading.Role,
		reading.SemanticSourceClass,
	)
	if !ok {
		return reading
	}
	reading.Metric = surface.Metric
	reading.AlarmMetric = surface.AlarmMetric

	reading.Primary = reading.Primary && surface.Primary
	if raw.ReadingScoped {
		reading.Health = raw.Health
		switch {
		case strings.TrimSpace(raw.Health) == "":
			reading.SourceAlarmDiagnostic = fmt.Sprintf(
				"Redfish reading source alarm is missing for %s %s",
				node.URI,
				raw.Path,
			)
		case healthAlarm(raw.Health) == "":
			reading.SourceAlarmDiagnostic = fmt.Sprintf(
				"Redfish reading source alarm is unrecognized for %s %s: %q",
				node.URI,
				raw.Path,
				raw.Health,
			)
		default:
			reading.SourceAlarm = healthAlarm(raw.Health)
		}
	}
	exact, sourceValue, ok := numericValue(raw.Value)
	if !ok {
		return reading
	}
	if reading.Family == "energy" {
		reading.SourceExact = exact
	}
	reading.SourceScale = rationalMultiplier(spec.Scale)
	reading.Value = scaleReadingValue(sourceValue, spec.Scale)
	reading.Valid = isFinite(reading.Value)
	if raw.RangeMin != nil {
		if _, value, ok := numericValue(raw.RangeMin); ok {
			normalized := scaleReadingValue(value, spec.Scale)
			if isFinite(normalized) {
				reading.RangeMin = &normalized
			}
		}
	}
	if raw.RangeMax != nil {
		if _, value, ok := numericValue(raw.RangeMax); ok {
			normalized := scaleReadingValue(value, spec.Scale)
			if isFinite(normalized) {
				reading.RangeMax = &normalized
			}
		}
	}
	if reading.Valid &&
		((reading.RangeMin != nil && reading.Value < *reading.RangeMin) ||
			(reading.RangeMax != nil && reading.Value > *reading.RangeMax)) {
		reading.Valid = false
	}
	return reading
}

func readingType(sourceType, units, fixedFamily string) (readingTypeDescriptor, bool) {
	if fixedFamily != "" {
		if spec, ok := matchFixedReadingFamily(fixedFamily); ok {
			return spec, true
		}
	}
	return matchReadingType(sourceType, units)
}

func readingBasis(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "zero":
		return "zero", true
	case "delta":
		return "delta", true
	case "headroom":
		return "headroom", true
	default:
		return "", false
	}
}

func readingSemanticClass(node *Resource, reading normalizedReading) string {
	switch {
	case reading.Family == "rotational_speed" &&
		(node.Kind == "fan" || strings.EqualFold(strings.TrimSpace(reading.PhysicalContext), "Fan")):
		return "fan"
	case reading.Family == "barometric_pressure":
		return "ambient_pressure"
	default:
		return "direct"
	}
}

func rationalMultiplier(value valueScale) float64 {
	if value.Den == 0 {
		return 1
	}
	return float64(value.Num) / float64(value.Den)
}

func scaleReadingValue(value float64, scale valueScale) float64 {
	return value * rationalMultiplier(scale)
}

func (c *Projector) readingObservations(node *Resource, reading normalizedReading) []Observation {
	labels := c.metricLabels(node, &reading)
	result := make([]Observation, 0, 2)
	if reading.Valid {
		result = append(result, Observation{
			Metric: reading.Metric,
			Value:  reading.Value,
			Labels: labels,
		})
	}
	if reading.SourceAlarm != "" && reading.AlarmMetric != "" {
		result = append(result, stateObservation(reading.AlarmMetric, reading.SourceAlarm, labels))
	}
	return result
}

func healthAlarm(value string) string {
	switch normalizeHealth(value) {
	case "ok":
		return "clear"
	case "warning":
		return "warning"
	case "critical":
		return "critical"
	default:
		return ""
	}
}

func normalizedTimestamp(value any) (int64, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return 0, false
	}
	return parsed.UnixMilli(), true
}

func (r normalizedReading) String() string {
	return fmt.Sprintf("%s/%s/%s", r.Family, r.Basis, r.Role)
}

func readingTimestamp(source rawReadingTimestamp) *int64 {
	if value, ok := normalizedTimestamp(source.Value); ok {
		return &value
	}
	return nil
}
