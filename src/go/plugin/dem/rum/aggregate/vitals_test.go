// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPercentiles(t *testing.T) {
	tests := map[string]struct {
		vals []float64
		p50  float64
		p75  float64
		p95  float64
	}{
		"1..100":       {vals: seq(1, 100), p50: 50, p75: 75, p95: 95},
		"single":       {vals: []float64{7}, p50: 7, p75: 7, p95: 7},
		"two":          {vals: []float64{10, 20}, p50: 10, p75: 20, p95: 20},
		"unsorted":     {vals: []float64{5, 1, 4, 2, 3}, p50: 3, p75: 4, p95: 5},
		"four samples": {vals: []float64{100, 200, 300, 400}, p50: 200, p75: 300, p95: 400},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := pctStats(tc.vals)
			got := [3]float64{result.P50, result.P75, result.P95}
			want := [3]float64{tc.p50, tc.p75, tc.p95}
			if got != want {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func seq(from, to int) []float64 {
	out := make([]float64, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, float64(i))
	}
	return out
}

func TestRatings(t *testing.T) {
	tests := map[string]struct {
		vital string
		vals  []float64
		want  VitalStats
	}{
		"lcp mixed": {
			vital: beacon.LCP, vals: []float64{1000, 2500, 3000, 5000},
			want: VitalStats{
				N:         4,
				P50:       2500,
				P75:       3000,
				P95:       5000,
				Good:      2,
				NeedsImpr: 1,
				Poor:      1,
			},
		},
		"cls boundary good inclusive": {
			vital: beacon.CLS, vals: []float64{0.1, 0.25, 0.26},
			want: VitalStats{
				N:         3,
				P50:       0.25,
				P75:       0.26,
				P95:       0.26,
				Good:      1,
				NeedsImpr: 1,
				Poor:      1,
			},
		},
		"inp all good": {
			vital: beacon.INP, vals: []float64{50, 100},
			want: VitalStats{
				N:         2,
				P50:       50,
				P75:       100,
				P95:       100,
				Good:      2,
				NeedsImpr: 0,
				Poor:      0,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := stats(tc.vital, tc.vals); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

// Read slots follow the emission order, so a slot index names its vital.
func TestVitalSlotsFollowEmissionOrder(t *testing.T) {
	require.Len(t, beacon.Vitals, vitalCount)
	for slot, name := range beacon.Vitals {
		index, ok := vitalIndex(name)
		require.True(t, ok, name)
		assert.Equal(t, slot, index, name)
	}
	_, ok := vitalIndex("unsupported")
	assert.False(t, ok)
}
