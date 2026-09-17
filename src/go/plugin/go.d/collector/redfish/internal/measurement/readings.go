// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"cmp"
	"strings"
	"time"
)

type normalizedReading struct {
	Key                   string
	IdentitySource        string
	SourcePath            string
	SourceType            string
	SourceUnits           string
	SourceBasis           string
	SourceExact           string
	SourceScale           float64
	Family                string
	Units                 string
	Basis                 string
	Role                  string
	Value                 float64
	Valid                 bool
	Primary               bool
	Metric                string
	AlarmMetric           string
	SemanticSourceClass   string
	PhysicalContext       string
	PhysicalSubcontext    string
	ImplementationType    string
	RangeMin              *float64
	RangeMax              *float64
	SourceAlarm           string
	Health                string
	SourceAlarmDiagnostic string
	DataSourceURI         *string
	SensorResetTime       *int64
	LifetimeStartDateTime *int64
}

type rawReading struct {
	SourceDocumentURI     string // Final document URI for relative DataSourceUri resolution.
	Path                  string
	IdentitySource        string
	Type                  string
	Units                 string
	Basis                 string
	Role                  string
	Value                 any
	ValuePresent          bool
	Primary               bool
	PhysicalContext       string
	PhysicalSubcontext    string
	ImplementationType    string
	RangeMin              any
	RangeMax              any
	Health                string
	ReadingScoped         bool
	FixedFamily           string
	DataSourceURI         *string
	SensorResetTime       rawReadingTimestamp
	LifetimeStartDateTime rawReadingTimestamp
}

func (c *Projector) readingsForNode(node *Resource, observedAt time.Time) []normalizedReading {
	raw := c.rawReadingsForNode(node)
	result := make([]normalizedReading, 0, len(raw)+1)
	for _, source := range raw {
		reading := normalizeReading(node, source)
		result = append(result, reading)
		if reading.Valid && reading.Primary && reading.Family == "energy" && reading.Basis == "zero" &&
			reading.SourceExact != "" {
			if derived, ok := c.derivedPowerReading(node, reading, observedAt); ok {
				result = append(result, derived)
			}
		}
	}
	return result
}

func (c *Projector) rawReadingsForNode(node *Resource) []rawReading {
	var result []rawReading
	if node.Kind == "sensor" &&
		node.SourceModel != "deprecated_thermal" &&
		node.SourceModel != "deprecated_power" &&
		node.SourceModel != "embedded_sensor_excerpt" {
		result = append(result, sensorRawReadings(node.Data)...)
	}
	switch node.SourceModel {
	case "deprecated_thermal":
		result = append(result, legacyThermalReadings(node)...)
	case "deprecated_power":
		result = append(result, legacyPowerReadings(node)...)
	}
	for _, source := range node.SensorExcerpts {
		result = mergeProvenSensorReadings(result, sensorExcerptReadings(source))
	}
	result = append(result, excerptReadings(node)...)
	result = deduplicateRawReadings(result)
	for index := range result {
		source := &result[index]
		source.IdentitySource = source.Path
		rawURI := ""
		if source.DataSourceURI != nil {
			rawURI = *source.DataSourceURI
		}
		baseURI := cmp.Or(strings.TrimSpace(source.SourceDocumentURI), strings.TrimSpace(node.URI))
		if canonical, ok := c.canonicalProvenance(baseURI, rawURI); ok {
			source.IdentitySource = canonical + "\x00" + source.Role
			source.DataSourceURI = &canonical
		}
	}
	return result
}

func mergeProvenSensorReadings(current, excerpts []rawReading) []rawReading {
	for _, excerpt := range excerpts {
		match := -1
		excerptType, excerptOK := readingType(excerpt.Type, excerpt.Units, excerpt.FixedFamily)
		for index := range current {
			candidate := &current[index]
			candidateType, candidateOK := readingType(candidate.Type, candidate.Units, candidate.FixedFamily)
			if excerptOK && candidateOK &&
				excerptType.Family == candidateType.Family &&
				excerpt.Role == candidate.Role &&
				strings.EqualFold(
					cmp.Or(strings.TrimSpace(excerpt.Basis), "Zero"),
					cmp.Or(strings.TrimSpace(candidate.Basis), "Zero"),
				) {
				match = index
				break
			}
		}
		if match < 0 {
			current = append(current, excerpt)
			continue
		}
		target := &current[match]
		if !target.ValuePresent && excerpt.ValuePresent {
			// Use the excerpt's numeric provenance while preserving explicit Sensor health.
			health := target.Health
			*target = excerpt
			if health != "" {
				target.Health = health
			}
		}
		if target.RangeMin == nil {
			target.RangeMin = excerpt.RangeMin
		}
		if target.RangeMax == nil {
			target.RangeMax = excerpt.RangeMax
		}
		if target.Health == "" {
			target.Health = excerpt.Health
		}
		if target.FixedFamily == "" {
			target.FixedFamily = excerpt.FixedFamily
		}
		if !target.SensorResetTime.Present {
			target.SensorResetTime = excerpt.SensorResetTime
		}
		if !target.LifetimeStartDateTime.Present {
			target.LifetimeStartDateTime = excerpt.LifetimeStartDateTime
		}
	}
	return current
}

func deduplicateRawReadings(values []rawReading) []rawReading {
	seen := make(map[string]struct{}, len(values))
	result := make([]rawReading, 0, len(values))
	for _, value := range values {
		key := value.Path + "\x00" + value.Role
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (c *Projector) canonicalProvenance(baseURI, raw string) (string, bool) {
	if c.resolveProvenance == nil {
		return "", false
	}
	return c.resolveProvenance(baseURI, raw)
}
