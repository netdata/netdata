// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	maxSchemaIdentityTokenBytes = 256
	maxRedfishVersionTokenBytes = 64
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
	"log_service":          {"LogService"},
	"log_entry":            {"LogEntry"},
	"legacy_thermal":       {"Thermal"},
	"legacy_power":         {"Power"},
	"update_service":       {"UpdateService"},
	"session_service":      {"SessionService"},
	"session":              {"Session"},
}

func validateResourceSchemaType(kind, value string) error {
	expected := resourceSchemaNames[kind]
	if len(expected) == 0 {
		return nil
	}
	name, namespace, ok := parseODataType(value)
	if !ok {
		return fmt.Errorf("%s resource has no valid @odata.type", kind)
	}
	for _, candidate := range expected {
		if name == candidate && validVersionedSchemaNamespace(candidate, namespace) {
			return nil
		}
	}
	return fmt.Errorf("%s resource has unexpected @odata.type", kind)
}

func validateCollectionSchemaType(value, expectedKind string) error {
	name, namespace, ok := parseODataType(value)
	if !ok || !strings.HasSuffix(name, "Collection") {
		return errors.New("collection has no valid @odata.type")
	}
	if namespace != name {
		return errors.New("collection has unexpected @odata.type")
	}
	if expectedKind == "" {
		return nil
	}
	for _, resourceName := range resourceSchemaNames[expectedKind] {
		if name == resourceName+"Collection" {
			return nil
		}
	}
	return fmt.Errorf("%s collection has unexpected @odata.type", expectedKind)
}

func validateRequiredResourceProperties(kind string, data map[string]any) error {
	for _, property := range []string{"Id", "Name"} {
		value, ok := stringValue(data[property])
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s resource has no usable %s", kind, property)
		}
	}
	return nil
}

func validateRequiredLogEntryProperties(data map[string]any, requireID bool) error {
	if requireID {
		if value, ok := stringValue(data["Id"]); !ok || strings.TrimSpace(value) == "" {
			return errors.New("LogEntry has no usable Id")
		}
	}
	for _, property := range []string{"Name", "EntryType"} {
		value, ok := stringValue(data[property])
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("LogEntry has no usable %s", property)
		}
	}
	return nil
}

func validVersionedSchemaNamespace(name, namespace string) bool {
	if len(name) > maxSchemaIdentityTokenBytes || len(namespace) > maxSchemaIdentityTokenBytes {
		return false
	}
	version, ok := strings.CutPrefix(namespace, name+".v")
	if !ok {
		return false
	}
	parts := strings.Split(version, "_")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return false
		}
	}
	return true
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

func sameResourceIdentity(left, right string) bool {
	if left == right {
		return true
	}
	return strings.TrimSuffix(left, "/") == strings.TrimSuffix(right, "/") &&
		strings.TrimSuffix(left, "/") == "/redfish/v1"
}

func validRedfishVersion(value string) bool {
	if len(value) > maxRedfishVersionTokenBytes {
		return false
	}
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return false
		}
	}
	return true
}
