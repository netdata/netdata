// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"time"
)

const maxPoolInventoryRecords = 100000

type poolMetricSample struct {
	key          string
	objects      int64
	available    int64
	used         int64
	utilization  float64
	readOps      int64
	writeOps     int64
	readBytes    int64
	writtenBytes int64
}

func (c *Collector) collectPools(ctx context.Context, mx map[string]int64) error {
	var pools []apiPoolResponse
	if err := c.apiClient.getJSON(ctx, "list pool statistics", urlPathApiPool, hdrAcceptVersion,
		url.Values{"stats": {"true"}}, &pools); err != nil {
		return err
	}
	if len(pools) > maxPoolInventoryRecords {
		return fmt.Errorf("list pool statistics exceeds the record limit of %d", maxPoolInventoryRecords)
	}

	sort.SliceStable(pools, func(i, j int) bool { return pools[i].PoolName < pools[j].PoolName })
	seenPoolNames := make(map[string]bool)
	for _, pool := range pools {
		if c.poolMatcher.MatchString(pool.PoolName) {
			if seenPoolNames[pool.PoolName] {
				return fmt.Errorf("list pool statistics returned a duplicate selected pool name")
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
			return err
		}
		selected = append(selected, sample)
		if len(selected) > c.MaxPools {
			c.suppressEntityMetrics("pool", c.seenPools)
			c.Limit("pool-cardinality", 1, time.Hour).Warningf(
				"selected pool count exceeds max_pools=%d; no per-pool metrics will be collected; narrow pool_selector or raise max_pools",
				c.MaxPools)
			return nil
		}
	}
	if err := validatePoolChartKeys(selected); err != nil {
		return err
	}

	now := c.now()
	seen := make(map[string]bool, len(selected))
	for _, sample := range selected {
		seen[sample.key] = true
		if _, ok := c.seenPools[sample.key]; !ok {
			c.seenPools[sample.key] = &entityState{}
			c.addPoolCharts(sample.key)
		}
		c.seenPools[sample.key].lastSeen = now
		emitPoolMetrics(mx, sample)
	}
	c.expireMissingEntities("pool", c.seenPools, seen, now)
	return nil
}

func validatePoolChartKeys(samples []poolMetricSample) error {
	seen := make(map[string]bool, len(samples))
	for _, sample := range samples {
		key := cleanChartID("pool_" + sample.key + "_")
		if seen[key] {
			return fmt.Errorf("selected pool names collide after legacy chart-ID normalization")
		}
		seen[key] = true
	}
	return nil
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
	utilization, err := pool.Stats.PercentUsed.Latest.Float64()
	if err != nil || math.IsNaN(utilization) || math.IsInf(utilization, 0) || utilization < 0 || utilization > 1 {
		return poolMetricSample{}, fmt.Errorf("list pool statistics returned utilization outside the 0-1 range")
	}
	return poolMetricSample{
		key: pool.PoolName, objects: parsed[0], available: parsed[1], used: parsed[2], utilization: utilization,
		readOps: parsed[3], writeOps: parsed[4], readBytes: parsed[5], writtenBytes: parsed[6],
	}, nil
}

func emitPoolMetrics(mx map[string]int64, sample poolMetricSample) {
	px := "pool_" + sample.key + "_"
	mx[px+"objects"] = sample.objects
	mx[px+"space_used_bytes"] = sample.used
	mx[px+"space_avail_bytes"] = sample.available
	mx[px+"space_utilization"] = int64(sample.utilization * 100 * precision)
	mx[px+"read_ops"] = sample.readOps
	mx[px+"read_bytes"] = sample.readBytes
	mx[px+"write_ops"] = sample.writeOps
	mx[px+"written_bytes"] = sample.writtenBytes
}
