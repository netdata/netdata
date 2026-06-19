// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioModuleReceiverPowerDbm = collectorapi.Priority + iota
	prioModuleLaserOutputPowerDbm
	prioModuleLaserBiasCurrent
	prioModuleTemperatureC
	prioModuleVoltage
)

var ifaceModuleEepromCharts = collectorapi.Charts{
	ifaceModuleReceiverPowerDbmChartTmpl.Copy(),
	ifaceModuleLaserPowerDbmChartTmpl.Copy(),
	ifaceModuleLaserBiasCurrentChartTmpl.Copy(),
	ifaceModuleTempCelsiusChartTmpl.Copy(),
	ifaceModuleVoltageChartTmpl.Copy(),
}

var (
	ifaceModuleReceiverPowerDbmChartTmpl = collectorapi.Chart{
		ID:       "iface_%s_module_receiver_power_dbm",
		Title:    "CollectorV1 Receiver Signal Average Optical Power",
		Units:    "dBm",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_receiver_signal_power",
		Priority: prioModuleReceiverPowerDbm,
		Dims: collectorapi.Dims{
			{ID: "iface_%s_receiver_signal_average_optical_power_dbm", Name: "rx_power", Div: precision},
		},
	}
	ifaceModuleLaserPowerDbmChartTmpl = collectorapi.Chart{
		ID:       "iface_%s_module_laser_output_power_dbm",
		Title:    "CollectorV1 Laser Output Power",
		Units:    "dBm",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_laser_output_power",
		Priority: prioModuleLaserOutputPowerDbm,
		Dims: collectorapi.Dims{
			{ID: "iface_%s_laser_output_power_dbm", Name: "tx_power", Div: precision},
		},
	}
	ifaceModuleLaserBiasCurrentChartTmpl = collectorapi.Chart{
		ID:       "iface_%s_module_laser_bias_current",
		Title:    "CollectorV1 Laser Bias Current",
		Units:    "mA",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_laser_bias_current",
		Priority: prioModuleLaserBiasCurrent,
		Dims: collectorapi.Dims{
			{ID: "iface_%s_laser_bias_current_ma", Name: "bias_current", Div: precision},
		},
	}
	ifaceModuleTempCelsiusChartTmpl = collectorapi.Chart{
		ID:       "iface_%s_module_temperature_c",
		Title:    "CollectorV1 Temperature",
		Units:    "Celsius",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_temperature",
		Priority: prioModuleTemperatureC,
		Dims: collectorapi.Dims{
			{ID: "iface_%s_module_temperature_c", Name: "temperature", Div: precision},
		},
	}
	ifaceModuleVoltageChartTmpl = collectorapi.Chart{
		ID:       "iface_%s_module_voltage",
		Title:    "CollectorV1 Voltage",
		Units:    "Volts",
		Fam:      "optical module",
		Ctx:      "ethtool.optical_module_voltage",
		Priority: prioModuleVoltage,
		Dims: collectorapi.Dims{
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
		chart.Labels = []collectorapi.Label{
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
