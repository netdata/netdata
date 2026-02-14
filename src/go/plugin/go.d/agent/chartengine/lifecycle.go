// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"

// materializedState tracks engine-owned chart lifecycle across successful cycles.
type materializedState struct {
	charts map[string]*materializedChartState
}

// materializedChartState tracks one materialized chart instance.
type materializedChartState struct {
	meta               program.ChartMeta
	lastSeenSuccessSeq uint64
	dimensions         map[string]*materializedDimensionState
}

// materializedDimensionState tracks one materialized dimension in a chart.
type materializedDimensionState struct {
	hidden             bool
	static             bool
	order              int
	lastSeenSuccessSeq uint64
}

func newMaterializedState() materializedState {
	return materializedState{
		charts: make(map[string]*materializedChartState),
	}
}

func (s *materializedState) ensureChart(chartID string, meta program.ChartMeta) (*materializedChartState, bool) {
	chart, ok := s.charts[chartID]
	if ok {
		return chart, false
	}
	chart = &materializedChartState{
		meta:       meta,
		dimensions: make(map[string]*materializedDimensionState),
	}
	s.charts[chartID] = chart
	return chart, true
}

func (c *materializedChartState) ensureDimension(name string, state dimensionState) (*materializedDimensionState, bool) {
	dim, ok := c.dimensions[name]
	if ok {
		return dim, false
	}
	dim = &materializedDimensionState{
		hidden: state.hidden,
		static: state.static,
		order:  state.order,
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
