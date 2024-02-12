// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

type nvmeDeviceList struct {
	Devices []struct {
		DevicePath   string     `json:"DevicePath"`
		UsedBytes    nvmeNumber `json:"UsedBytes"`
		PhysicalSize nvmeNumber `json:"PhysicalSize"`
		SectorSize   nvmeNumber `json:"SectorSize"`
	}
}

// See "Health Information Log Page" in the Current Specification Version
// https://nvmexpress.org/developers/nvme-specification/
type nvmeDeviceSmartLog struct {
	CriticalWarning    nvmeNumber `json:"critical_warning"`
	Temperature        nvmeNumber `json:"temperature"`
	AvailSpare         nvmeNumber `json:"avail_spare"`
	SpareThresh        nvmeNumber `json:"spare_thresh"`
	PercentUsed        nvmeNumber `json:"percent_used"`
	DataUnitsRead      nvmeNumber `json:"data_units_read"`
	DataUnitsWritten   nvmeNumber `json:"data_units_written"`
	HostReadCommands   nvmeNumber `json:"host_read_commands"`
	HostWriteCommands  nvmeNumber `json:"host_write_commands"`
	ControllerBusyTime nvmeNumber `json:"controller_busy_time"`
	PowerCycles        nvmeNumber `json:"power_cycles"`
	PowerOnHours       nvmeNumber `json:"power_on_hours"`
	UnsafeShutdowns    nvmeNumber `json:"unsafe_shutdowns"`
	MediaErrors        nvmeNumber `json:"media_errors"`
	NumErrLogEntries   nvmeNumber `json:"num_err_log_entries"`
	WarningTempTime    nvmeNumber `json:"warning_temp_time"`
	CriticalCompTime   nvmeNumber `json:"critical_comp_time"`
	ThmTemp1TransCount nvmeNumber `json:"thm_temp1_trans_count"`
	ThmTemp2TransCount nvmeNumber `json:"thm_temp2_trans_count"`
	ThmTemp1TotalTime  nvmeNumber `json:"thm_temp1_total_time"`
	ThmTemp2TotalTime  nvmeNumber `json:"thm_temp2_total_time"`
}

// nvme-cli 2.1.1 exposes some values as strings
type nvmeNumber string

func (n *nvmeNumber) UnmarshalJSON(b []byte) error {
	*n = nvmeNumber(bytes.Trim(b, "\""))
	return nil
}

type nvmeCLIExec struct {
	sudoPath   string
	nvmePath   string
	ndsudoPath string
	timeout    time.Duration
}

func (n *nvmeCLIExec) list() (*nvmeDeviceList, error) {
	var data []byte
	var err error

	if n.ndsudoPath != "" {
		data, err = n.executeNdSudo("nvme-list")
	} else {
		data, err = n.execute("list", "--output-format=json")
	}
	if err != nil {
		return nil, err
	}

	var v nvmeDeviceList
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

func (n *nvmeCLIExec) smartLog(devicePath string) (*nvmeDeviceSmartLog, error) {
	var data []byte
	var err error

	if n.ndsudoPath != "" {
		data, err = n.executeNdSudo("nvme-smart-log", "--device", devicePath)
	} else {
		data, err = n.execute("smart-log", devicePath, "--output-format=json")
	}
	if err != nil {
		return nil, err
	}

	var v nvmeDeviceSmartLog
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

func (n *nvmeCLIExec) execute(arg ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	if n.sudoPath != "" {
		args := append([]string{"-n", n.nvmePath}, arg...)
		return exec.CommandContext(ctx, n.sudoPath, args...).Output()
	}

	return exec.CommandContext(ctx, n.nvmePath, arg...).Output()
}

func (n *nvmeCLIExec) executeNdSudo(arg ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	return exec.CommandContext(ctx, n.ndsudoPath, arg...).Output()
}
