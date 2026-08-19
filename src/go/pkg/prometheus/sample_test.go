// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestSampleComponentToken(t *testing.T) {
	tests := map[SampleKind]string{
		SampleKindScalar:          "scalar",
		SampleKindHistogramBucket: "histogram_bucket",
		SampleKindHistogramSum:    "histogram_sum",
		SampleKindHistogramCount:  "histogram_count",
		SampleKindSummaryQuantile: "summary_quantile",
		SampleKindSummarySum:      "summary_sum",
		SampleKindSummaryCount:    "summary_count",
		SampleKind(255):           "",
	}
	for kind, want := range tests {
		if got := SampleComponentToken(kind); got != want {
			t.Fatalf("SampleComponentToken(%d) = %q, want %q", kind, got, want)
		}
	}
}

func TestMetricTypeToken(t *testing.T) {
	tests := map[model.MetricType]string{
		model.MetricTypeGauge:     "gauge",
		model.MetricTypeCounter:   "counter",
		model.MetricTypeHistogram: "histogram",
		model.MetricTypeSummary:   "summary",
		model.MetricTypeUnknown:   "untyped",
		model.MetricType("other"): "",
	}
	for typ, want := range tests {
		if got := MetricTypeToken(typ); got != want {
			t.Fatalf("MetricTypeToken(%q) = %q, want %q", typ, got, want)
		}
	}
}
