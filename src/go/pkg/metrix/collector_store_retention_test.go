// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectorStoreRetentionScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"retention aging advances on successful cycles only": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{
					expireAfterSuccessCycles: 2,
					maxSeries:                0,
				}
				cc := cycleController(t, s)
				m := s.Write().SnapshotMeter("collector")
				g := m.Gauge("g")

				cc.BeginCycle()
				g.Observe(11)
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				cc.AbortCycle()

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(ReadRaw()), "collector.g", nil, 11)

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				mustNoValue(t, s.Read(ReadRaw()), "collector.g", nil)
			},
		},
		"max-series cap evicts oldest series deterministically": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{
					expireAfterSuccessCycles: 0,
					maxSeries:                2,
				}
				cc := cycleController(t, s)
				m := s.Write().SnapshotMeter("collector")
				g := m.Gauge("g")
				lsa := m.LabelSet(Label{Key: "id", Value: "a"})
				lsb := m.LabelSet(Label{Key: "id", Value: "b"})
				lsc := m.LabelSet(Label{Key: "id", Value: "c"})

				cc.BeginCycle()
				g.Observe(1, lsa)
				g.Observe(2, lsb)
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				g.Observe(3, lsc)
				cc.CommitCycleSuccess()

				mustNoValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "a"})
				mustValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "b"}, 2)
				mustValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "c"}, 3)
			},
		},
		"max-series cap tie-break uses stable key order when lastSeen is equal": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				sv := collectorStoreViewForTest(t, s)
				sv.core.retention = collectorRetentionPolicy{
					expireAfterSuccessCycles: 0,
					maxSeries:                2,
				}
				cc := cycleController(t, s)
				m := s.Write().SnapshotMeter("collector")
				g := m.Gauge("g")
				lsa := m.LabelSet(Label{Key: "id", Value: "a"})
				lsb := m.LabelSet(Label{Key: "id", Value: "b"})
				lsc := m.LabelSet(Label{Key: "id", Value: "c"})

				// All series are written in the same successful cycle so they share
				// identical retention age; eviction must be deterministic by key.
				cc.BeginCycle()
				g.Observe(1, lsa)
				g.Observe(2, lsb)
				g.Observe(3, lsc)
				cc.CommitCycleSuccess()

				mustNoValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "a"})
				mustValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "b"}, 2)
				mustValue(t, s.Read(ReadRaw()), "collector.g", Labels{"id": "c"}, 3)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func collectorStoreViewForTest(t *testing.T, s CollectorStore) *storeView {
	t.Helper()
	v, ok := s.(*storeView)
	require.True(t, ok, "unexpected collector store implementation: %T", s)
	return v
}

func mustNoValue(t *testing.T, r Reader, name string, labels Labels) {
	t.Helper()
	_, ok := r.Value(name, labels)
	require.False(t, ok, "expected no value for %s labels=%v", name, labels)
}
