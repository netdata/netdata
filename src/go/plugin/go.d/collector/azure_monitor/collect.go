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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	statesObserved := c.observeResourceStates(c.now())
	if err != nil && !statesObserved {
		return err
	}

	c.observations.observeSamples(samples)
	c.observations.reobserveCachedObservations(dueInstruments)

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

func (c *Collector) collectQuerySamples(ctx context.Context, batches []queryBatch, queryNow time.Time) ([]metricSample, error) {
	if len(batches) == 0 {
		return nil, nil
	}

	results := c.queryExecutor.runQueryBatches(ctx, batches, queryNow, c.QueryOffset)

	var (
		allSamples []metricSample
		errCount   int
	)
	for i, res := range results {
		batch := &batches[i]
		if res.Err != nil {
			errCount++
			c.logBatchFailure(batch, res.Err)
			continue
		}
		if d := res.Diagnostics; d != nil {
			c.Debugf("metrics batch: profile=%s region=%s subscription=%.80q grain=%s resources=%d with_samples=%d empty=%d missing=%d metric_errors=%d empty_examples=%v missing_examples=%v error_examples=%v",
				batch.Profile.Name, batch.Region, batch.SubscriptionID, batch.TimeGrain, len(batch.Resources),
				d.withSamples, d.empty, d.missing, d.metricErrors, d.emptyExamples, d.missingExamples, d.errorExamples)
		}
		allSamples = append(allSamples, res.Samples...)
	}

	if errCount == len(results) {
		return nil, errors.New("all Azure Monitor batch queries failed")
	}

	return allSamples, nil
}
