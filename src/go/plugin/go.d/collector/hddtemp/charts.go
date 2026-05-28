// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDiskTemperature = collectorapi.Priority + iota
	prioDiskTemperatureSensorStatus
)

var (
	diskTemperatureChartsTmpl = collectorapi.Chart{
		ID:       "disk_%s_temperature",
		Title:    "Disk temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "hddtemp.disk_temperature",
		Type:     collectorapi.Line,
		Priority: prioDiskTemperature,
		Dims: collectorapi.Dims{
			{ID: "disk_%s_temperature", Name: "temperature"},
		},
	}
	diskTemperatureSensorChartsTmpl = collectorapi.Chart{
		ID:       "disk_%s_temperature_sensor_status",
		Title:    "Disk temperature sensor status",
		Units:    "status",
		Fam:      "sensor",
		Ctx:      "hddtemp.disk_temperature_sensor_status",
		Type:     collectorapi.Line,
		Priority: prioDiskTemperatureSensorStatus,
		Dims: collectorapi.Dims{
			{ID: "disk_%s_temp_sensor_status_ok", Name: "ok"},
			{ID: "disk_%s_temp_sensor_status_err", Name: "err"},
			{ID: "disk_%s_temp_sensor_status_na", Name: "na"},
			{ID: "disk_%s_temp_sensor_status_unk", Name: "unk"},
			{ID: "disk_%s_temp_sensor_status_nos", Name: "nos"},
			{ID: "disk_%s_temp_sensor_status_slp", Name: "slp"},
		},
	}
)

func (c *Collector) addDiskTempSensorStatusChart(id string, disk diskStats) {
	c.addDiskChart(id, disk, diskTemperatureSensorChartsTmpl.Copy())
}

func (c *Collector) addDiskTempChart(id string, disk diskStats) {
	c.addDiskChart(id, disk, diskTemperatureChartsTmpl.Copy())
}

func (c *Collector) addDiskChart(id string, disk diskStats, chart *collectorapi.Chart) {
	chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(id))
	chart.Labels = []collectorapi.Label{
		{Key: "disk_id", Value: id},
		{Key: "model", Value: disk.model},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, id)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}
