// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// Store is the collector-facing metrics container.
// Writes are cycle-scoped and reads are snapshot-bound.
type Store interface {
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

// CycleManagedStore extends Store with runtime-only cycle control.
type CycleManagedStore interface {
	Store
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
	Family(name string) (FamilyView, bool)
	ForEachByName(name string, fn func(labels LabelView, v SampleValue))
	ForEachSeries(fn func(name string, labels LabelView, v SampleValue))
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
	Counter(name string, opts ...InstrumentOption) SnapshotCounter
	Histogram(name string, opts ...InstrumentOption) SnapshotHistogram
	Summary(name string, opts ...InstrumentOption) SnapshotSummary
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	LabelSet(labels ...Label) LabelSet
}

// StatefulMeter declares stateful-mode instruments under a metric-name prefix.
type StatefulMeter interface {
	WithLabels(labels ...Label) StatefulMeter
	WithLabelSet(labels ...LabelSet) StatefulMeter
	Gauge(name string, opts ...InstrumentOption) StatefulGauge
	Counter(name string, opts ...InstrumentOption) StatefulCounter
	Histogram(name string, opts ...InstrumentOption) StatefulHistogram
	Summary(name string, opts ...InstrumentOption) StatefulSummary
	StateSet(name string, opts ...InstrumentOption) StateSetInstrument
	LabelSet(labels ...Label) LabelSet
}

// SnapshotGauge writes sampled absolute values; last write wins in a cycle.
type SnapshotGauge interface {
	Observe(v SampleValue, labels ...LabelSet)
}

// StatefulGauge writes maintained values:
// Set overwrites staged value, Add accumulates from committed baseline.
type StatefulGauge interface {
	Set(v SampleValue, labels ...LabelSet)
	Add(delta SampleValue, labels ...LabelSet)
}

type SnapshotCounter interface {
	ObserveTotal(v SampleValue, labels ...LabelSet)
}

type StatefulCounter interface {
	Add(delta SampleValue, labels ...LabelSet)
}

type SnapshotHistogram interface {
	ObservePoint(p HistogramPoint, labels ...LabelSet)
}

type StatefulHistogram interface {
	Observe(v SampleValue, labels ...LabelSet)
}

type SnapshotSummary interface {
	ObservePoint(p SummaryPoint, labels ...LabelSet)
}

type StatefulSummary interface {
	Observe(v SampleValue, labels ...LabelSet)
}

type StateSetInstrument interface {
	ObserveStateSet(p StateSetPoint, labels ...LabelSet)
	Enable(actives ...string)
}
