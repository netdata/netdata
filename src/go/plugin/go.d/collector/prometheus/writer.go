// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	commonmodel "github.com/prometheus/common/model"
)

type metricFamilyWriterPolicy struct {
	labelPrefix           string
	maxTSPerMetric        int
	isFallbackTypeGauge   func(string) bool
	isFallbackTypeCounter func(string) bool
}

type metricFamilyWriter struct {
	store   metrix.CollectorStore
	policy  metricFamilyWriterPolicy
	handles map[string]*metricFamilyHandle
	*logger.Logger
}

// metricFamilyHandle caches the per-name canonical distribution schema and instrument options so a
// family's series can be written each cycle without re-deriving them. Label keys are NOT cached:
// every series supplies its own labels (a family may mix label-key sets), so series are written
// through SnapshotMeter.WithLabels rather than a fixed label-key Vec.
type metricFamilyHandle struct {
	name             string
	typ              commonmodel.MetricType
	summaryQuantiles []float64
	histogramBounds  []float64
	opts             []metrix.InstrumentOption
}

func newMetricFamilyWriter(store metrix.CollectorStore, policy metricFamilyWriterPolicy, log *logger.Logger) *metricFamilyWriter {
	return &metricFamilyWriter{
		store:   store,
		policy:  policy,
		handles: make(map[string]*metricFamilyHandle),
		Logger:  log,
	}
}

// countWritable reports how many series across all families could be written. Used at Check to
// confirm the endpoint exposes usable metrics, before any cycle has run.
func (w *metricFamilyWriter) countWritable(mfs prompkg.MetricFamilies) int {
	count := 0
	for _, mf := range mfs {
		if w.skipMetricFamily(mf) {
			continue
		}

		typ, ok := w.resolveFamilyType(mf)
		if !ok {
			continue
		}

		schema, ok := w.canonicalSchema(mf, typ)
		if !ok {
			continue
		}

		for _, metric := range mf.Metrics() {
			if metricIsWritable(metric, typ, schema) {
				count++
			}
		}
	}
	return count
}

func (w *metricFamilyWriter) writeMetricFamilies(mfs prompkg.MetricFamilies) int {
	written := 0
	for _, mf := range mfs {
		if w.skipMetricFamily(mf) {
			continue
		}

		typ, ok := w.resolveFamilyType(mf)
		if !ok {
			continue
		}

		handle, ok := w.ensureHandle(mf, typ)
		if !ok {
			continue
		}

		for _, metric := range mf.Metrics() {
			if w.observeMetric(handle, metric) {
				written++
			}
		}
	}

	return written
}

func (w *metricFamilyWriter) skipMetricFamily(mf *prompkg.MetricFamily) bool {
	if strings.HasSuffix(mf.Name(), "_info") {
		return true
	}
	if w.policy.maxTSPerMetric > 0 && len(mf.Metrics()) > w.policy.maxTSPerMetric {
		w.Debugf("metric '%s' num of time series (%d) > limit (%d), skipping it",
			mf.Name(), len(mf.Metrics()), w.policy.maxTSPerMetric)
		return true
	}
	return false
}

func (w *metricFamilyWriter) resolveFamilyType(mf *prompkg.MetricFamily) (commonmodel.MetricType, bool) {
	switch mf.Type() {
	case commonmodel.MetricTypeGauge,
		commonmodel.MetricTypeCounter,
		commonmodel.MetricTypeSummary,
		commonmodel.MetricTypeHistogram:
		return mf.Type(), true
	case commonmodel.MetricTypeUnknown:
		if w.policy.isFallbackTypeGauge != nil && w.policy.isFallbackTypeGauge(mf.Name()) {
			return commonmodel.MetricTypeGauge, true
		}
		if (w.policy.isFallbackTypeCounter != nil && w.policy.isFallbackTypeCounter(mf.Name())) || strings.HasSuffix(mf.Name(), "_total") {
			return commonmodel.MetricTypeCounter, true
		}
		return "", false
	default:
		return "", false
	}
}

