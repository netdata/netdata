package prometheus

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
)

func TestMetricFamilies_Len(t *testing.T) {
	tests := map[string]struct {
		mfs     MetricFamilies
		wantLen int
	}{
		"initialized with two elements": {
			mfs:     MetricFamilies{"1": nil, "2": nil},
			wantLen: 2,
		},
		"not initialized": {
			mfs:     nil,
			wantLen: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.Len(), test.wantLen)
		})
	}
}

func TestMetricFamilies_Get(t *testing.T) {
	const n = "metric"

	tests := map[string]struct {
		mfs    MetricFamilies
		wantMF *MetricFamily
	}{
		"etric is found": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n}},
			wantMF: &MetricFamily{name: n},
		},
		"metric is not found": {
			mfs:    MetricFamilies{"!" + n: &MetricFamily{name: n}},
			wantMF: nil,
		},
		"not initialized": {
			mfs:    nil,
			wantMF: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.Get(n), test.wantMF)
		})
	}
}

func TestMetricFamilies_GetGauge(t *testing.T) {
	const n = "metric"

	tests := map[string]struct {
		mfs    MetricFamilies
		wantMF *MetricFamily
	}{
		"metric is found and is Gauge": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: &MetricFamily{name: n, typ: model.MetricTypeGauge},
		},
		"metric is found but it is not Gauge": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeUnknown}},
			wantMF: nil,
		},
		"metric is not found": {
			mfs:    MetricFamilies{"!" + n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"not initialized": {
			mfs:    nil,
			wantMF: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.GetGauge(n), test.wantMF)
		})
	}
}

func TestMetricFamilies_GetCounter(t *testing.T) {
	const n = "metric"

	tests := map[string]struct {
		mfs    MetricFamilies
		wantMF *MetricFamily
	}{
		"metric is found and is Counter": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeCounter}},
			wantMF: &MetricFamily{name: n, typ: model.MetricTypeCounter},
		},
		"metric is found but it is not Counter": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"metric is not found": {
			mfs:    MetricFamilies{"!" + n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"not initialized": {
			mfs:    nil,
			wantMF: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.GetCounter(n), test.wantMF)
		})
	}
}

func TestMetricFamilies_GetSummary(t *testing.T) {
	const n = "metric"

	tests := map[string]struct {
		mfs    MetricFamilies
		wantMF *MetricFamily
	}{
		"metric is found and is Summary": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeSummary}},
			wantMF: &MetricFamily{name: n, typ: model.MetricTypeSummary},
		},
		"metric is found but it is not Summary": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"metric is not found": {
			mfs:    MetricFamilies{"!" + n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"not initialized": {
			mfs:    nil,
			wantMF: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.GetSummary(n), test.wantMF)
		})
	}
}

func TestMetricFamilies_GetHistogram(t *testing.T) {
	const n = "metric"

	tests := map[string]struct {
		mfs    MetricFamilies
		wantMF *MetricFamily
	}{
		"metric is found and is Histogram": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeHistogram}},
			wantMF: &MetricFamily{name: n, typ: model.MetricTypeHistogram},
		},
		"metric is found but it is not Histogram": {
			mfs:    MetricFamilies{n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"metric is not found": {
			mfs:    MetricFamilies{"!" + n: &MetricFamily{name: n, typ: model.MetricTypeGauge}},
			wantMF: nil,
		},
		"not initialized": {
			mfs:    nil,
			wantMF: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.mfs.GetHistogram(n), test.wantMF)
		})
	}
}

func TestMetricFamily_Name(t *testing.T) {
	mf := &MetricFamily{name: "name"}
	assert.Equal(t, mf.Name(), "name")
}

func TestMetricFamily_Type(t *testing.T) {
	mf := &MetricFamily{typ: model.MetricTypeGauge}
	assert.Equal(t, mf.Type(), model.MetricTypeGauge)
}

func TestMetricFamily_Help(t *testing.T) {
	mf := &MetricFamily{help: "help"}
	assert.Equal(t, mf.Help(), "help")
}

