// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// snapshotStateSetInstrument writes sampled full-state points.
type snapshotStateSetInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulStateSetInstrument writes maintained full-state points.
type statefulStateSetInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// stagedStateSet holds one in-cycle stateset sample for a single series identity.
type stagedStateSet struct {
	key       string
	name      string
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	states    map[string]bool
}

// StateSet declares or reuses a snapshot stateset under this meter.
func (m *snapshotMeter) StateSet(name string, opts ...InstrumentOption) StateSetInstrument {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindStateSet, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	return &snapshotStateSetInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// StateSet declares or reuses a stateful stateset under this meter.
func (m *statefulMeter) StateSet(name string, opts ...InstrumentOption) StateSetInstrument {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindStateSet, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	return &statefulStateSetInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// ObserveStateSet writes a full-state sample for this collect cycle.
func (s *snapshotStateSetInstrument) ObserveStateSet(p StateSetPoint, labels ...LabelSet) {
	s.backend.recordStateSetObserve(s.desc, p, appendLabelSets(s.base, labels))
}

// Enable writes enum/bitset convenience sample with listed active states.
func (s *snapshotStateSetInstrument) Enable(actives ...string) {
	s.ObserveStateSet(stateSetPointFromActives(s.desc, actives...))
}

// ObserveStateSet writes a full-state sample for this collect cycle.
func (s *statefulStateSetInstrument) ObserveStateSet(p StateSetPoint, labels ...LabelSet) {
	s.backend.recordStateSetObserve(s.desc, p, appendLabelSets(s.base, labels))
}

// Enable writes enum/bitset convenience sample with listed active states.
func (s *statefulStateSetInstrument) Enable(actives ...string) {
	s.ObserveStateSet(stateSetPointFromActives(s.desc, actives...))
}

func stateSetPointFromActives(desc *instrumentDescriptor, actives ...string) StateSetPoint {
	schema := desc.stateSet
	if schema == nil {
		panic(errStateSetSchema)
	}

	if schema.mode == ModeEnum {
		if len(actives) != 1 {
			panic(errStateSetEnumCount)
		}
	}

	states := make(map[string]bool, len(schema.states))
	for _, st := range schema.states {
		states[st] = false
	}
	for _, active := range actives {
		if _, ok := schema.index[active]; !ok {
			panic(errStateSetUnknownState)
		}
		states[active] = true
	}
	return StateSetPoint{States: states}
}

// recordStateSetObserve writes one full-state stateset sample into the active frame.
func (c *storeCore) recordStateSetObserve(desc *instrumentDescriptor, point StateSetPoint, sets []LabelSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	schema := desc.stateSet
	if schema == nil {
		panic(errStateSetSchema)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errStateSetLabelKey)
	}

	states := normalizeStateSetPoint(point, schema)

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.stateSet[key]
	if !ok {
		entry = &stagedStateSet{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.stateSet[key] = entry
	}
	entry.states = states
}

func normalizeStateSetPoint(point StateSetPoint, schema *stateSetSchema) map[string]bool {
	states := make(map[string]bool, len(schema.states))
	for _, st := range schema.states {
		states[st] = false
	}
	for st, val := range point.States {
		if _, ok := schema.index[st]; !ok {
			panic(errStateSetUnknownState)
		}
		states[st] = val
	}

	if schema.mode == ModeEnum {
		active := 0
		for _, st := range schema.states {
			if states[st] {
				active++
			}
		}
		if active != 1 {
			panic(errStateSetEnumCount)
		}
	}

	return states
}

func labelsContainKey(labels []Label, key string) bool {
	for _, lbl := range labels {
		if lbl.Key == key {
			return true
		}
	}
	return false
}
