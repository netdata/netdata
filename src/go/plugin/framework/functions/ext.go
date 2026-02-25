// SPDX-License-Identifier: GPL-3.0-or-later

package functions

func (m *Manager) Register(name string, fn func(Function)) {
	if fn == nil {
		m.Warningf("not registering '%s': nil function", name)
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	fs, ok := m.FunctionRegistry[name]
	if !ok {
		m.Debugf("registering function '%s' (direct)", name)
		fs = &functionSet{prefixes: make(map[string]func(Function))}
		m.FunctionRegistry[name] = fs
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

	if _, ok := m.FunctionRegistry[name]; ok {
		delete(m.FunctionRegistry, name)
		m.Debugf("unregistering function '%s'", name)
	}
}

func (m *Manager) RegisterPrefix(name, prefix string, fn func(Function)) {
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

	fs := m.FunctionRegistry[name]
	if fs == nil {
		fs = &functionSet{prefixes: make(map[string]func(Function))}
		m.FunctionRegistry[name] = fs
	}

	if _, exists := fs.prefixes[prefix]; exists {
		m.Warningf("re-registering function '%s' with prefix '%s'", name, prefix)
	} else {
		m.Debugf("registering function '%s' with prefix '%s'", name, prefix)
	}

	fs.prefixes[prefix] = fn
}

func (m *Manager) UnregisterPrefix(name, prefix string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	fs, ok := m.FunctionRegistry[name]
	if !ok || fs.prefixes == nil {
		return
	}

	if _, exists := fs.prefixes[prefix]; exists {
		m.Debugf("unregistering function '%s' with prefix '%s'", name, prefix)
		delete(fs.prefixes, prefix)
	}

	if fs.direct == nil && len(fs.prefixes) == 0 {
		delete(m.FunctionRegistry, name)
	}
}
