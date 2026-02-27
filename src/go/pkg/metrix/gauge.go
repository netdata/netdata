// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// snapshotGaugeInstrument writes sampled absolute gauge values.
type snapshotGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulGaugeInstrument writes maintained gauge values.
type statefulGaugeInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// stagedGauge holds one in-cycle gauge sample for a single series identity.
type stagedGauge struct {
	key       string
	name      string
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	value     SampleValue
}

// Gauge declares or reuses a snapshot gauge under this meter.
func (m *snapshotMeter) Gauge(name string, opts ...InstrumentOption) SnapshotGauge {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindGauge, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	return &snapshotGaugeInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// Gauge declares or reuses a stateful gauge under this meter.
func (m *statefulMeter) Gauge(name string, opts ...InstrumentOption) StatefulGauge {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindGauge, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	return &statefulGaugeInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// Observe writes one absolute gauge value for this collect cycle.
func (g *snapshotGaugeInstrument) Observe(v SampleValue, labels ...LabelSet) {
	g.backend.recordGaugeSet(g.desc, v, appendLabelSets(g.base, labels))
}

// Set overwrites the staged gauge value for this collect cycle.
func (g *statefulGaugeInstrument) Set(v SampleValue, labels ...LabelSet) {
	g.backend.recordGaugeSet(g.desc, v, appendLabelSets(g.base, labels))
}

// Add accumulates on top of the committed baseline for this collect cycle.
func (g *statefulGaugeInstrument) Add(delta SampleValue, labels ...LabelSet) {
	g.backend.recordGaugeAdd(g.desc, delta, appendLabelSets(g.base, labels))
}

// recordGaugeSet writes one gauge sample into the active frame (last-write-wins in-cycle).
func (c *storeCore) recordGaugeSet(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.gauges[key]
	if !ok {
		entry = &stagedGauge{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.gauges[key] = entry
	}
	entry.value = value
}

// recordGaugeAdd accumulates delta into the active frame using committed baseline on first write.
func (c *storeCore) recordGaugeAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet) {
	mustFiniteSample(delta)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.gauges[key]
	if !ok {
		baseline := SampleValue(0)
		if existing := c.snapshot.Load().series[key]; existing != nil {
			baseline = existing.value
		}
		entry = &stagedGauge{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
			value:     baseline,
		}
		c.active.gauges[key] = entry
	}
	entry.value += delta
}
