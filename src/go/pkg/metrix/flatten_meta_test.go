// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenSeriesMetaCarriesOriginType(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"histogram flatten series carry source kind and role": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				h := s.Write().SnapshotMeter("svc").Histogram("latency", WithHistogramBounds(1, 2))
				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 2,
					Sum:   3,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()

				flat := s.Read(ReadFlatten())
				metaBucket, ok := flat.SeriesMeta("svc.latency_bucket", Labels{"le": "1"})
				require.True(t, ok)
				assert.Equal(t, MetricKindCounter, metaBucket.Kind)
				assert.Equal(t, MetricKindHistogram, metaBucket.SourceKind)
				assert.Equal(t, FlattenRoleHistogramBucket, metaBucket.FlattenRole)

				metaCount, ok := flat.SeriesMeta("svc.latency_count", nil)
				require.True(t, ok)
				assert.Equal(t, MetricKindCounter, metaCount.Kind)
				assert.Equal(t, MetricKindHistogram, metaCount.SourceKind)
				assert.Equal(t, FlattenRoleHistogramCount, metaCount.FlattenRole)
			},
		},
		"stateset flatten series carry source kind and role": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)

				ss := s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational"),
					WithStateSetMode(ModeEnum),
				)
				cc.BeginCycle()
				ss.Enable("operational")
				cc.CommitCycleSuccess()

				rawMeta, ok := s.Read().SeriesMeta("svc.mode", nil)
				require.True(t, ok)
				assert.Equal(t, MetricKindStateSet, rawMeta.Kind)
				assert.Equal(t, MetricKindStateSet, rawMeta.SourceKind)
				assert.Equal(t, FlattenRoleNone, rawMeta.FlattenRole)

				flatMeta, ok := s.Read(ReadFlatten()).SeriesMeta("svc.mode", Labels{"svc.mode": "operational"})
				require.True(t, ok)
				assert.Equal(t, MetricKindGauge, flatMeta.Kind)
				assert.Equal(t, MetricKindStateSet, flatMeta.SourceKind)
				assert.Equal(t, FlattenRoleStateSetState, flatMeta.FlattenRole)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
