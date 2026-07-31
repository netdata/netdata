// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

type normalizedReading struct {
	Key                   string
	IdentitySource        string
	SourcePath            string
	SourceType            string
	SourceUnits           string
	SourceBasis           string
	SourceValue           float64
	SourceExact           string
	SourceScale           float64
	Family                string
	Units                 string
	Basis                 string
	Role                  string
	Exposure              registry.Exposure
	AggregateClass        string
	Value                 float64
	Valid                 bool
	Primary               bool
	Metric                string
	Context               string
	AggregateSemantic     string
	AggregateKinds        []registry.Kind
	AlarmMetric           string
	SemanticSourceClass   string
	Histogram             string
	PhysicalContext       string
	PhysicalSubcontext    string
	ImplementationType    string
	RangeMin              *float64
	RangeMax              *float64
	SourceAlarm           string
	SourceAlarmDiagnostic string
	DerivedAlarm          string
	EffectiveAlarm        string
	EffectiveAlarmSource  string
	EffectiveAlarmReason  string
	Inventory             map[string]any
}

type rawReading struct {
	Path               string
	IdentitySource     string
	Type               string
	Units              string
	Basis              string
	Role               string
	Value              any
	Primary            bool
	PhysicalContext    string
	PhysicalSubcontext string
	ImplementationType string
	RangeMin           any
	RangeMax           any
	Thresholds         map[string]rawThreshold
	Health             string
	ReadingScoped      bool
	Inventory          map[string]any
}

type rawThreshold struct {
	Value              any
	Activation         string
	DwellTime          any
	HysteresisDuration any
	HysteresisReading  any
}

var thresholdPaths = []struct {
	Role string
	Path string
}{
	{"lower_caution", "LowerCaution"},
	{"lower_caution_user", "LowerCautionUser"},
	{"lower_critical", "LowerCritical"},
	{"lower_critical_user", "LowerCriticalUser"},
	{"lower_fatal", "LowerFatal"},
	{"upper_caution", "UpperCaution"},
	{"upper_caution_user", "UpperCautionUser"},
	{"upper_critical", "UpperCritical"},
	{"upper_critical_user", "UpperCriticalUser"},
	{"upper_fatal", "UpperFatal"},
}

func (c *protocolClient) readingsForNode(node *graphNode, observedAt time.Time) []normalizedReading {
	raw := c.rawReadingsForNode(node)
	result := make([]normalizedReading, 0, len(raw)+1)
	for _, source := range raw {
		reading := normalizeReading(node, source, c.config.Alarms.thresholdEvaluationEnabled())
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
		registry.AlgorithmRate,
		readingRateEpoch(source),
	)
	if !emit {
		return normalizedReading{}, false
	}
	surface, ok := registry.MatchReadingSurface("power", "zero", "energy_rate", "energy_rate")
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
	derived.SourceValue = 0
	derived.SourceExact = ""
	derived.Metric = surface.Metric
	derived.AggregateSemantic = surface.AggregateMetric
	derived.AggregateKinds = append([]registry.Kind(nil), surface.AggregateKinds...)
	derived.Exposure = surface.Exposure
	derived.Primary = surface.Primary
	derived.AggregateClass = surface.AggregateClass
	derived.AlarmMetric = ""
	derived.Histogram = ""
	derived.SemanticSourceClass = "energy_rate"
	derived.SourceAlarm = ""
	derived.DerivedAlarm = ""
	derived.EffectiveAlarm = ""
	derived.EffectiveAlarmSource = ""
	derived.EffectiveAlarmReason = ""
	derived.SourceAlarmDiagnostic = ""
	derived.Inventory = map[string]any{"derived_from_reading_key": source.Key}
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
	for _, column := range []string{
		"reading_data_source_uri",
		"sensor_reset_time",
		"lifetime_start_datetime",
	} {
		if value, ok := source.Inventory[column]; ok {
			if text, ok := rateEpochPart(value); ok {
				parts = append(parts, column, text)
			}
		}
	}
	return stableTupleDigest("netdata:redfish:reading-rate-epoch:v1", parts...)
}

