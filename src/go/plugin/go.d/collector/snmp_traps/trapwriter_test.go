// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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

	store, labels := collectSourceMetricsForEntry(t, metrics, entry)
	assertSourceMetricValue(t, store, "snmp_trap_source_pipeline_write_failed", labels, 0)
	assertSourceMetricValue(t, store, "snmp_trap_source_errors_otlp_export_failed", labels, 1)
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

// newJournalTrapWriterWithInterval mirrors newJournalTrapWriter but lets a test
// shorten the flush ticker so the time-only flush is observable without waiting
// a full second. It constructs the writer directly because flushInterval is read
// once when the worker starts, so mutating it after newJournalTrapWriter would race.
func newJournalTrapWriterWithInterval(j *JournalWriter, capacity int, interval time.Duration) *journalTrapWriter {
	tw := &journalTrapWriter{
		journal:                j,
		queue:                  make(chan *TrapEntry, capacity),
		flushCh:                make(chan chan error),
		doneCh:                 make(chan struct{}),
		flushInterval:          interval,
		retentionSweepInterval: journalRetentionSweepInterval(j),
		lastRetentionSweep:     time.Now(),
	}
	go tw.worker()
	return tw
}

// TestJournalTrapWriterTickerFlushesWithoutCountTrigger pins the time-only flush
// contract introduced when the count-based fsync trigger (defaultFlushEntries=1000)
// was removed: a handful of entries — far below the old 1000-entry threshold — must
// be fsynced to a real journal by the interval ticker alone, with no explicit
// Flush(), and must remain queryable after Close(). If a future change re-introduces
// a count gate or drops the ticker flush, this test fails.
func TestJournalTrapWriterTickerFlushesWithoutCountTrigger(t *testing.T) {
	requireJournalctl(t)

	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	tw := newJournalTrapWriterWithInterval(w, 1<<10, 100*time.Millisecond)

	const want = 5
	for range want {
		require.NoError(t, tw.Write(benchmarkTrapEntry()))
	}

	// No Flush(): the interval ticker must fsync these 5 entries on its own.
	// Bound the whole wait with a context so a stalled journalctl child is killed
	// rather than blocking past the deadline (the loop only re-checks the clock
	// between calls).
	journalDir := w.JournalDirectory()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	for ctx.Err() == nil {
		count = journalctlRowCount(ctx, journalDir, "TRAP_CATEGORY=security", "TRAP_OID")
		if count >= want {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.GreaterOrEqualf(t, count, want, "ticker did not flush %d entries without an explicit Flush", want)

	require.NoError(t, tw.Close())

	// Entries remain durable after Close.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	require.GreaterOrEqual(t, journalctlRowCount(ctx2, journalDir, "TRAP_CATEGORY=security", "TRAP_OID"), want)
}

// journalctlRowCount runs journalctl under ctx (so a stalled child is killed when
// ctx expires) and returns how many output rows contain field.
func journalctlRowCount(ctx context.Context, dir, match, field string) int {
	out, _ := exec.CommandContext(ctx, "journalctl", "--directory="+dir, match, "-o", "json", "--no-pager").CombinedOutput()
	return strings.Count(string(out), field)
}
