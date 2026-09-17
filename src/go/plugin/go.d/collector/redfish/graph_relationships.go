// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

type relationshipMode string

const (
	relationshipComponents = "component"
	relationshipEnrichment = "enrichment"
	relationshipLegacy     = "legacy"
)

type graphRelationship struct {
	ParentKind string
	Path       string
	ChildKind  string
	Family     string
	Mode       relationshipMode
	Embedded   bool
	Source     string
}

// Supported source links. These describe acquisition, not inferred ownership.
var graphRelationships = []graphRelationship{
	{ParentKind: "service", Path: "Storage", ChildKind: "storage", Family: "storage", Mode: "traversal"},
	{ParentKind: "service", Path: "UpdateService", ChildKind: "update_service", Family: "firmware", Mode: "traversal"},
	{
		ParentKind: "update_service",
		Path:       "FirmwareInventory",
		ChildKind:  "firmware",
		Family:     "firmware",
		Mode:       "traversal",
	},
	{
		ParentKind: "update_service",
		Path:       "SoftwareInventory",
		ChildKind:  "software",
		Family:     "firmware",
		Mode:       "traversal",
	},
	{ParentKind: "system", Path: "Processors", ChildKind: "processor", Family: "compute", Mode: "component"},
	{ParentKind: "system", Path: "Memory", ChildKind: "memory", Family: "memory", Mode: "component"},
	{ParentKind: "system", Path: "Storage", ChildKind: "storage", Family: "storage", Mode: "component"},
	{
		ParentKind: "system",
		Path:       "EthernetInterfaces",
		ChildKind:  "ethernet_interface",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "system",
		Path:       "NetworkInterfaces",
		ChildKind:  "network_interface",
		Family:     "network",
		Mode:       "component",
	},
	{ParentKind: "system", Path: "PCIeDevices", ChildKind: "pcie_device", Family: "pcie", Mode: "component"},
	{ParentKind: "system", Path: "PCIeFunctions", ChildKind: "pcie_function", Family: "pcie", Mode: "component"},
	{
		ParentKind: "system",
		Path:       "Redundancy",
		ChildKind:  "redundancy",
		Family:     "base",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "system",
		Path:       "Links.OffloadedNetworkDeviceFunctions",
		ChildKind:  "network_device_function",
		Family:     "network",
		Mode:       "association",
	},
	{ParentKind: "chassis", Path: "Drives", ChildKind: "drive", Family: "storage", Mode: "component"},
	{ParentKind: "chassis", Path: "Memory", ChildKind: "memory", Family: "memory", Mode: "component"},
	{
		ParentKind: "chassis",
		Path:       "NetworkAdapters",
		ChildKind:  "network_adapter",
		Family:     "network",
		Mode:       "component",
	},
	{ParentKind: "chassis", Path: "PCIeDevices", ChildKind: "pcie_device", Family: "pcie", Mode: "component"},
	{ParentKind: "chassis", Path: "Sensors", ChildKind: "sensor", Family: "sensors", Mode: "component"},
	{ParentKind: "chassis", Path: "Controls", ChildKind: "control", Family: "thermal", Mode: "component"},
	{ParentKind: "chassis", Path: "LeakDetectors", ChildKind: "leak_detector", Family: "thermal", Mode: "component"},
	{
		ParentKind: "chassis",
		Path:       "ThermalSubsystem",
		ChildKind:  "thermal_subsystem",
		Family:     "thermal",
		Mode:       "component",
	},
	{ParentKind: "chassis", Path: "PowerSubsystem", ChildKind: "power_subsystem", Family: "power", Mode: "component"},
	{ParentKind: "chassis", Path: "Processors", ChildKind: "processor", Family: "compute", Mode: "component"},
	{ParentKind: "chassis", Path: "Links.Storage", ChildKind: "storage", Family: "storage", Mode: "association"},
	{ParentKind: "chassis", Path: "Thermal", ChildKind: "legacy_thermal", Family: "thermal", Mode: "legacy"},
	{ParentKind: "chassis", Path: "Power", ChildKind: "legacy_power", Family: "power", Mode: "legacy"},
	{
		ParentKind: "chassis",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{ParentKind: "chassis", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{
		ParentKind: "manager",
		Path:       "EthernetInterfaces",
		ChildKind:  "ethernet_interface",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "manager",
		Path:       "DedicatedNetworkPorts",
		ChildKind:  "network_port",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "manager",
		Path:       "SharedNetworkPorts",
		ChildKind:  "network_port",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "manager",
		Path:       "Redundancy",
		ChildKind:  "redundancy",
		Family:     "base",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{ParentKind: "processor", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{ParentKind: "processor", Path: "Metrics", ChildKind: "processor_metrics", Family: "compute", Mode: "enrichment"},
	{
		ParentKind: "processor",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{ParentKind: "processor", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "memory", Path: "Metrics", ChildKind: "memory_metrics", Family: "memory", Mode: "enrichment"},
	{
		ParentKind: "memory",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{ParentKind: "memory", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "storage", Path: "Controllers", ChildKind: "storage_controller", Family: "storage", Mode: "component"},
	{ParentKind: "storage", Path: "Drives", ChildKind: "drive", Family: "storage", Mode: "component"},
	{ParentKind: "storage", Path: "Volumes", ChildKind: "volume", Family: "storage", Mode: "component"},
	{
		ParentKind: "storage",
		Path:       "Redundancy",
		ChildKind:  "redundancy",
		Family:     "storage",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{ParentKind: "storage", Path: "Metrics", ChildKind: "storage_metrics", Family: "storage", Mode: "enrichment"},
	{ParentKind: "storage_controller", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{
		ParentKind: "storage_controller",
		Path:       "Metrics",
		ChildKind:  "storage_controller_metrics",
		Family:     "storage",
		Mode:       "enrichment",
	},
	{
		ParentKind: "storage_controller",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{
		ParentKind: "storage_controller",
		Path:       "Assembly",
		ChildKind:  "assembly_document",
		Family:     "firmware",
		Mode:       "enrichment",
	},
	{ParentKind: "drive", Path: "Metrics", ChildKind: "drive_metrics", Family: "storage", Mode: "enrichment"},
	{
		ParentKind: "drive",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{ParentKind: "drive", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "volume", Path: "Metrics", ChildKind: "volume_metrics", Family: "storage", Mode: "enrichment"},
	{
		ParentKind: "network_adapter",
		Path:       "NetworkDeviceFunctions",
		ChildKind:  "network_device_function",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "network_adapter",
		Path:       "NetworkPorts",
		ChildKind:  "network_port",
		Family:     "network",
		Mode:       "component",
	},
	{ParentKind: "network_adapter", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{
		ParentKind: "network_adapter",
		Path:       "Metrics",
		ChildKind:  "network_adapter_metrics",
		Family:     "network",
		Mode:       "enrichment",
	},
	{
		ParentKind: "network_adapter",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{
		ParentKind: "network_adapter",
		Path:       "Assembly",
		ChildKind:  "assembly_document",
		Family:     "firmware",
		Mode:       "enrichment",
	},
	{
		ParentKind: "network_interface",
		Path:       "NetworkDeviceFunctions",
		ChildKind:  "network_device_function",
		Family:     "network",
		Mode:       "component",
	},
	{
		ParentKind: "network_interface",
		Path:       "NetworkPorts",
		ChildKind:  "network_port",
		Family:     "network",
		Mode:       "component",
	},
	{ParentKind: "network_interface", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{
		ParentKind: "network_device_function",
		Path:       "Metrics",
		ChildKind:  "network_device_function_metrics",
		Family:     "network",
		Mode:       "enrichment",
	},
	{ParentKind: "port", Path: "Metrics", ChildKind: "port_metrics", Family: "network", Mode: "enrichment"},
	{
		ParentKind: "port",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{ParentKind: "pcie_device", Path: "PCIeFunctions", ChildKind: "pcie_function", Family: "pcie", Mode: "component"},
	{
		ParentKind: "pcie_device",
		Path:       "EnvironmentMetrics",
		ChildKind:  "environment_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{
		ParentKind: "pcie_device",
		Path:       "Assembly",
		ChildKind:  "assembly_document",
		Family:     "firmware",
		Mode:       "enrichment",
	},
	{
		ParentKind: "thermal_subsystem",
		Path:       "CoolantConnectors",
		ChildKind:  "coolant_connector",
		Family:     "thermal",
		Mode:       "component",
	},
	{ParentKind: "thermal_subsystem", Path: "Fans", ChildKind: "fan", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Filters", ChildKind: "filter", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Heaters", ChildKind: "heater", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Pumps", ChildKind: "pump", Family: "thermal", Mode: "component"},
	{
		ParentKind: "thermal_subsystem",
		Path:       "LeakDetection",
		ChildKind:  "leak_detection",
		Family:     "thermal",
		Mode:       "component",
	},
	{
		ParentKind: "thermal_subsystem",
		Path:       "CoolantConnectorRedundancy",
		ChildKind:  "redundancy",
		Family:     "thermal",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "thermal_subsystem",
		Path:       "FanRedundancy",
		ChildKind:  "redundancy",
		Family:     "thermal",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "thermal_subsystem",
		Path:       "ThermalMetrics",
		ChildKind:  "thermal_metrics",
		Family:     "thermal",
		Mode:       "enrichment",
	},
	{
		ParentKind: "leak_detection",
		Path:       "LeakDetectorGroups",
		ChildKind:  "leak_detector_group",
		Family:     "thermal",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "leak_detection",
		Path:       "LeakDetectors",
		ChildKind:  "leak_detector",
		Family:     "thermal",
		Mode:       "component",
	},
	{
		ParentKind: "leak_detector_group",
		Path:       "Detectors",
		ChildKind:  "leak_detector",
		Family:     "thermal",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "power_subsystem",
		Path:       "PowerSupplies",
		ChildKind:  "power_supply",
		Family:     "power",
		Mode:       "component",
	},
	{ParentKind: "power_subsystem", Path: "Batteries", ChildKind: "battery", Family: "power", Mode: "component"},
	{
		ParentKind: "power_subsystem",
		Path:       "PowerSupplyRedundancy",
		ChildKind:  "redundancy",
		Family:     "power",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{
		ParentKind: "power_supply",
		Path:       "Metrics",
		ChildKind:  "power_supply_metrics",
		Family:     "power",
		Mode:       "enrichment",
	},
	{
		ParentKind: "power_supply",
		Path:       "Assembly",
		ChildKind:  "assembly_document",
		Family:     "firmware",
		Mode:       "enrichment",
	},
	{ParentKind: "battery", Path: "Metrics", ChildKind: "battery_metrics", Family: "power", Mode: "enrichment"},
	{ParentKind: "battery", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{
		ParentKind: "sensor",
		Path:       "SensorGroup",
		ChildKind:  "redundancy",
		Family:     "sensors",
		Mode:       "component",
		Embedded:   true,
		Source:     "embedded_excerpt",
	},
	{ParentKind: "heater", Path: "Metrics", ChildKind: "heater_metrics", Family: "thermal", Mode: "enrichment"},
	{
		ParentKind: "system",
		Path:       "ProcessorSummary.Metrics",
		ChildKind:  "processor_summary_metrics",
		Family:     "compute",
		Mode:       "enrichment",
	},
	{
		ParentKind: "system",
		Path:       "MemorySummary.Metrics",
		ChildKind:  "memory_summary_metrics",
		Family:     "memory",
		Mode:       "enrichment",
	},
}

func relationshipsFor(kind string) []graphRelationship {
	var result []graphRelationship
	for _, rel := range graphRelationships {
		if rel.ParentKind == kind {
			result = append(result, rel)
		}
	}
	return result
}

func (c *protocolClient) familyEnabled(family string) bool {
	if family == "" || family == "base" {
		return true
	}
	return c.families == nil || c.families[family]
}