func rateEpochPart(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return value, true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case uint64:
		return strconv.FormatUint(value, 10), true
	case float64:
		if isFinite(value) {
			return strconv.FormatFloat(value, 'g', -1, 64), true
		}
	case bool:
		return strconv.FormatBool(value), true
	}
	return "", false
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
		rawURI, _ := source.Inventory["reading_data_source_uri"].(string)
		if canonical, ok := c.canonicalReadingDataSourceURI(node, rawURI); ok {
			source.IdentitySource = canonical + "\x00" + source.Role
			source.Inventory["reading_data_source_uri"] = canonical
		}
	}
	return result
}

func sensorExcerptReadings(source sensorExcerptSource) []rawReading {
	value, present := valueAt(source.Data, "Reading")
	if !present {
		return nil
	}
	physical, _ := stringValueAt(source.Data, "PhysicalContext")
	subcontext, _ := stringValueAt(source.Data, "PhysicalSubContext")
	implementation, _ := stringValueAt(source.Data, "Implementation")
	health, _ := stringValueAt(source.Data, "Status.Health")
	inventory := readingInventory(source.Data)
	result := []rawReading{{
		Path:               source.Path + ".Reading",
		Type:               source.Type,
		Units:              source.Units,
		Basis:              "Zero",
		Role:               "input",
		Value:              value,
		Primary:            true,
		PhysicalContext:    physical,
		PhysicalSubcontext: subcontext,
		ImplementationType: implementation,
		RangeMin:           source.Data["ReadingRangeMin"],
		RangeMax:           source.Data["ReadingRangeMax"],
		Thresholds:         extractThresholds(source.Data, "Thresholds"),
		Health:             health,
		ReadingScoped:      true,
		Inventory:          inventory,
	}}
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
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			Inventory:          map[string]any{"fixed_family": auxiliary.Family},
		})
	}
	return result
}

