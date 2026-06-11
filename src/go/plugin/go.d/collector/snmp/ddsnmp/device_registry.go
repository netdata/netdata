// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"maps"
	"net/netip"
	"sort"
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
	byHostname: make(map[string]map[string]struct{}),
}

type deviceRegistry struct {
	mu         sync.RWMutex
	devices    map[string]DeviceConnectionInfo
	byHostname map[string]map[string]struct{}
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
	r.addHostnameIndexLocked(key, info.Hostname)
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
	devices := r.DevicesByHostname(hostname)
	if len(devices) == 0 {
		return DeviceConnectionInfo{}, false
	}
	return devices[0], true
}

// DevicesByHostname returns all deep-copied registered devices whose configured
// hostname matches the provided value. IP literals are normalized before
// comparison; DNS names are matched case-insensitively.
func (r *deviceRegistry) DevicesByHostname(hostname string) []DeviceConnectionInfo {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.byHostname != nil {
		keySet := r.byHostname[hostnameKey]
		if len(keySet) == 0 {
			return nil
		}
		keys := make([]string, 0, len(keySet))
		for key := range keySet {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		devices := make([]DeviceConnectionInfo, 0, len(keys))
		for _, key := range keys {
			info, ok := r.devices[key]
			if ok {
				devices = append(devices, cloneDeviceConnectionInfo(info))
			}
		}
		return devices
	}

	devices := make([]DeviceConnectionInfo, 0, 1)
	for _, info := range r.devices {
		if deviceHostnameIndexKey(info.Hostname) == hostnameKey {
			devices = append(devices, cloneDeviceConnectionInfo(info))
		}
	}
	return devices
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
		r.byHostname = make(map[string]map[string]struct{})
		for key, info := range r.devices {
			r.addHostnameIndexLocked(key, info.Hostname)
		}
	}
}

func (r *deviceRegistry) addHostnameIndexLocked(key, hostname string) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" {
		return
	}
	if r.byHostname == nil {
		r.byHostname = make(map[string]map[string]struct{})
	}
	keySet := r.byHostname[hostnameKey]
	if keySet == nil {
		keySet = make(map[string]struct{})
		r.byHostname[hostnameKey] = keySet
	}
	keySet[key] = struct{}{}
}

func (r *deviceRegistry) removeHostnameIndexLocked(key, hostname string) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" || r.byHostname == nil {
		return
	}
	keySet := r.byHostname[hostnameKey]
	if keySet == nil {
		return
	}
	delete(keySet, key)
	if len(keySet) == 0 {
		delete(r.byHostname, hostnameKey)
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
