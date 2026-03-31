// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
	resources, err := c.refreshCollectResources(ctx)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return nil
	}

	now := c.now()
	queryBatches := c.buildQueryBatches(resources, now)

	dueInstruments := dueInstrumentsForBatches(queryBatches)
	samples, err := c.collectQuerySamples(ctx, queryBatches, queryEndForCollect(now, c.QueryOffset))
	if err != nil {
		return err
	}

	observedThisCycle := c.observations.observeSamples(samples)
	c.observations.reobserveCachedObservations(dueInstruments, observedThisCycle)

	return nil
}

func (c *Collector) refreshCollectResources(ctx context.Context) ([]resourceInfo, error) {
	if err := c.ensureBootstrapped(ctx); err != nil {
		return nil, err
	}

	prevFetchCounter := c.discovery.FetchCounter
	resources, err := c.refreshDiscovery(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("resource discovery: %w", err)
	}
	if c.discovery.FetchCounter != prevFetchCounter {
		c.observations.pruneStaleResources(resources)
	}
	return resources, nil
}

func (c *Collector) collectQuerySamples(ctx context.Context, batches []queryBatch, queryEnd time.Time) ([]metricSample, error) {
	if len(batches) == 0 {
		return nil, nil
	}

	results := c.queryExecutor.runQueryBatches(ctx, batches, queryEnd)

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

func queryEndForCollect(now time.Time, queryOffsetSeconds int) time.Time {
	queryEnd := now.Add(-secondsToDuration(queryOffsetSeconds))
	if queryEnd.IsZero() {
		return now
	}
	return queryEnd
}
