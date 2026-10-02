// SPDX-License-Identifier: GPL-3.0-or-later

// Package history batches RUM records from a bounded channel into SQLite.
// Each site owns a writer, which flushes on timer/size triggers and drains
// accepted records on shutdown after ingestion has stopped.
package history

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
)

// Batching flushes after 5 seconds or 500 records. The 5000-record queue
// bounds memory while absorbing bursts between flushes; overflow is counted
// as dropped without blocking ingestion.
const (
	maxBatch    = 500
	batchPeriod = 5 * time.Second
	queueCap    = 5000
)

// Counters receives the rum.history chart bookkeeping (written/dropped
// dims). *agg.Aggregator satisfies this directly.
type Counters interface {
	Add(site, counter string, n uint64)
}

// Redactor scrubs secret-shaped content before storage. It runs off the
// aggregator lock, rather than in HistorySink methods: regex scans must
// not hold a.mu (see the HistorySink constant-time enqueue contract).
type Redactor interface {
	Apply(string) string
}

// recKind tags which payload a queued record carries.
type recKind int

const (
	kindSession recKind = iota
	kindEvent
	kindErrGroup
	kindErrOcc
)

type queued struct {
	kind     recKind
	session  agg.HistorySession
	event    agg.HistorySessionEvent
	errGroup agg.HistoryErrorGroup
	errOcc   agg.HistoryErrorOccurrence
}

func (q queued) site() string {
	switch q.kind {
	case kindSession:
		return q.session.Site
	case kindEvent:
		return q.event.Site
	case kindErrGroup:
		return q.errGroup.Site
	default:
		return q.errOcc.Site
	}
}

// Writer implements agg.HistorySink and drives the batch flush loop.
type Writer struct {
	st       *store.Store
	counters Counters
	redact   Redactor
	ch       chan queued

	// chanDrops accumulates channel-full drops between flushes without
	// blocking ingestion. Run merges them into the rum.history dropped
	// counter. dropMu is separate from the aggregator lock: HistorySink
	// methods must never call back into the aggregator while it holds a.mu.
	dropMu    sync.Mutex
	chanDrops map[string]int
}

// New builds a writer. redactor may be nil (no redaction — callers should
// always pass the real one; nil only eases tests).
func New(st *store.Store, counters Counters, redactor Redactor) *Writer {
	if redactor == nil {
		redactor = noopRedactor{}
	}
	return &Writer{
		st:        st,
		counters:  counters,
		redact:    redactor,
		ch:        make(chan queued, queueCap),
		chanDrops: map[string]int{},
	}
}

type noopRedactor struct{}

func (noopRedactor) Apply(s string) string { return s }

// ---- agg.HistorySink: called with the aggregator's lock held ----

func (w *Writer) Session(rec agg.HistorySession) {
	w.enqueue(queued{
		kind:    kindSession,
		session: rec,
	})
}
func (w *Writer) SessionEvent(rec agg.HistorySessionEvent) {
	w.enqueue(queued{
		kind:  kindEvent,
		event: rec,
	})
}
func (w *Writer) ErrorGroup(rec agg.HistoryErrorGroup) {
	w.enqueue(queued{
		kind:     kindErrGroup,
		errGroup: rec,
	})
}
func (w *Writer) ErrorOccurrence(rec agg.HistoryErrorOccurrence) {
	w.enqueue(queued{
		kind:   kindErrOcc,
		errOcc: rec,
	})
}

// enqueue never blocks: a full channel drops the record and counts it
// locally (flushed into the shared counters later, from Run's goroutine).
func (w *Writer) enqueue(q queued) {
	select {
	case w.ch <- q:
	default:
		w.dropMu.Lock()
		w.chanDrops[q.site()]++
		w.dropMu.Unlock()
	}
}

// takeChanDrops atomically reads and resets the accumulated drop counts.
func (w *Writer) takeChanDrops() map[string]int {
	w.dropMu.Lock()
	defer w.dropMu.Unlock()
	drops := w.chanDrops
	w.chanDrops = map[string]int{}
	return drops
}

