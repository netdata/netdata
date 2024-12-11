// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type (
	controllersInfoResponse struct {
		Controllers []struct {
			CommandStatus struct {
				Controller int    `json:"Controller"`
				Status     string `json:"Status"`
			} `json:"Command Status"`
			ResponseData controllerInfo `json:"Response Data"`
		} `json:"Controllers"`
	}
	controllerInfo struct {
		Basics struct {
			Controller   int    `json:"Controller"`
			Model        string `json:"Model"`
			SerialNumber string `json:"Serial Number"`
		} `json:"Basics"`
		Version struct {
			DriverName string `json:"Driver Name"`
		} `json:"Version"`
		Status struct {
			ControllerStatus string      `json:"Controller Status"`
			BBUStatus        *storNumber `json:"BBU Status"`
		} `json:"Status"`
		HwCfg struct {
			TemperatureSensorForROC string `json:"Temperature Sensor for ROC"`
			ROCTemperatureC         int    `json:"ROC temperature(Degree Celsius)"`
		} `json:"HwCfg"`
		BBUInfo []struct {
			Model string `json:"Model"`
			State string `json:"State"`
			Temp  string `json:"Temp"`
		} `json:"BBU_Info"`
		PDList []struct {
		} `json:"PD LIST"`
	}
)

func (c *Collector) collectMegaraidControllersInfo(mx map[string]int64, resp *controllersInfoResponse) error {
	for _, v := range resp.Controllers {
		cntrl := v.ResponseData

		cntrlNum := strconv.Itoa(cntrl.Basics.Controller)

		if !c.controllers[cntrlNum] {
			c.controllers[cntrlNum] = true
			c.addControllerCharts(cntrl)
		}

		px := fmt.Sprintf("cntrl_%s_", cntrlNum)

		for _, st := range []string{"healthy", "unhealthy"} {
			mx[px+"health_status_"+st] = 0
		}
		if strings.ToLower(cntrl.Status.ControllerStatus) == "optimal" {
			mx[px+"health_status_healthy"] = 1
		} else {
			mx[px+"health_status_unhealthy"] = 1
		}

		for _, st := range []string{"optimal", "degraded", "partially_degraded", "failed"} {
			mx[px+"status_"+st] = 0
		}
		mx[px+"status_"+strings.ToLower(cntrl.Status.ControllerStatus)] = 1

		if cntrl.Status.BBUStatus != nil {
			for _, st := range []string{"healthy", "unhealthy", "na"} {
				mx[px+"bbu_status_"+st] = 0
			}
			// https://github.com/prometheus-community/node-exporter-textfile-collector-scripts/issues/27
			switch *cntrl.Status.BBUStatus {
			case "0", "8", "4096": // 0 good, 8 charging
				mx[px+"bbu_status_healthy"] = 1
			case "NA", "N/A":
				mx[px+"bbu_status_na"] = 1
			default:
				mx[px+"bbu_status_unhealthy"] = 1
			}
		}

		for i, bbu := range cntrl.BBUInfo {
			bbuNum := strconv.Itoa(i)
			if k := cntrlNum + bbuNum; !c.bbu[k] {
				c.bbu[k] = true
				c.addBBUCharts(cntrlNum, bbuNum, bbu.Model)
			}

			px := fmt.Sprintf("bbu_%s_cntrl_%s_", bbuNum, cntrlNum)

			if v, ok := parseInt(getTemperature(bbu.Temp)); ok {
				mx[px+"temperature"] = v
			}
		}
	}

	return nil
}

func (c *Collector) collectMpt3sasControllersInfo(mx map[string]int64, resp *controllersInfoResponse) error {
	for _, v := range resp.Controllers {
		cntrl := v.ResponseData

		cntrlNum := strconv.Itoa(cntrl.Basics.Controller)

		if !c.controllers[cntrlNum] {
			c.controllers[cntrlNum] = true
			c.addControllerCharts(cntrl)
		}

		px := fmt.Sprintf("cntrl_%s_", cntrlNum)

		for _, st := range []string{"healthy", "unhealthy"} {
			mx[px+"health_status_"+st] = 0
		}

		if strings.EqualFold(cntrl.Status.ControllerStatus, "ok") {
			mx[px+"health_status_healthy"] = 1
		} else {
			mx[px+"health_status_unhealthy"] = 1
		}

		if strings.EqualFold(cntrl.HwCfg.TemperatureSensorForROC, "present") {
			mx[px+"roc_temperature_celsius"] = int64(cntrl.HwCfg.ROCTemperatureC)
		}
	}

	return nil
}

func (c *Collector) queryControllersInfo() (*controllersInfoResponse, error) {
	bs, err := c.exec.controllersInfo()
	if err != nil {
		return nil, err
	}

	if len(bs) == 0 {
		return nil, errors.New("empty response")
	}

	var resp controllersInfoResponse
	if err := json.Unmarshal(bs, &resp); err != nil {
		return nil, err
	}
	if len(resp.Controllers) == 0 {
		return nil, errors.New("no controllers found")
	}
	if st := resp.Controllers[0].CommandStatus.Status; st != "Success" {
		return nil, fmt.Errorf("command status error: %s", st)
	}

	return &resp, nil
}
