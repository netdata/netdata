// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func orderedMaterializedDimensionNames(dimensions map[string]*materializedDimensionState) []string {
	type staticEntry struct {
		name  string
		order int
	}
	staticEntries := make([]staticEntry, 0, len(dimensions))
	dynamicNames := make([]string, 0, len(dimensions))
	for name, dim := range dimensions {
		if dim.static {
			staticEntries = append(staticEntries, staticEntry{
				name:  name,
				order: dim.order,
			})
			continue
		}
		dynamicNames = append(dynamicNames, name)
	}
	sort.Slice(staticEntries, func(i, j int) bool {
		if staticEntries[i].order != staticEntries[j].order {
			return staticEntries[i].order < staticEntries[j].order
		}
		return staticEntries[i].name < staticEntries[j].name
	})
	sort.Strings(dynamicNames)
	out := make([]string, 0, len(staticEntries)+len(dynamicNames))
	for _, item := range staticEntries {
		out = append(out, item.name)
	}
	out = append(out, dynamicNames...)
	return out
}

func enforceLifecycleCaps(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
) ([]RemoveDimensionAction, []RemoveChartAction) {
	if len(chartsByID) == 0 || state == nil {
		return nil, nil
	}
	removeCharts := enforceChartInstanceCaps(currentSuccessSeq, chartsByID, state)
	removeDims := enforceDimensionCaps(currentSuccessSeq, chartsByID, state)
	return removeDims, removeCharts
}

func enforceChartInstanceCaps(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
) []RemoveChartAction {
	observedByTemplate := make(map[string][]string)
	for chartID, cs := range chartsByID {
		observedByTemplate[cs.templateID] = append(observedByTemplate[cs.templateID], chartID)
	}
	for templateID := range observedByTemplate {
		sort.Strings(observedByTemplate[templateID])
	}

	existingByTemplate := make(map[string][]string)
	for chartID, matChart := range state.charts {
		existingByTemplate[matChart.templateID] = append(existingByTemplate[matChart.templateID], chartID)
	}
	for templateID := range existingByTemplate {
		sort.Strings(existingByTemplate[templateID])
	}

	removeCharts := make([]RemoveChartAction, 0)

	templateIDs := make([]string, 0, len(observedByTemplate))
	for templateID := range observedByTemplate {
		templateIDs = append(templateIDs, templateID)
	}
	sort.Strings(templateIDs)

	for _, templateID := range templateIDs {
		observedIDs := observedByTemplate[templateID]
		var lifecycle program.LifecyclePolicy
		if len(observedIDs) > 0 {
			lifecycle = chartsByID[observedIDs[0]].lifecycle
		}
		// max_instances is a soft cap:
		// currently active chart instances are never evicted in the same successful cycle.
		maxInstances := lifecycle.MaxInstances
		if maxInstances <= 0 {
			continue
		}

		existingIDs := existingByTemplate[templateID]
		existingSet := make(map[string]struct{}, len(existingIDs))
		for _, id := range existingIDs {
			existingSet[id] = struct{}{}
		}
		newObserved := make([]string, 0, len(observedIDs))
		for _, id := range observedIDs {
			if _, ok := existingSet[id]; !ok {
				newObserved = append(newObserved, id)
			}
		}

		total := len(existingIDs) + len(newObserved)
		if total <= maxInstances {
			continue
		}
		overflow := total - maxInstances

		type chartCandidate struct {
			chartID  string
			lastSeen uint64
		}
		candidates := make([]chartCandidate, 0, len(existingIDs))
		for _, chartID := range existingIDs {
			matChart := state.charts[chartID]
			if matChart == nil {
				continue
			}
			// Never evict chart instances seen in the current successful cycle.
			if matChart.lastSeenSuccessSeq == currentSuccessSeq {
				continue
			}
			if _, seen := chartsByID[chartID]; seen {
				continue
			}
			candidates = append(candidates, chartCandidate{
				chartID:  chartID,
				lastSeen: matChart.lastSeenSuccessSeq,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].lastSeen != candidates[j].lastSeen {
				return candidates[i].lastSeen < candidates[j].lastSeen
			}
			return candidates[i].chartID < candidates[j].chartID
		})

		for i := 0; i < len(candidates) && overflow > 0; i++ {
			chartID := candidates[i].chartID
			matChart := state.charts[chartID]
			if matChart == nil {
				continue
			}
			removeCharts = append(removeCharts, RemoveChartAction{
				ChartID: chartID,
				Meta:    matChart.meta,
			})
			delete(state.charts, chartID)
			overflow--
		}

		if overflow > 0 {
			// Drop new chart instances deterministically when no eviction candidates remain.
			// If all existing instances are active and no new ones were observed, overflow remains
			// and the soft cap may be temporarily exceeded.
			for i := len(newObserved) - 1; i >= 0 && overflow > 0; i-- {
				delete(chartsByID, newObserved[i])
				overflow--
			}
		}
	}
	return removeCharts
}

