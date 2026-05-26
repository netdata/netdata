// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"maps"
	"net/netip"
	"strings"
	"sync"
)

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

	VnodeGUID     string
	VnodeHostname string
	VnodeLabels   map[string]string
}

// DeviceRegistry is a global registry where SNMP jobs register their connection
// info so the topology collector can discover which devices to poll.
var DeviceRegistry = &deviceRegistry{
	devices:    make(map[string]DeviceConnectionInfo),
	byHostname: make(map[string]string),
}

type deviceRegistry struct {
	mu         sync.RWMutex
	devices    map[string]DeviceConnectionInfo
	byHostname map[string]string
}

// Register adds or updates a device in the registry.
// Reference types are deep-copied to prevent data races with the caller.
func (r *deviceRegistry) Register(key string, info DeviceConnectionInfo) {
	r.mu.Lock()
	r.ensureMapsLocked()
	if old, ok := r.devices[key]; ok {
		r.removeHostnameIndexLocked(key, old.Hostname)
	}
	r.devices[key] = cloneDeviceConnectionInfo(info)
	if hostnameKey := deviceHostnameIndexKey(info.Hostname); hostnameKey != "" {
		r.byHostname[hostnameKey] = key
	}
	r.mu.Unlock()
}

// Unregister removes a device from the registry.
func (r *deviceRegistry) Unregister(key string) {
	r.mu.Lock()
	if old, ok := r.devices[key]; ok {
		r.removeHostnameIndexLocked(key, old.Hostname)
	}
	delete(r.devices, key)
	r.mu.Unlock()
}

// Devices returns a deep-copied snapshot of all registered devices.
func (r *deviceRegistry) Devices() []DeviceConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]DeviceConnectionInfo, 0, len(r.devices))
	for _, info := range r.devices {
		devices = append(devices, cloneDeviceConnectionInfo(info))
	}
	return devices
}

// DeviceByHostname returns a deep-copied registered device whose configured
// hostname matches the provided value. IP literals are normalized before
// comparison; DNS names are matched case-insensitively.
func (r *deviceRegistry) DeviceByHostname(hostname string) (DeviceConnectionInfo, bool) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" {
		return DeviceConnectionInfo{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.byHostname != nil {
		key, ok := r.byHostname[hostnameKey]
		if !ok {
			return DeviceConnectionInfo{}, false
		}
		info, ok := r.devices[key]
		if !ok {
			return DeviceConnectionInfo{}, false
		}
		return cloneDeviceConnectionInfo(info), true
	}

	for _, info := range r.devices {
		if deviceHostnameIndexKey(info.Hostname) == hostnameKey {
			return cloneDeviceConnectionInfo(info), true
		}
	}
	return DeviceConnectionInfo{}, false
}

// Len returns the number of registered devices.
func (r *deviceRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}

func cloneDeviceConnectionInfo(info DeviceConnectionInfo) DeviceConnectionInfo {
	dev := info
	if info.ManualProfiles != nil {
		dev.ManualProfiles = make([]string, len(info.ManualProfiles))
		copy(dev.ManualProfiles, info.ManualProfiles)
	}
	if info.VnodeLabels != nil {
		dev.VnodeLabels = make(map[string]string, len(info.VnodeLabels))
		maps.Copy(dev.VnodeLabels, info.VnodeLabels)
	}
	return dev
}

func (r *deviceRegistry) ensureMapsLocked() {
	if r.devices == nil {
		r.devices = make(map[string]DeviceConnectionInfo)
	}
	if r.byHostname == nil {
		r.byHostname = make(map[string]string)
		for key, info := range r.devices {
			if hostnameKey := deviceHostnameIndexKey(info.Hostname); hostnameKey != "" {
				r.byHostname[hostnameKey] = key
			}
		}
	}
}

func (r *deviceRegistry) removeHostnameIndexLocked(key, hostname string) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" || r.byHostname == nil || r.byHostname[hostnameKey] != key {
		return
	}

	delete(r.byHostname, hostnameKey)
	for otherKey, info := range r.devices {
		if otherKey != key && deviceHostnameIndexKey(info.Hostname) == hostnameKey {
			r.byHostname[hostnameKey] = otherKey
			return
		}
	}
}

func deviceHostnameIndexKey(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return ""
	}

	addr, err := netip.ParseAddr(hostname)
	if err == nil {
		return addr.Unmap().String()
	}

	return strings.ToLower(hostname)
}
