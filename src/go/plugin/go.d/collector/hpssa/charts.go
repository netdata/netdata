// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioControllerStatus = module.Priority + iota
	prioControllerTemperature

	prioControllerCacheModulePresenceStatus
	prioControllerCacheModuleStatus
	prioControllerCacheModuleTemperature
	prioControllerCacheModuleBatteryStatus

	prioArrayStatus

	prioLogicalDriveStatus

	prioPhysicalDriveStatus
	prioPhysicalDriveTemperature
)

var controllerChartsTmpl = module.Charts{
	controllerStatusChartTmpl.Copy(),
	controllerTemperatureChartTmpl.Copy(),

	controllerCacheModulePresenceStatusChartTmpl.Copy(),
	controllerCacheModuleStatusChartTmpl.Copy(),
	controllerCacheModuleTemperatureChartTmpl.Copy(),
	controllerCacheModuleBatteryStatusChartTmpl.Copy(),
}

var (
	controllerStatusChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_status",
		Title:    "Controller status",
		Units:    "status",
		Fam:      "controllers",
		Ctx:      "hpssa.controller_status",
		Type:     module.Line,
		Priority: prioControllerStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_status_ok", Name: "ok"},
			{ID: "cntrl_%s_slot_%s_status_nok", Name: "nok"},
		},
	}
	controllerTemperatureChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_temperature",
		Title:    "Controller temperature",
		Units:    "Celsius",
		Fam:      "controllers",
		Ctx:      "hpssa.controller_temperature",
		Type:     module.Line,
		Priority: prioControllerTemperature,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_temperature", Name: "temperature"},
		},
	}

	controllerCacheModulePresenceStatusChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_cache_presence_status",
		Title:    "Controller cache module presence",
		Units:    "status",
		Fam:      "cache",
		Ctx:      "hpssa.controller_cache_module_presence_status",
		Type:     module.Line,
		Priority: prioControllerCacheModulePresenceStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_cache_presence_status_present", Name: "present"},
			{ID: "cntrl_%s_slot_%s_cache_presence_status_not_present", Name: "not_present"},
		},
	}
	controllerCacheModuleStatusChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_cache_status",
		Title:    "Controller cache module status",
		Units:    "status",
		Fam:      "cache",
		Ctx:      "hpssa.controller_cache_module_status",
		Type:     module.Line,
		Priority: prioControllerCacheModuleStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_cache_status_ok", Name: "ok"},
			{ID: "cntrl_%s_slot_%s_cache_status_nok", Name: "nok"},
		},
	}
	controllerCacheModuleTemperatureChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_cache_temperature",
		Title:    "Controller cache module temperature",
		Units:    "Celsius",
		Fam:      "cache",
		Ctx:      "hpssa.controller_cache_module_temperature",
		Type:     module.Line,
		Priority: prioControllerCacheModuleTemperature,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_cache_temperature", Name: "temperature"},
		},
	}
	controllerCacheModuleBatteryStatusChartTmpl = module.Chart{
		ID:       "cntrl_%s_slot_%s_cache_battery_status",
		Title:    "Controller cache module battery status",
		Units:    "status",
		Fam:      "cache",
		Ctx:      "hpssa.controller_cache_module_battery_status",
		Type:     module.Line,
		Priority: prioControllerCacheModuleBatteryStatus,
		Dims: module.Dims{
			{ID: "cntrl_%s_slot_%s_cache_battery_status_ok", Name: "ok"},
			{ID: "cntrl_%s_slot_%s_cache_battery_status_nok", Name: "nok"},
		},
	}
)

var arrayChartsTmpl = module.Charts{
	arrayStatusChartTmpl.Copy(),
}

var (
	arrayStatusChartTmpl = module.Chart{
		ID:       "array_%s_cntrl_%s_slot_%s_status",
		Title:    "Array status",
		Units:    "status",
		Fam:      "arrays",
		Ctx:      "hpssa.array_status",
		Type:     module.Line,
		Priority: prioArrayStatus,
		Dims: module.Dims{
			{ID: "array_%s_cntrl_%s_slot_%s_status_ok", Name: "ok"},
			{ID: "array_%s_cntrl_%s_slot_%s_status_nok", Name: "nok"},
		},
	}
)

