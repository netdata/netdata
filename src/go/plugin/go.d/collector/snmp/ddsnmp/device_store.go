// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
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

// DeviceLifecycleInfo is the credential-free configured identity of an SNMP
// job. It is available before the job has enough device state for topology.
type DeviceLifecycleInfo struct {
	Hostname    string
	Port        int
	SNMPVersion string
}

// DeviceLifecyclePhase identifies the last completed collector lifecycle call.
type DeviceLifecyclePhase uint8

const (
	DeviceLifecyclePhaseUnknown DeviceLifecyclePhase = iota
	DeviceLifecyclePhaseInit
	DeviceLifecyclePhaseCheck
	DeviceLifecyclePhaseCollect
)

// DeviceLifecycleOutcome identifies the result of a completed lifecycle call.
type DeviceLifecycleOutcome uint8

const (
	DeviceLifecycleOutcomeUnknown DeviceLifecycleOutcome = iota
	DeviceLifecycleOutcomeSuccess
	DeviceLifecycleOutcomeFailed
)

// DeviceLifecycleStatus describes the last completed lifecycle call.
type DeviceLifecycleStatus struct {
	Phase       DeviceLifecyclePhase
	Outcome     DeviceLifecycleOutcome
	CompletedAt time.Time
}

// DeviceLifecycleEntry is one current normal-SNMP job incarnation.
type DeviceLifecycleEntry struct {
	RegistrationID DeviceRegistrationID
	Info           DeviceLifecycleInfo
	LastCompleted  DeviceLifecycleStatus
	TopologyReady  bool
}

// DeviceLifecycleCut is an immutable, independently sequenced snapshot of all
// current normal-SNMP job incarnations.
type DeviceLifecycleCut struct {
	Sequence   uint64
	CapturedAt time.Time
	Entries    []DeviceLifecycleEntry
}

type deviceLifecycleRecord struct {
	info          DeviceLifecycleInfo
	lastCompleted DeviceLifecycleStatus
}

// NewDeviceStore returns an empty SNMP device connection-state store.
func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		changes:            make(chan struct{}, 1),
		ownerRegistrations: make(map[string]DeviceRegistrationID),
		devices:            make(map[DeviceRegistrationID]DeviceConnectionInfo),
		lifecycles:         make(map[DeviceRegistrationID]deviceLifecycleRecord),
		byHostname:         make(map[string]map[string]struct{}),
	}
}

// DeviceStore holds SNMP device connection state shared between SNMP-family modules.
type DeviceStore struct {
	mu                    sync.RWMutex
	changes               chan struct{}
	writers               map[string]*DeviceWriter
	ownerRegistrations    map[string]DeviceRegistrationID
	devices               map[DeviceRegistrationID]DeviceConnectionInfo
	lifecycles            map[DeviceRegistrationID]deviceLifecycleRecord
	byHostname            map[string]map[string]struct{}
	lastRegistrationID    DeviceRegistrationID
	configurationRevision uint64
	lifecycleSequence     uint64
}

// RegisterJob starts or updates the credential-free lifecycle row for a
// normal-SNMP job. Its registration ID is retained if connection state is
// registered later.
func (s *DeviceStore) RegisterJob(ownerKey string, info DeviceLifecycleInfo) {
	s.mu.Lock()
	s.ensureMapsLocked()
	registrationID, exists := s.ownerRegistrations[ownerKey]
	if !exists {
		registrationID = s.nextRegistrationIDLocked()
		s.ownerRegistrations[ownerKey] = registrationID
	}
	record := s.lifecycles[registrationID]
	record.info = info
	s.lifecycles[registrationID] = record
	s.lifecycleSequence++
	s.mu.Unlock()
}

// RecordJobLifecycle updates the last completed lifecycle call for a current
// normal-SNMP job. Unknown owner keys are ignored so diagnostics cannot create
// a second, incomplete incarnation.
func (s *DeviceStore) RecordJobLifecycle(ownerKey string, status DeviceLifecycleStatus) {
	s.mu.Lock()
	if registrationID, ok := s.ownerRegistrations[ownerKey]; ok {
		record := s.lifecycles[registrationID]
		record.lastCompleted = status
		s.lifecycles[registrationID] = record
		s.lifecycleSequence++
	}
	s.mu.Unlock()
}

// LifecycleCut returns a deterministic snapshot of all current normal-SNMP
// jobs, including jobs that are not yet topology-ready.
func (s *DeviceStore) LifecycleCut() DeviceLifecycleCut {
	s.mu.RLock()
	defer s.mu.RUnlock()

	registrationIDs := make([]DeviceRegistrationID, 0, len(s.ownerRegistrations))
	for _, registrationID := range s.ownerRegistrations {
		registrationIDs = append(registrationIDs, registrationID)
	}
	slices.Sort(registrationIDs)

	cut := DeviceLifecycleCut{
		Sequence:   s.lifecycleSequence,
		CapturedAt: time.Now(),
		Entries:    make([]DeviceLifecycleEntry, 0, len(registrationIDs)),
	}
	for _, registrationID := range registrationIDs {
		record := s.lifecycles[registrationID]
		_, topologyReady := s.devices[registrationID]
		cut.Entries = append(cut.Entries, DeviceLifecycleEntry{
			RegistrationID: registrationID,
			Info:           record.info,
			LastCompleted:  record.lastCompleted,
			TopologyReady:  topologyReady,
		})
	}
	return cut
}

