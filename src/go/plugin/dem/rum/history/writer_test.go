// SPDX-License-Identifier: GPL-3.0-or-later
package history

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCounters struct {
	mu     sync.Mutex
	values map[[2]string]uint64
}

func newFakeCounters() *fakeCounters {
	return &fakeCounters{
		values: map[[2]string]uint64{},
	}
}
func (c *fakeCounters) Add(site, key string, n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[[2]string{site, key}] += n
}
func (c *fakeCounters) get(site, key string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[[2]string{site, key}]
}

type eventWriterFunc func(context.Context, store.RumEventRecord) (bool, error)

func (f eventWriterFunc) AppendRumEvent(ctx context.Context, r store.RumEventRecord) (bool, error) {
	return f(ctx, r)
}
func (eventWriterFunc) Sync(context.Context) error { return nil }

func drain(w *Writer) { ctx, cancel := context.WithCancel(context.Background()); cancel(); w.Run(ctx) }

func TestWriterDrainsRealJournalWithRedaction(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	counters := newFakeCounters()
	w := New(st, counters, secrets.NewRedactor("opaque-secret"))
	now := time.Now().UnixMicro()
	w.Event(
		agg.HistoryEvent{
			Site:      "site",
			SessionID: "session",
			TSUnixUS:  now,
			Type:      "pageview",
			Page:      "/opaque-secret",
			UserID:    "opaque-secret",
			Browser:   "Chrome",
		},
	)
	w.Event(
		agg.HistoryEvent{
			Site:        "site",
			SessionID:   "session",
			TSUnixUS:    now + 1,
			Type:        "error",
			Page:        "/opaque-secret",
			UserID:      "opaque-secret",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "opaque-secret failed",
			SampleStack: "at opaque-secret",
			Browser:     "Chrome",
		},
	)
	drain(w)
	rows, err := st.QueryRumSessionEvents(ctx, "site", "session")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "/[REDACTED]", rows[0].Page)
	sessions, err := st.QueryRumSessions(ctx, "site", 0, time.Now().Unix()+1, 2000)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "[REDACTED]", sessions[0].UserID)
	assert.EqualValues(t, 1, sessions[0].Pageviews)
	assert.EqualValues(t, 1, sessions[0].Errors)
	groups, err := st.QueryRumErrors(ctx, "site", "fp", 0, time.Now().Unix()+1)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "[REDACTED] failed", groups[0].Message)
	assert.Equal(t, "at [REDACTED]", groups[0].SampleStack)
	assert.EqualValues(t, 2, counters.get("site", agg.CounterHistoryWritten))
	assert.Zero(t, counters.get("site", agg.CounterHistoryDropped))
}
func TestQueueOverflowReportsOnlyRejectedRecords(t *testing.T) {
	counters := newFakeCounters()
	w := New(nil, counters, nil)
	calls := 0
	w.st = eventWriterFunc(func(context.Context, store.RumEventRecord) (bool, error) { calls++; return true, nil })
	for i := 0; i < queueCap+7; i++ {
		w.Event(agg.HistoryEvent{
			Site: "site",
			Type: "event",
		})
	}
	drain(w)
	assert.Equal(t, queueCap, calls)
	assert.EqualValues(t, queueCap, counters.get("site", agg.CounterHistoryWritten))
	assert.EqualValues(t, 7, counters.get("site", agg.CounterHistoryDropped))
}
func TestCancellationPreservesOnlyUnattemptedSuffix(t *testing.T) {
	counters := newFakeCounters()
	w := New(nil, counters, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := map[string]int{}
	calls := 0
	w.st = eventWriterFunc(func(writeCtx context.Context, r store.RumEventRecord) (bool, error) {
		calls++
		if calls == 4 {
			cancel()
			return false, writeCtx.Err()
		}
		attempts[r.Text]++
		return true, nil
	})
	for i := 0; i < maxBatch+7; i++ {
		w.Event(agg.HistoryEvent{
			Site: "site",
			Type: "event",
			Text: fmt.Sprint(i),
		})
	}
	w.Run(ctx)
	require.Len(t, attempts, maxBatch+7)
	for text, n := range attempts {
		assert.Equal(t, 1, n, "event %s replayed", text)
	}
	assert.EqualValues(t, maxBatch+7, counters.get("site", agg.CounterHistoryWritten))
	assert.Zero(t, counters.get("site", agg.CounterHistoryDropped))
}
func TestAttemptedAppendIsNeverReplayed(t *testing.T) {
	for name, writeErr := range map[string]error{"disk error": errors.New("disk full"), "cancelled write": context.Canceled, "successful cancellation race": nil} {
		t.Run(name, func(t *testing.T) {
			counters := newFakeCounters()
			w := New(nil, counters, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			w.st = eventWriterFunc(func(_ context.Context, r store.RumEventRecord) (bool, error) {
				calls++
				if calls == 1 {
					cancel()
					return true, writeErr
				}
				return true, nil
			})
			for range maxBatch {
				w.Event(agg.HistoryEvent{
					Site: "site",
					Type: "event",
				})
			}
			w.Run(ctx)
			assert.Equal(t, maxBatch, calls)
			wantWritten, wantDropped := maxBatch, 0
			if writeErr != nil {
				wantWritten--
				wantDropped++
			}
			assert.EqualValues(t, wantWritten, counters.get("site", agg.CounterHistoryWritten))
			assert.EqualValues(t, wantDropped, counters.get("site", agg.CounterHistoryDropped))
		})
	}
}
func TestFinalDrainSharesOneAdmissionDeadline(t *testing.T) {
	counters := newFakeCounters()
	w := New(nil, counters, nil)
	deadlines := []time.Time{}
	w.st = eventWriterFunc(func(ctx context.Context, r store.RumEventRecord) (bool, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		deadlines = append(deadlines, deadline)
		return true, nil
	})
	for range maxBatch + 7 {
		w.Event(agg.HistoryEvent{
			Site: "site",
			Type: "event",
		})
	}
	drain(w)
	require.Len(t, deadlines, maxBatch+7)
	for _, d := range deadlines {
		assert.Equal(t, deadlines[0], d)
	}
}

// Empty timer ticks must not dirty and fsync the shared journal per idle site.
type recordingEventWriter struct {
	appends int
	syncs   int
	syncErr error
}

func (w *recordingEventWriter) AppendRumEvent(context.Context, store.RumEventRecord) (bool, error) {
	w.appends++
	return true, nil
}
func (w *recordingEventWriter) Sync(context.Context) error {
	w.syncs++
	return w.syncErr
}

func TestEmptyFlushSkipsJournalSyncAndReportsQueueDrops(t *testing.T) {
	counters := newFakeCounters()
	w := New(nil, counters, nil)
	disk := &recordingEventWriter{}
	w.st = disk
	for range queueCap + 1 {
		w.Event(agg.HistoryEvent{
			Site: "site",
		})
	}
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Zero(t, disk.syncs)
	assert.EqualValues(t, 1, counters.get("site", agg.CounterHistoryDropped))
	assert.Equal(t, 1, w.flush(context.Background(), []agg.HistoryEvent{{Site: "site", Type: "event"}}))
	assert.Equal(t, 1, disk.appends)
	assert.Equal(t, 1, disk.syncs)
}

type cancelLastAppendWriter struct {
	cancel  context.CancelFunc
	appends int
	synced  bool
}

func (w *cancelLastAppendWriter) AppendRumEvent(context.Context, store.RumEventRecord) (bool, error) {
	w.appends++
	if w.appends == maxBatch {
		w.cancel()
	}
	return true, nil
}
func (w *cancelLastAppendWriter) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.synced = true
	return nil
}
func TestFinalDrainSyncsAfterCancellationFollowingLastAppend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := New(nil, newFakeCounters(), nil)
	disk := &cancelLastAppendWriter{
		cancel: cancel,
	}
	w.st = disk
	for range maxBatch {
		w.Event(agg.HistoryEvent{
			Site: "site",
			Type: "event",
		})
	}
	w.Run(ctx)
	assert.Equal(t, maxBatch, disk.appends)
	assert.True(t, disk.synced, "final drain must sync accepted appends with its detached context")
}
func TestIdleFlushRetriesFailedSyncWithoutReplayingAppend(t *testing.T) {
	w := New(nil, newFakeCounters(), nil)
	disk := &recordingEventWriter{
		syncErr: errors.New("sync failed"),
	}
	w.st = disk
	assert.Equal(t, 1, w.flush(context.Background(), []agg.HistoryEvent{{Site: "site", Type: "event"}}))
	disk.syncErr = nil
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Equal(t, 1, disk.appends)
	assert.Equal(t, 2, disk.syncs)
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Equal(t, 2, disk.syncs, "successful sync restores the idle shortcut")
}
