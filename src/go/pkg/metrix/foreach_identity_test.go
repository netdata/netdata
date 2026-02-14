// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForEachSeriesIdentityScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"returns stable id/hash and deterministic order": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("svc")
				g := sm.Gauge("requests")
				lsA := sm.LabelSet(Label{Key: "instance", Value: "a"})
				lsB := sm.LabelSet(Label{Key: "instance", Value: "b"})

				cc.BeginCycle()
				g.Observe(20, lsB)
				g.Observe(10, lsA)
				cc.CommitCycleSuccess()

				type gotSeries struct {
					id    SeriesID
					hash  uint64
					name  string
					value SampleValue
				}
				got := make([]gotSeries, 0)

				s.Read().ForEachSeriesIdentity(func(identity SeriesIdentity, _ SeriesMeta, name string, _ LabelView, v SampleValue) {
					got = append(got, gotSeries{
						id:    identity.ID,
						hash:  identity.Hash64,
						name:  name,
						value: v,
					})
				})

				require.Len(t, got, 2)
				assert.Equal(t, "svc.requests", got[0].name)
				assert.Equal(t, "svc.requests", got[1].name)
				assert.Contains(t, string(got[0].id), "instance\xffa")
				assert.Contains(t, string(got[1].id), "instance\xffb")
				assert.Equal(t, seriesIDHash(got[0].id), got[0].hash)
				assert.Equal(t, seriesIDHash(got[1].id), got[1].hash)
				assert.Equal(t, SampleValue(10), got[0].value)
				assert.Equal(t, SampleValue(20), got[1].value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
