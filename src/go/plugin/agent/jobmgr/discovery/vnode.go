// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

const (
	MaximumVNodeConfigurationRecords = 4_096
	MaximumVNodeConfigurationBytes   = 16 * 1024 * 1024
	vnodeRecordOverheadBytes         = 64
	vnodeLabelOverheadBytes          = 16
)

var (
	ErrVNodePreparedConsumed = errors.New("vnode configuration: prepared update consumed")
	ErrVNodeRevision         = errors.New("vnode configuration: expected revision differs")
	ErrVNodeNoChange         = errors.New("vnode configuration: no change")
	ErrVNodeCapacity         = errors.New("vnode configuration: capacity exhausted")
)

// VNodeConfiguration owns immutable configured-vnode values and their atomic
// revision changes.
type VNodeConfiguration struct {
	mu sync.Mutex // guards records + pending

	records      map[string]vnodeRecord         // committed vnode snapshots by name
	pending      map[string]*preparedVNodeState // in-flight prepared vnode edits by name
	bytes        int                            // committed vnode config bytes (accounting)
	pendingBytes int                            // in-flight vnode config bytes
	pendingNew   int                            // count of in-flight new-vnode edits
}

type vnodeRecord struct {
	snapshot jobruntime.VnodeSnapshot
	bytes    int
}

type PreparedVNode struct {
	state *preparedVNodeState
}

type ConfiguredVNode struct {
	ID       string                   // vnode name
	Snapshot jobruntime.VnodeSnapshot // the vnode snapshot (hostname, GUID, labels)
}

type preparedVNodeState struct {
	mu        sync.Mutex               // guards consumed
	consumed  bool                     // the prepared edit has been committed or aborted
	owner     *VNodeConfiguration      // the vnode configuration this edit belongs to
	id        string                   // vnode name
	expected  uint64                   // revision the edit was prepared against (optimistic check)
	next      jobruntime.VnodeSnapshot // the snapshot to commit
	bytes     int                      // config bytes of next (accounting)
	remove    bool                     // the edit removes the vnode
	newRecord bool                     // the edit creates a new vnode
}

func NewVNodeConfigurationWithInitial(initial map[string]*vnodes.VirtualNode) (*VNodeConfiguration, error) {
	configuration := &VNodeConfiguration{
		records: make(map[string]vnodeRecord),
		pending: make(map[string]*preparedVNodeState),
	}
	ids := slices.Sorted(maps.Keys(initial))
	for _, id := range ids {
		vnode := initial[id]
		if vnode == nil {
			continue
		}
		vnode = vnode.Copy()
		if vnode.Name == "" {
			vnode.Name = id
		}
		if vnode.Name != id {
			return nil, errors.New("vnode configuration: initial identity differs from map key")
		}
		prepared, err := configuration.PrepareUpsert(id, 0, vnode)
		if err != nil {
			return nil, err
		}
		if _, err := prepared.Commit(); err != nil {
			return nil, err
		}
	}
	return configuration, nil
}

