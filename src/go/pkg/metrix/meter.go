// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

type writeView struct {
	backend meterBackend
}

type snapshotMeter struct {
	backend meterBackend
	prefix  string
	sets    []LabelSet
}

type snapshotVecMeter struct {
	meter     *snapshotMeter
	labelKeys []string
}

type statefulMeter struct {
	backend meterBackend
	prefix  string
	sets    []LabelSet
}

type statefulVecMeter struct {
	meter     *statefulMeter
	labelKeys []string
}

// SnapshotMeter returns a snapshot-mode declaration/write scope.
func (w *writeView) SnapshotMeter(prefix string) SnapshotMeter {
	return &snapshotMeter{backend: w.backend, prefix: prefix}
}

// StatefulMeter returns a stateful-mode declaration/write scope.
func (w *writeView) StatefulMeter(prefix string) StatefulMeter {
	return &statefulMeter{backend: w.backend, prefix: prefix}
}

func (m *snapshotMeter) WithLabels(labels ...Label) SnapshotMeter {
	set := m.LabelSet(labels...)
	return m.WithLabelSet(set)
}

func (m *snapshotMeter) WithLabelSet(labels ...LabelSet) SnapshotMeter {
	for _, ls := range labels {
		if ls.set == nil || ls.set.owner != m.backend {
			panic(errForeignLabelSet)
		}
	}
	return &snapshotMeter{backend: m.backend, prefix: m.prefix, sets: appendLabelSets(m.sets, labels)}
}

func (m *snapshotMeter) Vec(labelKeys ...string) SnapshotVecMeter {
	return &snapshotVecMeter{
		meter:     m,
		labelKeys: append([]string(nil), labelKeys...),
	}
}

func (m *snapshotMeter) LabelSet(labels ...Label) LabelSet {
	return m.backend.compileLabelSet(labels...)
}

func (m *statefulMeter) WithLabels(labels ...Label) StatefulMeter {
	set := m.LabelSet(labels...)
	return m.WithLabelSet(set)
}

func (m *statefulMeter) WithLabelSet(labels ...LabelSet) StatefulMeter {
	for _, ls := range labels {
		if ls.set == nil || ls.set.owner != m.backend {
			panic(errForeignLabelSet)
		}
	}
	return &statefulMeter{backend: m.backend, prefix: m.prefix, sets: appendLabelSets(m.sets, labels)}
}

func (m *statefulMeter) Vec(labelKeys ...string) StatefulVecMeter {
	return &statefulVecMeter{
		meter:     m,
		labelKeys: append([]string(nil), labelKeys...),
	}
}

func (m *statefulMeter) LabelSet(labels ...Label) LabelSet {
	return m.backend.compileLabelSet(labels...)
}

func (m *snapshotVecMeter) Gauge(name string, opts ...InstrumentOption) SnapshotGaugeVec {
	return m.meter.GaugeVec(name, m.labelKeys, opts...)
}

func (m *snapshotVecMeter) Counter(name string, opts ...InstrumentOption) SnapshotCounterVec {
	return m.meter.CounterVec(name, m.labelKeys, opts...)
}

func (m *snapshotVecMeter) Histogram(name string, opts ...InstrumentOption) SnapshotHistogramVec {
	return m.meter.HistogramVec(name, m.labelKeys, opts...)
}

func (m *snapshotVecMeter) Summary(name string, opts ...InstrumentOption) SnapshotSummaryVec {
	return m.meter.SummaryVec(name, m.labelKeys, opts...)
}

func (m *snapshotVecMeter) StateSet(name string, opts ...InstrumentOption) SnapshotStateSetVec {
	return m.meter.StateSetVec(name, m.labelKeys, opts...)
}

func (m *statefulVecMeter) Gauge(name string, opts ...InstrumentOption) StatefulGaugeVec {
	return m.meter.GaugeVec(name, m.labelKeys, opts...)
}

func (m *statefulVecMeter) Counter(name string, opts ...InstrumentOption) StatefulCounterVec {
	return m.meter.CounterVec(name, m.labelKeys, opts...)
}

func (m *statefulVecMeter) Histogram(name string, opts ...InstrumentOption) StatefulHistogramVec {
	return m.meter.HistogramVec(name, m.labelKeys, opts...)
}

func (m *statefulVecMeter) Summary(name string, opts ...InstrumentOption) StatefulSummaryVec {
	return m.meter.SummaryVec(name, m.labelKeys, opts...)
}

func (m *statefulVecMeter) StateSet(name string, opts ...InstrumentOption) StatefulStateSetVec {
	return m.meter.StateSetVec(name, m.labelKeys, opts...)
}

// metricName composes meter prefix with instrument local name.
func metricName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}
