// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"strings"
)

// Each source names a scalar or SensorExcerpt property. Keys restrict map-valued
// sources to the approved members; arbitrary vendor members are not readings.
type excerptReadingSource struct {
	Path, Type, Units, Role string
	Keys                    []string
}

func excerptReadings(node *Resource) []rawReading {
	result := excerptDocumentReadings(nil, node.Kind, node.Data, "", []excerptReadingSource{
		{"SpeedPercent", "Percent", "%", "speed", nil},
		{"SecondarySpeedPercent", "Percent", "%", "speed", nil},
		{"PowerWatts", "Power", "W", "power", nil},
		{"PumpSpeedPercent", "Percent", "%", "speed", nil},
		{"InletPressurekPa", "PressurekPa", "kPa", "pressure", nil},
		{"FlowLitersPerMinute", "LiquidFlowLPM", "L/min", "flow", nil},
		{"ValvePositionPercent", "Valve", "%", "position", nil},
		{"HeatRemovedkW", "Heat", "kW", "heat_removed", nil},
		{"HumidityPercent", "Humidity", "%", "humidity", nil},
		{"StateOfHealthPercent", "Percent", "%", "state_of_health", nil},
		{"DeltaTemperatureCelsius", "Temperature", "Cel", "temperature", nil},
		{"ReturnTemperatureCelsius", "Temperature", "Cel", "temperature", nil},
		{"SupplyTemperatureCelsius", "Temperature", "Cel", "temperature", nil},
		{"DeltaPressurekPa", "PressurekPa", "kPa", "pressure", nil},
		{"ReturnPressurekPa", "PressurekPa", "kPa", "pressure", nil},
		{"SupplyPressurekPa", "PressurekPa", "kPa", "pressure", nil},
		{"DeltaLiquidPressurekPa", "PressurekPa", "kPa", "pressure", nil},
		{"CurrentAmps", "Current", "A", "current", nil},
		{"Voltage", "Voltage", "V", "voltage", nil},
		{"ReadingRPM", "Rotational", "{rev}/min", "speed", nil},
	})
	for enrichmentKey, resource := range node.Enrichment {
		var sources []excerptReadingSource
		switch {
		case strings.HasPrefix(enrichmentKey, "processor_metrics"):
			sources = []excerptReadingSource{
				{"CoreVoltage", "Voltage", "V", "core_voltage", nil},
			}
		case strings.HasPrefix(enrichmentKey, "power_supply_metrics"):
			sources = []excerptReadingSource{
				{"InputPowerWatts", "Power", "W", "input_power", nil},
				{"OutputPowerWatts", "Power", "W", "output_power", nil},
				{"InputCurrentAmps", "Current", "A", "input_current", nil},
				{"InputVoltage", "Voltage", "V", "input_voltage", nil},
				{"FrequencyHz", "Frequency", "Hz", "frequency", nil},
				{"TemperatureCelsius", "Temperature", "Cel", "temperature", nil},
				{"FanSpeedPercent", "Percent", "%", "fan_speed", nil},
				{"EnergykWh", "EnergykWh", "kW.h", "energy", nil},
				{
					"PolyPhasePowerWatts",
					"Power",
					"W",
					"power",
					[]string{
						"Line1ToLine2",
						"Line1ToNeutral",
						"Line2ToLine3",
						"Line2ToNeutral",
						"Line3ToLine1",
						"Line3ToNeutral",
					},
				},
				{
					"PolyPhaseEnergykWh",
					"EnergykWh",
					"kW.h",
					"energy",
					[]string{
						"Line1ToLine2",
						"Line1ToNeutral",
						"Line2ToLine3",
						"Line2ToNeutral",
						"Line3ToLine1",
						"Line3ToNeutral",
					},
				},
				{
					"PolyPhaseVoltage",
					"Voltage",
					"V",
					"voltage",
					[]string{
						"Line1ToLine2",
						"Line1ToNeutral",
						"Line2ToLine3",
						"Line2ToNeutral",
						"Line3ToLine1",
						"Line3ToNeutral",
					},
				},
				{"PolyPhaseCurrentAmps", "Current", "A", "current", []string{"Line1", "Line2", "Line3", "Neutral"}},
			}
		case strings.HasPrefix(enrichmentKey, "battery_metrics"):
			sources = []excerptReadingSource{
				{"InputCurrentAmps", "Current", "A", "input_current", nil},
				{"InputVoltage", "Voltage", "V", "input_voltage", nil},
				{"TemperatureCelsius", "Temperature", "Cel", "temperature", nil},
				{"ChargePercent", "Percent", "%", "charge", nil},
				{"StoredChargeAmpHours", "ChargeAh", "A.h", "stored_charge", nil},
				{"StoredEnergyWattHours", "EnergyWh", "W.h", "stored_energy", nil},
			}
		case strings.HasPrefix(enrichmentKey, "environment_metrics"):
			sources = []excerptReadingSource{
				{"AbsoluteHumidity", "AbsoluteHumidity", "g/m3", "absolute_humidity", nil},
				{"PowerWatts", "Power", "W", "power", nil},
				{"CurrentAmps", "Current", "A", "current", nil},
				{"Voltage", "Voltage", "V", "voltage", nil},
				{"AmbientTemperatureCelsius", "Temperature", "Cel", "ambient_temperature", nil},
				{"DewPointCelsius", "Temperature", "Cel", "dew_point", nil},
				{"TemperatureCelsius", "Temperature", "Cel", "temperature", nil},
				{"HumidityPercent", "Humidity", "%", "humidity", nil},
				{"PowerLoadPercent", "Percent", "%", "power_load", nil},
				{"EnergyJoules", "EnergyJoules", "J", "energy", nil},
				{"EnergykWh", "EnergykWh", "kW.h", "energy", nil},
			}
		case strings.HasPrefix(enrichmentKey, "thermal_metrics"):
			sources = []excerptReadingSource{
				{"PowerWatts", "Power", "W", "power", nil},
				{"DeltaPressurekPa", "PressurekPa", "kPa", "pressure", nil},
				{"AirFlowCubicMetersPerMinute", "AirFlowCMM", "m3/min", "airflow", nil},
				{"EnergykWh", "EnergykWh", "kW.h", "energy", nil},
				{
					"TemperatureSummaryCelsius",
					"Temperature",
					"Cel",
					"temperature",
					[]string{"Ambient", "Exhaust", "Intake", "Internal"},
				},
			}
		case strings.HasPrefix(enrichmentKey, "heater_metrics"):
			sources = []excerptReadingSource{
				{"PowerWatts", "Power", "W", "power", nil},
			}
		}
		// Keep acquired documents distinct before path-based reading deduplication.
		result = excerptDocumentReadings(result, enrichmentKey, resource.Data, resource.URI, sources)
	}
	return result
}

