// SPDX-License-Identifier: GPL-3.0-or-later

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
			ControllerStatus string     `json:"Controller Status"`
			BBUStatus        storNumber `json:"BBU Status"`
		} `json:"Status"`
		BBUInfo []struct {
			Model string `json:"Model"`
			State string `json:"State"`
			Temp  string `json:"Temp"`
		} `json:"BBU_Info"`
		PDList []struct {
		} `json:"PD LIST"`
	}
)

func (s *StorCli) collectControllersInfo(mx map[string]int64, resp *controllersInfoResponse) error {
	for _, v := range resp.Controllers {
		cntrl := v.ResponseData

		idx := strconv.Itoa(cntrl.Basics.Controller)
		if !s.controllers[idx] {
			s.controllers[idx] = true
			s.addControllerCharts(cntrl)
		}

		px := fmt.Sprintf("cntrl_%s_", idx)

		for _, st := range []string{"optimal", "degraded", "partially_degraded", "failed"} {
			mx[px+"status_"+st] = 0
		}
		mx[px+"status_"+strings.ToLower(cntrl.Status.ControllerStatus)] = 1

		for _, st := range []string{"healthy", "unhealthy", "na"} {
			mx[px+"bbu_status_"+st] = 0
		}
		// https://github.com/prometheus-community/node-exporter-textfile-collector-scripts/issues/27
		switch cntrl.Status.BBUStatus {
		case "0", "8", "4096": // 0 good, 8 charging
			mx[px+"bbu_status_healthy"] = 1
		case "NA", "N/A":
			mx[px+"bbu_status_na"] = 1
		default:
			mx[px+"bbu_status_unhealthy"] = 1
		}
	}
	return nil
}

func (s *StorCli) queryControllersInfo() (*controllersInfoResponse, error) {
	bs, err := s.exec.controllersInfo()
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
