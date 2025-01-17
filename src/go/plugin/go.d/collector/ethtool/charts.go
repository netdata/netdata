// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioModuleReceiverPowerDbm = module.Priority + iota
	prioModuleLaserOutputPowerDbm
	prioModuleLaserBiasCurrent
	prioModuleTemperatureC
	prioModuleVoltage
)

var ifaceModuleEepromCharts = module.Charts{
	ifaceModuleReceiverPowerDbmChartTmpl.Copy(),
	ifaceModuleLaserPowerDbmChartTmpl.Copy(),
	ifaceModuleLaserBiasCurrentChartTmpl.Copy(),
	ifaceModuleTempCelsiusChartTmpl.Copy(),
	ifaceModuleVoltageChartTmpl.Copy(),
}

var (
	ifaceModuleReceiverPowerDbmChartTmpl = module.Chart{
		ID:       "iface_%s_module_receiver_power_dbm",
		Title:    "Module Receiver Signal Average Optical Power",
		Units:    "dBm",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_receiver_signal_power",
		Priority: prioModuleReceiverPowerDbm,
		Dims: module.Dims{
			{ID: "iface_%s_receiver_signal_average_optical_power_dbm", Name: "rx_power", Div: precision},
		},
	}
	ifaceModuleLaserPowerDbmChartTmpl = module.Chart{
		ID:       "iface_%s_module_laser_output_power_dbm",
		Title:    "Module Laser Output Power",
		Units:    "dBm",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_laser_output_power",
		Priority: prioModuleLaserOutputPowerDbm,
		Dims: module.Dims{
			{ID: "iface_%s_laser_output_power_dbm", Name: "tx_power", Div: precision},
		},
	}
	ifaceModuleLaserBiasCurrentChartTmpl = module.Chart{
		ID:       "iface_%s_module_laser_bias_current",
		Title:    "Module Laser Bias Current",
		Units:    "mA",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_laser_bias_current",
		Priority: prioModuleLaserBiasCurrent,
		Dims: module.Dims{
			{ID: "iface_%s_laser_bias_current_ma", Name: "bias_current", Div: precision},
		},
	}
	ifaceModuleTempCelsiusChartTmpl = module.Chart{
		ID:       "iface_%s_module_temperature_c",
		Title:    "Module Temperature",
		Units:    "Celsius",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_temperature",
		Priority: prioModuleTemperatureC,
		Dims: module.Dims{
			{ID: "iface_%s_module_temperature_c", Name: "temperature", Div: precision},
		},
	}
	ifaceModuleVoltageChartTmpl = module.Chart{
		ID:       "iface_%s_module_voltage",
		Title:    "Module Voltage",
		Units:    "Volts",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_voltage",
		Priority: prioModuleVoltage,
		Dims: module.Dims{
			{ID: "iface_%s_module_voltage_v", Name: "voltage", Div: precision},
		},
	}
)

func (c *Collector) addModuleEepromCharts(iface string, eeprom *moduleEeprom) {
	if eeprom == nil || eeprom.ddm == nil {
		return
	}

	charts := ifaceModuleEepromCharts.Copy()

	if eeprom.ddm.laserBiasMA == nil {
		_ = charts.Remove(ifaceModuleLaserBiasCurrentChartTmpl.ID)
	}
	if eeprom.ddm.laserPowerDBM == nil {
		_ = charts.Remove(ifaceModuleLaserPowerDbmChartTmpl.ID)
	}
	if eeprom.ddm.rxSignalPowerDBM == nil {
		_ = charts.Remove(ifaceModuleLaserPowerDbmChartTmpl.ID)
	}
	if eeprom.ddm.tempC == nil {
		_ = charts.Remove(ifaceModuleTempCelsiusChartTmpl.ID)
	}
	if eeprom.ddm.voltageV == nil {
		_ = charts.Remove(ifaceModuleVoltageChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, iface)
		chart.Labels = []module.Label{
			{Key: "iface", Value: iface},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, iface)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add chart for interfce '%s': %v", iface, err)
	}
}
