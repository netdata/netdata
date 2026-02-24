// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioLDStatus = collectorapi.Priority + iota

	prioPDState
	prioPDSmartWarnings
	prioPDSmartTemperature
)

var ldChartsTmpl = collectorapi.Charts{
	ldStatusChartTmpl.Copy(),
}

var (
	ldStatusChartTmpl = collectorapi.Chart{
		ID:       "logical_device_%s_status",
		Title:    "Logical Device status",
		Units:    "status",
		Fam:      "ld health",
		Ctx:      "adaptecraid.logical_device_status",
		Type:     collectorapi.Line,
		Priority: prioLDStatus,
		Dims: collectorapi.Dims{
			{ID: "ld_%s_health_state_ok", Name: "ok"},
			{ID: "ld_%s_health_state_critical", Name: "critical"},
		},
	}
)

var pdChartsTmpl = collectorapi.Charts{
	pdStateChartTmpl.Copy(),
	pdSmartWarningChartTmpl.Copy(),
	pdTemperatureChartTmpl.Copy(),
}

var (
	pdStateChartTmpl = collectorapi.Chart{
		ID:       "physical_device_%s_state",
		Title:    "Physical Device state",
		Units:    "state",
		Fam:      "pd health",
		Ctx:      "adaptecraid.physical_device_state",
		Type:     collectorapi.Line,
		Priority: prioPDState,
		Dims: collectorapi.Dims{
			{ID: "pd_%s_health_state_ok", Name: "ok"},
			{ID: "pd_%s_health_state_critical", Name: "critical"},
		},
	}
	pdSmartWarningChartTmpl = collectorapi.Chart{
		ID:       "physical_device_%s_smart_warnings",
		Title:    "Physical Device SMART warnings",
		Units:    "warnings",
		Fam:      "pd smart",
		Ctx:      "adaptecraid.physical_device_smart_warnings",
		Type:     collectorapi.Line,
		Priority: prioPDSmartWarnings,
		Dims: collectorapi.Dims{
			{ID: "pd_%s_smart_warnings", Name: "smart"},
		},
	}
	pdTemperatureChartTmpl = collectorapi.Chart{
		ID:       "physical_device_%s_temperature",
		Title:    "Physical Device temperature",
		Units:    "Celsius",
		Fam:      "pd temperature",
		Ctx:      "adaptecraid.physical_device_temperature",
		Type:     collectorapi.Line,
		Priority: prioPDSmartTemperature,
		Dims: collectorapi.Dims{
			{ID: "pd_%s_temperature", Name: "temperature"},
		},
	}
)

func (c *Collector) addLogicalDeviceCharts(ld *logicalDevice) {
	charts := ldChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ld.number)
		chart.Labels = []collectorapi.Label{
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
		chart.Labels = []collectorapi.Label{
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