func excerptDocumentReadings(
	result []rawReading,
	pathPrefix string,
	document map[string]any,
	sourceDocumentURI string,
	sources []excerptReadingSource,
) []rawReading {
	physical, _ := stringValueAt(document, "PhysicalContext")
	for _, source := range sources {
		raw, present := valueAt(document, source.Path)
		if !present {
			continue
		}
		path := pathPrefix + "." + source.Path
		if source.Keys == nil {
			result = excerptValueReadings(result, raw, path, sourceDocumentURI, source, physical)
			continue
		}
		object, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range source.Keys {
			// Fixed-map scalars carry no parent physical context.
			if raw, present := object[key]; present {
				result = excerptValueReadings(result, raw, path+"."+key, sourceDocumentURI, source, "")
			}
		}
	}
	return result
}

func excerptValueReadings(
	result []rawReading,
	raw any,
	path, sourceDocumentURI string,
	source excerptReadingSource,
	physical string,
) []rawReading {
	object, isObject := raw.(map[string]any)
	if !isObject {
		reading := rawReading{
			Path:            path,
			Type:            source.Type,
			Units:           source.Units,
			Basis:           "Zero",
			Role:            source.Role,
			Value:           raw,
			ValuePresent:    true,
			Primary:         true,
			PhysicalContext: physical,
		}
		if source.Role == "stored_energy" {
			reading.FixedFamily = "stored_energy"
		}
		return append(result, reading)
	}
	readings := sensorExcerptReadings(SensorExcerpt{
		Path:  path,
		Type:  source.Type,
		Units: source.Units,
		Data:  object,
	})
	for index := range readings {
		reading := &readings[index]
		reading.SourceDocumentURI = sourceDocumentURI
		if reading.Role == "input" {
			reading.Role = source.Role
			if source.Role == "stored_energy" {
				reading.FixedFamily = "stored_energy"
			}
		}
	}
	return append(result, readings...)
}
