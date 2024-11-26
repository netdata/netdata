// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package nvme

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	_ = 2050 + iota // right after Disks section
	prioDeviceEstimatedEndurancePerc
	prioDeviceAvailableSparePerc
	prioDeviceCompositeTemperature
	prioDeviceIOTransferredCount
	prioDevicePowerCyclesCount
	prioDevicePowerOnTime
	prioDeviceUnsafeShutdownsCount
	prioDeviceCriticalWarningsState
	prioDeviceMediaErrorsRate
	prioDeviceErrorLogEntriesRate
	prioDeviceWarningCompositeTemperatureTime
	prioDeviceCriticalCompositeTemperatureTime
	prioDeviceThmTemp1TransitionsCount
	prioDeviceThmTemp2TransitionsRate
	prioDeviceThmTemp1Time
	prioDeviceThmTemp2Time
)

var deviceChartsTmpl = module.Charts{
	deviceEstimatedEndurancePercChartTmpl.Copy(),
	deviceAvailableSparePercChartTmpl.Copy(),
	deviceCompositeTemperatureChartTmpl.Copy(),
	deviceIOTransferredCountChartTmpl.Copy(),
	devicePowerCyclesCountChartTmpl.Copy(),
	devicePowerOnTimeChartTmpl.Copy(),
	deviceUnsafeShutdownsCountChartTmpl.Copy(),
	deviceCriticalWarningsStateChartTmpl.Copy(),
	deviceMediaErrorsRateChartTmpl.Copy(),
	deviceErrorLogEntriesRateChartTmpl.Copy(),
	deviceWarnCompositeTemperatureTimeChartTmpl.Copy(),
	deviceCritCompositeTemperatureTimeChartTmpl.Copy(),
	deviceThmTemp1TransitionsRateChartTmpl.Copy(),
	deviceThmTemp2TransitionsRateChartTmpl.Copy(),
	deviceThmTemp1TimeChartTmpl.Copy(),
	deviceThmTemp2TimeChartTmpl.Copy(),
}

var deviceEstimatedEndurancePercChartTmpl = module.Chart{
	ID:       "device_%s_estimated_endurance_perc",
	Title:    "Estimated endurance",
	Units:    "percentage",
	Fam:      "endurance",
	Ctx:      "nvme.device_estimated_endurance_perc",
	Priority: prioDeviceEstimatedEndurancePerc,
	Dims: module.Dims{
		{ID: "device_%s_percentage_used", Name: "used"},
	},
}
var deviceAvailableSparePercChartTmpl = module.Chart{
	ID:       "device_%s_available_spare_perc",
	Title:    "Remaining spare capacity",
	Units:    "percentage",
	Fam:      "spare",
	Ctx:      "nvme.device_available_spare_perc",
	Priority: prioDeviceAvailableSparePerc,
	Dims: module.Dims{
		{ID: "device_%s_available_spare", Name: "spare"},
	},
}
var deviceCompositeTemperatureChartTmpl = module.Chart{
	ID:       "device_%s_temperature",
	Title:    "Composite temperature",
	Units:    "celsius",
	Fam:      "temperature",
	Ctx:      "nvme.device_composite_temperature",
	Priority: prioDeviceCompositeTemperature,
	Dims: module.Dims{
		{ID: "device_%s_temperature", Name: "temperature"},
	},
}
var deviceIOTransferredCountChartTmpl = module.Chart{
	ID:       "device_%s_io_transferred_count",
	Title:    "Amount of data transferred to and from device",
	Units:    "bytes",
	Fam:      "transferred data",
	Ctx:      "nvme.device_io_transferred_count",
	Priority: prioDeviceIOTransferredCount,
	Type:     module.Area,
	Dims: module.Dims{
		{ID: "device_%s_data_units_read", Name: "read"},
		{ID: "device_%s_data_units_written", Name: "written", Mul: -1},
	},
}

