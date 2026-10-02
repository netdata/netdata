// SPDX-License-Identifier: GPL-3.0-or-later

// history.go declares the seam the aggregator uses to hand derived
// records to the persistent RUM history writer. The
// writer (plugin/dem/rum/history) implements HistorySink; agg stays
// ignorant of SQLite/redaction, same separation as beacon.Sink for OTLP.
package agg

// HistorySink receives one derived record per call, right where the
// aggregator's own session/error-group state was just updated so the
// record always matches what rum-sessions/rum-errors would show live.
//
// Every method is called from inside Ingest with a.mu held: implementations
// must return immediately — no I/O, no blocking channel send, and never a
// call back into the Aggregator (that would deadlock on the same mutex).
// A bounded non-blocking enqueue is the only correct shape here; drop
// accounting happens out of band, in the sink's own goroutine.
type HistorySink interface {
	Session(rec HistorySession)
	SessionEvent(rec HistorySessionEvent)
	ErrorGroup(rec HistoryErrorGroup)
	ErrorOccurrence(rec HistoryErrorOccurrence)
}

// HistorySession contributes newly investigated counts to a durable session.
// Counters are deltas since the previous record, including accumulated counts
// when a sampled session is promoted. Timestamps are Unix seconds.
type HistorySession struct {
	Site, SessionID                         string
	StartedAt, LastAt                       int64
	Pageviews, Errors                       uint64
	Frustrations                            uint64 // rage, dead and error clicks
	UserID                                  string // the site's own user id
	Browser, Device, Country, City, Version string
	EntryPage, LastPage                     string
}

// HistorySessionEvent is one rum_session_events insert. TSUnixUS is
// microseconds, matching the in-memory ring and the wire column.
type HistorySessionEvent struct {
	Site, SessionID  string
	TSUnixUS         int64
	Type, Page, Text string
	TraceID          string // hex, on traced requests
}

// HistoryErrorGroup is one rum_error_groups upsert. Timestamps are unix
// seconds; the store only honors FirstSeen on first insert.
type HistoryErrorGroup struct {
	Site, Fingerprint, Type, Message, SampleStack string
	FirstSeen, LastSeen                           int64
}

// HistoryErrorOccurrence is one rum_error_occurrences insert. TS is unix
// seconds.
type HistoryErrorOccurrence struct {
	Site, Fingerprint        string
	TS                       int64
	SessionID, Page, Browser string
}

// SetHistorySink wires the RUM history writer (nil disables persistence,
// e.g. history writing not configured). Call before Ingest starts.
func (a *Aggregator) SetHistorySink(h HistorySink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = h
}
