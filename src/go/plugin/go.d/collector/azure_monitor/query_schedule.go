// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"slices"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func (c *Collector) buildQueryBatches(resources []resourceInfo, now time.Time) []queryBatch {
	dueByGrain := make(map[string]bool)
	resourcesByType := c.indexResourcesByType(resources)
	var batches []queryBatch

	for _, profile := range c.runtime.Profiles {
		batches = append(batches, c.buildProfileQueryBatches(profile, resourcesByType[stringsLowerTrim(profile.ResourceType)], dueByGrain, now)...)
	}

	return batches
}

func (c *Collector) indexResourcesByType(resources []resourceInfo) map[string][]resourceInfo {
	if len(c.discovery.ByType) > 0 {
		return c.discovery.ByType
	}

	result := make(map[string][]resourceInfo)
	for _, resource := range resources {
		result[stringsLowerTrim(resource.Type)] = append(result[stringsLowerTrim(resource.Type)], resource)
	}
	return result
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

		for region, regionResources := range groupResourcesByRegion(resources) {
			for metricChunk := range slices.Chunk(metrics, c.MaxMetricsPerQuery) {
				names := batchMetricNames(metricChunk)
				aggregations := batchAggregations(metricChunk)
				for resourceChunk := range slices.Chunk(regionResources, c.MaxBatchResources) {
					batches = append(batches, queryBatch{
						Profile:        profile,
						Metrics:        metricChunk,
						MetricNames:    append([]string(nil), names...),
						Aggregations:   append([]string(nil), aggregations...),
						TimeGrain:      grain,
						TimeGrainEvery: azureprofiles.SupportedTimeGrains[grain],
						Region:         region,
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

func groupResourcesByRegion(resources []resourceInfo) map[string][]resourceInfo {
	result := make(map[string][]resourceInfo)
	for _, r := range resources {
		region := normalizeRegion(r.Region)
		result[region] = append(result[region], r)
	}
	return result
}

func secondsToDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
