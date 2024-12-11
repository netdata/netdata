// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	data, err := c.exec.controllersInfo()
	if err != nil {
		return nil, err
	}

	controllers, err := parseSsacliControllersInfo(data)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	c.collectControllers(mx, controllers)

	c.updateCharts(controllers)

	return mx, nil
}

func (c *Collector) collectControllers(mx map[string]int64, controllers map[string]*hpssaController) {
	for _, cntrl := range controllers {
		c.collectController(mx, cntrl)

		for _, pd := range cntrl.unassignedDrives {
			c.collectPhysicalDrive(mx, pd)
		}

		for _, arr := range cntrl.arrays {
			c.collectArray(mx, arr)

			for _, ld := range arr.logicalDrives {
				c.collectLogicalDrive(mx, ld)

				for _, pd := range ld.physicalDrives {
					c.collectPhysicalDrive(mx, pd)
				}
			}
		}
	}
}

func (c *Collector) collectController(mx map[string]int64, cntrl *hpssaController) {
	px := fmt.Sprintf("cntrl_%s_slot_%s_", cntrl.model, cntrl.slot)

	writeStatusOkNok(mx, px, cntrl.controllerStatus)

	if v, ok := parseNumber(cntrl.controllerTemperatureC); ok {
		mx[px+"temperature"] = v
	}

	mx[px+"cache_presence_status_present"] = 0
	mx[px+"cache_presence_status_not_present"] = 0
	if cntrl.cacheBoardPresent != "True" {
		mx[px+"cache_presence_status_not_present"] = 1
		return
	}

	mx[px+"cache_presence_status_present"] = 1

	writeStatusOkNok(mx, px+"cache_", cntrl.cacheStatus)

	if v, ok := parseNumber(cntrl.cacheModuleTemperatureC); ok {
		mx[px+"cache_temperature"] = v
	}

	writeStatusOkNok(mx, px+"cache_battery_", cntrl.batteryCapacitorStatus)
}

func (c *Collector) collectArray(mx map[string]int64, arr *hpssaArray) {
	if arr.cntrl == nil {
		return
	}

	px := fmt.Sprintf("array_%s_cntrl_%s_slot_%s_",
		arr.id, arr.cntrl.model, arr.cntrl.slot)

	writeStatusOkNok(mx, px, arr.status)
}

func (c *Collector) collectLogicalDrive(mx map[string]int64, ld *hpssaLogicalDrive) {
	if ld.cntrl == nil || ld.arr == nil {
		return
	}

	px := fmt.Sprintf("ld_%s_array_%s_cntrl_%s_slot_%s_",
		ld.id, ld.arr.id, ld.cntrl.model, ld.cntrl.slot)

	writeStatusOkNok(mx, px, ld.status)
}

func (c *Collector) collectPhysicalDrive(mx map[string]int64, pd *hpssaPhysicalDrive) {
	if pd.cntrl == nil {
		return
	}

	px := fmt.Sprintf("pd_%s_ld_%s_array_%s_cntrl_%s_slot_%s_",
		pd.location, pd.ldId(), pd.arrId(), pd.cntrl.model, pd.cntrl.slot)

	writeStatusOkNok(mx, px, pd.status)

	if v, ok := parseNumber(pd.currentTemperatureC); ok {
		mx[px+"temperature"] = v
	}
}

func parseNumber(s string) (int64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(v), true
}

func writeStatusOkNok(mx map[string]int64, prefix, status string) {
	if !strings.HasSuffix(prefix, "_") {
		prefix += "_"
	}

	mx[prefix+"status_ok"] = 0
	mx[prefix+"status_nok"] = 0

	switch status {
	case "":
	case "OK":
		mx[prefix+"status_ok"] = 1
	default:
		mx[prefix+"status_nok"] = 1
	}
}
