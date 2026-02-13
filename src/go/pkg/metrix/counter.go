// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// snapshotCounterInstrument writes sampled monotonic totals.
type snapshotCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulCounterInstrument writes maintained counter deltas.
type statefulCounterInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// stagedCounter holds one in-cycle counter current total for a series identity.
type stagedCounter struct {
	key       string
	name      string
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	current   SampleValue
}

// Counter declares or reuses a snapshot counter under this meter.
func (m *snapshotMeter) Counter(name string, opts ...InstrumentOption) SnapshotCounter {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindCounter, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	return &snapshotCounterInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// Counter declares or reuses a stateful counter under this meter.
func (m *statefulMeter) Counter(name string, opts ...InstrumentOption) StatefulCounter {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindCounter, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	return &statefulCounterInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// ObserveTotal writes one monotonic total sample for this collect cycle.
func (c *snapshotCounterInstrument) ObserveTotal(v SampleValue, labels ...LabelSet) {
	c.backend.recordCounterObserveTotal(c.desc, v, appendLabelSets(c.base, labels))
}

// Add accumulates a delta for this collect cycle.
func (c *statefulCounterInstrument) Add(delta SampleValue, labels ...LabelSet) {
	c.backend.recordCounterAdd(c.desc, delta, appendLabelSets(c.base, labels))
}

// recordCounterObserveTotal writes one sampled monotonic total for snapshot counters.
func (c *storeCore) recordCounterObserveTotal(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
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
	entry, ok := c.active.counters[key]
	if !ok {
		entry = &stagedCounter{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.counters[key] = entry
	}
	entry.current = value // last-write-wins in-cycle
}

// recordCounterAdd accumulates delta for stateful counters.
func (c *storeCore) recordCounterAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet) {
	mustFiniteSample(delta)

	if delta < 0 {
		panic(errCounterNegativeDelta)
	}

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
	entry, ok := c.active.counters[key]
	if !ok {
		baseline := SampleValue(0)
		if existing := c.snapshot.Load().series[key]; existing != nil && existing.desc != nil && existing.desc.kind == kindCounter {
			baseline = existing.counterCurrent
		}
		entry = &stagedCounter{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
			current:   baseline,
		}
		c.active.counters[key] = entry
	}
	entry.current += delta
}
