// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"strconv"
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
	Context               string
	AlarmMetric           string
	SemanticSourceClass   string
	PhysicalContext       string
	PhysicalSubcontext    string
	ImplementationType    string
	RangeMin              *float64
	RangeMax              *float64
	SourceAlarm           string
	SourceAlarmDiagnostic string
	DataSourceURI         *string
	SensorResetTime       *int64
	LifetimeStartDateTime *int64
}

type rawReading struct {
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

func (c *protocolClient) readingsForNode(node *graphNode, observedAt time.Time) []normalizedReading {
	raw := c.rawReadingsForNode(node)
	result := make([]normalizedReading, 0, len(raw)+1)
	for _, source := range raw {
		reading := normalizeReading(node, source)
		result = append(result, reading)
		if reading.Valid && reading.Primary && reading.Family == "energy" && reading.SourceExact != "" {
			if derived, ok := c.derivedPowerReading(node, reading, observedAt); ok {
				result = append(result, derived)
			}
		}
	}
	return result
}

func (c *protocolClient) derivedPowerReading(
	node *graphNode,
	source normalizedReading,
	observedAt time.Time,
) (normalizedReading, bool) {
	value, emit := c.rateValue(
		node.Key+"\x00reading-energy-rate\x00"+source.Key,
		source.SourceExact,
		source.SourceScale,
		observedAt,
		algorithmRate,
		readingRateEpoch(source),
	)
	if !emit {
		return normalizedReading{}, false
	}
	surface, ok := matchReading("power", "zero", "energy_rate", "energy_rate")
	if !ok {
		return normalizedReading{}, false
	}
	derived := source
	derived.IdentitySource = source.SourcePath + "\x00energy_rate"
	derived.Key = stableKey("netdata:redfish:reading:v1", node.Key+"\x00"+derived.IdentitySource, 32)
	derived.Family = "power"
	derived.Units = "watts"
	derived.Role = "energy_rate"
	derived.Value = value
	derived.SourceExact = ""
	derived.Metric = surface.Metric
	derived.Context = surface.Context
	derived.Primary = surface.Primary
	derived.AlarmMetric = ""
	derived.SemanticSourceClass = "energy_rate"
	derived.SourceAlarm = ""
	derived.SourceAlarmDiagnostic = ""
	return derived, true
}

func readingRateEpoch(source normalizedReading) string {
	parts := []string{
		source.SourcePath,
		source.SourceType,
		source.SourceUnits,
		source.SourceBasis,
		source.Family,
		source.Units,
		source.Basis,
	}
	if source.DataSourceURI != nil {
		parts = append(parts, "reading_data_source_uri", *source.DataSourceURI)
	}
	if source.SensorResetTime != nil {
		parts = append(parts, "sensor_reset_time", strconv.FormatInt(*source.SensorResetTime, 10))
	}
	if source.LifetimeStartDateTime != nil {
		parts = append(parts, "lifetime_start_datetime", strconv.FormatInt(*source.LifetimeStartDateTime, 10))
	}
	return stableTupleDigest("netdata:redfish:reading-rate-epoch:v1", parts...)
}

func (c *protocolClient) rawReadingsForNode(node *graphNode) []rawReading {
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
		if canonical, ok := c.canonicalReadingDataSourceURI(node, rawURI); ok {
			source.IdentitySource = canonical + "\x00" + source.Role
			source.DataSourceURI = &canonical
		}
	}
	return result
}

