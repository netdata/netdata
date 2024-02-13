// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"
)

func (n *NVMe) collect() (map[string]int64, error) {
	if n.exec == nil {
		return nil, errors.New("nvme-cli is not initialized (nil)")
	}

	now := time.Now()
	if n.forceListDevices || now.Sub(n.listDevicesTime) > n.listDevicesEvery {
		n.forceListDevices = false
		n.listDevicesTime = now
		if err := n.listNVMeDevices(); err != nil {
			return nil, err
		}
	}

	mx := make(map[string]int64)

	for path := range n.devicePaths {
		if err := n.collectNVMeDevice(mx, path); err != nil {
			n.Error(err)
			n.forceListDevices = true
			continue
		}
	}

	return mx, nil
}

func (n *NVMe) collectNVMeDevice(mx map[string]int64, devicePath string) error {
	stats, err := n.exec.smartLog(devicePath)
	if err != nil {
		return fmt.Errorf("exec nvme smart-log for '%s': %v", devicePath, err)
	}

	device := extractDeviceFromPath(devicePath)

	mx["device_"+device+"_temperature"] = int64(float64(parseValue(stats.Temperature)) - 273.15) // Kelvin => Celsius
	mx["device_"+device+"_percentage_used"] = parseValue(stats.PercentUsed)
	mx["device_"+device+"_available_spare"] = parseValue(stats.AvailSpare)
	mx["device_"+device+"_data_units_read"] = parseValue(stats.DataUnitsRead) * 1000 * 512       // units => bytes
	mx["device_"+device+"_data_units_written"] = parseValue(stats.DataUnitsWritten) * 1000 * 512 // units => bytes
	mx["device_"+device+"_host_read_commands"] = parseValue(stats.HostReadCommands)
	mx["device_"+device+"_host_write_commands"] = parseValue(stats.HostWriteCommands)
	mx["device_"+device+"_power_cycles"] = parseValue(stats.PowerCycles)
	mx["device_"+device+"_power_on_time"] = parseValue(stats.PowerOnHours) * 3600 // hours => seconds
	mx["device_"+device+"_unsafe_shutdowns"] = parseValue(stats.UnsafeShutdowns)
	mx["device_"+device+"_media_errors"] = parseValue(stats.MediaErrors)
	mx["device_"+device+"_num_err_log_entries"] = parseValue(stats.NumErrLogEntries)
	mx["device_"+device+"_controller_busy_time"] = parseValue(stats.ControllerBusyTime) * 60 // minutes => seconds
	mx["device_"+device+"_warning_temp_time"] = parseValue(stats.WarningTempTime) * 60       // minutes => seconds
	mx["device_"+device+"_critical_comp_time"] = parseValue(stats.CriticalCompTime) * 60     // minutes => seconds
	mx["device_"+device+"_thm_temp1_trans_count"] = parseValue(stats.ThmTemp1TransCount)
	mx["device_"+device+"_thm_temp2_trans_count"] = parseValue(stats.ThmTemp2TransCount)
	mx["device_"+device+"_thm_temp1_total_time"] = parseValue(stats.ThmTemp1TotalTime) // seconds
	mx["device_"+device+"_thm_temp2_total_time"] = parseValue(stats.ThmTemp2TotalTime) // seconds

	mx["device_"+device+"_critical_warning_available_spare"] = boolToInt(parseValue(stats.CriticalWarning)&1 != 0)
	mx["device_"+device+"_critical_warning_temp_threshold"] = boolToInt(parseValue(stats.CriticalWarning)&(1<<1) != 0)
	mx["device_"+device+"_critical_warning_nvm_subsystem_reliability"] = boolToInt(parseValue(stats.CriticalWarning)&(1<<2) != 0)
	mx["device_"+device+"_critical_warning_read_only"] = boolToInt(parseValue(stats.CriticalWarning)&(1<<3) != 0)
	mx["device_"+device+"_critical_warning_volatile_mem_backup_failed"] = boolToInt(parseValue(stats.CriticalWarning)&(1<<4) != 0)
	mx["device_"+device+"_critical_warning_persistent_memory_read_only"] = boolToInt(parseValue(stats.CriticalWarning)&(1<<5) != 0)

	return nil
}

func (n *NVMe) listNVMeDevices() error {
	devices, err := n.exec.list()
	if err != nil {
		return fmt.Errorf("exec nvme list: %v", err)
	}

	seen := make(map[string]bool)
	for _, v := range devices.Devices {
		device := extractDeviceFromPath(v.DevicePath)
		seen[device] = true

		if !n.devicePaths[v.DevicePath] {
			n.devicePaths[v.DevicePath] = true
			n.addDeviceCharts(device)
		}
	}
	for path := range n.devicePaths {
		device := extractDeviceFromPath(path)
		if !seen[device] {
			delete(n.devicePaths, device)
			n.removeDeviceCharts(device)
		}
	}

	return nil
}

func extractDeviceFromPath(devicePath string) string {
	_, name := filepath.Split(devicePath)
	return name
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func parseValue(s nvmeNumber) int64 {
	v, _ := strconv.ParseFloat(string(s), 64)
	return int64(v)
}
