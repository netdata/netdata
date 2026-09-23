// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"

// A rejected plan can preserve a definition past expiry while metric collection
// advances. Compare before materialization overwrites its settings and last-seen.
func needsChartRevival(previous *materializedChartState, current *chartState, seq uint64) bool {
	if !expiredBeforeCycle(previous.lastSeenSuccessSeq, seq, previous.lifecycle.ExpireAfterCycles) {
		return false
	}
	if chartDefinitionChanged(previous.emittedMeta, current.meta) {
		return true
	}
	for name, entry := range current.entries {
		if entry == nil || entry.seenSeq != current.currentBuildSeq {
			continue
		}
		if dim := previous.dimensions[name]; dim != nil && dimensionDefinitionChanged(dim, entry.dimensionState) {
			return true
		}
	}
	return false
}

// Chart creation, label updates and dimension actions all emit CHART commands.
// Record their final metadata in the staged state, so Abort preserves the prior
// published definition. Create/label actions use the same current chart metadata;
// removals are emitted last and may carry older metadata from cap enforcement.
func (s *materializedState) recordEmittedChartDefinitions(j *planJournal, actions []EngineAction) {
	remember := func(id string, meta program.ChartMeta) {
		if chart := s.charts[id]; chart != nil && chart.emittedMeta != meta {
			j.touchChart(chart)
			chart.emittedMeta = meta
		}
	}
	for _, action := range actions {
		switch a := action.(type) {
		case CreateChartAction:
			remember(a.ChartID, a.Meta)
		case CreateDimensionAction:
			remember(a.ChartID, a.ChartMeta)
		case UpdateChartLabelsAction:
			remember(a.ChartID, a.Meta)
		}
	}
	for _, action := range actions {
		if a, ok := action.(RemoveDimensionAction); ok {
			remember(a.ChartID, a.ChartMeta)
		}
	}
}

func expiredBeforeCycle(lastSeen, current uint64, expiry int) bool {
	// Returning at the boundary is still ordinary observation-before-expiry.
	return current > 0 && shouldExpire(lastSeen, current-1, expiry)
}

func chartDefinitionChanged(previous, current program.ChartMeta) bool {
	// Algorithm is chart policy; the resolved per-dimension algorithm goes on the wire.
	return previous.Title != current.Title || previous.Units != current.Units ||
		previous.Family != current.Family || previous.Context != current.Context ||
		previous.Type != current.Type || previous.Priority != current.Priority
}

func dimensionDefinitionChanged(previous *materializedDimensionState, current dimensionState) bool {
	return previous.algorithm != current.algorithm.programAlgorithm() ||
		effectiveDimensionScale(previous.multiplier) != effectiveDimensionScale(current.multiplier) ||
		effectiveDimensionScale(previous.divisor) != effectiveDimensionScale(current.divisor) ||
		previous.hidden != current.hidden || previous.float != current.float
}

func effectiveDimensionScale(value int) int {
	if value == 0 {
		return 1
	}
	return value
}
