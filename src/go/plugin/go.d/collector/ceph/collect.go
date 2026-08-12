// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const precision = 1000

var (
	maxInt64Float       = math.Nextafter(float64(math.MaxInt64), 0)
	maxScaledInt64Float = math.Nextafter(float64(math.MaxInt64)/precision, 0)
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)

	if _, err := c.probeClusterIdentity(ctx); err != nil {
		return nil, err
	}
	c.addClusterChartsOnce.Do(c.addClusterCharts)

	healthErr := c.collectHealth(ctx, mx)

	var osdErr error
	if c.shouldSkipEntityCollection("osd", c.OSDSelector, mx["osds_num"], c.MaxOSDs) {
		c.suppressEntityMetrics("osd", c.seenOsds)
	} else {
		osdErr = c.collectOsds(ctx, mx)
	}

	var poolErr error
	if c.shouldSkipEntityCollection("pool", c.PoolSelector, mx["pools_num"], c.MaxPools) {
		c.suppressEntityMetrics("pool", c.seenPools)
	} else {
		poolErr = c.collectPools(ctx, mx)
	}

	err := errors.Join(
		wrapCollectionError("health", healthErr),
		wrapCollectionError("OSD", osdErr),
		wrapCollectionError("pool", poolErr),
	)
	if len(mx) == 0 {
		return nil, err
	}

	mx["health_collection_failed"] = boolInt64(healthErr != nil)
	mx["osd_collection_failed"] = boolInt64(osdErr != nil)
	mx["pool_collection_failed"] = boolInt64(poolErr != nil)
	return mx, err
}

func (c *Collector) shouldSkipEntityCollection(kind, selector string, count int64, limit int) bool {
	if strings.TrimSpace(selector) != "*" || count <= int64(limit) {
		return false
	}
	c.Limit(kind+"-cardinality", 1, time.Hour).Warningf(
		"cluster reports %d %ss, exceeding max_%ss=%d; no per-%s metrics will be collected; narrow %s_selector or raise max_%ss",
		count, kind, kind, limit, kind, kind, kind)
	return true
}

func wrapCollectionError(component string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("collect %s features: %w", component, err)
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (c *Collector) probeClusterIdentity(ctx context.Context) (string, error) {
	fsid, err := c.apiClient.probeClusterIdentity(ctx)
	if err != nil {
		return "", err
	}

	c.identityMu.Lock()
	c.fsid = fsid
	c.identityMu.Unlock()

	return fsid, nil
}
