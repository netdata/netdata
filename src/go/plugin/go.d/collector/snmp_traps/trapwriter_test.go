// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ TrapWriter = (*mockTrapWriter)(nil)

type mockTrapWriter struct {
	mu                  sync.Mutex
	entries             []*TrapEntry
	flushes             int
	closed              bool
	err                 error
	binaryEncodedFields uint64
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

func (m *mockTrapWriter) BinaryEncodedFields() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.binaryEncodedFields
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

func TestFanoutTrapWriterSecondaryFailureDoesNotFailPrimaryWrite(t *testing.T) {
	primary := &mockTrapWriter{}
	secondary := &mockTrapWriter{err: errQueueFull}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Write(&TrapEntry{JobName: "local", Message: "trap"})
	require.NoError(t, err)
	assert.Len(t, primary.entries, 1)
	assert.Len(t, secondary.entries, 0)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed)
}

func TestFanoutTrapWriterPrimaryFailureStopsSecondaryWrite(t *testing.T) {
	primaryErr := errors.New("primary failed")
	primary := &mockTrapWriter{err: primaryErr}
	secondary := &mockTrapWriter{}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Write(&TrapEntry{JobName: "local", Message: "trap"})
	require.ErrorIs(t, err, primaryErr)
	assert.Len(t, primary.entries, 0)
	assert.Len(t, secondary.entries, 0)
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed)
}

func TestFanoutTrapWriterSecondaryFlushFailureReturnsErrorAfterPrimaryFlush(t *testing.T) {
	secondaryErr := errors.New("secondary failed")
	primary := &mockTrapWriter{}
	secondary := &mockTrapWriter{err: secondaryErr}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Flush()
	require.ErrorIs(t, err, secondaryErr)
	assert.Equal(t, 1, primary.flushes)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed)
}

func TestFanoutTrapWriterSecondaryCloseFailureReturnsErrorAfterPrimaryClose(t *testing.T) {
	secondaryErr := errors.New("secondary failed")
	primary := &mockTrapWriter{}
	secondary := &mockTrapWriter{err: secondaryErr}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Close()
	require.ErrorIs(t, err, secondaryErr)
	assert.True(t, primary.closed)
	assert.False(t, secondary.closed)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed)
}

func TestFanoutTrapWriterCloseReturnsPrimaryAndSecondaryErrors(t *testing.T) {
	primaryErr := errors.New("primary failed")
	secondaryErr := errors.New("secondary failed")
	primary := &mockTrapWriter{err: primaryErr}
	secondary := &mockTrapWriter{err: secondaryErr}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Close()
	require.ErrorIs(t, err, primaryErr)
	require.ErrorIs(t, err, secondaryErr)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed)
}

func TestFanoutTrapWriterForwardsBinaryEncodedFieldsFromPrimary(t *testing.T) {
	primary := &mockTrapWriter{binaryEncodedFields: 7}
	secondary := &mockTrapWriter{}
	writer := newFanoutTrapWriter(primary, secondary, nil)

	binaryEncoded, ok := writer.(interface{ BinaryEncodedFields() uint64 })
	require.True(t, ok)
	assert.Equal(t, uint64(7), binaryEncoded.BinaryEncodedFields())
}
