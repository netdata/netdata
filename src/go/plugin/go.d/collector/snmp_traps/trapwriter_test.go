// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ TrapWriter = (*mockTrapWriter)(nil)

type mockTrapWriter struct {
	mu                  sync.Mutex
	entries             []*TrapEntry
	flushes             int
	closeAttempts       int
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
	m.closeAttempts++
	m.closed = true
	if m.err != nil {
		return m.err
	}
	return nil
}

func TestFanoutTrapWriterSecondaryFailureDoesNotFailPrimaryWrite(t *testing.T) {
	primary := &mockTrapWriter{}
	secondary := &mockTrapWriter{err: errQueueFull}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	entry := testFanoutTrapEntry()
	err := writer.Write(entry)
	require.NoError(t, err)
	assert.Len(t, primary.entries, 1)
	assert.Len(t, secondary.entries, 0)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(0), metrics.pipeline.writeFailed.Load())

	sourceID, sourceKind := metrics.fallbackSourceIdentityForTest(entry)
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	collectSourceMetrics(store, "local", metrics)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
	labels := metrix.Labels{"job_name": "local", "source_id": sourceID, "source_kind": sourceKind}
	if v, ok := store.Read().Value("snmp_trap_source_pipeline_write_failed", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_source_pipeline_write_failed = %v/%v, want 0/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_source_errors_otlp_export_failed", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_source_errors_otlp_export_failed = %v/%v, want 1/true", v, ok)
	}
}

func TestFanoutTrapWriterBothWriteFailuresRecordOneTerminalPipelineFailure(t *testing.T) {
	primaryErr := errors.New("primary failed")
	primary := &mockTrapWriter{err: primaryErr}
	secondary := &mockTrapWriter{err: errQueueFull}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	entry := testFanoutTrapEntry()
	err := writer.Write(entry)
	require.ErrorIs(t, err, primaryErr)
	assert.Len(t, primary.entries, 0)
	assert.Len(t, secondary.entries, 0)
	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(0), metrics.pipeline.writeFailed.Load())

	metrics.recordWriteFailure(entry, trapWriteFailureJournal)
	assert.Equal(t, uint64(1), metrics.pipeline.writeFailed.Load())

	sourceID, sourceKind := metrics.fallbackSourceIdentityForTest(entry)
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	collectPipeline(store, "local", metrics)
	collectSourceMetrics(store, "local", metrics)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	jobLabels := metrix.Labels{"job_name": "local"}
	if v, ok := store.Read().Value("snmp_trap_pipeline_write_failed", jobLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_pipeline_write_failed = %v/%v, want 1/true", v, ok)
	}
	labels := metrix.Labels{"job_name": "local", "source_id": sourceID, "source_kind": sourceKind}
	for metric, expected := range map[string]float64{
		"snmp_trap_source_pipeline_write_failed":       1,
		"snmp_trap_source_errors_journal_write_failed": 1,
		"snmp_trap_source_errors_otlp_export_failed":   1,
	} {
		if v, ok := store.Read().Value(metric, labels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, expected)
		}
	}
}

func testFanoutTrapEntry() *TrapEntry {
	return &TrapEntry{
		JobName:       "local",
		Message:       "trap",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &TrapEnrichmentAudit{Source: &TrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
	}
}

func TestFanoutTrapWriterPrimaryFailureStillAttemptsSecondaryWrite(t *testing.T) {
	primaryErr := errors.New("primary failed")
	primary := &mockTrapWriter{err: primaryErr}
	secondary := &mockTrapWriter{}
	metrics := &perJobMetrics{}
	writer := newFanoutTrapWriter(primary, secondary, metrics)

	err := writer.Write(&TrapEntry{JobName: "local", Message: "trap"})
	require.ErrorIs(t, err, primaryErr)
	assert.Len(t, primary.entries, 0)
	assert.Len(t, secondary.entries, 1)
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed.Load())
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
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed.Load())
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
	assert.True(t, secondary.closed)
	assert.Equal(t, 1, primary.closeAttempts)
	assert.Equal(t, 1, secondary.closeAttempts)
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed.Load())
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
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed.Load())
}

func TestFanoutTrapWriterForwardsBinaryEncodedFieldsFromPrimary(t *testing.T) {
	primary := &mockTrapWriter{binaryEncodedFields: 7}
	secondary := &mockTrapWriter{}
	writer := newFanoutTrapWriter(primary, secondary, nil)

	binaryEncoded, ok := writer.(interface{ BinaryEncodedFields() uint64 })
	require.True(t, ok)
	assert.Equal(t, uint64(7), binaryEncoded.BinaryEncodedFields())
}

func TestJournalTrapWriterConcurrentWriteCloseDoesNotPanic(t *testing.T) {
	for range 50 {
		writer := newJournalTrapWriter(nil, 2)
		start := make(chan struct{})
		done := make(chan struct{})

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				<-start
				for {
					err := writer.Write(&TrapEntry{JobName: "local", Message: "trap"})
					if errors.Is(err, errWriterClosed) {
						return
					}
					if err != nil && !errors.Is(err, errQueueFull) {
						return
					}
				}
			})
		}

		close(start)
		time.Sleep(time.Millisecond)
		require.NoError(t, writer.Close())

		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("writer goroutines did not stop after Close")
		}
	}
}

func TestJournalRetentionSweepInterval(t *testing.T) {
	tests := map[string]struct {
		cfg  JournalConfig
		want time.Duration
	}{
		"no retention": {
			cfg:  JournalConfig{},
			want: 0,
		},
		"size only": {
			cfg:  JournalConfig{MaxSize: defaultMaxSize},
			want: maxRetentionSweepInterval,
		},
		"duration below minimum": {
			cfg:  JournalConfig{MaxDuration: time.Second},
			want: minRetentionSweepInterval,
		},
		"duration capped": {
			cfg:  JournalConfig{MaxDuration: 4 * maxRetentionSweepInterval},
			want: maxRetentionSweepInterval,
		},
		"rotation duration caps size only": {
			cfg:  JournalConfig{MaxSize: defaultMaxSize, RotateDur: 5 * time.Minute},
			want: 5 * time.Minute,
		},
	}

	if got := journalRetentionSweepInterval(nil); got != 0 {
		t.Fatalf("nil journal interval = %v, want 0", got)
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := journalRetentionSweepInterval(&JournalWriter{cfg: tc.cfg})
			assert.Equal(t, tc.want, got)
		})
	}
}
