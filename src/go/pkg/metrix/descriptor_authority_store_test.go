// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuthorityFingerprintSignedZero pins that the authority fingerprint matches series-authority
// equality for signed zero: -0.0 and +0.0 compare equal, so equal authorities must share a
// fingerprint (else a semantic authority over-records as two conflict buckets).
func TestAuthorityFingerprintSignedZero(t *testing.T) {
	negZero := math.Copysign(0, -1)
	a := &instrumentDescriptor{kind: kindHistogram, histogram: &histogramSchema{bounds: []float64{0, 1}}}
	b := &instrumentDescriptor{kind: kindHistogram, histogram: &histogramSchema{bounds: []float64{negZero, 1}}}
	require.True(t, descriptorSeriesAuthoritiesEqual(a, b), "+0.0 and -0.0 bounds are authority-equal")
	require.Equal(t, authorityFingerprintOf(a), authorityFingerprintOf(b), "equal authorities must share a fingerprint")
}

// TestBaselineSeriesAuthority pins that accumulating writes (stateful Add and
// cumulative windows) never seed their baseline from a series whose descriptor is
// series-authority-incompatible - e.g. a name whose kind was just superseded. A
// single helper (baselineSeriesForWrite) guards every additive baseline read, so
// gauge and measureset (which previously read any kind) and the kind-checked
// counter/histogram/summary paths all share the same authority check.
func TestBaselineSeriesAuthority(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"gauge superseding a counter does not inherit the counter's value": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: svc.m is a snapshot counter at 100.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(100)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: svc.m is written only as a stateful gauge Add(5). The counter is
				// superseded, so the gauge must start fresh (5), not from the counter (105).
				cc.BeginCycle()
				s.Write().StatefulMeter("svc").Gauge("m").Add(5)
				require.NoError(t, cc.CommitCycleSuccess())

				mustValue(t, s.Read(), "svc.m", nil, 5)
			},
		},
		"measureset-counter superseding a measureset-gauge does not inherit the gauge's value": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: svc.lat is a stateful measureset gauge, field value = 100.
				cc.BeginCycle()
				s.Write().StatefulMeter("svc").MeasureSetGauge("lat",
					WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
				).AddField("value", 100)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: svc.lat is written only as a measureset COUNTER (incompatible
				// semantics). It must start fresh (5), not from the superseded gauge (105).
				cc.BeginCycle()
				s.Write().StatefulMeter("svc").MeasureSetCounter("lat",
					WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
				).AddField("value", 5)
				require.NoError(t, cc.CommitCycleSuccess())

				mustValue(t, s.Read(ReadFlatten()), "svc.lat_value", measureSetFieldLabels("value"), 5)
			},
		},
		"compatible stateful gauge still carries its baseline across cycles": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				cc.BeginCycle()
				s.Write().StatefulMeter("svc").Gauge("m").Add(10)
				require.NoError(t, cc.CommitCycleSuccess())
				mustValue(t, s.Read(), "svc.m", nil, 10)

				// Same authority next cycle -> baseline carries (10 + 5 = 15).
				cc.BeginCycle()
				s.Write().StatefulMeter("svc").Gauge("m").Add(5)
				require.NoError(t, cc.CommitCycleSuccess())
				mustValue(t, s.Read(), "svc.m", nil, 15)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestSameKeyAuthorityConflict pins that two writes to the SAME series key with
