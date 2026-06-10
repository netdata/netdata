// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	commonmodel "github.com/prometheus/common/model"
)

// seriesCacheRetentionCycles bounds the per-series instrument cache: a cached handle not observed for
// this many successful cycles is evicted. This value mirrors two other retention windows that are NOT
// compiler-linked and MUST be kept in agreement: metrix's default store retention (so a cached handle
// lives as long as the series it writes) and the chart template's expiry (chartExpireAfterCycles, so a
// chart is not removed before the series feeding it). The cache also stays bounded under label churn.
const seriesCacheRetentionCycles = 10

type metricFamilyWriterPolicy struct {
	maxTSPerMetric        int
	isFallbackTypeGauge   matcher.Matcher
	isFallbackTypeCounter matcher.Matcher
}

type metricFamilyWriter struct {
	store   metrix.CollectorStore
	policy  metricFamilyWriterPolicy
	handles map[string]*metricFamilyHandle
	cycle   uint64
	*logger.Logger
}

// cachedInstrument is a per-series instrument handle plus the last cycle it was observed, so handles
// for series that stop appearing in scrapes can be evicted.
type cachedInstrument[T any] struct {
	inst     T
	lastSeen uint64
}

// metricFamilyHandle caches, per metric name, the canonical distribution schema, instrument options,
// and the per-series instrument handles. The family handle is created once per name and KEPT for the
// job's lifetime on purpose: metrix registers an instrument descriptor per name permanently (no
// unregister API), so if a name reappeared with a changed contract (kind, summary quantiles, or
// histogram bounds) and the writer re-registered it, metrix would panic on the mismatch. Keeping the
// handle lets ensureHandle detect that drift and skip it instead of re-registering.
//
// The per-series instrument handles inside ARE evicted: they are reused across cycles (skipping
// per-series instrument re-resolution) and dropped once a series goes unobserved for
// seriesCacheRetentionCycles, so the cache stays bounded under label-value churn — unlike a metrix
// vec, whose internal handle cache is unbounded. A family may mix label-key sets; each series is
// cached and written by its own full label tuple. Per-name state (this handle plus the metrix
// descriptor) is bounded by metric-name cardinality; metric-NAME churn (a Prometheus anti-pattern)
// grows it — an accepted metrix-store limit.
type metricFamilyHandle struct {
	name             string
	typ              commonmodel.MetricType
	summaryQuantiles []float64
	histogramBounds  []float64
	opts             []metrix.InstrumentOption

	gauges     map[string]*cachedInstrument[metrix.SnapshotGauge]
	counters   map[string]*cachedInstrument[metrix.SnapshotCounter]
	summaries  map[string]*cachedInstrument[metrix.SnapshotSummary]
	histograms map[string]*cachedInstrument[metrix.SnapshotHistogram]
}

