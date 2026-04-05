// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package sensors

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/sensors/lmsensors"
)

const (
	prioTemperatureSensorInput = collectorapi.Priority + iota
	prioTemperatureSensorAlarm

	prioVoltageSensorInput
	prioVoltageSensorAverage
	prioVoltageSensorAlarm

	prioFanSensorInput
	prioFanSensorAlarm

	prioCurrentSensorInput
	prioCurrentSensorAverage
	prioCurrentSensorAlarm

	prioPowerSensorInput
	prioPowerSensorAverage
	prioPowerSensorAlarm

	prioEnergySensorInput

	prioHumiditySensorInput

	prioIntrusionSensorAlarm
)

var temperatureSensorChartsTmpl = collectorapi.Charts{
	temperatureSensorInputChartTmpl.Copy(),
	temperatureSensorAlarmChartTmpl.Copy(),
}

var voltageSensorChartsTmpl = collectorapi.Charts{
	voltageSensorInputChartTmpl.Copy(),
	voltageSensorAverageChartTmpl.Copy(),
	voltageSensorAlarmChartTmpl.Copy(),
}

var fanSensorChartsTmpl = collectorapi.Charts{
	fanSensorInputChartTmpl.Copy(),
	fanSensorAlarmChartTmpl.Copy(),
}

var currentSensorChartsTmpl = collectorapi.Charts{
	currentSensorInputChartTmpl.Copy(),
	currentSensorAverageChartTmpl.Copy(),
	currentSensorAlarmChartTmpl.Copy(),
}

var powerSensorChartsTmpl = collectorapi.Charts{
	powerSensorInputChartTmpl.Copy(),
	powerSensorAverageChartTmpl.Copy(),
	powerSensorAlarmChartTmpl.Copy(),
}

var energySensorChartsTmpl = collectorapi.Charts{
	energySensorInputChartTmpl.Copy(),
}

var humiditySensorChartsTmpl = collectorapi.Charts{
	humiditySensorInputChartTmpl.Copy(),
}

var intrusionSensorChartsTmpl = collectorapi.Charts{
	intrusionSensorAlarmChartTmpl.Copy(),
}

var (
	temperatureSensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_temperature",
		Title:    "Sensor Temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "sensors.chip_sensor_temperature",
		Type:     collectorapi.Line,
		Priority: prioTemperatureSensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	temperatureSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_temperature_alarm",
		Title:    "Temperature Sensor Alarm",
		Units:    "status",
		Fam:      "temperature",
		Ctx:      "sensors.chip_sensor_temperature_alarm",
		Type:     collectorapi.Line,
		Priority: prioTemperatureSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	voltageSensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_voltage",
		Title:    "Sensor Voltage",
		Units:    "Volts",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage",
		Type:     collectorapi.Line,
		Priority: prioVoltageSensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	voltageSensorAverageChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_voltage_average",
		Title:    "Sensor Voltage Average",
		Units:    "Volts",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage_average",
		Type:     collectorapi.Line,
		Priority: prioVoltageSensorAverage,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	voltageSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_voltage_alarm",
		Title:    "Voltage Sensor Alarm",
		Units:    "status",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage_alarm",
		Type:     collectorapi.Line,
		Priority: prioVoltageSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	fanSensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_fan",
		Title:    "Sensor Fan",
		Units:    "RPM",
		Fam:      "fan",
		Ctx:      "sensors.chip_sensor_fan",
		Type:     collectorapi.Line,
		Priority: prioFanSensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	fanSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_fan_alarm",
		Title:    "Fan Sensor Alarm",
		Units:    "status",
		Fam:      "fan",
		Ctx:      "sensors.chip_sensor_fan_alarm",
		Type:     collectorapi.Line,
		Priority: prioFanSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	currentSensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_current",
		Title:    "Sensor Current",
		Units:    "Amperes",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current",
		Type:     collectorapi.Line,
		Priority: prioCurrentSensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	currentSensorAverageChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_current_average",
		Title:    "Sensor Current Average",
		Units:    "Amperes",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current_average",
		Type:     collectorapi.Line,
		Priority: prioCurrentSensorAverage,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	currentSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_current_alarm",
		Title:    "Sensor Alarm",
		Units:    "status",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current_alarm",
		Type:     collectorapi.Line,
		Priority: prioCurrentSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	powerSensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_power",
		Title:    "Sensor Power",
		Units:    "Watts",
		Fam:      "power",
		Ctx:      "sensors.chip_sensor_power",
		Type:     collectorapi.Line,
		Priority: prioPowerSensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	powerSensorAverageChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_power_average",
		Title:    "Sensor Power Average",
		Units:    "Watts",
		Fam:      "power",
		Ctx:      "sensors.chip_sensor_power_average",
		Type:     collectorapi.Line,
		Priority: prioPowerSensorAverage,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	powerSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_power_alarm",
		Title:    "Power Sensor Alarm",
		Units:    "status",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_power_alarm",
		Type:     collectorapi.Line,
		Priority: prioPowerSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	energySensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_energy",
		Title:    "Sensor Energy",
		Units:    "Joules",
		Fam:      "energy",
		Ctx:      "sensors.chip_sensor_energy",
		Type:     collectorapi.Line,
		Priority: prioEnergySensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
)

