// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// vnodeStore owns manager vnode state. It is intentionally lock-free because
// mutations are serialized by Manager.run and startup initialization.
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

func (s *vnodeStore) Upsert(cfg *vnodes.VirtualNode) (changed bool, affectedJobNames []string, err error) {
	if cfg == nil {
		return false, nil, fmt.Errorf("nil vnode config")
	}
	if orig, ok := s.items[cfg.Name]; ok && orig.Equal(cfg) {
		return false, nil, nil
	}
	s.items[cfg.Name] = cfg
	return true, nil, nil
}

func (s *vnodeStore) Remove(name string) (removed bool, err error) {
	if _, ok := s.items[name]; !ok {
		return false, nil
	}
	delete(s.items, name)
	return true, nil
}

func (s *vnodeStore) ForEach(fn func(cfg *vnodes.VirtualNode) bool) {
	for _, cfg := range s.items {
		if !fn(cfg) {
			return
		}
	}
}
