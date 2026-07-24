// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnoderegistry diagnoses conflicting v2 vnode metadata shared across jobs.
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

// Registration reports a successfully emitted owner's registry observation.
type Registration struct {
	// Info is the successfully emitted owner's normalized metadata.
	Info netdataapi.HostInfo

	// Conflicting is metadata retained for another owner of the same GUID.
	Conflicting netdataapi.HostInfo

	// MetadataConflict is true when another owner reports different metadata.
	MetadataConflict bool

	// ConflictFirstSeen is true only for the first conflicting observation of a
	// distinct metadata state. Callers should use this to avoid log spam.
	ConflictFirstSeen bool
}

type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	owners         map[Owner]netdataapi.HostInfo
	reportedStates map[string]struct{}
	reportedOrder  []string
}

// New returns an empty concurrency-safe vnode registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Register records that owner successfully emitted metrics under info.GUID.
//
// Metadata is retained per owner. Callers should log a first-seen conflict as a
// warning because different definitions for one GUID cannot both be canonical.
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
			owners:         map[Owner]netdataapi.HostInfo{owner: cloneHostInfo(info)},
			reportedStates: make(map[string]struct{}),
		}
		r.entries[info.GUID] = ent
		return Registration{Info: cloneHostInfo(info)}, nil
	}

	ent.owners[owner] = cloneHostInfo(info)
	result := Registration{Info: cloneHostInfo(info)}
	for otherOwner, otherInfo := range ent.owners {
		if otherOwner == owner || hostInfoEqual(otherInfo, info) {
			continue
		}
		result.Conflicting = cloneHostInfo(otherInfo)
		result.MetadataConflict = true
		result.ConflictFirstSeen = markReportedState(ent, hostInfoFingerprint(info))
		break
	}
	return result, nil
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
