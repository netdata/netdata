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

type statefulMeter struct {
	backend meterBackend
	prefix  string
	sets    []LabelSet
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

func (m *snapshotMeter) Histogram(name string, opts ...InstrumentOption) SnapshotHistogram {
	_ = name
	_ = opts
	return snapshotHistogramNotImplemented{}
}

func (m *snapshotMeter) Summary(name string, opts ...InstrumentOption) SnapshotSummary {
	_ = name
	_ = opts
	return snapshotSummaryNotImplemented{}
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

func (m *statefulMeter) Histogram(name string, opts ...InstrumentOption) StatefulHistogram {
	_ = name
	_ = opts
	return statefulHistogramNotImplemented{}
}

func (m *statefulMeter) Summary(name string, opts ...InstrumentOption) StatefulSummary {
	_ = name
	_ = opts
	return statefulSummaryNotImplemented{}
}

func (m *statefulMeter) LabelSet(labels ...Label) LabelSet {
	return m.backend.compileLabelSet(labels...)
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
