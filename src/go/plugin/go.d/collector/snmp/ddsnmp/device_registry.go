// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "sync"

// DeviceConnectionInfo holds SNMP connection parameters for a device.
// Registered by SNMP collector jobs, consumed by the topology collector.
type DeviceConnectionInfo struct {
	Hostname        string
	Port            int
	SNMPVersion     string
	Community       string
	V3User          string
	V3SecurityLevel string
	V3AuthProto     string
	V3AuthKey       string
	V3PrivProto     string
	V3PrivKey       string
	MaxRepetitions  uint32
	MaxOIDs         int
	Timeout         int
	Retries         int
	SysObjectID     string
	SysDescr        string
	SysName         string
	SysContact      string
	SysLocation     string
	Vendor          string
	Model           string

	DisableBulkWalk bool

	ManualProfiles    []string
	TopologyAutoprobe bool

	VnodeGUID   string
	VnodeLabels map[string]string
}

// DeviceRegistry is a global registry where SNMP jobs register their connection
// info so the topology collector can discover which devices to poll.
var DeviceRegistry = &deviceRegistry{
	devices: make(map[string]DeviceConnectionInfo),
}

type deviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]DeviceConnectionInfo
}

// Register adds or updates a device in the registry.
func (r *deviceRegistry) Register(key string, info DeviceConnectionInfo) {
	r.mu.Lock()
	r.devices[key] = info
	r.mu.Unlock()
}

// Unregister removes a device from the registry.
func (r *deviceRegistry) Unregister(key string) {
	r.mu.Lock()
	delete(r.devices, key)
	r.mu.Unlock()
}

// Devices returns a snapshot of all registered devices.
func (r *deviceRegistry) Devices() []DeviceConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]DeviceConnectionInfo, 0, len(r.devices))
	for _, info := range r.devices {
		devices = append(devices, info)
	}
	return devices
}

// Len returns the number of registered devices.
func (r *deviceRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}