func TestMetricFamily_Metrics(t *testing.T) {
	metrics := []Metric{{gauge: &Gauge{value: 1}, counter: &Counter{value: 1}}}
	mf := &MetricFamily{metrics: metrics}
	assert.Equal(t, mf.Metrics(), metrics)
}

func TestMetric_Labels(t *testing.T) {
	lbs := labels.Labels{{Name: "1", Value: "1"}, {Name: "2", Value: "2"}}
	m := &Metric{labels: lbs}
	assert.Equal(t, m.Labels(), lbs)
}

func TestMetric_Gauge(t *testing.T) {
	tests := map[string]struct {
		m    *Metric
		want *Gauge
	}{
		"gauge set": {
			m:    &Metric{gauge: &Gauge{value: 1}},
			want: &Gauge{value: 1},
		},
		"gauge not set": {
			m:    &Metric{},
			want: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.m.Gauge(), test.want)
		})
	}
}

func TestMetric_Counter(t *testing.T) {
	tests := map[string]struct {
		m    *Metric
		want *Counter
	}{
		"counter set": {
			m:    &Metric{counter: &Counter{value: 1}},
			want: &Counter{value: 1},
		},
		"counter not set": {
			m:    &Metric{},
			want: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.m.Counter(), test.want)
		})
	}
}

func TestMetric_Summary(t *testing.T) {
	tests := map[string]struct {
		m    *Metric
		want *Summary
	}{
		"summary set": {
			m:    &Metric{summary: &Summary{sum: 0.1, count: 3}},
			want: &Summary{sum: 0.1, count: 3},
		},
		"summary not set": {
			m:    &Metric{},
			want: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.m.Summary(), test.want)
		})
	}
}

func TestMetric_Histogram(t *testing.T) {
	tests := map[string]struct {
		m    *Metric
		want *Histogram
	}{
		"histogram set": {
			m:    &Metric{histogram: &Histogram{sum: 0.1, count: 3}},
			want: &Histogram{sum: 0.1, count: 3},
		},
		"histogram not set": {
			m:    &Metric{},
			want: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.m.Histogram(), test.want)
		})
	}
}

func TestGauge_Value(t *testing.T) {
	assert.Equal(t, Gauge{value: 1}.Value(), 1.0)
}

func TestCounter_Value(t *testing.T) {
	assert.Equal(t, Counter{value: 1}.Value(), 1.0)
}

func TestSummary_Sum(t *testing.T) {
	assert.Equal(t, Summary{sum: 1}.Sum(), 1.0)
}

func TestSummary_Count(t *testing.T) {
	assert.Equal(t, Summary{count: 1}.Count(), 1.0)
}

func TestSummary_Quantiles(t *testing.T) {
	assert.Equal(t,
		Summary{quantiles: []Quantile{{quantile: 0.1, value: 1}}}.Quantiles(),
		[]Quantile{{quantile: 0.1, value: 1}},
	)
}

func TestQuantile_Value(t *testing.T) {
	assert.Equal(t, Quantile{value: 1}.Value(), 1.0)
}

func TestQuantile_Quantile(t *testing.T) {
	assert.Equal(t, Quantile{quantile: 0.1}.Quantile(), 0.1)
}

func TestHistogram_Sum(t *testing.T) {
	assert.Equal(t, Histogram{sum: 1}.Sum(), 1.0)
}

func TestHistogram_Count(t *testing.T) {
	assert.Equal(t, Histogram{count: 1}.Count(), 1.0)
}

func TestHistogram_Buckets(t *testing.T) {
	assert.Equal(t,
		Histogram{buckets: []Bucket{{upperBound: 0.1, cumulativeCount: 1}}}.Buckets(),
		[]Bucket{{upperBound: 0.1, cumulativeCount: 1}},
	)
}

func TestBucket_UpperBound(t *testing.T) {
	assert.Equal(t, Bucket{upperBound: 0.1}.UpperBound(), 0.1)
}

func TestBucket_CumulativeCount(t *testing.T) {
	assert.Equal(t, Bucket{cumulativeCount: 1}.CumulativeCount(), 1.0)
}
