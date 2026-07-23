// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

func flattenSnapshot(src *readSnapshot) *readSnapshot {
	series := snapshotSeriesView(src)
	dst := &readSnapshot{
		collectMeta: src.collectMeta,
		series:      make(map[string]*committedSeries, len(series)),
	}

	for _, s := range series {
		if s.desc == nil {
			continue
		}
		switch s.desc.kind {
		case kindGauge, kindCounter:
			dst.series[s.key] = s
		case kindHistogram:
			appendFlattenedHistogramSeries(dst, s)
		case kindSummary:
			appendFlattenedSummarySeries(dst, s)
		case kindStateSet:
			appendFlattenedStateSetSeries(dst, s)
		case kindMeasureSet:
			appendFlattenedMeasureSetSeries(dst, s)
		}
	}

	dst.index = buildSnapshotSeriesIndex(dst.series, dst.collectMeta)
	return dst
}

func appendFlattenedHistogramSeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.histogram
	if schema == nil {
		return
	}
	if len(src.histogramCumulative) != len(schema.bounds) {
		// Defensive guard against malformed snapshots.
		// Histogram() follows the same rule and returns unavailable.
		return
	}

	prevCumulative := SampleValue(0)
	for i, ub := range schema.bounds {
		cumulative := src.histogramCumulative[i]
		bucketValue := cumulative - prevCumulative
		prevCumulative = cumulative

		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[HistogramBucketLabel] = formatHistogramBucketLabel(ub)

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}

		name := src.name + "_bucket"
		key := makeSeriesKey(src.hostScopeKey, name, labelsKey)
		series := &committedSeries{
			id:           SeriesID(key),
			hash64:       seriesIDHash(SeriesID(key)),
			key:          key,
			name:         name,
			hostScopeKey: src.hostScopeKey,
			hostScope:    src.hostScope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc: &instrumentDescriptor{
				name:      name,
				kind:      kindCounter,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: bucketValue,
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindCounter,
				MetricKindHistogram,
				FlattenRoleHistogramBucket,
			),
		}
		previous := SampleValue(0)
		hasPrev := flattenedCounterDeltaSupported(src) && src.histogramHasPrev && len(src.histogramPreviousCumulative) == len(schema.bounds)
		if hasPrev {
			previous = src.histogramPreviousCumulative[i] - previousHistogramBucketFloor(src.histogramPreviousCumulative, i)
		}
		setFlattenedCounterState(series, bucketValue, previous, hasPrev, src.histogramCurrentSeq, src.histogramPreviousSeq)
		dst.series[key] = series
	}

	infMap := make(map[string]string, len(src.labels)+1)
	for _, lbl := range src.labels {
		infMap[lbl.Key] = lbl.Value
	}
	infMap[HistogramBucketLabel] = formatHistogramBucketLabel(math.Inf(1))
	infLabels, infLabelsKey, err := canonicalizeLabels(infMap)
	if err == nil {
		infName := src.name + "_bucket"
		infKey := makeSeriesKey(src.hostScopeKey, infName, infLabelsKey)
		series := &committedSeries{
			id:           SeriesID(infKey),
			hash64:       seriesIDHash(SeriesID(infKey)),
			key:          infKey,
			name:         infName,
			hostScopeKey: src.hostScopeKey,
			hostScope:    src.hostScope,
			labels:       infLabels,
			labelsKey:    infLabelsKey,
			desc: &instrumentDescriptor{
				name:      infName,
				kind:      kindCounter,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: src.histogramCount - prevCumulative,
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindCounter,
				MetricKindHistogram,
				FlattenRoleHistogramBucket,
			),
		}
		previous := SampleValue(0)
		hasPrev := flattenedCounterDeltaSupported(src) && src.histogramHasPrev && len(src.histogramPreviousCumulative) == len(schema.bounds)
		if hasPrev {
			previous = src.histogramPreviousCount
			if len(src.histogramPreviousCumulative) > 0 {
				previous -= src.histogramPreviousCumulative[len(src.histogramPreviousCumulative)-1]
			}
		}
		setFlattenedCounterState(series, src.histogramCount-prevCumulative, previous, hasPrev, src.histogramCurrentSeq, src.histogramPreviousSeq)
		dst.series[infKey] = series
	}

	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_count",
		src.labels,
		src.histogramCount,
		src.histogramPreviousCount,
		flattenedCounterDeltaSupported(src) && src.histogramHasPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_sum",
		src.labels,
		src.histogramSum,
		src.histogramPreviousSum,
		flattenedCounterDeltaSupported(src) && src.histogramHasPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramSum),
		src.desc,
	)
}

func appendFlattenedHistogramScalar(dst *readSnapshot, src *committedSeries, name string, labels []Label, value, previous SampleValue, hasPrev bool, meta SeriesMeta, desc *instrumentDescriptor) {
	labelsMap := make(map[string]string, len(labels))
	for _, lbl := range labels {
		labelsMap[lbl.Key] = lbl.Value
	}
	items, labelsKey, err := canonicalizeLabels(labelsMap)
	if err != nil {
		return
	}
	key := makeSeriesKey(src.hostScopeKey, name, labelsKey)
	series := &committedSeries{
		id:           SeriesID(key),
		hash64:       seriesIDHash(SeriesID(key)),
		key:          key,
		name:         name,
		hostScopeKey: src.hostScopeKey,
		hostScope:    src.hostScope,
		labels:       items,
		labelsKey:    labelsKey,
		desc: &instrumentDescriptor{
			name:      name,
			kind:      kindCounter,
			mode:      desc.mode,
			freshness: desc.freshness,
			window:    desc.window,
			meta:      desc.meta,
		},
		value: value,
		meta:  meta,
	}
	series.counterNoResetFallback = flattenedCounterNoResetFallback(meta)
	setFlattenedCounterState(series, value, previous, hasPrev, flattenedCounterCurrentSeq(src, meta), flattenedCounterPreviousSeq(src, meta))
	dst.series[key] = series
}

func appendFlattenedSummarySeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.summary
	if !summaryQuantilesMatchSchema(src) {
		return
	}

	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_count",
		src.labels,
		src.summaryCount,
		src.summaryPreviousCount,
		flattenedCounterDeltaSupported(src) && src.summaryHasPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummaryCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_sum",
		src.labels,
		src.summarySum,
		src.summaryPreviousSum,
		flattenedCounterDeltaSupported(src) && src.summaryHasPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummarySum),
		src.desc,
	)

	if schema == nil {
		return
	}

	for i, q := range schema.quantiles {
		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[SummaryQuantileLabel] = formatSummaryQuantileLabel(q)

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}
		key := makeSeriesKey(src.hostScopeKey, src.name, labelsKey)
		dst.series[key] = &committedSeries{
			id:           SeriesID(key),
			hash64:       seriesIDHash(SeriesID(key)),
			key:          key,
			name:         src.name,
			hostScopeKey: src.hostScopeKey,
			hostScope:    src.hostScope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc: &instrumentDescriptor{
				name:      src.name,
				kind:      kindGauge,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: src.summaryQuantiles[i],
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindGauge,
				MetricKindSummary,
				FlattenRoleSummaryQuantile,
			),
		}
	}
}

func summaryQuantilesMatchSchema(src *committedSeries) bool {
	schema := src.desc.summary
	if schema == nil {
		return len(src.summaryQuantiles) == 0
	}
	return len(src.summaryQuantiles) == len(schema.quantiles)
}

func previousHistogramBucketFloor(values []SampleValue, idx int) SampleValue {
	if idx == 0 {
		return 0
	}
	return values[idx-1]
}

func flattenedCounterDeltaSupported(src *committedSeries) bool {
	// Cycle-window histogram and summary samples are independent per cycle.
	return src.desc != nil && src.desc.window == WindowCumulative
}

func setFlattenedCounterState(series *committedSeries, current, previous SampleValue, hasPrev bool, currentSeq, previousSeq uint64) {
	series.counterCurrent = current
	series.counterCurrentSeq = currentSeq
	if !hasPrev {
		return
	}
	series.counterPrevious = previous
	series.counterPreviousSeq = previousSeq
	series.counterHasPrev = true
}

func flattenedCounterCurrentSeq(src *committedSeries, meta SeriesMeta) uint64 {
	switch meta.SourceKind {
	case MetricKindHistogram:
		return src.histogramCurrentSeq
	case MetricKindSummary:
		return src.summaryCurrentSeq
	default:
		return 0
	}
}

func flattenedCounterPreviousSeq(src *committedSeries, meta SeriesMeta) uint64 {
	switch meta.SourceKind {
	case MetricKindHistogram:
		return src.histogramPreviousSeq
	case MetricKindSummary:
		return src.summaryPreviousSeq
	default:
		return 0
	}
}

func flattenedCounterNoResetFallback(meta SeriesMeta) bool {
	return meta.FlattenRole == FlattenRoleHistogramSum || meta.FlattenRole == FlattenRoleSummarySum
}

func appendFlattenedStateSetSeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.stateSet
	if schema == nil {
		return
	}

	for _, state := range schema.states {
		value := SampleValue(0)
		if src.stateSetValues[state] {
			value = 1
		}

		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[src.name] = state

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}

		key := makeSeriesKey(src.hostScopeKey, src.name, labelsKey)
		dst.series[key] = &committedSeries{
			id:           SeriesID(key),
			hash64:       seriesIDHash(SeriesID(key)),
			key:          key,
			name:         src.name,
			hostScopeKey: src.hostScopeKey,
			hostScope:    src.hostScope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc: &instrumentDescriptor{
				name:      src.name,
				kind:      kindGauge,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: value,
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindGauge,
				MetricKindStateSet,
				FlattenRoleStateSetState,
			),
		}
	}
}

func appendFlattenedMeasureSetSeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.measureSet
	if schema == nil || len(src.measureSetValues) != len(schema.fields) {
		return
	}

	kind := MetricKindGauge
	descKind := kindGauge
	if schema.semantics == MeasureSetSemanticsCounter {
		kind = MetricKindCounter
		descKind = kindCounter
	}

	for i, field := range schema.fields {
		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[MeasureSetFieldLabel] = field.Name

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}

		name := src.name + "_" + field.Name
		key := makeSeriesKey(src.hostScopeKey, name, labelsKey)
		meta := src.desc.meta
		meta.Float = field.Float

		series := &committedSeries{
			id:           SeriesID(key),
			hash64:       seriesIDHash(SeriesID(key)),
			key:          key,
			name:         name,
			hostScopeKey: src.hostScopeKey,
			hostScope:    src.hostScope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc: &instrumentDescriptor{
				name:      name,
				kind:      descKind,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      meta,
			},
			value: src.measureSetValues[i],
			meta: flattenedSeriesMeta(
				src.meta,
				kind,
				MetricKindMeasureSet,
				FlattenRoleMeasureSetField,
			),
		}
		if schema.semantics == MeasureSetSemanticsCounter {
			series.counterCurrent = src.measureSetValues[i]
			series.counterCurrentSeq = src.measureSetCurrentSeq
			if src.measureSetHasPrev && len(src.measureSetPreviousValues) == len(schema.fields) {
				series.counterHasPrev = true
				series.counterPrevious = src.measureSetPreviousValues[i]
				series.counterPreviousSeq = src.measureSetPreviousSeq
			}
		}
		dst.series[key] = series
	}
}
