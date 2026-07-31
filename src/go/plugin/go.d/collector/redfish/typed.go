// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/stmcginnis/gofish/schemas"
)

var typedResourceFactories = map[string]func() any{
	"system":                          newTypedResource[schemas.ComputerSystem],
	"chassis":                         newTypedResource[schemas.Chassis],
	"manager":                         newTypedResource[schemas.Manager],
	"processor":                       newTypedResource[schemas.Processor],
	"processor_metrics":               newTypedResource[schemas.ProcessorMetrics],
	"processor_summary_metrics":       newTypedResource[schemas.ProcessorMetrics],
	"memory":                          newTypedResource[schemas.Memory],
	"memory_metrics":                  newTypedResource[schemas.MemoryMetrics],
	"memory_summary_metrics":          newTypedResource[schemas.MemoryMetrics],
	"storage":                         newTypedResource[schemas.Storage],
	"storage_metrics":                 newTypedResource[schemas.StorageMetrics],
	"storage_controller":              newTypedResource[schemas.StorageController],
	"storage_controller_metrics":      newTypedResource[schemas.StorageControllerMetrics],
	"drive":                           newTypedResource[schemas.Drive],
	"drive_metrics":                   newTypedResource[schemas.DriveMetrics],
	"volume":                          newTypedResource[schemas.Volume],
	"volume_metrics":                  newTypedResource[schemas.VolumeMetrics],
	"network_adapter":                 newTypedResource[schemas.NetworkAdapter],
	"network_adapter_metrics":         newTypedResource[schemas.NetworkAdapterMetrics],
	"network_device_function":         newTypedResource[schemas.NetworkDeviceFunction],
	"network_device_function_metrics": newTypedResource[schemas.NetworkDeviceFunctionMetrics],
	"ethernet_interface":              newTypedResource[schemas.EthernetInterface],
	"network_interface":               newTypedResource[schemas.NetworkInterface],
	"network_port":                    newTypedResource[schemas.NetworkPort],
	"port":                            newTypedResource[schemas.Port],
	"port_metrics":                    newTypedResource[schemas.PortMetrics],
	"pcie_device":                     newTypedResource[schemas.PCIeDevice],
	"pcie_function":                   newTypedResource[schemas.PCIeFunction],
	"fan":                             newTypedResource[schemas.Fan],
	"pump":                            newTypedResource[schemas.Pump],
	"power_supply":                    newTypedResource[schemas.PowerSupplyUnit],
	"power_supply_metrics":            newTypedResource[schemas.PowerSupplyMetrics],
	"battery":                         newTypedResource[schemas.Battery],
	"battery_metrics":                 newTypedResource[schemas.BatteryMetrics],
	"sensor":                          newTypedResource[schemas.Sensor],
	"thermal_subsystem":               newTypedResource[schemas.ThermalSubsystem],
	"thermal_metrics":                 newTypedResource[schemas.ThermalMetrics],
	"power_subsystem":                 newTypedResource[schemas.PowerSubsystem],
	"coolant_connector":               newTypedResource[schemas.CoolantConnector],
	"filter":                          newTypedResource[schemas.Filter],
	"heater":                          newTypedResource[schemas.Heater],
	"heater_metrics":                  newTypedResource[schemas.HeaterMetrics],
	"leak_detection":                  newTypedResource[schemas.LeakDetection],
	"leak_detector":                   newTypedResource[schemas.LeakDetector],
	"control":                         newTypedResource[schemas.Control],
	"environment_metrics":             newTypedResource[schemas.EnvironmentMetrics],
	"firmware":                        newTypedResource[schemas.SoftwareInventory],
	"software":                        newTypedResource[schemas.SoftwareInventory],
	"assembly":                        newTypedResource[schemas.Assembly],
	"assembly_document":               newTypedResource[schemas.Assembly],
	"log_service":                     newTypedResource[schemas.LogService],
	"legacy_thermal":                  newTypedResource[schemas.Thermal],
	"legacy_power":                    newTypedResource[schemas.Power],
	"update_service":                  newTypedResource[schemas.UpdateService],
}

func newTypedResource[T any]() any { return new(T) }

// decodeTypedResource validates and retains the gofish semantic representation
// for every standard resource kind used by the collector. The separate
// UseNumber raw envelope remains authoritative for property presence, exact
// cumulative totals, standard annotations, and fields not modeled by gofish.
func decodeTypedResource(kind string, raw []byte) (any, error) {
	var envelope struct {
		ODataType string `json:"@odata.type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode %s resource envelope: %w", kind, err)
	}
	if err := validateResourceSchemaType(kind, envelope.ODataType); err != nil {
		return nil, err
	}

	factory, ok := typedResourceFactories[kind]
	if !ok {
		return nil, nil
	}
	target := factory()
	if err := json.Unmarshal(raw, target); err != nil {
		return nil, fmt.Errorf("decode typed %s resource: %w", kind, err)
	}
	clearTypedRawData(target)
	return target, nil
}

func clearTypedRawData(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return
	}
	field := value.FieldByName("RawData")
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.Slice {
		field.SetZero()
	}
}
