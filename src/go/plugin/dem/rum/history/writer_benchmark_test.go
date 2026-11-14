// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
)

type discardEventWriter struct{}

func (discardEventWriter) AppendEvent(context.Context, EventRecord) (bool, error) { return true, nil }
func (discardEventWriter) Sync(context.Context) error                             { return nil }

type discardCounters struct{}

func (discardCounters) Add(string, uint64) {}

// Measures the writer's per-batch work without journal I/O: admission checks,
// accounting and one batch sync per full batch. Cost is O(batch) with no
// allocation. Timings are local trends, not CI gates.
func BenchmarkWriterFlush(b *testing.B) {
	w := NewWriter(nil, discardCounters{})
	w.st = discardEventWriter{}
	batch := make([]aggregate.HistoryEvent, maxBatch)
	for i := range batch {
		batch[i] = aggregate.HistoryEvent{
			VnodeGUID:   "11111111-1111-4111-8111-111111111111",
			Revision:    1,
			Site:        "shop",
			SessionID:   "session",
			ObservedUS:  1_700_000_000_000_000 + int64(i),
			Type:        "error",
			Page:        "/checkout",
			Browser:     "Firefox",
			Country:     "GR",
			Device:      "desktop",
			Version:     "release-1",
			UserID:      "user-id",
			Fingerprint: "fingerprint",
			ErrorType:   "TypeError",
			Message:     "request failed",
			SampleStack: "app.js:10",
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if n := w.flush(ctx, batch); n != len(batch) {
			b.Fatalf("flushed %d of %d", n, len(batch))
		}
	}
}
