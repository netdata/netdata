// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioLDStatus = module.Priority + iota

	prioPDState
	prioPDSmartWarnings
	prioPDSmartTemperature
)

var ldChartsTmpl = module.Charts{
	ldStatusChartTmpl.Copy(),
}

var (
	ldStatusChartTmpl = module.Chart{
		ID:       "logical_device_%s_status",
		Title:    "Logical Device status",
		Units:    "status",
		Fam:      "ld health",
		Ctx:      "adaptecraid.logical_device_status",
		Type:     module.Line,
		Priority: prioLDStatus,
		Dims: module.Dims{
			{ID: "ld_%s_health_state_ok", Name: "ok"},
			{ID: "ld_%s_health_state_critical", Name: "critical"},
		},
	}
)

var pdChartsTmpl = module.Charts{
	pdStateChartTmpl.Copy(),
	pdSmartWarningChartTmpl.Copy(),
	pdTemperatureChartTmpl.Copy(),
}

var (
	pdStateChartTmpl = module.Chart{
		ID:       "physical_device_%s_state",
		Title:    "Physical Device state",
		Units:    "state",
		Fam:      "pd health",
		Ctx:      "adaptecraid.physical_device_state",
		Type:     module.Line,
		Priority: prioPDState,
		Dims: module.Dims{
			{ID: "pd_%s_health_state_ok", Name: "ok"},
			{ID: "pd_%s_health_state_critical", Name: "critical"},
		},
	}
	pdSmartWarningChartTmpl = module.Chart{
		ID:       "physical_device_%s_smart_warnings",
		Title:    "Physical Device SMART warnings",
		Units:    "warnings",
		Fam:      "pd smart",
		Ctx:      "adaptecraid.physical_device_smart_warnings",
		Type:     module.Line,
		Priority: prioPDSmartWarnings,
		Dims: module.Dims{
			{ID: "pd_%s_smart_warnings", Name: "smart"},
		},
	}
	pdTemperatureChartTmpl = module.Chart{
		ID:       "physical_device_%s_temperature",
		Title:    "Physical Device temperature",
		Units:    "Celsius",
		Fam:      "pd temperature",
		Ctx:      "adaptecraid.physical_device_temperature",
		Type:     module.Line,
		Priority: prioPDSmartTemperature,
		Dims: module.Dims{
			{ID: "pd_%s_temperature", Name: "temperature"},
		},
	}
)

func (c *Collector) addLogicalDeviceCharts(ld *logicalDevice) {
	charts := ldChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ld.number)
		chart.Labels = []module.Label{
			{Key: "ld_number", Value: ld.number},
			{Key: "ld_name", Value: ld.name},
			{Key: "raid_level", Value: ld.raidLevel},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ld.number)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPhysicalDeviceCharts(pd *physicalDevice) {
	charts := pdChartsTmpl.Copy()

	if _, err := strconv.ParseInt(pd.temperature, 10, 64); err != nil {
		_ = charts.Remove(pdTemperatureChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pd.number)
		chart.Labels = []module.Label{
			{Key: "pd_number", Value: pd.number},
			{Key: "location", Value: pd.location},
			{Key: "vendor", Value: pd.vendor},
			{Key: "model", Value: pd.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, pd.number)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
