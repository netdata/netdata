// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

func flattenSnapshot(src *readSnapshot) *readSnapshot {
	series := snapshotSeriesView(src)
	dst := &readSnapshot{
		collectMeta: src.collectMeta,
		series:      make(map[string]*committedSeries, len(series)),
		byName:      make(map[string][]*committedSeries),
	}

	for _, s := range series {
		if s.desc == nil {
			continue
		}
		switch s.desc.kind {
		case kindGauge, kindCounter:
			dst.series[s.key] = cloneCommittedSeries(s)
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

	dst.byName = buildByName(dst.series)
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
		dst.series[key] = &committedSeries{
			id:           SeriesID(key),
			hash64:       seriesIDHash(SeriesID(key)),
			key:          key,
			name:         name,
			hostScopeKey: src.hostScopeKey,
			hostScope:    cloneHostScope(src.hostScope),
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
		dst.series[infKey] = &committedSeries{
			id:           SeriesID(infKey),
			hash64:       seriesIDHash(SeriesID(infKey)),
			key:          infKey,
			name:         infName,
			hostScopeKey: src.hostScopeKey,
			hostScope:    cloneHostScope(src.hostScope),
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
	}

	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_count",
		src.labels,
		src.histogramCount,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_sum",
		src.labels,
		src.histogramSum,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramSum),
		src.desc,
	)
}

func appendFlattenedHistogramScalar(dst *readSnapshot, src *committedSeries, name string, labels []Label, value SampleValue, meta SeriesMeta, desc *instrumentDescriptor) {
	labelsMap := make(map[string]string, len(labels))
	for _, lbl := range labels {
		labelsMap[lbl.Key] = lbl.Value
	}
	items, labelsKey, err := canonicalizeLabels(labelsMap)
	if err != nil {
		return
	}
	key := makeSeriesKey(src.hostScopeKey, name, labelsKey)
	dst.series[key] = &committedSeries{
		id:           SeriesID(key),
		hash64:       seriesIDHash(SeriesID(key)),
		key:          key,
		name:         name,
		hostScopeKey: src.hostScopeKey,
		hostScope:    cloneHostScope(src.hostScope),
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
}

func appendFlattenedSummarySeries(dst *readSnapshot, src *committedSeries) {
	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_count",
		src.labels,
		src.summaryCount,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummaryCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src,
		src.name+"_sum",
		src.labels,
		src.summarySum,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummarySum),
		src.desc,
	)

	schema := src.desc.summary
	if schema == nil {
		return
	}
	if len(src.summaryQuantiles) != len(schema.quantiles) {
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
			hostScope:    cloneHostScope(src.hostScope),
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
			hostScope:    cloneHostScope(src.hostScope),
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
			hostScope:    cloneHostScope(src.hostScope),
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
