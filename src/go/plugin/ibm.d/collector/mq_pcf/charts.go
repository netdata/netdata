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

	// Overview charts priority (higher priority = shown first)
	prioQueuesOverview
	prioChannelsOverview

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
	// Overview charts for monitoring status
	{
		ID:       "queues_overview",
		Title:    "Queues Monitoring Status",
		Units:    "queues",
		Fam:      "overview",
		Ctx:      "mq_pcf.queues_overview",
		Priority: prioQueuesOverview,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "queues_monitored", Name: "monitored"},
			{ID: "queues_excluded", Name: "excluded"},
			{ID: "queues_model", Name: "model"},
			{ID: "queues_unauthorized", Name: "unauthorized"},
			{ID: "queues_unknown", Name: "unknown"},
			{ID: "queues_failed", Name: "failed"},
		},
	},
	{
		ID:       "channels_overview",
		Title:    "Channels Monitoring Status",
		Units:    "channels",
		Fam:      "overview",
		Ctx:      "mq_pcf.channels_overview",
		Priority: prioChannelsOverview,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "channels_monitored", Name: "monitored"},
			{ID: "channels_excluded", Name: "excluded"},
			{ID: "channels_unauthorized", Name: "unauthorized"},
			{ID: "channels_failed", Name: "failed"},
		},
	},
	// Note: Queue Manager CPU usage is not available through standard PCF commands
	// CPU usage requires MQ resource monitoring which may not be enabled
	// Note: Queue Manager memory usage is not available through standard PCF commands
	// Memory usage requires MQ resource monitoring which may not be enabled
	// See collector_metrics_summary.md for details on available vs missing metrics
	// Note: Queue Manager log usage is not available through standard PCF commands
	// Log usage requires MQ resource monitoring which may not be enabled
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
		ID:       "queue_%s_config",
		Title:    "Queue Configuration Limits",
		Units:    "messages",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_config",
		Priority: prioQueueDepth + 1,
		Dims: module.Dims{
			{ID: "queue_%s_max_depth", Name: "max_depth"},
			{ID: "queue_%s_backout_threshold", Name: "backout_threshold"},
			{ID: "queue_%s_trigger_depth", Name: "trigger_depth"},
		},
	},
	{
		ID:       "queue_%s_inhibit",
		Title:    "Queue Inhibit Status",
		Units:    "status",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_inhibit",
		Priority: prioQueueDepth + 2,
		Dims: module.Dims{
			{ID: "queue_%s_inhibit_get", Name: "inhibit_get"},
			{ID: "queue_%s_inhibit_put", Name: "inhibit_put"},
		},
	},
	{
		ID:       "queue_%s_defaults",
		Title:    "Queue Default Settings",
		Units:    "value",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_defaults",
		Priority: prioQueueDepth + 3,
		Dims: module.Dims{
			{ID: "queue_%s_def_priority", Name: "priority"},
			{ID: "queue_%s_def_persistence", Name: "persistence"},
		},
	},
	{
		ID:       "queue_%s_activity",
		Title:    "Queue Activity Metrics",
		Units:    "connections",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_activity",
		Priority: prioQueueDepth + 4,
		Dims: module.Dims{
			{ID: "queue_%s_open_input_count", Name: "open_for_input"},
			{ID: "queue_%s_open_output_count", Name: "open_for_output"},
		},
	},
	{
		ID:       "queue_%s_messages",
		Title:    "Queue Message Counters",
		Units:    "messages",
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
	{
		ID:       "queue_%s_depth_events",
		Title:    "Queue Depth Event Thresholds",
		Units:    "messages",
		Fam:      "queues",
		Ctx:      "mq_pcf.queue_depth_events",
		Priority: prioQueueDepth + 5,
		Dims: module.Dims{
			{ID: "queue_%s_depth_high_limit", Name: "high_limit"},
			{ID: "queue_%s_depth_low_limit", Name: "low_limit"},
			{ID: "queue_%s_high_q_depth", Name: "peak_depth"},
		},
	},
}

