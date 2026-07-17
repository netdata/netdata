// SPDX-License-Identifier: GPL-3.0-or-later

package vnoderegistry

import (
	"errors"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

var (
	ErrMetadataReservationBusy     = errors.New("vnoderegistry: metadata reservation busy")
	ErrMetadataReservationConsumed = errors.New("vnoderegistry: metadata reservation consumed")
	ErrMetadataStaleGeneration     = errors.New("vnoderegistry: stale metadata generation")
	ErrMetadataConflictingOwner    = errors.New("vnoderegistry: conflicting metadata owner")
	ErrMetadataStaleLease          = errors.New("vnoderegistry: stale metadata lease")
	ErrMetadataHistoryCapacity     = errors.New("vnoderegistry: metadata history capacity exhausted")
)

const MaximumMetadataHistoryRecords = 65_536

type MetadataAuthority struct {
	ID         string
	Generation uint64
}

type PreparedMetadata struct {
	state *preparedMetadataState
}

type preparedMetadataState struct {
	mu          sync.Mutex
	transferred bool
	data        metadataReservationData
}

type MetadataReservation struct {
	state *metadataReservationState
}

type metadataReservationState struct {
	mu       sync.Mutex
	consumed bool
	data     metadataReservationData
}

type metadataReservationData struct {
	registry   *Registry
	token      uint64
	info       netdataapi.HostInfo
	authority  MetadataAuthority
	revision   uint64
	needDefine bool
	newHistory bool
}

type MetadataLease struct {
	registry  *Registry
	guid      string
	authority MetadataAuthority
}

func (prepared PreparedMetadata) Valid() bool {
	if prepared.state == nil {
		return false
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	return !prepared.state.transferred &&
		prepared.state.data.registry != nil &&
		prepared.state.data.token != 0
}

func (prepared PreparedMetadata) Info() netdataapi.HostInfo {
	if prepared.state == nil {
		return netdataapi.HostInfo{}
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.transferred {
		return netdataapi.HostInfo{}
	}
	return cloneHostInfo(prepared.state.data.info)
}

func (prepared PreparedMetadata) Revision() uint64 {
	if prepared.state == nil {
		return 0
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.transferred {
		return 0
	}
	return prepared.state.data.revision
}

func (prepared PreparedMetadata) NeedDefine() bool {
	if prepared.state == nil {
		return false
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	return !prepared.state.transferred && prepared.state.data.needDefine
}

func (prepared PreparedMetadata) Transfer() (MetadataReservation, error) {
	if prepared.state == nil {
		return MetadataReservation{}, ErrMetadataReservationConsumed
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.transferred ||
		prepared.state.data.registry == nil ||
		prepared.state.data.token == 0 {
		return MetadataReservation{}, ErrMetadataReservationConsumed
	}
	prepared.state.transferred = true
	return MetadataReservation{
		state: &metadataReservationState{data: prepared.state.data},
	}, nil
}

func (prepared PreparedMetadata) Abort() error {
	if prepared.state == nil {
		return ErrMetadataReservationConsumed
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.transferred ||
		prepared.state.data.registry == nil ||
		prepared.state.data.token == 0 {
		return ErrMetadataReservationConsumed
	}
	prepared.state.transferred = true
	return prepared.state.data.registry.abortMetadata(prepared.state.data)
}

func (reservation MetadataReservation) Commit() (MetadataLease, error) {
	data, err := reservation.take()
	if err != nil {
		return MetadataLease{}, err
	}
	return data.registry.commitMetadata(data)
}

func (reservation MetadataReservation) Abort() error {
	data, err := reservation.take()
	if err != nil {
		return err
	}
	return data.registry.abortMetadata(data)
}

func (reservation MetadataReservation) take() (metadataReservationData, error) {
	if reservation.state == nil {
		return metadataReservationData{}, ErrMetadataReservationConsumed
	}
	reservation.state.mu.Lock()
	defer reservation.state.mu.Unlock()
	if reservation.state.consumed ||
		reservation.state.data.registry == nil ||
		reservation.state.data.token == 0 {
		return metadataReservationData{}, ErrMetadataReservationConsumed
	}
	reservation.state.consumed = true
	return reservation.state.data, nil
}

// PrepareMetadata builds one private metadata postimage. The registry changes
// only when the transferred reservation commits after output succeeds.
func (r *Registry) PrepareMetadata(
	authority MetadataAuthority,
	info netdataapi.HostInfo,
) (PreparedMetadata, error) {
	if r == nil {
		return PreparedMetadata{}, errors.New("vnoderegistry: nil registry")
	}
	authority.ID = strings.TrimSpace(authority.ID)
	if authority.ID == "" || authority.Generation == 0 {
		return PreparedMetadata{}, errors.New("vnoderegistry: invalid metadata authority")
	}
	var err error
	info, err = chartemit.PrepareHostInfo(info)
	if err != nil {
		return PreparedMetadata{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializeMetadataTransactionsLocked()
	if r.reservations[info.GUID] != 0 {
		return PreparedMetadata{}, ErrMetadataReservationBusy
	}
	owner := Owner(authority.ID)
	current := r.entries[info.GUID]
	historyKey := metadataHistoryKey{guid: info.GUID, owner: owner}
	if generation := r.metadataHistory[historyKey]; generation >= authority.Generation {
		return PreparedMetadata{}, ErrMetadataStaleGeneration
	}
	changed := current == nil || !hostInfoEqual(current.info, info)
	if changed && current != nil {
		for other := range current.owners {
			if other != owner {
				return PreparedMetadata{}, ErrMetadataConflictingOwner
			}
		}
	}
	revision := uint64(1)
	if current != nil {
		revision = current.revision
		if changed {
			revision++
		}
	}
	r.nextToken++
	if r.nextToken == 0 {
		return PreparedMetadata{}, errors.New("vnoderegistry: metadata token wrapped")
	}
	_, historyExists := r.metadataHistory[historyKey]
	if !historyExists {
		if len(r.metadataHistory) == MaximumMetadataHistoryRecords {
			return PreparedMetadata{}, ErrMetadataHistoryCapacity
		}
		r.metadataHistory[historyKey] = 0
	}
	r.reservations[info.GUID] = r.nextToken
	return PreparedMetadata{
		state: &preparedMetadataState{data: metadataReservationData{
			registry: r, token: r.nextToken, info: cloneHostInfo(info),
			authority: authority, revision: revision, needDefine: changed,
			newHistory: !historyExists,
		}},
	}, nil
}

func (r *Registry) commitMetadata(data metadataReservationData) (MetadataLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesMetadataReservationLocked(data) {
		return MetadataLease{}, ErrMetadataReservationConsumed
	}
	delete(r.reservations, data.info.GUID)
	current := r.entries[data.info.GUID]
	if current == nil {
		current = &entry{
			owners:         make(map[Owner]ownerFacet),
			reportedStates: make(map[string]struct{}),
		}
		r.entries[data.info.GUID] = current
	}
	current.info = cloneHostInfo(data.info)
	current.revision = data.revision
	owner := Owner(data.authority.ID)
	current.owners[owner] |= ownerFacetMetadata
	owners := r.leaseOwners[data.info.GUID]
	if owners == nil {
		owners = make(map[Owner]uint64)
		r.leaseOwners[data.info.GUID] = owners
	}
	owners[owner] = data.authority.Generation
	r.metadataHistory[metadataHistoryKey{guid: data.info.GUID, owner: owner}] =
		data.authority.Generation
	return MetadataLease{
		registry: r, guid: data.info.GUID, authority: data.authority,
	}, nil
}

func (r *Registry) abortMetadata(data metadataReservationData) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesMetadataReservationLocked(data) {
		return ErrMetadataReservationConsumed
	}
	delete(r.reservations, data.info.GUID)
	if data.newHistory {
		key := metadataHistoryKey{guid: data.info.GUID, owner: Owner(data.authority.ID)}
		if r.metadataHistory[key] == 0 {
			delete(r.metadataHistory, key)
		}
	}
	return nil
}

func (r *Registry) ReleaseMetadata(lease MetadataLease) (bool, error) {
	if r == nil ||
		lease.registry != r ||
		lease.guid == "" ||
		lease.authority.ID == "" ||
		lease.authority.Generation == 0 {
		return false, ErrMetadataStaleLease
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reservations[lease.guid] != 0 {
		return false, ErrMetadataReservationBusy
	}
	owner := Owner(lease.authority.ID)
	current := r.entries[lease.guid]
	owners := r.leaseOwners[lease.guid]
	if current == nil || owners == nil || owners[owner] != lease.authority.Generation {
		return false, ErrMetadataStaleLease
	}
	delete(owners, owner)
	current.removeOwnerFacet(owner, ownerFacetMetadata)
	if len(owners) == 0 {
		delete(r.leaseOwners, lease.guid)
	}
	if len(current.owners) != 0 {
		return false, nil
	}
	delete(r.entries, lease.guid)
	return true, nil
}

func (r *Registry) initializeMetadataTransactionsLocked() {
	if r.entries == nil {
		r.entries = make(map[string]*entry)
	}
	if r.reservations == nil {
		r.reservations = make(map[string]uint64)
	}
	if r.leaseOwners == nil {
		r.leaseOwners = make(map[string]map[Owner]uint64)
	}
	if r.metadataHistory == nil {
		r.metadataHistory = make(map[metadataHistoryKey]uint64)
	}
}

func (r *Registry) matchesMetadataReservationLocked(data metadataReservationData) bool {
	return data.registry == r &&
		data.token != 0 &&
		r.reservations[data.info.GUID] == data.token
}
