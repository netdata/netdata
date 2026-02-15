// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenSnapshotScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"malformed histogram cumulative length is skipped safely": {
			run: func(t *testing.T) {
				src := &readSnapshot{
					series: map[string]*committedSeries{
						"svc.latency": {
							key:                 "svc.latency",
							name:                "svc.latency",
							desc:                &instrumentDescriptor{name: "svc.latency", kind: kindHistogram, mode: modeSnapshot, freshness: FreshnessCycle, histogram: &histogramSchema{bounds: []float64{1, 2}}},
							meta:                SeriesMeta{LastSeenSuccessSeq: 1},
							value:               0,
							histogramCount:      2,
							histogramSum:        3,
							histogramCumulative: []SampleValue{1}, // malformed: len 1, bounds len 2
						},
					},
				}

				flat := flattenSnapshot(src)
				r := &storeReader{snap: flat}

				_, ok := r.Value("svc.latency_bucket", Labels{"le": "1"})
				require.False(t, ok, "expected malformed histogram bucket series to be skipped")
				_, ok = r.Value("svc.latency_count", nil)
				require.False(t, ok, "expected malformed histogram count series to be skipped")
				_, ok = r.Value("svc.latency_sum", nil)
				require.False(t, ok, "expected malformed histogram sum series to be skipped")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