func sensorExcerptReadings(source sensorExcerptSource) []rawReading {
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
	for _, auxiliary := range []struct {
		Path, Type, Units, Family, Role string
	}{
		{"SpeedRPM", "Rotational", "{rev}/min", "rotational_speed", "speed_rpm"},
		{"ApparentVA", "ApparentPower", "V.A", "apparent_power", "apparent_va"},
		{"ReactiveVAR", "ReactivePower", "var", "reactive_power", "reactive_var"},
		{"ApparentkVAh", "ApparentEnergy", "kV.A.h", "apparent_energy", "apparent_kvah"},
		{"ReactivekVARh", "ReactiveEnergy", "kvar.h", "reactive_energy", "reactive_kvarh"},
		{"PhaseAngleDegrees", "PhaseAngle", "deg", "phase_angle", "phase_angle_degrees"},
		{"PowerFactor", "PowerFactor", "1", "power_factor", "power_factor"},
	} {
		auxValue, ok := valueAt(source.Data, auxiliary.Path)
		if !ok {
			continue
		}
		result = append(result, rawReading{
			Path:               source.Path + "." + auxiliary.Path,
			Type:               auxiliary.Type,
			Units:              auxiliary.Units,
			Basis:              "Zero",
			Role:               auxiliary.Role,
			Value:              auxValue,
			ValuePresent:       true,
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			FixedFamily:        auxiliary.Family,
		})
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
				strings.EqualFold(firstNonEmpty(excerpt.Basis, "Zero"), firstNonEmpty(candidate.Basis, "Zero")) {
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

func (c *protocolClient) canonicalReadingDataSourceURI(node *graphNode, raw string) (string, bool) {
	if raw == "" || c == nil || c.root == nil {
		return "", false
	}
	base := c.root
	if node != nil && node.URI != "" {
		if target, err := c.resolveURI(c.root, node.URI, false); err == nil {
			base = target
		}
	}
	target, err := resolveRedfishURI(c.origin, base, raw, uriProvenance)
	if err != nil {
		return "", false
	}
	return canonicalProvenanceURI(target), true
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
	result = append(result, sensorElectricalAuxiliaries(data, physical, subcontext, implementation)...)
	return result
}

func sensorElectricalAuxiliaries(data map[string]any, physical, subcontext, implementation string) []rawReading {
	specs := []struct {
		Path   string
		Type   string
		Units  string
		Family string
		Role   string
	}{
		{"SpeedRPM", "Rotational", "{rev}/min", "rotational_speed", "speed_rpm"},
		{"ApparentVA", "ApparentPower", "V.A", "apparent_power", "apparent_va"},
		{"ReactiveVAR", "ReactivePower", "var", "reactive_power", "reactive_var"},
		{"ApparentkVAh", "ApparentEnergy", "kV.A.h", "apparent_energy", "apparent_kvah"},
		{"ReactivekVARh", "ReactiveEnergy", "kvar.h", "reactive_energy", "reactive_kvarh"},
		{"CrestFactor", "CrestFactor", "1", "crest_factor", "crest_factor"},
		{"PhaseAngleDegrees", "PhaseAngle", "deg", "phase_angle", "phase_angle_degrees"},
		{"PowerFactor", "PowerFactor", "1", "power_factor", "power_factor"},
		{"THDPercent", "HarmonicDistortion", "%", "harmonic_distortion", "thd_percent"},
		{"LoadPercent", "Percent", "%", "percentage", "load_percent"},
	}
	var result []rawReading
	for _, spec := range specs {
		value, ok := valueAt(data, spec.Path)
		if !ok {
			continue
		}
		result = append(result, rawReading{
			Path:               "Sensor." + spec.Path,
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
		})
	}
	return result
}

func legacyThermalReadings(node *graphNode) []rawReading {
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

func legacyPowerReadings(node *graphNode) []rawReading {
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
	node *graphNode,
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

func excerptReadings(node *graphNode) []rawReading {
	var result []rawReading
	addObjectAt := func(document map[string]any, path, sourcePath, sourceType, units, role string) {
		raw, ok := valueAt(document, path)
		if !ok || raw == nil {
			return
		}
		object, isObject := raw.(map[string]any)
		if !isObject {
			physical, _ := stringValueAt(document, "PhysicalContext")
			reading := rawReading{
				Path:            node.Kind + "." + sourcePath,
				Type:            sourceType,
				Units:           units,
				Basis:           "Zero",
				Role:            role,
				Value:           raw,
				ValuePresent:    true,
				Primary:         true,
				PhysicalContext: physical,
			}
			if role == "stored_energy" {
				reading.FixedFamily = "stored_energy"
			}
			result = append(result, reading)
			return
		}
		readings := sensorExcerptReadings(sensorExcerptSource{
			Path:  node.Kind + "." + sourcePath,
			Type:  sourceType,
			Units: units,
			Data:  object,
		})
		for index := range readings {
			reading := &readings[index]
			if reading.Role == "input" {
				reading.Role = role
				if role == "stored_energy" {
					reading.FixedFamily = "stored_energy"
				}
			}
		}
		result = append(result, readings...)
	}

	addObject := func(document map[string]any, path, sourceType, units, role string) {
		addObjectAt(document, path, path, sourceType, units, role)
	}
	addFixedMap := func(document map[string]any, path, sourceType, units, role string, keys []string) {
		raw, ok := valueAt(document, path)
		if !ok {
			return
		}
		object, ok := raw.(map[string]any)
		if !ok {
			return
		}
		for _, key := range keys {
			if value, present := object[key]; present {
				addObjectAt(map[string]any{"item": value}, "item", path+"."+key, sourceType, units, role)
			}
		}
	}
	addObject(node.Data, "SpeedPercent", "Percent", "%", "speed")
	addObject(node.Data, "SecondarySpeedPercent", "Percent", "%", "speed")
	addObject(node.Data, "PowerWatts", "Power", "W", "power")
	addObject(node.Data, "PumpSpeedPercent", "Percent", "%", "speed")
	addObject(node.Data, "InletPressurekPa", "PressurekPa", "kPa", "pressure")
	addObject(node.Data, "FlowLitersPerMinute", "LiquidFlowLPM", "L/min", "flow")
	addObject(node.Data, "ValvePositionPercent", "Valve", "%", "position")
	addObject(node.Data, "HeatRemovedkW", "Heat", "kW", "heat_removed")
	addObject(node.Data, "HumidityPercent", "Humidity", "%", "humidity")
	addObject(node.Data, "StateOfHealthPercent", "Percent", "%", "state_of_health")
	addObject(node.Data, "DeltaTemperatureCelsius", "Temperature", "Cel", "temperature")
	addObject(node.Data, "ReturnTemperatureCelsius", "Temperature", "Cel", "temperature")
	addObject(node.Data, "SupplyTemperatureCelsius", "Temperature", "Cel", "temperature")
	addObject(node.Data, "DeltaPressurekPa", "PressurekPa", "kPa", "pressure")
	addObject(node.Data, "ReturnPressurekPa", "PressurekPa", "kPa", "pressure")
	addObject(node.Data, "SupplyPressurekPa", "PressurekPa", "kPa", "pressure")
	addObject(node.Data, "DeltaLiquidPressurekPa", "PressurekPa", "kPa", "pressure")
	addObject(node.Data, "CurrentAmps", "Current", "A", "current")
	addObject(node.Data, "Voltage", "Voltage", "V", "voltage")
	addObject(node.Data, "ReadingRPM", "Rotational", "{rev}/min", "speed")
	for kind, document := range node.Enrichment {
		switch {
		case strings.HasPrefix(kind, "processor_metrics"):
			addObject(document, "CoreVoltage", "Voltage", "V", "core_voltage")
		case strings.HasPrefix(kind, "power_supply_metrics"):
			addObject(document, "InputPowerWatts", "Power", "W", "input_power")
			addObject(document, "OutputPowerWatts", "Power", "W", "output_power")
			addObject(document, "InputCurrentAmps", "Current", "A", "input_current")
			addObject(document, "InputVoltage", "Voltage", "V", "input_voltage")
			addObject(document, "FrequencyHz", "Frequency", "Hz", "frequency")
			addObject(document, "TemperatureCelsius", "Temperature", "Cel", "temperature")
			addObject(document, "FanSpeedPercent", "Percent", "%", "fan_speed")
			addObject(document, "EnergykWh", "EnergykWh", "kW.h", "energy")
			addFixedMap(document, "PolyPhasePowerWatts", "Power", "W", "power",
				[]string{"Line1ToLine2", "Line1ToNeutral", "Line2ToLine3", "Line2ToNeutral", "Line3ToLine1", "Line3ToNeutral"})
			addFixedMap(document, "PolyPhaseEnergykWh", "EnergykWh", "kW.h", "energy",
				[]string{"Line1ToLine2", "Line1ToNeutral", "Line2ToLine3", "Line2ToNeutral", "Line3ToLine1", "Line3ToNeutral"})
			addFixedMap(document, "PolyPhaseVoltage", "Voltage", "V", "voltage",
				[]string{"Line1ToLine2", "Line1ToNeutral", "Line2ToLine3", "Line2ToNeutral", "Line3ToLine1", "Line3ToNeutral"})
			addFixedMap(document, "PolyPhaseCurrentAmps", "Current", "A", "current",
				[]string{"Line1", "Line2", "Line3", "Neutral"})
		case strings.HasPrefix(kind, "battery_metrics"):
			addObject(document, "InputCurrentAmps", "Current", "A", "input_current")
			addObject(document, "InputVoltage", "Voltage", "V", "input_voltage")
			addObject(document, "TemperatureCelsius", "Temperature", "Cel", "temperature")
			addObject(document, "ChargePercent", "Percent", "%", "charge")
			addObject(document, "StoredChargeAmpHours", "ChargeAh", "A.h", "stored_charge")
			addObject(document, "StoredEnergyWattHours", "EnergyWh", "W.h", "stored_energy")
		case strings.HasPrefix(kind, "environment_metrics"):
			addObject(document, "AbsoluteHumidity", "AbsoluteHumidity", "g/m3", "absolute_humidity")
			addObject(document, "PowerWatts", "Power", "W", "power")
			addObject(document, "CurrentAmps", "Current", "A", "current")
			addObject(document, "Voltage", "Voltage", "V", "voltage")
			addObject(document, "AmbientTemperatureCelsius", "Temperature", "Cel", "ambient_temperature")
			addObject(document, "DewPointCelsius", "Temperature", "Cel", "dew_point")
			addObject(document, "TemperatureCelsius", "Temperature", "Cel", "temperature")
			addObject(document, "HumidityPercent", "Humidity", "%", "humidity")
			addObject(document, "PowerLoadPercent", "Percent", "%", "power_load")
			addObject(document, "EnergyJoules", "EnergyJoules", "J", "energy")
			addObject(document, "EnergykWh", "EnergykWh", "kW.h", "energy")
		case strings.HasPrefix(kind, "thermal_metrics"):
			addObject(document, "PowerWatts", "Power", "W", "power")
			addObject(document, "DeltaPressurekPa", "PressurekPa", "kPa", "pressure")
			addObject(document, "AirFlowCubicMetersPerMinute", "AirFlowCMM", "m3/min", "airflow")
			addObject(document, "EnergykWh", "EnergykWh", "kW.h", "energy")
			addFixedMap(document, "TemperatureSummaryCelsius", "Temperature", "Cel", "temperature",
				[]string{"Ambient", "Exhaust", "Intake", "Internal"})
		case strings.HasPrefix(kind, "heater_metrics"):
			addObject(document, "PowerWatts", "Power", "W", "power")
		}
	}
	return result
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

func normalizeReading(node *graphNode, raw rawReading) normalizedReading {
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
	reading.Key = stableKey("netdata:redfish:reading:v1", node.Key+"\x00"+identitySource, 32)
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
	reading.Context = surface.Context
	reading.AlarmMetric = surface.AlarmMetric

	reading.Primary = reading.Primary && surface.Primary
	if raw.ReadingScoped {
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
		if spec, ok := matchFixedReadingFamily(fixedFamily, sourceType, units); ok {
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

func readingSemanticClass(node *graphNode, reading normalizedReading) string {
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

func (c *protocolClient) readingObservations(node *graphNode, reading normalizedReading) []hardwareObservation {
	labels := c.metricLabels(node, &reading)
	result := make([]hardwareObservation, 0, 2)
	if reading.Valid {
		result = append(result, hardwareObservation{
			Metric: reading.Metric,
			Value:  reading.Value,
			Labels: labels,
		})
	}
	if reading.SourceAlarm != "" && reading.AlarmMetric != "" {
		result = append(result, stateObservation(reading.AlarmMetric, reading.SourceAlarm, alarmStates, labels))
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

func valueAt(data map[string]any, path string) (any, bool) {
	return jsonPath(data, path)
}

func firstValue(data map[string]any, paths ...string) (any, bool) {
	for _, path := range paths {
		if value, ok := valueAt(data, path); ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func (r normalizedReading) String() string {
	return fmt.Sprintf("%s/%s/%s", r.Family, r.Basis, r.Role)
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
	return rawReadingTimestamp{Value: value, Present: present}
}

func readingTimestamp(source rawReadingTimestamp) *int64 {
	if value, ok := normalizedTimestamp(source.Value); ok {
		return &value
	}
	return nil
}
