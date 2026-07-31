// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"testing"
)

// TestInventoryDeclarationsMatchSchemaGroundedCorrections is intentionally
// independent of inventoryFieldSpecs. It freezes the DMTF 2026.1 source shapes
// that differ from the originally generated declarations.
func TestInventoryDeclarationsMatchSchemaGroundedCorrections(t *testing.T) {
	type key struct {
		kind Kind
		path string
	}
	type expected struct {
		typ        ColumnType
		sourceType ColumnType
		structured bool
	}
	want := make(map[key]expected)
	add := func(kind Kind, typ ColumnType, paths ...string) {
		t.Helper()
		for _, path := range paths {
			current := want[key{kind: kind, path: path}]
			if current.typ != "" && current.typ != typ {
				t.Fatalf("conflicting golden types for %s.%s", kind, path)
			}
			current.typ = typ
			want[key{kind: kind, path: path}] = current
		}
	}
	structured := func(kind Kind, path string) {
		current := want[key{kind: kind, path: path}]
		current.structured = true
		want[key{kind: kind, path: path}] = current
	}
	source := func(kind Kind, path string, sourceType, typ ColumnType) {
		current := want[key{kind: kind, path: path}]
		current.sourceType = sourceType
		current.typ = typ
		want[key{kind: kind, path: path}] = current
	}

	add("system", ColumnEnum, "SystemType", "LastResetCause", "PowerRestorePolicy", "PowerMode")
	add("chassis", ColumnEnum, "ChassisType", "ThermalDirection", "PhysicalSecurity.IntrusionSensor")
	add("manager", ColumnEnum, "ManagerType", "DateTimeSource")
	add("processor", ColumnEnum, "ProcessorType", "ProcessorArchitecture", "InstructionSet", "TurboState")
	add("memory", ColumnEnum, "MemoryType", "MemoryDeviceType", "BaseModuleType", "ErrorCorrection", "SecurityState")
	add("storage", ColumnEnum, "EncryptionMode")
	add("storage_controller", ColumnEnum, "PCIeInterface.PCIeType", "NVMeControllerProperties.ControllerType")
	add("drive", ColumnEnum, "MediaType", "Protocol", "DriveFormFactor", "SlotFormFactor", "HotspareType", "EncryptionStatus")
	add("volume", ColumnEnum, "RAIDType", "VolumeType", "VolumeUsage", "ReadCachePolicy", "WriteCachePolicy", "WriteCacheState")
	add("network_device_function", ColumnEnum, "NetDevFuncType")
	add("ethernet_interface", ColumnEnum, "LinkStatus", "EthernetInterfaceType")
	add("network_port", ColumnEnum, "ActiveLinkTechnology", "LinkStatus")
	add("port", ColumnEnum, "PortProtocol", "PortType", "PortMedium", "LinkNetworkTechnology", "LinkState", "LinkStatus")
	add("pcie_device", ColumnEnum,
		"DeviceType", "PCIeInterface.PCIeType", "PCIeInterface.MaxPCIeType", "Slot.LaneSplitting", "Slot.PCIeType",
		"Slot.SlotType", "Slot.Location.PartLocation.LocationType", "Slot.Location.PartLocation.Orientation",
		"Slot.Location.PartLocation.Reference", "Slot.Location.Placement.RackOffsetUnits")
	add("pcie_function", ColumnEnum, "FunctionType", "FunctionProtocol", "DeviceClass")
	add("pump", ColumnEnum, "PumpType")
	add("power_supply", ColumnEnum,
		"PowerSupplyType", "LineInputStatus", "PhaseWiringType", "PlugType", "InputNominalVoltageType", "OutputNominalVoltageType")
	add("battery", ColumnEnum, "BatteryChemistryType", "EnergyStorageType", "ChargeState")
	add("coolant_connector", ColumnEnum, "CoolantConnectorType", "Coolant.CoolantType")
	add("leak_detector", ColumnEnum, "LeakDetectorType", "DetectorState", "WarningReactionType", "CriticalReactionType")
	add("control", ColumnEnum, "ControlType", "ControlMode", "Implementation")
	add("redundancy", ColumnEnum, "Mode", "RedundancyType")
	add("firmware", ColumnEnum, "VersionScheme", "ReleaseType")
	add("software", ColumnEnum, "VersionScheme", "ReleaseType")
	add("log_service", ColumnEnum, "LogEntryType", "OverWritePolicy")

	add("chassis", ColumnFloat, "HeightRackUnits")
	add("processor", ColumnInteger, "TDPWatts", "MaxTDPWatts")
	add("drive", ColumnFloat, "RotationSpeedRPM")
	add("volume", ColumnInteger, "RemainingCapacityPercent")
	add("network_device_function", ColumnInteger, "VirtualFunctionAllocation")
	add("network_port", ColumnString, "PhysicalPortNumber")
	add("pcie_device", ColumnInteger, "Slot.Location.Placement.RackOffset")
	add("pcie_function", ColumnInteger, "FunctionId")
	add("pcie_function", ColumnString, "FunctionNumber", "SegmentNumber", "BusNumber", "DeviceNumber")
	add("fan", ColumnInteger, "FanDiameterMm")
	add("pump", ColumnFloat, "ServiceHours")
	add("coolant_connector", ColumnFloat, "Coolant.RatedServiceHours", "Coolant.ServiceHours")
	add("filter", ColumnFloat, "ServiceHours", "RatedServiceHours")
	add("battery", ColumnFloat,
		"CapacityActualAmpHours", "CapacityRatedAmpHours", "CapacityActualWattHours", "CapacityRatedWattHours")
	add("thermal_subsystem", ColumnBoolean, "FansFullSpeedOverrideEnable")
	add("log_service", ColumnBoolean, "Overflow", "Persistency")
	add("manager", ColumnTimestamp, "DateTime")
	add("memory", ColumnString, "DeviceID")

	add("network_device_function", ColumnString, "NetDevFuncCapabilities")
	structured("network_device_function", "NetDevFuncCapabilities")
	add("log_service", ColumnString, "LogPurposes")
	structured("log_service", "LogPurposes")

	source("storage_controller", "SpeedGbps", ColumnFloat, ColumnInteger)
	source("drive", "NegotiatedSpeedGbs", ColumnFloat, ColumnInteger)
	source("drive", "CapableSpeedGbs", ColumnFloat, ColumnInteger)
	source("port", "CurrentSpeedGbps", ColumnFloat, ColumnInteger)
	source("port", "ConfiguredSpeedGbps", ColumnFloat, ColumnInteger)
	source("port", "MaxSpeedGbps", ColumnFloat, ColumnInteger)

	if len(want) != 120 {
		t.Fatalf("schema-grounded correction count = %d, want 120", len(want))
	}
	actual := make(map[key]InventoryFieldSpec)
	for _, field := range MustCompile().Inventory {
		actual[key{kind: field.Kind, path: field.Path}] = field
	}
	for id, expectation := range want {
		field, ok := actual[id]
		if !ok {
			t.Errorf("schema field %s.%s is missing", id.kind, id.path)
			continue
		}
		if field.Type != expectation.typ || field.SourceType != expectation.sourceType || field.Structured != expectation.structured {
			t.Errorf(
				"schema field %s.%s = type:%s source_type:%s structured:%t, want type:%s source_type:%s structured:%t",
				id.kind, id.path, field.Type, field.SourceType, field.Structured,
				expectation.typ, expectation.sourceType, expectation.structured,
			)
		}
	}
	if _, ok := actual[key{kind: "memory", path: "DeviceId"}]; ok {
		t.Error(fmt.Sprintf("obsolete schema spelling memory.%s remains declared", "DeviceId"))
	}
}
