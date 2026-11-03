// SPDX-License-Identifier: GPL-3.0-or-later

// Package history persists and queries RUM investigation records. Its queued
// writer drains accepted records after the owning site's producers have joined.
package history

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
)

// The existing burst budget bounds queued browser detail; a full queue reports
// drops without blocking measurement. Small batches amortize counter updates.
const (
	maxBatch    = 500
	batchPeriod = 5 * time.Second
	queueCap    = 5000
)

type Counters interface {
	Add(counter string, n uint64)
}
type eventWriter interface {
	AppendEvent(context.Context, EventRecord) (attempted bool, err error)
	Sync(context.Context) error
}

type Writer struct {
	siteName  string
	st        eventWriter
	counters  Counters
	ch        chan aggregate.HistoryEvent
	dropMu    sync.Mutex
	chanDrops uint64
	// Run owns pendingSync; attempted appends can need a sync even after an error.
	pendingSync bool
}

// NewWriter persists receiver-normalized observations without transforming them again.
func NewWriter(siteName string, st *Store, counters Counters) *Writer {
	return &Writer{
		st:       st,
		counters: counters,
		ch:       make(chan aggregate.HistoryEvent, queueCap),
		siteName: siteName,
	}
}

func (w *Writer) Event(rec aggregate.HistoryEvent) {
	if rec.Site != w.siteName {
		return
	}
	select {
	case w.ch <- rec:
	default:
		w.dropMu.Lock()
		w.chanDrops++
		w.dropMu.Unlock()
	}
}
func (w *Writer) reportQueueDrops() {
	w.dropMu.Lock()
	drops := w.chanDrops
	w.chanDrops = 0
	w.dropMu.Unlock()
	if drops != 0 {
		w.counters.Add(aggregate.CounterHistoryDropped, drops)
	}
}

// Run preserves only entries that were never admitted to append. Each attempted
// append is counted once, even if retirement races success or a disk error.
// The SDK cannot interrupt a filesystem write; contexts bound admission and
// subsequent work, and the host's fail-stop path owns a non-quiescent process.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]aggregate.HistoryEvent, 0, maxBatch)
steady:
	for {
		select {
		case <-ctx.Done():
			break steady
		case rec := <-w.ch:
			batch = append(batch, rec)
			if len(batch) >= maxBatch {
				consumed := w.flush(ctx, batch)
				if consumed == len(batch) {
					batch = batch[:0]
				} else {
					batch = batch[consumed:]
				}
				if len(batch) != 0 {
					break steady
				}
			}
		case <-ticker.C:
			consumed := w.flush(ctx, batch)
			if consumed == len(batch) {
				batch = batch[:0]
			} else {
				batch = batch[consumed:]
			}
			if len(batch) != 0 {
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
				w.counters.Add(aggregate.CounterHistoryDropped, uint64(len(remaining)))
			}
			return
		}
	}
}

// flush returns the consumed prefix. Only cancelled admission leaves a suffix
// for final drain: journal appends have no batch transaction or rollback.
func (w *Writer) flush(ctx context.Context, batch []aggregate.HistoryEvent) int {
	w.reportQueueDrops()
	if len(batch) == 0 && !w.pendingSync {
		return 0
	}
	for i, rec := range batch {
		if ctx.Err() != nil {
			return i
		}
		r := EventRecord{
			ExperienceID: rec.ExperienceID,
			View:         rec.View,
			ViewID:       rec.ViewID,
			MetricID:     rec.MetricID,
			Revision:     rec.Revision,

			Site:        rec.Site,
			SessionID:   rec.SessionID,
			ObservedUS:  rec.ObservedUS,
			Type:        rec.Type,
			Page:        rec.Page,
			Text:        rec.Text,
			TraceID:     rec.TraceID,
			Browser:     rec.Browser,
			Device:      rec.Device,
			Country:     rec.Country,
			Version:     rec.Version,
			UserID:      rec.UserID,
			Fingerprint: rec.Fingerprint,
			ErrorType:   rec.ErrorType,
			Message:     rec.Message,
			SampleStack: rec.SampleStack,
		}
		attempted, err := w.st.AppendEvent(ctx, r)
		if attempted {
			w.pendingSync = true
		}
		if !attempted && ctx.Err() != nil {
			return i
		}
		counter := aggregate.CounterHistoryWritten
		if err != nil {
			counter = aggregate.CounterHistoryDropped
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