func enforceDimensionCaps(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
) []RemoveDimensionAction {
	// Per-chart dimension caps: evict least-recently-seen inactive dims first, then drop new dims.
	removeDims := make([]RemoveDimensionAction, 0)
	chartIDs := make([]string, 0, len(chartsByID))
	for chartID := range chartsByID {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)
	for _, chartID := range chartIDs {
		cs := chartsByID[chartID]
		maxDims := cs.lifecycle.Dimensions.MaxDims
		if maxDims <= 0 {
			continue
		}
		matChart := state.charts[chartID]
		existingCount := 0
		if matChart != nil {
			existingCount = len(matChart.dimensions)
		}

		newNames := make([]string, 0, cs.observedCount)
		for name, entry := range cs.entries {
			if entry == nil || entry.seenSeq != cs.currentBuildSeq {
				continue
			}
			if matChart == nil || matChart.dimensions[name] == nil {
				newNames = append(newNames, name)
			}
		}
		sort.Strings(newNames)

		total := existingCount + len(newNames)
		if total <= maxDims {
			continue
		}
		overflow := total - maxDims

		if matChart != nil && overflow > 0 {
			type dimCandidate struct {
				name     string
				lastSeen uint64
			}
			candidates := make([]dimCandidate, 0, len(matChart.dimensions))
			for name, dim := range matChart.dimensions {
				if dim.lastSeenSuccessSeq == currentSuccessSeq {
					continue
				}
				if entry, seen := cs.entries[name]; seen && entry != nil && entry.seenSeq == cs.currentBuildSeq {
					continue
				}
				candidates = append(candidates, dimCandidate{
					name:     name,
					lastSeen: dim.lastSeenSuccessSeq,
				})
			}
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].lastSeen != candidates[j].lastSeen {
					return candidates[i].lastSeen < candidates[j].lastSeen
				}
				return candidates[i].name < candidates[j].name
			})
			for i := 0; i < len(candidates) && overflow > 0; i++ {
				name := candidates[i].name
				dim := matChart.dimensions[name]
				if dim == nil {
					continue
				}
				removeDims = append(removeDims, RemoveDimensionAction{
					ChartID:    chartID,
					ChartMeta:  matChart.meta,
					Name:       name,
					Hidden:     dim.hidden,
					Algorithm:  dim.algorithm,
					Multiplier: dim.multiplier,
					Divisor:    dim.divisor,
				})
				matChart.removeDimension(name)
				overflow--
			}
		}

		if overflow > 0 {
			orderedObserved := orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
			// Drop newest/least-priority candidates first (end of deterministic order).
			for i := len(orderedObserved) - 1; i >= 0 && overflow > 0; i-- {
				name := orderedObserved[i]
				if matChart != nil && matChart.dimensions[name] != nil {
					continue
				}
				delete(cs.entries, name)
				cs.observedCount--
				overflow--
			}
		}
	}
	return removeDims
}