// incompatible authorities (e.g. snapshot vs stateful mode) no longer collapse into
// one staged entry: the second write records a conflict so commit-time resolution
// drops the name. Previously only summary/stateset/measureset guarded this; the guard
// now covers gauge, counter, and histogram too.
func TestSameKeyAuthorityConflict(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"gauge snapshot vs stateful on one key is dropped": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				s.Write().StatefulMeter("svc").Gauge("m").Add(2) // same key, incompatible mode
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)

				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read()
				mustNoValue(t, r, "svc.m", nil)
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"counter snapshot vs stateful on one key is dropped": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(1)
				s.Write().StatefulMeter("svc").Counter("m").Add(2) // same key, incompatible mode
				s.Write().SnapshotMeter("svc").Counter("ok").ObserveTotal(9)

				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read()
				mustNoValue(t, r, "svc.m", nil)
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestHistogramEffectiveBoundsAuthority pins that histogram authority is resolved by
// the bounds actually OBSERVED this cycle, not by the descriptor alone. A nil-bounds
// snapshot histogram is a wildcard descriptor, so comparing descriptors would let
// genuinely different bucket schemas collapse (silent mixed schemas) or drift-panic at
// commit. Same-observed-bounds writes still dedup; different observed bounds drop.
func TestHistogramEffectiveBoundsAuthority(t *testing.T) {
	pt := func(uppers ...float64) HistogramPoint {
		buckets := make([]BucketPoint, len(uppers))
		for i, ub := range uppers {
			buckets[i] = BucketPoint{UpperBound: ub, CumulativeCount: 1}
		}
		return HistogramPoint{Count: 1, Sum: 1, Buckets: buckets}
	}

	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"nil-bounds and explicit-bounds with different observed bounds are dropped": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).
					Histogram("h").ObservePoint(pt(1, 2))
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).
					Histogram("h", WithHistogramBounds(5, 6)).ObservePoint(pt(5, 6))
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)

				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read(ReadFlatten())
				mustNoValue(t, r, "svc.h_count", Labels{"id": "a"})
				mustNoValue(t, r, "svc.h_count", Labels{"id": "b"})
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"two nil-bounds with different observed bounds are dropped, not panicked": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				require.NotPanics(t, func() {
					s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).
						Histogram("h").ObservePoint(pt(1, 2))
					s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).
						Histogram("h").ObservePoint(pt(5, 6))
				})
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read(ReadFlatten())
				mustNoValue(t, r, "svc.h_count", Labels{"id": "a"})
				mustNoValue(t, r, "svc.h_count", Labels{"id": "b"})
			},
		},
		"nil-bounds and explicit-bounds with the same observed bounds both commit": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).
					Histogram("h").ObservePoint(pt(1, 2))
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).
					Histogram("h", WithHistogramBounds(1, 2)).ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read(ReadFlatten())
				mustValue(t, r, "svc.h_count", Labels{"id": "a"}, 1)
				mustValue(t, r, "svc.h_count", Labels{"id": "b"}, 1)
			},
		},
		"same-key histogram with different observed bounds is dropped, not panicked": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				h := s.Write().SnapshotMeter("svc").Histogram("h")
				require.NotPanics(t, func() {
					h.ObservePoint(pt(1, 2))
					h.ObservePoint(pt(5, 6)) // same key, different observed bounds
				})
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read(ReadFlatten())
				mustNoValue(t, r, "svc.h_count", nil)
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"nil-bounds histogram observing different bounds next cycle supersedes without panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())

				// The committed registry now holds explicit bounds (installed from the
				// published series). A next-cycle nil-bounds handle observing different
				// bounds must not panic; the captured schema is superseded at commit.
				newPoint := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 5, CumulativeCount: 1}, {UpperBound: 6, CumulativeCount: 1}}}
				cc.BeginCycle()
				require.NotPanics(t, func() {
					s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(newPoint)
				})
				require.NoError(t, cc.CommitCycleSuccess())
				mustHistogram(t, s.Read(), "svc.h", nil, newPoint)
			},
		},
		"nil-bounds histogram observing the same bounds next cycle is stable": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())

				// Same effective bounds must NOT false-supersede (the P1-a regression):
				// the value updates in place across cycles.
				samePoint := HistogramPoint{Count: 5, Sum: 5, Buckets: []BucketPoint{{UpperBound: 1, CumulativeCount: 5}, {UpperBound: 2, CumulativeCount: 5}}}
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(samePoint)
				require.NoError(t, cc.CommitCycleSuccess())
				mustHistogram(t, s.Read(), "svc.h", nil, samePoint)
			},
		},
		"nil-then-explicit registration with the same bounds both commit": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				// Register nil-bounds FIRST, then explicit (reverse order), same bounds.
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Histogram("h").ObservePoint(pt(1, 2))
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Histogram("h", WithHistogramBounds(1, 2)).ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())
				r := s.Read(ReadFlatten())
				mustValue(t, r, "svc.h_count", Labels{"id": "a"}, 1)
				mustValue(t, r, "svc.h_count", Labels{"id": "b"}, 1)
			},
		},
		"init nil then explicit-committed then nil different bounds supersedes without panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				// Init-register nil-bounds (no active cycle): instruments[name] is nil-bounds.
				_ = s.Write().SnapshotMeter("svc").Histogram("h")

				// Cycle 1: commit EXPLICIT bounds [1,2]. Explicit series capture no persistent
				// schema (the round-4 divergence); install-from-accepted converges the registry.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("h", WithHistogramBounds(1, 2)).ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: a nil-bounds handle observing different bounds must not panic; the
				// converged committed authority supersedes.
				newPoint := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 5, CumulativeCount: 1}, {UpperBound: 6, CumulativeCount: 1}}}
				cc.BeginCycle()
				require.NotPanics(t, func() {
					s.Write().SnapshotMeter("svc").Histogram("h").ObservePoint(newPoint)
				})
				require.NoError(t, cc.CommitCycleSuccess())
				mustHistogram(t, s.Read(), "svc.h", nil, newPoint)
			},
		},
		"evicted nil-bounds histogram leaves no orphaned schema": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{maxSeries: 1}
				cc := cycleController(t, s)

				// Cycle 1: nil-bounds histogram svc.a[1,2] + gauge svc.z. maxSeries=1 evicts
				// the lower-keyed series (svc.a) after its bounds were captured this cycle.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Histogram("a").ObservePoint(pt(1, 2))
				s.Write().SnapshotMeter("svc").Gauge("z").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: svc.a with DIFFERENT bounds must not panic against a stale orphaned
				// schema - per-cycle schema means no persistent orphan can exist.
				other := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 5, CumulativeCount: 1}, {UpperBound: 6, CumulativeCount: 1}}}
				cc.BeginCycle()
				require.NotPanics(t, func() {
					s.Write().SnapshotMeter("svc").Histogram("a").ObservePoint(other)
				})
				require.NoError(t, cc.CommitCycleSuccess())
			},
		},
		"init nil-bounds histogram with conflicting first observations drops only that name": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				// Init-register nil-bounds svc.h (no active cycle): a wildcard registration
				// with no live series - NOT an established authority.
				_ = s.Write().SnapshotMeter("svc").Histogram("h")

				// First observed cycle: two conflicting bounds for svc.h + an unrelated gauge.
				// With no live committed authority, svc.h is ambiguous and must be DROPPED,
				// not fail the whole cycle - the unrelated gauge still commits.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Histogram("h").ObservePoint(pt(1, 2))
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Histogram("h").ObservePoint(HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 5, CumulativeCount: 1}, {UpperBound: 6, CumulativeCount: 1}}})
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read(ReadFlatten())
				mustNoValue(t, r, "svc.h_count", Labels{"id": "a"})
				mustNoValue(t, r, "svc.h_count", Labels{"id": "b"})
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"init nil-bounds histogram declaration survives a stale-handle first observation": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				cc := cycleController(t, s)
				// Stale nil-bounds handle with NO unit, from an aborted cycle (before init reg).
				cc.BeginCycle()
				stale := s.Write().SnapshotMeter("svc").Histogram("h")
				cc.AbortCycle()

				// Init-register svc.h nil-bounds WITH unit=bytes (no active cycle).
				_ = s.Write().SnapshotMeter("svc").Histogram("h", WithUnit("bytes"))

				// First observation uses the stale NO-unit handle. The committed declaration
				// (unit=bytes) is a wildcard registration but its metadata must be preserved.
				cc.BeginCycle()
				stale.ObservePoint(pt(1, 2))
				require.NoError(t, cc.CommitCycleSuccess())

				require.NotNil(t, sv.core.instruments["svc.h"])
				require.Equal(t, "bytes", sv.core.instruments["svc.h"].meta.Unit, "committed nil-bounds declaration must be preserved")
				for _, series := range sv.core.snapshot.Load().series {
					if series.name == "svc.h" {
						require.Equal(t, "bytes", series.desc.meta.Unit)
					}
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestCanonicalDescriptorMetadata pins that commit publishes a single CANONICAL
// descriptor per name: compatible-but-divergent metadata (a cached handle that sets
// fewer fields, or complementary fields across labels) is reconciled deterministically
// so every series - and the reader - sees the same metadata, independent of label/order.
func TestCanonicalDescriptorMetadata(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"cached unset-metadata handle does not erase committed metadata on a new label": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// A cached no-unit handle (label b) from an aborted cycle, before any commit.
				cc.BeginCycle()
				stale := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m")
				cc.AbortCycle()

				// Commit svc.m{id=a} with unit=bytes.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes")).Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// The stale no-unit handle observes its new label. Canonicalization must
				// publish the committed (bytes) metadata for it, not empty metadata.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes")).Observe(2)
				stale.Observe(3)
				require.NoError(t, cc.CommitCycleSuccess())

				meta, ok := s.Read().MetricMeta("svc.m")
				require.True(t, ok)
				require.Equal(t, "bytes", meta.Unit, "metadata must be canonical across labels")
			},
		},
		"new name merges explicit and unset handles deterministically": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// A stale explicit (unit=bytes) handle for a NEW name, from an aborted cycle.
				cc.BeginCycle()
				explicit := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes"))
				cc.AbortCycle()

				// A fresh unset handle + the stale explicit handle both observe the new name
				// in one cycle. Canonicalization is order-independent: unit=bytes wins
				// (unset never conflicts).
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m").Observe(2)
				explicit.Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				meta, ok := s.Read().MetricMeta("svc.m")
				require.True(t, ok)
				require.Equal(t, "bytes", meta.Unit)
			},
		},
		"complementary metadata across labels is unioned": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Two stale handles for a NEW name: one sets unit, the other description.
				cc.BeginCycle()
				hUnit := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes"))
				cc.AbortCycle()
				cc.BeginCycle()
				hDesc := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m", WithDescription("d"))
				cc.AbortCycle()

				cc.BeginCycle()
				hUnit.Observe(1)
				hDesc.Observe(2)
				require.NoError(t, cc.CommitCycleSuccess())

				meta, ok := s.Read().MetricMeta("svc.m")
				require.True(t, ok)
				require.Equal(t, "bytes", meta.Unit)
				require.Equal(t, "d", meta.Description)
			},
		},
		"carried unobserved series of an observed name is canonicalized": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				cc := cycleController(t, s)
				// A cached bytes-unit handle (label b) from an aborted cycle, before any commit.
				cc.BeginCycle()
				bytesHandle := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m", WithUnit("bytes"))
				cc.AbortCycle()

				// Cycle 1: commit svc.m{id=a} and svc.m{id=b}, both WITHOUT a unit.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m").Observe(1)
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m").Observe(2)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: only id=b is observed (unit=bytes). The canonical becomes bytes;
				// the carried, unobserved id=a series must be canonicalized to bytes too, so
				// every series and the registry agree (not just the observed one).
				cc.BeginCycle()
				bytesHandle.Observe(3)
				require.NoError(t, cc.CommitCycleSuccess())

				for _, series := range sv.core.snapshot.Load().series {
					if series.name == "svc.m" {
						require.Equal(t, "bytes", series.desc.meta.Unit, "every svc.m series must carry the canonical unit")
					}
				}
				require.Equal(t, "bytes", sv.core.instruments["svc.m"].meta.Unit)
			},
		},
		"same-key complementary metadata is unioned regardless of write order": {
			run: func(t *testing.T) {
				for _, reversed := range []bool{false, true} {
					s := NewCollectorStore()
					cc := cycleController(t, s)
					// Two cached handles for the SAME key (no labels): complementary metadata.
					cc.BeginCycle()
					hUnit := s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("bytes"))
					cc.AbortCycle()
					cc.BeginCycle()
					hDesc := s.Write().SnapshotMeter("svc").Gauge("m", WithDescription("d"))
					cc.AbortCycle()

					cc.BeginCycle()
					if reversed {
						hDesc.Observe(2)
						hUnit.Observe(1)
					} else {
						hUnit.Observe(1)
						hDesc.Observe(2)
					}
					require.NoError(t, cc.CommitCycleSuccess())

					meta, ok := s.Read().MetricMeta("svc.m")
					require.Truef(t, ok, "reversed=%v", reversed)
					require.Equalf(t, "bytes", meta.Unit, "reversed=%v", reversed)
					require.Equalf(t, "d", meta.Description, "reversed=%v", reversed)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestCommitTimeDeclarationConflict pins that a declaration (metadata) conflict between
// series-authority-compatible descriptors is caught at commit even when a cached handle
// bypasses registration - a reused handle from an aborted cycle carries a stale unit
// that conflicts with the committed declaration, so the cycle must fail transactionally.
func TestCommitTimeDeclarationConflict(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"cached handle with a conflicting unit fails the commit": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// A gauge registered with unit=seconds in a cycle that aborts: the handle
				// survives, but the registration is discarded (never committed).
				cc.BeginCycle()
				stale := s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("seconds"))
				cc.AbortCycle()

				// Commit the same name with unit=bytes.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("bytes")).Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Reusing the stale handle bypasses registration (no re-check), so the
				// unit conflict must be caught at commit -> the cycle fails, transactionally.
				cc.BeginCycle()
				stale.Observe(2)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "unit mismatch")

				// The failed cycle did not corrupt the committed gauge: it can still be
				// observed (with the committed unit) and read back in a fresh cycle.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("bytes")).Observe(7)
				require.NoError(t, cc.CommitCycleSuccess())
				mustValue(t, s.Read(), "svc.m", nil, 7)
			},
		},
		"same-key stale metadata conflict fails the commit": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// A stale unit=seconds handle, created before any commit (so registration
				// does not yet conflict).
				cc.BeginCycle()
				stale := s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("seconds"))
				cc.AbortCycle()

				// Commit svc.m with unit=bytes.
				cc.BeginCycle()
				valid := s.Write().SnapshotMeter("svc").Gauge("m", WithUnit("bytes"))
				valid.Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Same cycle, SAME key: valid (bytes) then stale (seconds). The stale write
				// must be recorded as a conflict (not silently applied under bytes metadata),
				// failing the cycle.
				cc.BeginCycle()
				valid.Observe(2)
				stale.Observe(3)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "unit mismatch")
			},
		},
		"different-label explicit and unset metadata both commit (preserve-first)": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				// Same name, different labels: one sets unit, the other leaves it unset.
				// Preserve-first makes this compatible - it must NOT be wrongly failed.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes")).Observe(1)
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m").Observe(2)
				require.NoError(t, cc.CommitCycleSuccess())
				r := s.Read()
				mustValue(t, r, "svc.m", Labels{"id": "a"}, 1)
				mustValue(t, r, "svc.m", Labels{"id": "b"}, 2)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestPendingMetadataConflict pins that a metadata (declaration) conflict between two
