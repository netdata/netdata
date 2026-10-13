// SPDX-License-Identifier: GPL-3.0-or-later
package history

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCounters struct {
	mu     sync.Mutex
	values map[string]uint64
}

func newFakeCounters() *fakeCounters {
	return &fakeCounters{
		values: map[string]uint64{},
	}
}
func (c *fakeCounters) Add(key string, n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] += n
}
func (c *fakeCounters) get(key string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key]
}

type eventWriterFunc func(context.Context, EventRecord) (bool, error)

func (f eventWriterFunc) AppendEvent(ctx context.Context, r EventRecord) (bool, error) {
	return f(ctx, r)
}
func (eventWriterFunc) Sync(context.Context) error { return nil }

func drain(w *Writer) { ctx, cancel := context.WithCancel(context.Background()); cancel(); w.Run(ctx) }

func TestWriterDrainsAlreadyNormalizedRecords(t *testing.T) {
	ctx := context.Background()
	st, err := demjournal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	counters := newFakeCounters()
	w := NewWriter("site", NewStore(st), counters)
	now := time.Now().UnixMicro()
	w.Event(
		aggregate.HistoryEvent{
			Site:       "site",
			SessionID:  "session",
			ObservedUS: now,
			Type:       "pageview",
			Page:       "/[REDACTED]",
			UserID:     "[REDACTED]",
			Browser:    "Chrome",
		},
	)
	w.Event(
		aggregate.HistoryEvent{
			Site:        "site",
			SessionID:   "session",
			ObservedUS:  now + 1,
			Type:        "error",
			Page:        "/[REDACTED]",
			UserID:      "[REDACTED]",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "[REDACTED] failed",
			SampleStack: "at [REDACTED]",
			Browser:     "Chrome",
		},
	)
	drain(w)
	rows, err := NewStore(st).QuerySessionEvents(ctx, "site", "session")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "/[REDACTED]", rows[0].Page)
	sessions, err := NewStore(st).QuerySessions(ctx, "site", "", 0, time.Now().Unix()+1, 2000)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, []string{"[REDACTED]"}, sessions[0].UserIDs)
	assert.EqualValues(t, 1, sessions[0].Pageviews)
	assert.EqualValues(t, 1, sessions[0].Errors)
	groups, err := NewStore(st).QueryErrors(ctx, "site", "fp", 0, time.Now().Unix()+1)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "[REDACTED] failed", groups[0].Message)
	assert.Equal(t, "at [REDACTED]", groups[0].SampleStack)
	assert.EqualValues(t, 2, counters.get(aggregate.CounterHistoryWritten))
	assert.Zero(t, counters.get(aggregate.CounterHistoryDropped))
}
func TestQueueOverflowReportsOnlyRejectedRecords(t *testing.T) {
	counters := newFakeCounters()
	w := NewWriter("site", nil, counters)
	calls := 0
	w.st = eventWriterFunc(func(context.Context, EventRecord) (bool, error) { calls++; return true, nil })
	for i := 0; i < queueCap+7; i++ {
		w.Event(aggregate.HistoryEvent{
			Site: "site",
			Type: "event",
		})
	}
	drain(w)
	assert.Equal(t, queueCap, calls)
	assert.EqualValues(t, queueCap, counters.get(aggregate.CounterHistoryWritten))
	assert.EqualValues(t, 7, counters.get(aggregate.CounterHistoryDropped))
}
func TestCancellationPreservesOnlyUnattemptedSuffix(t *testing.T) {
	counters := newFakeCounters()
	w := NewWriter("site", nil, counters)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := map[string]int{}
	calls := 0
	w.st = eventWriterFunc(func(writeCtx context.Context, r EventRecord) (bool, error) {
		calls++
		if calls == 4 {
			cancel()
			return false, writeCtx.Err()
		}
		attempts[r.Text]++
		return true, nil
	})
	for i := 0; i < maxBatch+7; i++ {
		w.Event(aggregate.HistoryEvent{
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
	assert.EqualValues(t, maxBatch+7, counters.get(aggregate.CounterHistoryWritten))
	assert.Zero(t, counters.get(aggregate.CounterHistoryDropped))
}
func TestAttemptedAppendIsNeverReplayed(t *testing.T) {
	for name, writeErr := range map[string]error{"disk error": errors.New("disk full"), "cancelled write": context.Canceled, "successful cancellation race": nil} {
		t.Run(name, func(t *testing.T) {
			counters := newFakeCounters()
			w := NewWriter("site", nil, counters)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			w.st = eventWriterFunc(func(_ context.Context, r EventRecord) (bool, error) {
				calls++
				if calls == 1 {
					cancel()
					return true, writeErr
				}
				return true, nil
			})
			for range maxBatch {
				w.Event(aggregate.HistoryEvent{
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
			assert.EqualValues(t, wantWritten, counters.get(aggregate.CounterHistoryWritten))
			assert.EqualValues(t, wantDropped, counters.get(aggregate.CounterHistoryDropped))
		})
	}
}
func TestFinalDrainSharesOneAdmissionDeadline(t *testing.T) {
	counters := newFakeCounters()
	w := NewWriter("site", nil, counters)
	deadlines := []time.Time{}
	w.st = eventWriterFunc(func(ctx context.Context, r EventRecord) (bool, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		deadlines = append(deadlines, deadline)
		return true, nil
	})
	for range maxBatch + 7 {
		w.Event(aggregate.HistoryEvent{
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

func (w *recordingEventWriter) AppendEvent(context.Context, EventRecord) (bool, error) {
	w.appends++
	return true, nil
}
func (w *recordingEventWriter) Sync(context.Context) error {
	w.syncs++
	return w.syncErr
}

func TestEmptyFlushSkipsJournalSyncAndReportsQueueDrops(t *testing.T) {
	counters := newFakeCounters()
	w := NewWriter("site", nil, counters)
	disk := &recordingEventWriter{}
	w.st = disk
	for range queueCap + 1 {
		w.Event(aggregate.HistoryEvent{
			Site: "site",
		})
	}
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Zero(t, disk.syncs)
	assert.EqualValues(t, 1, counters.get(aggregate.CounterHistoryDropped))
	assert.Equal(t, 1, w.flush(context.Background(), []aggregate.HistoryEvent{{Site: "site", Type: "event"}}))
	assert.Equal(t, 1, disk.appends)
	assert.Equal(t, 1, disk.syncs)
}

type cancelLastAppendWriter struct {
	cancel  context.CancelFunc
	appends int
	synced  bool
}

func (w *cancelLastAppendWriter) AppendEvent(context.Context, EventRecord) (bool, error) {
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
	w := NewWriter("site", nil, newFakeCounters())
	disk := &cancelLastAppendWriter{
		cancel: cancel,
	}
	w.st = disk
	for range maxBatch {
		w.Event(aggregate.HistoryEvent{
			Site: "site",
			Type: "event",
		})
	}
	w.Run(ctx)
	assert.Equal(t, maxBatch, disk.appends)
	assert.True(t, disk.synced, "final drain must sync accepted appends with its detached context")
}
func TestIdleFlushRetriesFailedSyncWithoutReplayingAppend(t *testing.T) {
	w := NewWriter("site", nil, newFakeCounters())
	disk := &recordingEventWriter{
		syncErr: errors.New("sync failed"),
	}
	w.st = disk
	assert.Equal(t, 1, w.flush(context.Background(), []aggregate.HistoryEvent{{Site: "site", Type: "event"}}))
	disk.syncErr = nil
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Equal(t, 1, disk.appends)
	assert.Equal(t, 2, disk.syncs)
	assert.Zero(t, w.flush(context.Background(), nil))
	assert.Equal(t, 2, disk.syncs, "successful sync restores the idle shortcut")
}

func TestWrongSiteDoesNotQueueOrCount(t *testing.T) {
	st, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	counters := newFakeCounters()
	w := NewWriter("site", NewStore(st), counters)
	for range queueCap + 1 {
		w.Event(aggregate.HistoryEvent{
			Site:       "other",
			SessionID:  "session",
			Type:       "event",
			ObservedUS: time.Now().UnixMicro(),
		})
	}
	require.Empty(t, w.ch)
	drain(w)
	require.Empty(t, counters.values)
	rows, err := NewStore(st).QuerySessionEvents(context.Background(), "other", "session")
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = NewStore(st).QuerySessionEvents(context.Background(), "site", "session")
	require.NoError(t, err)
	require.Empty(t, rows)
}
