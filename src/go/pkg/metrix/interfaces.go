// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// CollectorStore is the collector-facing metrics container.
// Writes are cycle-scoped and reads are snapshot-bound.
type CollectorStore interface {
	Read(opts ...ReadOption) Reader
	Write() Writer
}

// RuntimeStore is reserved for out-of-cycle internal instrumentation.
// It is intentionally stateful-only on the write side.
type RuntimeStore interface {
	Read(opts ...ReadOption) Reader
	Write() RuntimeWriter
}

// CycleController owns collect-cycle transitions for a cycle-managed store.
// Collector code does not call these methods directly.
type CycleController interface {
	BeginCycle()
	CommitCycleSuccess()
	AbortCycle()
}

// CycleManagedStore extends CollectorStore with runtime-only cycle control.
type CycleManagedStore interface {
	CollectorStore
	CycleController() CycleController
}

// Reader exposes immutable snapshot reads.
// Read() defaults to freshness-filtered visibility; raw visibility is enabled via read options.
type Reader interface {
	Value(name string, labels Labels) (SampleValue, bool)
	Delta(name string, labels Labels) (SampleValue, bool)
	Histogram(name string, labels Labels) (HistogramPoint, bool)
	Summary(name string, labels Labels) (SummaryPoint, bool)
	StateSet(name string, labels Labels) (StateSetPoint, bool)
	SeriesMeta(name string, labels Labels) (SeriesMeta, bool)
	// MetricMeta resolves metadata by metric name in the active reader view.
	// With Read(ReadFlatten()), lookups use flattened scalar series names.
	// Example: histogram families resolve via *_bucket/*_count/*_sum names.
	MetricMeta(name string) (MetricMeta, bool)
	CollectMeta() CollectMeta
	// Family returns a scalar-only view. For non-scalar families use Histogram/Summary/StateSet,
	// or use Read(ReadFlatten()) at reader acquisition time.
	Family(name string) (FamilyView, bool)
	// ForEachByName iterates scalar series only. For non-scalar families use typed getters
	// or Read(ReadFlatten()).
	ForEachByName(name string, fn func(labels LabelView, v SampleValue))
	// ForEachSeries iterates scalar series only. For non-scalar families use typed getters
	// or Read(ReadFlatten()).
	ForEachSeries(fn func(name string, labels LabelView, v SampleValue))
	// ForEachSeriesIdentity iterates scalar series and includes stable identity/hash.
	// For non-scalar families use typed getters or Read(ReadFlatten()).
	ForEachSeriesIdentity(fn func(identity SeriesIdentity, meta SeriesMeta, name string, labels LabelView, v SampleValue))
	// ForEachMatch iterates scalar series only. For non-scalar families use typed getters
	// or Read(ReadFlatten()).
	ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue))
}

// SeriesIdentityRawIterator is an optional reader fast path that exposes
// raw canonical label slices to avoid per-series LabelView interface wrapping
// in hot iteration paths.
//
// Labels are snapshot-owned and immutable for the lifetime of that reader.
// Callers must not mutate returned label slices.
type SeriesIdentityRawIterator interface {
	ForEachSeriesIdentityRaw(fn func(identity SeriesIdentity, meta SeriesMeta, name string, labels []Label, v SampleValue))
}

// Writer is the declaration/write entrypoint for collection stores.
// Metric mode is selected via SnapshotMeter or StatefulMeter.
type Writer interface {
	SnapshotMeter(prefix string) SnapshotMeter
	StatefulMeter(prefix string) StatefulMeter
}

// RuntimeWriter is stateful-only by design.
type RuntimeWriter interface {
	StatefulMeter(prefix string) StatefulMeter
}

// SnapshotMeter declares snapshot-mode instruments under a metric-name prefix.
type SnapshotMeter interface {
	WithLabels(labels ...Label) SnapshotMeter
	WithLabelSet(labels ...LabelSet) SnapshotMeter
	// Vec binds a reusable vec label-key schema for multiple vector instruments.
	Vec(labelKeys ...string) SnapshotVecMeter
	Gauge(name string, opts ...InstrumentOption) SnapshotGauge
	Counter(name string, opts ...InstrumentOption) SnapshotCounter
	Histogram(name string, opts ...InstrumentOption) SnapshotHistogram
	Summary(name string, opts ...InstrumentOption) SnapshotSummary
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	LabelSet(labels ...Label) LabelSet
}

// SnapshotVecMeter declares snapshot vec instruments sharing one label-key schema.
type SnapshotVecMeter interface {
	Gauge(name string, opts ...InstrumentOption) SnapshotGaugeVec
	Counter(name string, opts ...InstrumentOption) SnapshotCounterVec
	Histogram(name string, opts ...InstrumentOption) SnapshotHistogramVec
	Summary(name string, opts ...InstrumentOption) SnapshotSummaryVec
	StateSet(name string, opts ...InstrumentOption) SnapshotStateSetVec
}

