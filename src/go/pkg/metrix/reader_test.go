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
		"malformed summary quantile length skips quantile series but keeps count sum": {
			run: func(t *testing.T) {
				src := &readSnapshot{
					series: map[string]*committedSeries{
						"svc.latency": {
							key:   "svc.latency",
							name:  "svc.latency",
							desc:  &instrumentDescriptor{name: "svc.latency", kind: kindSummary, mode: modeSnapshot, freshness: FreshnessCycle, summary: &summarySchema{quantiles: []float64{0.5, 0.9}}},
							meta:  SeriesMeta{LastSeenSuccessSeq: 1},
							value: 0,

							summaryCount:     2,
							summarySum:       1.2,
							summaryQuantiles: []SampleValue{0.4}, // malformed: len 1, quantiles len 2
						},
					},
				}

				flat := flattenSnapshot(src)
				require.Contains(t, flat.series, "svc.latency_count")
				require.Contains(t, flat.series, "svc.latency_sum")
				r := &storeReader{snap: flat, raw: true}

				mustValue(t, r, "svc.latency_count", nil, 2)
				mustValue(t, r, "svc.latency_sum", nil, 1.2)
				_, ok := r.Value("svc.latency", Labels{"quantile": "0.5"})
				require.False(t, ok, "expected malformed summary quantile series to be skipped")
			},
		},
		"flattened key collision suppresses both outputs and tracks metadata": {
			run: func(t *testing.T) {
				histKey := makeSeriesKey("svc.latency", "")
				countKey := makeSeriesKey("svc.latency_count", "")
				src := &readSnapshot{
					collectMeta: CollectMeta{
						LastAttemptStatus: CollectStatusSuccess,
						LastAttemptSeq:    1,
						LastSuccessSeq:    1,
					},
					series: map[string]*committedSeries{
						histKey: {
							key:                 histKey,
							name:                "svc.latency",
							desc:                &instrumentDescriptor{name: "svc.latency", kind: kindHistogram, mode: modeSnapshot, freshness: FreshnessCycle, histogram: &histogramSchema{bounds: []float64{1}}},
							meta:                SeriesMeta{LastSeenSuccessSeq: 1},
							histogramCount:      1,
							histogramSum:        0.1,
							histogramCumulative: []SampleValue{1},
						},
						countKey: {
							key:   countKey,
							name:  "svc.latency_count",
							desc:  &instrumentDescriptor{name: "svc.latency_count", kind: kindCounter, mode: modeSnapshot, freshness: FreshnessCycle},
							meta:  SeriesMeta{LastSeenSuccessSeq: 1},
							value: 7,
						},
					},
				}

				flat := flattenSnapshot(src)
				require.Equal(t, uint64(1), flat.collectMeta.FlattenCollisionCount)

				r := &storeReader{snap: flat, raw: true, flattened: true}
				_, ok := r.Value("svc.latency_count", nil)
				require.False(t, ok, "expected collided flattened count series to be suppressed")
				mustValue(t, r, "svc.latency_sum", nil, 0.1)
				mustValue(t, r, "svc.latency_bucket", Labels{"le": "1"}, 1)
				mustValue(t, r, "svc.latency_bucket", Labels{"le": "+Inf"}, 1)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
