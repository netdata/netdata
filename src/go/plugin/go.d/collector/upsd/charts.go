// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioUpsLoad = module.Priority + iota
	prioUpsLoadUsage
	prioUpsStatus
	prioUpsTemperature

	prioBatteryCharge
	prioBatteryEstimatedRuntime
	prioBatteryVoltage
	prioBatteryVoltageNominal

	prioInputVoltage
	prioInputVoltageNominal
	prioInputCurrent
	prioInputCurrentNominal
	prioInputFrequency
	prioInputFrequencyNominal

	prioOutputVoltage
	prioOutputVoltageNominal
	prioOutputCurrent
	prioOutputCurrentNominal
	prioOutputFrequency
	prioOutputFrequencyNominal
)

var upsChartsTmpl = module.Charts{
	upsLoadChartTmpl.Copy(),
	upsLoadUsageChartTmpl.Copy(),
	upsStatusChartTmpl.Copy(),
	upsTemperatureChartTmpl.Copy(),

	upsBatteryChargePercentChartTmpl.Copy(),
	upsBatteryEstimatedRuntimeChartTmpl.Copy(),
	upsBatteryVoltageChartTmpl.Copy(),
	upsBatteryVoltageNominalChartTmpl.Copy(),

	upsInputVoltageChartTmpl.Copy(),
	upsInputVoltageNominalChartTmpl.Copy(),
	upsInputCurrentChartTmpl.Copy(),
	upsInputCurrentNominalChartTmpl.Copy(),
	upsInputFrequencyChartTmpl.Copy(),
	upsInputFrequencyNominalChartTmpl.Copy(),

	upsOutputVoltageChartTmpl.Copy(),
	upsOutputVoltageNominalChartTmpl.Copy(),
	upsOutputCurrentChartTmpl.Copy(),
	upsOutputCurrentNominalChartTmpl.Copy(),
	upsOutputFrequencyChartTmpl.Copy(),
	upsOutputFrequencyNominalChartTmpl.Copy(),
}

var (
	upsLoadChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.load_percentage",
		Title:    "UPS load",
		Units:    "percentage",
		Fam:      "ups",
		Ctx:      "upsd.ups_load",
		Priority: prioUpsLoad,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "ups_%s_ups.load", Name: "load", Div: varPrecision},
		},
	}
	upsLoadUsageChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.load_usage",
		Title:    "UPS load usage (power output)",
		Units:    "Watts",
		Fam:      "ups",
		Ctx:      "upsd.ups_load_usage",
		Priority: prioUpsLoadUsage,
		Dims: module.Dims{
			{ID: "ups_%s_ups.load.usage", Name: "load_usage", Div: varPrecision},
		},
	}
	upsStatusChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.status",
		Title:    "UPS status",
		Units:    "status",
		Fam:      "ups",
		Ctx:      "upsd.ups_status",
		Priority: prioUpsStatus,
		Dims: module.Dims{
			{ID: "ups_%s_ups.status.OL", Name: "on_line"},
			{ID: "ups_%s_ups.status.OB", Name: "on_battery"},
			{ID: "ups_%s_ups.status.LB", Name: "low_battery"},
			{ID: "ups_%s_ups.status.HB", Name: "high_battery"},
			{ID: "ups_%s_ups.status.RB", Name: "replace_battery"},
			{ID: "ups_%s_ups.status.CHRG", Name: "charging"},
			{ID: "ups_%s_ups.status.DISCHRG", Name: "discharging"},
			{ID: "ups_%s_ups.status.BYPASS", Name: "bypass"},
			{ID: "ups_%s_ups.status.CAL", Name: "calibration"},
			{ID: "ups_%s_ups.status.OFF", Name: "offline"},
			{ID: "ups_%s_ups.status.OVER", Name: "overloaded"},
			{ID: "ups_%s_ups.status.TRIM", Name: "trim_input_voltage"},
			{ID: "ups_%s_ups.status.BOOST", Name: "boost_input_voltage"},
			{ID: "ups_%s_ups.status.FSD", Name: "forced_shutdown"},
			{ID: "ups_%s_ups.status.other", Name: "other"},
		},
	}
	upsTemperatureChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.temperature",
		Title:    "UPS temperature",
		Units:    "Celsius",
		Fam:      "ups",
		Ctx:      "upsd.ups_temperature",
		Priority: prioUpsTemperature,
		Dims: module.Dims{
			{ID: "ups_%s_ups.temperature", Name: "temperature", Div: varPrecision},
		},
	}
)

