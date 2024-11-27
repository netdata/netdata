// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioControllerHealthStatus = module.Priority + iota
	prioControllerStatus
	prioControllerBBUStatus
	prioControllerROCTemperature

	prioPhysDriveErrors
	prioPhysDrivePredictiveFailures
	prioPhysDriveSmartAlertStatus
	prioPhysDriveTemperature

	prioBBUTemperature
)

var controllerMegaraidChartsTmpl = module.Charts{
	controllerHealthStatusChartTmpl.Copy(),
	controllerStatusChartTmpl.Copy(),
	controllerBBUStatusChartTmpl.Copy(),
}

var controllerMpt3sasChartsTmpl = module.Charts{
	controllerHealthStatusChartTmpl.Copy(),
	controllerROCTemperatureChartTmpl.Copy(),
}

var (
	controllerHealthStatusChartTmpl = module.Chart{
		ID:       "controller_%s_health_status",
		Title:    "Controller health status",
		Units:    "status",
		Fam:      "cntrl status",
		Ctx:      "storcli.controller_health_status",
		Type:     module.Line,
		Priority: prioControllerHealthStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_health_status_healthy", Name: "healthy"},
			{ID: "cntrl_%s_health_status_unhealthy", Name: "unhealthy"},
		},
	}
	controllerStatusChartTmpl = module.Chart{
		ID:       "controller_%s_status",
		Title:    "Controller status",
		Units:    "status",
		Fam:      "cntrl status",
		Ctx:      "storcli.controller_status",
		Type:     module.Line,
		Priority: prioControllerStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_status_optimal", Name: "optimal"},
			{ID: "cntrl_%s_status_degraded", Name: "degraded"},
			{ID: "cntrl_%s_status_partially_degraded", Name: "partially_degraded"},
			{ID: "cntrl_%s_status_failed", Name: "failed"},
		},
	}
	controllerBBUStatusChartTmpl = module.Chart{
		ID:       "controller_%s_bbu_status",
		Title:    "Controller BBU status",
		Units:    "status",
		Fam:      "cntrl status",
		Ctx:      "storcli.controller_bbu_status",
		Type:     module.Line,
		Priority: prioControllerBBUStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_bbu_status_healthy", Name: "healthy"},
			{ID: "cntrl_%s_bbu_status_unhealthy", Name: "unhealthy"},
			{ID: "cntrl_%s_bbu_status_na", Name: "na"},
		},
	}
	controllerROCTemperatureChartTmpl = module.Chart{
		ID:       "controller_%s_roc_temperature",
		Title:    "Controller ROC temperature",
		Units:    "Celsius",
		Fam:      "cntrl roc temperature",
		Ctx:      "storcli.controller_roc_temperature",
		Type:     module.Line,
		Priority: prioControllerROCTemperature,
		Dims: module.Dims{
			{ID: "cntrl_%s_roc_temperature_celsius", Name: "temperature"},
		},
	}
)

var physDriveChartsTmpl = module.Charts{
	physDriveMediaErrorsRateChartTmpl.Copy(),
	physDrivePredictiveFailuresRateChartTmpl.Copy(),
	physDriveSmartAlertStatusChartTmpl.Copy(),
	physDriveTemperatureChartTmpl.Copy(),
}

