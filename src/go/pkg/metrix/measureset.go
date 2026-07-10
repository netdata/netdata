// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

const MeasureSetFieldLabel = "measure_field"

// stagedMeasureSet holds one in-cycle MeasureSet sample for a single series identity.
type stagedMeasureSet struct {
	key          string
	name         string
	hostScopeKey string
	hostScope    HostScope
	labels       []Label
	labelsKey    string
	desc         *instrumentDescriptor
	values       []SampleValue
}

// snapshotMeasureSetGaugeInstrument writes sampled MeasureSet gauge points.
type snapshotMeasureSetGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
	base    []LabelSet
}

// snapshotMeasureSetCounterInstrument writes sampled MeasureSet counter totals.
type snapshotMeasureSetCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
	base    []LabelSet
}

// statefulMeasureSetGaugeInstrument writes maintained MeasureSet gauge points.
type statefulMeasureSetGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
	base    []LabelSet
}

// statefulMeasureSetCounterInstrument writes maintained MeasureSet counter deltas.
type statefulMeasureSetCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
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
		scope:   m.scope,
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
		scope:   m.scope,
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
		scope:   m.scope,
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
		scope:   m.scope,
		base:    appendLabelSets(m.sets, nil),
	}
}

func (m *snapshotMeasureSetGaugeInstrument) ObservePoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeObservePoint(m.desc, m.scope, p, appendLabelSets(m.base, labels))
}

func (m *snapshotMeasureSetGaugeInstrument) ObserveFields(fields map[string]SampleValue, labels ...LabelSet) {
	m.ObservePoint(measureSetPointFromFields(fields, m.desc.measureSet), labels...)
}

func (m *snapshotMeasureSetCounterInstrument) ObserveTotalPoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetCounterObserveTotalPoint(m.desc, m.scope, p, appendLabelSets(m.base, labels))
}

func (m *snapshotMeasureSetCounterInstrument) ObserveTotalFields(fields map[string]SampleValue, labels ...LabelSet) {
	m.ObserveTotalPoint(measureSetPointFromFields(fields, m.desc.measureSet), labels...)
}

func (m *statefulMeasureSetGaugeInstrument) SetPoint(p MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeSetPoint(m.desc, m.scope, p, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetGaugeInstrument) SetFields(fields map[string]SampleValue, labels ...LabelSet) {
	m.SetPoint(measureSetPointFromFields(fields, m.desc.measureSet), labels...)
}

func (m *statefulMeasureSetGaugeInstrument) SetField(field string, value SampleValue, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeSetField(m.desc, m.scope, field, value, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetGaugeInstrument) AddPoint(delta MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetGaugeAddPoint(m.desc, m.scope, delta, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetGaugeInstrument) AddFields(delta map[string]SampleValue, labels ...LabelSet) {
	m.AddPoint(measureSetPointFromFields(delta, m.desc.measureSet), labels...)
}

func (m *statefulMeasureSetGaugeInstrument) AddField(field string, delta SampleValue, labels ...LabelSet) {
	m.AddPoint(singleMeasureSetPoint(field, delta, m.desc.measureSet), labels...)
}

func (m *statefulMeasureSetCounterInstrument) AddPoint(delta MeasureSetPoint, labels ...LabelSet) {
	m.backend.recordMeasureSetCounterAddPoint(m.desc, m.scope, delta, appendLabelSets(m.base, labels))
}

func (m *statefulMeasureSetCounterInstrument) AddFields(delta map[string]SampleValue, labels ...LabelSet) {
	m.AddPoint(measureSetPointFromFields(delta, m.desc.measureSet), labels...)
}

func (m *statefulMeasureSetCounterInstrument) AddField(field string, delta SampleValue, labels ...LabelSet) {
	m.AddPoint(singleMeasureSetPoint(field, delta, m.desc.measureSet), labels...)
}
