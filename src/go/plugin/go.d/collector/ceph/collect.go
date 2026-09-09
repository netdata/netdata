// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type metricCount struct {
	value   int64
	present bool
}

type healthCollectionResult struct {
	observed bool
	osds     metricCount
	pools    metricCount
}

func (c *Collector) collect(ctx context.Context) error {
	fsid, err := c.probeClusterIdentity(ctx)
	if err != nil {
		return err
	}
	clusterLabels := c.metrics.clusterLabels(fsid)

	health, healthErr := c.collectHealth(ctx, clusterLabels)

	var osdObserved bool
	var osdErr error
	if !c.shouldSkipEntityCollection("osd", c.OSDSelector, health.osds, c.MaxOSDs) {
		osdObserved, osdErr = c.collectOsds(ctx, fsid)
	}

	var poolObserved bool
	var poolErr error
	if !c.shouldSkipEntityCollection("pool", c.PoolSelector, health.pools, c.MaxPools) {
		poolObserved, poolErr = c.collectPools(ctx, fsid)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	err = errors.Join(
		wrapCollectionError("health", healthErr),
		wrapCollectionError("OSD", osdErr),
		wrapCollectionError("pool", poolErr),
	)
	if !health.observed && !osdObserved && !poolObserved {
		if err != nil {
			return err
		}
		return errors.New("no metrics collected")
	}

	c.metrics.writeComponentStatuses(clusterLabels, healthErr, osdErr, poolErr)
	if err != nil {
		// A component failure does not invalidate the other independently collected
		// samples. Returning nil commits the partial cycle and the status metrics.
		c.Limit("collection", 1, time.Minute).Error(err)
	}
	return nil
}

func (c *Collector) shouldSkipEntityCollection(kind, selector string, count metricCount, limit int) bool {
	if !count.present || strings.TrimSpace(selector) != "*" || count.value <= int64(limit) {
		return false
	}
	c.Limit(kind+"-cardinality", 1, time.Hour).Warningf(
		"cluster reports %d %ss, exceeding max_%ss=%d; no per-%s metrics will be collected; narrow %s_selector or raise max_%ss",
		count.value, kind, kind, limit, kind, kind, kind)
	return true
}

func wrapCollectionError(component string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("collect %s features: %w", component, err)
}

func (c *Collector) probeClusterIdentity(ctx context.Context) (string, error) {
	return c.apiClient.probeClusterIdentity(ctx)
}
