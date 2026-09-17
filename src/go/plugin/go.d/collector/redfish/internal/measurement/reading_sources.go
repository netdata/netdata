// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

type readingTypeKey struct{ SourceType, Units string }
type readingTypeDescriptor struct {
	Family, Units string
	Scale         valueScale
}
type readingKey struct{ Family, Basis, Role, SemanticClass string }
type readingDescriptor struct {
	Metric, AlarmMetric string
	Primary             bool
}

// Exact source types and units prevent accidental interpretation of vendor units.
var readingTypes = map[readingTypeKey]readingTypeDescriptor{
	{"Temperature", "Cel"}:               {"temperature", "Celsius", valueScale{1, 1}},
	{"Humidity", "%"}:                    {"humidity", "percentage", valueScale{1, 1}},
	{"Power", "W"}:                       {"power", "watts", valueScale{1, 1}},
	{"EnergykWh", "kW.h"}:                {"energy", "joules", valueScale{3600000, 1}},
	{"EnergyJoules", "J"}:                {"energy", "joules", valueScale{1, 1}},
	{"EnergyWh", "W.h"}:                  {"energy", "joules", valueScale{3600, 1}},
	{"ChargeAh", "A.h"}:                  {"charge", "ampere-hours", valueScale{1, 1}},
	{"Voltage", "V"}:                     {"voltage", "volts", valueScale{1, 1}},
	{"Current", "A"}:                     {"current", "amperes", valueScale{1, 1}},
	{"Frequency", "Hz"}:                  {"frequency", "hertz", valueScale{1, 1}},
	{"Pressure", "Pa"}:                   {"pressure", "pascals", valueScale{1, 1}},
	{"PressurekPa", "kPa"}:               {"pressure", "pascals", valueScale{1000, 1}},
	{"PressurePa", "Pa"}:                 {"pressure", "pascals", valueScale{1, 1}},
	{"LiquidLevel", "cm"}:                {"liquid_level", "meters", valueScale{1, 100}},
	{"Rotational", "{rev}/min"}:          {"rotational_speed", "RPM", valueScale{1, 1}},
	{"Rotational", "RPM"}:                {"rotational_speed", "RPM", valueScale{1, 1}},
	{"AirFlow", "[ft_i]3/min"}:           {"air_flow", "cubic-meters/minute", valueScale{28316846592, 1000000000000}},
	{"AirFlowCMM", "m3/min"}:             {"air_flow", "cubic-meters/minute", valueScale{1, 1}},
	{"LiquidFlow", "L/s"}:                {"liquid_flow", "liters/minute", valueScale{60, 1}},
	{"LiquidFlowLPM", "L/min"}:           {"liquid_flow", "liters/minute", valueScale{1, 1}},
	{"Barometric", "mm[Hg]"}:             {"barometric_pressure", "pascals", valueScale{133322387415, 1000000000}},
	{"Altitude", "m"}:                    {"altitude", "meters", valueScale{1, 1}},
	{"Percent", "%"}:                     {"percentage", "percentage", valueScale{1, 1}},
	{"AbsoluteHumidity", "g/m3"}:         {"absolute_humidity", "grams/cubic-meter", valueScale{1, 1}},
	{"Heat", "kW"}:                       {"heat", "watts", valueScale{1000, 1}},
	{"LinearPosition", "m"}:              {"linear_position", "meters", valueScale{1, 1}},
	{"LinearVelocity", "m/s"}:            {"linear_velocity", "meters/second", valueScale{1, 1}},
	{"LinearAcceleration", "m/s2"}:       {"linear_acceleration", "meters/second2", valueScale{1, 1}},
	{"RotationalPosition", "rad"}:        {"rotational_position", "radians", valueScale{1, 1}},
	{"RotationalVelocity", "rad/s"}:      {"rotational_velocity", "radians/second", valueScale{1, 1}},
	{"RotationalAcceleration", "rad/s2"}: {"rotational_acceleration", "radians/second2", valueScale{1, 1}},
	{"Valve", "%"}:                       {"valve_position", "percentage", valueScale{1, 1}},
}

