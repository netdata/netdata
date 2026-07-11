// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import "context"

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
	c.refreshTags(ctx) // best-effort tag enrichment; never gates collection (INV.2)

	plan := c.currentQueryPlan()

	now := c.now()
	due := c.observations.dueGroups(c.queryGroups, now)

	dueQueries := filterDueQueries(c.queryGroups, c.queriesByGroup, due)
	samples, noData, failed, err := c.executeQueries(ctx, dueQueries, now)
	if err != nil {
		return err
	}
	// If the parent context was canceled or timed out during discovery/query, abort
	// the cycle instead of committing a partial/stale frame. A per-call GetMetricData
	// timeout uses a derived context, so it does not trip this and stays fail-soft.
	if err := ctx.Err(); err != nil {
		return err
	}

	// A (target, region, period) group is "queried" only if it was due AND nothing failed
	// for it. Advance the schedule only for those; the rest (not due, or
	// due-but-failed) re-emit their cached values and remain due for retry.
	queried := make(map[queryGroupKey]bool, len(due))
	for key := range due {
		if !failed[key] {
			queried[key] = true
		}
	}
	c.observations.advanceSchedule(queried, now)
	c.observations.observe(dueQueries, samples, noData, queried)

	c.Debugf("CloudWatch collect: %d planned quer(y/ies), %d group(s) due, %d sample(s), %d group(s) failed this cycle",
		len(plan), len(due), len(samples), len(failed))
	return nil
}
