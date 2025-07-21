package mq

import (
	"strings"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

// collectSysTopics collects resource metrics from $SYS topics
// This provides Queue Manager CPU, memory, and log utilization metrics
func (c *Collector) collectSysTopics() error {
	if !c.Config.CollectSysTopics {
		// $SYS topic collection is disabled
		return nil
	}

	c.Debugf("Collecting metrics from $SYS topics")

	// Get the list of topics to monitor
	// Replace QM1 with actual queue manager name
	topics := []string{
		"$SYS/MQ/INFO/QMGR/" + c.Config.QueueManager + "/ResourceStatistics/QueueManager",
		"$SYS/MQ/INFO/QMGR/" + c.Config.QueueManager + "/Log/Log_Utilization_Summary",
	}

	// Collect messages from $SYS topics
	result, err := c.client.GetSysTopicMessages(topics)
	if err != nil {
		c.Warningf("Failed to collect $SYS topic messages: %v", err)
		return nil // Don't fail the entire collection
	}

	c.Debugf("Retrieved %d messages from $SYS topics", len(result.Messages))

	// Process the messages by topic type
	for _, msg := range result.Messages {
		switch {
		case strings.Contains(msg.Topic, "ResourceStatistics/QueueManager"):
			c.processSysResourceStats(msg)
		case strings.Contains(msg.Topic, "Log_Utilization_Summary"):
			c.processSysLogUtilization(msg)
		}
	}

	return nil
}

// processSysResourceStats processes Queue Manager resource statistics from $SYS topics
func (c *Collector) processSysResourceStats(msg pcf.SysTopicMessage) {
	// Since we don't know the exact attribute IDs, we log what we find
	// and skip metrics if attributes aren't available
	c.Debugf("Processing $SYS resource statistics with %d attributes", len(msg.Data))
	
	// For now, we'll skip metric collection until we can determine the correct attribute IDs
	// from an actual MQ instance with $SYS topics enabled
	c.Warningf("$SYS topic resource metrics are not yet implemented - unknown attribute mappings")
	
	// Log all attributes for debugging to help identify the correct ones
	for key, value := range msg.Data {
		c.Debugf("  Resource stat attribute %v = %v", key, value)
	}
}

// processSysLogUtilization processes log utilization metrics from $SYS topics
func (c *Collector) processSysLogUtilization(msg pcf.SysTopicMessage) {
	// Since we don't know the exact attribute IDs, we log what we find
	c.Debugf("Processing $SYS log utilization with %d attributes", len(msg.Data))
	
	// For now, we'll skip metric collection until we can determine the correct attribute IDs
	c.Warningf("$SYS topic log metrics are not yet implemented - unknown attribute mappings")
	
	// Log all attributes for debugging
	for key, value := range msg.Data {
		c.Debugf("  Log stat attribute %v = %v", key, value)
	}
}