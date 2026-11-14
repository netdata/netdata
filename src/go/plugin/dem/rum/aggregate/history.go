// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

// HistorySink accepts self-contained investigation events. Event is called
// under the aggregator lock: it must not perform I/O, wait for queue space or
// call back into the aggregator; a short leaf lock, such as a counter update,
// is allowed. Normalization precedes ingestion; persistence runs off-lock.
type HistorySink interface {
	Event(HistoryEvent)
}

// HistoryEvent is the immutable, self-contained investigation record: it carries
// the metadata needed to investigate an event even after earlier journal files
// expire. The history package persists this type as-is. ObservedUS is the Agent
// receipt time, preserved through delayed promotion and persistence; the journal
// assigns saved time separately. Activity events record sessions with no new
// timeline event and are omitted from timeline displays.
type HistoryEvent struct {
	VnodeGUID                                    string
	ExperienceID, View, ViewID, MetricID         string
	Revision                                     uint64
	Site, SessionID                              string
	ObservedUS                                   int64
	Type, Page, Text, TraceID                    string
	Browser, Device, Country, Version, UserID    string
	Fingerprint, ErrorType, Message, SampleStack string
}

func (a *Aggregator) SetHistorySink(h HistorySink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = h
}