func mergeProvenSensorReadings(current, excerpts []rawReading) []rawReading {
	for _, excerpt := range excerpts {
		match := -1
		excerptType, excerptOK := readingType(excerpt.Type, excerpt.Units, excerpt.Inventory)
		for index := range current {
			candidate := &current[index]
			candidateType, candidateOK := readingType(candidate.Type, candidate.Units, candidate.Inventory)
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
		if target.RangeMin == nil {
			target.RangeMin = excerpt.RangeMin
		}
		if target.RangeMax == nil {
			target.RangeMax = excerpt.RangeMax
		}
		if target.Health == "" {
			target.Health = excerpt.Health
		}
		if target.Thresholds == nil {
			target.Thresholds = make(map[string]rawThreshold)
		}
		for role, threshold := range excerpt.Thresholds {
			if _, exists := target.Thresholds[role]; !exists {
				target.Thresholds[role] = threshold
			}
		}
		if target.Inventory == nil {
			target.Inventory = make(map[string]any)
		}
		for key, value := range excerpt.Inventory {
			if key == "reading_data_source_uri" {
				continue
			}
			if _, exists := target.Inventory[key]; !exists {
				target.Inventory[key] = value
			}
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
	thresholds := extractThresholds(data, "Thresholds")
	rangeMin, _ := valueAt(data, "ReadingRangeMin")
	rangeMax, _ := valueAt(data, "ReadingRangeMax")
	reading, present := valueAt(data, "Reading")
	if !present {
		return nil
	}
	result := []rawReading{{
		Path:               "Sensor.Reading",
		Type:               sourceType,
		Units:              units,
		Basis:              basis,
		Role:               "input",
		Value:              reading,
		Primary:            true,
		PhysicalContext:    physical,
		PhysicalSubcontext: subcontext,
		ImplementationType: implementation,
		RangeMin:           rangeMin,
		RangeMax:           rangeMax,
		Thresholds:         thresholds,
		Health:             health,
		ReadingScoped:      true,
		Inventory:          readingInventory(data),
	}}
	auxiliaries := []struct {
		Path string
		Role string
	}{
		{"AverageReading", "average"},
		{"LowestIntervalReading", "lowest_interval"},
		{"PeakIntervalReading", "peak_interval"},
		{"LowestReading", "lowest_since_reset"},
		{"PeakReading", "peak_since_reset"},
		{"ReadingRangeMin", "reading_range_min"},
		{"ReadingRangeMax", "reading_range_max"},
		{"MinAllowableOperatingValue", "minimum_allowable"},
		{"MaxAllowableOperatingValue", "maximum_allowable"},
		{"AdjustedMinAllowableOperatingValue", "adjusted_minimum_allowable"},
		{"AdjustedMaxAllowableOperatingValue", "adjusted_maximum_allowable"},
	}
	for _, auxiliary := range auxiliaries {
		value, ok := valueAt(data, auxiliary.Path)
		if !ok {
			continue
		}
		result = append(result, rawReading{
			Path:               "Sensor." + auxiliary.Path,
			Type:               sourceType,
			Units:              units,
			Basis:              basis,
			Role:               auxiliary.Role,
			Value:              value,
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			RangeMin:           rangeMin,
			RangeMax:           rangeMax,
			Inventory:          readingInventory(data),
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
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			Inventory:          map[string]any{"fixed_family": spec.Family},
		})
	}
	return result
}

func legacyThermalReadings(node *graphNode) []rawReading {
	health, _ := stringValueAt(node.Data, "Status.Health")
	switch node.SourcePath {
	case "Temperatures":
		value, ok := firstValue(node.Data, "ReadingCelsius", "Reading")
		if !ok {
			return nil
		}
		return []rawReading{legacyReading(node, value, "Temperature", "Cel", "input", health, legacyThresholds(node.Data, "Celsius"))}
	case "Fans":
		value, ok := firstValue(node.Data, "Reading", "ReadingRPM")
		if !ok {
			return nil
		}
		units, _ := stringValueAt(node.Data, "ReadingUnits")
		sourceType := "Rotational"
		if strings.EqualFold(units, "Percent") || units == "%" {
			sourceType, units = "Percent", "%"
		} else if units == "" || strings.EqualFold(units, "RPM") {
			units = "{rev}/min"
		}
		return []rawReading{legacyReading(node, value, sourceType, units, "input", health, legacyThresholds(node.Data, ""))}
	default:
		return nil
	}
}

func legacyPowerReadings(node *graphNode) []rawReading {
	health, _ := stringValueAt(node.Data, "Status.Health")
	switch node.SourcePath {
	case "PowerControl":
		value, ok := firstValue(node.Data, "PowerConsumedWatts", "PowerRequestedWatts", "PowerAvailableWatts")
		if !ok {
			return nil
		}
		return []rawReading{legacyReading(node, value, "Power", "W", "input", health, legacyThresholds(node.Data, "Watts"))}
	case "Voltages":
		value, ok := firstValue(node.Data, "ReadingVolts", "Reading")
		if !ok {
			return nil
		}
		return []rawReading{legacyReading(node, value, "Voltage", "V", "input", health, legacyThresholds(node.Data, "Volts"))}
	default:
		return nil
	}
}

func legacyReading(
	node *graphNode,
	value any,
	sourceType, units, role, health string,
	thresholds map[string]rawThreshold,
) rawReading {
	physical, _ := stringValueAt(node.Data, "PhysicalContext")
	return rawReading{
		Path:            node.SourceModel + "." + node.SourcePath,
		Type:            sourceType,
		Units:           units,
		Basis:           "Zero",
		Role:            role,
		Value:           value,
		Primary:         true,
		PhysicalContext: physical,
		Thresholds:      thresholds,
		Health:          health,
		ReadingScoped:   true,
		Inventory:       readingInventory(node.Data),
	}
}

func excerptReadings(node *graphNode) []rawReading {
	var result []rawReading
	add := func(document map[string]any, path, sourceType, units, role string) {
		value, ok := valueAt(document, path)
		if !ok {
			return
		}
		physical, _ := stringValueAt(document, "PhysicalContext")
		result = append(result, rawReading{
			Path:            node.Kind + "." + path,
			Type:            sourceType,
			Units:           units,
			Basis:           "Zero",
			Role:            role,
			Value:           value,
			Primary:         true,
			PhysicalContext: physical,
		})
	}
	addObjectAt := func(document map[string]any, path, sourcePath, sourceType, units, role string) {
		raw, ok := valueAt(document, path)
		if !ok || raw == nil {
			return
		}
		object, isObject := raw.(map[string]any)
		if !isObject {
			result = append(result, rawReading{
				Path:    node.Kind + "." + sourcePath,
				Type:    sourceType,
				Units:   units,
				Basis:   "Zero",
				Role:    role,
				Value:   raw,
				Primary: true,
			})
			return
		}
		value, ok := object["Reading"]
		if !ok || value == nil {
			return
		}
		physical, _ := stringValueAt(object, "PhysicalContext")
		subcontext, _ := stringValueAt(object, "PhysicalSubContext")
		implementation, _ := stringValueAt(object, "Implementation")
		inventory := readingInventory(object)
		if role == "stored_energy" {
			inventory["fixed_family"] = "stored_energy"
		}
		result = append(result, rawReading{
			Path:               node.Kind + "." + sourcePath + ".Reading",
			Type:               sourceType,
			Units:              units,
			Basis:              "Zero",
			Role:               role,
			Value:              value,
			Primary:            true,
			PhysicalContext:    physical,
			PhysicalSubcontext: subcontext,
			ImplementationType: implementation,
			RangeMin:           object["ReadingRangeMin"],
			RangeMax:           object["ReadingRangeMax"],
			Thresholds:         extractThresholds(object, "Thresholds"),
			Inventory:          inventory,
		})
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
			auxValue, present := object[auxiliary.Path]
			if !present || auxValue == nil {
				continue
			}
			result = append(result, rawReading{
				Path: node.Kind + "." + sourcePath + "." + auxiliary.Path,
				Type: auxiliary.Type, Units: auxiliary.Units, Basis: "Zero", Role: auxiliary.Role,
				Value: auxValue, PhysicalContext: physical, PhysicalSubcontext: subcontext,
				ImplementationType: implementation,
				Inventory:          map[string]any{"fixed_family": auxiliary.Family},
			})
		}
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
	add(node.Data, "SpeedPercent.Reading", "Percent", "%", "speed")
	add(node.Data, "SpeedPercent", "Percent", "%", "speed")
	add(node.Data, "ReadingRPM", "Rotational", "{rev}/min", "speed")
	add(node.Data, "PowerWatts.Reading", "Power", "W", "power")
	add(node.Data, "PowerWatts", "Power", "W", "power")
	add(node.Data, "PumpSpeedPercent.Reading", "Percent", "%", "speed")
	add(node.Data, "PumpSpeedPercent", "Percent", "%", "speed")
	add(node.Data, "InletPressurekPa.Reading", "PressurekPa", "kPa", "pressure")
	add(node.Data, "FlowLitersPerMinute.Reading", "LiquidFlowLPM", "L/min", "flow")
	add(node.Data, "ValvePositionPercent.Reading", "Valve", "%", "position")
	add(node.Data, "HeatRemovedkW.Reading", "Heat", "kW", "heat_removed")
	add(node.Data, "HumidityPercent.Reading", "Humidity", "%", "humidity")
	add(node.Data, "HumidityPercent", "Humidity", "%", "humidity")
	for kind, document := range node.Enrichment {
		switch {
		case strings.HasPrefix(kind, "processor_metrics"):
			add(document, "CoreVoltage.Reading", "Voltage", "V", "core_voltage")
		case strings.HasPrefix(kind, "power_supply_metrics"):
			addObject(document, "InputPowerWatts", "Power", "W", "input_power")
			addObject(document, "OutputPowerWatts", "Power", "W", "output_power")
			addObject(document, "InputCurrentAmps", "Current", "A", "input_current")
			addObject(document, "InputVoltage", "Voltage", "V", "input_voltage")
			addObject(document, "FrequencyHz", "Frequency", "Hz", "frequency")
			addObject(document, "TemperatureCelsius", "Temperature", "Cel", "temperature")
			addObject(document, "FanSpeedPercent", "Percent", "%", "fan_speed")
			addObject(document, "EnergykWh", "EnergykWh", "kW.h", "energy")
			add(document, "InputPowerWatts.Reading", "Power", "W", "input_power")
			add(document, "OutputPowerWatts.Reading", "Power", "W", "output_power")
			add(document, "InputCurrentAmps.Reading", "Current", "A", "input_current")
			add(document, "InputVoltage.Reading", "Voltage", "V", "input_voltage")
			add(document, "FrequencyHz.Reading", "Frequency", "Hz", "frequency")
			add(document, "TemperatureCelsius.Reading", "Temperature", "Cel", "temperature")
			add(document, "FanSpeedPercent.Reading", "Percent", "%", "fan_speed")
			add(document, "EnergykWh.Reading", "EnergykWh", "kW.h", "energy")
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
			add(document, "InputCurrentAmps.Reading", "Current", "A", "input_current")
			add(document, "InputVoltage.Reading", "Voltage", "V", "input_voltage")
			add(document, "TemperatureCelsius.Reading", "Temperature", "Cel", "temperature")
			add(document, "ChargePercent.Reading", "Percent", "%", "charge")
			add(document, "StoredChargeAmpHours.Reading", "ChargeAh", "A.h", "stored_charge")
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
			add(document, "PowerWatts.Reading", "Power", "W", "power")
			add(document, "CurrentAmps.Reading", "Current", "A", "current")
			add(document, "Voltage.Reading", "Voltage", "V", "voltage")
			add(document, "AmbientTemperatureCelsius.Reading", "Temperature", "Cel", "ambient_temperature")
			add(document, "DewPointCelsius.Reading", "Temperature", "Cel", "dew_point")
			add(document, "TemperatureCelsius.Reading", "Temperature", "Cel", "temperature")
			add(document, "HumidityPercent.Reading", "Humidity", "%", "humidity")
			add(document, "PowerLoadPercent.Reading", "Percent", "%", "power_load")
			add(document, "EnergyJoules.Reading", "EnergyJoules", "J", "energy")
			add(document, "EnergykWh.Reading", "EnergykWh", "kW.h", "energy")
		case strings.HasPrefix(kind, "thermal_metrics"):
			addObject(document, "PowerWatts", "Power", "W", "power")
			addObject(document, "DeltaPressurekPa", "PressurekPa", "kPa", "pressure")
			addObject(document, "AirFlowCubicMetersPerMinute", "AirFlowCMM", "m3/min", "airflow")
			addObject(document, "EnergykWh", "EnergykWh", "kW.h", "energy")
			add(document, "PowerWatts.Reading", "Power", "W", "power")
			add(document, "DeltaPressurekPa.Reading", "PressurekPa", "kPa", "pressure")
			add(document, "AirFlowCubicMetersPerMinute.Reading", "AirFlowCMM", "m3/min", "airflow")
			add(document, "EnergykWh.Reading", "EnergykWh", "kW.h", "energy")
			addFixedMap(document, "TemperatureSummaryCelsius", "Temperature", "Cel", "temperature",
				[]string{"Ambient", "Exhaust", "Intake", "Internal"})
		case strings.HasPrefix(kind, "heater_metrics"):
			addObject(document, "PowerWatts", "Power", "W", "power")
			add(document, "PowerWatts.Reading", "Power", "W", "power")
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

func normalizeReading(node *graphNode, raw rawReading, evaluate bool) normalizedReading {
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
		Inventory:          make(map[string]any),
	}
	if reading.Role == "" {
		reading.Role = "input"
	}
	reading.Key = stableKey("netdata:redfish:reading:v1", node.Key+"\x00"+identitySource, 32)
	spec, fixed := readingType(raw.Type, raw.Units, raw.Inventory)
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
	reading.Inventory = normalizeReadingInventory(raw.Inventory, spec.Scale)
	reading.SemanticSourceClass = readingSemanticClass(node, reading)
	surface, ok := registry.MatchReadingSurface(
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
	reading.AggregateSemantic = surface.AggregateMetric
	reading.AggregateKinds = append([]registry.Kind(nil), surface.AggregateKinds...)
	reading.Exposure = surface.Exposure
	reading.Primary = reading.Primary && surface.Primary
	reading.AggregateClass = surface.AggregateClass
	reading.Histogram = surface.Histogram
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
	maps.Copy(reading.Inventory, thresholdInventory(raw.Thresholds, rationalMultiplier(spec.Scale)))
	exact, sourceValue, ok := numericValue(raw.Value)
	if !ok {
		reading.EffectiveAlarm, reading.EffectiveAlarmSource = fuseAlarm(reading.SourceAlarm, "")
		return reading
	}
	if reading.Family == "energy" {
		reading.Inventory["reading_source_value_exact"] = exact
		reading.SourceExact = exact
	}
	reading.SourceValue = sourceValue
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
	if evaluate && reading.Valid {
		reading.DerivedAlarm, reading.EffectiveAlarmReason = deriveAlarm(
			reading.Value,
			raw.Thresholds,
			rationalMultiplier(spec.Scale),
		)
	}
	reading.EffectiveAlarm, reading.EffectiveAlarmSource = fuseAlarm(reading.SourceAlarm, reading.DerivedAlarm)
	return reading
}

func readingType(sourceType, units string, inventory map[string]any) (registry.ReadingTypeSpec, bool) {
	if fixed, ok := inventory["fixed_family"].(string); ok {
		if spec, ok := registry.MatchFixedReadingFamily(fixed, sourceType, units); ok {
			return spec, true
		}
	}
	return registry.MatchReadingType(sourceType, units)
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

func rationalMultiplier(value registry.Rational) float64 {
	if value.Den == 0 {
		return 1
	}
	return float64(value.Num) / float64(value.Den)
}

func scaleReadingValue(value float64, scale registry.Rational) float64 {
	return value * rationalMultiplier(scale)
}

func (c *protocolClient) readingObservations(node *graphNode, reading normalizedReading) []hardwareObservation {
	labels := c.metricLabels(node, &reading)
	scope := c.scopeForNode(node)
	result := make([]hardwareObservation, 0, 2)
	if reading.Valid {
		result = append(result, hardwareObservation{
			Metric: reading.Metric,
			Value:  reading.Value,
			Labels: labels,
			Scope:  scope,
		})
	}
	if reading.EffectiveAlarm != "" && reading.AlarmMetric != "" {
		result = append(result, stateObservation(reading.AlarmMetric, reading.EffectiveAlarm, registry.AlarmStates, labels, scope))
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

func deriveAlarm(value float64, thresholds map[string]rawThreshold, multiplier float64) (string, string) {
	type candidate struct {
		State string
		Rank  int
		Role  string
	}
	best := candidate{}
	evaluated := false
	for role, threshold := range thresholds {
		if strings.EqualFold(strings.TrimSpace(threshold.Activation), "Disabled") {
			continue
		}
		_, source, ok := numericValue(threshold.Value)
		if !ok {
			continue
		}
		evaluated = true
		boundary := source * multiplier
		triggered := strings.HasPrefix(role, "lower_") && value < boundary ||
			strings.HasPrefix(role, "upper_") && value > boundary
		if !triggered {
			continue
		}
		state, rank := "warning", 1
		if strings.Contains(role, "critical") {
			state, rank = "critical", 2
		} else if strings.Contains(role, "fatal") {
			state, rank = "emergency", 3
		}
		if rank > best.Rank {
			best = candidate{State: state, Rank: rank, Role: role}
		}
	}
	if best.State == "" {
		if evaluated {
			return "clear", "thresholds_clear"
		}
		return "", ""
	}
	return best.State, "threshold_" + best.Role
}

func fuseAlarm(source, derived string) (string, string) {
	// A valid abnormal BMC decision has precedence over current-value
	// threshold evaluation. This preserves firmware dwell and hysteresis;
	// evaluation is a fallback only when the source is clear or absent.
	if alarmRank(source) > 0 {
		return source, "source"
	}
	if derived != "" && alarmRank(derived) > alarmRank(source) {
		if source == "" {
			return derived, "derived"
		}
		return derived, "combined"
	}
	if source != "" {
		return source, "source"
	}
	if derived != "" {
		return derived, "derived"
	}
	return "", ""
}

func alarmRank(value string) int {
	switch value {
	case "warning", "cap", "alarm":
		return 1
	case "critical":
		return 2
	case "emergency", "fault":
		return 3
	default:
		return 0
	}
}

func extractThresholds(data map[string]any, base string) map[string]rawThreshold {
	result := make(map[string]rawThreshold)
	for _, threshold := range thresholdPaths {
		prefix := base
		if prefix != "" {
			prefix += "."
		}
		prefix += threshold.Path
		value, ok := valueAt(data, prefix+".Reading")
		if !ok {
			continue
		}
		activation, _ := stringValueAt(data, prefix+".Activation")
		dwellTime, _ := valueAt(data, prefix+".DwellTime")
		hysteresisDuration, _ := valueAt(data, prefix+".HysteresisDuration")
		hysteresisReading, _ := valueAt(data, prefix+".HysteresisReading")
		result[threshold.Role] = rawThreshold{
			Value: value, Activation: activation,
			DwellTime: dwellTime, HysteresisDuration: hysteresisDuration,
			HysteresisReading: hysteresisReading,
		}
	}
	return result
}

func legacyThresholds(data map[string]any, suffix string) map[string]rawThreshold {
	result := make(map[string]rawThreshold)
	mapping := []struct {
		Role string
		Name string
	}{
		{"lower_caution", "LowerThresholdNonCritical"},
		{"lower_critical", "LowerThresholdCritical"},
		{"lower_fatal", "LowerThresholdFatal"},
		{"upper_caution", "UpperThresholdNonCritical"},
		{"upper_critical", "UpperThresholdCritical"},
		{"upper_fatal", "UpperThresholdFatal"},
	}
	for _, item := range mapping {
		for _, path := range []string{item.Name + suffix, item.Name} {
			value, ok := valueAt(data, path)
			if ok {
				result[item.Role] = rawThreshold{Value: value}
				break
			}
		}
	}
	return result
}

func thresholdInventory(thresholds map[string]rawThreshold, multiplier float64) map[string]any {
	result := make(map[string]any)
	for role, threshold := range thresholds {
		if exact, value, ok := numericValue(threshold.Value); ok {
			result["threshold_"+role+"_source"] = value
			result["threshold_"+role] = value * multiplier
			_ = exact
		}
		if threshold.Activation != "" {
			result["threshold_"+role+"_activation"] = threshold.Activation
		}
		if threshold.DwellTime != nil {
			if _, seconds, ok := numericSourceValue(threshold.DwellTime, registry.AlgorithmDurationPercent); ok && seconds >= 0 {
				result["threshold_"+role+"_dwell_seconds"] = seconds
			}
		}
		if threshold.HysteresisDuration != nil {
			if _, seconds, ok := numericSourceValue(threshold.HysteresisDuration, registry.AlgorithmDurationPercent); ok && seconds >= 0 {
				result["threshold_"+role+"_hysteresis_duration_seconds"] = seconds
			}
		}
		if threshold.HysteresisReading != nil {
			if _, value, ok := numericValue(threshold.HysteresisReading); ok {
				result["threshold_"+role+"_hysteresis_source"] = value
				result["threshold_"+role+"_hysteresis"] = value * multiplier
			}
		}
	}
	return result
}

func readingInventory(data map[string]any) map[string]any {
	result := make(map[string]any)
	for _, field := range []struct {
		Path   string
		Column string
	}{
		{"ReadingTime", "reading_time"},
		{"ReadingRangeMin", "reading_range_min_source"},
		{"ReadingRangeMax", "reading_range_max_source"},
		{"AverageReading", "reading_average_source"},
		{"LowestIntervalReading", "reading_lowest_interval_source"},
		{"PeakIntervalReading", "reading_peak_interval_source"},
		{"LowestReading", "reading_lowest_source"},
		{"PeakReading", "reading_peak_source"},
		{"MinAllowableOperatingValue", "minimum_allowable_source"},
		{"MaxAllowableOperatingValue", "maximum_allowable_source"},
		{"AdjustedMinAllowableOperatingValue", "adjusted_minimum_allowable_source"},
		{"AdjustedMaxAllowableOperatingValue", "adjusted_maximum_allowable_source"},
		{"ReadingAccuracy", "reading_accuracy_source"},
		{"Accuracy", "accuracy_percent"},
		{"Precision", "precision"},
		{"AveragingInterval", "averaging_interval"},
		{"AveragingIntervalAchieved", "averaging_interval_achieved"},
		{"SensorResetTime", "sensor_reset_time"},
		{"LowestReadingTime", "lowest_reading_time"},
		{"PeakReadingTime", "peak_reading_time"},
		{"Calibration", "calibration_source"},
		{"CalibrationTime", "calibration_time"},
		{"ElectricalContext", "electrical_context"},
		{"PhysicalContext", "physical_context"},
		{"PhysicalSubContext", "physical_subcontext"},
		{"Implementation", "implementation_type"},
		{"VoltageType", "voltage_type"},
		{"LifetimeReading", "lifetime_reading_source"},
		{"LifetimeStartDateTime", "lifetime_start_datetime"},
		{"DataSourceUri", "reading_data_source_uri"},
	} {
		if value, ok := valueAt(data, field.Path); ok {
			result[field.Column] = value
		}
	}
	return result
}

func normalizeReadingInventory(source map[string]any, scale registry.Rational) map[string]any {
	result := make(map[string]any)
	scaled := map[string]string{
		"reading_range_min_source":          "reading_range_min",
		"reading_range_max_source":          "reading_range_max",
		"reading_average_source":            "reading_average",
		"reading_lowest_interval_source":    "reading_lowest_interval",
		"reading_peak_interval_source":      "reading_peak_interval",
		"reading_lowest_source":             "reading_lowest",
		"reading_peak_source":               "reading_peak",
		"minimum_allowable_source":          "minimum_allowable",
		"maximum_allowable_source":          "maximum_allowable",
		"adjusted_minimum_allowable_source": "adjusted_minimum_allowable",
		"adjusted_maximum_allowable_source": "adjusted_maximum_allowable",
		"reading_accuracy_source":           "reading_accuracy",
		"calibration_source":                "calibration",
	}
	for sourceColumn, normalizedColumn := range scaled {
		raw, ok := source[sourceColumn]
		if !ok {
			continue
		}
		if _, value, ok := numericValue(raw); ok {
			result[sourceColumn] = value
			normalized := scaleReadingValue(value, scale)
			if isFinite(normalized) {
				result[normalizedColumn] = normalized
			}
		}
	}
	for _, column := range []string{
		"accuracy_percent",
		"precision",
		"lifetime_reading_source",
	} {
		if _, value, ok := numericValue(source[column]); ok {
			result[column] = value
		}
	}
	if raw, ok := source["averaging_interval"]; ok {
		if _, seconds, ok := numericSourceValue(raw, registry.AlgorithmDurationPercent); ok && seconds >= 0 {
			result["averaging_interval"] = seconds
		}
	}
	if value, ok := source["averaging_interval_achieved"].(bool); ok {
		result["averaging_interval_achieved"] = value
	}
	for _, column := range []string{
		"reading_time",
		"sensor_reset_time",
		"lowest_reading_time",
		"peak_reading_time",
		"calibration_time",
		"lifetime_start_datetime",
	} {
		if value, ok := normalizedTimestamp(source[column]); ok {
			result[column] = value
		}
	}
	for _, column := range []string{
		"implementation_type",
		"electrical_context",
		"physical_context",
		"physical_subcontext",
		"voltage_type",
		"reading_data_source_uri",
	} {
		if value, ok := source[column].(string); ok {
			result[column] = value
		}
	}
	return result
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
