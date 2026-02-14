// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// CollectorStore is the collector-facing metrics container.
// Writes are cycle-scoped and reads are snapshot-bound.
type CollectorStore interface {
	Read() Reader
	ReadRaw() Reader
	Write() Writer
}

// RuntimeStore is reserved for out-of-cycle internal instrumentation.
// It is intentionally stateful-only on the write side.
type RuntimeStore interface {
	Read() Reader
	ReadRaw() Reader
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
// Read() and ReadRaw() differ only by freshness visibility policy.
type Reader interface {
	Value(name string, labels Labels) (SampleValue, bool)
	Delta(name string, labels Labels) (SampleValue, bool)
	Histogram(name string, labels Labels) (HistogramPoint, bool)
	Summary(name string, labels Labels) (SummaryPoint, bool)
	StateSet(name string, labels Labels) (StateSetPoint, bool)
	SeriesMeta(name string, labels Labels) (SeriesMeta, bool)
	CollectMeta() CollectMeta
	Flatten() Reader
	// Family returns a scalar-only view. For non-scalar families use Histogram/Summary/StateSet,
	// or call Flatten() first and iterate flattened scalar series.
	Family(name string) (FamilyView, bool)
	// ForEachByName iterates scalar series only. For non-scalar families use typed getters
	// or Flatten() first.
	ForEachByName(name string, fn func(labels LabelView, v SampleValue))
	// ForEachSeries iterates scalar series only. For non-scalar families use typed getters
	// or Flatten() first.
	ForEachSeries(fn func(name string, labels LabelView, v SampleValue))
	// ForEachSeriesIdentity iterates scalar series and includes stable identity/hash.
	// For non-scalar families use typed getters or Flatten() first.
	ForEachSeriesIdentity(fn func(identity SeriesIdentity, name string, labels LabelView, v SampleValue))
	// ForEachMatch iterates scalar series only. For non-scalar families use typed getters
	// or Flatten() first.
	ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue))
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
	Gauge(name string, opts ...InstrumentOption) SnapshotGauge
	GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotGaugeVec
	Counter(name string, opts ...InstrumentOption) SnapshotCounter
	CounterVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotCounterVec
	Histogram(name string, opts ...InstrumentOption) SnapshotHistogram
	HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotHistogramVec
	Summary(name string, opts ...InstrumentOption) SnapshotSummary
	SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotSummaryVec
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotStateSetVec
	LabelSet(labels ...Label) LabelSet
}

// StatefulMeter declares stateful-mode instruments under a metric-name prefix.
type StatefulMeter interface {
	WithLabels(labels ...Label) StatefulMeter
	WithLabelSet(labels ...LabelSet) StatefulMeter
	Gauge(name string, opts ...InstrumentOption) StatefulGauge
	GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulGaugeVec
	Counter(name string, opts ...InstrumentOption) StatefulCounter
	CounterVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulCounterVec
	Histogram(name string, opts ...InstrumentOption) StatefulHistogram
	HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulHistogramVec
	Summary(name string, opts ...InstrumentOption) StatefulSummary
	SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulSummaryVec
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulStateSetVec
	LabelSet(labels ...Label) LabelSet
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
