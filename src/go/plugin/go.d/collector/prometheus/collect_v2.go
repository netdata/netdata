// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

func (c *Collector) checkV2() (int, error) {
	mfs, runtime, err := c.scrapeMetricFamiliesWithRuntime(true)
	if err != nil {
		return 0, err
	}
	c.runtime = runtime

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
		return 0, nil
	}

	store := metrix.NewCollectorStore()
	mw := newMetricFamilyWriter(store, c)

	return mw.CountWritable(mfs), nil
}

func (c *Collector) collectV2(context.Context) error {
	mfs, err := c.scrapeMetricFamilies()
	if err != nil {
		return err
	}

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
		return nil
	}

	written := c.mw.WriteMetricFamilies(mfs)
	if written == 0 {
		c.Warningf("endpoint '%s' produced 0 writable metric series", c.URL)
	}

	return nil
}

func (c *Collector) scrapeMetricFamilies() (prompkg.MetricFamilies, error) {
	mfs, _, err := c.scrapeMetricFamiliesWithRuntime(false)
	return mfs, err
}

func (c *Collector) scrapeMetricFamiliesWithRuntime(checking bool) (prompkg.MetricFamilies, *collectorRuntime, error) {
	if c.pipe == nil {
		return nil, nil, fmt.Errorf("prometheus pipeline is not initialized")
	}

	batch, err := c.pipe.CollectSamples()
	if err != nil {
		return nil, nil, err
	}

	mfs, runtime, err := c.processScrapeBatch(batch, checking)
	if err != nil {
		return nil, nil, err
	}

	if c.ExpectedPrefix != "" {
		if !hasPrefix(mfs, c.ExpectedPrefix) {
			return nil, nil, fmt.Errorf("'%s' metrics have no expected prefix (%s)", c.URL, c.ExpectedPrefix)
		}
		c.ExpectedPrefix = ""
	}

	if c.MaxTS > 0 {
		if n := calcMetrics(mfs); n > c.MaxTS {
			return nil, nil, fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.MaxTS)
		}
		c.MaxTS = 0
	}

	return mfs, runtime, nil
}

type metricFamilyWriter struct {
	store   metrix.CollectorStore
	coll    *Collector
	handles map[string]*metricFamilyHandle
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

func newMetricFamilyWriter(store metrix.CollectorStore, coll *Collector) *metricFamilyWriter {
	return &metricFamilyWriter{
		store:   store,
		coll:    coll,
		handles: make(map[string]*metricFamilyHandle),
	}
}

func (w *metricFamilyWriter) CountWritable(mfs prompkg.MetricFamilies) int {
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

func (w *metricFamilyWriter) WriteMetricFamilies(mfs prompkg.MetricFamilies) int {
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
				w.warnf("skip metric family '%s': %v", mf.Name(), err)
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
	if w.coll != nil && w.coll.MaxTSPerMetric > 0 && len(mf.Metrics()) > w.coll.MaxTSPerMetric {
		w.debugf(
			"metric '%s' num of time series (%d) > limit (%d), skipping it",
			mf.Name(),
			len(mf.Metrics()),
			w.coll.MaxTSPerMetric,
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
		if w.coll != nil && w.coll.isFallbackTypeGauge(mf.Name()) {
			return commonmodel.MetricTypeGauge, true
		}
		if (w.coll != nil && w.coll.isFallbackTypeCounter(mf.Name())) || strings.HasSuffix(mf.Name(), "_total") {
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
			w.warnf("skip metric family '%s': metric type drift (%s -> %s)", mf.Name(), handle.typ, typ)
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
		w.warnf("skip metric family '%s': unsupported type %s", mf.Name(), typ)
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
		s := metric.Summary()
		point, ok := toSummaryPoint(s)
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
		h := metric.Histogram()
		point, ok := toHistogramPoint(h)
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

type metricFamilySchema struct {
	labelKeys        []string
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
	schema := metricFamilySchema{labelKeys: labelKeys(metric.Labels())}

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

func metricIsWritable(metric prompkg.Metric, typ commonmodel.MetricType, schema metricFamilySchema) bool {
	metricSchema, ok := deriveMetricSchema(metric, typ)
	if !ok {
		return false
	}
	if !equalStrings(schema.labelKeys, metricSchema.labelKeys) {
		return false
	}
	if !equalFloat64s(schema.summaryQuantiles, metricSchema.summaryQuantiles) {
		return false
	}
	if !equalFloat64s(schema.histogramBounds, metricSchema.histogramBounds) {
		return false
	}
	return true
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
		if !isFinite(q.Quantile()) || q.Quantile() < 0 || q.Quantile() > 1 || !isFinite(q.Value()) {
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

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].UpperBound < buckets[j].UpperBound })
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
	sort.Float64s(qs)
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
	sort.Float64s(bounds)
	for i := 1; i < len(bounds); i++ {
		if bounds[i] <= bounds[i-1] {
			return nil, false
		}
	}
	return bounds, true
}

func labelKeys(lbs promlabels.Labels) []string {
	keys := make([]string, 0, len(lbs))
	for _, label := range lbs {
		keys = append(keys, label.Name)
	}
	return keys
}

func labelValues(lbs promlabels.Labels, keys []string) ([]string, error) {
	if len(lbs) != len(keys) {
		return nil, fmt.Errorf("label key count mismatch")
	}

	values := make([]string, len(keys))
	for i, key := range keys {
		label := lbs[i]
		if label.Name != key {
			return nil, fmt.Errorf("label key mismatch")
		}
		values[i] = label.Value
	}
	return values, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalFloat64s(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (w *metricFamilyWriter) warnf(format string, args ...any) {
	if w.coll != nil {
		w.coll.Warningf(format, args...)
	}
}

func (w *metricFamilyWriter) debugf(format string, args ...any) {
	if w.coll != nil {
		w.coll.Debugf(format, args...)
	}
}
