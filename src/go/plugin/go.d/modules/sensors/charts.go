// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/sensors/lmsensors"
)

const (
	prioSensorTemperature = module.Priority + iota
	prioSensorVoltage
	prioSensorCurrent
	prioSensorPower
	prioSensorFan
	prioSensorEnergy
	prioSensorHumidity
	prioSensorIntrusion
)

var sensorTemperatureChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_temperature",
	Title:    "Sensor temperature",
	Units:    "Celsius",
	Fam:      "temperature",
	Ctx:      "sensors.sensor_temperature",
	Type:     module.Line,
	Priority: prioSensorTemperature,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "temperature", Div: precision},
	},
}

var sensorVoltageChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_voltage",
	Title:    "Sensor voltage",
	Units:    "Volts",
	Fam:      "voltage",
	Ctx:      "sensors.sensor_voltage",
	Type:     module.Line,
	Priority: prioSensorVoltage,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "voltage", Div: precision},
	},
}

var sensorCurrentChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_current",
	Title:    "Sensor current",
	Units:    "Amperes",
	Fam:      "current",
	Ctx:      "sensors.sensor_current",
	Type:     module.Line,
	Priority: prioSensorCurrent,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "current", Div: precision},
	},
}

var sensorPowerChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_power",
	Title:    "Sensor power",
	Units:    "Watts",
	Fam:      "power",
	Ctx:      "sensors.sensor_power",
	Type:     module.Line,
	Priority: prioSensorPower,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "power", Div: precision},
	},
}

var sensorFanChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_fan",
	Title:    "Sensor fan speed",
	Units:    "RPM",
	Fam:      "fan",
	Ctx:      "sensors.sensor_fan_speed",
	Type:     module.Line,
	Priority: prioSensorFan,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "fan", Div: precision},
	},
}

var sensorEnergyChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_energy",
	Title:    "Sensor energy",
	Units:    "Joules",
	Fam:      "energy",
	Ctx:      "sensors.sensor_energy",
	Type:     module.Line,
	Priority: prioSensorEnergy,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "energy", Div: precision},
	},
}

var sensorHumidityChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_humidity",
	Title:    "Sensor humidity",
	Units:    "percent",
	Fam:      "humidity",
	Ctx:      "sensors.sensor_humidity",
	Type:     module.Area,
	Priority: prioSensorHumidity,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s", Name: "humidity", Div: precision},
	},
}

var sensorIntrusionChartTmpl = module.Chart{
	ID:       "sensor_chip_%s_feature_%s_subfeature_%s_intrusion",
	Title:    "Sensor intrusion",
	Units:    "status",
	Fam:      "intrusion",
	Ctx:      "sensors.sensor_intrusion",
	Type:     module.Line,
	Priority: prioSensorIntrusion,
	Dims: module.Dims{
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s_alarm_off", Name: "alarm_off"},
		{ID: "sensor_chip_%s_feature_%s_subfeature_%s_alarm_on", Name: "alarm_on"},
	},
}

func (s *Sensors) addExecSensorChart(sn execSensor) {
	var chart *module.Chart

	switch sn.sensorType() {
	case sensorTypeTemp:
		chart = sensorTemperatureChartTmpl.Copy()
	case sensorTypeVoltage:
		chart = sensorVoltageChartTmpl.Copy()
	case sensorTypePower:
		chart = sensorPowerChartTmpl.Copy()
	case sensorTypeHumidity:
		chart = sensorHumidityChartTmpl.Copy()
	case sensorTypeFan:
		chart = sensorFanChartTmpl.Copy()
	case sensorTypeCurrent:
		chart = sensorCurrentChartTmpl.Copy()
	case sensorTypeEnergy:
		chart = sensorEnergyChartTmpl.Copy()
	default:
		return
	}

	chip, feat, subfeat := snakeCase(sn.chip), snakeCase(sn.feature), snakeCase(sn.subfeature)

	chart.ID = fmt.Sprintf(chart.ID, chip, feat, subfeat)
	chart.Labels = []module.Label{
		{Key: "chip", Value: sn.chip},
		{Key: "feature", Value: sn.feature},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, chip, feat, subfeat)
	}

	if err := s.Charts().Add(chart); err != nil {
		s.Warning(err)
	}
}

func (s *Sensors) addSysfsSensorChart(devName string, sn lmsensors.Sensor) {
	var chart *module.Chart
	var feat, subfeat string
	devName = snakeCase(devName)

	switch v := sn.(type) {
	case *lmsensors.TemperatureSensor:
		chart = sensorTemperatureChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_input"
	case *lmsensors.VoltageSensor:
		chart = sensorVoltageChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_input"
	case *lmsensors.CurrentSensor:
		chart = sensorCurrentChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_input"
	case *lmsensors.PowerSensor:
		chart = sensorPowerChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_average"
	case *lmsensors.FanSensor:
		chart = sensorFanChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_input"
	case *lmsensors.IntrusionSensor:
		chart = sensorIntrusionChartTmpl.Copy()
		feat, subfeat = firstNotEmpty(v.Label, v.Name), v.Name+"_alarm"
	default:
		return
	}

	feat, subfeat = snakeCase(feat), snakeCase(subfeat)

	chart.ID = fmt.Sprintf(chart.ID, devName, feat, subfeat)
	chart.Labels = []module.Label{
		{Key: "chip", Value: devName},
		{Key: "feature", Value: feat},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, devName, feat, subfeat)
	}

	if err := s.Charts().Add(chart); err != nil {
		s.Warning(err)
	}
}

func (s *Sensors) removeSensorChart(px string) {
	for _, chart := range *s.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
			return
		}
	}
}
