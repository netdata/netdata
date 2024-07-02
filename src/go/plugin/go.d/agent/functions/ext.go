// SPDX-License-Identifier: GPL-3.0-or-later

package functions

func (m *Manager) Register(name string, fn func(Function)) {
	if fn == nil {
		m.Warningf("not registering '%s': nil function", name)
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	if _, ok := m.FunctionRegistry[name]; !ok {
		m.Debugf("registering function '%s'", name)
	} else {
		m.Warningf("re-registering function '%s'", name)
	}
	m.FunctionRegistry[name] = fn
}

func (m *Manager) Unregister(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if _, ok := m.FunctionRegistry[name]; !ok {
		delete(m.FunctionRegistry, name)
		m.Debugf("unregistering function '%s'", name)
	}
}
