// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"strings"
)

func (m *Manager) Register(name string, fn func(Function)) {
	if fn == nil {
		m.Warningf("not registering '%s': nil function", name)
		return
	}

	m.RegisterWithContext(name, func(_ context.Context, f Function) {
		fn(f)
	})
}

func (m *Manager) RegisterWithContext(name string, fn Handler) {
	if fn == nil {
		m.Warningf("not registering '%s': nil function", name)
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	fs, ok := m.functionRegistry[name]
	if !ok {
		m.Debugf("registering function '%s' (direct)", name)
		fs = &functionSet{prefixes: make(map[string]Handler)}
		m.functionRegistry[name] = fs
	} else {
		if fs.direct != nil {
			m.Warningf("re-registering direct function '%s'", name)
		} else {
			m.Debugf("registering function '%s' (direct)", name)
		}
	}

	fs.direct = fn
}

func (m *Manager) Unregister(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if _, ok := m.functionRegistry[name]; ok {
		delete(m.functionRegistry, name)
		m.Debugf("unregistering function '%s'", name)
	}
}

func (m *Manager) RegisterPrefix(name, prefix string, fn func(Function)) {
	if fn == nil {
		m.Warningf("not registering '%s' with prefix '%s': nil function", name, prefix)
		return
	}

	m.RegisterPrefixWithContext(name, prefix, func(_ context.Context, f Function) {
		fn(f)
	})
}

func (m *Manager) RegisterPrefixWithContext(name, prefix string, fn Handler) {
	if fn == nil {
		m.Warningf("not registering '%s' with prefix '%s': nil function", name, prefix)
		return
	}
	if prefix == "" {
		m.Warningf("not registering '%s': empty prefix", name)
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	fs := m.functionRegistry[name]
	if fs == nil {
		fs = &functionSet{prefixes: make(map[string]Handler)}
		m.functionRegistry[name] = fs
	}

	if _, exists := fs.prefixes[prefix]; exists {
		m.Warningf("re-registering function '%s' with prefix '%s'", name, prefix)
		fs.prefixes[prefix] = fn
		return
	}

	for existing := range fs.prefixes {
		if prefixesOverlap(existing, prefix) {
			m.Errorf(
				"not registering function '%s' with prefix '%s': overlaps with existing prefix '%s'",
				name,
				prefix,
				existing,
			)
			return
		}
	}

	m.Debugf("registering function '%s' with prefix '%s'", name, prefix)
	fs.prefixes[prefix] = fn
}

// RegisterPrefixLaneDeriver attaches a lane-key derivation callback to an
// existing prefix registration. Registering for an unknown name/prefix is a
// no-op with a warning; passing nil removes the deriver.
func (m *Manager) RegisterPrefixLaneDeriver(name, prefix string, derive LaneKeyDeriver) {
	m.mux.Lock()
	defer m.mux.Unlock()

	fs, ok := m.functionRegistry[name]
	if !ok || fs.prefixes == nil {
		m.Warningf("not attaching lane deriver to '%s' prefix '%s': not registered", name, prefix)
		return
	}
	if _, exists := fs.prefixes[prefix]; !exists {
		m.Warningf("not attaching lane deriver to '%s' prefix '%s': prefix not registered", name, prefix)
		return
	}
	if derive == nil {
		delete(fs.prefixDerivers, prefix)
		return
	}
	if fs.prefixDerivers == nil {
		fs.prefixDerivers = make(map[string]LaneKeyDeriver)
	}
	fs.prefixDerivers[prefix] = derive
}

func prefixesOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}

	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func (m *Manager) UnregisterPrefix(name, prefix string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	fs, ok := m.functionRegistry[name]
	if !ok || fs.prefixes == nil {
		return
	}

	if _, exists := fs.prefixes[prefix]; exists {
		m.Debugf("unregistering function '%s' with prefix '%s'", name, prefix)
		delete(fs.prefixes, prefix)
		// The deriver belongs to the registration: a later re-register of
		// the same prefix must not inherit a stale deriver.
		delete(fs.prefixDerivers, prefix)
	}

	if fs.direct == nil && len(fs.prefixes) == 0 {
		delete(m.functionRegistry, name)
	}
}
