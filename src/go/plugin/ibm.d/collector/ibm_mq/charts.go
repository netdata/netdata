//go:build ignore
// +build ignore

package ibm_mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var charts = module.Charts{
	"ibm_mq.queue.depth_current": {
		Title: "Queue Current Depth",
		Units: "messages",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.depth_current",
		Dims:  module.Dims{"depth": {Name: "depth", Algo: "gauge"}},
	},
	"ibm_mq.queue.depth_max": {
		Title: "Queue Max Depth",
		Units: "messages",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.depth_max",
		Dims:  module.Dims{"depth": {Name: "depth", Algo: "gauge"}},
	},
	"ibm_mq.queue.depth_percent": {
		Title: "Queue Depth Percent",
		Units: "percent",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.depth_percent",
		Dims:  module.Dims{"depth": {Name: "depth", Algo: "gauge"}},
	},
	"ibm_mq.queue.msg_deq_count": {
		Title: "Queue Dequeued Messages",
		Units: "messages",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.msg_deq_count",
		Dims:  module.Dims{"count": {Name: "count", Algo: "incremental"}},
	},
	"ibm_mq.queue.msg_enq_count": {
		Title: "Queue Enqueued Messages",
		Units: "messages",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.msg_enq_count",
		Dims:  module.Dims{"count": {Name: "count", Algo: "incremental"}},
	},
	"ibm_mq.queue.oldest_message_age": {
		Title: "Queue Oldest Message Age",
		Units: "seconds",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.oldest_message_age",
		Dims:  module.Dims{"age": {Name: "age", Algo: "gauge"}},
	},
	"ibm_mq.queue.open_input_count": {
		Title: "Queue Open Input Connections",
		Units: "connections",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.open_input_count",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.queue.open_output_count": {
		Title: "Queue Open Output Connections",
		Units: "connections",
		Fam:   "queue",
		Ctx:   "ibm_mq.queue.open_output_count",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.batches": {
		Title: "Channel Batches",
		Units: "batches",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.batches",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.buffers_rcvd": {
		Title: "Channel Buffers Received",
		Units: "buffers",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.buffers_rcvd",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.buffers_sent": {
		Title: "Channel Buffers Sent",
		Units: "buffers",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.buffers_sent",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.bytes_rcvd": {
		Title: "Channel Bytes Received",
		Units: "bytes",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.bytes_rcvd",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.bytes_sent": {
		Title: "Channel Bytes Sent",
		Units: "bytes",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.bytes_sent",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.current_msgs": {
		Title: "Channel In-Doubt Messages",
		Units: "messages",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.current_msgs",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.channel.msgs": {
		Title: "Channel Messages Sent or Received",
		Units: "messages",
		Fam:   "channel",
		Ctx:   "ibm_mq.channel.msgs",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.queue_manager.dist_lists": {
		Title: "Queue Manager Distribution Lists",
		Units: "lists",
		Fam:   "queue_manager",
		Ctx:   "ibm_mq.queue_manager.dist_lists",
		Dims:  module.Dims{"count": {Name: "count", Algo: "gauge"}},
	},
	"ibm_mq.queue_manager.max_msg_list": {
		Title: "Queue Manager Max Message Length",
		Units: "bytes",
		Fam:   "queue_manager",
		Ctx:   "ibm_mq.queue_manager.max_msg_list",
		Dims:  module.Dims{"length": {Name: "length", Algo: "gauge"}},
	},
}
