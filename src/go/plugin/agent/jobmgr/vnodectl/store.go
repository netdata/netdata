// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// vnodeStore is intentionally lock-free because mutations are serialized by jobmgr.
// Any caller outside that serialized path must add its own synchronization first.
type vnodeStore struct {
	items map[string]*vnodes.VirtualNode
}

func newVnodeStore(items map[string]*vnodes.VirtualNode) *vnodeStore {
	if items == nil {
		items = make(map[string]*vnodes.VirtualNode)
	}
	return &vnodeStore{items: items}
}

func (s *vnodeStore) Lookup(name string) (*vnodes.VirtualNode, bool) {
	cfg, ok := s.items[name]
	return cfg, ok
}

func (s *vnodeStore) Upsert(cfg *vnodes.VirtualNode) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("nil vnode config")
	}
	if orig, ok := s.items[cfg.Name]; ok && sameStoredVnode(orig, cfg) {
		return false, nil
	}
	s.items[cfg.Name] = cfg
	return true, nil
}

func (s *vnodeStore) Remove(name string) bool {
	if _, ok := s.items[name]; !ok {
		return false
	}
	delete(s.items, name)
	return true
}

func (s *vnodeStore) ForEach(fn func(cfg *vnodes.VirtualNode) bool) {
	for _, cfg := range s.items {
		if !fn(cfg) {
			return
		}
	}
}

func sameStoredVnode(orig, next *vnodes.VirtualNode) bool {
	if orig == nil || next == nil {
		return orig == next
	}
	return orig.Equal(next) &&
		orig.SourceType == next.SourceType
}
