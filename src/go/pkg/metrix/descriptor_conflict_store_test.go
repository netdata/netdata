// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDescriptorConflictResolution pins the commit-time state model (step 2b-1):
//   - an incompatible registration during a cycle no longer panics; it is deferred.
//   - two incompatible kinds OBSERVED for one name in the same cycle is an
//     unresolvable conflict that fails the whole commit (nothing published).
//   - a committed kind that is NOT observed this cycle is superseded by the newly
//     observed kind, and the old kind's carried-forward series are removed by name.
func TestDescriptorConflictResolution(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"incompatible in-cycle registration is deferred, not panicked": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				_ = s.Write().SnapshotMeter("svc").Gauge("m")
				require.NotPanics(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m")
				})
				// Neither kind was observed, so there is nothing to reconcile.
				require.NoError(t, cc.CommitCycleSuccess())
			},
		},
		"ambiguous multi-kind on a new name is dropped, not failed (other metrics still commit)": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				// svc.m is written as two incompatible kinds with no established authority.
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(2)
				// An unrelated metric in the same cycle must still commit.
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)

				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read()
				_, ok := r.Value("svc.m", nil)
				require.False(t, ok, "ambiguous multi-kind name must be dropped for the cycle")
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"conflict with an established committed kind fails the commit": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: establish svc.m as a gauge.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: the established gauge is actively observed AND a conflicting
				// counter is observed for the same name -> unresolvable, fail the cycle.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(2)
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(3)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "conflicting instrument kinds")

				// Transactional: the failed cycle did not mutate the committed gauge, so
				// declaring svc.m as a counter (outside a cycle) still conflicts.
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m")
				})
			},
		},
		"unobserved committed kind is superseded by a newly observed kind": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: svc.m is a gauge.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: svc.m is written only as a counter (the gauge is not observed).
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(5)
				require.NoError(t, cc.CommitCycleSuccess())

				// svc.m is now a counter; re-declaring it as a gauge conflicts.
				mustValue(t, s.Read(), "svc.m", nil, 5)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("m")
				})
			},
		},
		"supersede removes the old kind's carried-forward series by name": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: gauge svc.m{id=old}.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "old"}).Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())
				mustValue(t, s.Read(), "svc.m", Labels{"id": "old"}, 1)

				// Cycle 2: counter svc.m{id=new} only -> supersede removes ALL svc.m series.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "new"}).Counter("m").ObserveTotal(5)
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read()
				mustValue(t, r, "svc.m", Labels{"id": "new"}, 5)
				_, ok := r.Value("svc.m", Labels{"id": "old"})
				require.False(t, ok, "old gauge series must be removed when the name is superseded")
			},
		},
		"an unobserved in-cycle registration does not overwrite the committed registry": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				// Observe a gauge, and separately DECLARE (never observe) a conflicting counter.
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				_ = s.Write().SnapshotMeter("svc").Counter("m")
				require.NoError(t, cc.CommitCycleSuccess())

				// Only the observed gauge is committed; the unobserved counter registration
				// must NOT be installed. Declaring a counter (outside a cycle) still conflicts.
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m")
				})
			},
		},
		"same-key summary with a different schema is caught, not silently kept": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				// Two summary handles for the SAME name+labels but different quantile schemas,
				// both observed. The second write must not silently apply under the first schema.
				s.Write().SnapshotMeter("svc").Summary("lat", WithSummaryQuantiles(0.5)).
					ObservePoint(SummaryPoint{Count: 1, Sum: 1, Quantiles: []QuantilePoint{{Quantile: 0.5, Value: 1}}})
				s.Write().SnapshotMeter("svc").Summary("lat", WithSummaryQuantiles(0.9)).
					ObservePoint(SummaryPoint{Count: 1, Sum: 1, Quantiles: []QuantilePoint{{Quantile: 0.9, Value: 1}}})
				// An unrelated metric must still commit.
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)

				require.NoError(t, cc.CommitCycleSuccess())
				r := s.Read()
				_, ok := r.Value("svc.lat", nil)
				require.False(t, ok, "conflicting-schema summary must be dropped, not silently committed")
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"histogram nil-bounds vs explicit-bounds grouping is order-independent": {
			run: func(t *testing.T) {
				pt := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 1, CumulativeCount: 1}, {UpperBound: 2, CumulativeCount: 1}}}
				// Repeat: staged-map iteration order is nondeterministic, so a directional
				// wildcard would sometimes wrongly split these into two authorities and drop them.
				for i := range 64 {
					s := NewCollectorStore()
					cc := cycleController(t, s)
					cc.BeginCycle()
					// Same name, different labels: one nil-bounds snapshot histogram, one explicit.
					s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Histogram("h").ObservePoint(pt)
					s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Histogram("h", WithHistogramBounds(1, 2)).ObservePoint(pt)
					require.NoError(t, cc.CommitCycleSuccess())

					r := s.Read(ReadFlatten())
					_, okA := r.Value("svc.h_count", Labels{"id": "a"})
					_, okB := r.Value("svc.h_count", Labels{"id": "b"})
					require.Truef(t, okA && okB, "both histograms must commit regardless of iteration order (i=%d)", i)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestSameKeyEvidenceRetainsDistinctDescriptors pins that same-key reconciliation records EVERY
// distinct authority/declaration, not just the first differing one. The staged entry carries the
// running-canonical merge of compatible writes; an incompatible one is recorded as conflict
// evidence, deduped by FULL descriptor identity (authority fingerprint + declaration fingerprint)
// within its bucket. A distinct descriptor on one key must not be lost, or a real conflict would be
// silently published under the first authority's metadata.
func TestSameKeyEvidenceRetainsDistinctDescriptors(t *testing.T) {
	// stale builds a cached handle for svc.m in its own aborted cycle, so reusing it later
	// bypasses registration's conflict check and exercises the commit-time resolver directly.
	stale := func(t *testing.T, s CollectorStore, cc CycleController, opts ...InstrumentOption) SnapshotGauge {
		t.Helper()
		cc.BeginCycle()
		h := s.Write().SnapshotMeter("svc").Gauge("m", opts...)
		cc.AbortCycle()
		return h
	}

	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"a third same-key declaration conflict is not lost to dedup": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				hUnset := stale(t, s, cc)
				hBytes := stale(t, s, cc, WithUnit("bytes"))
				hSeconds := stale(t, s, cc, WithUnit("seconds"))

				// unset merges, bytes merges into the canonical, seconds conflicts. The seconds
				// declaration must reach the resolver -> unit conflict -> fail the cycle.
				cc.BeginCycle()
				hUnset.Observe(1)
				hBytes.Observe(2)
				hSeconds.Observe(3)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "unit mismatch")

				// Transactional: nothing published for the new name.
				_, ok := s.Read().Value("svc.m", nil)
				require.False(t, ok, "a same-key conflict must not publish a value under the first authority")
			},
		},
		"three complementary same-key declarations all survive the merge": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				hUnset := stale(t, s, cc)
				hBytes := stale(t, s, cc, WithUnit("bytes"))
				hDesc := stale(t, s, cc, WithDescription("d"))

				// All three are compatible; the running canonical must union every field, not
				// keep only the first differing one (which dedup-by-key would have dropped).
				cc.BeginCycle()
				hUnset.Observe(1)
				hBytes.Observe(2)
				hDesc.Observe(3)
				require.NoError(t, cc.CommitCycleSuccess())

				meta, ok := s.Read().MetricMeta("svc.m")
				require.True(t, ok)
				require.Equal(t, "bytes", meta.Unit)
				require.Equal(t, "d", meta.Description)
			},
		},
		"a later same-key mode conflict is not hidden by dedup": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				hUnset := stale(t, s, cc)
				hBytes := stale(t, s, cc, WithUnit("bytes"))
				// A stateful (additive) gauge handle on the SAME key: an incompatible mode.
				cc.BeginCycle()
				hStateful := s.Write().StatefulMeter("svc").Gauge("m")
				cc.AbortCycle()

				cc.BeginCycle()
				hUnset.Observe(1)
				hBytes.Observe(2)
				hStateful.Add(5)
				// Two incompatible modes, no established authority -> ambiguous -> drop the name.
				require.NoError(t, cc.CommitCycleSuccess())
				_, ok := s.Read().Value("svc.m", nil)
				require.False(t, ok, "a mode conflict must drop the name, not publish under one mode")
			},
		},
		"a repeated same-authority histogram conflict records once": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				cc := cycleController(t, s)
				cc.BeginCycle()
				// One nil-bounds snapshot histogram handle: entry bounds [1,2], then the SAME
				// conflicting bounds [5,6] flooded 1000 times. All 1000 share one authority
				// (fingerprint), so the conflict is recorded once and later repeats are dropped by a
				// no-clone identity check (evidence stays O(1)).
				h := s.Write().SnapshotMeter("svc").Histogram("h")
				h.ObservePoint(HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 1, CumulativeCount: 1}, {UpperBound: 2, CumulativeCount: 1}}})
				for range 1000 {
					h.ObservePoint(HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 5, CumulativeCount: 1}, {UpperBound: 6, CumulativeCount: 1}}})
				}
				require.Len(t, sv.core.active.conflicts, 1, "a repeated same-authority conflict must record once")
				// The name is unresolvable (two effective schemas) -> dropped, cycle commits.
				require.NoError(t, cc.CommitCycleSuccess())
				_, ok := s.Read(ReadFlatten()).Value("svc.h_count", nil)
				require.False(t, ok)
			},
		},
		"many distinct same-key handles sharing an authority record one conflict": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				cc := cycleController(t, s)
				// Distinct stale handles (each from its own aborted cycle) for one key: a unit=bytes
				// gauge entry plus several unit=seconds gauges (all the gauge authority, conflicting
				// declaration). All conflicts share one authority fingerprint, so the first records
				// and the rest dedup -> evidence stays O(1), not O(handles).
				entry := stale(t, s, cc, WithUnit("bytes"))
				var dups []SnapshotGauge
				for range 8 {
					dups = append(dups, stale(t, s, cc, WithUnit("seconds")))
				}
				cc.BeginCycle()
				entry.Observe(1)
				for _, h := range dups {
					h.Observe(2)
				}
				require.Len(t, sv.core.active.conflicts, 1, "distinct handles sharing an authority must record one conflict")
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "unit mismatch")
			},
		},
		"histogram: committed bounds observed as a same-key conflict still fails loud": {
			run: func(t *testing.T) {
				histPoint := func(uppers ...float64) HistogramPoint {
					b := make([]BucketPoint, len(uppers))
					for i, ub := range uppers {
						b[i] = BucketPoint{UpperBound: ub, CumulativeCount: 1}
					}
					return HistogramPoint{Count: 1, Sum: 1, Buckets: b}
				}
				s := NewCollectorStore()
				cc := cycleController(t, s)
				// Cycle 1: establish svc.h with bounds [7,8].
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(histPoint(7, 8))
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: one handle observes [1,2] (entry), [5,6] (a distinct authority -> conflict),
				// then the committed [7,8] (another distinct authority -> its own conflict). Because
				// each distinct authority is recorded (not collapsed), the committed-matching [7,8]
				// reaches the resolver, so the cycle FAILs loud rather than silently DROPping.
				cc.BeginCycle()
				h := s.Write().SnapshotMeter("svc").Histogram("h")
				h.ObservePoint(histPoint(1, 2))
				h.ObservePoint(histPoint(5, 6))
				h.ObservePoint(histPoint(7, 8))
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "conflicting instrument kinds")
			},
		},
		"declaration conflict within a conflict authority fails deterministically across orders": {
			run: func(t *testing.T) {
				// Same key via stale handles (bypass registration): a snapshot gauge, plus two
				// STATEFUL gauges sharing the stateful-gauge authority but declaring conflicting
				// units (bytes vs seconds). Whichever authority becomes the staged entry, the
				// bytes-vs-seconds declaration conflict WITHIN the stateful authority must be recorded
				// and caught -> FAIL every time, not an order-dependent DROP.
				writes := []func(snap SnapshotGauge, b, sc StatefulGauge){
					func(snap SnapshotGauge, b, sc StatefulGauge) { snap.Observe(1); b.Add(2); sc.Add(3) },
					func(snap SnapshotGauge, b, sc StatefulGauge) { b.Add(2); snap.Observe(1); sc.Add(3) },
					func(snap SnapshotGauge, b, sc StatefulGauge) { sc.Add(3); b.Add(2); snap.Observe(1) },
				}
				for i, w := range writes {
					s := NewCollectorStore()
					cc := cycleController(t, s)
					cc.BeginCycle()
					snap := s.Write().SnapshotMeter("svc").Gauge("m")
					cc.AbortCycle()
					cc.BeginCycle()
					b := s.Write().StatefulMeter("svc").Gauge("m", WithUnit("bytes"))
					cc.AbortCycle()
					cc.BeginCycle()
					sc := s.Write().StatefulMeter("svc").Gauge("m", WithUnit("seconds"))
					cc.AbortCycle()

					cc.BeginCycle()
					w(snap, b, sc)
					err := cc.CommitCycleSuccess()
					require.Errorf(t, err, "order %d", i)
					require.ErrorContainsf(t, err, "unit mismatch", "order %d", i)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
