// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"fmt"
	"sort"

	catosdk "github.com/catonetworks/cato-go-sdk"
	"github.com/sourcegraph/conc/pool"
)

type accountMetricsResult struct {
	siteID string
	res    *catosdk.AccountMetrics
	err    error
}

func (c *Collector) collectMetrics(ctx context.Context, sites map[string]*siteState) error {
	ids := append([]string(nil), c.discovery.siteIDs...)
	if len(ids) == 0 {
		for id := range sites {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}

	p := pool.NewWithResults[accountMetricsResult]().WithMaxGoroutines(defaultMetricsParallel)
	for _, siteID := range ids {
		p.Go(func() accountMetricsResult {
			res, err := c.client.AccountMetrics(ctx, c.AccountID, []string{siteID}, defaultMetricsTimeFrame, defaultMetricsBuckets, nil)
			return accountMetricsResult{siteID: siteID, res: res, err: err}
		})
	}

	var errCount int
	for _, result := range p.Wait() {
		if result.err != nil {
			errCount++
			c.Debugf("accountMetrics query failed for site_id=%s, error_class=%s", result.siteID, classifyCatoError(result.err))
			continue
		}
		for _, issue := range mergeMetrics(result.res, sites) {
			c.logNormalizationIssue(normalizationSurfaceMetrics, issue)
		}
	}

	if errCount > 0 && errCount == len(ids) {
		return fmt.Errorf("all accountMetrics site queries failed")
	}
	if errCount > 0 {
		return fmt.Errorf("%d of %d accountMetrics site queries failed", errCount, len(ids))
	}
	return nil
}