// StatefulMeter declares stateful-mode instruments under a metric-name prefix.
type StatefulMeter interface {
	WithLabels(labels ...Label) StatefulMeter
	WithLabelSet(labels ...LabelSet) StatefulMeter
	// Vec binds a reusable vec label-key schema for multiple vector instruments.
	Vec(labelKeys ...string) StatefulVecMeter
	Gauge(name string, opts ...InstrumentOption) StatefulGauge
	Counter(name string, opts ...InstrumentOption) StatefulCounter
	Histogram(name string, opts ...InstrumentOption) StatefulHistogram
	Summary(name string, opts ...InstrumentOption) StatefulSummary
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	LabelSet(labels ...Label) LabelSet
}

// StatefulVecMeter declares stateful vec instruments sharing one label-key schema.
type StatefulVecMeter interface {
	Gauge(name string, opts ...InstrumentOption) StatefulGaugeVec
	Counter(name string, opts ...InstrumentOption) StatefulCounterVec
	Histogram(name string, opts ...InstrumentOption) StatefulHistogramVec
	Summary(name string, opts ...InstrumentOption) StatefulSummaryVec
	StateSet(name string, opts ...InstrumentOption) StatefulStateSetVec
}

// SnapshotGauge writes sampled absolute values; last write wins in a cycle.
type SnapshotGauge interface {
	Observe(v SampleValue, labels ...LabelSet)
}

// SnapshotGaugeVec provides labeled series handles for snapshot gauges.
// It follows the Prometheus vec pattern:
// - GetWithLabelValues returns (metric, error)
// - WithLabelValues panics on invalid label values
type SnapshotGaugeVec interface {
	GetWithLabelValues(labelValues ...string) (SnapshotGauge, error)
	WithLabelValues(labelValues ...string) SnapshotGauge
}

// StatefulGauge writes maintained values:
// Set overwrites staged value, Add accumulates from committed baseline.
type StatefulGauge interface {
	Set(v SampleValue, labels ...LabelSet)
	Add(delta SampleValue, labels ...LabelSet)
}

// StatefulGaugeVec provides labeled series handles for stateful gauges.
type StatefulGaugeVec interface {
	GetWithLabelValues(labelValues ...string) (StatefulGauge, error)
	WithLabelValues(labelValues ...string) StatefulGauge
}

type SnapshotCounter interface {
	ObserveTotal(v SampleValue, labels ...LabelSet)
}

// SnapshotCounterVec provides labeled series handles for snapshot counters.
type SnapshotCounterVec interface {
	GetWithLabelValues(labelValues ...string) (SnapshotCounter, error)
	WithLabelValues(labelValues ...string) SnapshotCounter
}

type StatefulCounter interface {
	Add(delta SampleValue, labels ...LabelSet)
}

// StatefulCounterVec provides labeled series handles for stateful counters.
type StatefulCounterVec interface {
	GetWithLabelValues(labelValues ...string) (StatefulCounter, error)
	WithLabelValues(labelValues ...string) StatefulCounter
}

type SnapshotHistogram interface {
	ObservePoint(p HistogramPoint, labels ...LabelSet)
}

// SnapshotHistogramVec provides labeled series handles for snapshot histograms.
type SnapshotHistogramVec interface {
	GetWithLabelValues(labelValues ...string) (SnapshotHistogram, error)
	WithLabelValues(labelValues ...string) SnapshotHistogram
}

type StatefulHistogram interface {
	Observe(v SampleValue, labels ...LabelSet)
}

// StatefulHistogramVec provides labeled series handles for stateful histograms.
type StatefulHistogramVec interface {
	GetWithLabelValues(labelValues ...string) (StatefulHistogram, error)
	WithLabelValues(labelValues ...string) StatefulHistogram
}

type SnapshotSummary interface {
	ObservePoint(p SummaryPoint, labels ...LabelSet)
}

// SnapshotSummaryVec provides labeled series handles for snapshot summaries.
type SnapshotSummaryVec interface {
	GetWithLabelValues(labelValues ...string) (SnapshotSummary, error)
	WithLabelValues(labelValues ...string) SnapshotSummary
}

type StatefulSummary interface {
	Observe(v SampleValue, labels ...LabelSet)
}

// StatefulSummaryVec provides labeled series handles for stateful summaries.
type StatefulSummaryVec interface {
	GetWithLabelValues(labelValues ...string) (StatefulSummary, error)
	WithLabelValues(labelValues ...string) StatefulSummary
}

type StateSetInstrument interface {
	ObserveStateSet(p StateSetPoint, labels ...LabelSet)
	Enable(actives ...string)
}

// SnapshotStateSetVec provides labeled series handles for snapshot statesets.
type SnapshotStateSetVec interface {
	GetWithLabelValues(labelValues ...string) (StateSetInstrument, error)
	WithLabelValues(labelValues ...string) StateSetInstrument
}

// StatefulStateSetVec provides labeled series handles for stateful statesets.
type StatefulStateSetVec interface {
	GetWithLabelValues(labelValues ...string) (StateSetInstrument, error)
	WithLabelValues(labelValues ...string) StateSetInstrument
}