func (vc *VNodeConfiguration) PrepareUpsert(
	id string,
	expected uint64,
	vnode *vnodes.VirtualNode,
) (PreparedVNode, error) {
	if vc == nil || id == "" || id != strings.TrimSpace(id) ||
		expected == ^uint64(0) || vnode == nil {
		return PreparedVNode{}, errors.New("vnode configuration: invalid preparation")
	}
	nextVNode := vnode.Copy()
	nextBytes, err := vnodeConfigurationBytes(id, nextVNode)
	if err != nil {
		return PreparedVNode{}, err
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current := vc.records[id]
	if current.snapshot.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	if vc.pending[id] != nil {
		return PreparedVNode{}, errors.New("vnode configuration: update already pending")
	}
	if current.snapshot.Vnode != nil && vnodeConfigurationEqual(current.snapshot.Vnode, nextVNode) {
		return PreparedVNode{}, ErrVNodeNoChange
	}
	newRecord := current.snapshot.Vnode == nil
	if newRecord &&
		len(vc.records)+vc.pendingNew == MaximumVNodeConfigurationRecords {
		return PreparedVNode{}, ErrVNodeCapacity
	}
	if nextBytes > MaximumVNodeConfigurationBytes-vc.bytes-vc.pendingBytes {
		return PreparedVNode{}, ErrVNodeCapacity
	}
	metadataRevision := uint64(1)
	if current.snapshot.Vnode != nil {
		metadataRevision = current.snapshot.MetadataRevision
		if !vnodeMetadataEqual(current.snapshot.Vnode, nextVNode) {
			if metadataRevision == ^uint64(0) {
				return PreparedVNode{}, errors.New("vnode configuration: metadata revision wrapped")
			}
			metadataRevision++
		}
	}
	state := &preparedVNodeState{
		owner: vc, id: strings.Clone(id), expected: expected,
		next: jobruntime.VnodeSnapshot{
			Vnode: nextVNode, Revision: expected + 1,
			MetadataRevision: metadataRevision,
		},
		bytes: nextBytes, newRecord: newRecord,
	}
	vc.pending[state.id] = state
	vc.pendingBytes += nextBytes
	if newRecord {
		vc.pendingNew++
	}
	return PreparedVNode{state: state}, nil
}

func (vc *VNodeConfiguration) PrepareRemove(
	id string,
	expected uint64,
) (PreparedVNode, error) {
	if vc == nil || id == "" || id != strings.TrimSpace(id) ||
		expected == 0 || expected == ^uint64(0) {
		return PreparedVNode{}, errors.New("vnode configuration: invalid removal")
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current, ok := vc.records[id]
	if !ok || current.snapshot.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	if vc.pending[id] != nil {
		return PreparedVNode{}, errors.New("vnode configuration: update already pending")
	}
	state := &preparedVNodeState{
		owner: vc, id: strings.Clone(id), expected: expected,
		next: jobruntime.VnodeSnapshot{
			Revision: expected + 1, MetadataRevision: current.snapshot.MetadataRevision,
		},
		remove: true,
	}
	vc.pending[state.id] = state
	return PreparedVNode{state: state}, nil
}

func (pv PreparedVNode) Commit() (jobruntime.VnodeSnapshot, error) {
	if pv.state == nil {
		return jobruntime.VnodeSnapshot{}, ErrVNodePreparedConsumed
	}
	state := pv.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.consumed {
		return jobruntime.VnodeSnapshot{}, ErrVNodePreparedConsumed
	}
	configuration := state.owner
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	if configuration.pending[state.id] != state {
		return jobruntime.VnodeSnapshot{}, errors.New("vnode configuration: stale prepared update")
	}
	current := configuration.records[state.id]
	if current.snapshot.Revision != state.expected {
		return jobruntime.VnodeSnapshot{}, ErrVNodeRevision
	}
	state.consumed = true
	delete(configuration.pending, state.id)
	configuration.pendingBytes -= state.bytes
	if state.newRecord {
		configuration.pendingNew--
	}
	if state.remove {
		delete(configuration.records, state.id)
		configuration.bytes -= current.bytes
		return state.next.Copy(), nil
	}
	configuration.bytes += state.bytes - current.bytes
	configuration.records[state.id] = vnodeRecord{
		snapshot: state.next.Copy(),
		bytes:    state.bytes,
	}
	return state.next.Copy(), nil
}

func (pv PreparedVNode) Abort() error {
	if pv.state == nil {
		return ErrVNodePreparedConsumed
	}
	state := pv.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.consumed {
		return ErrVNodePreparedConsumed
	}
	configuration := state.owner
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	if configuration.pending[state.id] != state {
		return errors.New("vnode configuration: stale prepared update")
	}
	state.consumed = true
	delete(configuration.pending, state.id)
	configuration.pendingBytes -= state.bytes
	if state.newRecord {
		configuration.pendingNew--
	}
	return nil
}

func (vc *VNodeConfiguration) Lookup(
	id string,
) (jobruntime.VnodeSnapshot, bool) {
	if vc == nil {
		return jobruntime.VnodeSnapshot{}, false
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	record, ok := vc.records[id]
	return record.snapshot.Copy(), ok
}

func (vc *VNodeConfiguration) Entries() []ConfiguredVNode {
	if vc == nil {
		return nil
	}
	vc.mu.Lock()
	entries := make([]ConfiguredVNode, 0, len(vc.records))
	for id, record := range vc.records {
		entries = append(entries, ConfiguredVNode{
			ID:       id,
			Snapshot: record.snapshot.Copy(),
		})
	}
	vc.mu.Unlock()
	slices.SortFunc(entries, func(a, b ConfiguredVNode) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return entries
}

func vnodeConfigurationBytes(id string, vnode *vnodes.VirtualNode) (int, error) {
	size := vnodeRecordOverheadBytes + len(id) + len(vnode.Name) +
		len(vnode.Hostname) + len(vnode.GUID) + len(vnode.Source) +
		len(vnode.SourceType)
	for key, value := range vnode.Labels {
		step := vnodeLabelOverheadBytes + len(key) + len(value)
		if step > MaximumVNodeConfigurationBytes-size {
			return 0, ErrVNodeCapacity
		}
		size += step
	}
	if size > MaximumVNodeConfigurationBytes {
		return 0, ErrVNodeCapacity
	}
	return size, nil
}

func vnodeConfigurationEqual(left, right *vnodes.VirtualNode) bool {
	return left.Name == right.Name && left.Hostname == right.Hostname &&
		left.GUID == right.GUID && left.Source == right.Source &&
		left.SourceType == right.SourceType && maps.Equal(left.Labels, right.Labels)
}

func vnodeMetadataEqual(left, right *vnodes.VirtualNode) bool {
	return left.Hostname == right.Hostname && left.GUID == right.GUID &&
		maps.Equal(left.Labels, right.Labels)
}
