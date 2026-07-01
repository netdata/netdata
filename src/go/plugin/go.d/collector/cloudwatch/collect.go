// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import "context"

func (c *Collector) collect(ctx context.Context) error {
	if err := c.ensureAccountIdentity(ctx); err != nil {
		return err
	}
	if err := c.ensureProfiles(); err != nil {
		return err
	}
	if err := c.refreshDiscovery(ctx); err != nil {
		return err
	}

	plan := c.buildQueryPlan()
	c.observations.pruneObserved(plan)

	now := c.now()
	due := c.observations.dueGroups(plan, now)

	dueQueries := filterDueQueries(plan, due)
	samples, failed, err := c.executeQueries(ctx, dueQueries, now)
	if err != nil {
		return err
	}

	// A (region, period) group is "queried" only if it was due AND nothing failed
	// for it. Advance the schedule only for those; the rest (not due, or
	// due-but-failed) re-emit their cached values and remain due for retry.
	queried := make(map[queryGroupKey]bool, len(due))
	for key := range due {
		if !failed[key] {
			queried[key] = true
		}
	}
	c.observations.advanceSchedule(queried, now)
	c.observations.observe(dueQueries, samples, queried)

	c.Debugf("CloudWatch collect: %d planned quer(y/ies), %d group(s) due, %d sample(s), %d group(s) failed this cycle",
		len(plan), len(due), len(samples), len(failed))
	return nil
}
