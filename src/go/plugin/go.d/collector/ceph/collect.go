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
	var fsid string
	if err := c.apiClient.getJSON(ctx, "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &fsid); err != nil {
		if !isUnsupportedEndpointError(err) {
			return "", fmt.Errorf("get cluster identity: %w", err)
		}

		// TODO: Remove the Pacific 16 and Quincy 17 monitor fallback only when
		// the collector intentionally raises its minimum release to Reef 18.
		var monitor struct {
			MonStatus struct {
				MonMap struct {
					FSID string `json:"fsid"`
				} `json:"monmap"`
			} `json:"mon_status"`
		}
		if err := c.apiClient.getJSON(ctx, "get legacy cluster FSID", urlPathApiMonitor, hdrAcceptVersion, nil, &monitor); err != nil {
			return "", fmt.Errorf("get cluster identity: %w", err)
		}
		fsid = monitor.MonStatus.MonMap.FSID
	}
	if fsid == "" {
		return "", errors.New("get cluster identity: empty FSID")
	}

	c.identityMu.Lock()
	switch {
	case c.fsid == "":
		c.fsid = fsid
	case c.fsid != fsid:
		c.identityMu.Unlock()
		return "", errors.New("cluster identity changed after active-MGR discovery")
	}
	c.identityMu.Unlock()

	if !c.FunctionOnly {
		c.addClusterChartsOnce.Do(c.addClusterCharts)
	}
	return fsid, nil
}
