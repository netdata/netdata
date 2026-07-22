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

var (
	ErrVNodePreparedConsumed = errors.New("vnode configuration: prepared update consumed")
	ErrVNodeRevision         = errors.New("vnode configuration: expected revision differs")
	ErrVNodeNoChange         = errors.New("vnode configuration: no change")
)

// VNodeConfiguration owns immutable configured-vnode values and their atomic
// revision changes.
type VNodeConfiguration struct {
	mu sync.Mutex // guards records

	records map[string]jobruntime.VnodeSnapshot // committed vnode snapshots by name
}

type PreparedVNode struct {
	state *preparedVNodeState
}

type ConfiguredVNode struct {
	ID       string                   // vnode name
	Snapshot jobruntime.VnodeSnapshot // the vnode snapshot (hostname, GUID, labels)
}

type preparedVNodeState struct {
	mu       sync.Mutex               // guards consumed
	consumed bool                     // the prepared edit has been committed or aborted
	owner    *VNodeConfiguration      // the vnode configuration this edit belongs to
	id       string                   // vnode name
	expected uint64                   // revision the edit was prepared against (optimistic check)
	next     jobruntime.VnodeSnapshot // the snapshot to commit
	remove   bool                     // the edit removes the vnode
}

func NewVNodeConfigurationWithInitial(initial map[string]*vnodes.VirtualNode) (*VNodeConfiguration, error) {
	configuration := &VNodeConfiguration{
		records: make(map[string]jobruntime.VnodeSnapshot),
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
	if vc == nil || id == "" || id != strings.TrimSpace(id) || expected == ^uint64(0) || vnode == nil {
		return PreparedVNode{}, errors.New("vnode configuration: invalid preparation")
	}
	nextVNode := vnode.Copy()
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current := vc.records[id]
	if current.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	if current.Vnode != nil && vnodeConfigurationEqual(current.Vnode, nextVNode) {
		return PreparedVNode{}, ErrVNodeNoChange
	}
	metadataRevision := uint64(1)
	if current.Vnode != nil {
		metadataRevision = current.MetadataRevision
		if !vnodeMetadataEqual(current.Vnode, nextVNode) {
			if metadataRevision == ^uint64(0) {
				return PreparedVNode{}, errors.New("vnode configuration: metadata revision wrapped")
			}
			metadataRevision++
		}
	}
	state := &preparedVNodeState{
		owner:    vc,
		id:       strings.Clone(id),
		expected: expected,
		next: jobruntime.VnodeSnapshot{
			Vnode:            nextVNode,
			Revision:         expected + 1,
			MetadataRevision: metadataRevision,
		},
	}
	return PreparedVNode{
		state: state,
	}, nil
}

func (vc *VNodeConfiguration) PrepareRemove(id string, expected uint64) (PreparedVNode, error) {
	if vc == nil || id == "" || id != strings.TrimSpace(id) || expected == 0 || expected == ^uint64(0) {
		return PreparedVNode{}, errors.New("vnode configuration: invalid removal")
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current, ok := vc.records[id]
	if !ok || current.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	state := &preparedVNodeState{
		owner:    vc,
		id:       strings.Clone(id),
		expected: expected,
		next: jobruntime.VnodeSnapshot{
			Revision:         expected + 1,
			MetadataRevision: current.MetadataRevision,
		},
		remove: true,
	}
	return PreparedVNode{
		state: state,
	}, nil
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
	current := configuration.records[state.id]
	if current.Revision != state.expected {
		return jobruntime.VnodeSnapshot{}, ErrVNodeRevision
	}
	state.consumed = true
	if state.remove {
		delete(configuration.records, state.id)
		return state.next.Copy(), nil
	}
	configuration.records[state.id] = state.next.Copy()
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
	state.consumed = true
	return nil
}

func (vc *VNodeConfiguration) Lookup(id string) (jobruntime.VnodeSnapshot, bool) {
	if vc == nil {
		return jobruntime.VnodeSnapshot{}, false
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	snapshot, ok := vc.records[id]
	return snapshot.Copy(), ok
}

func (vc *VNodeConfiguration) Entries() []ConfiguredVNode {
	if vc == nil {
		return nil
	}
	vc.mu.Lock()
	entries := make([]ConfiguredVNode, 0, len(vc.records))
	for id, snapshot := range vc.records {
		entries = append(entries, ConfiguredVNode{
			ID:       id,
			Snapshot: snapshot.Copy(),
		})
	}
	vc.mu.Unlock()
	slices.SortFunc(entries, func(a, b ConfiguredVNode) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return entries
}

func vnodeConfigurationEqual(left, right *vnodes.VirtualNode) bool {
	return left.Name == right.Name && left.Hostname == right.Hostname &&
		left.GUID == right.GUID && left.Source == right.Source &&
		left.SourceType == right.SourceType && maps.Equal(left.Labels, right.Labels)
}

func vnodeMetadataEqual(left, right *vnodes.VirtualNode) bool {
	return left.Hostname == right.Hostname && left.GUID == right.GUID && maps.Equal(left.Labels, right.Labels)
}
