
package mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

// Collector is the collector type.
type Collector struct {
	framework.Collector
	Config `yaml:",inline" json:",inline"`  // Embed config to receive YAML unmarshal
	client *pcf.Client
	
	// Resolved effective intervals (auto-detected or user-configured)
	effectiveStatisticsInterval int
	effectiveSysTopicInterval   int
}

// CollectOnce is called by the framework to collect metrics.
func (c *Collector) CollectOnce() error {
	if !c.client.IsConnected() {
		if err := c.client.Connect(); err != nil {
			return framework.ClassifyError(err, framework.ErrorTemporary)
		}
	}

	// Get MQ connection info and set as global labels
	version, edition, endpoint, err := c.client.GetConnectionInfo()
	if err == nil {
		c.SetGlobalLabel("version", version)
		c.SetGlobalLabel("edition", edition)
		c.SetGlobalLabel("endpoint", endpoint)
	} else {
		c.SetGlobalLabel("version", "unknown")
		c.SetGlobalLabel("edition", "unknown")
		c.SetGlobalLabel("endpoint", "unknown")
	}

	// Collect queue manager metrics
	if err := c.collectQueueManagerMetrics(); err != nil {
		return err
	}

	// Collect queue metrics
	if c.Config.CollectQueues {
		if err := c.collectQueueMetrics(); err != nil {
			c.Warningf("failed to collect queue metrics: %v", err)
		}
	}

	// Collect channel metrics
	if c.Config.CollectChannels {
		if err := c.collectChannelMetrics(); err != nil {
			c.Warningf("failed to collect channel metrics: %v", err)
		}
	}

	// Collect topic metrics
	if c.Config.CollectTopics {
		if err := c.collectTopicMetrics(); err != nil {
			c.Warningf("failed to collect topic metrics: %v", err)
		}
	}

	// Collect listener metrics
	if c.Config.CollectListeners {
		if err := c.collectListenerMetrics(); err != nil {
			c.Warningf("failed to collect listener metrics: %v", err)
		}
	}

	// Collect subscription metrics
	if c.Config.CollectSubscriptions {
		if err := c.collectSubscriptions(); err != nil {
			c.Warningf("failed to collect subscription metrics: %v", err)
		}
	}

	// Collect statistics queue metrics (advanced metrics) - every iteration
	if c.Config.CollectStatisticsQueue {
		if err := c.collectStatistics(); err != nil {
			c.Warningf("failed to collect statistics queue metrics: %v", err)
		}
	}

	// Collect $SYS topic metrics (resource metrics) - every iteration
	if c.Config.CollectSysTopics {
		if err := c.collectSysTopics(); err != nil {
			c.Warningf("failed to collect $SYS topic metrics: %v", err)
		}
	}

	return nil
}



func (c *Collector) collectQueueManagerMetrics() error {
	metrics, err := c.client.GetQueueManagerStatus()
	if err != nil {
		return err
	}
	
	contexts.QueueManager.Status.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerStatusValues{
		Status: metrics.Status,
	})
	
	// Collect connection count if available
	if metrics.ConnectionCount.IsCollected() {
		contexts.QueueManager.ConnectionCount.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerConnectionCountValues{
			Connections: metrics.ConnectionCount.Int64(),
		})
	}
	
	// Collect uptime if available
	if metrics.Uptime.IsCollected() {
		contexts.QueueManager.Uptime.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerUptimeValues{
			Uptime: metrics.Uptime.Int64(),
		})
	}
	
	return nil
}

// GetEffectiveStatisticsInterval returns the resolved statistics interval
// (auto-detected from STATINT or user-configured value)
func (c *Collector) GetEffectiveStatisticsInterval() int {
	return c.effectiveStatisticsInterval
}

// GetEffectiveSysTopicInterval returns the resolved $SYS topic interval
// (auto-detected from MONINT or user-configured value)
func (c *Collector) GetEffectiveSysTopicInterval() int {
	return c.effectiveSysTopicInterval
}

// resolveIntervals determines the effective intervals to use
func (c *Collector) resolveIntervals() {
	// Statistics interval: auto-detected STATINT overwrites user configuration
	if autoDetected := c.client.GetStatisticsInterval(); autoDetected > 0 {
		c.effectiveStatisticsInterval = int(autoDetected)
		c.Infof("Using auto-detected statistics interval from STATINT: %d seconds (overwrites config)", c.effectiveStatisticsInterval)
	} else {
		// Use configured value (default 60s)
		c.effectiveStatisticsInterval = c.Config.StatisticsInterval
		c.Infof("Using configured statistics interval: %d seconds (STATINT not available)", c.effectiveStatisticsInterval)
	}

	// $SYS topic interval: use configured value (default 10s, user can override)
	c.effectiveSysTopicInterval = c.Config.SysTopicInterval
	c.Infof("Using configured $SYS topic interval: %d seconds", c.effectiveSysTopicInterval)
}
