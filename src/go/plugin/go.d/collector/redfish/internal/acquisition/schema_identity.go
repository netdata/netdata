// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"slices"
	"strings"
)

const (
	maxSchemaIdentityTokenBytes = 256
)

var resourceSchemaNames = map[string][]string{
	"service":                    {"ServiceRoot"},
	"system":                     {"ComputerSystem"},
	"chassis":                    {"Chassis"},
	"manager":                    {"Manager"},
	"processor":                  {"Processor"},
	"processor_metrics":          {"ProcessorMetrics"},
	"processor_summary_metrics":  {"ProcessorMetrics"},
	"memory":                     {"Memory"},
	"memory_metrics":             {"MemoryMetrics"},
	"memory_summary_metrics":     {"MemoryMetrics"},
	"storage":                    {"Storage"},
	"storage_metrics":            {"StorageMetrics"},
	"storage_controller":         {"StorageController"},
	"storage_controller_metrics": {"StorageControllerMetrics"},
	"drive":                      {"Drive"},
	"drive_metrics":              {"DriveMetrics"},
	"volume":                     {"Volume"},
	"volume_metrics":             {"VolumeMetrics"},
	"network_adapter":            {"NetworkAdapter"},
	"network_adapter_metrics":    {"NetworkAdapterMetrics"},
	"network_device_function":    {"NetworkDeviceFunction"},
	"network_device_function_metrics": {
		"NetworkDeviceFunctionMetrics",
	},
	"ethernet_interface":   {"EthernetInterface"},
	"network_interface":    {"NetworkInterface"},
	"network_port":         {"NetworkPort"},
	"port":                 {"Port"},
	"port_metrics":         {"PortMetrics"},
	"pcie_device":          {"PCIeDevice"},
	"pcie_function":        {"PCIeFunction"},
	"fan":                  {"Fan"},
	"pump":                 {"Pump"},
	"power_supply":         {"PowerSupply"},
	"power_supply_metrics": {"PowerSupplyMetrics"},
	"battery":              {"Battery"},
	"battery_metrics":      {"BatteryMetrics"},
	"sensor":               {"Sensor"},
	"thermal_subsystem":    {"ThermalSubsystem"},
	"thermal_metrics":      {"ThermalMetrics"},
	"power_subsystem":      {"PowerSubsystem"},
	"coolant_connector":    {"CoolantConnector"},
	"filter":               {"Filter"},
	"heater":               {"Heater"},
	"heater_metrics":       {"HeaterMetrics"},
	"leak_detection":       {"LeakDetection"},
	"leak_detector":        {"LeakDetector"},
	"control":              {"Control"},
	"environment_metrics":  {"EnvironmentMetrics"},
	"firmware":             {"SoftwareInventory"},
	"software":             {"SoftwareInventory"},
	"assembly":             {"Assembly"},
	"assembly_document":    {"Assembly"},
	"legacy_thermal":       {"Thermal"},
	"legacy_power":         {"Power"},
	"update_service":       {"UpdateService"},
}

func validateResourceSchemaType(kind, value string) error {
	expected := resourceSchemaNames[kind]
	if len(expected) == 0 {
		return nil
	}
	name, _, ok := parseODataType(value)
	if !ok {
		return fmt.Errorf("%s resource has no valid @odata.type", kind)
	}
	for _, candidate := range expected {
		if name == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s resource has unexpected @odata.type", kind)
}

func parseODataType(value string) (name, namespace string, ok bool) {
	if len(value) > maxSchemaIdentityTokenBytes {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "#") {
		return "", "", false
	}
	value = strings.TrimPrefix(value, "#")
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	if slices.Contains(parts, "") {
		return "", "", false
	}
	return parts[len(parts)-1], strings.Join(parts[:len(parts)-1], "."), true
}
