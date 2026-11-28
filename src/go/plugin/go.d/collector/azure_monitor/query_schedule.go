// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"slices"
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

	var batches []queryBatch
	var byRegion map[subscriptionRegionKey][]resourceInfo
	for _, chunk := range profile.chunks {
		grain := chunk.TimeGrain
		due, checked := dueByGrain[grain]
		if !checked {
			due = !now.Before(c.nextCollectByGrain[grain])
			dueByGrain[grain] = due
			if due {
				c.nextCollectByGrain[grain] = now.Add(azureprofiles.SupportedTimeGrains[grain])
			}
		}
		if !due {
			continue
		}
		if byRegion == nil {
			byRegion = groupResourcesBySubscriptionRegion(resources)
		}
		for key, groupedResources := range byRegion {
			for resourceChunk := range slices.Chunk(groupedResources, c.Limits.MaxBatchResources) {
				batches = append(batches, queryBatch{
					chunk:          chunk,
					mask:           1<<len(chunk.metrics) - 1,
					SubscriptionID: key.SubscriptionID,
					Profile:        profile,
					Metrics:        chunk.metrics,
					MetricNames:    chunk.names,
					Aggregation:    chunk.Aggregation,
					TimeGrain:      grain,
					TimeGrainEvery: azureprofiles.SupportedTimeGrains[grain],
					Region:         key.Region,
					Resources:      resourceChunk,
				})
			}
		}
	}

	return batches
}

func batchMetricNames(metrics []*metricRuntime) []string {
	names := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		names = append(names, metric.AzureName)
	}
	return names
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
