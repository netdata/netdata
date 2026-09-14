// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func (c *storeCore) recordMeasureSetGaugeObservePoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet) {
	c.recordMeasureSetGaugeSetPoint(desc, scope, point, sets)
}

func (c *storeCore) recordMeasureSetGaugeSetPoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet) {
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
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.measureSetGauges[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		entry = &stagedMeasureSet{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
		}
		c.active.measureSetGauges[key] = entry
	}
	entry.values = append(entry.values[:0], values...)
}

func (c *storeCore) recordMeasureSetGaugeAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet) {
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
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.measureSetGauges[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		baseline := make([]SampleValue, len(schema.fields))
		if existing := c.baselineSeriesForWrite(key, desc); existing != nil {
			baseline = append(baseline[:0], existing.measureSetValues...)
			if len(baseline) != len(schema.fields) {
				baseline = make([]SampleValue, len(schema.fields))
			}
		}
		entry = &stagedMeasureSet{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
			values:       baseline,
		}
		c.active.measureSetGauges[key] = entry
	}
	for i, deltaValue := range values {
		entry.values[i] += deltaValue
	}
}

func (c *storeCore) recordMeasureSetGaugeSetField(desc *instrumentDescriptor, scope HostScope, field string, value SampleValue, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	fieldIndex := mustMeasureSetFieldIndex(field, schema)
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
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.measureSetGauges[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		baseline := make([]SampleValue, len(schema.fields))
		if existing := c.baselineSeriesForWrite(key, desc); existing != nil {
			baseline = append(baseline[:0], existing.measureSetValues...)
			if len(baseline) != len(schema.fields) {
				baseline = make([]SampleValue, len(schema.fields))
			}
		}
		entry = &stagedMeasureSet{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
			values:       baseline,
		}
		c.active.measureSetGauges[key] = entry
	} else if len(entry.values) == 0 {
		entry.values = make([]SampleValue, len(schema.fields))
	}
	entry.values[fieldIndex] = value
}

func (c *storeCore) recordMeasureSetCounterObserveTotalPoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet) {
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
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.measureSetCounters[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		entry = &stagedMeasureSet{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
		}
		c.active.measureSetCounters[key] = entry
	}
	entry.values = append(entry.values[:0], values...)
}

func (c *storeCore) recordMeasureSetCounterAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet) {
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
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.measureSetCounters[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		baseline := make([]SampleValue, len(schema.fields))
		if existing := c.baselineSeriesForWrite(key, desc); existing != nil {
			baseline = append(baseline[:0], existing.measureSetValues...)
			if len(baseline) != len(schema.fields) {
				baseline = make([]SampleValue, len(schema.fields))
			}
		}
		entry = &stagedMeasureSet{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
			values:       baseline,
		}
		c.active.measureSetCounters[key] = entry
	}
	for i, deltaValue := range values {
		entry.values[i] += deltaValue
	}
}
