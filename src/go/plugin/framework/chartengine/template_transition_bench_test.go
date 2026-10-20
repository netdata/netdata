// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// Measure preparation against one committed inventory. Abort keeps that starting
// inventory fixed, so every iteration exercises the named transition. Snapshots
// are compiled outside timing. The current case includes existing planner work;
// compare changes against it to see the additional reconciliation/cache cost.
func BenchmarkTemplateSetTransition(b *testing.B) {
	const dimensions = 10
	for _, entryCount := range []int{10, 100} {
		b.Run(fmt.Sprintf("entries_%d", entryCount), func(b *testing.B) {
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			if !ok {
				b.Fatal("store does not expose cycle control")
			}
			cycle := managed.CycleController()
			cycle.BeginCycle()
			entries := make([]TemplateEntry, 0, entryCount+1)
			meter := store.Write().SnapshotMeter("")
			for i := 0; i <= entryCount; i++ {
				id := fmt.Sprintf("metric_%d", i)
				entry := nativeEntry(id, id, id)
				dim := &entry.Groups[0].Charts[0].Dimensions[0]
				dim.Name = ""
				dim.NameFromLabel = "id"
				entries = append(entries, entry)
				for j := range dimensions {
					meter.Gauge(id).Observe(metrix.SampleValue(j), meter.LabelSet(metrix.Label{
						Key:   "id",
						Value: strconv.Itoa(j),
					}))
				}
			}
			if err := cycle.CommitCycleSuccess(); err != nil {
				b.Fatal(err)
			}
			prepare := func(entries []TemplateEntry) *TemplateSet {
				set, err := NewTemplateSet(TemplateSetSpec{
					Entries: entries,
				})
				if err != nil {
					b.Fatal(err)
				}
				return set
			}
			current := prepare(entries[:entryCount])
			replaced := current.Entries()
			replaced[entryCount-1].Groups[0].Charts[0].ID = "replacement"
			for _, tc := range []struct {
				name                     string
				set                      *TemplateSet
				charts, creates, removes int
			}{
				{name: "current", set: current, charts: entryCount},
				{name: "add", set: prepare(entries), charts: entryCount + 1, creates: 1},
				{name: "remove", set: prepare(entries[:entryCount-1]), charts: entryCount - 1, removes: 1},
				{name: "replace", set: prepare(replaced), charts: entryCount, creates: 1, removes: 1},
			} {
				b.Run(tc.name, func(b *testing.B) {
					raw := store.Read(metrix.ReadRaw())
					reader := &benchmarkSequenceReader{
						Reader: raw,
						raw:    raw.(metrix.SeriesIdentityRawIterator),
						seq:    1,
					}
					engine, err := New(WithRuntimeStore(nil))
					if err != nil {
						b.Fatal(err)
					}
					warm, err := engine.PreparePlanWithOptions(reader, PlanOptions{
						TemplateSet: current,
					})
					if err != nil {
						b.Fatal(err)
					}
					if err := warm.Commit(); err != nil {
						b.Fatal(err)
					}
					var plan Plan
					b.ReportAllocs()
					b.ResetTimer()
					for i := range b.N {
						reader.seq = uint64(i) + 2
						attempt, err := engine.PreparePlanWithOptions(reader, PlanOptions{
							TemplateSet: tc.set,
						})
						if err != nil {
							b.Fatal(err)
						}
						plan = attempt.Plan()
						attempt.Abort()
					}
					b.StopTimer()
					charts, creates, removes, values := 0, 0, 0, 0
					for _, action := range plan.Actions {
						switch a := action.(type) {
						case CreateChartAction:
							creates++
						case RemoveChartAction:
							removes++
						case UpdateChartAction:
							charts++
							values += len(a.Values)
						}
					}
					if charts != tc.charts || creates != tc.creates || removes != tc.removes || values != tc.charts*dimensions {
						b.Fatalf("unexpected plan: charts=%d creates=%d removes=%d values=%d", charts, creates, removes, values)
					}
				})
			}
		})
	}
}
