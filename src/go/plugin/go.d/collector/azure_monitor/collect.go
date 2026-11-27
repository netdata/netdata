// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
	// Reserve the closing part of the interval for publishing and the next scheduled tick.
	interval := secondsToDuration(c.UpdateEvery)
	recoveryDeadline := time.Now().Add(interval - interval/10)
	hasResources, err := c.refreshCollectResources(ctx)
	if err != nil {
		return err
	}
	if !hasResources {
		return nil
	}

	now := c.now()
	queryBatches := c.buildQueryBatches(now)

	dueInstruments := dueInstrumentsForBatches(queryBatches)
	samples, err := c.collectQuerySamples(ctx, queryBatches, now, recoveryDeadline)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	statesObserved := c.observeResourceStates(c.now())
	cachedObserved := c.observations.reobserveCachedObservations(dueInstruments)
	if err != nil && !statesObserved && !cachedObserved {
		return err
	}

	c.observations.observeSamples(samples)

	return nil
}

func (c *Collector) refreshCollectResources(ctx context.Context) (bool, error) {
	if err := c.ensureBootstrapped(ctx); err != nil {
		return false, err
	}

	prevFetchCounter := c.discovery.FetchCounter
	resources, err := c.refreshDiscovery(ctx, false)
	if err != nil {
		if c.discovery.FetchedAt.IsZero() {
			return false, fmt.Errorf("resource discovery: %w", err)
		}
		c.Limit("discovery:refresh", 1, time.Hour).Warningf("resource discovery refresh failed, retaining inventory without stale resource states: %v", err)
		resources = c.discovery.Resources
	}
	if c.discovery.FetchCounter != prevFetchCounter || c.observations.resources == nil {
		c.observations.reconcileResources(c.discovery.ByProfile)
	}
	return len(resources) > 0, nil
}
