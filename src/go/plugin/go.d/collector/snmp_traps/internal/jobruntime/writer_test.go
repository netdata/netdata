// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
)

var _ output.Writer = (*mockTrapWriter)(nil)

type mockTrapWriter struct {
	mu                  sync.Mutex
	entries             []*model.TrapEntry
	flushes             int
	closeAttempts       int
	closed              bool
	err                 error
	binaryEncodedFields uint64
}

func (m *mockTrapWriter) Write(entry *model.TrapEntry) error {
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

func (m *mockTrapWriter) BinaryEncodedFields() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.binaryEncodedFields
}

func (m *mockTrapWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeAttempts++
	m.closed = true
	return m.err
}