func (w *metricFamilyWriter) ensureHandle(mf *prompkg.MetricFamily, typ commonmodel.MetricType) (*metricFamilyHandle, bool) {
	if handle, ok := w.handles[mf.Name()]; ok {
		if handle.typ != typ {
			w.Debugf("skip metric family '%s': metric type drift (%s -> %s)", mf.Name(), handle.typ, typ)
			return nil, false
		}
		return handle, true
	}

	schema, ok := deriveMetricFamilySchema(mf, typ)
	if !ok {
		return nil, false
	}

	opts := []metrix.InstrumentOption{
		metrix.WithChartFamily(getChartFamily(mf.Name())),
		metrix.WithChartPriority(getChartPriority(mf.Name())),
		metrix.WithUnit(instrumentUnit(mf.Name(), typ)),
		metrix.WithFloat(true),
		metrix.WithDescription(getChartTitle(mf.Name(), mf.Help())),
	}
	switch typ {
	case commonmodel.MetricTypeSummary:
		opts = append(opts, metrix.WithSummaryQuantiles(schema.summaryQuantiles...))
	case commonmodel.MetricTypeHistogram:
		opts = append(opts, metrix.WithHistogramBounds(schema.histogramBounds...))
	}

	handle := &metricFamilyHandle{
		name:             mf.Name(),
		typ:              typ,
		summaryQuantiles: slices.Clone(schema.summaryQuantiles),
		histogramBounds:  slices.Clone(schema.histogramBounds),
		opts:             opts,
	}
	w.handles[mf.Name()] = handle
	return handle, true
}

func (w *metricFamilyWriter) canonicalSchema(mf *prompkg.MetricFamily, typ commonmodel.MetricType) (metricFamilySchema, bool) {
	if handle, ok := w.handles[mf.Name()]; ok {
		if handle.typ != typ {
			return metricFamilySchema{}, false
		}
		return metricFamilySchema{
			summaryQuantiles: handle.summaryQuantiles,
			histogramBounds:  handle.histogramBounds,
		}, true
	}

	return deriveMetricFamilySchema(mf, typ)
}

func (w *metricFamilyWriter) observeMetric(handle *metricFamilyHandle, metric prompkg.Metric) bool {
	schema, ok := deriveMetricSchema(metric, handle.typ)
	if !ok {
		return false
	}
	if !slices.Equal(handle.summaryQuantiles, schema.summaryQuantiles) || !slices.Equal(handle.histogramBounds, schema.histogramBounds) {
		w.Debugf("skip a series of metric '%s': distribution schema drift", handle.name)
		return false
	}

	meter := w.store.Write().SnapshotMeter("").WithLabels(w.seriesLabels(metric)...)

	switch handle.typ {
	case commonmodel.MetricTypeGauge:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeGauge)
		if !ok {
			return false
		}
		meter.Gauge(handle.name, handle.opts...).Observe(value)
		return true
	case commonmodel.MetricTypeCounter:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeCounter)
		if !ok {
			return false
		}
		meter.Counter(handle.name, handle.opts...).ObserveTotal(value)
		return true
	case commonmodel.MetricTypeSummary:
		point, ok := toSummaryPoint(metric.Summary())
		if !ok {
			return false
		}
		meter.Summary(handle.name, handle.opts...).ObservePoint(point)
		return true
	case commonmodel.MetricTypeHistogram:
		point, ok := toHistogramPoint(metric.Histogram())
		if !ok {
			return false
		}
		meter.Histogram(handle.name, handle.opts...).ObservePoint(point)
		return true
	default:
		return false
	}
}

// seriesLabels converts a scraped series' labels into metrix labels, applying the configured
// label_prefix to each label key (V1 prepended "<prefix>_" to label keys).
func (w *metricFamilyWriter) seriesLabels(metric prompkg.Metric) []metrix.Label {
	lbs := metric.Labels()
	out := make([]metrix.Label, 0, len(lbs))
	for _, l := range lbs {
		key := l.Name
		if w.policy.labelPrefix != "" {
			key = w.policy.labelPrefix + "_" + l.Name
		}
		out = append(out, metrix.Label{Key: key, Value: l.Value})
	}
	return out
}

// instrumentUnit returns the chart unit metrix should carry for a family. V1 appends "/s" to summary
// quantile units (except seconds/time). chartengine autogen adds "/s" itself for the incremental
// counter/_sum routes but uses the unit as-is for the absolute summary-quantile route, so the writer
// must add it for summaries; gauges/counters/histograms pass the base unit.
func instrumentUnit(name string, typ commonmodel.MetricType) string {
	unit := getChartUnits(name)
	if typ == commonmodel.MetricTypeSummary {
		switch unit {
		case "seconds", "time":
		default:
			unit += "/s"
		}
	}
	return unit
}
