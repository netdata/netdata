// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// vnodeStore is read from collector command effects and running job goroutines
// while vnode mutations stay loop-serialized, so access is guarded by mu.
type vnodeStore struct {
	mu           sync.RWMutex
	items        map[string]*vnodeEntry
	nextRevision uint64
}

type vnodeEntry struct {
	cfg              *vnodes.VirtualNode
	revision         uint64
	metadataRevision uint64
}

type Snapshot struct {
	Vnode            *vnodes.VirtualNode
	Revision         uint64
	MetadataRevision uint64
}

func newVnodeStore(items map[string]*vnodes.VirtualNode) *vnodeStore {
	s := &vnodeStore{items: make(map[string]*vnodeEntry)}
	for name, cfg := range items {
		if cfg == nil {
			continue
		}
		cp := cfg.Copy()
		if cp.Name == "" {
			cp.Name = name
		}
		s.nextRevision++
		s.items[name] = &vnodeEntry{
			cfg:              cp,
			revision:         s.nextRevision,
			metadataRevision: s.nextRevision,
		}
	}
	return s
}

func (s *vnodeStore) Lookup(name string) (*vnodes.VirtualNode, bool) {
	snap, ok := s.LookupSnapshot(name)
	if !ok {
		return nil, false
	}
	return snap.Vnode, true
}

func (s *vnodeStore) LookupSnapshot(name string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ent, ok := s.items[name]
	if !ok {
		return Snapshot{}, false
	}
	return ent.snapshot(), true
}

func (s *vnodeStore) Upsert(cfg *vnodes.VirtualNode) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg == nil {
		return false, fmt.Errorf("nil vnode config")
	}
	if orig, ok := s.items[cfg.Name]; ok && sameStoredVnode(orig.cfg, cfg) {
		return false, nil
	}
	next := cfg.Copy()
	s.nextRevision++
	metadataRevision := s.nextRevision
	if orig, ok := s.items[cfg.Name]; ok && orig.cfg.Equal(next) {
		metadataRevision = orig.metadataRevision
	}
	s.items[cfg.Name] = &vnodeEntry{
		cfg:              next,
		revision:         s.nextRevision,
		metadataRevision: metadataRevision,
	}
	return true, nil
}

func (s *vnodeStore) Remove(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[name]; !ok {
		return false
	}
	delete(s.items, name)
	return true
}

func (s *vnodeStore) ForEach(fn func(cfg *vnodes.VirtualNode) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ent := range s.items {
		if !fn(ent.cfg.Copy()) {
			return
		}
	}
}

func (e *vnodeEntry) snapshot() Snapshot {
	return Snapshot{
		Vnode:            e.cfg.Copy(),
		Revision:         e.revision,
		MetadataRevision: e.metadataRevision,
	}
}

func sameStoredVnode(orig, next *vnodes.VirtualNode) bool {
	if orig == nil || next == nil {
		return orig == next
	}
	return orig.Equal(next) &&
		orig.SourceType == next.SourceType
}