var logicalDriveChartsTmpl = module.Charts{
	logicalDriveStatusChartTmpl.Copy(),
}

var (
	logicalDriveStatusChartTmpl = module.Chart{
		ID:       "ld_%s_array_%s_cntrl_%s_slot_%s_status",
		Title:    "Logical Drive status",
		Units:    "status",
		Fam:      "logical drives",
		Ctx:      "hpssa.logical_drive_status",
		Type:     module.Line,
		Priority: prioLogicalDriveStatus,
		Dims: module.Dims{
			{ID: "ld_%s_array_%s_cntrl_%s_slot_%s_status_ok", Name: "ok"},
			{ID: "ld_%s_array_%s_cntrl_%s_slot_%s_status_nok", Name: "nok"},
		},
	}
)

var physicalDriveChartsTmpl = module.Charts{
	physicalDriveStatusChartTmpl.Copy(),
	physicalDriveTemperatureChartTmpl.Copy(),
}

var (
	physicalDriveStatusChartTmpl = module.Chart{
		ID:       "pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_status",
		Title:    "Physical Drive status",
		Units:    "status",
		Fam:      "physical drives",
		Ctx:      "hpssa.physical_drive_status",
		Type:     module.Line,
		Priority: prioPhysicalDriveStatus,
		Dims: module.Dims{
			{ID: "pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_status_ok", Name: "ok"},
			{ID: "pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_status_nok", Name: "nok"},
		},
	}
	physicalDriveTemperatureChartTmpl = module.Chart{
		ID:       "pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_temperature",
		Title:    "Physical Drive temperature",
		Units:    "Celsius",
		Fam:      "physical drives",
		Ctx:      "hpssa.physical_drive_temperature",
		Type:     module.Line,
		Priority: prioPhysicalDriveTemperature,
		Dims: module.Dims{
			{ID: "pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_temperature", Name: "temperature"},
		},
	}
)

func (c *Collector) updateCharts(controllers map[string]*hpssaController) {
	seenControllers := make(map[string]bool)
	seenArrays := make(map[string]bool)
	seenLDrives := make(map[string]bool)
	seenPDrives := make(map[string]bool)

	for _, cntrl := range controllers {
		key := cntrl.uniqueKey()
		seenControllers[key] = true
		if _, ok := c.seenControllers[key]; !ok {
			c.seenControllers[key] = cntrl
			c.addControllerCharts(cntrl)
		}

		for _, pd := range cntrl.unassignedDrives {
			key := pd.uniqueKey()
			seenPDrives[key] = true
			if _, ok := c.seenPDrives[key]; !ok {
				c.seenPDrives[key] = pd
				c.addPhysicalDriveCharts(pd)
			}
		}

		for _, arr := range cntrl.arrays {
			key := arr.uniqueKey()
			seenArrays[key] = true
			if _, ok := c.seenArrays[key]; !ok {
				c.seenArrays[key] = arr
				c.addArrayCharts(arr)
			}

			for _, ld := range arr.logicalDrives {
				key := ld.uniqueKey()
				seenLDrives[key] = true
				if _, ok := c.seenLDrives[key]; !ok {
					c.seenLDrives[key] = ld
					c.addLogicalDriveCharts(ld)
				}

				for _, pd := range ld.physicalDrives {
					key := pd.uniqueKey()
					seenPDrives[key] = true
					if _, ok := c.seenPDrives[key]; !ok {
						c.seenPDrives[key] = pd
						c.addPhysicalDriveCharts(pd)
					}
				}
			}
		}
	}

	for k, cntrl := range c.seenControllers {
		if !seenControllers[k] {
			delete(c.seenControllers, k)
			c.removeControllerCharts(cntrl)
		}
	}
	for k, arr := range c.seenArrays {
		if !seenArrays[k] {
			delete(c.seenArrays, k)
			c.removeArrayCharts(arr)
		}
	}
	for k, ld := range c.seenLDrives {
		if !seenLDrives[k] {
			delete(c.seenLDrives, k)
			c.removeLogicalDriveCharts(ld)
		}
	}
	for k, pd := range c.seenPDrives {
		if !seenPDrives[k] {
			delete(c.seenPDrives, k)
			c.removePhysicalDriveCharts(pd)
		}
	}
}

