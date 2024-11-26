// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package sensors

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/sensors/lmsensors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const precision = 1000

func (c *Collector) collect() (map[string]int64, error) {
	if c.sc == nil {
		return nil, errors.New("sysfs scanner is not initialized")
	}

	chips, err := c.sc.Scan()
	if err != nil {
		return nil, err
	}

	if len(chips) == 0 {
		return nil, errors.New("no chips found on the system")
	}

	mx := make(map[string]int64)

	for _, chip := range chips {
		for _, sn := range chip.Sensors.Voltage {
			writeVoltage(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Fan {
			writeFan(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Temperature {
			writeTemperature(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Current {
			writeCurrent(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Power {
			writePower(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Energy {
			writeEnergy(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Humidity {
			writeHumidity(mx, chip, sn)
		}
		for _, sn := range chip.Sensors.Intrusion {
			writeIntrusion(mx, chip, sn)
		}
	}

	c.updateCharts(chips)

	return mx, nil
}

func writeVoltage(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.VoltageSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
	writeMetric(mx, px+"input", sn.Input)
	writeMetric(mx, px+"average", sn.Average)
	writeMetric(mx, px+"min", sn.Min)
	writeMetric(mx, px+"max", sn.Max)
	writeMetric(mx, px+"lcrit", sn.CritMin)
	writeMetric(mx, px+"crit", sn.CritMax)
	writeMetric(mx, px+"lowest", sn.Lowest)
	writeMetric(mx, px+"highest", sn.Highest)
}

func writeFan(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.FanSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
	writeMetric(mx, px+"input", sn.Input)
	writeMetric(mx, px+"min", sn.Min)
	writeMetric(mx, px+"max", sn.Max)
	writeMetric(mx, px+"target", sn.Target)
}

func writeTemperature(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.TemperatureSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
	writeMetric(mx, px+"input", sn.Input)
	writeMetric(mx, px+"min", sn.Min)
	writeMetric(mx, px+"max", sn.Max)
	writeMetric(mx, px+"lcrit", sn.CritMin)
	writeMetric(mx, px+"crit", sn.CritMax)
	writeMetric(mx, px+"emergency", sn.Emergency)
	writeMetric(mx, px+"lowest", sn.Lowest)
	writeMetric(mx, px+"highest", sn.Highest)
}

func writeCurrent(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.CurrentSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
	writeMetric(mx, px+"max", sn.Max)
	writeMetric(mx, px+"min", sn.Min)
	writeMetric(mx, px+"lcrit", sn.CritMin)
	writeMetric(mx, px+"crit", sn.CritMax)
	writeMetric(mx, px+"input", sn.Input)
	writeMetric(mx, px+"average", sn.Average)
	writeMetric(mx, px+"lowest", sn.Lowest)
	writeMetric(mx, px+"highest", sn.Highest)
}

func writePower(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.PowerSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
	writeMetric(mx, px+"average", sn.Average)
	writeMetric(mx, px+"average_highest", sn.AverageHighest)
	writeMetric(mx, px+"average_lowest", sn.AverageLowest)
	writeMetric(mx, px+"average_max", sn.AverageMax)
	writeMetric(mx, px+"average_min", sn.AverageMin)
	writeMetric(mx, px+"input", sn.Input)
	writeMetric(mx, px+"input_highest", sn.InputHighest)
	writeMetric(mx, px+"input_lowest", sn.InputLowest)
	writeMetric(mx, px+"accuracy", sn.Accuracy)
	writeMetric(mx, px+"cap", sn.Cap)
	writeMetric(mx, px+"cap_max", sn.CapMax)
	writeMetric(mx, px+"cap_min", sn.CapMin)
	writeMetric(mx, px+"max", sn.Max)
	writeMetric(mx, px+"crit", sn.CritMax)
}

func writeEnergy(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.EnergySensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetric(mx, px+"input", sn.Input)
}

func writeHumidity(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.HumiditySensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetric(mx, px+"input", sn.Input)
}

func writeIntrusion(mx map[string]int64, chip *lmsensors.Chip, sn *lmsensors.IntrusionSensor) {
	px := sensorPrefix(chip.UniqueName, sn.Name)

	mx[px+"read_time"] = sn.ReadTime.Milliseconds()
	writeMetricAlarm(mx, px, sn.Alarm)
}

func writeMetric(mx map[string]int64, key string, value *float64) {
	if value != nil {
		mx[key] = int64(*value * precision)
	}
}

func writeMetricAlarm(mx map[string]int64, px string, value *bool) {
	if value != nil {
		mx[px+"alarm_clear"] = metrix.Bool(!*value)
		mx[px+"alarm_triggered"] = metrix.Bool(*value)
	}
}

func sensorPrefix(chip, sensor string) string {
	return fmt.Sprintf("chip_%s_sensor_%s_", chip, sensor)
}
