// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// stagedMeasureSet holds one in-cycle MeasureSet sample for a single series identity.
type stagedMeasureSet struct {
	key       string
	name      string
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	values    []SampleValue
}

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

func normalizeMeasureSetPoint(point MeasureSetPoint, schema *measureSetSchema) []SampleValue {
	if schema == nil {
		panic(errMeasureSetSchema)
	}
	if len(point.Values) != len(schema.fields) {
		panic(errMeasureSetPoint)
	}

	values := make([]SampleValue, len(point.Values))
	for i, v := range point.Values {
		mustFiniteSample(v)
		values[i] = v
	}
	return values
}

func normalizeMeasureSetCounterDelta(delta MeasureSetPoint, schema *measureSetSchema) []SampleValue {
	values := normalizeMeasureSetPoint(delta, schema)
	for _, v := range values {
		if v < 0 {
			panic(errCounterNegativeDelta)
		}
	}
	return values
}

func (c *storeCore) recordMeasureSetGaugeObservePoint(desc *instrumentDescriptor, point MeasureSetPoint, sets []LabelSet) {
	c.recordMeasureSetGaugeSetPoint(desc, point, sets)
}

func (c *storeCore) recordMeasureSetGaugeSetPoint(desc *instrumentDescriptor, point MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetPoint(point, schema)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errMeasureSetLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.measureSetGauges[key]
	if !ok {
		entry = &stagedMeasureSet{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.measureSetGauges[key] = entry
	}
	entry.values = append(entry.values[:0], values...)
}

func (c *storeCore) recordMeasureSetGaugeAddPoint(desc *instrumentDescriptor, delta MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetPoint(delta, schema)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errMeasureSetLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.measureSetGauges[key]
	if !ok {
		baseline := make([]SampleValue, len(schema.fields))
		if existing := c.snapshot.Load().series[key]; existing != nil {
			baseline = append(baseline[:0], existing.measureSetValues...)
			if len(baseline) != len(schema.fields) {
				baseline = make([]SampleValue, len(schema.fields))
			}
		}
		entry = &stagedMeasureSet{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
			values:    baseline,
		}
		c.active.measureSetGauges[key] = entry
	}
	for i, deltaValue := range values {
		entry.values[i] += deltaValue
	}
}

func (c *storeCore) recordMeasureSetCounterObserveTotalPoint(desc *instrumentDescriptor, point MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsCounter {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetPoint(point, schema)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errMeasureSetLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.measureSetCounters[key]
	if !ok {
		entry = &stagedMeasureSet{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.measureSetCounters[key] = entry
	}
	entry.values = append(entry.values[:0], values...)
}

func (c *storeCore) recordMeasureSetCounterAddPoint(desc *instrumentDescriptor, delta MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsCounter {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetCounterDelta(delta, schema)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errMeasureSetLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.measureSetCounters[key]
	if !ok {
		baseline := make([]SampleValue, len(schema.fields))
		if existing := c.snapshot.Load().series[key]; existing != nil {
			baseline = append(baseline[:0], existing.measureSetValues...)
			if len(baseline) != len(schema.fields) {
				baseline = make([]SampleValue, len(schema.fields))
			}
		}
		entry = &stagedMeasureSet{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
			values:    baseline,
		}
		c.active.measureSetCounters[key] = entry
	}
	for i, deltaValue := range values {
		entry.values[i] += deltaValue
	}
}
