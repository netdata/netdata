// SPDX-License-Identifier: GPL-3.0-or-later

package registry

var readingTypeSpecs = []ReadingTypeSpec{
	{0, "Temperature", []string{"Cel"}, "temperature", "Celsius", Identity},
	{1, "Humidity", []string{"%"}, "humidity", "percentage", Identity},
	{2, "Power", []string{"W"}, "power", "watts", Identity},
	{3, "EnergykWh", []string{"kW.h"}, "energy", "joules", Rational{Num: 3_600_000, Den: 1}},
	{4, "EnergyJoules", []string{"J"}, "energy", "joules", Identity},
	{5, "EnergyWh", []string{"W.h"}, "energy", "joules", Rational{Num: 3_600, Den: 1}},
	{6, "ChargeAh", []string{"A.h"}, "charge", "ampere-hours", Identity},
	{7, "Voltage", []string{"V"}, "voltage", "volts", Identity},
	{8, "Current", []string{"A"}, "current", "amperes", Identity},
	{9, "Frequency", []string{"Hz"}, "frequency", "hertz", Identity},
	{10, "Pressure", []string{"Pa"}, "pressure", "pascals", Identity},
	{11, "PressurekPa", []string{"kPa"}, "pressure", "pascals", Rational{Num: 1_000, Den: 1}},
	{12, "PressurePa", []string{"Pa"}, "pressure", "pascals", Identity},
	{13, "LiquidLevel", []string{"cm"}, "liquid_level", "meters", Rational{Num: 1, Den: 100}},
	{14, "Rotational", []string{"{rev}/min", "RPM"}, "rotational_speed", "RPM", Identity},
	{15, "AirFlow", []string{"[ft_i]3/min"}, "air_flow", "cubic-meters/minute", Rational{Num: 28_316_846_592, Den: 1_000_000_000_000}},
	{16, "AirFlowCMM", []string{"m3/min"}, "air_flow", "cubic-meters/minute", Identity},
	{17, "LiquidFlow", []string{"L/s"}, "liquid_flow", "liters/minute", Rational{Num: 60, Den: 1}},
	{18, "LiquidFlowLPM", []string{"L/min"}, "liquid_flow", "liters/minute", Identity},
	{19, "Barometric", []string{"mm[Hg]"}, "barometric_pressure", "pascals", Rational{Num: 133_322_387_415, Den: 1_000_000_000}},
	{20, "Altitude", []string{"m"}, "altitude", "meters", Identity},
	{21, "Percent", []string{"%"}, "percentage", "percentage", Identity},
	{22, "AbsoluteHumidity", []string{"g/m3"}, "absolute_humidity", "grams/cubic-meter", Identity},
	{23, "Heat", []string{"kW"}, "heat", "watts", Rational{Num: 1_000, Den: 1}},
	{24, "LinearPosition", []string{"m"}, "linear_position", "meters", Identity},
	{25, "LinearVelocity", []string{"m/s"}, "linear_velocity", "meters/second", Identity},
	{26, "LinearAcceleration", []string{"m/s2"}, "linear_acceleration", "meters/second2", Identity},
	{27, "RotationalPosition", []string{"rad"}, "rotational_position", "radians", Identity},
	{28, "RotationalVelocity", []string{"rad/s"}, "rotational_velocity", "radians/second", Identity},
	{29, "RotationalAcceleration", []string{"rad/s2"}, "rotational_acceleration", "radians/second2", Identity},
	{30, "Valve", []string{"%"}, "valve_position", "percentage", Identity},
}

var readingRoleSpecs = []ReadingRoleSpec{
	{0, "input", ExposureOperationalReading, true},
	{1, "average", ExposureOperationalReading, false},
	{2, "lowest_interval", ExposureOperationalReading, false},
	{3, "peak_interval", ExposureOperationalReading, false},
	{4, "lowest_since_reset", ExposureOperationalReading, false},
	{5, "peak_since_reset", ExposureOperationalReading, false},
	{6, "reading_range_min", ExposureInventoryOnly, false},
	{7, "reading_range_max", ExposureInventoryOnly, false},
	{8, "minimum_allowable", ExposureInventoryOnly, false},
	{9, "maximum_allowable", ExposureInventoryOnly, false},
	{10, "adjusted_minimum_allowable", ExposureInventoryOnly, false},
	{11, "adjusted_maximum_allowable", ExposureInventoryOnly, false},
}

