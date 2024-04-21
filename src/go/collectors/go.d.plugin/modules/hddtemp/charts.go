// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	prioDiskTemperature = module.Priority + iota
	prioDiskTemperatureSensorStatus
)

var (
	diskTemperatureChartsTmpl = module.Chart{
		ID:       "disk_%s_temperature",
		Title:    "Disk temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "hddtemp.disk_temperature",
		Type:     module.Line,
		Priority: prioDiskTemperature,
		Dims: module.Dims{
			{ID: "disk_%s_temperature", Name: "temperature"},
		},
	}
	diskTemperatureSensorChartsTmpl = module.Chart{
		ID:       "disk_%s_temperature_sensor_status",
		Title:    "Disk temperature sensor status",
		Units:    "status",
		Fam:      "sensor",
		Ctx:      "hddtemp.disk_temperature_sensor_status",
		Type:     module.Line,
		Priority: prioDiskTemperatureSensorStatus,
		Dims: module.Dims{
			{ID: "disk_%s_temp_sensor_status_ok", Name: "ok"},
			{ID: "disk_%s_temp_sensor_status_err", Name: "err"},
			{ID: "disk_%s_temp_sensor_status_na", Name: "na"},
			{ID: "disk_%s_temp_sensor_status_unk", Name: "unk"},
			{ID: "disk_%s_temp_sensor_status_nos", Name: "nos"},
			{ID: "disk_%s_temp_sensor_status_slp", Name: "slp"},
		},
	}
)

func (h *HddTemp) addDiskTempSensorStatusChart(id string, disk diskStats) {
	h.addDiskChart(id, disk, diskTemperatureSensorChartsTmpl.Copy())
}

func (h *HddTemp) addDiskTempChart(id string, disk diskStats) {
	h.addDiskChart(id, disk, diskTemperatureChartsTmpl.Copy())
}

func (h *HddTemp) addDiskChart(id string, disk diskStats, chart *module.Chart) {
	chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(id))
	chart.Labels = []module.Label{
		{Key: "disk_id", Value: id},
		{Key: "model", Value: disk.model},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, id)
	}

	if err := h.Charts().Add(chart); err != nil {
		h.Warning(err)
	}
}
