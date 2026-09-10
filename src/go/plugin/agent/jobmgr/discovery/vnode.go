// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"cmp"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

var (
	ErrVNodePreparedConsumed = errors.New("vnode configuration: prepared update consumed")
	ErrVNodeRevision         = errors.New("vnode configuration: expected revision differs")
	ErrVNodeNoChange         = errors.New("vnode configuration: no change")
)

// AcquisitionToken identifies one configuration incarnation and acquisition
// setting generation. Label-only edits preserve it; remove/re-add never does.
type AcquisitionToken struct{ name string }
type vnodeRecord struct {
	config   *vnodes.Config
	metadata *vnodes.Metadata
	token    *AcquisitionToken
	snapshot jobruntime.VnodeSnapshot
	failed   bool
	pending  bool
}

// VNodeConfiguration separates authored secrets, acquired metadata and public snapshots.
// Records are immutable after commit; pointer comparison also fences remove/re-add ABA.
type VNodeConfiguration struct {
	mu          sync.Mutex
	records     map[string]*vnodeRecord
	definitions map[string]*hostoutput.Definition
	guids       map[string]string
	hostnames   map[string]string
}
type PreparedVNode struct{ state *preparedVNodeState }
type ConfiguredVNode struct {
	ID       string
	Config   *vnodes.Config
	Snapshot jobruntime.VnodeSnapshot
	Failed   bool
	Pending  bool
}
type preparedVNodeState struct {
	mu          sync.Mutex
	consumed    bool
	owner       *VNodeConfiguration
	id          string
	expected    *vnodeRecord
	next        *vnodeRecord
	remove      bool
	definition  *hostoutput.Definition
	metadataErr error
}

func NewVNodeConfigurationWithInitial(initial map[string]*vnodes.Config) (*VNodeConfiguration, error) {
	vc := &VNodeConfiguration{records: make(map[string]*vnodeRecord), definitions: make(map[string]*hostoutput.Definition), guids: make(map[string]string), hostnames: make(map[string]string)}
	for _, id := range slices.Sorted(maps.Keys(initial)) {
		c := initial[id].Copy()
		if c == nil {
			continue
		}
		if c.Name == "" {
			c.Name = id
		}
		if c.Name != id {
			return nil, errors.New("vnode configuration: initial identity differs from map key")
		}
		prepared, err := vc.PrepareUpsert(id, 0, c)
		if err != nil {
			return nil, err
		}
		if _, err = prepared.Commit(); err != nil {
			return nil, err
		}
	}
	return vc, nil
}
func (vc *VNodeConfiguration) PrepareUpsert(id string, expected uint64, config *vnodes.Config) (PreparedVNode, error) {
	if vc == nil || id == "" || id != strings.TrimSpace(id) || expected == ^uint64(0) || config == nil || config.Name != id {
		return PreparedVNode{}, errors.New("vnode configuration: invalid preparation")
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current := vc.records[id]
	if recordRevision(current) != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	next := &vnodeRecord{config: config.Copy()}
	next.config.NormalizeCredentials()
	if current != nil {
		if reflect.DeepEqual(current.config, next.config) {
			return PreparedVNode{}, ErrVNodeNoChange
		}
		if err := current.config.ValidateUpdate(next.config); err != nil {
			return PreparedVNode{}, err
		}
		next.metadata = current.metadata
		next.token = current.token
		next.failed = current.failed
		next.pending = current.pending
	}
	if next.config.IsSNMP() && (current == nil || !current.config.SameAcquisition(next.config)) {
		next.token = &AcquisitionToken{name: id}
		next.failed = false
		next.pending = true
	}
	return vc.prepareLocked(id, current, next, false)
}

// PrepareMetadata applies the current authored overrides, even when they changed
// while this request was in flight. Errors retain the last usable acquisition.
func (vc *VNodeConfiguration) PrepareMetadata(token *AcquisitionToken, metadata *vnodes.Metadata, failed bool) (PreparedVNode, error) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	if token == nil {
		return PreparedVNode{}, ErrVNodeRevision
	}
	current := vc.records[token.name]
	if current == nil || current.token != token {
		return PreparedVNode{}, ErrVNodeRevision
	}
	next := *current
	var metadataErr error
	next.failed = failed
	next.pending = false
	if metadata != nil && (!failed || current.metadata == nil) {
		next.metadata = metadata.Copy()
	}
	if resolved := next.config.Resolve(next.metadata); resolved != nil {
		if err := vnodes.ValidateConfigured(resolved); err != nil {
			metadataErr = err
			next.metadata = current.metadata
			next.failed = true
		}
	}
	prepared, err := vc.prepareLocked(token.name, current, &next, false)
	if err != nil {
		// Conflicting acquired hostnames cannot replace valid last-known metadata.
		metadataErr = err
		next.metadata = current.metadata
		next.failed = true
		prepared, err = vc.prepareLocked(token.name, current, &next, false)
	}
	if err == nil {
		prepared.state.metadataErr = metadataErr
	}
	return prepared, err
}