var (
	upsBatteryChargePercentChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.battery_charge_percentage",
		Title:    "UPS Battery charge",
		Units:    "percentage",
		Fam:      "battery",
		Ctx:      "upsd.ups_battery_charge",
		Priority: prioBatteryCharge,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "ups_%s_battery.charge", Name: "charge", Div: varPrecision},
		},
	}
	upsBatteryEstimatedRuntimeChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.battery_estimated_runtime",
		Title:    "UPS Battery estimated runtime",
		Units:    "seconds",
		Fam:      "battery",
		Ctx:      "upsd.ups_battery_estimated_runtime",
		Priority: prioBatteryEstimatedRuntime,
		Dims: module.Dims{
			{ID: "ups_%s_battery.runtime", Name: "runtime", Div: varPrecision},
		},
	}
	upsBatteryVoltageChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.battery_voltage",
		Title:    "UPS Battery voltage",
		Units:    "Volts",
		Fam:      "battery",
		Ctx:      "upsd.ups_battery_voltage",
		Priority: prioBatteryVoltage,
		Dims: module.Dims{
			{ID: "ups_%s_battery.voltage", Name: "voltage", Div: varPrecision},
		},
	}
	upsBatteryVoltageNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.battery_voltage_nominal",
		Title:    "UPS Battery voltage nominal",
		Units:    "Volts",
		Fam:      "battery",
		Ctx:      "upsd.ups_battery_voltage_nominal",
		Priority: prioBatteryVoltageNominal,
		Dims: module.Dims{
			{ID: "ups_%s_battery.voltage.nominal", Name: "nominal_voltage", Div: varPrecision},
		},
	}
)

var (
	upsInputVoltageChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_voltage",
		Title:    "UPS Input voltage",
		Units:    "Volts",
		Fam:      "input",
		Ctx:      "upsd.ups_input_voltage",
		Priority: prioInputVoltage,
		Dims: module.Dims{
			{ID: "ups_%s_input.voltage", Name: "voltage", Div: varPrecision},
		},
	}
	upsInputVoltageNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_voltage_nominal",
		Title:    "UPS Input voltage nominal",
		Units:    "Volts",
		Fam:      "input",
		Ctx:      "upsd.ups_input_voltage_nominal",
		Priority: prioInputVoltageNominal,
		Dims: module.Dims{
			{ID: "ups_%s_input.voltage.nominal", Name: "nominal_voltage", Div: varPrecision},
		},
	}
	upsInputCurrentChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_current",
		Title:    "UPS Input current",
		Units:    "Ampere",
		Fam:      "input",
		Ctx:      "upsd.ups_input_current",
		Priority: prioInputCurrent,
		Dims: module.Dims{
			{ID: "ups_%s_input.current", Name: "current", Div: varPrecision},
		},
	}
	upsInputCurrentNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_current_nominal",
		Title:    "UPS Input current nominal",
		Units:    "Ampere",
		Fam:      "input",
		Ctx:      "upsd.ups_input_current_nominal",
		Priority: prioInputCurrentNominal,
		Dims: module.Dims{
			{ID: "ups_%s_input.current.nominal", Name: "nominal_current", Div: varPrecision},
		},
	}
	upsInputFrequencyChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_frequency",
		Title:    "UPS Input frequency",
		Units:    "Hz",
		Fam:      "input",
		Ctx:      "upsd.ups_input_frequency",
		Priority: prioInputFrequency,
		Dims: module.Dims{
			{ID: "ups_%s_input.frequency", Name: "frequency", Div: varPrecision},
		},
	}
	upsInputFrequencyNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.input_frequency_nominal",
		Title:    "UPS Input frequency nominal",
		Units:    "Hz",
		Fam:      "input",
		Ctx:      "upsd.ups_input_frequency_nominal",
		Priority: prioInputFrequencyNominal,
		Dims: module.Dims{
			{ID: "ups_%s_input.frequency.nominal", Name: "nominal_frequency", Div: varPrecision},
		},
	}
)

