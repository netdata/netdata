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
// Registered by SNMP collector jobs, consumed by SNMP-family modules.
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

// NewDeviceStore returns an empty SNMP device connection-state store.
func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		devices:    make(map[string]DeviceConnectionInfo),
		byHostname: make(map[string]map[string]struct{}),
	}
}

// DeviceStore holds SNMP device connection state shared between SNMP-family modules.
type DeviceStore struct {
	mu         sync.RWMutex
	devices    map[string]DeviceConnectionInfo
	byHostname map[string]map[string]struct{}
}

// Register adds or updates a device in the store.
// Reference types are deep-copied to prevent data races with the caller.
func (s *DeviceStore) Register(key string, info DeviceConnectionInfo) {
	s.mu.Lock()
	s.ensureMapsLocked()
	if old, ok := s.devices[key]; ok {
		s.removeHostnameIndexLocked(key, old.Hostname)
	}
	s.devices[key] = cloneDeviceConnectionInfo(info)
	s.addHostnameIndexLocked(key, info.Hostname)
	s.mu.Unlock()
}

// Unregister removes a device from the store.
func (s *DeviceStore) Unregister(key string) {
	s.mu.Lock()
	if old, ok := s.devices[key]; ok {
		s.removeHostnameIndexLocked(key, old.Hostname)
	}
	delete(s.devices, key)
	s.mu.Unlock()
}

// Devices returns a deep-copied snapshot of all registered devices.
func (s *DeviceStore) Devices() []DeviceConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]DeviceConnectionInfo, 0, len(s.devices))
	for _, info := range s.devices {
		devices = append(devices, cloneDeviceConnectionInfo(info))
	}
	return devices
}

// DevicesByHostname returns all deep-copied registered devices whose configured
// hostname matches the provided value. IP literals are normalized before
// comparison; DNS names are matched case-insensitively.
func (s *DeviceStore) DevicesByHostname(hostname string) []DeviceConnectionInfo {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.byHostname != nil {
		keySet := s.byHostname[hostnameKey]
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
			info, ok := s.devices[key]
			if ok {
				devices = append(devices, cloneDeviceConnectionInfo(info))
			}
		}
		return devices
	}

	devices := make([]DeviceConnectionInfo, 0, 1)
	for _, info := range s.devices {
		if deviceHostnameIndexKey(info.Hostname) == hostnameKey {
			devices = append(devices, cloneDeviceConnectionInfo(info))
		}
	}
	return devices
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

func (s *DeviceStore) ensureMapsLocked() {
	if s.devices == nil {
		s.devices = make(map[string]DeviceConnectionInfo)
	}
	if s.byHostname == nil {
		s.byHostname = make(map[string]map[string]struct{})
		for key, info := range s.devices {
			s.addHostnameIndexLocked(key, info.Hostname)
		}
	}
}

func (s *DeviceStore) addHostnameIndexLocked(key, hostname string) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" {
		return
	}
	if s.byHostname == nil {
		s.byHostname = make(map[string]map[string]struct{})
	}
	keySet := s.byHostname[hostnameKey]
	if keySet == nil {
		keySet = make(map[string]struct{})
		s.byHostname[hostnameKey] = keySet
	}
	keySet[key] = struct{}{}
}

func (s *DeviceStore) removeHostnameIndexLocked(key, hostname string) {
	hostnameKey := deviceHostnameIndexKey(hostname)
	if hostnameKey == "" || s.byHostname == nil {
		return
	}
	keySet := s.byHostname[hostnameKey]
	if keySet == nil {
		return
	}
	delete(keySet, key)
	if len(keySet) == 0 {
		delete(s.byHostname, hostnameKey)
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
