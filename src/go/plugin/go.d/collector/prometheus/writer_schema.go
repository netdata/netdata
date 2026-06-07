// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"cmp"
	"math"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	commonmodel "github.com/prometheus/common/model"
)

// metricFamilySchema captures the per-name distribution schema that metrix requires to stay stable
// across a family's series: the quantile set for summaries, the bucket bounds for histograms.
// Label keys are intentionally NOT part of the schema — each series carries its own labels, so a
// family may legitimately mix label-key sets (V1 rendered every series independently).
type metricFamilySchema struct {
	summaryQuantiles []float64
	histogramBounds  []float64
}

func deriveMetricFamilySchema(mf *prompkg.MetricFamily, typ commonmodel.MetricType) (metricFamilySchema, bool) {
	for _, metric := range mf.Metrics() {
		schema, ok := deriveMetricSchema(metric, typ)
		if ok {
			return schema, true
		}
	}

	return metricFamilySchema{}, false
}

func deriveMetricSchema(metric prompkg.Metric, typ commonmodel.MetricType) (metricFamilySchema, bool) {
	var schema metricFamilySchema

	switch typ {
	case commonmodel.MetricTypeGauge:
		if _, ok := metricScalarValue(metric, commonmodel.MetricTypeGauge); !ok {
			return metricFamilySchema{}, false
		}
	case commonmodel.MetricTypeCounter:
		if _, ok := metricScalarValue(metric, commonmodel.MetricTypeCounter); !ok {
			return metricFamilySchema{}, false
		}
	case commonmodel.MetricTypeSummary:
		summary := metric.Summary()
		if summary == nil {
			return metricFamilySchema{}, false
		}
		qs, ok := summaryQuantiles(summary)
		if !ok {
			return metricFamilySchema{}, false
		}
		if _, ok := toSummaryPoint(summary); !ok {
			return metricFamilySchema{}, false
		}
		schema.summaryQuantiles = qs
	case commonmodel.MetricTypeHistogram:
		histogram := metric.Histogram()
		if histogram == nil {
			return metricFamilySchema{}, false
		}
		bounds, ok := histogramBounds(histogram)
		if !ok {
			return metricFamilySchema{}, false
		}
		if _, ok := toHistogramPoint(histogram); !ok {
			return metricFamilySchema{}, false
		}
		schema.histogramBounds = bounds
	default:
		return metricFamilySchema{}, false
	}

	return schema, true
}

// metricIsWritable reports whether a series can be written under the family's canonical schema.
// Only the distribution schema must match (metrix keys hist/summary schema by metric name); label
// keys may differ between series and are written per-series.
func metricIsWritable(metric prompkg.Metric, typ commonmodel.MetricType, schema metricFamilySchema) bool {
	metricSchema, ok := deriveMetricSchema(metric, typ)
	if !ok {
		return false
	}
	return slices.Equal(schema.summaryQuantiles, metricSchema.summaryQuantiles) &&
		slices.Equal(schema.histogramBounds, metricSchema.histogramBounds)
}

func metricScalarValue(metric prompkg.Metric, typ commonmodel.MetricType) (float64, bool) {
	switch typ {
	case commonmodel.MetricTypeGauge:
		if gauge := metric.Gauge(); gauge != nil && isFinite(gauge.Value()) {
			return gauge.Value(), true
		}
	case commonmodel.MetricTypeCounter:
		if counter := metric.Counter(); counter != nil && isFinite(counter.Value()) {
			return counter.Value(), true
		}
	}

	if untyped := metric.Untyped(); untyped != nil && isFinite(untyped.Value()) {
		return untyped.Value(), true
	}

	return 0, false
}

func toSummaryPoint(summary *prompkg.Summary) (metrix.SummaryPoint, bool) {
	if summary == nil || len(summary.Quantiles()) == 0 {
		return metrix.SummaryPoint{}, false
	}
	if !isFinite(summary.Count()) || !isFinite(summary.Sum()) || summary.Count() < 0 {
		return metrix.SummaryPoint{}, false
	}

	quantiles := make([]metrix.QuantilePoint, 0, len(summary.Quantiles()))
	for _, q := range summary.Quantiles() {
		// Prometheus summaries can emit NaN quantile values before observing samples.
		// Keep them: metrix stores a NaN quantile value and chartengine renders it as a
		// gap. Only an infinite quantile value is rejected.
		if !isFinite(q.Quantile()) || q.Quantile() < 0 || q.Quantile() > 1 || math.IsInf(q.Value(), 0) {
			return metrix.SummaryPoint{}, false
		}
		quantiles = append(quantiles, metrix.QuantilePoint{
			Quantile: q.Quantile(),
			Value:    q.Value(),
		})
	}

	return metrix.SummaryPoint{
		Count:     summary.Count(),
		Sum:       summary.Sum(),
		Quantiles: quantiles,
	}, true
}