// ---- flush loop ----

// Run batches while producers are active and drains accepted records on stop.
// The owner must join producers before cancellation. Final SQL work has a
// fixed five-second deadline independent of public collection settings.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]queued, 0, maxBatch)
	flush := func() {
		w.flush(ctx, batch)
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			finalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Drain whatever is already queued before the final flush —
			// unlike the steady-state ticks, shutdown gets exactly one
			// last transaction, so nothing enqueued before this point is
			// silently lost ("flushed on shutdown").
			for drained := false; !drained; {
				select {
				case q := <-w.ch:
					batch = append(batch, q)
				default:
					drained = true
				}
			}
			w.flush(finalCtx, batch)
			return
		case q := <-w.ch:
			batch = append(batch, q)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// flush partitions one batch by record kind, writes it in one transaction,
// and reports written/dropped counts per site on rum.history.
func (w *Writer) flush(ctx context.Context, batch []queued) {
	chanDrops := w.takeChanDrops()
	if len(batch) == 0 && len(chanDrops) == 0 {
		return
	}
	var b store.RumHistoryBatch
	for _, q := range batch {
		switch q.kind {
		case kindSession:
			r := q.session
			b.Sessions = append(b.Sessions, store.RumSessionRecord{
				Site:      r.Site,
				SessionID: r.SessionID,
				StartedAt: r.StartedAt,
				LastAt:    r.LastAt,
				Pageviews: int64(r.Pageviews),
				Errors:    int64(r.Errors),
				Browser: w.redact.Apply(
					r.Browser,
				),
				Device:       w.redact.Apply(r.Device),
				Country:      w.redact.Apply(r.Country),
				City:         w.redact.Apply(r.City),
				Version:      w.redact.Apply(r.Version),
				EntryPage:    w.redact.Apply(r.EntryPage),
				LastPage:     w.redact.Apply(r.LastPage),
				UserID:       w.redact.Apply(r.UserID),
				Frustrations: int64(r.Frustrations),
			})
		case kindEvent:
			r := q.event
			b.Events = append(b.Events, store.RumSessionEventRecord{
				Site:      r.Site,
				SessionID: r.SessionID,
				TSUnixUS:  r.TSUnixUS,
				Type: w.redact.Apply(
					r.Type,
				),
				Page:    w.redact.Apply(r.Page),
				Text:    w.redact.Apply(r.Text),
				TraceID: r.TraceID,
			})
		case kindErrGroup:
			r := q.errGroup
			b.ErrGroups = append(b.ErrGroups, store.RumErrorGroupRecord{
				Site:        r.Site,
				Fingerprint: r.Fingerprint,
				Type:        w.redact.Apply(r.Type),
				Message:     w.redact.Apply(r.Message),
				SampleStack: w.redact.Apply(r.SampleStack),
				FirstSeen:   r.FirstSeen,
				LastSeen:    r.LastSeen,
			})
		case kindErrOcc:
			r := q.errOcc
			b.ErrOccs = append(b.ErrOccs, store.RumErrorOccurrenceRecord{
				Site:        r.Site,
				Fingerprint: r.Fingerprint,
				TS:          r.TS,
				SessionID:   r.SessionID,
				Page:        w.redact.Apply(r.Page),
				Browser:     w.redact.Apply(r.Browser),
			})
		}
	}

	written, dropped, err := w.st.WriteRumHistoryBatch(ctx, b)
	if err != nil {
		// A failed flush counts the batch as dropped. Retrying could stall the
		// next flush behind a persistently failing write.
		dropped = map[string]int{}
		for _, q := range batch {
			dropped[q.site()]++
		}
	}
	for site, n := range written {
		if n > 0 {
			w.counters.Add(site, agg.CounterHistoryWritten, uint64(n))
		}
	}
	for site, n := range dropped {
		if n > 0 {
			w.counters.Add(site, agg.CounterHistoryDropped, uint64(n))
		}
	}
	for site, n := range chanDrops {
		if n > 0 {
			w.counters.Add(site, agg.CounterHistoryDropped, uint64(n))
		}
	}
}
