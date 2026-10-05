// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

// HistorySink accepts self-contained investigation events. Event is called
// under the aggregator lock: it must enqueue without I/O or blocking and must
// never call back into the aggregator. Normalization precedes ingestion; persistence runs off-lock.
type HistorySink interface {
	Event(HistoryEvent)
}

// HistoryEvent carries the metadata needed to investigate an event even after
// earlier journal files expire. TSUnixUS is the original observation time;
// the history writer assigns saved time separately. Activity events record
// sessions with no new timeline event and are omitted from timeline displays.
type HistoryEvent struct {
	Site, SessionID                              string
	TSUnixUS                                     int64
	Type, Page, Text, TraceID                    string
	Browser, Device, Country, Version, UserID    string
	Fingerprint, ErrorType, Message, SampleStack string
}

func (a *Aggregator) SetHistorySink(h HistorySink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = h
}
