// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"strings"
)

func sensorExcerptReadings(source SensorExcerpt) []rawReading {
	value, present := valueAt(source.Data, "Reading")
	physical, _ := stringValueAt(source.Data, "PhysicalContext")
	subcontext, _ := stringValueAt(source.Data, "PhysicalSubContext")
	implementation, _ := stringValueAt(source.Data, "Implementation")
	health, _ := stringValueAt(source.Data, "Status.Health")
	var result []rawReading
	if present || health != "" {
		result = append(result, rawReading{
			Path:                  source.Path + ".Reading",
			Type:                  source.Type,
			Units:                 source.Units,
			Basis:                 "Zero",
			Role:                  "input",
			Value:                 value,
			ValuePresent:          present,
			Primary:               true,
			PhysicalContext:       physical,
			PhysicalSubcontext:    subcontext,
			ImplementationType:    implementation,
			RangeMin:              source.Data["ReadingRangeMin"],
			RangeMax:              source.Data["ReadingRangeMax"],
			Health:                health,
			ReadingScoped:         true,
			DataSourceURI:         readingSourceURI(source.Data, "DataSourceUri"),
			SensorResetTime:       readingSourceTimestamp(source.Data, "SensorResetTime"),
			LifetimeStartDateTime: readingSourceTimestamp(source.Data, "LifetimeStartDateTime"),
		})
	}
	result = append(
		result,
		electricalAuxiliaryReadings(source.Data, source.Path, false, physical, subcontext, implementation)...)
	return result
}

func sensorRawReadings(data map[string]any) []rawReading {
	sourceType, _ := stringValueAt(data, "ReadingType")
	units, _ := stringValueAt(data, "ReadingUnits")
	basis, _ := stringValueAt(data, "ReadingBasis")
	if basis == "" {
		basis = "Zero"
	}
	health, _ := stringValueAt(data, "Status.Health")
	physical, _ := stringValueAt(data, "PhysicalContext")
	subcontext, _ := stringValueAt(data, "PhysicalSubContext")
	implementation, _ := stringValueAt(data, "Implementation")
	rangeMin, _ := valueAt(data, "ReadingRangeMin")
	rangeMax, _ := valueAt(data, "ReadingRangeMax")
	reading, present := valueAt(data, "Reading")
	var result []rawReading
	if present || health != "" {
		result = append(result, rawReading{
			Path:                  "Sensor.Reading",
			Type:                  sourceType,
			Units:                 units,
			Basis:                 basis,
			Role:                  "input",
			Value:                 reading,
			ValuePresent:          present,
			Primary:               true,
			PhysicalContext:       physical,
			PhysicalSubcontext:    subcontext,
			ImplementationType:    implementation,
			RangeMin:              rangeMin,
			RangeMax:              rangeMax,
			Health:                health,
			ReadingScoped:         true,
			DataSourceURI:         readingSourceURI(data, "DataSourceUri"),
			SensorResetTime:       readingSourceTimestamp(data, "SensorResetTime"),
			LifetimeStartDateTime: readingSourceTimestamp(data, "LifetimeStartDateTime"),
		})
	}
	auxiliaries := []struct {
		Path string
		Role string
	}{
		{"AverageReading", "average"},
		{"LowestIntervalReading", "lowest_interval"},
		{"PeakIntervalReading", "peak_interval"},
		{"LowestReading", "lowest_since_reset"},
		{"PeakReading", "peak_since_reset"},
	}
	for _, auxiliary := range auxiliaries {
		value, ok := valueAt(data, auxiliary.Path)
		if !ok {
			continue
		}
		result = append(result, rawReading{
			Path:                  "Sensor." + auxiliary.Path,
			Type:                  sourceType,
			Units:                 units,
			Basis:                 basis,
			Role:                  auxiliary.Role,
			Value:                 value,
			ValuePresent:          true,
			PhysicalContext:       physical,
			PhysicalSubcontext:    subcontext,
			ImplementationType:    implementation,
			RangeMin:              rangeMin,
			RangeMax:              rangeMax,
			DataSourceURI:         readingSourceURI(data, "DataSourceUri"),
			SensorResetTime:       readingSourceTimestamp(data, "SensorResetTime"),
			LifetimeStartDateTime: readingSourceTimestamp(data, "LifetimeStartDateTime"),
		})
	}
	result = append(result, electricalAuxiliaryReadings(data, "Sensor", true, physical, subcontext, implementation)...)
	return result
}