// series-authority-compatible registrations stays fail-loud even when the committed
// authority for that name is incompatible - the per-name pending authority list is
// consulted so the second declaration is compared against the first.
func TestPendingMetadataConflict(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"conflicting units fail even under an incompatible committed authority": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Establish an incompatible committed authority (gauge) for svc.m.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Two counter declarations share an authority; their unit conflict is a bug.
				cc.BeginCycle()
				_ = s.Write().SnapshotMeter("svc").Counter("m", WithUnit("bytes"))
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m", WithUnit("seconds"))
				})
			},
		},
		"matching units dedup even under an incompatible committed authority": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				require.NotPanics(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m", WithUnit("bytes"))
					_ = s.Write().SnapshotMeter("svc").Counter("m", WithUnit("bytes"))
				})
				cc.AbortCycle()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestInstallDescriptorAfterRetention pins that committed descriptors are installed
// only for series that SURVIVE retention: a series evicted in the same cycle it is
// first observed must not leave an orphan descriptor behind.
func TestInstallDescriptorAfterRetention(t *testing.T) {
	s := NewCollectorStore()
	sv := collectorStoreViewForTest(t, s)
	sv.core.retention = collectorRetentionPolicy{maxSeries: 1}
	cc := cycleController(t, s)

	cc.BeginCycle()
	s.Write().SnapshotMeter("svc").Gauge("a").Observe(1)
	s.Write().SnapshotMeter("svc").Gauge("b").Observe(2)
	require.NoError(t, cc.CommitCycleSuccess())

	series := sv.core.snapshot.Load().series
	require.Len(t, series, 1, "maxSeries=1 must evict one of the two new series")
	require.Len(t, sv.core.instruments, 1, "an evicted series must not leave an orphan descriptor")
	for _, srv := range series {
		_, ok := sv.core.instruments[srv.name]
		require.True(t, ok, "the surviving series must have its descriptor installed")
	}
}

