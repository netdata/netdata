// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
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
	samples, err := c.collectQuerySamples(ctx, queryBatches, now)
	if err != nil {
		return err
	}

	observedThisCycle := c.observations.observeSamples(samples)
	c.observations.reobserveCachedObservations(dueInstruments, observedThisCycle)

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
		c.Warningf("resource discovery refresh failed, continuing with last known discovery snapshot: %v", err)
		resources = c.discovery.Resources
	}
	if c.discovery.FetchCounter != prevFetchCounter {
		c.observations.pruneStaleResources(c.discovery.ByProfile)
	}
	return len(resources) > 0, nil
}

func (c *Collector) collectQuerySamples(ctx context.Context, batches []queryBatch, queryNow time.Time) ([]metricSample, error) {
	if len(batches) == 0 {
		return nil, nil
	}

	results := c.queryExecutor.runQueryBatches(ctx, batches, queryNow, c.QueryOffset)

	var (
		allSamples []metricSample
		errCount   int
	)
	for _, res := range results {
		if res.Err != nil {
			errCount++
			c.Warningf("collection batch failed: %v", res.Err)
			continue
		}
		allSamples = append(allSamples, res.Samples...)
	}

	if errCount == len(results) {
		return nil, errors.New("all Azure Monitor batch queries failed")
	}

	return allSamples, nil
}