// toHistogramPoint validates and converts a scraped histogram. A present le="+Inf" bucket is kept for
// the count checks but is informational for output: metrix synthesizes the le="+Inf" flattened series
// from Count. Valid Prometheus histograms (the +Inf bucket equals Count) are therefore identical to
// V1; a malformed +Inf count is superseded by Count rather than rejected.
func toHistogramPoint(histogram *prompkg.Histogram) (metrix.HistogramPoint, bool) {
	if histogram == nil || len(histogram.Buckets()) == 0 {
		return metrix.HistogramPoint{}, false
	}
	if !isFinite(histogram.Count()) || !isFinite(histogram.Sum()) || histogram.Count() < 0 {
		return metrix.HistogramPoint{}, false
	}

	buckets := make([]metrix.BucketPoint, 0, len(histogram.Buckets()))
	for _, b := range histogram.Buckets() {
		if (math.IsNaN(b.UpperBound()) || math.IsInf(b.UpperBound(), -1)) || !isFinite(b.CumulativeCount()) || b.CumulativeCount() < 0 {
			return metrix.HistogramPoint{}, false
		}
		buckets = append(buckets, metrix.BucketPoint{
			UpperBound:      b.UpperBound(),
			CumulativeCount: b.CumulativeCount(),
		})
	}

	slices.SortFunc(buckets, func(a, b metrix.BucketPoint) int { return cmp.Compare(a.UpperBound, b.UpperBound) })
	for i := 1; i < len(buckets); i++ {
		if buckets[i].UpperBound <= buckets[i-1].UpperBound {
			return metrix.HistogramPoint{}, false
		}
		if buckets[i].CumulativeCount < buckets[i-1].CumulativeCount {
			return metrix.HistogramPoint{}, false
		}
	}
	for i := len(buckets) - 1; i >= 0; i-- {
		if math.IsInf(buckets[i].UpperBound, +1) {
			continue
		}
		if buckets[i].CumulativeCount > histogram.Count() {
			return metrix.HistogramPoint{}, false
		}
		break
	}

	return metrix.HistogramPoint{
		Count:   histogram.Count(),
		Sum:     histogram.Sum(),
		Buckets: buckets,
	}, true
}

func summaryQuantiles(summary *prompkg.Summary) ([]float64, bool) {
	if summary == nil || len(summary.Quantiles()) == 0 {
		return nil, false
	}

	qs := make([]float64, 0, len(summary.Quantiles()))
	for _, q := range summary.Quantiles() {
		if !isFinite(q.Quantile()) || q.Quantile() < 0 || q.Quantile() > 1 {
			return nil, false
		}
		qs = append(qs, q.Quantile())
	}
	slices.Sort(qs)
	for i := 1; i < len(qs); i++ {
		if qs[i] <= qs[i-1] {
			return nil, false
		}
	}
	return qs, true
}

func histogramBounds(histogram *prompkg.Histogram) ([]float64, bool) {
	if histogram == nil || len(histogram.Buckets()) == 0 {
		return nil, false
	}

	bounds := make([]float64, 0, len(histogram.Buckets()))
	for _, b := range histogram.Buckets() {
		if math.IsNaN(b.UpperBound()) || math.IsInf(b.UpperBound(), -1) {
			return nil, false
		}
		if math.IsInf(b.UpperBound(), +1) {
			continue
		}
		bounds = append(bounds, b.UpperBound())
	}
	if len(bounds) == 0 {
		return []float64{}, true
	}
	slices.Sort(bounds)
	for i := 1; i < len(bounds); i++ {
		if bounds[i] <= bounds[i-1] {
			return nil, false
		}
	}
	return bounds, true
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