// MetadataError explains a rejected acquisition whose last-good fallback was
// prepared successfully. Callers report it only after committing that fallback.
func (pv PreparedVNode) MetadataError() error {
	if pv.state == nil {
		return nil
	}
	return pv.state.metadataErr
}
func (vc *VNodeConfiguration) PrepareRemove(id string, expected uint64) (PreparedVNode, error) {
	if vc == nil || id == "" || id != strings.TrimSpace(id) || expected == 0 || expected == ^uint64(0) {
		return PreparedVNode{}, errors.New("vnode configuration: invalid removal")
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	current := vc.records[id]
	if current == nil || recordRevision(current) != expected {
		return PreparedVNode{}, ErrVNodeRevision
	}
	return vc.prepareLocked(id, current, &vnodeRecord{}, true)
}
func recordRevision(r *vnodeRecord) uint64 {
	if r == nil {
		return 0
	}
	return r.snapshot.Revision
}
func (vc *VNodeConfiguration) prepareLocked(id string, current, next *vnodeRecord, remove bool) (PreparedVNode, error) {
	revision := recordRevision(current)
	if revision == ^uint64(0) {
		return PreparedVNode{}, ErrVNodeRevision
	}
	next.snapshot = jobruntime.VnodeSnapshot{Revision: revision + 1}
	if current != nil {
		next.snapshot.MetadataRevision = current.snapshot.MetadataRevision
	}
	var definition *hostoutput.Definition
	if !remove {
		resolved := next.config.Resolve(next.metadata)
		if resolved != nil {
			var err error
			definition, err = hostoutput.NewDefinition(netdataapi.HostInfo{GUID: resolved.GUID, Hostname: resolved.Hostname, Labels: resolved.HostLabels()})
			if err != nil {
				return PreparedVNode{}, err
			}
			if previous := vc.definitions[hostoutput.GUIDKey(resolved.GUID)]; previous.Equal(definition) {
				definition = previous
			}
			if current == nil || current.snapshot.Vnode == nil || !vnodeMetadataEqual(current.snapshot.Vnode, resolved) {
				if next.snapshot.MetadataRevision == ^uint64(0) {
					return PreparedVNode{}, ErrVNodeRevision
				}
				next.snapshot.MetadataRevision++
			}
		}
		next.snapshot.Vnode = resolved
		if err := vc.validateUniqueLocked(id, next.config, resolved); err != nil {
			return PreparedVNode{}, err
		}
	}
	return PreparedVNode{state: &preparedVNodeState{owner: vc, id: id, expected: current, next: next, remove: remove, definition: definition}}, nil
}
func recordHostname(config *vnodes.Config, resolved *vnodes.VirtualNode) string {
	if resolved != nil {
		return resolved.Hostname
	}
	return config.Hostname
}
func (vc *VNodeConfiguration) validateUniqueLocked(id string, config *vnodes.Config, resolved *vnodes.VirtualNode) error {
	if other, ok := vc.guids[hostoutput.GUIDKey(config.IdentityGUID())]; ok && other != id {
		return errors.New("duplicate configured vnode GUID")
	}
	if hostname := recordHostname(config, resolved); hostname != "" {
		if other, ok := vc.hostnames[hostname]; ok && other != id {
			return errors.New("duplicate configured vnode hostname")
		}
	}
	return nil
}

func (pv PreparedVNode) Commit() (jobruntime.VnodeSnapshot, error) {
	if pv.state == nil {
		return jobruntime.VnodeSnapshot{}, ErrVNodePreparedConsumed
	}
	s := pv.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed {
		return jobruntime.VnodeSnapshot{}, ErrVNodePreparedConsumed
	}
	vc := s.owner
	vc.mu.Lock()
	defer vc.mu.Unlock()
	if vc.records[s.id] != s.expected {
		return jobruntime.VnodeSnapshot{}, ErrVNodeRevision
	}
	if !s.remove {
		if err := vc.validateUniqueLocked(s.id, s.next.config, s.next.snapshot.Vnode); err != nil {
			return jobruntime.VnodeSnapshot{}, err
		}
	}
	s.consumed = true
	if s.expected != nil {
		delete(vc.guids, hostoutput.GUIDKey(s.expected.config.IdentityGUID()))
		delete(vc.hostnames, recordHostname(s.expected.config, s.expected.snapshot.Vnode))
	}
	if s.expected != nil && s.expected.snapshot.Vnode != nil {
		delete(vc.definitions, hostoutput.GUIDKey(s.expected.snapshot.Vnode.GUID))
	}
	if s.remove {
		delete(vc.records, s.id)
	} else {
		vc.records[s.id] = s.next
		vc.guids[hostoutput.GUIDKey(s.next.config.IdentityGUID())] = s.id
		if hostname := recordHostname(s.next.config, s.next.snapshot.Vnode); hostname != "" {
			vc.hostnames[hostname] = s.id
		}
		if s.next.snapshot.Vnode != nil {
			vc.definitions[hostoutput.GUIDKey(s.next.snapshot.Vnode.GUID)] = s.definition
		}
	}
	return s.next.snapshot.Copy(), nil
}
func (pv PreparedVNode) Abort() error {
	if pv.state == nil {
		return ErrVNodePreparedConsumed
	}
	s := pv.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed {
		return ErrVNodePreparedConsumed
	}
	s.consumed = true
	return nil
}
func (vc *VNodeConfiguration) Lookup(id string) (jobruntime.VnodeSnapshot, bool) {
	if vc == nil {
		return jobruntime.VnodeSnapshot{}, false
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	r, ok := vc.records[id]
	if !ok {
		return jobruntime.VnodeSnapshot{}, false
	}
	return r.snapshot.Copy(), true
}
func (vc *VNodeConfiguration) Authored(id string) (ConfiguredVNode, bool) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	r, ok := vc.records[id]
	if !ok {
		return ConfiguredVNode{}, false
	}
	return configuredVNode(id, r), true
}
func configuredVNode(id string, r *vnodeRecord) ConfiguredVNode {
	return ConfiguredVNode{ID: id, Config: r.config.Copy(), Snapshot: r.snapshot.Copy(), Failed: r.failed, Pending: r.pending}
}
func (vc *VNodeConfiguration) Acquisition(id string) (*AcquisitionToken, vnodes.SNMPConfig) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	r := vc.records[id]
	if r == nil || r.token == nil {
		return nil, vnodes.SNMPConfig{}
	}
	return r.token, r.config.ModeSNMP.Copy()
}
func (vc *VNodeConfiguration) Definition(guid string) *hostoutput.Definition {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.definitions[hostoutput.GUIDKey(guid)]
}
func (vc *VNodeConfiguration) Entries() []ConfiguredVNode {
	if vc == nil {
		return nil
	}
	vc.mu.Lock()
	entries := make([]ConfiguredVNode, 0, len(vc.records))
	for id, r := range vc.records {
		entries = append(entries, configuredVNode(id, r))
	}
	vc.mu.Unlock()
	slices.SortFunc(entries, func(a, b ConfiguredVNode) int { return cmp.Compare(a.ID, b.ID) })
	return entries
}
func vnodeMetadataEqual(left, right *vnodes.VirtualNode) bool {
	return left.Hostname == right.Hostname && left.GUID == right.GUID && maps.Equal(left.HostLabels(), right.HostLabels())
}
