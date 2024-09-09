// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioUpsStatus = module.Priority + iota
	prioUpsSelftest

	prioUpsBatteryCharge
	prioUpsBatteryTimeRemaining
	prioUpsBatteryTimeSinceReplacement
	prioUpsBatteryVoltage

	prioUpsLoadCapacityUtilization
	prioUpsLoad

	prioUpsTemperature

	prioUpsInputVoltage
	prioUpsInputFrequency

	prioUpsOutputVoltage
)

var charts = module.Charts{
	statusChart.Copy(),
	selftestChart.Copy(),

	batteryChargeChart.Copy(),
	batteryTimeRemainingChart.Copy(),
	batteryTimeSinceReplacementChart.Copy(),
	batteryVoltageChart.Copy(),

	loadCapacityUtilizationChart.Copy(),
	loadChart.Copy(),

	internalTemperatureChart.Copy(),

	inputVoltageChart.Copy(),
	inputFrequencyChart.Copy(),

	outputVoltageChart.Copy(),
}

// Status
var (
	statusChart = func() module.Chart {
		chart := module.Chart{
			ID:       "ups_status",
			Title:    "UPS Status",
			Units:    "status",
			Fam:      "status",
			Ctx:      "apcupsd.ups_status",
			Priority: prioUpsStatus,
			Type:     module.Line,
		}
		for _, v := range upsStatuses {
			chart.Dims = append(chart.Dims, &module.Dim{ID: "status_" + v, Name: v})
		}
		return chart
	}()
	selftestChart = func() module.Chart {
		chart := module.Chart{
			ID:       "ups_selftest",
			Title:    "UPS Self-Test Status",
			Units:    "status",
			Fam:      "status",
			Ctx:      "apcupsd.ups_selftest",
			Priority: prioUpsSelftest,
			Type:     module.Line,
		}
		for _, v := range upsSelftestStatuses {
			chart.Dims = append(chart.Dims, &module.Dim{ID: "selftest_" + v, Name: v})
		}
		return chart
	}()
)

// Battery
var (
	batteryChargeChart = module.Chart{
		ID:       "ups_battery_charge",
		Title:    "UPS Battery Charge",
		Units:    "percent",
		Fam:      "battery",
		Ctx:      "apcupsd.ups_battery_charge",
		Priority: prioUpsBatteryCharge,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "battery_charge", Name: "charge", Div: precision},
		},
	}
	batteryTimeRemainingChart = module.Chart{
		ID:       "ups_battery_time_remaining",
		Title:    "UPS Estimated Runtime on Battery",
		Units:    "seconds",
		Fam:      "battery",
		Ctx:      "apcupsd.ups_battery_time_remaining",
		Priority: prioUpsBatteryTimeRemaining,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "timeleft", Name: "timeleft", Div: precision},
		},
	}
	batteryTimeSinceReplacementChart = module.Chart{
		ID:       "ups_battery_time_since_replacement",
		Title:    "UPS Time Since Battery Replacement",
		Units:    "seconds",
		Fam:      "battery",
		Ctx:      "apcupsd.ups_battery_time_since_replacement",
		Priority: prioUpsBatteryTimeSinceReplacement,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "battery_seconds_since_replacement", Name: "since_replacement"},
		},
	}
	batteryVoltageChart = module.Chart{
		ID:       "ups_battery_voltage",
		Title:    "UPS Battery Voltage",
		Units:    "Volts",
		Fam:      "battery",
		Ctx:      "apcupsd.ups_battery_voltage",
		Priority: prioUpsBatteryVoltage,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "battery_voltage", Name: "voltage", Div: precision},
			{ID: "battery_voltage_nominal", Name: "nominal_voltage", Div: precision},
		},
	}
)

// Load
var (
	loadCapacityUtilizationChart = module.Chart{
		ID:       "ups_load_capacity_utilization",
		Title:    "UPS Load Capacity Utilization",
		Units:    "percent",
		Fam:      "load",
		Ctx:      "apcupsd.ups_load_capacity_utilization",
		Priority: prioUpsLoadCapacityUtilization,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "load_percent", Name: "load", Div: precision},
		},
	}
	loadChart = module.Chart{
		ID:       "ups_load",
		Title:    "UPS Load",
		Units:    "Watts",
		Fam:      "load",
		Ctx:      "apcupsd.ups_load",
		Priority: prioUpsLoad,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "load", Name: "load", Div: precision},
		},
	}
)

// Temperature
var (
	internalTemperatureChart = module.Chart{
		ID:       "ups_temperature",
		Title:    "UPS Internal Temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "apcupsd.ups_temperature",
		Priority: prioUpsTemperature,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "itemp", Name: "temperature", Div: precision},
		},
	}
)

// Input
var (
	inputVoltageChart = module.Chart{
		ID:       "ups_input_voltage",
		Title:    "UPS Input Voltage",
		Units:    "Volts",
		Fam:      "input",
		Ctx:      "apcupsd.ups_input_voltage",
		Priority: prioUpsInputVoltage,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "input_voltage", Name: "voltage", Div: precision},
			{ID: "input_voltage_min", Name: "min_voltage", Div: precision},
			{ID: "input_voltage_max", Name: "max_voltage", Div: precision},
		},
	}
	inputFrequencyChart = module.Chart{
		ID:       "ups_input_frequency",
		Title:    "UPS Input Frequency",
		Units:    "Hz",
		Fam:      "input",
		Ctx:      "apcupsd.ups_input_frequency",
		Priority: prioUpsInputFrequency,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "input_frequency", Name: "frequency", Div: precision},
		},
	}
)

// Output
var (
	outputVoltageChart = module.Chart{
		ID:       "ups_output_voltage",
		Title:    "UPS Output Voltage",
		Units:    "Volts",
		Fam:      "output",
		Ctx:      "apcupsd.ups_output_voltage",
		Priority: prioUpsOutputVoltage,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "output_voltage", Name: "voltage", Div: precision},
		},
	}
)
