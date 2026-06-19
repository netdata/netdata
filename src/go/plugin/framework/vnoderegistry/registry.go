// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnoderegistry tracks v2 vnode HOST_DEFINE metadata shared across jobs.
package vnoderegistry

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

const maxReportedMetadataStatesPerGUID = 64

// Owner identifies one runtime owner of a vnode GUID.
//
// Jobruntime v2 uses stable per-job/per-scope owner IDs so a job can release
// registry ownership after it has emitted cleanup for the corresponding scope.
type Owner string

// Registration reports the result of registering vnode metadata for an owner.
type Registration struct {
	// Info is the metadata retained by the registry after registration.
	Info netdataapi.HostInfo

	// Previous is set when an existing GUID's metadata was updated.
	Previous netdataapi.HostInfo

	// NeedDefine is true when this registration created or updated the registry entry.
	NeedDefine bool

	// OwnerAdded is true when this call added a new owner record.
	OwnerAdded bool

	// MetadataUpdated is true when Info replaced previously retained metadata.
	MetadataUpdated bool

	// UpdateFirstSeen is true only for the first occurrence of a distinct metadata
	// transition. Callers should use this to avoid log spam.
	UpdateFirstSeen bool

	revision         uint64
	previousRevision uint64
}

type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	info           netdataapi.HostInfo
	revision       uint64
	owners         map[Owner]struct{}
	reportedStates map[string]struct{}
	reportedOrder  []string
}

// New returns an empty concurrency-safe vnode registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Register records that owner emits metrics under info.GUID.
//
// New metadata for an existing GUID replaces the retained metadata. This keeps
// runtime vnode updates simple: callers should log MetadataUpdated as a warning
// because repeated conflicting writers can still cause metadata flip-flop.
func (r *Registry) Register(owner Owner, info netdataapi.HostInfo) (Registration, error) {
	if r == nil {
		return Registration{}, fmt.Errorf("vnoderegistry: nil registry")
	}
	owner = Owner(strings.TrimSpace(string(owner)))
	if owner == "" {
		return Registration{}, fmt.Errorf("vnoderegistry: owner is required")
	}
	info, err := chartemit.PrepareHostInfo(info)
	if err != nil {
		return Registration{}, fmt.Errorf("vnoderegistry: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries == nil {
		r.entries = make(map[string]*entry)
	}

	ent, ok := r.entries[info.GUID]
	if !ok {
		ent = &entry{
			info:           cloneHostInfo(info),
			revision:       1,
			owners:         map[Owner]struct{}{owner: {}},
			reportedStates: make(map[string]struct{}),
		}
		r.entries[info.GUID] = ent
		return Registration{
			Info:       cloneHostInfo(ent.info),
			NeedDefine: true,
			OwnerAdded: true,
			revision:   ent.revision,
		}, nil
	}

	_, hadOwner := ent.owners[owner]
	ent.owners[owner] = struct{}{}

	result := Registration{
		Info:       cloneHostInfo(ent.info),
		OwnerAdded: !hadOwner,
		revision:   ent.revision,
	}
	if hostInfoEqual(ent.info, info) {
		return result, nil
	}

	previous := cloneHostInfo(ent.info)
	previousRevision := ent.revision
	ent.info = cloneHostInfo(info)
	ent.revision++
	result.Info = cloneHostInfo(ent.info)
	result.Previous = previous
	result.revision = ent.revision
	result.previousRevision = previousRevision
	result.NeedDefine = true
	result.MetadataUpdated = true

	updateKey := hostInfoFingerprint(info)
	if markReportedState(ent, updateKey) {
		result.UpdateFirstSeen = true
	}
	return result, nil
}

// Rollback undoes a registration that has not been emitted successfully.
//
// Rollback is best-effort: if another registration changed the same GUID after
// reg, the metadata restore is skipped to avoid undoing a later writer.
func (r *Registry) Rollback(owner Owner, reg Registration) {
	if r == nil {
		return
	}
	owner = Owner(strings.TrimSpace(string(owner)))
	guid := strings.TrimSpace(reg.Info.GUID)
	if owner == "" || guid == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ent, ok := r.entries[guid]
	if !ok {
		return
	}
	if reg.OwnerAdded {
		delete(ent.owners, owner)
	}
	if reg.MetadataUpdated && ent.revision == reg.revision && hostInfoEqual(ent.info, reg.Info) {
		ent.info = cloneHostInfo(reg.Previous)
		ent.revision = reg.previousRevision
	}
	if len(ent.owners) == 0 {
		delete(r.entries, guid)
	}
}

// Release removes one owner record for guid. It returns true when the GUID entry
// was removed because no owners remain.
func (r *Registry) Release(owner Owner, guid string) bool {
	if r == nil {
		return false
	}
	owner = Owner(strings.TrimSpace(string(owner)))
	guid = strings.TrimSpace(guid)
	if owner == "" || guid == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ent, ok := r.entries[guid]
	if !ok {
		return false
	}
	delete(ent.owners, owner)
	if len(ent.owners) > 0 {
		return false
	}
	delete(r.entries, guid)
	return true
}

// Lookup returns the retained metadata for guid.
func (r *Registry) Lookup(guid string) (netdataapi.HostInfo, bool) {
	if r == nil {
		return netdataapi.HostInfo{}, false
	}
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return netdataapi.HostInfo{}, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ent, ok := r.entries[guid]
	if !ok {
		return netdataapi.HostInfo{}, false
	}
	return cloneHostInfo(ent.info), true
}

// Owners returns the sorted owner IDs currently registered for guid.
func (r *Registry) Owners(guid string) []Owner {
	if r == nil {
		return nil
	}
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ent, ok := r.entries[guid]
	if !ok {
		return nil
	}
	owners := make([]Owner, 0, len(ent.owners))
	for owner := range ent.owners {
		owners = append(owners, owner)
	}
	slices.Sort(owners)
	return owners
}

// Len returns the number of retained GUID entries.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func cloneHostInfo(info netdataapi.HostInfo) netdataapi.HostInfo {
	return netdataapi.HostInfo{
		GUID:     info.GUID,
		Hostname: info.Hostname,
		Labels:   maps.Clone(info.Labels),
	}
}

