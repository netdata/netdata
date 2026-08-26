// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"maps"
	"net/netip"
	"slices"
	"strconv"
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

// DeviceRegistrationID identifies one uninterrupted registration lifetime.
// IDs are unique within one DeviceStore and never reused.
type DeviceRegistrationID uint64

func (id DeviceRegistrationID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

// DeviceEntry is one registered SNMP job and its connection state.
type DeviceEntry struct {
	RegistrationID DeviceRegistrationID
	Info           DeviceConnectionInfo
}

// NewDeviceStore returns an empty SNMP device connection-state store.
func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		ownerRegistrations: make(map[string]DeviceRegistrationID),
		devices:            make(map[DeviceRegistrationID]DeviceConnectionInfo),
		byHostname:         make(map[string]map[string]struct{}),
	}
}

// DeviceStore holds SNMP device connection state shared between SNMP-family modules.
type DeviceStore struct {
	mu                 sync.RWMutex
	ownerRegistrations map[string]DeviceRegistrationID
	devices            map[DeviceRegistrationID]DeviceConnectionInfo
	byHostname         map[string]map[string]struct{}
	lastRegistrationID DeviceRegistrationID
}

// Register adds or updates a device by its caller-owned lookup key. An update
// retains the current registration ID; a new registration receives a new ID.
// Reference types are deep-copied to prevent data races with the caller.
func (s *DeviceStore) Register(ownerKey string, info DeviceConnectionInfo) {
	s.mu.Lock()
	s.ensureMapsLocked()
	registrationID, exists := s.ownerRegistrations[ownerKey]
	if exists {
		s.removeHostnameIndexLocked(ownerKey, s.devices[registrationID].Hostname)
	} else {
		registrationID = s.nextRegistrationIDLocked()
		s.ownerRegistrations[ownerKey] = registrationID
	}
	s.devices[registrationID] = cloneDeviceConnectionInfo(info)
	s.addHostnameIndexLocked(ownerKey, info.Hostname)
	s.mu.Unlock()
}

// Unregister removes a device from the store.
func (s *DeviceStore) Unregister(ownerKey string) {
	s.mu.Lock()
	if registrationID, ok := s.ownerRegistrations[ownerKey]; ok {
		s.removeHostnameIndexLocked(ownerKey, s.devices[registrationID].Hostname)
		delete(s.devices, registrationID)
		delete(s.ownerRegistrations, ownerKey)
	}
	s.mu.Unlock()
}

// Entries returns a deterministic, deep-copied snapshot of all registered jobs.
func (s *DeviceStore) Entries() []DeviceEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	registrationIDs := make([]DeviceRegistrationID, 0, len(s.devices))
	for registrationID := range s.devices {
		registrationIDs = append(registrationIDs, registrationID)
	}
	slices.Sort(registrationIDs)

	entries := make([]DeviceEntry, 0, len(registrationIDs))
	for _, registrationID := range registrationIDs {
		entries = append(entries, DeviceEntry{
			RegistrationID: registrationID,
			Info:           cloneDeviceConnectionInfo(s.devices[registrationID]),
		})
	}
	return entries
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
		slices.Sort(keys)

		devices := make([]DeviceConnectionInfo, 0, len(keys))
		for _, ownerKey := range keys {
			registrationID, ok := s.ownerRegistrations[ownerKey]
			if ok {
				devices = append(devices, cloneDeviceConnectionInfo(s.devices[registrationID]))
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
	if s.ownerRegistrations == nil {
		s.ownerRegistrations = make(map[string]DeviceRegistrationID)
	}
	if s.devices == nil {
		s.devices = make(map[DeviceRegistrationID]DeviceConnectionInfo)
	}
	if s.byHostname == nil {
		s.byHostname = make(map[string]map[string]struct{})
		for ownerKey, registrationID := range s.ownerRegistrations {
			s.addHostnameIndexLocked(ownerKey, s.devices[registrationID].Hostname)
		}
	}
}

func (s *DeviceStore) nextRegistrationIDLocked() DeviceRegistrationID {
	if s.lastRegistrationID == ^DeviceRegistrationID(0) {
		panic("SNMP DeviceStore registration ID space exhausted")
	}
	s.lastRegistrationID++
	return s.lastRegistrationID
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