func newMetricFamilyWriter(store metrix.CollectorStore, policy metricFamilyWriterPolicy, log *logger.Logger) *metricFamilyWriter {
	// A family with no configured fallback matcher uses a never-matching matcher, so
	// resolveFamilyType can call MatchString unconditionally (no per-cycle nil check).
	if policy.isFallbackTypeGauge == nil {
		policy.isFallbackTypeGauge = matcher.FALSE()
	}
	if policy.isFallbackTypeCounter == nil {
		policy.isFallbackTypeCounter = matcher.FALSE()
	}
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

		schema, ok := deriveMetricFamilySchema(mf, typ)
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
	w.cycle++

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

	w.evictStaleSeries()
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
		if w.policy.isFallbackTypeGauge.MatchString(mf.Name()) {
			return commonmodel.MetricTypeGauge, true
		}
		if w.policy.isFallbackTypeCounter.MatchString(mf.Name()) || strings.HasSuffix(mf.Name(), "_total") {
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

	handle := &metricFamilyHandle{
		name:             mf.Name(),
		typ:              typ,
		summaryQuantiles: slices.Clone(schema.summaryQuantiles),
		histogramBounds:  slices.Clone(schema.histogramBounds),
		opts:             opts,
	}

	switch typ {
	case commonmodel.MetricTypeGauge:
		handle.gauges = make(map[string]*cachedInstrument[metrix.SnapshotGauge])
	case commonmodel.MetricTypeCounter:
		handle.counters = make(map[string]*cachedInstrument[metrix.SnapshotCounter])
	case commonmodel.MetricTypeSummary:
		handle.opts = append(handle.opts, metrix.WithSummaryQuantiles(schema.summaryQuantiles...))
		handle.summaries = make(map[string]*cachedInstrument[metrix.SnapshotSummary])
	case commonmodel.MetricTypeHistogram:
		handle.opts = append(handle.opts, metrix.WithHistogramBounds(schema.histogramBounds...))
		handle.histograms = make(map[string]*cachedInstrument[metrix.SnapshotHistogram])
	}

	w.handles[mf.Name()] = handle
	return handle, true
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

	sig := w.seriesSig(metric)

	switch handle.typ {
	case commonmodel.MetricTypeGauge:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeGauge)
		if !ok {
			return false
		}
		inst := getOrCreateInstrument(handle.gauges, sig, w.cycle, func() metrix.SnapshotGauge {
			return w.store.Write().SnapshotMeter("").WithLabels(w.seriesLabels(metric)...).Gauge(handle.name, handle.opts...)
		})
		inst.Observe(value)
		return true
	case commonmodel.MetricTypeCounter:
		value, ok := metricScalarValue(metric, commonmodel.MetricTypeCounter)
		if !ok {
			return false
		}
		inst := getOrCreateInstrument(handle.counters, sig, w.cycle, func() metrix.SnapshotCounter {
			return w.store.Write().SnapshotMeter("").WithLabels(w.seriesLabels(metric)...).Counter(handle.name, handle.opts...)
		})
		inst.ObserveTotal(value)
		return true
	case commonmodel.MetricTypeSummary:
		point, ok := toSummaryPoint(metric.Summary())
		if !ok {
			return false
		}
		inst := getOrCreateInstrument(handle.summaries, sig, w.cycle, func() metrix.SnapshotSummary {
			return w.store.Write().SnapshotMeter("").WithLabels(w.seriesLabels(metric)...).Summary(handle.name, handle.opts...)
		})
		inst.ObservePoint(point)
		return true
	case commonmodel.MetricTypeHistogram:
		point, ok := toHistogramPoint(metric.Histogram())
		if !ok {
			return false
		}
		inst := getOrCreateInstrument(handle.histograms, sig, w.cycle, func() metrix.SnapshotHistogram {
			return w.store.Write().SnapshotMeter("").WithLabels(w.seriesLabels(metric)...).Histogram(handle.name, handle.opts...)
		})
		inst.ObservePoint(point)
		return true
	default:
		return false
	}
}

// getOrCreateInstrument returns the cached instrument handle for a series signature, creating and
// caching it on first use, and stamps it as observed in the current cycle.
func getOrCreateInstrument[T any](m map[string]*cachedInstrument[T], sig string, cycle uint64, create func() T) T {
	e, ok := m[sig]
	if !ok {
		e = &cachedInstrument[T]{inst: create()}
		m[sig] = e
	}
	e.lastSeen = cycle
	return e.inst
}

// evictStaleSeries drops cached instrument handles for series not observed within the retention
// window, keeping the cache bounded under label-value churn.
func (w *metricFamilyWriter) evictStaleSeries() {
	for _, h := range w.handles {
		switch h.typ {
		case commonmodel.MetricTypeGauge:
			evictStaleInstruments(h.gauges, w.cycle)
		case commonmodel.MetricTypeCounter:
			evictStaleInstruments(h.counters, w.cycle)
		case commonmodel.MetricTypeSummary:
			evictStaleInstruments(h.summaries, w.cycle)
		case commonmodel.MetricTypeHistogram:
			evictStaleInstruments(h.histograms, w.cycle)
		}
	}
}

func evictStaleInstruments[T any](m map[string]*cachedInstrument[T], cycle uint64) {
	for sig, e := range m {
		if e.lastSeen+seriesCacheRetentionCycles <= cycle {
			delete(m, sig)
		}
	}
}

// seriesSig builds a collision-safe key identifying a scraped series by its label tuple.
// Prometheus labels are sorted by name, so the key is stable for a given series.
func (w *metricFamilyWriter) seriesSig(metric prompkg.Metric) string {
	lbs := metric.Labels()
	if len(lbs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range lbs {
		b.WriteString(strconv.Itoa(len(l.Name)))
		b.WriteByte(':')
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(len(l.Value)))
		b.WriteByte(':')
		b.WriteString(l.Value)
		b.WriteByte('\xff')
	}
	return b.String()
}

// seriesLabels converts a scraped series' labels into metrix labels.
func (w *metricFamilyWriter) seriesLabels(metric prompkg.Metric) []metrix.Label {
	lbs := metric.Labels()
	out := make([]metrix.Label, 0, len(lbs))
	for _, l := range lbs {
		out = append(out, metrix.Label{Key: l.Name, Value: l.Value})
	}
	return out
}
