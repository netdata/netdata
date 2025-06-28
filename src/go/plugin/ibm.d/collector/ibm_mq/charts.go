// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package ibm_mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	// Priority constants for chart ordering
	prioQueueDepth = module.Priority + iota
	prioQueueDepthMax
	prioQueueDepthPercent
	prioQueueMessages
	prioQueueConnections
	prioQueueAge
	
	prioChannelBatches
	prioChannelBuffers
	prioChannelBytes
	prioChannelMessages
	
	prioQueueManagerLists
	prioQueueManagerMsgLength
)

var baseCharts = module.Charts{
	{
		ID:       "queue_depth_current",
		Title:    "Queue Current Depth",
		Units:    "messages",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_depth_current",
		Priority: prioQueueDepth,
		Dims: module.Dims{
			{ID: "depth", Name: "depth"},
		},
	},
	{
		ID:       "queue_depth_max",
		Title:    "Queue Max Depth",
		Units:    "messages",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_depth_max",
		Priority: prioQueueDepthMax,
		Dims: module.Dims{
			{ID: "depth", Name: "depth"},
		},
	},
	{
		ID:       "queue_depth_percent",
		Title:    "Queue Depth Percent",
		Units:    "percent",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_depth_percent",
		Priority: prioQueueDepthPercent,
		Dims: module.Dims{
			{ID: "depth", Name: "depth"},
		},
	},
	{
		ID:       "queue_msg_deq_count",
		Title:    "Queue Dequeued Messages",
		Units:    "messages/s",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_msg_deq_count",
		Priority: prioQueueMessages,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "dequeued", Name: "dequeued", Algo: module.Incremental},
		},
	},
	{
		ID:       "queue_msg_enq_count",
		Title:    "Queue Enqueued Messages",
		Units:    "messages/s",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_msg_enq_count",
		Priority: prioQueueMessages + 1,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "enqueued", Name: "enqueued", Algo: module.Incremental},
		},
	},
	{
		ID:       "queue_oldest_message_age",
		Title:    "Queue Oldest Message Age",
		Units:    "seconds",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_oldest_message_age",
		Priority: prioQueueAge,
		Dims: module.Dims{
			{ID: "age", Name: "age"},
		},
	},
	{
		ID:       "queue_open_input_count",
		Title:    "Queue Open Input Connections",
		Units:    "connections",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_open_input_count",
		Priority: prioQueueConnections,
		Dims: module.Dims{
			{ID: "input", Name: "input"},
		},
	},
	{
		ID:       "queue_open_output_count",
		Title:    "Queue Open Output Connections",
		Units:    "connections",
		Fam:      "queue",
		Ctx:      "ibm_mq.queue_open_output_count",
		Priority: prioQueueConnections + 1,
		Dims: module.Dims{
			{ID: "output", Name: "output"},
		},
	},
	{
		ID:       "channel_batches",
		Title:    "Channel Batches",
		Units:    "batches",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_batches",
		Priority: prioChannelBatches,
		Dims: module.Dims{
			{ID: "batches", Name: "batches"},
		},
	},
	{
		ID:       "channel_buffers_rcvd",
		Title:    "Channel Buffers Received",
		Units:    "buffers",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_buffers_rcvd",
		Priority: prioChannelBuffers,
		Dims: module.Dims{
			{ID: "received", Name: "received"},
		},
	},
	{
		ID:       "channel_buffers_sent",
		Title:    "Channel Buffers Sent",
		Units:    "buffers",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_buffers_sent",
		Priority: prioChannelBuffers + 1,
		Dims: module.Dims{
			{ID: "sent", Name: "sent"},
		},
	},
	{
		ID:       "channel_bytes_rcvd",
		Title:    "Channel Bytes Received",
		Units:    "bytes",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_bytes_rcvd",
		Priority: prioChannelBytes,
		Dims: module.Dims{
			{ID: "received", Name: "received"},
		},
	},
	{
		ID:       "channel_bytes_sent",
		Title:    "Channel Bytes Sent",
		Units:    "bytes",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_bytes_sent",
		Priority: prioChannelBytes + 1,
		Dims: module.Dims{
			{ID: "sent", Name: "sent"},
		},
	},
	{
		ID:       "channel_current_msgs",
		Title:    "Channel In-Doubt Messages",
		Units:    "messages",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_current_msgs",
		Priority: prioChannelMessages,
		Dims: module.Dims{
			{ID: "in_doubt", Name: "in_doubt"},
		},
	},
	{
		ID:       "channel_msgs",
		Title:    "Channel Messages Sent or Received",
		Units:    "messages",
		Fam:      "channel",
		Ctx:      "ibm_mq.channel_msgs",
		Priority: prioChannelMessages + 1,
		Dims: module.Dims{
			{ID: "total", Name: "total"},
		},
	},
	{
		ID:       "queue_manager_dist_lists",
		Title:    "Queue Manager Distribution Lists",
		Units:    "lists",
		Fam:      "queue_manager",
		Ctx:      "ibm_mq.queue_manager_dist_lists",
		Priority: prioQueueManagerLists,
		Dims: module.Dims{
			{ID: "dist_lists", Name: "dist_lists"},
		},
	},
	{
		ID:       "queue_manager_max_msg_list",
		Title:    "Queue Manager Max Message Length",
		Units:    "bytes",
		Fam:      "queue_manager",
		Ctx:      "ibm_mq.queue_manager_max_msg_list",
		Priority: prioQueueManagerMsgLength,
		Dims: module.Dims{
			{ID: "max_length", Name: "max_length"},
		},
	},
}