var fixedReadingFamilies = map[string]readingTypeDescriptor{
	"apparent_power":      {"apparent_power", "volt-amperes", valueScale{1, 1}},
	"reactive_power":      {"reactive_power", "vars", valueScale{1, 1}},
	"apparent_energy":     {"apparent_energy", "joule-equivalent", valueScale{3600000, 1}},
	"reactive_energy":     {"reactive_energy", "joule-equivalent", valueScale{3600000, 1}},
	"crest_factor":        {"crest_factor", "ratio", valueScale{1, 1}},
	"power_factor":        {"power_factor", "ratio", valueScale{1, 1}},
	"phase_angle":         {"phase_angle", "degrees", valueScale{1, 1}},
	"harmonic_distortion": {"harmonic_distortion", "percentage", valueScale{1, 1}},
	"rotational_speed":    {"rotational_speed", "RPM", valueScale{1, 1}},
	"stored_energy":       {"stored_energy", "watt-hours", valueScale{1, 1}},
}

// These mappings describe accepted source semantics, not generated charts.
// Chart presentation is authored independently in charts.yaml.
var readingDescriptors = map[readingKey]readingDescriptor{
	{"temperature", "zero", "input", "direct"}: {
		"system_hw_sensor_temperature_input",
		"system_hw_sensor_temperature_alarm",
		true,
	},
	{"temperature", "zero", "average", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "zero", "lowest_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "zero", "peak_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "zero", "lowest_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "zero", "peak_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "delta", "input", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		true,
	},
	{"temperature", "delta", "average", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "delta", "lowest_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "delta", "peak_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "delta", "lowest_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "delta", "peak_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "headroom", "input", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		true,
	},
	{"temperature", "headroom", "average", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "headroom", "lowest_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "headroom", "peak_interval", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "headroom", "lowest_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"temperature", "headroom", "peak_since_reset", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		false,
	},
	{"humidity", "zero", "input", "direct"}: {
		"system_hw_sensor_humidity_input",
		"system_hw_sensor_humidity_alarm",
		true,
	},
	{"humidity", "zero", "average", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "zero", "lowest_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "zero", "peak_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "zero", "lowest_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "zero", "peak_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "delta", "input", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		true,
	},
	{"humidity", "delta", "average", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "delta", "lowest_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "delta", "peak_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "delta", "lowest_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "delta", "peak_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "headroom", "input", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		true,
	},
	{"humidity", "headroom", "average", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "headroom", "lowest_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "headroom", "peak_interval", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "headroom", "lowest_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"humidity", "headroom", "peak_since_reset", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		false,
	},
	{"power", "zero", "input", "direct"}: {
		"system_hw_sensor_power_input",
		"system_hw_sensor_power_alarm",
		true,
	},
	{"power", "zero", "average", "direct"}:          {"system_hw_sensor_power_average", "", false},
	{"power", "zero", "lowest_interval", ""}:        {"reading_power_value", "reading_alarm", false},
	{"power", "zero", "peak_interval", ""}:          {"reading_power_value", "reading_alarm", false},
	{"power", "zero", "lowest_since_reset", ""}:     {"reading_power_value", "reading_alarm", false},
	{"power", "zero", "peak_since_reset", ""}:       {"reading_power_value", "reading_alarm", false},
	{"power", "delta", "input", ""}:                 {"reading_power_value", "reading_alarm", true},
	{"power", "delta", "average", ""}:               {"reading_power_value", "reading_alarm", false},
	{"power", "delta", "lowest_interval", ""}:       {"reading_power_value", "reading_alarm", false},
	{"power", "delta", "peak_interval", ""}:         {"reading_power_value", "reading_alarm", false},
	{"power", "delta", "lowest_since_reset", ""}:    {"reading_power_value", "reading_alarm", false},
	{"power", "delta", "peak_since_reset", ""}:      {"reading_power_value", "reading_alarm", false},
	{"power", "headroom", "input", ""}:              {"reading_power_value", "reading_alarm", true},
	{"power", "headroom", "average", ""}:            {"reading_power_value", "reading_alarm", false},
	{"power", "headroom", "lowest_interval", ""}:    {"reading_power_value", "reading_alarm", false},
	{"power", "headroom", "peak_interval", ""}:      {"reading_power_value", "reading_alarm", false},
	{"power", "headroom", "lowest_since_reset", ""}: {"reading_power_value", "reading_alarm", false},
	{"power", "headroom", "peak_since_reset", ""}:   {"reading_power_value", "reading_alarm", false},
	{"energy", "zero", "input", "direct"}: {
		"system_hw_sensor_energy_input",
		"system_hw_sensor_energy_alarm",
		true,
	},
	{"energy", "zero", "average", ""}:                {"reading_energy_value", "reading_alarm", false},
	{"energy", "zero", "lowest_interval", ""}:        {"reading_energy_value", "reading_alarm", false},
	{"energy", "zero", "peak_interval", ""}:          {"reading_energy_value", "reading_alarm", false},
	{"energy", "zero", "lowest_since_reset", ""}:     {"reading_energy_value", "reading_alarm", false},
	{"energy", "zero", "peak_since_reset", ""}:       {"reading_energy_value", "reading_alarm", false},
	{"energy", "delta", "input", ""}:                 {"reading_energy_value", "reading_alarm", true},
	{"energy", "delta", "average", ""}:               {"reading_energy_value", "reading_alarm", false},
	{"energy", "delta", "lowest_interval", ""}:       {"reading_energy_value", "reading_alarm", false},
	{"energy", "delta", "peak_interval", ""}:         {"reading_energy_value", "reading_alarm", false},
	{"energy", "delta", "lowest_since_reset", ""}:    {"reading_energy_value", "reading_alarm", false},
	{"energy", "delta", "peak_since_reset", ""}:      {"reading_energy_value", "reading_alarm", false},
	{"energy", "headroom", "input", ""}:              {"reading_energy_value", "reading_alarm", true},
	{"energy", "headroom", "average", ""}:            {"reading_energy_value", "reading_alarm", false},
	{"energy", "headroom", "lowest_interval", ""}:    {"reading_energy_value", "reading_alarm", false},
	{"energy", "headroom", "peak_interval", ""}:      {"reading_energy_value", "reading_alarm", false},
	{"energy", "headroom", "lowest_since_reset", ""}: {"reading_energy_value", "reading_alarm", false},
	{"energy", "headroom", "peak_since_reset", ""}:   {"reading_energy_value", "reading_alarm", false},
	{"charge", "zero", "input", ""}:                  {"reading_charge_value", "reading_alarm", true},
	{"charge", "zero", "average", ""}:                {"reading_charge_value", "reading_alarm", false},
	{"charge", "zero", "lowest_interval", ""}:        {"reading_charge_value", "reading_alarm", false},
	{"charge", "zero", "peak_interval", ""}:          {"reading_charge_value", "reading_alarm", false},
	{"charge", "zero", "lowest_since_reset", ""}:     {"reading_charge_value", "reading_alarm", false},
	{"charge", "zero", "peak_since_reset", ""}:       {"reading_charge_value", "reading_alarm", false},
	{"charge", "delta", "input", ""}:                 {"reading_charge_value", "reading_alarm", true},
	{"charge", "delta", "average", ""}:               {"reading_charge_value", "reading_alarm", false},
	{"charge", "delta", "lowest_interval", ""}:       {"reading_charge_value", "reading_alarm", false},
	{"charge", "delta", "peak_interval", ""}:         {"reading_charge_value", "reading_alarm", false},
	{"charge", "delta", "lowest_since_reset", ""}:    {"reading_charge_value", "reading_alarm", false},
	{"charge", "delta", "peak_since_reset", ""}:      {"reading_charge_value", "reading_alarm", false},
	{"charge", "headroom", "input", ""}:              {"reading_charge_value", "reading_alarm", true},
	{"charge", "headroom", "average", ""}:            {"reading_charge_value", "reading_alarm", false},
	{"charge", "headroom", "lowest_interval", ""}:    {"reading_charge_value", "reading_alarm", false},
	{"charge", "headroom", "peak_interval", ""}:      {"reading_charge_value", "reading_alarm", false},
	{"charge", "headroom", "lowest_since_reset", ""}: {"reading_charge_value", "reading_alarm", false},
	{"charge", "headroom", "peak_since_reset", ""}:   {"reading_charge_value", "reading_alarm", false},
	{"voltage", "zero", "input", "direct"}: {
		"system_hw_sensor_voltage_input",
		"system_hw_sensor_voltage_alarm",
		true,
	},
	{"voltage", "zero", "average", "direct"}: {"system_hw_sensor_voltage_average", "", false},
	{"voltage", "zero", "lowest_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "zero", "peak_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "zero", "lowest_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "zero", "peak_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "delta", "input", ""}: {"reading_voltage_value", "reading_alarm", true},
	{"voltage", "delta", "average", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "delta", "lowest_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "delta", "peak_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "delta", "lowest_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "delta", "peak_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "headroom", "input", ""}: {"reading_voltage_value", "reading_alarm", true},
	{"voltage", "headroom", "average", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "headroom", "lowest_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "headroom", "peak_interval", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "headroom", "lowest_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"voltage", "headroom", "peak_since_reset", ""}: {
		"reading_voltage_value",
		"reading_alarm",
		false,
	},
	{"current", "zero", "input", "direct"}: {
		"system_hw_sensor_current_input",
		"system_hw_sensor_current_alarm",
		true,
	},
	{"current", "zero", "average", "direct"}: {"system_hw_sensor_current_average", "", false},
	{"current", "zero", "lowest_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "zero", "peak_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "zero", "lowest_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "zero", "peak_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "delta", "input", ""}: {"reading_current_value", "reading_alarm", true},
	{"current", "delta", "average", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "delta", "lowest_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "delta", "peak_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "delta", "lowest_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "delta", "peak_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "headroom", "input", ""}: {"reading_current_value", "reading_alarm", true},
	{"current", "headroom", "average", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "headroom", "lowest_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "headroom", "peak_interval", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "headroom", "lowest_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"current", "headroom", "peak_since_reset", ""}: {
		"reading_current_value",
		"reading_alarm",
		false,
	},
	{"frequency", "zero", "input", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		true,
	},
	{"frequency", "zero", "average", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "zero", "lowest_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "zero", "peak_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "zero", "lowest_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "zero", "peak_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "delta", "input", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		true,
	},
	{"frequency", "delta", "average", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "delta", "lowest_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "delta", "peak_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "delta", "lowest_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "delta", "peak_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "headroom", "input", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		true,
	},
	{"frequency", "headroom", "average", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "headroom", "lowest_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "headroom", "peak_interval", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "headroom", "lowest_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"frequency", "headroom", "peak_since_reset", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		false,
	},
	{"pressure", "zero", "input", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		true,
	},
	{"pressure", "zero", "average", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "zero", "lowest_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "zero", "peak_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "zero", "lowest_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "zero", "peak_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "delta", "input", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		true,
	},
	{"pressure", "delta", "average", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "delta", "lowest_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "delta", "peak_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "delta", "lowest_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "delta", "peak_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "headroom", "input", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		true,
	},
	{"pressure", "headroom", "average", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "headroom", "lowest_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "headroom", "peak_interval", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "headroom", "lowest_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"pressure", "headroom", "peak_since_reset", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "zero", "input", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		true,
	},
	{"liquid_level", "zero", "average", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "zero", "lowest_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "zero", "peak_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "zero", "lowest_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "zero", "peak_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "delta", "input", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		true,
	},
	{"liquid_level", "delta", "average", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "delta", "lowest_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "delta", "peak_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "delta", "lowest_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "delta", "peak_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "headroom", "input", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		true,
	},
	{"liquid_level", "headroom", "average", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "headroom", "lowest_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "headroom", "peak_interval", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "headroom", "lowest_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"liquid_level", "headroom", "peak_since_reset", ""}: {
		"reading_liquid_level_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "input", "fan"}: {
		"system_hw_sensor_fan_input",
		"system_hw_sensor_fan_alarm",
		true,
	},
	{"rotational_speed", "zero", "input", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		true,
	},
	{"rotational_speed", "zero", "average", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "lowest_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "peak_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "lowest_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "peak_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "delta", "input", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		true,
	},
	{"rotational_speed", "delta", "average", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "delta", "lowest_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "delta", "peak_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "delta", "lowest_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "delta", "peak_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "headroom", "input", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		true,
	},
	{"rotational_speed", "headroom", "average", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "headroom", "lowest_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "headroom", "peak_interval", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "headroom", "lowest_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "headroom", "peak_since_reset", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "zero", "input", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		true,
	},
	{"air_flow", "zero", "average", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "zero", "lowest_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "zero", "peak_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "zero", "lowest_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "zero", "peak_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "delta", "input", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		true,
	},
	{"air_flow", "delta", "average", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "delta", "lowest_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "delta", "peak_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "delta", "lowest_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "delta", "peak_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "headroom", "input", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		true,
	},
	{"air_flow", "headroom", "average", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "headroom", "lowest_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "headroom", "peak_interval", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "headroom", "lowest_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"air_flow", "headroom", "peak_since_reset", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "zero", "input", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		true,
	},
	{"liquid_flow", "zero", "average", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "zero", "lowest_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "zero", "peak_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "zero", "lowest_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "zero", "peak_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "delta", "input", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		true,
	},
	{"liquid_flow", "delta", "average", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "delta", "lowest_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "delta", "peak_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "delta", "lowest_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "delta", "peak_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "headroom", "input", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		true,
	},
	{"liquid_flow", "headroom", "average", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "headroom", "lowest_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "headroom", "peak_interval", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "headroom", "lowest_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"liquid_flow", "headroom", "peak_since_reset", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "zero", "input", "ambient_pressure"}: {
		"system_hw_sensor_pressure_input",
		"system_hw_sensor_pressure_alarm",
		true,
	},
	{"barometric_pressure", "zero", "average", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "zero", "lowest_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "zero", "peak_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "zero", "lowest_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "zero", "peak_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "delta", "input", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		true,
	},
	{"barometric_pressure", "delta", "average", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "delta", "lowest_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "delta", "peak_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "delta", "lowest_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "delta", "peak_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "headroom", "input", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		true,
	},
	{"barometric_pressure", "headroom", "average", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "headroom", "lowest_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "headroom", "peak_interval", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "headroom", "lowest_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"barometric_pressure", "headroom", "peak_since_reset", ""}: {
		"reading_barometric_pressure_value",
		"reading_alarm",
		false,
	},
	{"altitude", "zero", "input", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		true,
	},
	{"altitude", "zero", "average", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "zero", "lowest_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "zero", "peak_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "zero", "lowest_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "zero", "peak_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "delta", "input", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		true,
	},
	{"altitude", "delta", "average", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "delta", "lowest_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "delta", "peak_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "delta", "lowest_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "delta", "peak_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "headroom", "input", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		true,
	},
	{"altitude", "headroom", "average", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "headroom", "lowest_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "headroom", "peak_interval", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "headroom", "lowest_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"altitude", "headroom", "peak_since_reset", ""}: {
		"reading_altitude_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "input", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"percentage", "zero", "average", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "lowest_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "peak_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "lowest_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "peak_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "delta", "input", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"percentage", "delta", "average", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "delta", "lowest_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "delta", "peak_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "delta", "lowest_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "delta", "peak_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "headroom", "input", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"percentage", "headroom", "average", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "headroom", "lowest_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "headroom", "peak_interval", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "headroom", "lowest_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "headroom", "peak_since_reset", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "zero", "input", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		true,
	},
	{"absolute_humidity", "zero", "average", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "zero", "lowest_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "zero", "peak_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "zero", "lowest_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "zero", "peak_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "delta", "input", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		true,
	},
	{"absolute_humidity", "delta", "average", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "delta", "lowest_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "delta", "peak_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "delta", "lowest_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "delta", "peak_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "headroom", "input", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		true,
	},
	{"absolute_humidity", "headroom", "average", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "headroom", "lowest_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "headroom", "peak_interval", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "headroom", "lowest_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"absolute_humidity", "headroom", "peak_since_reset", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		false,
	},
	{"heat", "zero", "input", ""}:                  {"reading_heat_value", "reading_alarm", true},
	{"heat", "zero", "average", ""}:                {"reading_heat_value", "reading_alarm", false},
	{"heat", "zero", "lowest_interval", ""}:        {"reading_heat_value", "reading_alarm", false},
	{"heat", "zero", "peak_interval", ""}:          {"reading_heat_value", "reading_alarm", false},
	{"heat", "zero", "lowest_since_reset", ""}:     {"reading_heat_value", "reading_alarm", false},
	{"heat", "zero", "peak_since_reset", ""}:       {"reading_heat_value", "reading_alarm", false},
	{"heat", "delta", "input", ""}:                 {"reading_heat_value", "reading_alarm", true},
	{"heat", "delta", "average", ""}:               {"reading_heat_value", "reading_alarm", false},
	{"heat", "delta", "lowest_interval", ""}:       {"reading_heat_value", "reading_alarm", false},
	{"heat", "delta", "peak_interval", ""}:         {"reading_heat_value", "reading_alarm", false},
	{"heat", "delta", "lowest_since_reset", ""}:    {"reading_heat_value", "reading_alarm", false},
	{"heat", "delta", "peak_since_reset", ""}:      {"reading_heat_value", "reading_alarm", false},
	{"heat", "headroom", "input", ""}:              {"reading_heat_value", "reading_alarm", true},
	{"heat", "headroom", "average", ""}:            {"reading_heat_value", "reading_alarm", false},
	{"heat", "headroom", "lowest_interval", ""}:    {"reading_heat_value", "reading_alarm", false},
	{"heat", "headroom", "peak_interval", ""}:      {"reading_heat_value", "reading_alarm", false},
	{"heat", "headroom", "lowest_since_reset", ""}: {"reading_heat_value", "reading_alarm", false},
	{"heat", "headroom", "peak_since_reset", ""}:   {"reading_heat_value", "reading_alarm", false},
	{"linear_position", "zero", "input", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		true,
	},
	{"linear_position", "zero", "average", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "zero", "lowest_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "zero", "peak_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "zero", "lowest_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "zero", "peak_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "delta", "input", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		true,
	},
	{"linear_position", "delta", "average", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "delta", "lowest_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "delta", "peak_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "delta", "lowest_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "delta", "peak_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "headroom", "input", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		true,
	},
	{"linear_position", "headroom", "average", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "headroom", "lowest_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "headroom", "peak_interval", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "headroom", "lowest_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_position", "headroom", "peak_since_reset", ""}: {
		"reading_linear_position_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "zero", "input", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		true,
	},
	{"linear_velocity", "zero", "average", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "zero", "lowest_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "zero", "peak_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "zero", "lowest_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "zero", "peak_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "delta", "input", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		true,
	},
	{"linear_velocity", "delta", "average", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "delta", "lowest_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "delta", "peak_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "delta", "lowest_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "delta", "peak_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "headroom", "input", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		true,
	},
	{"linear_velocity", "headroom", "average", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "headroom", "lowest_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "headroom", "peak_interval", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "headroom", "lowest_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_velocity", "headroom", "peak_since_reset", ""}: {
		"reading_linear_velocity_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "zero", "input", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		true,
	},
	{"linear_acceleration", "zero", "average", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "zero", "lowest_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "zero", "peak_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "zero", "lowest_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "zero", "peak_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "delta", "input", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		true,
	},
	{"linear_acceleration", "delta", "average", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "delta", "lowest_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "delta", "peak_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "delta", "lowest_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "delta", "peak_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "headroom", "input", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		true,
	},
	{"linear_acceleration", "headroom", "average", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "headroom", "lowest_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "headroom", "peak_interval", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "headroom", "lowest_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"linear_acceleration", "headroom", "peak_since_reset", ""}: {
		"reading_linear_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "zero", "input", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		true,
	},
	{"rotational_position", "zero", "average", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "zero", "lowest_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "zero", "peak_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "zero", "lowest_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "zero", "peak_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "delta", "input", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		true,
	},
	{"rotational_position", "delta", "average", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "delta", "lowest_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "delta", "peak_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "delta", "lowest_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "delta", "peak_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "headroom", "input", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		true,
	},
	{"rotational_position", "headroom", "average", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "headroom", "lowest_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "headroom", "peak_interval", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "headroom", "lowest_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_position", "headroom", "peak_since_reset", ""}: {
		"reading_rotational_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "zero", "input", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		true,
	},
	{"rotational_velocity", "zero", "average", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "zero", "lowest_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "zero", "peak_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "zero", "lowest_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "zero", "peak_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "delta", "input", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		true,
	},
	{"rotational_velocity", "delta", "average", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "delta", "lowest_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "delta", "peak_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "delta", "lowest_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "delta", "peak_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "headroom", "input", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		true,
	},
	{"rotational_velocity", "headroom", "average", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "headroom", "lowest_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "headroom", "peak_interval", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "headroom", "lowest_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_velocity", "headroom", "peak_since_reset", ""}: {
		"reading_rotational_velocity_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "zero", "input", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		true,
	},
	{"rotational_acceleration", "zero", "average", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "zero", "lowest_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "zero", "peak_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "zero", "lowest_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "zero", "peak_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "delta", "input", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		true,
	},
	{"rotational_acceleration", "delta", "average", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "delta", "lowest_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "delta", "peak_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "delta", "lowest_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "delta", "peak_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "headroom", "input", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		true,
	},
	{"rotational_acceleration", "headroom", "average", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "headroom", "lowest_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "headroom", "peak_interval", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "headroom", "lowest_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"rotational_acceleration", "headroom", "peak_since_reset", ""}: {
		"reading_rotational_acceleration_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "zero", "input", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		true,
	},
	{"valve_position", "zero", "average", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "zero", "lowest_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "zero", "peak_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "zero", "lowest_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "zero", "peak_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "delta", "input", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		true,
	},
	{"valve_position", "delta", "average", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "delta", "lowest_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "delta", "peak_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "delta", "lowest_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "delta", "peak_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "headroom", "input", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		true,
	},
	{"valve_position", "headroom", "average", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "headroom", "lowest_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "headroom", "peak_interval", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "headroom", "lowest_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"valve_position", "headroom", "peak_since_reset", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		false,
	},
	{"rotational_speed", "zero", "speed_rpm", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		false,
	},
	{"apparent_power", "zero", "apparent_va", ""}: {
		"reading_apparent_power_value",
		"reading_alarm",
		false,
	},
	{"reactive_power", "zero", "reactive_var", ""}: {
		"reading_reactive_power_value",
		"reading_alarm",
		false,
	},
	{"apparent_energy", "zero", "apparent_kvah", ""}: {
		"reading_apparent_energy_value",
		"reading_alarm",
		false,
	},
	{"reactive_energy", "zero", "reactive_kvarh", ""}: {
		"reading_reactive_energy_value",
		"reading_alarm",
		false,
	},
	{"crest_factor", "zero", "crest_factor", ""}: {
		"reading_crest_factor_value",
		"reading_alarm",
		false,
	},
	{"phase_angle", "zero", "phase_angle_degrees", ""}: {
		"reading_phase_angle_value",
		"reading_alarm",
		false,
	},
	{"power_factor", "zero", "power_factor", ""}: {
		"reading_power_factor_value",
		"reading_alarm",
		false,
	},
	{"harmonic_distortion", "zero", "thd_percent", ""}: {
		"reading_harmonic_distortion_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "load_percent", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		false,
	},
	{"percentage", "zero", "speed", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"rotational_speed", "zero", "speed", ""}: {
		"reading_rotational_speed_value",
		"reading_alarm",
		true,
	},
	{"power", "zero", "power", ""}: {"reading_power_value", "reading_alarm", true},
	{"pressure", "zero", "pressure", ""}: {
		"reading_pressure_value",
		"reading_alarm",
		true,
	},
	{"liquid_flow", "zero", "flow", ""}: {
		"reading_liquid_flow_value",
		"reading_alarm",
		true,
	},
	{"valve_position", "zero", "position", ""}: {
		"reading_valve_position_value",
		"reading_alarm",
		true,
	},
	{"heat", "zero", "heat_removed", ""}:     {"reading_heat_value", "reading_alarm", true},
	{"voltage", "zero", "core_voltage", ""}:  {"reading_voltage_value", "reading_alarm", true},
	{"power", "zero", "input_power", ""}:     {"reading_power_value", "reading_alarm", true},
	{"power", "zero", "output_power", ""}:    {"reading_power_value", "reading_alarm", true},
	{"current", "zero", "input_current", ""}: {"reading_current_value", "reading_alarm", true},
	{"voltage", "zero", "input_voltage", ""}: {"reading_voltage_value", "reading_alarm", true},
	{"frequency", "zero", "frequency", ""}: {
		"reading_frequency_value",
		"reading_alarm",
		true,
	},
	{"temperature", "zero", "temperature", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		true,
	},
	{"percentage", "zero", "fan_speed", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"energy", "zero", "energy", ""}: {"reading_energy_value", "reading_alarm", true},
	{"percentage", "zero", "charge", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"percentage", "zero", "state_of_health", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"charge", "zero", "stored_charge", ""}: {"reading_charge_value", "reading_alarm", true},
	{"stored_energy", "zero", "stored_energy", ""}: {
		"reading_stored_energy_value",
		"reading_alarm",
		true,
	},
	{"current", "zero", "current", ""}: {"reading_current_value", "reading_alarm", true},
	{"voltage", "zero", "voltage", ""}: {"reading_voltage_value", "reading_alarm", true},
	{"temperature", "zero", "ambient_temperature", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		true,
	},
	{"temperature", "zero", "dew_point", ""}: {
		"reading_temperature_value",
		"reading_alarm",
		true,
	},
	{"humidity", "zero", "humidity", ""}: {
		"reading_humidity_value",
		"reading_alarm",
		true,
	},
	{"percentage", "zero", "power_load", ""}: {
		"reading_percentage_value",
		"reading_alarm",
		true,
	},
	{"air_flow", "zero", "airflow", ""}: {
		"reading_air_flow_value",
		"reading_alarm",
		true,
	},
	{"absolute_humidity", "zero", "absolute_humidity", ""}: {
		"reading_absolute_humidity_value",
		"reading_alarm",
		true,
	},
	{"power", "zero", "energy_rate", "energy_rate"}: {"reading_power_value", "reading_alarm", true},
}

func matchReadingType(sourceType, units string) (readingTypeDescriptor, bool) {
	value, ok := readingTypes[readingTypeKey{sourceType, units}]
	return value, ok
}

func matchFixedReadingFamily(family string) (readingTypeDescriptor, bool) {
	value, ok := fixedReadingFamilies[family]
	return value, ok
}

func matchReading(family, basis, role, semantic string) (readingDescriptor, bool) {
	key := readingKey{family, basis, role, semantic}
	if value, ok := readingDescriptors[key]; ok {
		return value, true
	}
	key.SemanticClass = ""
	value, ok := readingDescriptors[key]
	return value, ok
}