func (c *Collector) addControllerCharts(cntrl *hpssaController) {
	charts := controllerChartsTmpl.Copy()

	if cntrl.controllerTemperatureC == "" {
		_ = charts.Remove(controllerTemperatureChartTmpl.ID)
	}

	if cntrl.cacheBoardPresent != "True" {
		_ = charts.Remove(controllerCacheModuleStatusChartTmpl.ID)
		_ = charts.Remove(controllerCacheModuleTemperatureChartTmpl.ID)
		_ = charts.Remove(controllerCacheModuleBatteryStatusChartTmpl.ID)
	}
	if cntrl.cacheModuleTemperatureC == "" {
		_ = charts.Remove(controllerCacheModuleTemperatureChartTmpl.ID)
	}
	if cntrl.batteryCapacitorStatus == "" {
		_ = charts.Remove(controllerCacheModuleBatteryStatusChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(cntrl.model), cntrl.slot)
		chart.Labels = []module.Label{
			{Key: "slot", Value: cntrl.slot},
			{Key: "model", Value: cntrl.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cntrl.model, cntrl.slot)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeControllerCharts(cntrl *hpssaController) {
	px := fmt.Sprintf("cntrl_%s_slot_%s_", strings.ToLower(cntrl.model), cntrl.slot)
	c.removeCharts(px)
}

func (c *Collector) addArrayCharts(arr *hpssaArray) {
	charts := arrayChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, arr.id, strings.ToLower(arr.cntrl.model), arr.cntrl.slot)
		chart.Labels = []module.Label{
			{Key: "slot", Value: arr.cntrl.slot},
			{Key: "array_id", Value: arr.id},
			{Key: "interface_type", Value: arr.interfaceType},
			{Key: "array_type", Value: arr.arrayType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, arr.id, arr.cntrl.model, arr.cntrl.slot)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeArrayCharts(arr *hpssaArray) {
	px := fmt.Sprintf("array_%s_cntrl_%s_slot_%s_", arr.id, strings.ToLower(arr.cntrl.model), arr.cntrl.slot)
	c.removeCharts(px)
}

func (c *Collector) addLogicalDriveCharts(ld *hpssaLogicalDrive) {
	charts := logicalDriveChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ld.id, ld.arr.id, strings.ToLower(ld.cntrl.model), ld.cntrl.slot)
		chart.Labels = []module.Label{
			{Key: "slot", Value: ld.cntrl.slot},
			{Key: "array_id", Value: ld.arr.id},
			{Key: "logical_drive_id", Value: ld.id},
			{Key: "disk_name", Value: ld.diskName},
			{Key: "drive_type", Value: ld.driveType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ld.id, ld.arr.id, ld.cntrl.model, ld.cntrl.slot)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeLogicalDriveCharts(ld *hpssaLogicalDrive) {
	px := fmt.Sprintf("ld_%s_array_%s_cntrl_%s_slot_%s_", ld.id, ld.arr.id, strings.ToLower(ld.cntrl.model), ld.cntrl.slot)
	c.removeCharts(px)
}

func (c *Collector) addPhysicalDriveCharts(pd *hpssaPhysicalDrive) {
	charts := physicalDriveChartsTmpl.Copy()

	if pd.currentTemperatureC == "" {
		_ = charts.Remove(physicalDriveTemperatureChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pd.location, pd.ldId(), pd.arrId(), strings.ToLower(pd.cntrl.model), pd.cntrl.slot)
		chart.Labels = []module.Label{
			{Key: "slot", Value: pd.cntrl.slot},
			{Key: "array_id", Value: pd.arrId()},
			{Key: "logical_drive_id", Value: pd.ldId()},
			{Key: "location", Value: pd.location},
			{Key: "interface_type", Value: pd.interfaceType},
			{Key: "drive_type", Value: pd.driveType},
			{Key: "model", Value: pd.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, pd.location, pd.ldId(), pd.arrId(), pd.cntrl.model, pd.cntrl.slot)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removePhysicalDriveCharts(pd *hpssaPhysicalDrive) {
	px := fmt.Sprintf("pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_",
		pd.location, pd.ldId(), pd.arrId(), strings.ToLower(pd.cntrl.model), pd.cntrl.slot)
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
