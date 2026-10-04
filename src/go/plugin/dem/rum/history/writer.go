// SPDX-License-Identifier: GPL-3.0-or-later

// Package history queues investigation events off the ingestion lock and drains
// accepted records after the owning site's producers have joined.
package history

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
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
type Redactor interface{ Apply(string) string }
type eventWriter interface {
	AppendRumEvent(context.Context, store.RumEventRecord) (attempted bool, err error)
	Sync(context.Context) error
}

type Writer struct {
	siteName  string
	st        eventWriter
	counters  Counters
	redact    Redactor
	ch        chan agg.HistoryEvent
	dropMu    sync.Mutex
	chanDrops uint64
	// Run owns pendingSync; attempted appends can need a sync even after an error.
	pendingSync bool
}

func New(siteName string, st *store.Store, counters Counters, redactor Redactor) *Writer {
	if redactor == nil {
		redactor = noopRedactor{}
	}
	return &Writer{
		st:       st,
		counters: counters,
		redact:   redactor,
		ch:       make(chan agg.HistoryEvent, queueCap),
		siteName: siteName,
	}
}

type noopRedactor struct{}

func (noopRedactor) Apply(s string) string { return s }

func (w *Writer) Event(rec agg.HistoryEvent) {
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
		w.counters.Add(agg.CounterHistoryDropped, drops)
	}
}

// Run preserves only entries that were never admitted to append. Each attempted
// append is counted once, even if retirement races success or a disk error.
// The SDK cannot interrupt a filesystem write; contexts bound admission and
// subsequent work, and the host's fail-stop path owns a non-quiescent process.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]agg.HistoryEvent, 0, maxBatch)
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
				w.counters.Add(agg.CounterHistoryDropped, uint64(len(remaining)))
			}
			return
		}
	}
}

// flush returns the consumed prefix. Only cancelled admission leaves a suffix
// for final drain: journal appends have no batch transaction or rollback.
func (w *Writer) flush(ctx context.Context, batch []agg.HistoryEvent) int {
	w.reportQueueDrops()
	if len(batch) == 0 && !w.pendingSync {
		return 0
	}
	for i, rec := range batch {
		if ctx.Err() != nil {
			return i
		}
		r := store.RumEventRecord{
			Site:      rec.Site,
			SessionID: rec.SessionID,
			TSUnixUS:  rec.TSUnixUS,
			Type:      rec.Type,
			Page:      w.redact.Apply(rec.Page),
			Text:      w.redact.Apply(rec.Text),
			TraceID:   rec.TraceID,
			Browser: w.redact.Apply(
				rec.Browser,
			),
			Device:      w.redact.Apply(rec.Device),
			Country:     w.redact.Apply(rec.Country),
			City:        w.redact.Apply(rec.City),
			Version:     w.redact.Apply(rec.Version),
			UserID:      w.redact.Apply(rec.UserID),
			Fingerprint: rec.Fingerprint,
			ErrorType: w.redact.Apply(
				rec.ErrorType,
			),
			Message:     w.redact.Apply(rec.Message),
			SampleStack: w.redact.Apply(rec.SampleStack),
		}
		attempted, err := w.st.AppendRumEvent(ctx, r)
		if attempted {
			w.pendingSync = true
		}
		if !attempted && ctx.Err() != nil {
			return i
		}
		counter := agg.CounterHistoryWritten
		if err != nil {
			counter = agg.CounterHistoryDropped
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
