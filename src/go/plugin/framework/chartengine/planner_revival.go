// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"

// A rejected plan can preserve a definition past expiry while metric collection
// advances. Compare before materialization overwrites its settings and last-seen.
func needsChartRevival(previous *materializedChartState, current *chartState, seq uint64) bool {
	if !expiredBeforeCycle(previous.lastSeenSuccessSeq, seq, previous.lifecycle.ExpireAfterCycles) {
		return false
	}
	if chartDefinitionChanged(previous.meta, current.meta) {
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
