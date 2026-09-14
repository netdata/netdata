// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"time"
)

type poolMetricSample struct {
	name             string
	objects          int64
	available        int64
	used             int64
	utilizationRatio float64
	readOps          int64
	writeOps         int64
	readBytes        int64
	writtenBytes     int64
}

func (c *Collector) collectPools(ctx context.Context, fsid string) (bool, error) {
	var pools []apiPoolResponse
	if err := c.apiClient.getJSON(ctx, "list pool statistics", urlPathApiPool, hdrAcceptVersion,
		url.Values{
			"stats": {"true"},
		}, &pools); err != nil {
		return false, err
	}
	seenPoolNames := make(map[string]bool)
	for _, pool := range pools {
		if c.poolMatcher.MatchString(pool.PoolName) {
			if seenPoolNames[pool.PoolName] {
				return false, fmt.Errorf("list pool statistics returned a duplicate selected pool name")
			}
			seenPoolNames[pool.PoolName] = true
		}
	}

	selected := make([]poolMetricSample, 0, min(c.MaxPools, len(pools)))
	for _, pool := range pools {
		if !c.poolMatcher.MatchString(pool.PoolName) {
			continue
		}
		sample, err := poolSample(pool)
		if err != nil {
			return false, err
		}
		selected = append(selected, sample)
		if len(selected) > c.MaxPools {
			c.Limit("pool-cardinality", 1, time.Hour).Warningf(
				"selected pool count exceeds max_pools=%d; no per-pool metrics will be collected; narrow pool_selector or raise max_pools",
				c.MaxPools)
			return false, nil
		}
	}
	for _, sample := range selected {
		c.metrics.writePool(fsid, sample)
	}
	return len(selected) > 0, nil
}

func poolSample(pool apiPoolResponse) (poolMetricSample, error) {
	if pool.PoolName == "" {
		return poolMetricSample{}, fmt.Errorf("list pool statistics returned an empty pool name")
	}
	values := []struct {
		name  string
		value any
	}{
		{name: "objects", value: pool.Stats.Objects.Latest},
		{name: "available bytes", value: pool.Stats.AvailRaw.Latest},
		{name: "used bytes", value: pool.Stats.BytesUsed.Latest},
		{name: "read operations", value: pool.Stats.Reads.Latest},
		{name: "write operations", value: pool.Stats.Writes.Latest},
		{name: "read bytes", value: pool.Stats.ReadBytes.Latest},
		{name: "written bytes", value: pool.Stats.WrittenBytes.Latest},
	}
	parsed := make([]int64, len(values))
	for i, metric := range values {
		value, err := exactInt64(metric.value)
		if err != nil || value < 0 {
			return poolMetricSample{}, fmt.Errorf("list pool statistics returned invalid %s", metric.name)
		}
		parsed[i] = value
	}
	// Dashboard forwards Ceph's 0-1 df JSON ratio; only the ceph df text table converts it to percent.
	utilizationRatio, err := pool.Stats.PercentUsed.Latest.Float64()
	if err != nil || math.IsNaN(utilizationRatio) || math.IsInf(utilizationRatio, 0) || utilizationRatio < 0 ||
		utilizationRatio > 1 {
		return poolMetricSample{}, fmt.Errorf("list pool statistics returned utilization ratio outside the 0-1 range")
	}
	return poolMetricSample{
		name:             pool.PoolName,
		objects:          parsed[0],
		available:        parsed[1],
		used:             parsed[2],
		utilizationRatio: utilizationRatio,
		readOps:          parsed[3],
		writeOps:         parsed[4],
		readBytes:        parsed[5],
		writtenBytes:     parsed[6],
	}, nil
}
