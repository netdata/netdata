// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	// Priority constants for chart ordering
	prioQueueManagerStatus = module.Priority + iota
	prioQueueManagerCPU
	prioQueueManagerMemory
	prioQueueManagerLog
	
	prioQueueDepth
	prioQueueMessages
	prioQueueAge
	prioQueueConnections
	
	prioChannelStatus
	prioChannelMessages
	prioChannelBytes
	prioChannelBatches
	
	prioTopicPublishers
	prioTopicSubscribers
	prioTopicMessages
)

// Base charts for queue manager level metrics
var baseCharts = module.Charts{
	{
		ID:       "qmgr_status",
		Title:    "Queue Manager Status",
		Units:    "status",
		Fam:      "queue_manager",
		Ctx:      "mq_pcf.qmgr_status",
		Priority: prioQueueManagerStatus,
		Dims: module.Dims{
			{ID: "qmgr_status", Name: "status"},
		},
	},
	{
		ID:       "qmgr_cpu_usage",
		Title:    "Queue Manager CPU Usage",
		Units:    "percentage",
		Fam:      "queue_manager",
		Ctx:      "mq_pcf.qmgr_cpu_usage",
		Priority: prioQueueManagerCPU,
		Dims: module.Dims{
			{ID: "qmgr_cpu_usage", Name: "cpu_usage", Div: precision},
		},
	},
	{
		ID:       "qmgr_memory_usage",
		Title:    "Queue Manager Memory Usage",
		Units:    "bytes",
		Fam:      "queue_manager",
		Ctx:      "mq_pcf.qmgr_memory_usage",
		Priority: prioQueueManagerMemory,
		Dims: module.Dims{
			{ID: "qmgr_memory_usage", Name: "memory_usage"},
		},
	},
	{
		ID:       "qmgr_log_usage",
		Title:    "Queue Manager Log Usage",
		Units:    "percentage",
		Fam:      "queue_manager",
		Ctx:      "mq_pcf.qmgr_log_usage",
		Priority: prioQueueManagerLog,
		Dims: module.Dims{
			{ID: "qmgr_log_usage", Name: "log_usage", Div: precision},
		},
	},
}

// Template charts for queue instances
var queueChartsTmpl = module.Charts{
	{
		ID:       "queue_%s_depth",
		Title:    "Queue Current Depth",
		Units:    "messages",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_depth",
		Priority: prioQueueDepth,
		Dims: module.Dims{
			{ID: "queue_%s_depth", Name: "depth"},
		},
	},
	{
		ID:       "queue_%s_messages",
		Title:    "Queue Message Rate",
		Units:    "messages/s",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_messages",
		Priority: prioQueueMessages,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "queue_%s_enqueued", Name: "enqueued", Algo: module.Incremental},
			{ID: "queue_%s_dequeued", Name: "dequeued", Algo: module.Incremental},
		},
	},
	{
		ID:       "queue_%s_oldest_message_age",
		Title:    "Queue Oldest Message Age",
		Units:    "seconds",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_oldest_message_age",
		Priority: prioQueueAge,
		Dims: module.Dims{
			{ID: "queue_%s_oldest_message_age", Name: "age"},
		},
	},
}

// Template charts for channel instances
var channelChartsTmpl = module.Charts{
	{
		ID:       "channel_%s_status",
		Title:    "Channel Status",
		Units:    "status",
		Fam:      "channels",
		Ctx:      "mq_pcf.channel_status",
		Priority: prioChannelStatus,
		Dims: module.Dims{
			{ID: "channel_%s_status", Name: "status"},
		},
	},
	{
		ID:       "channel_%s_messages",
		Title:    "Channel Message Rate",
		Units:    "messages/s",
		Fam:      "channels",
		Ctx:      "mq_pcf.channel_messages",
		Priority: prioChannelMessages,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "channel_%s_messages", Name: "messages", Algo: module.Incremental},
		},
	},
	{
		ID:       "channel_%s_bytes",
		Title:    "Channel Data Transfer Rate",
		Units:    "bytes/s",
		Fam:      "channels",
		Ctx:      "mq_pcf.channel_bytes",
		Priority: prioChannelBytes,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "channel_%s_bytes", Name: "bytes", Algo: module.Incremental},
		},
	},
	{
		ID:       "channel_%s_batches",
		Title:    "Channel Batch Rate",
		Units:    "batches/s",
		Fam:      "channels",
		Ctx:      "mq_pcf.channel_batches",
		Priority: prioChannelBatches,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "channel_%s_batches", Name: "batches", Algo: module.Incremental},
		},
	},
}

// Template charts for topic instances
var topicChartsTmpl = module.Charts{
	{
		ID:       "topic_%s_publishers",
		Title:    "Topic Publishers",
		Units:    "publishers",
		Fam:      "topics",
		Ctx:      "mq_pcf.topic_publishers",
		Priority: prioTopicPublishers,
		Dims: module.Dims{
			{ID: "topic_%s_publishers", Name: "publishers"},
		},
	},
	{
		ID:       "topic_%s_subscribers",
		Title:    "Topic Subscribers",
		Units:    "subscribers",
		Fam:      "topics",
		Ctx:      "mq_pcf.topic_subscribers",
		Priority: prioTopicSubscribers,
		Dims: module.Dims{
			{ID: "topic_%s_subscribers", Name: "subscribers"},
		},
	},
	{
		ID:       "topic_%s_messages",
		Title:    "Topic Message Rate",
		Units:    "messages/s",
		Fam:      "topics",
		Ctx:      "mq_pcf.topic_messages",
		Priority: prioTopicMessages,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "topic_%s_messages", Name: "messages", Algo: module.Incremental},
		},
	},
}

// newInstanceCharts creates charts for a dynamic instance (queue, channel, topic)
func (c *Collector) newInstanceCharts(tmpl module.Charts, instanceName, labelKey string) *module.Charts {
	charts := tmpl.Copy()
	cleanName := c.cleanName(instanceName)
	
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: labelKey, Value: instanceName},
		}
		
		// Add version labels if available
		if c.version != "" && c.edition != "" {
			chart.Labels = append(chart.Labels, 
				module.Label{Key: "mq_version", Value: c.version},
				module.Label{Key: "mq_edition", Value: c.edition},
			)
		}
		
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}
	
	return charts
}

func (c *Collector) newQueueCharts(queueName string) *module.Charts {
	return c.newInstanceCharts(queueChartsTmpl, queueName, "queue")
}

func (c *Collector) newChannelCharts(channelName string) *module.Charts {
	return c.newInstanceCharts(channelChartsTmpl, channelName, "channel")
}

func (c *Collector) newTopicCharts(topicName string) *module.Charts {
	return c.newInstanceCharts(topicChartsTmpl, topicName, "topic")
}