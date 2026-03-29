// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	commonmodel "github.com/prometheus/common/model"
)

type metricFamilyWriterPolicy struct {
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

type metricFamilyHandle struct {
	name             string
	typ              commonmodel.MetricType
	labelKeys        []string
	summaryQuantiles []float64
	histogramBounds  []float64

	gaugeVec     metrix.SnapshotGaugeVec
	counterVec   metrix.SnapshotCounterVec
	summaryVec   metrix.SnapshotSummaryVec
	histogramVec metrix.SnapshotHistogramVec
}

func newMetricFamilyWriter(store metrix.CollectorStore, policy metricFamilyWriterPolicy, log *logger.Logger) *metricFamilyWriter {
	return &metricFamilyWriter{
		store:   store,
		policy:  policy,
		handles: make(map[string]*metricFamilyHandle),
		Logger:  log,
	}
}

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

		schema, ok := w.canonicalMetricFamilySchema(mf, typ)
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
			ok, err := w.observeMetric(handle, metric)
			if err != nil {
				w.Warningf("skip metric family '%s': %v", mf.Name(), err)
				continue
			}
			if ok {
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
		w.Debugf(
			"metric '%s' num of time series (%d) > limit (%d), skipping it",
			mf.Name(),
			len(mf.Metrics()),
			w.policy.maxTSPerMetric,
		)
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
			w.Warningf("skip metric family '%s': metric type drift (%s -> %s)", mf.Name(), handle.typ, typ)
			return nil, false
		}
		return handle, true
	}

	schema, ok := deriveMetricFamilySchema(mf, typ)
	if !ok {
		return nil, false
	}

	vec := w.store.Write().SnapshotMeter("").Vec(schema.labelKeys...)
	opts := []metrix.InstrumentOption{
		metrix.WithChartFamily(getChartFamily(mf.Name())),
		metrix.WithChartPriority(getChartPriority(mf.Name())),
		metrix.WithUnit(getChartUnits(mf.Name())),
		metrix.WithFloat(true),
	}
	if help := strings.TrimSpace(mf.Help()); help != "" {
		opts = append(opts, metrix.WithDescription(help))
	}

	handle := &metricFamilyHandle{
		name:             mf.Name(),
		typ:              typ,
		labelKeys:        append([]string(nil), schema.labelKeys...),
		summaryQuantiles: append([]float64(nil), schema.summaryQuantiles...),
		histogramBounds:  append([]float64(nil), schema.histogramBounds...),
	}

	switch typ {
	case commonmodel.MetricTypeGauge:
		handle.gaugeVec = vec.Gauge(mf.Name(), opts...)
	case commonmodel.MetricTypeCounter:
		handle.counterVec = vec.Counter(mf.Name(), opts...)
	case commonmodel.MetricTypeSummary:
		handle.summaryVec = vec.Summary(mf.Name(), append(opts, metrix.WithSummaryQuantiles(schema.summaryQuantiles...))...)
	case commonmodel.MetricTypeHistogram:
		handle.histogramVec = vec.Histogram(mf.Name(), append(opts, metrix.WithHistogramBounds(schema.histogramBounds...))...)
	default:
		w.Warningf("skip metric family '%s': unsupported type %s", mf.Name(), typ)
		return nil, false
	}

	w.handles[mf.Name()] = handle
	return handle, true
}

func (w *metricFamilyWriter) canonicalMetricFamilySchema(mf *prompkg.MetricFamily, typ commonmodel.MetricType) (metricFamilySchema, bool) {
	if handle, ok := w.handles[mf.Name()]; ok {
		if handle.typ != typ {
			return metricFamilySchema{}, false
		}
		return metricFamilySchema{
			labelKeys:        append([]string(nil), handle.labelKeys...),
			summaryQuantiles: append([]float64(nil), handle.summaryQuantiles...),
			histogramBounds:  append([]float64(nil), handle.histogramBounds...),
		}, true
	}

	return deriveMetricFamilySchema(mf, typ)
}

func (w *metricFamilyWriter) observeMetric(handle *metricFamilyHandle, metric prompkg.Metric) (bool, error) {
	schema, ok := deriveMetricSchema(metric, handle.typ)
	if !ok {
		return false, nil
	}
	if !equalStrings(handle.labelKeys, schema.labelKeys) {
		return false, nil
	}
	if !equalFloat64s(handle.summaryQuantiles, schema.summaryQuantiles) {
		return false, nil
	}
	if !equalFloat64s(handle.histogramBounds, schema.histogramBounds) {
		return false, nil
	}

	values, err := labelValues(metric.Labels(), handle.labelKeys)
	if err != nil {
		return false, err
	}

	switch handle.typ {
	case commonmodel.MetricTypeGauge:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeGauge)
		if !ok {
			return false, nil
		}
		inst, err := handle.gaugeVec.GetWithLabelValues(values...)
		if err != nil {
			return false, err
		}
		inst.Observe(value)
		return true, nil
	case commonmodel.MetricTypeCounter:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeCounter)
		if !ok {
			return false, nil
		}
		inst, err := handle.counterVec.GetWithLabelValues(values...)
		if err != nil {
			return false, err
		}
		inst.ObserveTotal(value)
		return true, nil
	case commonmodel.MetricTypeSummary:
		point, ok := toSummaryPoint(metric.Summary())
		if !ok {
			return false, nil
		}
		inst, err := handle.summaryVec.GetWithLabelValues(values...)
		if err != nil {
			return false, err
		}
		inst.ObservePoint(point)
		return true, nil
	case commonmodel.MetricTypeHistogram:
		point, ok := toHistogramPoint(metric.Histogram())
		if !ok {
			return false, nil
		}
		inst, err := handle.histogramVec.GetWithLabelValues(values...)
		if err != nil {
			return false, err
		}
		inst.ObservePoint(point)
		return true, nil
	default:
		return false, nil
	}
}
