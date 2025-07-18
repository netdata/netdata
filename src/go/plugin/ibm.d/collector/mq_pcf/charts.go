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