var (
	physDriveMediaErrorsRateChartTmpl = module.Chart{
		ID:       "phys_drive_%s_cntrl_%s_media_errors_rate",
		Title:    "Physical Drive media errors rate",
		Units:    "errors/s",
		Fam:      "pd errors",
		Ctx:      "storcli.phys_drive_errors",
		Type:     module.Line,
		Priority: prioPhysDriveErrors,
		Dims: module.Dims{
			{ID: "phys_drive_%s_cntrl_%s_media_error_count", Name: "media"},
			{ID: "phys_drive_%s_cntrl_%s_other_error_count", Name: "other"},
		},
	}
	physDrivePredictiveFailuresRateChartTmpl = module.Chart{
		ID:       "phys_drive_%s_cntrl_%s_predictive_failures_rate",
		Title:    "Physical Drive predictive failures rate",
		Units:    "failures/s",
		Fam:      "pd errors",
		Ctx:      "storcli.phys_drive_predictive_failures",
		Type:     module.Line,
		Priority: prioPhysDrivePredictiveFailures,
		Dims: module.Dims{
			{ID: "phys_drive_%s_cntrl_%s_predictive_failure_count", Name: "predictive_failures"},
		},
	}
	physDriveSmartAlertStatusChartTmpl = module.Chart{
		ID:       "phys_drive_%s_cntrl_%s_smart_alert_status",
		Title:    "Physical Drive SMART alert status",
		Units:    "status",
		Fam:      "pd smart",
		Ctx:      "storcli.phys_drive_smart_alert_status",
		Type:     module.Line,
		Priority: prioPhysDriveSmartAlertStatus,
		Dims: module.Dims{
			{ID: "phys_drive_%s_cntrl_%s_smart_alert_status_active", Name: "active"},
			{ID: "phys_drive_%s_cntrl_%s_smart_alert_status_inactive", Name: "inactive"},
		},
	}
	physDriveTemperatureChartTmpl = module.Chart{
		ID:       "phys_drive_%s_cntrl_%s_temperature",
		Title:    "Physical Drive temperature",
		Units:    "Celsius",
		Fam:      "pd temperature",
		Ctx:      "storcli.phys_drive_temperature",
		Type:     module.Line,
		Priority: prioPhysDriveTemperature,
		Dims: module.Dims{
			{ID: "phys_drive_%s_cntrl_%s_temperature", Name: "temperature"},
		},
	}
)

var bbuChartsTmpl = module.Charts{
	bbuTemperatureChartTmpl.Copy(),
}

var (
	bbuTemperatureChartTmpl = module.Chart{
		ID:       "bbu_%s_cntrl_%s_temperature",
		Title:    "BBU temperature",
		Units:    "Celsius",
		Fam:      "bbu temperature",
		Ctx:      "storcli.bbu_temperature",
		Type:     module.Line,
		Priority: prioBBUTemperature,
		Dims: module.Dims{
			{ID: "bbu_%s_cntrl_%s_temperature", Name: "temperature"},
		},
	}
)

func (c *Collector) addControllerCharts(cntrl controllerInfo) {
	var charts *module.Charts

	switch cntrl.Version.DriverName {
	case driverNameMegaraid:
		charts = controllerMegaraidChartsTmpl.Copy()
	case driverNameSas:
		charts = controllerMpt3sasChartsTmpl.Copy()
		if !strings.EqualFold(cntrl.HwCfg.TemperatureSensorForROC, "present") {
			_ = charts.Remove(controllerROCTemperatureChartTmpl.ID)
		}
	default:
		return
	}

	num := strconv.Itoa(cntrl.Basics.Controller)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, num)
		chart.Labels = []module.Label{
			{Key: "controller_number", Value: num},
			{Key: "model", Value: strings.TrimSpace(cntrl.Basics.Model)},
			{Key: "driver_name", Value: cntrl.Version.DriverName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, num)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPhysDriveCharts(cntrlNum int, di *driveInfo, ds *driveState, da *driveAttrs) {
	charts := physDriveChartsTmpl.Copy()

	if _, ok := parseInt(getTemperature(ds.DriveTemperature)); !ok {
		_ = charts.Remove(physDriveTemperatureChartTmpl.ID)
	}

	num := strconv.Itoa(cntrlNum)

	var enc, slot string
	if parts := strings.Split(di.EIDSlt, ":"); len(parts) == 2 { // EID:Slt
		enc, slot = parts[0], parts[1]
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, da.WWN, num)
		chart.Labels = []module.Label{
			{Key: "controller_number", Value: num},
			{Key: "enclosure_number", Value: enc},
			{Key: "slot_number", Value: slot},
			{Key: "media_type", Value: di.Med},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, da.WWN, num)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addBBUCharts(cntrlNum, bbuNum, model string) {
	charts := bbuChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, bbuNum, cntrlNum)
		chart.Labels = []module.Label{
			{Key: "controller_number", Value: cntrlNum},
			{Key: "bbu_number", Value: bbuNum},
			{Key: "model", Value: model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, bbuNum, cntrlNum)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
