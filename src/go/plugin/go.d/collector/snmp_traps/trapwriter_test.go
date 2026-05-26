// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "sync"

var _ TrapWriter = (*mockTrapWriter)(nil)

type mockTrapWriter struct {
	mu              sync.Mutex
	entries         []*TrapEntry
	flushes         int
	closed          bool
	err             error
	sanitizedFields uint64
}

func (m *mockTrapWriter) Write(entry *TrapEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockTrapWriter) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.flushes++
	return nil
}

func (m *mockTrapWriter) SanitizedFields() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sanitizedFields
}

func (m *mockTrapWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.closed = true
	return nil
}
