// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"math"
	"slices"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type (
	MetricFamilies map[string]*MetricFamily

	MetricFamily struct {
		name    string
		help    string
		typ     model.MetricType
		metrics []Metric
	}
	Metric struct {
		labels    []labels.Label
		gauge     *Gauge
		counter   *Counter
		summary   *Summary
		histogram *Histogram
		untyped   *Untyped
	}
	Gauge struct {
		value float64
	}
	Counter struct {
		value float64
	}
	Summary struct {
		sum       float64
		count     float64
		quantiles []Quantile
	}
	Quantile struct {
		quantile float64
		value    float64
	}
	Histogram struct {
		sum     float64
		count   float64
		buckets []Bucket
	}
	Bucket struct {
		upperBound      float64
		cumulativeCount float64
	}
	Untyped struct {
		value float64
	}
)

func (mfs MetricFamilies) Len() int {
	return len(mfs)
}

func (mfs MetricFamilies) Get(name string) *MetricFamily {
	return (mfs)[name]
}

func (mfs MetricFamilies) GetGauge(name string) *MetricFamily {
	return mfs.get(name, model.MetricTypeGauge)
}

func (mfs MetricFamilies) GetCounter(name string) *MetricFamily {
	return mfs.get(name, model.MetricTypeCounter)
}

func (mfs MetricFamilies) GetSummary(name string) *MetricFamily {
	return mfs.get(name, model.MetricTypeSummary)
}

func (mfs MetricFamilies) GetHistogram(name string) *MetricFamily {
	return mfs.get(name, model.MetricTypeHistogram)
}

func (mfs MetricFamilies) get(name string, typ model.MetricType) *MetricFamily {
	mf := mfs.Get(name)
	if mf == nil || mf.typ != typ {
		return nil
	}
	return mf
}

func (mf *MetricFamily) Name() string           { return mf.name }
func (mf *MetricFamily) Help() string           { return mf.help }
func (mf *MetricFamily) Type() model.MetricType { return mf.typ }
func (mf *MetricFamily) Metrics() []Metric      { return mf.metrics }

func (m *Metric) Labels() labels.Labels { return m.labels }
func (m *Metric) Gauge() *Gauge         { return m.gauge }
func (m *Metric) Counter() *Counter     { return m.counter }
func (m *Metric) Summary() *Summary     { return m.summary }
func (m *Metric) Histogram() *Histogram { return m.histogram }
func (m *Metric) Untyped() *Untyped     { return m.untyped }

func (g Gauge) Value() float64   { return g.value }
func (c Counter) Value() float64 { return c.value }
func (u Untyped) Value() float64 { return u.value }

func (s Summary) Count() float64        { return s.count }
func (s Summary) Sum() float64          { return s.sum }
func (s Summary) Quantiles() []Quantile { return s.quantiles }

// IsNaN reports whether every quantile value is NaN, which a Prometheus summary emits for an
// empty observation window (a summary with no quantiles also reports true). Callers skip such
// a summary so a chart is not created until it carries a real value.
func (s Summary) IsNaN() bool {
	return !slices.ContainsFunc(s.quantiles, func(q Quantile) bool { return !math.IsNaN(q.value) })
}

func (q Quantile) Quantile() float64 { return q.quantile }
func (q Quantile) Value() float64    { return q.value }

func (h Histogram) Count() float64    { return h.count }
func (h Histogram) Sum() float64      { return h.sum }
func (h Histogram) Buckets() []Bucket { return h.buckets }

func (b Bucket) UpperBound() float64      { return b.upperBound }
func (b Bucket) CumulativeCount() float64 { return b.cumulativeCount }
