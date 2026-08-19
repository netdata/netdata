// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func orderedMaterializedDimensionNames(dimensions map[string]*materializedDimensionState) []string {
	staticEntries := make([]staticDimensionOrderEntry, 0, len(dimensions))
	dynamicEntries := make([]dynamicDimensionOrderEntry, 0, len(dimensions))
	for name, dim := range dimensions {
		if dim.static {
			staticEntries = append(staticEntries, staticDimensionOrderEntry{
				name:  name,
				order: dim.order,
			})
			continue
		}
		dynamicEntries = append(dynamicEntries, dynamicDimensionOrderEntry{
			name:    name,
			sortKey: dim.sortKey,
		})
	}
	return orderedDimensionNames(staticEntries, dynamicEntries)
}

type staticDimensionOrderEntry struct {
	name  string
	order int
}

type dynamicDimensionOrderEntry struct {
	name    string
	sortKey dimensionSortKey
}

func orderedDimensionNames(staticEntries []staticDimensionOrderEntry, dynamicEntries []dynamicDimensionOrderEntry) []string {
	sort.Slice(staticEntries, func(i, j int) bool {
		if staticEntries[i].order != staticEntries[j].order {
			return staticEntries[i].order < staticEntries[j].order
		}
		return staticEntries[i].name < staticEntries[j].name
	})
	sort.Slice(dynamicEntries, func(i, j int) bool {
		return lessDynamicDimension(dynamicEntries[i], dynamicEntries[j])
	})
	out := make([]string, 0, len(staticEntries)+len(dynamicEntries))
	for _, item := range staticEntries {
		out = append(out, item.name)
	}
	for _, item := range dynamicEntries {
		out = append(out, item.name)
	}
	return out
}

func lessDynamicDimension(lhs, rhs dynamicDimensionOrderEntry) bool {
	if lhs.sortKey.kind != rhs.sortKey.kind {
		return lhs.sortKey.kind < rhs.sortKey.kind
	}
	if lhs.sortKey.kind == dimensionSortHistogramBucket {
		if lhs.sortKey.upperBound != rhs.sortKey.upperBound {
			return lhs.sortKey.upperBound < rhs.sortKey.upperBound
		}
	}
	return lhs.name < rhs.name
}

func enforceLifecycleCapsWithObserver(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
	observe func(PlanRouteDiagnostic),
) ([]RemoveDimensionAction, []RemoveChartAction) {
	if len(chartsByID) == 0 || state == nil {
		return nil, nil
	}
	removeCharts := enforceChartInstanceCapsWithObserver(currentSuccessSeq, chartsByID, state, observe)
	removeDims := enforceDimensionCapsWithObserver(currentSuccessSeq, chartsByID, state, observe)
	return removeDims, removeCharts
}

func enforceChartInstanceCapsWithObserver(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
	observe func(PlanRouteDiagnostic),
) []RemoveChartAction {
	var observedByTemplate map[string][]string
	for chartID, cs := range chartsByID {
		if cs.lifecycle.MaxInstances <= 0 {
			continue
		}
		if observedByTemplate == nil {
			observedByTemplate = make(map[string][]string)
		}
		observedByTemplate[cs.templateID] = append(observedByTemplate[cs.templateID], chartID)
	}
	if len(observedByTemplate) == 0 {
		return nil
	}
	for templateID := range observedByTemplate {
		sort.Strings(observedByTemplate[templateID])
	}

	existingByTemplate := make(map[string][]string)
	for chartID, matChart := range state.charts {
		if _, enabled := observedByTemplate[matChart.templateID]; !enabled {
			continue
		}
		existingByTemplate[matChart.templateID] = append(existingByTemplate[matChart.templateID], chartID)
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
				chartID := newObserved[i]
				if observe != nil {
					cs := chartsByID[chartID]
					fact := PlanRouteDiagnostic{
						Decision: PlanRouteLifecycleRejected,
						Reason:   PlanRouteReasonChartInstanceCap,
						ChartID:  chartID,
					}
					if cs != nil {
						fact.ChartTemplateID = cs.templateID
					}
					observe(fact)
				}
				delete(chartsByID, chartID)
				overflow--
			}
		}
	}
	return removeCharts
}

func enforceDimensionCapsWithObserver(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
	observe func(PlanRouteDiagnostic),
) []RemoveDimensionAction {
	// Per-chart dimension caps: evict least-recently-seen inactive dims first, then drop new dims.
	var chartIDs []string
	for chartID, cs := range chartsByID {
		if cs.lifecycle.Dimensions.MaxDims <= 0 {
			continue
		}
		if chartIDs == nil {
			chartIDs = make([]string, 0, len(chartsByID))
		}
		chartIDs = append(chartIDs, chartID)
	}
	if len(chartIDs) == 0 {
		return nil
	}
	sort.Strings(chartIDs)

	removeDims := make([]RemoveDimensionAction, 0)
	for _, chartID := range chartIDs {
		cs := chartsByID[chartID]
		maxDims := cs.lifecycle.Dimensions.MaxDims
		matChart := state.charts[chartID]
		existingCount := 0
		if matChart != nil {
			existingCount = len(matChart.dimensions)
		}

		newCount := 0
		for name, entry := range cs.entries {
			if entry == nil || entry.seenSeq != cs.currentBuildSeq {
				continue
			}
			if matChart == nil || matChart.dimensions[name] == nil {
				newCount++
			}
		}

		total := existingCount + newCount
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
					Float:      dim.float,
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
				if observe != nil {
					observe(PlanRouteDiagnostic{
						Decision:        PlanRouteLifecycleRejected,
						Reason:          PlanRouteReasonDimensionCap,
						ChartTemplateID: cs.templateID,
						ChartID:         chartID,
						DimensionName:   name,
					})
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
				Float:      dim.float,
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

func orderedObservedDimensionNames(entries map[string]*dimBuildEntry, seenSeq uint64) []string {
	staticEntries := make([]staticDimensionOrderEntry, 0, len(entries))
	dynamicEntries := make([]dynamicDimensionOrderEntry, 0, len(entries))
	for name, entry := range entries {
		if entry == nil || entry.seenSeq != seenSeq {
			continue
		}
		if entry.static {
			staticEntries = append(staticEntries, staticDimensionOrderEntry{
				name:  name,
				order: entry.order,
			})
			continue
		}
		dynamicEntries = append(dynamicEntries, dynamicDimensionOrderEntry{
			name:    name,
			sortKey: entry.sortKey,
		})
	}
	return orderedDimensionNames(staticEntries, dynamicEntries)
}
