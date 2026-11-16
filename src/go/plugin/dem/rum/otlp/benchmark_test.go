// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

type benchmarkCounters struct{}

func (benchmarkCounters) Add(string, uint64) {}

// Ingestion is O(records + spans + attributes), independent of other sites.
// This exercises a continuing session with a pageview and browser trace while
// draining its queues locally; allocations reflect per-beacon wire construction.
func BenchmarkIngest(b *testing.B) {
	shared := &transport{
		siteName: "s1",
		counters: benchmarkCounters{},
	}
	e := &Logs{
		transport: shared,
		now:       func() time.Time { return t0 },
		ch:        make(chan *logspb.LogRecord, queueCap),
	}
	traces := &Traces{
		transport: shared,
		spanCh:    make(chan spanItem, spanQueueCap),
	}
	event := tracedBeacon()
	traces.Ingest(aggregate.Result{
		Observation:  event,
		Investigated: true,
	})
	e.Ingest(aggregate.Result{
		Observation:  event,
		Investigated: true,
		PageView:     true,
	})
	for len(e.ch) != 0 {
		<-e.ch
	}
	<-traces.spanCh
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		traces.Ingest(aggregate.Result{
			Observation:  event,
			Investigated: true,
		})
		e.Ingest(aggregate.Result{
			Observation:  event,
			Investigated: true,
			PageView:     true,
		})
		<-e.ch
		<-traces.spanCh
	}
}

// Normalized console records have optional error context; queue drainage keeps
// the measurement independent of receiver availability and queue capacity.
func BenchmarkConsoleIngest(b *testing.B) {
	for name, log := range map[string]beacon.Log{
		"plain": {Level: "info", Message: "checkout opened"},
		"error": {Level: "error", Message: "payment failed", ErrorType: "TypeError", Stack: "checkout (app.js:12:3)"},
	} {
		b.Run(name, func(b *testing.B) {
			e := &Logs{
				transport: &transport{
					siteName: "s1",
					counters: benchmarkCounters{},
				},
				now: func() time.Time { return t0 },
				ch:  make(chan *logspb.LogRecord, queueCap),
			}
			event := mkBeacon()
			event.Logs = []beacon.Log{log}
			result := aggregate.Result{
				Observation:  event,
				Investigated: true,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				e.Ingest(result)
				<-e.ch
			}
		})
	}
}