func hostInfoEqual(left, right netdataapi.HostInfo) bool {
	return left.GUID == right.GUID &&
		left.Hostname == right.Hostname &&
		maps.Equal(left.Labels, right.Labels)
}

func hostInfoFingerprint(info netdataapi.HostInfo) string {
	var b strings.Builder
	writeHostInfoFingerprint(&b, info)
	return b.String()
}

func writeHostInfoFingerprint(b *strings.Builder, info netdataapi.HostInfo) {
	writeFingerprintPart(b, info.GUID)
	writeFingerprintPart(b, info.Hostname)

	keys := make([]string, 0, len(info.Labels))
	for key := range info.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeFingerprintPart(b, key)
		writeFingerprintPart(b, info.Labels[key])
	}
}

func writeFingerprintPart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('\xff')
}

func markReportedState(ent *entry, key string) bool {
	if ent.reportedStates == nil {
		ent.reportedStates = make(map[string]struct{})
	}
	if _, seen := ent.reportedStates[key]; seen {
		return false
	}
	if len(ent.reportedOrder) >= maxReportedMetadataStatesPerGUID {
		evicted := ent.reportedOrder[0]
		copy(ent.reportedOrder, ent.reportedOrder[1:])
		ent.reportedOrder = ent.reportedOrder[:len(ent.reportedOrder)-1]
		delete(ent.reportedStates, evicted)
	}
	ent.reportedStates[key] = struct{}{}
	ent.reportedOrder = append(ent.reportedOrder, key)
	return true
}