// TestCommitCostGuards pins the commit-cost envelope for the canonicalization work:
// repeated same-handle writes must not accumulate per-write descriptor evidence, and a
// sparse commit over a large retained name must not clone every retained series.
func TestCommitCostGuards(t *testing.T) {
	t.Run("repeated same-handle histogram writes do not grow conflict evidence", func(t *testing.T) {
		s := NewCollectorStore()
		sv := collectorStoreViewForTest(t, s)
		cc := cycleController(t, s)
		cc.BeginCycle()
		// Stateful: 1000 Observe on one handle.
		hs := s.Write().StatefulMeter("svc").Histogram("hs", WithHistogramBounds(1, 2))
		for i := 0; i < 1000; i++ {
			hs.Observe(1)
		}
		// Snapshot: 1000 ObservePoint with the same bounds on one handle.
		hp := s.Write().SnapshotMeter("svc").Histogram("hp")
		pt := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: 1, CumulativeCount: 1}, {UpperBound: 2, CumulativeCount: 1}}}
		for i := 0; i < 1000; i++ {
			hp.ObservePoint(pt)
		}
		require.Empty(t, sv.core.active.conflicts, "same-handle repeats must not append descriptor evidence")
		require.NoError(t, cc.CommitCycleSuccess())
	})

	t.Run("sparse commit over a large retained name is O(touched) allocations", func(t *testing.T) {
		const retained = 2000
		s := NewCollectorStore()
		cc := cycleController(t, s)
		labels := make([]string, retained)
		for i := range labels {
			labels[i] = strconv.Itoa(i)
		}

		cc.BeginCycle()
		for i := 0; i < retained; i++ {
			s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: labels[i]}).Gauge("m").Observe(SampleValue(i))
		}
		require.NoError(t, cc.CommitCycleSuccess())

		// A cycle touching only 5 of the retained series must not clone all of them.
		allocs := testing.AllocsPerRun(2, func() {
			cc.BeginCycle()
			for i := 0; i < 5; i++ {
				s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: labels[i]}).Gauge("m").Observe(SampleValue(i))
			}
			_ = cc.CommitCycleSuccess()
		})
		require.Lessf(t, allocs, float64(retained/4), "sparse commit allocs must be O(touched), not O(retained) (got %.0f allocs)", allocs)
	})
}