// Template charts for channel instances
var channelChartsTmpl = module.Charts{
	{
		ID:       "channel_%s_status",
		Title:    "Channel Status",
		Units:    "boolean",
		Fam:      "channels/status",
		Ctx:      "mq_pcf.channel_status",
		Priority: prioChannelStatus,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "channel_%s_inactive", Name: "inactive"},
			{ID: "channel_%s_binding", Name: "binding"},
			{ID: "channel_%s_starting", Name: "starting"},
			{ID: "channel_%s_running", Name: "running"},
			{ID: "channel_%s_stopping", Name: "stopping"},
			{ID: "channel_%s_retrying", Name: "retrying"},
			{ID: "channel_%s_stopped", Name: "stopped"},
			{ID: "channel_%s_requesting", Name: "requesting"},
			{ID: "channel_%s_paused", Name: "paused"},
			{ID: "channel_%s_disconnected", Name: "disconnected"},
			{ID: "channel_%s_initializing", Name: "initializing"},
			{ID: "channel_%s_switching", Name: "switching"},
		},
	},
	{
		ID:       "channel_%s_batch_size",
		Title:    "Channel Batch Size",
		Units:    "messages",
		Fam:      "channels/config/batch",
		Ctx:      "mq_pcf.channel_batch_size",
		Priority: prioChannelStatus + 1,
		Dims: module.Dims{
			{ID: "channel_%s_batch_size", Name: "batch_size"},
		},
	},
	{
		ID:       "channel_%s_batch_interval",
		Title:    "Channel Batch Interval",
		Units:    "seconds",
		Fam:      "channels/config/batch",
		Ctx:      "mq_pcf.channel_batch_interval",
		Priority: prioChannelStatus + 2,
		Dims: module.Dims{
			{ID: "channel_%s_batch_interval", Name: "batch_interval", Div: precision},
		},
	},
	{
		ID:       "channel_%s_timeouts_config",
		Title:    "Channel Timeout Settings",
		Units:    "seconds",
		Fam:      "channels/config/timeout",
		Ctx:      "mq_pcf.channel_timeouts_config",
		Priority: prioChannelStatus + 3,
		Dims: module.Dims{
			{ID: "channel_%s_disc_interval", Name: "disconnect_interval", Div: precision},
			{ID: "channel_%s_hb_interval", Name: "heartbeat_interval", Div: precision},
			{ID: "channel_%s_keep_alive_interval", Name: "keep_alive_interval", Div: precision},
		},
	},
	{
		ID:       "channel_%s_retry_config",
		Title:    "Channel Retry Configuration",
		Units:    "value",
		Fam:      "channels/config/retry",
		Ctx:      "mq_pcf.channel_retry_config",
		Priority: prioChannelStatus + 4,
		Dims: module.Dims{
			{ID: "channel_%s_short_retry", Name: "short_retry_count"},
			{ID: "channel_%s_short_timer", Name: "short_retry_interval", Div: precision},
			{ID: "channel_%s_long_retry", Name: "long_retry_count"},
			{ID: "channel_%s_long_timer", Name: "long_retry_interval", Div: precision},
		},
	},
	{
		ID:       "channel_%s_limits_config",
		Title:    "Channel Limits",
		Units:    "value",
		Fam:      "channels/config/limit",
		Ctx:      "mq_pcf.channel_limits_config",
		Priority: prioChannelStatus + 5,
		Dims: module.Dims{
			{ID: "channel_%s_max_msg_length", Name: "max_message_length"},
			{ID: "channel_%s_sharing_conversations", Name: "sharing_conversations"},
			{ID: "channel_%s_network_priority", Name: "network_priority"},
		},
	},
	{
		ID:       "channel_%s_messages",
		Title:    "Channel Message Rate",
		Units:    "messages/s",
		Fam:      "channels/activity",
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
		Fam:      "channels/activity",
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
		Fam:      "channels/activity",
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
