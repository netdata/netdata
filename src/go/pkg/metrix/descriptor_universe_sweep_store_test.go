// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDescriptorUniverseSweep pins the bounded-growth contract: a descriptor is kept
// while its name has a live series and for descriptorGraceCycles successful commits
// after its last series is evicted, then swept from instruments. A continuously
// observed name is never swept; a swept name re-establishes when observed again.
func TestDescriptorUniverseSweep(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"last-series eviction sweeps the descriptor after grace, keeps it during grace": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{expireAfterSuccessCycles: 1, descriptorGraceCycles: 2}
				cc := cycleController(t, s)

				// Cycle 1 (successSeq=1): observe -> live.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("g").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", true)

				// Cycle 2 (successSeq=2): not observed -> series expires (expire=1); descriptor
				// goes idle at 2 and is kept (grace not elapsed).
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", true)
				_, ok := s.Read(ReadRaw()).Value("svc.g", nil)
				require.False(t, ok, "the series must be evicted by retention")

				// Cycle 3 (successSeq=3): 3-2=1 < grace(2) -> still kept.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", true)

				// Cycle 4 (successSeq=4): 4-2=2 >= grace(2) -> swept.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", false)
				require.Equal(t, uint64(1), s.Read().CollectMeta().EvictedDescriptors)
			},
		},
		"a continuously observed name is never swept": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{expireAfterSuccessCycles: 1, descriptorGraceCycles: 1}
				cc := cycleController(t, s)

				for i := range 10 {
					cc.BeginCycle()
					s.Write().SnapshotMeter("svc").Gauge("g").Observe(SampleValue(i))
					require.NoError(t, cc.CommitCycleSuccess())
				}
				requireInstrument(t, sv, "svc.g", true)
				require.Equal(t, uint64(0), s.Read().CollectMeta().EvictedDescriptors)
			},
		},
		"a registered-but-never-published descriptor is swept after grace": {
			run: func(t *testing.T) {
				s := NewCollectorStore(WithExpireAfterSuccessCycles(1), WithDescriptorGraceCycles(2))
				sv := collectorStoreViewForTest(t, s)

				// Declared outside a cycle -> Init-time eager install, never observed.
				s.Write().SnapshotMeter("svc").Gauge("x")
				requireInstrument(t, sv, "svc.x", true)

				cc := cycleController(t, s)
				// Cycle 1 (successSeq=1): idle at 1, kept.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.x", true)
				// Cycle 2 (successSeq=2): 2-1=1 < grace(2) -> kept.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.x", true)
				// Cycle 3 (successSeq=3): 3-1=2 >= grace(2) -> swept.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.x", false)
				require.Equal(t, uint64(1), s.Read().CollectMeta().EvictedDescriptors)
			},
		},
		"grace zero sweeps the descriptor the cycle its last series is evicted": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{expireAfterSuccessCycles: 1, descriptorGraceCycles: 0}
				cc := cycleController(t, s)

				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("g").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Series evicted this cycle; grace=0 -> descriptor swept the same cycle.
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", false)
				require.Equal(t, uint64(1), s.Read().CollectMeta().EvictedDescriptors)
			},
		},
		"a swept name re-establishes when observed again": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{expireAfterSuccessCycles: 1, descriptorGraceCycles: 0}
				cc := cycleController(t, s)

				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("g").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", false)

				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("g").Observe(5)
				require.NoError(t, cc.CommitCycleSuccess())
				requireInstrument(t, sv, "svc.g", true)
				mustValue(t, s.Read(ReadRaw()), "svc.g", nil, 5)
			},
		},
		"supersede of an idle-stamped name whose replacement is evicted leaves no zeroSince orphan": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{expireAfterSuccessCycles: 1, maxSeries: 1, descriptorGraceCycles: 100}
				cc := cycleController(t, s)

				// Cycle 1: observe svc.a (gauge) -> committed, live.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("a").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// Cycle 2: not observed -> series expires (expire=1); svc.a goes idle (grace=100).
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
				require.Contains(t, sv.core.instrumentZeroSince, "svc.a", "an idle name must be stamped")

				// Cycle 3: supersede svc.a with a new kind AND observe svc.z. maxSeries=1 evicts
				// the new svc.a series (smaller key) before canonicalization, so svc.a never
				// re-enters instruments. Its idle stamp must be cleared, not orphaned forever.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Counter("a").ObserveTotal(2)
				s.Write().SnapshotMeter("svc").Gauge("z").Observe(3)
				require.NoError(t, cc.CommitCycleSuccess())

				requireInstrument(t, sv, "svc.a", false)
				require.NotContains(t, sv.core.instrumentZeroSince, "svc.a", "no orphan idle stamp after supersede")
				// The invariant the fix maintains: instrumentZeroSince is a subset of instruments.
				for name := range sv.core.instrumentZeroSince {
					require.Contains(t, sv.core.instruments, name, "instrumentZeroSince must stay a subset of instruments")
				}
			},
		},
		"dropped-name and eviction counters accumulate and survive abort": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				// Ambiguous multi-kind name is dropped; an unrelated name still commits.
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				s.Write().SnapshotMeter("svc").Counter("m").ObserveTotal(2)
				s.Write().SnapshotMeter("svc").Gauge("ok").Observe(9)
				require.NoError(t, cc.CommitCycleSuccess())
				require.Equal(t, uint64(1), s.Read().CollectMeta().DroppedNames)
				require.Equal(t, uint64(0), s.Read().CollectMeta().EvictedDescriptors)

				// An aborted cycle must not change the counters.
				cc.BeginCycle()
				cc.AbortCycle()
				require.Equal(t, uint64(1), s.Read().CollectMeta().DroppedNames)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) { tc.run(t) })
	}
}

func requireInstrument(t *testing.T, sv *storeView, name string, want bool) {
	t.Helper()
	_, ok := sv.core.instruments[name]
	require.Equalf(t, want, ok, "instruments[%q] present=%v, want %v", name, ok, want)
}
