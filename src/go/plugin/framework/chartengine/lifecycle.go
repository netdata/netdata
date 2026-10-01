// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// materializedState tracks engine-owned chart lifecycle across successful cycles.
type materializedState struct {
	charts map[string]*materializedChartState
	// chartDeletes counts deletions from charts since the map was last rebuilt.
	chartDeletes int
}

// materializedChartState tracks one materialized chart instance.
type materializedChartState struct {
	templateID         string
	meta               program.ChartMeta
	emittedMeta        program.ChartMeta
	lifecycle          program.LifecyclePolicy
	lastSeenSuccessSeq uint64
	dimensions         map[string]*materializedDimensionState
	presentation       *materializedChartPresentation
	orderedDimsDirty   bool
	scratchEntries     map[string]*dimBuildEntry
	// journalToken marks the build that created or already recorded this chart.
	journalToken uint64
	// dimDeletes and scratchDeletes count deletions from dimensions and scratchEntries
	// since each map was last rebuilt.
	dimDeletes     int32
	scratchDeletes int32
}

// materializedChartPresentation uses copy-on-write for ordered dimensions,
// canonical labels, and exact series membership.
type materializedChartPresentation struct {
	orderedDims     []string
	labelValues     map[string]string
	labelMembership []chartLabelMembership
}

// materializedDimensionState tracks one materialized dimension in a chart.
type materializedDimensionState struct {
	hidden             bool
	float              bool
	static             bool
	order              int
	sortKey            dimensionSortKey
	algorithm          program.Algorithm
	multiplier         int
	divisor            int
	lastSeenSuccessSeq uint64
	journalToken       uint64
}

func newMaterializedState() materializedState {
	return materializedState{
		charts: make(map[string]*materializedChartState),
	}
}

func (s *materializedState) ensureChart(
	j *planJournal,
	chartID string,
	templateID string,
	meta program.ChartMeta,
	lifecycle program.LifecyclePolicy,
) (*materializedChartState, bool) {
	chart, ok := s.charts[chartID]
	if ok {
		if chart.meta != meta || chart.lifecycle != lifecycle {
			j.touchChart(chart)
			chart.meta = meta
			chart.lifecycle = lifecycle
		}
		return chart, false
	}
	chart = j.newChart(&materializedChartState{
		templateID: templateID,
		meta:       meta,
		lifecycle:  lifecycle,
		dimensions: make(map[string]*materializedDimensionState),
	})
	j.putChart(s.charts, chartID, chart)
	return chart, true
}

func (c *materializedChartState) ensureDimension(
	j *planJournal,
	name string,
	state dimensionState,
) (*materializedDimensionState, bool) {
	algorithm := state.algorithm.programAlgorithm()
	dim, ok := c.dimensions[name]
	if ok {
		// Ordering follows the latest observation; creation settings describe the
		// published dimension until it is recreated.
		if dim.static != state.static || dim.order != state.order || dim.sortKey != state.sortKey {
			j.touchChart(c)
			c.orderedDimsDirty = true
			j.touchDim(dim)
			dim.static = state.static
			dim.order = state.order
			dim.sortKey = state.sortKey
		}
		return dim, false
	}
	dim = &materializedDimensionState{
		hidden:     state.hidden,
		float:      state.float,
		static:     state.static,
		order:      state.order,
		sortKey:    state.sortKey,
		algorithm:  algorithm,
		multiplier: state.multiplier,
		divisor:    state.divisor,
	}
	j.putDim(c, name, dim)
	j.touchChart(c)
	c.orderedDimsDirty = true
	return dim, true
}

func (c *materializedChartState) removeDimension(j *planJournal, name string) {
	if _, ok := c.dimensions[name]; !ok {
		return
	}
	j.deleteDim(c, name)
	j.touchChart(c)
	c.orderedDimsDirty = true
}

func (c *materializedChartState) orderedDimensionNames(j *planJournal) []string {
	if !c.orderedDimsDirty && c.presentation != nil && len(c.presentation.orderedDims) == len(c.dimensions) {
		return c.presentation.orderedDims
	}
	j.touchChart(c)
	next := materializedChartPresentation{}
	if c.presentation != nil {
		next = *c.presentation
	}
	next.orderedDims = orderedMaterializedDimensionNames(c.dimensions)
	c.presentation = &next
	c.orderedDimsDirty = false
	return c.presentation.orderedDims
}

func (c *materializedChartState) checkoutScratchEntries(j *planJournal, dimCap int) map[string]*dimBuildEntry {
	if c.scratchEntries != nil {
		return c.scratchEntries
	}
	j.touchChart(c)
	c.scratchEntries = make(map[string]*dimBuildEntry, dimCap)
	return c.scratchEntries
}

func (c *materializedChartState) dimensionScratchEntries() map[string]*dimBuildEntry {
	if c == nil {
		return nil
	}
	return c.scratchEntries
}

// storeScratchEntries adopts a build's scratch map when it is not already the chart's own.
func (c *materializedChartState) storeScratchEntries(
	j *planJournal,
	entries map[string]*dimBuildEntry,
	owner *materializedChartState,
) {
	if owner == c {
		return
	}
	j.touchChart(c)
	c.scratchEntries = entries
}

func (c *materializedChartState) pruneScratchEntries(j *planJournal, currentSeq uint64) {
	if len(c.scratchEntries) == 0 {
		return
	}
	for name, entry := range c.scratchEntries {
		if entry == nil {
			j.deleteEntry(c, c.scratchEntries, name)
			continue
		}
		if entry.seenSeq == currentSeq {
			continue
		}
		if _, keep := c.dimensions[name]; keep {
			continue
		}
		j.deleteEntry(c, c.scratchEntries, name)
	}
}

func (c *materializedChartState) replaceLabels(
	j *planJournal,
	values map[string]string,
	membership []chartLabelMembership,
) {
	j.touchChart(c)
	next := materializedChartPresentation{}
	if c.presentation != nil {
		next = *c.presentation
	}
	next.labelValues = values
	next.labelMembership = membership
	c.presentation = &next
}

func (c *materializedChartState) replaceLabelMembership(j *planJournal, membership []chartLabelMembership) {
	j.touchChart(c)
	next := materializedChartPresentation{}
	if c.presentation != nil {
		next = *c.presentation
	}
	next.labelMembership = membership
	c.presentation = &next
}

func shouldExpire(lastSeenSuccessSeq, currentSuccessSeq uint64, expireAfterCycles int) bool {
	if expireAfterCycles <= 0 {
		return false
	}
	if lastSeenSuccessSeq == 0 || currentSuccessSeq <= lastSeenSuccessSeq {
		return false
	}
	missedCycles := currentSuccessSeq - lastSeenSuccessSeq
	return missedCycles >= uint64(expireAfterCycles)
}
