// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "maps"

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
	V3ContextName   string
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

	ManualProfiles []string

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
// Reference types are deep-copied to prevent data races with the caller.
func (r *deviceRegistry) Register(key string, info DeviceConnectionInfo) {
	dev := info
	if info.ManualProfiles != nil {
		dev.ManualProfiles = make([]string, len(info.ManualProfiles))
		copy(dev.ManualProfiles, info.ManualProfiles)
	}
	if info.VnodeLabels != nil {
		dev.VnodeLabels = make(map[string]string, len(info.VnodeLabels))
		maps.Copy(dev.VnodeLabels, info.VnodeLabels)
	}
	r.mu.Lock()
	r.devices[key] = dev
	r.mu.Unlock()
}

// Unregister removes a device from the registry.
func (r *deviceRegistry) Unregister(key string) {
	r.mu.Lock()
	delete(r.devices, key)
	r.mu.Unlock()
}

// Devices returns a deep-copied snapshot of all registered devices.
func (r *deviceRegistry) Devices() []DeviceConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]DeviceConnectionInfo, 0, len(r.devices))
	for _, info := range r.devices {
		dev := info
		if info.ManualProfiles != nil {
			dev.ManualProfiles = make([]string, len(info.ManualProfiles))
			copy(dev.ManualProfiles, info.ManualProfiles)
		}
		if info.VnodeLabels != nil {
			dev.VnodeLabels = make(map[string]string, len(info.VnodeLabels))
			maps.Copy(dev.VnodeLabels, info.VnodeLabels)
		}
		devices = append(devices, dev)
	}
	return devices
}

// Len returns the number of registered devices.
func (r *deviceRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}
