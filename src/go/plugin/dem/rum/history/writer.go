// SPDX-License-Identifier: GPL-3.0-or-later

// Package history persists and queries RUM investigation records. Its queued
// writer drains accepted records after the owning site's producers have joined.
package history

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

// The existing burst budget bounds queued browser detail; a full queue counts
// drops without blocking measurement. Batches amortize journal syncs.
const (
	maxBatch    = 500
	batchPeriod = 5 * time.Second
	queueCap    = 5000
)

// Counters receives the writer's accounting; the owning site job publishes it.
// Event calls Add under the producer's lock when the queue is full, so Add must
// be a short update under at most a leaf lock, with no I/O and no call back
// into the producer.
type Counters interface {
	Add(counter string, n uint64)
}

// Counter names reported through Counters: the rum.history chart dimensions.
const (
	CounterWritten = "history_written"
	CounterDropped = "history_dropped"
)

type eventWriter interface {
	AppendEvent(context.Context, EventRecord) (attempted bool, err error)
	Sync(context.Context) error
}

type Writer struct {
	st       eventWriter
	counters Counters
	ch       chan EventRecord
	// Run owns pendingSync; attempted appends can need a sync even after an error.
	pendingSync bool
}

// NewWriter persists receiver-normalized observations without transforming them again.
func NewWriter(st *Store, counters Counters) *Writer {
	return &Writer{
		st:       st,
		counters: counters,
		ch:       make(chan EventRecord, queueCap),
	}
}

// Event enqueues one record without blocking; a full queue counts it as dropped.
func (w *Writer) Event(rec EventRecord) {
	select {
	case w.ch <- rec:
	default:
		w.counters.Add(CounterDropped, 1)
	}
}

// Run preserves only entries that were never admitted to append. Each attempted
// append is counted once, even if retirement races success or a disk error.
// The SDK cannot interrupt a filesystem write; contexts bound admission and
// subsequent work, and the host's fail-stop path owns a non-quiescent process.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]EventRecord, 0, maxBatch)
	// flushSteady keeps an unattempted suffix for the final drain and reports
	// whether the whole batch was consumed.
	flushSteady := func() bool {
		consumed := w.flush(ctx, batch)
		if consumed < len(batch) {
			batch = batch[consumed:]
			return false
		}
		batch = batch[:0]
		return true
	}
steady:
	for {
		select {
		case <-ctx.Done():
			break steady
		case rec := <-w.ch:
			batch = append(batch, rec)
			if len(batch) >= maxBatch && !flushSteady() {
				break steady
			}
		case <-ticker.C:
			if !flushSteady() {
				break steady
			}
		}
	}
	finalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		select {
		case rec := <-w.ch:
			batch = append(batch, rec)
		default:
			remaining := batch[w.flush(finalCtx, batch):]
			if len(remaining) != 0 {
				w.counters.Add(CounterDropped, uint64(len(remaining)))
			}
			return
		}
	}
}

// flush returns the consumed prefix. Only cancelled admission leaves a suffix
// for final drain: journal appends have no batch transaction or rollback.
func (w *Writer) flush(ctx context.Context, batch []EventRecord) int {
	if len(batch) == 0 && !w.pendingSync {
		return 0
	}
	for i, rec := range batch {
		if ctx.Err() != nil {
			return i
		}
		attempted, err := w.st.AppendEvent(ctx, rec)
		if attempted {
			w.pendingSync = true
		}
		if !attempted && ctx.Err() != nil {
			return i
		}
		counter := CounterWritten
		if err != nil {
			counter = CounterDropped
		}
		w.counters.Add(counter, 1)
	}
	if w.pendingSync {
		if err := w.st.Sync(ctx); err == nil {
			w.pendingSync = false
		} else if ctx.Err() == nil {
			logger.New().Warningf("syncing RUM journal failed: %v", err)
		}
	}
	return len(batch)
}
