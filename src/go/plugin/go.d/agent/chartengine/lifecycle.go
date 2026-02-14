// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"

// materializedState tracks engine-owned chart lifecycle across successful cycles.
type materializedState struct {
	charts map[string]*materializedChartState
}

// materializedChartState tracks one materialized chart instance.
type materializedChartState struct {
	templateID         string
	meta               program.ChartMeta
	lifecycle          program.LifecyclePolicy
	lastSeenSuccessSeq uint64
	dimensions         map[string]*materializedDimensionState
}

// materializedDimensionState tracks one materialized dimension in a chart.
type materializedDimensionState struct {
	name               string
	hidden             bool
	static             bool
	order              int
	algorithm          program.Algorithm
	multiplier         int
	divisor            int
	lastSeenSuccessSeq uint64
}

func newMaterializedState() materializedState {
	return materializedState{
		charts: make(map[string]*materializedChartState),
	}
}

func (s *materializedState) ensureChart(
	chartID string,
	templateID string,
	meta program.ChartMeta,
	lifecycle program.LifecyclePolicy,
) (*materializedChartState, bool) {
	chart, ok := s.charts[chartID]
	if ok {
		if chart.templateID != templateID {
			chart.templateID = templateID
			chart.dimensions = make(map[string]*materializedDimensionState)
		}
		chart.meta = meta
		chart.lifecycle = lifecycle
		return chart, false
	}
	chart = &materializedChartState{
		templateID: templateID,
		meta:       meta,
		lifecycle:  lifecycle,
		dimensions: make(map[string]*materializedDimensionState),
	}
	s.charts[chartID] = chart
	return chart, true
}

func (c *materializedChartState) ensureDimension(name string, state dimensionState) (*materializedDimensionState, bool) {
	dim, ok := c.dimensions[name]
	if ok {
		dim.hidden = state.hidden
		dim.static = state.static
		dim.order = state.order
		dim.algorithm = state.algorithm
		dim.multiplier = state.multiplier
		dim.divisor = state.divisor
		return dim, false
	}
	dim = &materializedDimensionState{
		name:       name,
		hidden:     state.hidden,
		static:     state.static,
		order:      state.order,
		algorithm:  state.algorithm,
		multiplier: state.multiplier,
		divisor:    state.divisor,
	}
	c.dimensions[name] = dim
	return dim, true
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
