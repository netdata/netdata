// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"errors"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ Writer = (*mockWriter)(nil)

type mockWriter struct {
	mu                  sync.Mutex
	name                string
	calls               *[]string
	entries             []*model.TrapEntry
	flushes             int
	closeAttempts       int
	closed              bool
	err                 error
	binaryEncodedFields uint64
}

func (m *mockWriter) Write(entry *model.TrapEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockWriter) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls != nil {
		*m.calls = append(*m.calls, m.name+".flush")
	}
	if m.err != nil {
		return m.err
	}
	m.flushes++
	return nil
}

func (m *mockWriter) BinaryEncodedFields() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.binaryEncodedFields
}

func (m *mockWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls != nil {
		*m.calls = append(*m.calls, m.name+".close")
	}
	m.closeAttempts++
	m.closed = true
	return m.err
}

func TestCoordinatorSecondaryFailureDoesNotFailPrimaryWrite(t *testing.T) {
	primary := &mockWriter{}
	secondary := &mockWriter{err: ErrQueueFull}
	var outcomes []Outcome
	writer := NewCoordinator(primary, secondary, BackendOTLP, func(outcome Outcome) {
		outcomes = append(outcomes, outcome)
	})

	entry := testCoordinatorEntry()
	require.NoError(t, writer.Write(entry))
	assert.Len(t, primary.entries, 1)
	assert.Empty(t, secondary.entries)
	require.Len(t, outcomes, 1)
	assert.Equal(t, Outcome{
		Backend:       BackendOTLP,
		Stage:         StageEnqueue,
		FailedEntries: 1,
		Err:           ErrQueueFull,
	}, outcomes[0])
}

func TestCoordinatorBothWriteFailuresReturnOnlyPrimaryError(t *testing.T) {
	primaryErr := errors.New("primary failed")
	primary := &mockWriter{err: primaryErr}
	secondary := &mockWriter{err: ErrQueueFull}
	var outcomes []Outcome
	writer := NewCoordinator(primary, secondary, BackendOTLP, func(outcome Outcome) {
		outcomes = append(outcomes, outcome)
	})

	err := writer.Write(testCoordinatorEntry())
	require.ErrorIs(t, err, primaryErr)
	assert.Empty(t, primary.entries)
	assert.Empty(t, secondary.entries)
	require.Len(t, outcomes, 1)
	assert.False(t, outcomes[0].Authoritative)
}

func TestCoordinatorPrimaryFailureStillAttemptsSecondaryWrite(t *testing.T) {
	primaryErr := errors.New("primary failed")
	primary := &mockWriter{err: primaryErr}
	secondary := &mockWriter{}
	writer := NewCoordinator(primary, secondary, BackendOTLP, nil)

	entry := testCoordinatorEntry()
	err := writer.Write(entry)
	require.ErrorIs(t, err, primaryErr)
	assert.Empty(t, primary.entries)
	assert.Equal(t, []*model.TrapEntry{entry}, secondary.entries)
}

func TestCoordinatorFlushAndClosePreserveOrderAndJoinErrors(t *testing.T) {
	primaryErr := errors.New("primary failed")
	secondaryErr := errors.New("secondary failed")
	var calls []string
	primary := &mockWriter{name: "primary", calls: &calls, err: primaryErr}
	secondary := &mockWriter{name: "secondary", calls: &calls, err: secondaryErr}
	writer := NewCoordinator(primary, secondary, BackendOTLP, nil)

	flushErr := writer.Flush()
	require.ErrorIs(t, flushErr, primaryErr)
	require.ErrorIs(t, flushErr, secondaryErr)

	closeErr := writer.Close()
	require.ErrorIs(t, closeErr, primaryErr)
	require.ErrorIs(t, closeErr, secondaryErr)
	assert.Equal(t, 1, primary.closeAttempts)
	assert.Equal(t, 1, secondary.closeAttempts)
	assert.Equal(t, []string{
		"primary.flush",
		"secondary.flush",
		"primary.close",
		"secondary.close",
	}, calls)
}

func TestCoordinatorForwardsBinaryEncodedFieldsFromPrimary(t *testing.T) {
	primary := &mockWriter{binaryEncodedFields: 7}
	secondary := &mockWriter{}
	writer := NewCoordinator(primary, secondary, BackendOTLP, nil)

	binaryEncoded, ok := writer.(BinaryFieldCounter)
	require.True(t, ok)
	assert.Equal(t, uint64(7), binaryEncoded.BinaryEncodedFields())
}

func TestCoordinatorReturnsSingleWriterDirectly(t *testing.T) {
	writer := &mockWriter{}
	assert.Same(t, writer, NewCoordinator(writer, nil, BackendOTLP, nil))
	assert.Same(t, writer, NewCoordinator(nil, writer, BackendOTLP, nil))
}

func testCoordinatorEntry() *model.TrapEntry {
	return &model.TrapEntry{
		JobName:       "local",
		Message:       "trap",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10:9162",
		Enrichment: &model.TrapEnrichmentAudit{
			Source: &model.TrapSourceAudit{
				Selected: "192.0.2.10",
				Method:   "udp_peer",
			},
		},
	}
}
