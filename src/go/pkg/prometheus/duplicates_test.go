// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindSampleDuplicatesUsesAssemblyComponentIdentity(t *testing.T) {
	tests := map[string]struct {
		exposition string
		kind       SampleKind
	}{
		"scalar": {
			exposition: "# TYPE app_value gauge\napp_value{id=\"a\"} 1\napp_value{id=\"a\"} 2\n",
			kind:       SampleKindScalar,
		},
		"histogram bucket": {
			exposition: "# TYPE app_latency histogram\napp_latency_bucket{id=\"a\",le=\"1\"} 1\napp_latency_bucket{le=\"1.0\",id=\"a\"} 2\n",
			kind:       SampleKindHistogramBucket,
		},
		"histogram sum": {
			exposition: "# TYPE app_latency histogram\napp_latency_sum{id=\"a\"} 1\napp_latency_sum{id=\"a\"} 2\n",
			kind:       SampleKindHistogramSum,
		},
		"histogram count": {
			exposition: "# TYPE app_latency histogram\napp_latency_count{id=\"a\"} 1\napp_latency_count{id=\"a\"} 2\n",
			kind:       SampleKindHistogramCount,
		},
		"summary quantile": {
			exposition: "# TYPE app_latency summary\napp_latency{id=\"a\",quantile=\"0.5\"} 1\napp_latency{quantile=\"0.5\",id=\"a\"} 2\n",
			kind:       SampleKindSummaryQuantile,
		},
		"summary sum": {
			exposition: "# TYPE app_latency summary\napp_latency_sum{id=\"a\"} 1\napp_latency_sum{id=\"a\"} 2\n",
			kind:       SampleKindSummarySum,
		},
		"summary count": {
			exposition: "# TYPE app_latency summary\napp_latency_count{id=\"a\"} 1\napp_latency_count{id=\"a\"} 2\n",
			kind:       SampleKindSummaryCount,
		},
		"positive infinity bucket": {
			exposition: "# TYPE app_latency histogram\napp_latency_bucket{id=\"a\",le=\"+Inf\"} 1\napp_latency_bucket{le=\"+Inf\",id=\"a\"} 2\n",
			kind:       SampleKindHistogramBucket,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var parser promTextParser
			batch, err := parser.parseToSamples([]byte(tc.exposition))
			require.NoError(t, err)
			require.Len(t, batch.Samples, 2)
			assert.Equal(t, tc.kind, batch.Samples[0].Kind)

			duplicates := FindSampleDuplicates(batch)
			require.Equal(t, []SampleDuplicate{{FirstIndex: 0, DuplicateIndex: 1}}, duplicates)
		})
	}
}

func TestFindSampleDuplicatesKeepsDistinctComponents(t *testing.T) {
	var parser promTextParser
	batch, err := parser.parseToSamples([]byte(`# TYPE app_latency histogram
app_latency_bucket{id="a",le="1"} 1
app_latency_bucket{id="a",le="2"} 2
app_latency_bucket{id="b",le="1"} 3
app_latency_bucket{id="a",le="+Inf"} 3
app_latency_sum{id="a"} 3
app_latency_count{id="a"} 3
`))
	require.NoError(t, err)
	assert.Empty(t, FindSampleDuplicates(batch))
}

func BenchmarkFindSampleDuplicates(b *testing.B) {
	const sampleCount = 10_000
	batch := SampleBatch{Samples: make([]Sample, 0, sampleCount)}
	for i := range sampleCount {
		batch.Samples = append(batch.Samples, Sample{
			Name:       "app_value",
			Labels:     labels.FromStrings("instance", strconv.Itoa(i)),
			Kind:       SampleKindScalar,
			FamilyType: model.MetricTypeGauge,
		})
	}

	b.ReportAllocs()
	for b.Loop() {
		if duplicates := FindSampleDuplicates(batch); len(duplicates) != 0 {
			b.Fatalf("unexpected duplicates: %v", duplicates)
		}
	}
}