var devicePowerCyclesCountChartTmpl = module.Chart{
	ID:       "device_%s_power_cycles_count",
	Title:    "Power cycles",
	Units:    "cycles",
	Fam:      "power cycles",
	Ctx:      "nvme.device_power_cycles_count",
	Priority: prioDevicePowerCyclesCount,
	Dims: module.Dims{
		{ID: "device_%s_power_cycles", Name: "power"},
	},
}
var devicePowerOnTimeChartTmpl = module.Chart{
	ID:       "device_%s_power_on_time",
	Title:    "Power-on time",
	Units:    "seconds",
	Fam:      "power-on time",
	Ctx:      "nvme.device_power_on_time",
	Priority: prioDevicePowerOnTime,
	Dims: module.Dims{
		{ID: "device_%s_power_on_time", Name: "power-on"},
	},
}
var deviceCriticalWarningsStateChartTmpl = module.Chart{
	ID:       "device_%s_critical_warnings_state",
	Title:    "Critical warnings state",
	Units:    "state",
	Fam:      "critical warnings",
	Ctx:      "nvme.device_critical_warnings_state",
	Priority: prioDeviceCriticalWarningsState,
	Dims: module.Dims{
		{ID: "device_%s_critical_warning_available_spare", Name: "available_spare"},
		{ID: "device_%s_critical_warning_temp_threshold", Name: "temp_threshold"},
		{ID: "device_%s_critical_warning_nvm_subsystem_reliability", Name: "nvm_subsystem_reliability"},
		{ID: "device_%s_critical_warning_read_only", Name: "read_only"},
		{ID: "device_%s_critical_warning_volatile_mem_backup_failed", Name: "volatile_mem_backup_failed"},
		{ID: "device_%s_critical_warning_persistent_memory_read_only", Name: "persistent_memory_read_only"},
	},
}
var deviceUnsafeShutdownsCountChartTmpl = module.Chart{
	ID:       "device_%s_unsafe_shutdowns_count",
	Title:    "Unsafe shutdowns",
	Units:    "shutdowns",
	Fam:      "shutdowns",
	Ctx:      "nvme.device_unsafe_shutdowns_count",
	Priority: prioDeviceUnsafeShutdownsCount,
	Dims: module.Dims{
		{ID: "device_%s_unsafe_shutdowns", Name: "unsafe"},
	},
}
var deviceMediaErrorsRateChartTmpl = module.Chart{
	ID:       "device_%s_media_errors_rate",
	Title:    "Media and data integrity errors",
	Units:    "errors/s",
	Fam:      "media errors",
	Ctx:      "nvme.device_media_errors_rate",
	Priority: prioDeviceMediaErrorsRate,
	Dims: module.Dims{
		{ID: "device_%s_media_errors", Name: "media", Algo: module.Incremental},
	},
}
var deviceErrorLogEntriesRateChartTmpl = module.Chart{
	ID:       "device_%s_error_log_entries_rate",
	Title:    "Error log entries",
	Units:    "entries/s",
	Fam:      "error log",
	Ctx:      "nvme.device_error_log_entries_rate",
	Priority: prioDeviceErrorLogEntriesRate,
	Dims: module.Dims{
		{ID: "device_%s_num_err_log_entries", Name: "error_log", Algo: module.Incremental},
	},
}
var deviceWarnCompositeTemperatureTimeChartTmpl = module.Chart{
	ID:       "device_%s_warning_composite_temperature_time",
	Title:    "Warning composite temperature time",
	Units:    "seconds",
	Fam:      "warn temp time",
	Ctx:      "nvme.device_warning_composite_temperature_time",
	Priority: prioDeviceWarningCompositeTemperatureTime,
	Dims: module.Dims{
		{ID: "device_%s_warning_temp_time", Name: "wctemp"},
	},
}
var deviceCritCompositeTemperatureTimeChartTmpl = module.Chart{
	ID:       "device_%s_critical_composite_temperature_time",
	Title:    "Critical composite temperature time",
	Units:    "seconds",
	Fam:      "crit temp time",
	Ctx:      "nvme.device_critical_composite_temperature_time",
	Priority: prioDeviceCriticalCompositeTemperatureTime,
	Dims: module.Dims{
		{ID: "device_%s_critical_comp_time", Name: "cctemp"},
	},
}
var (
	deviceThmTemp1TransitionsRateChartTmpl = module.Chart{
		ID:       "device_%s_thm_temp1_transitions_rate",
		Title:    "Thermal management temp1 transitions",
		Units:    "transitions/s",
		Fam:      "thermal mgmt transitions",
		Ctx:      "nvme.device_thermal_mgmt_temp1_transitions_rate",
		Priority: prioDeviceThmTemp1TransitionsCount,
		Dims: module.Dims{
			{ID: "device_%s_thm_temp1_trans_count", Name: "temp1", Algo: module.Incremental},
		},
	}
	deviceThmTemp2TransitionsRateChartTmpl = module.Chart{
		ID:       "device_%s_thm_temp2_transitions_rate",
		Title:    "Thermal management temp2 transitions",
		Units:    "transitions/s",
		Fam:      "thermal mgmt transitions",
		Ctx:      "nvme.device_thermal_mgmt_temp2_transitions_rate",
		Priority: prioDeviceThmTemp2TransitionsRate,
		Dims: module.Dims{
			{ID: "device_%s_thm_temp2_trans_count", Name: "temp2", Algo: module.Incremental},
		},
	}
)
var (
	deviceThmTemp1TimeChartTmpl = module.Chart{
		ID:       "device_%s_thm_temp1_time",
		Title:    "Thermal management temp1 time",
		Units:    "seconds",
		Fam:      "thermal mgmt time",
		Ctx:      "nvme.device_thermal_mgmt_temp1_time",
		Priority: prioDeviceThmTemp1Time,
		Dims: module.Dims{
			{ID: "device_%s_thm_temp1_total_time", Name: "temp1"},
		},
	}
	deviceThmTemp2TimeChartTmpl = module.Chart{
		ID:       "device_%s_thm_temp2_time",
		Title:    "Thermal management temp1 time",
		Units:    "seconds",
		Fam:      "thermal mgmt time",
		Ctx:      "nvme.device_thermal_mgmt_temp2_time",
		Priority: prioDeviceThmTemp2Time,
		Dims: module.Dims{
			{ID: "device_%s_thm_temp2_total_time", Name: "temp2"},
		},
	}
)

func (c *Collector) addDeviceCharts(devicePath, model string) {
	device := extractDeviceFromPath(devicePath)

	charts := deviceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, device)
		chart.Labels = []module.Label{
			{Key: "device", Value: device},
			{Key: "model_number", Value: model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, device)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDeviceCharts(devicePath string) {
	device := extractDeviceFromPath(devicePath)

	px := fmt.Sprintf("device_%s", device)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
