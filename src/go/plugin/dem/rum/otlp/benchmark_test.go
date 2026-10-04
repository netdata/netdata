// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

import (
	"testing"
	"time"
)

type benchmarkCounters struct{}

func (benchmarkCounters) Add(string, uint64) {}

// Ingestion is O(records + spans + attributes), independent of other sites.
// This exercises a continuing session with a pageview and browser trace while
// draining its queues locally; allocations reflect per-beacon wire construction.
func BenchmarkIngest(b *testing.B) {
	e := &Exporter{
		siteName: "s1",
		counters: benchmarkCounters{},
		sessions: newSessionTracker(),
		now:      func() time.Time { return t0 },
		ch:       make(chan queued, queueCap),
		spanCh:   make(chan spanItem, spanQueueCap),
	}
	event := tracedBeacon()
	event.PageView = true
	e.Ingest(event)
	for len(e.ch) != 0 {
		<-e.ch
	}
	<-e.spanCh
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		e.Ingest(event)
		<-e.ch
		<-e.spanCh
	}
}
