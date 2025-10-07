package mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
)

// collectSysTopics collects resource metrics using IBM MQ's resource monitoring system
func (c *Collector) collectSysTopics() error {
	if !c.Config.CollectSysTopics {
		// $SYS topic collection is disabled
		return nil
	}

	c.Debugf("Collecting metrics from $SYS topics using resource monitoring")

	// Check if resource monitoring is supported
	if !c.client.IsResourceMonitoringSupported() {
		c.Infof("Resource monitoring not supported on this queue manager (requires MQ V9+ on distributed platforms)")
		// Disable future collection attempts
		c.Config.CollectSysTopics = false
		return nil
	}

	// Enable resource monitoring on first use
	if err := c.client.EnableResourceMonitoring(); err != nil {
		c.Warningf("Failed to enable resource monitoring: %v", err)
		return nil // Don't fail the entire collection
	}

	// Get resource publications
	result, err := c.client.GetResourcePublications()
	if err != nil {
		c.Warningf("Failed to get resource publications: %v", err)
		return nil // Don't fail the entire collection
	}

	c.Debugf("Retrieved %d resource publications", result.Stats.Discovery.AvailableItems)

	// Process CPU metrics
	if result.UserCPUPercent.IsCollected() || result.SystemCPUPercent.IsCollected() {
		values := contexts.QueueManagerResourcesCPUUsageValues{}

		if result.UserCPUPercent.IsCollected() {
			// Convert percentage (0-100) to basis points (0-10000) for precision
			values.User = result.UserCPUPercent.Int64() * 100
		}
		if result.SystemCPUPercent.IsCollected() {
			// Convert percentage (0-100) to basis points (0-10000) for precision
			values.System = result.SystemCPUPercent.Int64() * 100
		}

		contexts.QueueManagerResources.CPUUsage.SetUpdateEvery(c.State, contexts.EmptyLabels{}, c.GetEffectiveSysTopicInterval())
		contexts.QueueManagerResources.CPUUsage.Set(c.State, contexts.EmptyLabels{}, values)
	}

	// Process memory metrics
	if result.MemoryUsedMB.IsCollected() {
		values := contexts.QueueManagerResourcesMemoryUsageValues{
			// Convert MB to bytes
			Total: result.MemoryUsedMB.Int64() * 1024 * 1024,
		}
		contexts.QueueManagerResources.MemoryUsage.SetUpdateEvery(c.State, contexts.EmptyLabels{}, c.GetEffectiveSysTopicInterval())
		contexts.QueueManagerResources.MemoryUsage.Set(c.State, contexts.EmptyLabels{}, values)
	}

	// Process log utilization metrics
	if result.LogUsedBytes.IsCollected() && result.LogMaxBytes.IsCollected() {
		used := result.LogUsedBytes.Int64()
		max := result.LogMaxBytes.Int64()

		// Calculate utilization percentage if both values are available
		if max > 0 {
			// Convert to 0.01% units (basis points) as expected by the context
			utilPercent := (used * 10000) / max
			contexts.QueueManagerResources.LogUtilization.SetUpdateEvery(c.State, contexts.EmptyLabels{}, c.GetEffectiveSysTopicInterval())
			contexts.QueueManagerResources.LogUtilization.Set(c.State, contexts.EmptyLabels{},
				contexts.QueueManagerResourcesLogUtilizationValues{
					Used: utilPercent,
				})
		}

		// Also set log file size
		contexts.QueueManagerResources.LogFileSize.SetUpdateEvery(c.State, contexts.EmptyLabels{}, c.GetEffectiveSysTopicInterval())
		contexts.QueueManagerResources.LogFileSize.Set(c.State, contexts.EmptyLabels{},
			contexts.QueueManagerResourcesLogFileSizeValues{
				Size: max,
			})
	}

	return nil
}
