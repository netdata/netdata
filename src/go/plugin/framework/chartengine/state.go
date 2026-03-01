// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type engineState struct {
	cfg          engineConfig
	program      *program.Program
	matchIndex   matchIndex
	routeCache   *routeCache
	materialized materializedState
	hints        plannerSizingHints
	buildSeq     buildSeqState
	// plannerBuildSeq is runtime-mode build-cycle sequence used only by
	// per-build dedupe/scratch bookkeeping.
	plannerBuildSeq uint64
	stats           engineStats
	runtimeStore    metrix.RuntimeStore
	runtimeStats    *runtimeMetrics
	log             *logger.Logger
}

type plannerSizingHints struct {
	chartsByID int
	seenInfer  int
}

type buildSeqState struct {
	initialized bool
	violating   bool
	lastSuccess uint64
}
