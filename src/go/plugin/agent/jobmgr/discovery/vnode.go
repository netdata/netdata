// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"
	"maps"
	"sort"
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
	mu sync.Mutex

	records      map[string]vnodeRecord
	pending      map[string]*preparedVNodeState
	bytes        int
	pendingBytes int
	pendingNew   int
}

type vnodeRecord struct {
	snapshot jobruntime.VnodeSnapshot
	bytes    int
}

type PreparedVNode struct {
	state *preparedVNodeState
}

type ConfiguredVNode struct {
	ID       string
	Snapshot jobruntime.VnodeSnapshot
}

type preparedVNodeState struct {
	mu        sync.Mutex
	consumed  bool
	owner     *VNodeConfiguration
	id        string
	expected  uint64
	next      jobruntime.VnodeSnapshot
	bytes     int
	remove    bool
	newRecord bool
}

func NewVNodeConfiguration() *VNodeConfiguration {
	return &VNodeConfiguration{
		records: make(map[string]vnodeRecord),
		pending: make(map[string]*preparedVNodeState),
	}
}

func NewVNodeConfigurationWithInitial(
	initial map[string]*vnodes.VirtualNode,
) (*VNodeConfiguration, error) {
	configuration := NewVNodeConfiguration()
	ids := make([]string, 0, len(initial))
	for id := range initial {
		ids = append(ids, id)
	}
	sort.Strings(ids)
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
			return nil, errors.New(
				"vnode configuration: initial identity differs from map key",
			)
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

func (configuration *VNodeConfiguration) PrepareUpsert(
	id string,
	expected uint64,
	vnode *vnodes.VirtualNode,
) (PreparedVNode, error) {
	if configuration == nil || id == "" || id != strings.TrimSpace(id) ||
		expected == ^uint64(0) || vnode == nil {
		return PreparedVNode{}, errors.New("vnode configuration: invalid preparation")
	}
	nextVNode := vnode.Copy()
	nextBytes, err := vnodeConfigurationBytes(id, nextVNode)
	if err != nil {
		return PreparedVNode{}, err
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	current := configuration.records[id]
	if current.snapshot.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	if configuration.pending[id] != nil {
		return PreparedVNode{}, errors.New("vnode configuration: update already pending")
	}
	if current.snapshot.Vnode != nil && vnodeConfigurationEqual(current.snapshot.Vnode, nextVNode) {
		return PreparedVNode{}, ErrVNodeNoChange
	}
	newRecord := current.snapshot.Vnode == nil
	if newRecord &&
		len(configuration.records)+configuration.pendingNew == MaximumVNodeConfigurationRecords {
		return PreparedVNode{}, ErrVNodeCapacity
	}
	if nextBytes > MaximumVNodeConfigurationBytes-configuration.bytes-configuration.pendingBytes {
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
		owner: configuration, id: strings.Clone(id), expected: expected,
		next: jobruntime.VnodeSnapshot{
			Vnode: nextVNode, Revision: expected + 1,
			MetadataRevision: metadataRevision,
		},
		bytes: nextBytes, newRecord: newRecord,
	}
	configuration.pending[state.id] = state
	configuration.pendingBytes += nextBytes
	if newRecord {
		configuration.pendingNew++
	}
	return PreparedVNode{state: state}, nil
}

func (configuration *VNodeConfiguration) PrepareRemove(
	id string,
	expected uint64,
) (PreparedVNode, error) {
	if configuration == nil || id == "" || id != strings.TrimSpace(id) ||
		expected == 0 || expected == ^uint64(0) {
		return PreparedVNode{}, errors.New("vnode configuration: invalid removal")
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	current, ok := configuration.records[id]
	if !ok || current.snapshot.Revision != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	if configuration.pending[id] != nil {
		return PreparedVNode{}, errors.New("vnode configuration: update already pending")
	}
	state := &preparedVNodeState{
		owner: configuration, id: strings.Clone(id), expected: expected,
		next: jobruntime.VnodeSnapshot{
			Revision: expected + 1, MetadataRevision: current.snapshot.MetadataRevision,
		},
		remove: true,
	}
	configuration.pending[state.id] = state
	return PreparedVNode{state: state}, nil
}

func (prepared PreparedVNode) Commit() (jobruntime.VnodeSnapshot, error) {
	if prepared.state == nil {
		return jobruntime.VnodeSnapshot{}, ErrVNodePreparedConsumed
	}
	state := prepared.state
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

func (prepared PreparedVNode) Abort() error {
	if prepared.state == nil {
		return ErrVNodePreparedConsumed
	}
	state := prepared.state
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

func (configuration *VNodeConfiguration) Lookup(
	id string,
) (jobruntime.VnodeSnapshot, bool) {
	if configuration == nil {
		return jobruntime.VnodeSnapshot{}, false
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	record, ok := configuration.records[id]
	return record.snapshot.Copy(), ok
}

func (configuration *VNodeConfiguration) Entries() []ConfiguredVNode {
	if configuration == nil {
		return nil
	}
	configuration.mu.Lock()
	entries := make([]ConfiguredVNode, 0, len(configuration.records))
	for id, record := range configuration.records {
		entries = append(entries, ConfiguredVNode{
			ID:       id,
			Snapshot: record.snapshot.Copy(),
		})
	}
	configuration.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

type VNodeConfigurationCensus struct {
	Records      int
	Pending      int
	Bytes        int
	PendingBytes int
}

func (configuration *VNodeConfiguration) Census() VNodeConfigurationCensus {
	if configuration == nil {
		return VNodeConfigurationCensus{}
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	return VNodeConfigurationCensus{
		Records: len(configuration.records), Pending: len(configuration.pending),
		Bytes: configuration.bytes, PendingBytes: configuration.pendingBytes,
	}
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
