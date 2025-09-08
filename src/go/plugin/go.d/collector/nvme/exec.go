// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package nvme

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type nvmeDeviceList struct {
	Devices []struct {
		DevicePath   string `json:"DevicePath"`
		Firmware     string `json:"Firmware"`
		ModelNumber  string `json:"ModelNumber"`
		SerialNumber string `json:"SerialNumber"`
	}
}

func (n *nvmeDeviceList) UnmarshalJSON(b []byte) error {
	type plain nvmeDeviceList

	if err := json.Unmarshal(b, (*plain)(n)); err != nil {
		return err
	}

	if len(n.Devices) > 0 && n.Devices[0].DevicePath != "" {
		return nil
	}

	n.Devices = n.Devices[:0]

	var v211Format struct {
		Devices []struct {
			Subsystems []struct {
				Controllers []struct {
					SerialNumber string `json:"SerialNumber"`
					ModelNumber  string `json:"ModelNumber"`
					Firmware     string `json:"Firmware"`
					Namespaces   []struct {
						NameSpace string `json:"NameSpace"`
					} `json:"Namespaces"`
				} `json:"Controllers"`
			} `json:"Subsystems"`
		} `json:"Devices"`
	}

	if err := json.Unmarshal(b, &v211Format); err != nil {
		return err
	}

	for _, device := range v211Format.Devices {
		for _, subsystem := range device.Subsystems {
			for _, controller := range subsystem.Controllers {
				for _, namespace := range controller.Namespaces {
					devPath := namespace.NameSpace
					if !strings.HasPrefix(devPath, "/dev/") {
						devPath = "/dev/" + devPath
					}
					n.Devices = append(n.Devices, struct {
						DevicePath   string `json:"DevicePath"`
						Firmware     string `json:"Firmware"`
						ModelNumber  string `json:"ModelNumber"`
						SerialNumber string `json:"SerialNumber"`
					}{
						DevicePath:   devPath,
						Firmware:     controller.Firmware,
						ModelNumber:  controller.ModelNumber,
						SerialNumber: controller.SerialNumber,
					})
				}
			}
		}
	}

	return nil
}

// See "Health Information Log Page" in the Current Specification Version
// https://nvmexpress.org/developers/nvme-specification/
type nvmeDeviceSmartLog struct {
	CriticalWarningValue nvmeNumber
	CriticalWarning      json.RawMessage `json:"critical_warning"`
	Temperature          nvmeNumber      `json:"temperature"`
	AvailSpare           nvmeNumber      `json:"avail_spare"`
	SpareThresh          nvmeNumber      `json:"spare_thresh"`
	PercentUsed          nvmeNumber      `json:"percent_used"`
	DataUnitsRead        nvmeNumber      `json:"data_units_read"`
	DataUnitsWritten     nvmeNumber      `json:"data_units_written"`
	HostReadCommands     nvmeNumber      `json:"host_read_commands"`
	HostWriteCommands    nvmeNumber      `json:"host_write_commands"`
	ControllerBusyTime   nvmeNumber      `json:"controller_busy_time"`
	PowerCycles          nvmeNumber      `json:"power_cycles"`
	PowerOnHours         nvmeNumber      `json:"power_on_hours"`
	UnsafeShutdowns      nvmeNumber      `json:"unsafe_shutdowns"`
	MediaErrors          nvmeNumber      `json:"media_errors"`
	NumErrLogEntries     nvmeNumber      `json:"num_err_log_entries"`
	WarningTempTime      nvmeNumber      `json:"warning_temp_time"`
	CriticalCompTime     nvmeNumber      `json:"critical_comp_time"`
	ThmTemp1TransCount   nvmeNumber      `json:"thm_temp1_trans_count"`
	ThmTemp2TransCount   nvmeNumber      `json:"thm_temp2_trans_count"`
	ThmTemp1TotalTime    nvmeNumber      `json:"thm_temp1_total_time"`
	ThmTemp2TotalTime    nvmeNumber      `json:"thm_temp2_total_time"`
}

func (n *nvmeDeviceSmartLog) UnmarshalJSON(b []byte) error {
	type plain nvmeDeviceSmartLog

	if err := json.Unmarshal(b, (*plain)(n)); err != nil {
		return err
	}

	if n.CriticalWarning != nil {
		var v211Format struct {
			Value int `json:"value"`
		}
		if err := json.Unmarshal(n.CriticalWarning, &v211Format); err == nil {
			n.CriticalWarningValue = nvmeNumber(strconv.Itoa(v211Format.Value))
		} else {
			n.CriticalWarningValue = nvmeNumber(n.CriticalWarning)
		}
	}

	return nil
}

// nvme-cli 2.1.1 exposes some values as strings
type nvmeNumber string

func (n *nvmeNumber) UnmarshalJSON(b []byte) error {
	*n = nvmeNumber(bytes.Trim(b, "\""))
	return nil
}

type nvmeCli interface {
	list() (*nvmeDeviceList, error)
	smartLog(devicePath string) (*nvmeDeviceSmartLog, error)
}

type nvmeCLIExec struct {
	ndsudoPath string
	timeout    time.Duration
}

func (n *nvmeCLIExec) list() (*nvmeDeviceList, error) {
	bs, err := cmd.RunNDSudo(nil, n.timeout, "nvme-list")
	if err != nil {
		return nil, err
	}

	var v nvmeDeviceList
	if err := json.Unmarshal(bs, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

func (n *nvmeCLIExec) smartLog(devicePath string) (*nvmeDeviceSmartLog, error) {
	bs, err := cmd.RunNDSudo(nil, n.timeout, "nvme-smart-log", "--device", devicePath)
	if err != nil {
		return nil, err
	}

	var v nvmeDeviceSmartLog
	if err := json.Unmarshal(bs, &v); err != nil {
		return nil, err
	}

	return &v, nil
}