func collectExpiryRemovals(
	currentSuccessSeq uint64,
	state *materializedState,
) ([]RemoveDimensionAction, []RemoveChartAction) {
	if state == nil || len(state.charts) == 0 {
		return nil, nil
	}

	chartIDs := make([]string, 0, len(state.charts))
	for chartID := range state.charts {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	toRemoveChart := make(map[string]struct{})
	for _, chartID := range chartIDs {
		matChart := state.charts[chartID]
		if shouldExpire(matChart.lastSeenSuccessSeq, currentSuccessSeq, matChart.lifecycle.ExpireAfterCycles) {
			toRemoveChart[chartID] = struct{}{}
		}
	}

	removeDims := make([]RemoveDimensionAction, 0)
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; removed {
			continue
		}
		matChart := state.charts[chartID]
		expireAfter := matChart.lifecycle.Dimensions.ExpireAfterCycles
		if expireAfter <= 0 || len(matChart.dimensions) == 0 {
			continue
		}

		dimNames := make([]string, 0, len(matChart.dimensions))
		for name := range matChart.dimensions {
			dimNames = append(dimNames, name)
		}
		sort.Strings(dimNames)
		for _, name := range dimNames {
			dim := matChart.dimensions[name]
			if !shouldExpire(dim.lastSeenSuccessSeq, currentSuccessSeq, expireAfter) {
				continue
			}
			removeDims = append(removeDims, RemoveDimensionAction{
				ChartID:    chartID,
				ChartMeta:  matChart.meta,
				Name:       name,
				Hidden:     dim.hidden,
				Algorithm:  dim.algorithm,
				Multiplier: dim.multiplier,
				Divisor:    dim.divisor,
			})
			matChart.removeDimension(name)
		}
	}

	removeCharts := make([]RemoveChartAction, 0, len(toRemoveChart))
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; !removed {
			continue
		}
		matChart := state.charts[chartID]
		if matChart == nil {
			continue
		}
		removeCharts = append(removeCharts, RemoveChartAction{
			ChartID: chartID,
			Meta:    matChart.meta,
		})
		delete(state.charts, chartID)
	}
	return removeDims, removeCharts
}

func orderedDimensionNamesFromState(dimensions map[string]dimensionState) []string {
	type staticEntry struct {
		name  string
		order int
	}
	staticEntries := make([]staticEntry, 0, len(dimensions))
	dynamicNames := make([]string, 0, len(dimensions))
	for name, state := range dimensions {
		if state.static {
			staticEntries = append(staticEntries, staticEntry{
				name:  name,
				order: state.order,
			})
			continue
		}
		dynamicNames = append(dynamicNames, name)
	}
	sort.Slice(staticEntries, func(i, j int) bool {
		if staticEntries[i].order != staticEntries[j].order {
			return staticEntries[i].order < staticEntries[j].order
		}
		return staticEntries[i].name < staticEntries[j].name
	})

	sort.Strings(dynamicNames)
	out := make([]string, 0, len(staticEntries)+len(dynamicNames))
	for _, entry := range staticEntries {
		out = append(out, entry.name)
	}
	out = append(out, dynamicNames...)
	return out
}

func orderedObservedDimensionNames(entries map[string]*dimBuildEntry, seenSeq uint64) []string {
	type staticEntry struct {
		name  string
		order int
	}
	staticEntries := make([]staticEntry, 0, len(entries))
	dynamicNames := make([]string, 0, len(entries))
	for name, entry := range entries {
		if entry == nil || entry.seenSeq != seenSeq {
			continue
		}
		if entry.static {
			staticEntries = append(staticEntries, staticEntry{
				name:  name,
				order: entry.order,
			})
			continue
		}
		dynamicNames = append(dynamicNames, name)
	}
	sort.Slice(staticEntries, func(i, j int) bool {
		if staticEntries[i].order != staticEntries[j].order {
			return staticEntries[i].order < staticEntries[j].order
		}
		return staticEntries[i].name < staticEntries[j].name
	})
	sort.Strings(dynamicNames)
	out := make([]string, 0, len(staticEntries)+len(dynamicNames))
	for _, entry := range staticEntries {
		out = append(out, entry.name)
	}
	out = append(out, dynamicNames...)
	return out
}
