// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"

// Public aliases for emitter/integration packages. Keep these stable to avoid
// leaking internal/program imports outside chartengine.
type (
	ChartMeta = program.ChartMeta
	Algorithm = program.Algorithm
	ChartType = program.ChartType
)

const (
	AlgorithmAbsolute    = program.AlgorithmAbsolute
	AlgorithmIncremental = program.AlgorithmIncremental
)

const (
	ChartTypeLine    = program.ChartTypeLine
	ChartTypeArea    = program.ChartTypeArea
	ChartTypeStacked = program.ChartTypeStacked
	ChartTypeHeatmap = program.ChartTypeHeatmap
)
