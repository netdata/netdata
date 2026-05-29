// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"fmt"
	"slices"
	"sort"
)

func (c *Collector) collectMetrics(ctx context.Context, sites map[string]*siteState) error {
	ids := append([]string(nil), c.discovery.siteIDs...)
	if len(ids) == 0 {
		for id := range sites {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}

	var errCount int
	var successCount int
	var batchCount int
	for batch := range slices.Chunk(ids, defaultMaxSitesPerQuery) {
		batchCount++
		res, err := c.client.AccountMetrics(ctx, c.AccountID, batch, defaultMetricsTimeFrame, defaultMetricsBuckets, nil)
		if err != nil {
			errCount++
			c.markOperationFailure(operationMetrics, err)
			c.markOperationAffectedSites(operationMetrics, err, len(batch))
			c.Debugf("accountMetrics batch failed for %d site(s), error_class=%s", len(batch), classifyCatoError(err))
			continue
		}
		successCount++
		for _, issue := range mergeMetrics(res, sites) {
			c.markNormalizationIssue(normalizationSurfaceMetrics, issue)
		}
	}

	if errCount == 0 && successCount > 0 {
		c.markOperationSuccess(operationMetrics)
	}
	if errCount > 0 && errCount == batchCount {
		return fmt.Errorf("all accountMetrics batches failed")
	}
	if errCount > 0 {
		return fmt.Errorf("%d of %d accountMetrics batches failed", errCount, batchCount)
	}
	return nil
}
