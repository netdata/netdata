// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"maps"
)

func newCommittedSeries(key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	return &committedSeries{
		id:           SeriesID(key),
		hash64:       seriesIDHash(SeriesID(key)),
		key:          key,
		name:         name,
		hostScopeKey: hostScopeKey,
		hostScope:    cloneHostScope(hostScope),
		labels:       append([]Label(nil), labels...),
		labelsKey:    labelsKey,
		desc:         desc,
		meta:         baseSeriesMeta(desc),
	}
}

type committedSeriesCloneKind uint8

const (
	committedSeriesCloneFull committedSeriesCloneKind = iota
	committedSeriesCloneHistogramOverwrite
	committedSeriesCloneHistogramMutation
)

func ensureCommitSeriesMutable(old, next *readSnapshot, key string) *committedSeries {
	series, _ := ensureCommitSeriesMutableWithClone(old, next, key, committedSeriesCloneFull)
	return series
}

func ensureCommitSeriesMutableWithClone(old, next *readSnapshot, key string, cloneKind committedSeriesCloneKind) (*committedSeries, *committedSeries) {
	series := next.series[key]
	if series == nil {
		return nil, nil
	}
	if oldSeries, ok := old.series[key]; ok && oldSeries == series {
		series = cloneCommittedSeriesForKind(oldSeries, cloneKind)
		next.series[key] = series
		ensureSeriesMeta(series.desc, &series.meta)
		return series, oldSeries
	}
	ensureSeriesMeta(series.desc, &series.meta)
	return series, nil
}

func getOrCreateCommitSeries(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series, _ := ensureCommitSeriesMutableWithClone(old, next, key, committedSeriesCloneFull)
	if series != nil {
		return series
	}
	series = newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
	next.series[key] = series
	return series
}

func getOrCreateCommitHistogramSeries(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series, previous := ensureCommitSeriesMutableWithClone(old, next, key, committedSeriesCloneHistogramOverwrite)
	if series != nil {
		if previous != nil {
			rememberHistogramPreviousFrom(series, previous, desc)
		}
		return series
	}
	series = newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
	next.series[key] = series
	return series
}

func refreshCommittedHostScopes(old, next *readSnapshot, scopes map[string]HostScope) {
	if len(scopes) == 0 {
		return
	}
	for key, series := range next.series {
		scope, ok := scopes[series.hostScopeKey]
		if !ok || hostScopeEqual(series.hostScope, scope) {
			continue
		}
		series = ensureCommitSeriesMutable(old, next, key)
		if series == nil {
			continue
		}
		series.hostScope = cloneHostScope(scope)
	}
}

func markSeriesSeen(series *committedSeries, attemptSeq, successSeq uint64) {
	series.meta.LastSeenSuccessSeq = attemptSeq
	series.lastSeenSuccessCycle = successSeq
}

// makeSeriesKey joins host scope, metric name, and canonical label key into one stable identity key.
func makeSeriesKey(hostScopeKey, name, labelsKey string) string {
	base := name
	if labelsKey == "" {
		base = name
	} else {
		base = name + "\xfe" + labelsKey
	}
	if hostScopeKey == "" {
		return base
	}
	return hostScopeKey + "\xff" + base
}

func cloneCommittedSeries(s *committedSeries) *committedSeries {
	cp := cloneCommittedSeriesBase(s)
	if len(s.histogramCumulative) > 0 {
		cp.histogramCumulative = append([]SampleValue(nil), s.histogramCumulative...)
	}
	if len(s.histogramPreviousCumulative) > 0 {
		cp.histogramPreviousCumulative = append([]SampleValue(nil), s.histogramPreviousCumulative...)
	}
	return &cp
}

func cloneCommittedSeriesForKind(s *committedSeries, kind committedSeriesCloneKind) *committedSeries {
	switch kind {
	case committedSeriesCloneHistogramOverwrite:
		return cloneCommittedSeriesForHistogramOverwrite(s)
	case committedSeriesCloneHistogramMutation:
		return cloneCommittedSeriesForHistogramMutation(s)
	default:
		return cloneCommittedSeries(s)
	}
}

func cloneCommittedSeriesForHistogramOverwrite(s *committedSeries) *committedSeries {
	if s.desc == nil || s.desc.kind != kindHistogram {
		return cloneCommittedSeries(s)
	}
	cp := cloneCommittedSeriesBase(s)
	cp.histogramCumulative = nil
	cp.histogramPreviousCumulative = nil
	return &cp
}

func cloneCommittedSeriesForHistogramMutation(s *committedSeries) *committedSeries {
	if s.desc == nil || s.desc.kind != kindHistogram {
		return cloneCommittedSeries(s)
	}
	cp := cloneCommittedSeriesBase(s)
	if len(s.histogramCumulative) > 0 {
		cp.histogramCumulative = append([]SampleValue(nil), s.histogramCumulative...)
	}
	cp.histogramPreviousCumulative = nil
	return &cp
}

func cloneCommittedSeriesBase(s *committedSeries) committedSeries {
	cp := *s
	ensureSeriesMeta(cp.desc, &cp.meta)
	cp.hostScope = cloneHostScope(s.hostScope)
	// cp.labels intentionally reuses the original immutable label slice.
	// Label identity is part of the series key and is never mutated after publish.
	if s.stateSetValues != nil {
		cp.stateSetValues = cloneStateMap(s.stateSetValues)
	}
	if len(s.measureSetValues) > 0 {
		cp.measureSetValues = append([]SampleValue(nil), s.measureSetValues...)
	}
	if len(s.measureSetPreviousValues) > 0 {
		cp.measureSetPreviousValues = append([]SampleValue(nil), s.measureSetPreviousValues...)
	}
	if len(s.summaryQuantiles) > 0 {
		cp.summaryQuantiles = append([]SampleValue(nil), s.summaryQuantiles...)
	}
	if s.summarySketch != nil {
		cp.summarySketch = s.summarySketch.clone()
	}
	return cp
}

func cloneStateMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	maps.Copy(out, in)
	return out
}

func isScalarKind(kind metricKind) bool {
	return kind == kindGauge || kind == kindCounter
}
