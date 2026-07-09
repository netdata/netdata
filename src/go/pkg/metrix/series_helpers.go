// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"maps"
	"sort"
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

func ensureCommitSeriesMutable(old, next *readSnapshot, key string) *committedSeries {
	series := next.series[key]
	if series == nil {
		return nil
	}
	if oldSeries, ok := old.series[key]; ok && oldSeries == series {
		series = cloneCommittedSeries(series)
		next.series[key] = series
	}
	ensureSeriesMeta(series.desc, &series.meta)
	return series
}

func getOrCreateCommitSeries(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series := ensureCommitSeriesMutable(old, next, key)
	if series != nil {
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

// buildByName builds deterministic per-name iteration lists for snapshot readers.
func buildByName(series map[string]*committedSeries) map[string][]*committedSeries {
	byName := make(map[string][]*committedSeries)
	for _, s := range series {
		if s.desc == nil || !isScalarKind(s.desc.kind) {
			continue
		}
		byName[s.name] = append(byName[s.name], s)
	}
	for _, lst := range byName {
		sort.Slice(lst, func(i, j int) bool {
			if lst[i].hostScopeKey != lst[j].hostScopeKey {
				return lst[i].hostScopeKey < lst[j].hostScopeKey
			}
			return lst[i].labelsKey < lst[j].labelsKey
		})
	}
	return byName
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
	if len(s.histogramCumulative) > 0 {
		cp.histogramCumulative = append([]SampleValue(nil), s.histogramCumulative...)
	}
	if len(s.histogramPreviousCumulative) > 0 {
		cp.histogramPreviousCumulative = append([]SampleValue(nil), s.histogramPreviousCumulative...)
	}
	if len(s.summaryQuantiles) > 0 {
		cp.summaryQuantiles = append([]SampleValue(nil), s.summaryQuantiles...)
	}
	if s.summarySketch != nil {
		cp.summarySketch = s.summarySketch.clone()
	}
	return &cp
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
