// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
)

// No earlier Go StatsD implementation exists for a before/after baseline.
// These benchmarks cover the new component's input and actual native output;
// ns/op is a development-machine trend, never a CI timing gate.
func BenchmarkIngest(b *testing.B) {
	for name, line := range map[string]string{
		"counter":   "requests:2|c|@.5|#route:/api,region:eu",
		"gauge":     "connections:5|g|#pool:main",
		"timer":     "latency:15|ms|@.3|#route:/api",
		"histogram": "difference:-10|h|#queue:work",
		"set":       "members:user-125|s|#pool:main",
	} {
		b.Run(name, func(b *testing.B) {
			f := newCoreFixture(b, 1000, 5*time.Minute)
			f.ingest(b, line)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := f.c.receiver.ingest(line, f.time); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func mixedRecords(n int) []string {
	lines := make([]string, n)
	for i := range n {
		payload := []string{"2|c|@.5", "10|g", "15|ms|@.3", "-10|h", "member-a|s"}[i%5]
		lines[i] = fmt.Sprintf("metric%d:%s|#region:eu,pool:main", i, payload)
	}
	return lines
}

// Stable normal mix: one counter/gauge/timer/histogram/set per five identities.
// Parsing/update is O(record bytes + label sort + bounded estimator update).
// Publication is O(live identities + estimator bins log bins + native output).
func BenchmarkMixedPublication(b *testing.B) {
	for _, size := range []int{100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			f := newCoreFixture(b, size, 5*time.Minute)
			lines := mixedRecords(size)
			for range 3 {
				f.ingest(b, lines...)
				f.collect(b, false, false)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, line := range lines {
					if err := f.c.receiver.ingest(line, f.time); err != nil {
						b.Fatal(err)
					}
				}
				f.collect(b, false, false)
			}
		})
	}
}

func BenchmarkCapacityPressure(b *testing.B) {
	f := newCoreFixture(b, 1000, 5*time.Minute)
	f.ingest(b, mixedRecords(1000)...)
	f.collect(b, false, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.c.receiver.ingest("additional:1|c", f.time); err != rejectCapacity {
			b.Fatal(err)
		}
	}
}

// Run with -benchtime=1x. Measures retained Go heap, not process RSS or a peak
// allocation guarantee. Both receiving and detached windows remain reachable.
func BenchmarkOwnershipEnvelope(b *testing.B) {
	for _, shape := range []string{"mixed1000", "signed1000"} {
		b.Run(shape, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f := newCoreFixture(b, 1000, 5*time.Minute)
				lines := mixedRecords(1000)
				feed := func() { f.ingest(b, lines...) }
				if shape == "signed1000" {
					// Saturated sign stores are a capacity envelope, not a typical workload.
					feed = func() {
						for series := 0; series < 1000; series++ {
							for bin := 0; bin < 1024; bin++ {
								v := math.Exp(float64(bin) * .018001) // Below 1024 occupied log bins at .009 mapping.
								f.ingest(
									b,
									fmt.Sprintf("metric%d:%g|h", series, v),
									fmt.Sprintf("metric%d:%g|h", series, -v),
								)
							}
						}
					}
				}
				runtime.GC()
				var before, windows, published runtime.MemStats
				runtime.ReadMemStats(&before)
				feed()
				f.cycle.BeginCycle()
				batch, err := f.c.receiver.cut(f.time)
				if err != nil {
					b.Fatal(err)
				}
				feed()
				runtime.GC()
				runtime.ReadMemStats(&windows)
				for _, m := range batch {
					f.c.writeMeasurement(m)
				}
				f.c.receiver.release(batch)
				f.finish(b, false, false)
				runtime.GC()
				runtime.ReadMemStats(&published)
				b.ReportMetric(float64(windows.HeapAlloc-before.HeapAlloc)/1e6, "two_windows_MB")
				b.ReportMetric(float64(published.HeapAlloc-before.HeapAlloc)/1e6, "with_native_MB")
				runtime.KeepAlive(f)
				runtime.KeepAlive(batch)
			}
		})
	}
}
