// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"slices"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func (c *Collector) buildQueryBatches(now time.Time) []queryBatch {
	dueByGrain := make(map[string]bool)
	var batches []queryBatch

	for _, profile := range c.runtime.Profiles {
		batches = append(batches, c.buildProfileQueryBatches(profile, c.discovery.ByProfile[profile.Name], dueByGrain, now)...)
	}

	return batches
}

func (c *Collector) buildProfileQueryBatches(profile *profileRuntime, resources []resourceInfo, dueByGrain map[string]bool, now time.Time) []queryBatch {
	if len(resources) == 0 {
		return nil
	}

	metricsByGrain := c.dueMetricsByGrain(profile, dueByGrain, now)
	if len(metricsByGrain) == 0 {
		return nil
	}

	var batches []queryBatch
	for grain, metrics := range metricsByGrain {
		if len(metrics) == 0 {
			continue
		}

		for key, groupedResources := range groupResourcesBySubscriptionRegion(resources) {
			for metricChunk := range slices.Chunk(metrics, c.Limits.MaxMetricsPerQuery) {
				names := batchMetricNames(metricChunk)
				aggregations := batchAggregations(metricChunk)
				for resourceChunk := range slices.Chunk(groupedResources, c.Limits.MaxBatchResources) {
					batches = append(batches, queryBatch{
						SubscriptionID: key.SubscriptionID,
						Profile:        profile,
						Metrics:        metricChunk,
						MetricNames:    append([]string(nil), names...),
						Aggregations:   append([]string(nil), aggregations...),
						TimeGrain:      grain,
						TimeGrainEvery: azureprofiles.SupportedTimeGrains[grain],
						Region:         key.Region,
						Resources:      append([]resourceInfo(nil), resourceChunk...),
					})
				}
			}
		}
	}

	return batches
}

func (c *Collector) dueMetricsByGrain(profile *profileRuntime, dueByGrain map[string]bool, now time.Time) map[string][]*metricRuntime {
	metricsByGrain := make(map[string][]*metricRuntime)
	for _, metric := range profile.Metrics {
		due, ok := dueByGrain[metric.TimeGrain]
		if !ok {
			next, hasNext := c.nextCollectByGrain[metric.TimeGrain]
			due = !hasNext || !now.Before(next)
			dueByGrain[metric.TimeGrain] = due
			if due {
				c.nextCollectByGrain[metric.TimeGrain] = now.Add(metric.TimeGrainEvery)
			}
		}
		if !due {
			continue
		}
		metricsByGrain[metric.TimeGrain] = append(metricsByGrain[metric.TimeGrain], metric)
	}
	return metricsByGrain
}

func batchMetricNames(metrics []*metricRuntime) []string {
	names := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		names = append(names, metric.AzureName)
	}
	return names
}

func batchAggregations(metrics []*metricRuntime) []string {
	seen := make(map[string]struct{})
	list := make([]string, 0)
	for _, metric := range metrics {
		for _, series := range metric.Series {
			if _, ok := seen[series.Aggregation]; ok {
				continue
			}
			seen[series.Aggregation] = struct{}{}
			list = append(list, series.Aggregation)
		}
	}
	sort.Strings(list)
	return list
}

type subscriptionRegionKey struct {
	SubscriptionID string
	Region         string
}

func groupResourcesBySubscriptionRegion(resources []resourceInfo) map[subscriptionRegionKey][]resourceInfo {
	result := make(map[subscriptionRegionKey][]resourceInfo)
	for _, r := range resources {
		key := subscriptionRegionKey{
			SubscriptionID: stringsTrim(r.SubscriptionID),
			Region:         normalizeRegion(r.Region),
		}
		result[key] = append(result[key], r)
	}
	return result
}

func secondsToDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