type readingSurfaceTemplate struct {
	Family         string
	Units          string
	Role           string
	Exposure       Exposure
	Primary        bool
	AggregateKinds []Kind
	Histogram      string
}

func auxiliaryReading(
	family, units, role string,
	aggregateKinds []Kind,
	histogram string,
) readingSurfaceTemplate {
	return readingSurfaceTemplate{
		Family: family, Units: units, Role: role,
		Exposure: ExposureOperationalReading, Primary: false,
		AggregateKinds: aggregateKinds, Histogram: histogram,
	}
}

func primaryReading(
	family, units, role string,
	aggregateKinds []Kind,
	histogram string,
) readingSurfaceTemplate {
	return readingSurfaceTemplate{
		Family: family, Units: units, Role: role,
		Exposure: ExposureOperationalReading, Primary: true,
		AggregateKinds: aggregateKinds, Histogram: histogram,
	}
}

// fixedReadingSurfaces are standard excerpt semantics which do not share the
// generic Sensor role vocabulary. Source-path adapters are kept separately in
// the collector because they also describe graph acquisition.
var fixedReadingSurfaces = []readingSurfaceTemplate{
	auxiliaryReading("rotational_speed", "RPM", "speed_rpm", []Kind{"chassis"}, ""),
	auxiliaryReading("apparent_power", "volt-amperes", "apparent_va", []Kind{"chassis"}, ""),
	auxiliaryReading("reactive_power", "vars", "reactive_var", []Kind{"chassis"}, ""),
	auxiliaryReading("apparent_energy", "joule-equivalent", "apparent_kvah", []Kind{"chassis"}, ""),
	auxiliaryReading("reactive_energy", "joule-equivalent", "reactive_kvarh", []Kind{"chassis"}, ""),
	auxiliaryReading("crest_factor", "ratio", "crest_factor", []Kind{"chassis"}, ""),
	auxiliaryReading("phase_angle", "degrees", "phase_angle_degrees", []Kind{"chassis"}, ""),
	auxiliaryReading("power_factor", "ratio", "power_factor", []Kind{"chassis"}, ""),
	auxiliaryReading("harmonic_distortion", "percentage", "thd_percent", []Kind{"chassis"}, "percentage"),
	auxiliaryReading("percentage", "percentage", "load_percent", []Kind{"chassis"}, "percentage"),
	primaryReading("percentage", "percentage", "speed", []Kind{"thermal_subsystem", "chassis"}, "percentage"),
	primaryReading("rotational_speed", "RPM", "speed", []Kind{"thermal_subsystem", "chassis"}, ""),
	primaryReading("power", "watts", "power", []Kind{"thermal_subsystem", "chassis"}, ""),
	primaryReading("pressure", "pascals", "pressure", []Kind{"thermal_subsystem", "chassis"}, ""),
	primaryReading("liquid_flow", "liters/minute", "flow", []Kind{"thermal_subsystem", "chassis"}, ""),
	primaryReading("valve_position", "percentage", "position", []Kind{"thermal_subsystem", "chassis"}, "percentage"),
	primaryReading("heat", "watts", "heat_removed", []Kind{"thermal_subsystem", "chassis"}, ""),
	primaryReading("voltage", "volts", "core_voltage", []Kind{"system", "chassis"}, ""),
	primaryReading("power", "watts", "input_power", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("power", "watts", "output_power", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("current", "amperes", "input_current", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("voltage", "volts", "input_voltage", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("frequency", "hertz", "frequency", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("temperature", "Celsius", "temperature", []Kind{"power_subsystem", "chassis"}, "temperature"),
	primaryReading("percentage", "percentage", "fan_speed", []Kind{"power_subsystem", "chassis"}, "percentage"),
	primaryReading("energy", "joules", "energy", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("percentage", "percentage", "charge", []Kind{"power_subsystem", "chassis"}, "percentage"),
	primaryReading("percentage", "percentage", "state_of_health", []Kind{"power_subsystem", "chassis"}, "percentage"),
	primaryReading("charge", "ampere-hours", "stored_charge", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("stored_energy", "watt-hours", "stored_energy", []Kind{"power_subsystem", "chassis"}, ""),
	primaryReading("current", "amperes", "current", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, ""),
	primaryReading("voltage", "volts", "voltage", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, ""),
	primaryReading("temperature", "Celsius", "ambient_temperature", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, "temperature"),
	primaryReading("temperature", "Celsius", "dew_point", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, "temperature"),
	primaryReading("temperature", "Celsius", "temperature", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, "temperature"),
	primaryReading("humidity", "percentage", "humidity", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, "percentage"),
	primaryReading("percentage", "percentage", "power_load", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, "percentage"),
	primaryReading("air_flow", "cubic-meters/minute", "airflow", []Kind{"chassis"}, ""),
	primaryReading("absolute_humidity", "grams/cubic-meter", "absolute_humidity", []Kind{"system", "chassis", "storage", "network_adapter", "network_interface"}, ""),
}

var histogramSpecs = []HistogramSpec{
	{
		ID: "temperature",
		Buckets: []HistogramBucket{
			{ID: "lt_40", UpperExclusive: new(float64(40))},
			{ID: "40_50", UpperExclusive: new(float64(50))},
			{ID: "50_60", UpperExclusive: new(float64(60))},
			{ID: "60_70", UpperExclusive: new(float64(70))},
			{ID: "70_80", UpperExclusive: new(float64(80))},
			{ID: "80_85", UpperExclusive: new(float64(85))},
			{ID: "85_90", UpperExclusive: new(float64(90))},
			{ID: "90_95", UpperExclusive: new(float64(95))},
			{ID: "95_100", UpperExclusive: new(float64(100))},
			{ID: "ge_100"},
		},
	},
	{
		ID: "percentage",
		Buckets: []HistogramBucket{
			{ID: "lt_0", UpperExclusive: new(float64(0))},
			{ID: "0_10", UpperExclusive: new(float64(10))},
			{ID: "10_20", UpperExclusive: new(float64(20))},
			{ID: "20_30", UpperExclusive: new(float64(30))},
			{ID: "30_40", UpperExclusive: new(float64(40))},
			{ID: "40_50", UpperExclusive: new(float64(50))},
			{ID: "50_60", UpperExclusive: new(float64(60))},
			{ID: "60_70", UpperExclusive: new(float64(70))},
			{ID: "70_80", UpperExclusive: new(float64(80))},
			{ID: "80_90", UpperExclusive: new(float64(90))},
			{ID: "90_100", UpperExclusive: new(float64(100)), UpperInclusive: true},
			{ID: "gt_100"},
		},
	},
	{
		ID: "range_percentage",
		Buckets: []HistogramBucket{
			{ID: "lt_0", UpperExclusive: new(float64(0))},
			{ID: "0_10", UpperExclusive: new(float64(10))},
			{ID: "10_20", UpperExclusive: new(float64(20))},
			{ID: "20_30", UpperExclusive: new(float64(30))},
			{ID: "30_40", UpperExclusive: new(float64(40))},
			{ID: "40_50", UpperExclusive: new(float64(50))},
			{ID: "50_60", UpperExclusive: new(float64(60))},
			{ID: "60_70", UpperExclusive: new(float64(70))},
			{ID: "70_80", UpperExclusive: new(float64(80))},
			{ID: "80_90", UpperExclusive: new(float64(90))},
			{ID: "90_100", UpperExclusive: new(float64(100)), UpperInclusive: true},
			{ID: "gt_100"},
		},
	},
}