// TestMultiAuthorityCommittedObserved pins the multi-authority fail-vs-drop decision: the resolver
// keeps EVERY distinct observed authority (fingerprint-indexed, no cap) and tracks committed-observed
// over all of them, so an established authority actively written alongside incompatible ones FAILs
// loud (never a silent gap on an established series), while an ambiguous NEW name with no committed
// authority DROPs - regardless of how many authorities are observed or in what order.
func TestMultiAuthorityCommittedObserved(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"committed histogram observed alongside two incompatible kinds fails, not drops": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Cycle 1: establish svc.m as a stateful histogram.
				cc.BeginCycle()
				s.Write().StatefulMeter("svc").Histogram("m", WithHistogramBounds(1, 2)).Observe(0.5)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: svc.m is observed as a gauge, a counter, AND the established histogram
				// (matching bounds). The committed authority IS actively observed among the three ->
				// fail loud (never a silent gap on an established series).
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(2)
				s.Write().StatefulMeter("svc").Histogram("m", WithHistogramBounds(1, 2)).Observe(0.7)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "conflicting instrument kinds")
			},
		},
		"ambiguous multi-kind on a new name drops (no committed authority)": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Three incompatible kinds for a NEW name, none committed: still ambiguous ->
				// drop this name (the resolver must not turn drop into fail when nothing is
				// established). An unrelated metric must still commit.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(2)
				s.Write().StatefulMeter("svc").Histogram("m", WithHistogramBounds(1, 2)).Observe(0.7)
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)
				require.NoError(t, cc.CommitCycleSuccess())

				r := s.Read()
				_, ok := r.Value("svc.m", nil)
				require.False(t, ok, "ambiguous new name must be dropped")
				mustValue(t, r, "svc.ok", nil, 9)
			},
		},
		"a declaration conflict on a third authority fails deterministically across map orders": {
			run: func(t *testing.T) {
				// Same name, three authorities via STALE handles (aborted cycles bypass
				// registration's declaration check): a bytes gauge, a stateful (additive) gauge, and
				// a seconds gauge, each on a distinct label. The resolver keeps every distinct
				// authority (no cap), so the bytes-vs-seconds declaration conflict is caught
				// regardless of map-iteration order -> FAIL every time, never an order-dependent DROP.
				// Repeat to exercise staged-map order.
				for i := 0; i < 64; i++ {
					s := NewCollectorStore()
					cc := cycleController(t, s)

					cc.BeginCycle()
					hBytes := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "a"}).Gauge("m", WithUnit("bytes"))
					cc.AbortCycle()
					cc.BeginCycle()
					hStateful := s.Write().StatefulMeter("svc").WithLabels(Label{Key: "id", Value: "b"}).Gauge("m")
					cc.AbortCycle()
					cc.BeginCycle()
					hSeconds := s.Write().SnapshotMeter("svc").WithLabels(Label{Key: "id", Value: "c"}).Gauge("m", WithUnit("seconds"))
					cc.AbortCycle()

					cc.BeginCycle()
					hBytes.Observe(1)
					hStateful.Add(2)
					hSeconds.Observe(3)
					err := cc.CommitCycleSuccess()
					require.Errorf(t, err, "iteration %d", i)
					require.ErrorContainsf(t, err, "unit mismatch", "iteration %d", i)
				}
			},
		},
		"committed declaration conflict is caught even when the name is multi-authority": {
			run: func(t *testing.T) {
				pt := func(uppers ...float64) HistogramPoint {
					b := make([]BucketPoint, len(uppers))
					for i, ub := range uppers {
						b[i] = BucketPoint{UpperBound: ub, CumulativeCount: 1}
					}
					return HistogramPoint{Count: 1, Sum: 1, Buckets: b}
				}
				s := NewCollectorStore()
				cc := cycleController(t, s)
				// A stale unit=seconds histogram handle from an aborted cycle (created before any
				// committed authority, so its registration is not rejected).
				cc.BeginCycle()
				stale := s.Write().SnapshotMeter("svc").Histogram("m", WithUnit("seconds"))
				cc.AbortCycle()
				// Init-register a committed nil-bounds histogram with unit=bytes (never observed -> a
				// wildcard committed authority, so realCommittedAuthority returns nil).
				_ = s.Write().SnapshotMeter("svc").Histogram("m", WithUnit("bytes"))

				// The stale handle observes two different bounds (multi-authority). The committed
				// bytes vs observed seconds is a declaration conflict that must FAIL loud even though
				// the multi-authority name would otherwise DROP.
				cc.BeginCycle()
				stale.ObservePoint(pt(1, 2))
				stale.ObservePoint(pt(5, 6))
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.ErrorContains(t, err, "unit mismatch")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
