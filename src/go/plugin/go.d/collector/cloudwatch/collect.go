// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
	if err := c.ensurePlan(); err != nil {
		return err
	}
	if err := c.ensureTargets(ctx); err != nil {
		return err
	}
	if err := c.refreshDiscovery(ctx); err != nil {
		return err
	}
	c.refreshTags(ctx)

	plan, err := c.currentQueryPlan()
	if err != nil {
		return err
	}

	now := c.now()
	due := c.observations.dueQueries(plan, now)
	execution := c.executeQueries(ctx, due, now)
	// If the parent context was canceled or timed out during discovery/query, abort
	// the cycle instead of committing a partial/stale frame. A per-call GetMetricData
	// timeout uses a derived context, so it does not trip this and stays fail-soft.
	if err := ctx.Err(); err != nil {
		return err
	}

	c.observations.applyOutcomes(due, execution.outcomes, now, time.Duration(c.UpdateEvery)*time.Second)
	written := c.observations.emit(plan)

	c.Debugf("CloudWatch collect: %d planned quer(y/ies), %d due, %d terminal outcome(s), %d transient outcome(s), %d emitted sample(s)",
		len(plan), len(due), execution.terminal, execution.transient, written)
	if len(due) > 0 && execution.terminal == 0 && execution.transient == len(due) && written == 0 {
		return errors.New("all due CloudWatch queries failed transiently and no retained observations are available")
	}
	return nil
}
