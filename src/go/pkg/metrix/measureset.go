// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// snapshotMeasureSetGaugeInstrument writes sampled MeasureSet gauge points.
type snapshotMeasureSetGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// snapshotMeasureSetCounterInstrument writes sampled MeasureSet counter totals.
type snapshotMeasureSetCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulMeasureSetGaugeInstrument writes maintained MeasureSet gauge points.
type statefulMeasureSetGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulMeasureSetCounterInstrument writes maintained MeasureSet counter deltas.
type statefulMeasureSetCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

func appendMeasureSetSemantics(opts []InstrumentOption, semantics MeasureSetSemantics) []InstrumentOption {
	out := make([]InstrumentOption, 0, len(opts)+1)
	out = append(out, withMeasureSetSemantics(semantics))
	out = append(out, opts...)
	return out
}

// MeasureSetGauge declares or reuses a snapshot MeasureSet with gauge semantics.
func (m *snapshotMeter) MeasureSetGauge(name string, opts ...InstrumentOption) SnapshotMeasureSetGauge {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindMeasureSet, modeSnapshot, appendMeasureSetSemantics(opts, MeasureSetSemanticsGauge)...)
	if err != nil {
		panic(err)
	}
	return &snapshotMeasureSetGaugeInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// MeasureSetCounter declares or reuses a snapshot MeasureSet with counter semantics.
func (m *snapshotMeter) MeasureSetCounter(name string, opts ...InstrumentOption) SnapshotMeasureSetCounter {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindMeasureSet, modeSnapshot, appendMeasureSetSemantics(opts, MeasureSetSemanticsCounter)...)
	if err != nil {
		panic(err)
	}
	return &snapshotMeasureSetCounterInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// MeasureSetGauge declares or reuses a stateful MeasureSet with gauge semantics.
func (m *statefulMeter) MeasureSetGauge(name string, opts ...InstrumentOption) StatefulMeasureSetGauge {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindMeasureSet, modeStateful, appendMeasureSetSemantics(opts, MeasureSetSemanticsGauge)...)
	if err != nil {
		panic(err)
	}
	return &statefulMeasureSetGaugeInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// MeasureSetCounter declares or reuses a stateful MeasureSet with counter semantics.
func (m *statefulMeter) MeasureSetCounter(name string, opts ...InstrumentOption) StatefulMeasureSetCounter {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindMeasureSet, modeStateful, appendMeasureSetSemantics(opts, MeasureSetSemanticsCounter)...)
	if err != nil {
		panic(err)
	}
	return &statefulMeasureSetCounterInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

func (m *snapshotMeasureSetGaugeInstrument) ObservePoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeObservePoint(m.desc, p, appendLabelSets(m.base, labels))
}

func (m *snapshotMeasureSetCounterInstrument) ObserveTotalPoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetCounterObserveTotalPoint(m.desc, p, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetGaugeInstrument) SetPoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeSetPoint(m.desc, p, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetGaugeInstrument) AddPoint(delta MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeAddPoint(m.desc, delta, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetCounterInstrument) AddPoint(delta MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetCounterAddPoint(m.desc, delta, appendLabelSets(m.base, labels))
}

func (c *storeCore) recordMeasureSetGaugeObservePoint(_ *instrumentDescriptor, _ MeasureSetPoint, _ []LabelSet) {
	panic(errMeasureSetPending)
}

func (c *storeCore) recordMeasureSetGaugeSetPoint(_ *instrumentDescriptor, _ MeasureSetPoint, _ []LabelSet) {
	panic(errMeasureSetPending)
}

func (c *storeCore) recordMeasureSetGaugeAddPoint(_ *instrumentDescriptor, _ MeasureSetPoint, _ []LabelSet) {
	panic(errMeasureSetPending)
}

func (c *storeCore) recordMeasureSetCounterObserveTotalPoint(_ *instrumentDescriptor, _ MeasureSetPoint, _ []LabelSet) {
	panic(errMeasureSetPending)
}

func (c *storeCore) recordMeasureSetCounterAddPoint(_ *instrumentDescriptor, _ MeasureSetPoint, _ []LabelSet) {
	panic(errMeasureSetPending)
}
