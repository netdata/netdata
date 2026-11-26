// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func buildPlan(engine *Engine, reader metrix.Reader) (Plan, error) {
	return prepareAndCommitPlan(engine, reader)
}

// buildPlanAt plans and commits like a job run started at at.
func buildPlanAt(engine *Engine, reader metrix.Reader, at time.Time) (Plan, error) {
	attempt, err := engine.PreparePlanWithOptions(reader, PlanOptions{CollectedAt: at})
	if err != nil {
		return Plan{}, err
	}
	return attempt.Plan(), attempt.Commit()
}