var (
	upsOutputVoltageChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_voltage",
		Title:    "UPS Output voltage",
		Units:    "Volts",
		Fam:      "output",
		Ctx:      "upsd.ups_output_voltage",
		Priority: prioOutputVoltage,
		Dims: module.Dims{
			{ID: "ups_%s_output.voltage", Name: "voltage", Div: varPrecision},
		},
	}
	upsOutputVoltageNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_voltage_nominal",
		Title:    "UPS Output voltage nominal",
		Units:    "Volts",
		Fam:      "output",
		Ctx:      "upsd.ups_output_voltage_nominal",
		Priority: prioOutputVoltageNominal,
		Dims: module.Dims{
			{ID: "ups_%s_output.voltage.nominal", Name: "nominal_voltage", Div: varPrecision},
		},
	}
	upsOutputCurrentChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_current",
		Title:    "UPS Output current",
		Units:    "Ampere",
		Fam:      "output",
		Ctx:      "upsd.ups_output_current",
		Priority: prioOutputCurrent,
		Dims: module.Dims{
			{ID: "ups_%s_output.current", Name: "current", Div: varPrecision},
		},
	}
	upsOutputCurrentNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_current_nominal",
		Title:    "UPS Output current nominal",
		Units:    "Ampere",
		Fam:      "output",
		Ctx:      "upsd.ups_output_current_nominal",
		Priority: prioOutputCurrentNominal,
		Dims: module.Dims{
			{ID: "ups_%s_output.current.nominal", Name: "nominal_current", Div: varPrecision},
		},
	}
	upsOutputFrequencyChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_frequency",
		Title:    "UPS Output frequency",
		Units:    "Hz",
		Fam:      "output",
		Ctx:      "upsd.ups_output_frequency",
		Priority: prioOutputFrequency,
		Dims: module.Dims{
			{ID: "ups_%s_output.frequency", Name: "frequency", Div: varPrecision},
		},
	}
	upsOutputFrequencyNominalChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "%s.output_frequency_nominal",
		Title:    "UPS Output frequency nominal",
		Units:    "Hz",
		Fam:      "output",
		Ctx:      "upsd.ups_output_frequency_nominal",
		Priority: prioOutputFrequencyNominal,
		Dims: module.Dims{
			{ID: "ups_%s_output.frequency.nominal", Name: "nominal_frequency", Div: varPrecision},
		},
	}
)

func (c *Collector) addUPSCharts(ups upsUnit) {
	charts := upsChartsTmpl.Copy()

	var removed []string
	for _, v := range []struct{ v, id string }{
		{varUpsLoad, upsLoadChartTmpl.ID},
		{varUpsLoad, upsLoadUsageChartTmpl.ID},

		{varBatteryVoltage, upsBatteryVoltageChartTmpl.ID},
		{varBatteryVoltageNominal, upsBatteryVoltageNominalChartTmpl.ID},

		{varUpsTemperature, upsTemperatureChartTmpl.ID},

		{varInputVoltage, upsInputVoltageChartTmpl.ID},
		{varInputVoltageNominal, upsInputVoltageNominalChartTmpl.ID},
		{varInputCurrent, upsInputCurrentChartTmpl.ID},
		{varInputCurrentNominal, upsInputCurrentNominalChartTmpl.ID},
		{varInputFrequency, upsInputFrequencyChartTmpl.ID},
		{varInputFrequencyNominal, upsInputFrequencyNominalChartTmpl.ID},

		{varOutputVoltage, upsOutputVoltageChartTmpl.ID},
		{varOutputVoltageNominal, upsOutputVoltageNominalChartTmpl.ID},
		{varOutputCurrent, upsOutputCurrentChartTmpl.ID},
		{varOutputCurrentNominal, upsOutputCurrentNominalChartTmpl.ID},
		{varOutputFrequency, upsOutputFrequencyChartTmpl.ID},
		{varOutputFrequencyNominal, upsOutputFrequencyNominalChartTmpl.ID},
	} {
		if !hasVar(ups.vars, v.v) {
			removed = append(removed, v.v)
			_ = charts.Remove(v.id)
		}
	}

	c.Debugf("UPS '%s' no metrics: %v", ups.name, removed)

	name := cleanUpsName(ups.name)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "ups_name", Value: ups.name},
			{Key: "battery_type", Value: ups.vars[varBatteryType]},
			{Key: "device_model", Value: ups.vars[varDeviceModel]},
			{Key: "device_serial", Value: ups.vars[varDeviceSerial]},
			{Key: "device_manufacturer", Value: ups.vars[varDeviceMfr]},
			{Key: "device_type", Value: ups.vars[varDeviceType]},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ups.name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeUPSCharts(name string) {
	name = cleanUpsName(name)
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, name) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanUpsName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}