// ReplaceJob atomically removes a prior configuration incarnation, when
// different, and publishes the current lifecycle plus any topology-ready state.
// The returned writer replaces the prior runtime's update authority.
func (s *DeviceStore) ReplaceJob(
	previousOwnerKey string,
	ownerKey string,
	info DeviceLifecycleInfo,
	status DeviceLifecycleStatus,
	device *DeviceConnectionInfo,
) *DeviceWriter {
	if ownerKey == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureMapsLocked()
	if previousOwnerKey != "" && previousOwnerKey != ownerKey {
		s.removeRegistrationLocked(previousOwnerKey)
	}
	registrationID, exists := s.ownerRegistrations[ownerKey]
	if !exists {
		registrationID = s.nextRegistrationIDLocked()
		s.ownerRegistrations[ownerKey] = registrationID
	} else if device, ok := s.devices[registrationID]; ok {
		s.removeHostnameIndexLocked(ownerKey, device.Hostname)
		delete(s.devices, registrationID)
	}
	s.lifecycles[registrationID] = deviceLifecycleRecord{
		info:          info,
		lastCompleted: status,
	}
	if device != nil {
		cloned := cloneDeviceConnectionInfo(*device)
		s.devices[registrationID] = cloned
		s.addHostnameIndexLocked(ownerKey, cloned.Hostname)
	}
	s.lifecycleSequence++
	s.configurationRevision++
	s.notifyConfigurationChangedLocked()
	writer := &DeviceWriter{store: s, owner: ownerKey}
	if s.writers == nil {
		s.writers = make(map[string]*DeviceWriter)
	}
	s.writers[ownerKey] = writer
	s.mu.Unlock()
	return writer
}

// Register adds or updates a device by its caller-owned lookup key. An update
// retains the current registration ID; a new registration receives a new ID.
// Reference types are deep-copied to prevent data races with the caller.
func (s *DeviceStore) Register(ownerKey string, info DeviceConnectionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerLocked(ownerKey, info)
}

func (s *DeviceStore) registerLocked(ownerKey string, info DeviceConnectionInfo) {
	s.ensureMapsLocked()
	registrationID, exists := s.ownerRegistrations[ownerKey]
	if exists {
		s.removeHostnameIndexLocked(ownerKey, s.devices[registrationID].Hostname)
	} else {
		registrationID = s.nextRegistrationIDLocked()
		s.ownerRegistrations[ownerKey] = registrationID
	}
	s.devices[registrationID] = cloneDeviceConnectionInfo(info)
	record := s.lifecycles[registrationID]
	record.info = lifecycleInfoFromDeviceConnection(info)
	s.lifecycles[registrationID] = record
	s.addHostnameIndexLocked(ownerKey, info.Hostname)
	s.lifecycleSequence++
}

// Unregister removes a complete job incarnation from the store.
func (s *DeviceStore) Unregister(ownerKey string) {
	s.mu.Lock()
	if _, ok := s.ownerRegistrations[ownerKey]; ok {
		s.removeRegistrationLocked(ownerKey)
		s.configurationRevision++
		s.notifyConfigurationChangedLocked()
		s.lifecycleSequence++
	}
	s.mu.Unlock()
}

func (s *DeviceStore) removeRegistrationLocked(ownerKey string) {
	registrationID, ok := s.ownerRegistrations[ownerKey]
	if !ok {
		return
	}
	if info, exists := s.devices[registrationID]; exists {
		s.removeHostnameIndexLocked(ownerKey, info.Hostname)
	}
	delete(s.devices, registrationID)
	delete(s.lifecycles, registrationID)
	delete(s.ownerRegistrations, ownerKey)
	delete(s.writers, ownerKey)
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

func lifecycleInfoFromDeviceConnection(info DeviceConnectionInfo) DeviceLifecycleInfo {
	return DeviceLifecycleInfo{
		Hostname:    info.Hostname,
		Port:        info.Port,
		SNMPVersion: info.SNMPVersion,
	}
}

func (s *DeviceStore) ensureMapsLocked() {
	if s.ownerRegistrations == nil {
		s.ownerRegistrations = make(map[string]DeviceRegistrationID)
	}
	if s.devices == nil {
		s.devices = make(map[DeviceRegistrationID]DeviceConnectionInfo)
	}
	if s.lifecycles == nil {
		s.lifecycles = make(map[DeviceRegistrationID]deviceLifecycleRecord)
		for _, registrationID := range s.ownerRegistrations {
			if info, ok := s.devices[registrationID]; ok {
				s.lifecycles[registrationID] = deviceLifecycleRecord{info: lifecycleInfoFromDeviceConnection(info)}
			}
		}
	}
	if s.byHostname == nil {
		s.byHostname = make(map[string]map[string]struct{})
		for ownerKey, registrationID := range s.ownerRegistrations {
			if info, ok := s.devices[registrationID]; ok {
				s.addHostnameIndexLocked(ownerKey, info.Hostname)
			}
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

// DeviceWriter is authority for one accepted runtime. Replacement/removal
// revokes old writers even when the exact configuration identity is reused.
type DeviceWriter struct {
	store *DeviceStore
	owner string
}

func (w *DeviceWriter) UpdateDevice(info DeviceConnectionInfo) {
	if w == nil {
		return
	}
	s := w.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writers[w.owner] == w {
		s.registerLocked(w.owner, info)
	}
}

func (w *DeviceWriter) RecordLifecycle(status DeviceLifecycleStatus) {
	if w == nil {
		return
	}
	s := w.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writers[w.owner] != w {
		return
	}
	id := s.ownerRegistrations[w.owner]
	record := s.lifecycles[id]
	record.lastCompleted = status
	s.lifecycles[id] = record
	s.lifecycleSequence++
}

// ConfigurationRevision fences snapshots across accepted job replacement/removal.
func (s *DeviceStore) ConfigurationRevision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configurationRevision
}

func (s *DeviceStore) ConfigurationChanges() <-chan struct{} { return s.changes }
func (s *DeviceStore) notifyConfigurationChangedLocked() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}
