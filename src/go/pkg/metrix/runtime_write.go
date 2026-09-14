// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func (r *runtimeStoreBackend) recordGaugeSet(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		series.value = value
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordGaugeAdd(desc *instrumentDescriptor, scope HostScope, delta SampleValue, sets []LabelSet) {
	mustFiniteSample(delta)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		series.value += delta
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordCounterObserveTotal(_ *instrumentDescriptor, _ HostScope, _ SampleValue, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordCounterAdd(desc *instrumentDescriptor, scope HostScope, delta SampleValue, sets []LabelSet) {
	mustFiniteSample(delta)

	if delta < 0 {
		panic(errCounterNegativeDelta)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)

		hadCurrent := series.desc != nil && series.desc.kind == kindCounter && series.counterCurrentSeq > 0
		if hadCurrent {
			series.counterPrevious = series.counterCurrent
			series.counterPreviousSeq = series.counterCurrentSeq
			series.counterHasPrev = true
		} else {
			series.counterPrevious = 0
			series.counterPreviousSeq = 0
			series.counterHasPrev = false
		}

		series.counterCurrent += delta
		// Runtime delta contiguity is per-series, not global store sequence.
		series.counterCurrentSeq++
		series.value = series.counterCurrent
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordHistogramObservePoint(_ *instrumentDescriptor, _ HostScope, _ HistogramPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordHistogramObserve(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	schema := desc.histogram
	if schema == nil || len(schema.bounds) == 0 {
		panic(errHistogramBounds)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, HistogramBucketLabel) {
		panic(errHistogramLabelKey)
	}
	scope = mustNormalizeHostScope(scope)

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureHistogramSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)

		if series.desc.histogram == nil || !equalHistogramBounds(series.desc.histogram.bounds, schema.bounds) {
			panic("metrix: histogram schema drift detected")
		}
		if len(series.histogramCumulative) == 0 {
			series.histogramCumulative = make([]SampleValue, len(schema.bounds))
		}

		idx := findHistogramBucket(schema.bounds, value)
		if idx < len(series.histogramCumulative) {
			for i := idx; i < len(series.histogramCumulative); i++ {
				series.histogramCumulative[i]++
			}
		}
		series.histogramCount++
		series.histogramSum += value
		series.histogramCurrentSeq++
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordSummaryObservePoint(_ *instrumentDescriptor, _ HostScope, _ SummaryPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordSummaryObserve(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, SummaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}
	scope = mustNormalizeHostScope(scope)

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series, expired := r.runtimeEnsureSummarySeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		if expired {
			delete(r.summarySketches, key)
		}

		rememberSummaryPrevious(series, desc)
		series.summaryCount++
		series.summarySum += value
		series.summaryCurrentSeq++

		qs := desc.summaryQuantiles()
		if len(qs) > 0 {
			sketch := r.summarySketches[key]
			if sketch == nil {
				sketch = newSummaryQuantileSketch(desc.summaryReservoirSize(), summarySketchSeed(key))
				r.summarySketches[key] = sketch
			}
			sketch.observe(value)
			series.summaryQuantiles = sketch.quantiles(qs)
		} else {
			delete(r.summarySketches, key)
			series.summaryQuantiles = nil
		}
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordStateSetObserve(desc *instrumentDescriptor, scope HostScope, point StateSetPoint, sets []LabelSet) {
	schema := desc.stateSet
	if schema == nil {
		panic(errStateSetSchema)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errStateSetLabelKey)
	}
	scope = mustNormalizeHostScope(scope)
	states := normalizeStateSetPoint(point, schema)

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		series.stateSetValues = cloneStateMap(states)
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordMeasureSetGaugeObservePoint(_ *instrumentDescriptor, _ HostScope, _ MeasureSetPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordMeasureSetGaugeSetPoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetPoint(point, schema)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		series.measureSetValues = append(series.measureSetValues[:0], values...)
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordMeasureSetGaugeAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetPoint(delta, schema)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		if len(series.measureSetValues) == 0 {
			series.measureSetValues = make([]SampleValue, len(schema.fields))
		}
		for i, deltaValue := range values {
			series.measureSetValues[i] += deltaValue
		}
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordMeasureSetGaugeSetField(desc *instrumentDescriptor, scope HostScope, field string, value SampleValue, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsGauge {
		panic(errMeasureSetSchema)
	}

	fieldIndex := mustMeasureSetFieldIndex(field, schema)
	mustFiniteSample(value)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		if len(series.measureSetValues) == 0 {
			series.measureSetValues = make([]SampleValue, len(schema.fields))
		}
		series.measureSetValues[fieldIndex] = value
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordMeasureSetCounterObserveTotalPoint(_ *instrumentDescriptor, _ HostScope, _ MeasureSetPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordMeasureSetCounterAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet) {
	schema := desc.measureSet
	if schema == nil || schema.semantics != MeasureSetSemanticsCounter {
		panic(errMeasureSetSchema)
	}

	values := normalizeMeasureSetCounterDelta(delta, schema)

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, MeasureSetFieldLabel) {
		panic(errMeasureSetLabelKey)
	}
	scope = mustNormalizeHostScope(scope)
	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := r.runtimeEnsureSeriesMutable(old, next, key, desc.name, scope.ScopeKey, scope, labels, labelsKey, desc, nowUnixNano)
		if len(series.measureSetValues) == 0 {
			series.measureSetValues = make([]SampleValue, len(schema.fields))
		}
		if series.measureSetCurrentSeq > 0 {
			series.measureSetPreviousValues = append(series.measureSetPreviousValues[:0], series.measureSetValues...)
			series.measureSetPreviousSeq = series.measureSetCurrentSeq
			series.measureSetHasPrev = true
		} else {
			series.measureSetPreviousValues = nil
			series.measureSetPreviousSeq = 0
			series.measureSetHasPrev = false
		}
		for i, deltaValue := range values {
			series.measureSetValues[i] += deltaValue
		}
		series.measureSetCurrentSeq++
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}
