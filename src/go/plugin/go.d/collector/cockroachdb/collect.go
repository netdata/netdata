// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func validCockroachDBMetrics(scraped prometheus.Series) bool {
	return scraped.FindByName("sql_restart_savepoint_count_internal").Len() > 0
}

func (c *Collector) collect() (map[string]int64, error) {
	scraped, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if !validCockroachDBMetrics(scraped) {
		return nil, errors.New("returned metrics aren't CockroachDB metrics")
	}

	mx := collectScraped(scraped, metrics)
	calcUsableCapacity(mx)
	calcUnusableCapacity(mx)
	calcTotalCapacityUsedPercentage(mx)
	calcUsableCapacityUsedPercentage(mx)
	calcRocksDBCacheHitRate(mx)
	calcActiveReplicas(mx)
	calcCPUUsagePercent(mx)

	return stm.ToMap(mx), nil
}

const precision = 1000

func collectScraped(scraped prometheus.Series, metricList []string) map[string]float64 {
	mx := make(map[string]float64)
	for _, name := range metricList {
		for _, m := range scraped.FindByName(name) {
			if isMetricFloat(name) {
				mx[name] += m.Value * precision
			} else {
				mx[name] += m.Value
			}
		}
	}
	return mx
}

func calcUsableCapacity(mx map[string]float64) {
	if !hasAll(mx, metricCapacityAvailable, metricCapacityUsed) {
		return
	}
	available := mx[metricCapacityAvailable]
	used := mx[metricCapacityUsed]

	mx[metricCapacityUsable] = available + used
}

func calcUnusableCapacity(mx map[string]float64) {
	if !hasAll(mx, metricCapacity, metricCapacityAvailable, metricCapacityUsed) {
		return
	}
	total := mx[metricCapacity]
	available := mx[metricCapacityAvailable]
	used := mx[metricCapacityUsed]

	mx[metricCapacityUnusable] = total - (available + used)
}

func calcTotalCapacityUsedPercentage(mx map[string]float64) {
	if !hasAll(mx, metricCapacity, metricCapacityUnusable, metricCapacityUsed) {
		return
	}
	total := mx[metricCapacity]
	unusable := mx[metricCapacityUnusable]
	used := mx[metricCapacityUsed]

	if mx[metricCapacity] == 0 {
		mx[metricCapacityUsedPercentage] = 0
	} else {
		mx[metricCapacityUsedPercentage] = (unusable + used) / total * 100 * precision
	}
}

func calcUsableCapacityUsedPercentage(mx map[string]float64) {
	if !hasAll(mx, metricCapacityUsable, metricCapacityUsed) {
		return
	}
	usable := mx[metricCapacityUsable]
	used := mx[metricCapacityUsed]

	if usable == 0 {
		mx[metricCapacityUsableUsedPercentage] = 0
	} else {
		mx[metricCapacityUsableUsedPercentage] = used / usable * 100 * precision
	}
}

func calcRocksDBCacheHitRate(mx map[string]float64) {
	if !hasAll(mx, metricRocksDBBlockCacheHits, metricRocksDBBlockCacheMisses) {
		return
	}
	hits := mx[metricRocksDBBlockCacheHits]
	misses := mx[metricRocksDBBlockCacheMisses]

	if sum := hits + misses; sum == 0 {
		mx[metricRocksDBBlockCacheHitRate] = 0
	} else {
		mx[metricRocksDBBlockCacheHitRate] = hits / sum * 100 * precision
	}
}

func calcActiveReplicas(mx map[string]float64) {
	if !hasAll(mx, metricReplicasQuiescent) {
		return
	}
	total := mx[metricReplicas]
	quiescent := mx[metricReplicasQuiescent]

	mx[metricReplicasActive] = total - quiescent
}

func calcCPUUsagePercent(mx map[string]float64) {
	if hasAll(mx, metricSysCPUUserPercent) {
		mx[metricSysCPUUserPercent] *= 100
	}
	if hasAll(mx, metricSysCPUSysPercent) {
		mx[metricSysCPUSysPercent] *= 100
	}
	if hasAll(mx, metricSysCPUCombinedPercentNormalized) {
		mx[metricSysCPUCombinedPercentNormalized] *= 100
	}
}

func isMetricFloat(name string) bool {
	// only Float metrics (see NewGaugeFloat64 in the cockroach repo):
	// - GcPausePercent, CPUUserPercent, CPUCombinedPercentNorm, AverageQueriesPerSecond, AverageWritesPerSecond
	switch name {
	case metricSysCPUUserPercent,
		metricSysCPUSysPercent,
		metricSysCPUCombinedPercentNormalized,
		metricRebalancingQueriesPerSecond,
		metricRebalancingWritesPerSecond:
		return true
	}
	return false
}

func hasAll(mx map[string]float64, key string, rest ...string) bool {
	_, ok := mx[key]
	if len(rest) == 0 {
		return ok
	}
	return ok && hasAll(mx, rest[0], rest[1:]...)
}