var (
	humiditySensorInputChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_humidity",
		Title:    "Sensor Humidity",
		Units:    "percent",
		Fam:      "humidity",
		Ctx:      "sensors.chip_sensor_humidity",
		Type:     collectorapi.Line,
		Priority: prioHumiditySensorInput,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
)

var (
	intrusionSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "%s_%s_intrusion_alarm",
		Title:    "Sensor Intrusion Alarm",
		Units:    "status",
		Fam:      "intrusion",
		Ctx:      "sensors.chip_sensor_intrusion_alarm",
		Type:     collectorapi.Line,
		Priority: prioIntrusionSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

func (c *Collector) updateCharts(chips []*lmsensors.Chip) {
	seen := make(map[string]bool)

	for _, chip := range chips {
		for _, sn := range chip.Sensors.Voltage {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addVoltageCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Fan {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addFanCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Temperature {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addTemperatureCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Current {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addCurrentCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Power {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addPowerCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Energy {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addEnergyCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Humidity {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addHumidityCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Intrusion {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !c.seenSensors[key] {
				c.seenSensors[key] = true
				c.addIntrusionCharts(chip, sn)
			}
		}
	}

	for key := range c.seenSensors {
		if !seen[key] {
			delete(c.seenSensors, key)
			c.removeSensorChart(key)
		}
	}
}

func (c *Collector) addTemperatureCharts(chip *lmsensors.Chip, sn *lmsensors.TemperatureSensor) {
	charts := temperatureSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(temperatureSensorInputChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(temperatureSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addVoltageCharts(chip *lmsensors.Chip, sn *lmsensors.VoltageSensor) {
	charts := voltageSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(voltageSensorInputChartTmpl.ID)
	}
	if sn.Average == nil {
		_ = charts.Remove(voltageSensorAverageChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(voltageSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addFanCharts(chip *lmsensors.Chip, sn *lmsensors.FanSensor) {
	charts := fanSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(fanSensorInputChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(fanSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addCurrentCharts(chip *lmsensors.Chip, sn *lmsensors.CurrentSensor) {
	charts := currentSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(currentSensorInputChartTmpl.ID)
	}
	if sn.Average == nil {
		_ = charts.Remove(currentSensorAverageChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(currentSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addPowerCharts(chip *lmsensors.Chip, sn *lmsensors.PowerSensor) {
	charts := powerSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(powerSensorInputChartTmpl.ID)
	}
	if sn.Average == nil {
		_ = charts.Remove(powerSensorAverageChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(powerSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addEnergyCharts(chip *lmsensors.Chip, sn *lmsensors.EnergySensor) {
	charts := energySensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(energySensorInputChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addHumidityCharts(chip *lmsensors.Chip, sn *lmsensors.HumiditySensor) {
	charts := humiditySensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(humiditySensorInputChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addIntrusionCharts(chip *lmsensors.Chip, sn *lmsensors.IntrusionSensor) {
	charts := intrusionSensorChartsTmpl.Copy()

	if sn.Alarm == nil {
		_ = charts.Remove(intrusionSensorAlarmChartTmpl.ID)
	}

	c.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (c *Collector) addCharts(charts *collectorapi.Charts, chipUniqueName, chipSysDevice, snName, snLabel string) {
	if len(*charts) == 0 {
		return
	}

	if lbl := c.relabel(chipUniqueName, snName); lbl != "" {
		snLabel = lbl
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, chipUniqueName, snName)
		chart.ID = cleanChartId(chart.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "chip", Value: chipSysDevice},
			{Key: "chip_id", Value: chipUniqueName},
			{Key: "sensor", Value: snName},
			{Key: "label", Value: snLabel},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, chipUniqueName, snName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeSensorChart(px string) {
	px = cleanChartId(px)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
			return
		}
	}
}

func (c *Collector) relabel(chipUniqueName, snName string) string {
	for _, rv := range c.Relabel {
		if rv.Chip == "" {
			return ""
		}

		mr, err := matcher.NewSimplePatternsMatcher(rv.Chip)
		if err != nil {
			c.Debugf("failed to create simple pattern matcher from '%s': %v", rv.Chip, err)
			return ""
		}

		if !mr.MatchString(chipUniqueName) {
			return ""
		}

		for _, sv := range rv.Sensors {
			if sv.Name == snName {
				return sv.Label
			}
		}
	}
	return ""
}

func cleanChartId(id string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return strings.ToLower(r.Replace(id))
}