func electricalAuxiliaryReadings(
	data map[string]any,
	path string,
	standalone bool,
	physical, subcontext, implementation string,
) []rawReading {
	specs := []struct {
		Path           string
		Type           string
		Units          string
		Family         string
		Role           string
		StandaloneOnly bool
	}{
		{"SpeedRPM", "Rotational", "{rev}/min", "rotational_speed", "speed_rpm", false},
		{"ApparentVA", "ApparentPower", "V.A", "apparent_power", "apparent_va", false},
		{"ReactiveVAR", "ReactivePower", "var", "reactive_power", "reactive_var", false},
		{"ApparentkVAh", "ApparentEnergy", "kV.A.h", "apparent_energy", "apparent_kvah", false},
		{"ReactivekVARh", "ReactiveEnergy", "kvar.h", "reactive_energy", "reactive_kvarh", false},
		{"CrestFactor", "CrestFactor", "1", "crest_factor", "crest_factor", true},
		{"PhaseAngleDegrees", "PhaseAngle", "deg", "phase_angle", "phase_angle_degrees", false},
		{"PowerFactor", "PowerFactor", "1", "power_factor", "power_factor", false},
		{"THDPercent", "HarmonicDistortion", "%", "harmonic_distortion", "thd_percent", true},
		{"LoadPercent", "Percent", "%", "percentage", "load_percent", true},
	}
	var result []rawReading
	dataSourceURI := readingSourceURI(data, "DataSourceUri")
	for _, spec := range specs {
		if spec.StandaloneOnly && !standalone {
			continue
		}
		value, ok := valueAt(data, spec.Path)
		if !ok {
			continue
		}
		result = append(result, rawReading{
			Path:               path + "." + spec.Path,
			Type:               spec.Type,
			Units:              spec.Units,
			Basis:              "Zero",
			Role:               spec.Role,
			Value:              value,
			ValuePresent:       true,
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			FixedFamily:        spec.Family,
			DataSourceURI:      dataSourceURI,
		})
	}
	return result
}

func legacyThermalReadings(node *Resource) []rawReading {
	health, _ := stringValueAt(node.Data, "Status.Health")
	switch node.SourcePath {
	case "Temperatures":
		value, ok := firstValue(node.Data, "ReadingCelsius", "Reading")
		if !ok && health == "" {
			return nil
		}
		return []rawReading{legacyReading(node, value, ok, "Temperature", "Cel", "input", health)}
	case "Fans":
		value, ok := firstValue(node.Data, "Reading", "ReadingRPM")
		if !ok && health == "" {
			return nil
		}
		units, _ := stringValueAt(node.Data, "ReadingUnits")
		sourceType := "Rotational"
		if strings.EqualFold(units, "Percent") || units == "%" {
			sourceType, units = "Percent", "%"
		} else if units == "" || strings.EqualFold(units, "RPM") {
			units = "{rev}/min"
		}
		return []rawReading{legacyReading(node, value, ok, sourceType, units, "input", health)}
	default:
		return nil
	}
}

func legacyPowerReadings(node *Resource) []rawReading {
	health, _ := stringValueAt(node.Data, "Status.Health")
	switch node.SourcePath {
	case "PowerControl":
		// Requested and available watts describe budgets, not consumption.
		value, ok := valueAt(node.Data, "PowerConsumedWatts")
		if !ok && health == "" {
			return nil
		}
		return []rawReading{legacyReading(node, value, ok, "Power", "W", "input", health)}
	case "Voltages":
		value, ok := firstValue(node.Data, "ReadingVolts", "Reading")
		if !ok && health == "" {
			return nil
		}
		return []rawReading{legacyReading(node, value, ok, "Voltage", "V", "input", health)}
	default:
		return nil
	}
}

func legacyReading(
	node *Resource,
	value any,
	present bool,
	sourceType, units, role, health string,
) rawReading {
	physical, _ := stringValueAt(node.Data, "PhysicalContext")
	return rawReading{
		Path:                  node.SourceModel + "." + node.SourcePath,
		Type:                  sourceType,
		Units:                 units,
		Basis:                 "Zero",
		Role:                  role,
		Value:                 value,
		ValuePresent:          present,
		Primary:               true,
		PhysicalContext:       physical,
		Health:                health,
		ReadingScoped:         true,
		DataSourceURI:         readingSourceURI(node.Data, "DataSourceUri"),
		SensorResetTime:       readingSourceTimestamp(node.Data, "SensorResetTime"),
		LifetimeStartDateTime: readingSourceTimestamp(node.Data, "LifetimeStartDateTime"),
	}
}

func valueAt(data map[string]any, path string) (any, bool) {
	return Properties(data).Lookup(path)
}

// firstValue keeps the existing non-null fallback order, while remembering an
// explicitly null reading when none of the alternate properties supplies a value.
func firstValue(data map[string]any, paths ...string) (any, bool) {
	present := false
	for _, path := range paths {
		if value, ok := valueAt(data, path); ok {
			present = true
			if value != nil {
				return value, true
			}
		}
	}
	return nil, present
}

type rawReadingTimestamp struct {
	Value   string
	Present bool
}

func readingSourceURI(data map[string]any, key string) *string {
	value, ok := data[key].(string)
	if !ok {
		return nil
	}
	return &value
}

func readingSourceTimestamp(data map[string]any, key string) rawReadingTimestamp {
	raw, present := data[key]
	value, _ := raw.(string)
	return rawReadingTimestamp{
		Value:   value,
		Present: present,
	}
}
