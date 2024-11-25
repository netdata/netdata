// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package sensors

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/sensors/lmsensors"
)

const (
	prioTemperatureSensorInput = module.Priority + iota
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

var temperatureSensorChartsTmpl = module.Charts{
	temperatureSensorInputChartTmpl.Copy(),
	temperatureSensorAlarmChartTmpl.Copy(),
}

var voltageSensorChartsTmpl = module.Charts{
	voltageSensorInputChartTmpl.Copy(),
	voltageSensorAverageChartTmpl.Copy(),
	voltageSensorAlarmChartTmpl.Copy(),
}

var fanSensorChartsTmpl = module.Charts{
	fanSensorInputChartTmpl.Copy(),
	fanSensorAlarmChartTmpl.Copy(),
}

var currentSensorChartsTmpl = module.Charts{
	currentSensorInputChartTmpl.Copy(),
	currentSensorAverageChartTmpl.Copy(),
	currentSensorAlarmChartTmpl.Copy(),
}

var powerSensorChartsTmpl = module.Charts{
	powerSensorInputChartTmpl.Copy(),
	powerSensorAverageChartTmpl.Copy(),
	powerSensorAlarmChartTmpl.Copy(),
}

var energySensorChartsTmpl = module.Charts{
	energySensorInputChartTmpl.Copy(),
}

var humiditySensorChartsTmpl = module.Charts{
	humiditySensorInputChartTmpl.Copy(),
}

var intrusionSensorChartsTmpl = module.Charts{
	intrusionSensorAlarmChartTmpl.Copy(),
}

var (
	temperatureSensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_temperature",
		Title:    "Sensor Temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "sensors.chip_sensor_temperature",
		Type:     module.Line,
		Priority: prioTemperatureSensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	temperatureSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_temperature_alarm",
		Title:    "Temperature Sensor Alarm",
		Units:    "status",
		Fam:      "temperature",
		Ctx:      "sensors.chip_sensor_temperature_alarm",
		Type:     module.Line,
		Priority: prioTemperatureSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	voltageSensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_voltage",
		Title:    "Sensor Voltage",
		Units:    "Volts",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage",
		Type:     module.Line,
		Priority: prioVoltageSensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	voltageSensorAverageChartTmpl = module.Chart{
		ID:       "%s_%s_voltage_average",
		Title:    "Sensor Voltage Average",
		Units:    "Volts",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage_average",
		Type:     module.Line,
		Priority: prioVoltageSensorAverage,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	voltageSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_voltage_alarm",
		Title:    "Voltage Sensor Alarm",
		Units:    "status",
		Fam:      "voltage",
		Ctx:      "sensors.chip_sensor_voltage_alarm",
		Type:     module.Line,
		Priority: prioVoltageSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	fanSensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_fan",
		Title:    "Sensor Fan",
		Units:    "RPM",
		Fam:      "fan",
		Ctx:      "sensors.chip_sensor_fan",
		Type:     module.Line,
		Priority: prioFanSensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	fanSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_fan_alarm",
		Title:    "Fan Sensor Alarm",
		Units:    "status",
		Fam:      "fan",
		Ctx:      "sensors.chip_sensor_fan_alarm",
		Type:     module.Line,
		Priority: prioFanSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	currentSensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_current",
		Title:    "Sensor Current",
		Units:    "Amperes",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current",
		Type:     module.Line,
		Priority: prioCurrentSensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	currentSensorAverageChartTmpl = module.Chart{
		ID:       "%s_%s_current_average",
		Title:    "Sensor Current Average",
		Units:    "Amperes",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current_average",
		Type:     module.Line,
		Priority: prioCurrentSensorAverage,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	currentSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_current_alarm",
		Title:    "Sensor Alarm",
		Units:    "status",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_current_alarm",
		Type:     module.Line,
		Priority: prioCurrentSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	powerSensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_power",
		Title:    "Sensor Power",
		Units:    "Watts",
		Fam:      "power",
		Ctx:      "sensors.chip_sensor_power",
		Type:     module.Line,
		Priority: prioPowerSensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
	powerSensorAverageChartTmpl = module.Chart{
		ID:       "%s_%s_power_average",
		Title:    "Sensor Power Average",
		Units:    "Watts",
		Fam:      "power",
		Ctx:      "sensors.chip_sensor_power_average",
		Type:     module.Line,
		Priority: prioPowerSensorAverage,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_average", Name: "average", Div: precision},
		},
	}
	powerSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_power_alarm",
		Title:    "Power Sensor Alarm",
		Units:    "status",
		Fam:      "current",
		Ctx:      "sensors.chip_sensor_power_alarm",
		Type:     module.Line,
		Priority: prioPowerSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

var (
	energySensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_energy",
		Title:    "Sensor Energy",
		Units:    "Joules",
		Fam:      "energy",
		Ctx:      "sensors.chip_sensor_energy",
		Type:     module.Line,
		Priority: prioEnergySensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
)

var (
	humiditySensorInputChartTmpl = module.Chart{
		ID:       "%s_%s_humidity",
		Title:    "Sensor Humidity",
		Units:    "percent",
		Fam:      "humidity",
		Ctx:      "sensors.chip_sensor_humidity",
		Type:     module.Line,
		Priority: prioHumiditySensorInput,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_input", Name: "input", Div: precision},
		},
	}
)

var (
	intrusionSensorAlarmChartTmpl = module.Chart{
		ID:       "%s_%s_intrusion_alarm",
		Title:    "Sensor Intrusion Alarm",
		Units:    "status",
		Fam:      "intrusion",
		Ctx:      "sensors.chip_sensor_intrusion_alarm",
		Type:     module.Line,
		Priority: prioIntrusionSensorAlarm,
		Dims: module.Dims{
			{ID: "chip_%s_sensor_%s_alarm_clear", Name: "clear"},
			{ID: "chip_%s_sensor_%s_alarm_triggered", Name: "triggered"},
		},
	}
)

func (s *Sensors) updateCharts(chips []*lmsensors.Chip) {
	seen := make(map[string]bool)

	for _, chip := range chips {
		for _, sn := range chip.Sensors.Voltage {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addVoltageCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Fan {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addFanCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Temperature {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addTemperatureCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Current {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addCurrentCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Power {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addPowerCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Energy {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addEnergyCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Humidity {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addHumidityCharts(chip, sn)
			}
		}
		for _, sn := range chip.Sensors.Intrusion {
			key := chip.UniqueName + "_" + sn.Name
			seen[key] = true
			if !s.seenSensors[key] {
				s.seenSensors[key] = true
				s.addIntrusionCharts(chip, sn)
			}
		}
	}

	for key := range s.seenSensors {
		if !seen[key] {
			delete(s.seenSensors, key)
			s.removeSensorChart(key)
		}
	}
}

func (s *Sensors) addTemperatureCharts(chip *lmsensors.Chip, sn *lmsensors.TemperatureSensor) {
	charts := temperatureSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(temperatureSensorInputChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(temperatureSensorAlarmChartTmpl.ID)
	}

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addVoltageCharts(chip *lmsensors.Chip, sn *lmsensors.VoltageSensor) {
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

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addFanCharts(chip *lmsensors.Chip, sn *lmsensors.FanSensor) {
	charts := fanSensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(fanSensorInputChartTmpl.ID)
	}
	if sn.Alarm == nil {
		_ = charts.Remove(fanSensorAlarmChartTmpl.ID)
	}

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addCurrentCharts(chip *lmsensors.Chip, sn *lmsensors.CurrentSensor) {
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

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addPowerCharts(chip *lmsensors.Chip, sn *lmsensors.PowerSensor) {
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

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addEnergyCharts(chip *lmsensors.Chip, sn *lmsensors.EnergySensor) {
	charts := energySensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(energySensorInputChartTmpl.ID)
	}

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addHumidityCharts(chip *lmsensors.Chip, sn *lmsensors.HumiditySensor) {
	charts := humiditySensorChartsTmpl.Copy()

	if sn.Input == nil {
		_ = charts.Remove(humiditySensorInputChartTmpl.ID)
	}

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addIntrusionCharts(chip *lmsensors.Chip, sn *lmsensors.IntrusionSensor) {
	charts := intrusionSensorChartsTmpl.Copy()

	if sn.Alarm == nil {
		_ = charts.Remove(intrusionSensorAlarmChartTmpl.ID)
	}

	s.addCharts(charts, chip.UniqueName, chip.SysDevice, sn.Name, sn.Label)
}

func (s *Sensors) addCharts(charts *module.Charts, chipUniqueName, chipSysDevice, snName, snLabel string) {
	if len(*charts) == 0 {
		return
	}

	if lbl := s.relabel(chipUniqueName, snName); lbl != "" {
		snLabel = lbl
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, chipUniqueName, snName)
		chart.ID = cleanChartId(chart.ID)
		chart.Labels = []module.Label{
			{Key: "chip", Value: chipSysDevice},
			{Key: "chip_id", Value: chipUniqueName},
			{Key: "sensor", Value: snName},
			{Key: "label", Value: snLabel},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, chipUniqueName, snName)
		}
	}

	if err := s.Charts().Add(*charts...); err != nil {
		s.Warning(err)
	}
}

func (s *Sensors) removeSensorChart(px string) {
	px = cleanChartId(px)

	for _, chart := range *s.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
			return
		}
	}
}

func (s *Sensors) relabel(chipUniqueName, snName string) string {
	for _, rv := range s.Relabel {
		if rv.Chip == "" {
			return ""
		}

		mr, err := matcher.NewSimplePatternsMatcher(rv.Chip)
		if err != nil {
			s.Debugf("failed to create simple pattern matcher from '%s': %v", rv.Chip, err)
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
