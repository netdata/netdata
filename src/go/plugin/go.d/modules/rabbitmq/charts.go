// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioMessagesCount = module.Priority + iota
	prioMessagesRate

	prioObjectsCount

	prioConnectionChurnRate
	prioChannelChurnRate
	prioQueueChurnRate

	prioFileDescriptorsCount
	prioSocketsCount
	prioErlangProcessesCount
	prioErlangRunQueueProcessesCount
	prioMemoryUsage
	prioDiskSpaceFreeSize

	prioVhostMessagesCount
	prioVhostMessagesRate

	prioQueueMessagesCount
	prioQueueMessagesRate
)

var baseCharts = module.Charts{
	chartMessagesCount.Copy(),
	chartMessagesRate.Copy(),

	chartObjectsCount.Copy(),

	chartConnectionChurnRate.Copy(),
	chartChannelChurnRate.Copy(),
	chartQueueChurnRate.Copy(),

	chartFileDescriptorsCount.Copy(),
	chartSocketsCount.Copy(),
	chartErlangProcessesCount.Copy(),
	chartErlangRunQueueProcessesCount.Copy(),
	chartMemoryUsage.Copy(),
	chartDiskSpaceFreeSize.Copy(),
}

var chartsTmplVhost = module.Charts{
	chartTmplVhostMessagesCount.Copy(),
	chartTmplVhostMessagesRate.Copy(),
}

var chartsTmplQueue = module.Charts{
	chartTmplQueueMessagesCount.Copy(),
	chartTmplQueueMessagesRate.Copy(),
}

var (
	chartMessagesCount = module.Chart{
		ID:       "messages_count",
		Title:    "Messages",
		Units:    "messages",
		Fam:      "messages",
		Ctx:      "rabbitmq.messages_count",
		Type:     module.Stacked,
		Priority: prioMessagesCount,
		Dims: module.Dims{
			{ID: "queue_totals_messages_ready", Name: "ready"},
			{ID: "queue_totals_messages_unacknowledged", Name: "unacknowledged"},
		},
	}
	chartMessagesRate = module.Chart{
		ID:       "messages_rate",
		Title:    "Messages",
		Units:    "messages/s",
		Fam:      "messages",
		Ctx:      "rabbitmq.messages_rate",
		Priority: prioMessagesRate,
		Dims: module.Dims{
			{ID: "message_stats_ack", Name: "ack", Algo: module.Incremental},
			{ID: "message_stats_publish", Name: "publish", Algo: module.Incremental},
			{ID: "message_stats_publish_in", Name: "publish_in", Algo: module.Incremental},
			{ID: "message_stats_publish_out", Name: "publish_out", Algo: module.Incremental},
			{ID: "message_stats_confirm", Name: "confirm", Algo: module.Incremental},
			{ID: "message_stats_deliver", Name: "deliver", Algo: module.Incremental},
			{ID: "message_stats_deliver_no_ack", Name: "deliver_no_ack", Algo: module.Incremental},
			{ID: "message_stats_get", Name: "get", Algo: module.Incremental},
			{ID: "message_stats_get_no_ack", Name: "get_no_ack", Algo: module.Incremental},
			{ID: "message_stats_deliver_get", Name: "deliver_get", Algo: module.Incremental},
			{ID: "message_stats_redeliver", Name: "redeliver", Algo: module.Incremental},
			{ID: "message_stats_return_unroutable", Name: "return_unroutable", Algo: module.Incremental},
		},
	}
	chartObjectsCount = module.Chart{
		ID:       "objects_count",
		Title:    "Objects",
		Units:    "objects",
		Fam:      "objects",
		Ctx:      "rabbitmq.objects_count",
		Priority: prioObjectsCount,
		Dims: module.Dims{
			{ID: "object_totals_channels", Name: "channels"},
			{ID: "object_totals_consumers", Name: "consumers"},
			{ID: "object_totals_connections", Name: "connections"},
			{ID: "object_totals_queues", Name: "queues"},
			{ID: "object_totals_exchanges", Name: "exchanges"},
		},
	}

	chartConnectionChurnRate = module.Chart{
		ID:       "connection_churn_rate",
		Title:    "Connection churn",
		Units:    "operations/s",
		Fam:      "churn",
		Ctx:      "rabbitmq.connection_churn_rate",
		Priority: prioConnectionChurnRate,
		Dims: module.Dims{
			{ID: "churn_rates_connection_created", Name: "created", Algo: module.Incremental},
			{ID: "churn_rates_connection_closed", Name: "closed", Algo: module.Incremental},
		},
	}
	chartChannelChurnRate = module.Chart{
		ID:       "channel_churn_rate",
		Title:    "Channel churn",
		Units:    "operations/s",
		Fam:      "churn",
		Ctx:      "rabbitmq.channel_churn_rate",
		Priority: prioChannelChurnRate,
		Dims: module.Dims{
			{ID: "churn_rates_channel_created", Name: "created", Algo: module.Incremental},
			{ID: "churn_rates_channel_closed", Name: "closed", Algo: module.Incremental},
		},
	}
	chartQueueChurnRate = module.Chart{
		ID:       "queue_churn_rate",
		Title:    "Queue churn",
		Units:    "operations/s",
		Fam:      "churn",
		Ctx:      "rabbitmq.queue_churn_rate",
		Priority: prioQueueChurnRate,
		Dims: module.Dims{
			{ID: "churn_rates_queue_created", Name: "created", Algo: module.Incremental},
			{ID: "churn_rates_queue_deleted", Name: "deleted", Algo: module.Incremental},
			{ID: "churn_rates_queue_declared", Name: "declared", Algo: module.Incremental},
		},
	}
)

var (
	chartFileDescriptorsCount = module.Chart{
		ID:       "file_descriptors_count",
		Title:    "File descriptors",
		Units:    "fd",
		Fam:      "node stats",
		Ctx:      "rabbitmq.file_descriptors_count",
		Type:     module.Stacked,
		Priority: prioFileDescriptorsCount,
		Dims: module.Dims{
			{ID: "fd_total", Name: "available"},
			{ID: "fd_used", Name: "used"},
		},
	}
	chartSocketsCount = module.Chart{
		ID:       "sockets_used_count",
		Title:    "Used sockets",
		Units:    "sockets",
		Fam:      "node stats",
		Ctx:      "rabbitmq.sockets_count",
		Type:     module.Stacked,
		Priority: prioSocketsCount,
		Dims: module.Dims{
			{ID: "sockets_total", Name: "available"},
			{ID: "sockets_used", Name: "used"},
		},
	}
	chartErlangProcessesCount = module.Chart{
		ID:       "erlang_processes_count",
		Title:    "Erlang processes",
		Units:    "processes",
		Fam:      "node stats",
		Ctx:      "rabbitmq.erlang_processes_count",
		Type:     module.Stacked,
		Priority: prioErlangProcessesCount,
		Dims: module.Dims{
			{ID: "proc_available", Name: "available"},
			{ID: "proc_used", Name: "used"},
		},
	}
	chartErlangRunQueueProcessesCount = module.Chart{
		ID:       "erlang_run_queue_processes_count",
		Title:    "Erlang run queue",
		Units:    "processes",
		Fam:      "node stats",
		Ctx:      "rabbitmq.erlang_run_queue_processes_count",
		Priority: prioErlangRunQueueProcessesCount,
		Dims: module.Dims{
			{ID: "run_queue", Name: "length"},
		},
	}
	chartMemoryUsage = module.Chart{
		ID:       "memory_usage",
		Title:    "Memory",
		Units:    "bytes",
		Fam:      "node stats",
		Ctx:      "rabbitmq.memory_usage",
		Priority: prioMemoryUsage,
		Dims: module.Dims{
			{ID: "mem_used", Name: "used"},
		},
	}
	chartDiskSpaceFreeSize = module.Chart{
		ID:       "disk_space_free_size",
		Title:    "Free disk space",
		Units:    "bytes",
		Fam:      "node stats",
		Ctx:      "rabbitmq.disk_space_free_size",
		Type:     module.Area,
		Priority: prioDiskSpaceFreeSize,
		Dims: module.Dims{
			{ID: "disk_free", Name: "free"},
		},
	}
)

var (
	chartTmplVhostMessagesCount = module.Chart{
		ID:       "vhost_%s_message_count",
		Title:    "Vhost messages",
		Units:    "messages",
		Fam:      "vhost messages",
		Ctx:      "rabbitmq.vhost_messages_count",
		Type:     module.Stacked,
		Priority: prioVhostMessagesCount,
		Dims: module.Dims{
			{ID: "vhost_%s_messages_ready", Name: "ready"},
			{ID: "vhost_%s_messages_unacknowledged", Name: "unacknowledged"},
		},
	}
	chartTmplVhostMessagesRate = module.Chart{
		ID:       "vhost_%s_message_stats",
		Title:    "Vhost messages rate",
		Units:    "messages/s",
		Fam:      "vhost messages",
		Ctx:      "rabbitmq.vhost_messages_rate",
		Type:     module.Stacked,
		Priority: prioVhostMessagesRate,
		Dims: module.Dims{
			{ID: "vhost_%s_message_stats_ack", Name: "ack", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_confirm", Name: "confirm", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_deliver", Name: "deliver", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_get", Name: "get", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_get_no_ack", Name: "get_no_ack", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_publish", Name: "publish", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_redeliver", Name: "redeliver", Algo: module.Incremental},
			{ID: "vhost_%s_message_stats_return_unroutable", Name: "return_unroutable", Algo: module.Incremental},
		},
	}
)

var (
	chartTmplQueueMessagesCount = module.Chart{
		ID:       "queue_%s_vhost_%s_message_count",
		Title:    "Queue messages",
		Units:    "messages",
		Fam:      "queue messages",
		Ctx:      "rabbitmq.queue_messages_count",
		Type:     module.Stacked,
		Priority: prioQueueMessagesCount,
		Dims: module.Dims{
			{ID: "queue_%s_vhost_%s_messages_ready", Name: "ready"},
			{ID: "queue_%s_vhost_%s_messages_unacknowledged", Name: "unacknowledged"},
			{ID: "queue_%s_vhost_%s_messages_paged_out", Name: "paged_out"},
			{ID: "queue_%s_vhost_%s_messages_persistent", Name: "persistent"},
		},
	}
	chartTmplQueueMessagesRate = module.Chart{
		ID:       "queue_%s_vhost_%s_message_stats",
		Title:    "Queue messages rate",
		Units:    "messages/s",
		Fam:      "queue messages",
		Ctx:      "rabbitmq.queue_messages_rate",
		Type:     module.Stacked,
		Priority: prioQueueMessagesRate,
		Dims: module.Dims{
			{ID: "queue_%s_vhost_%s_message_stats_ack", Name: "ack", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_confirm", Name: "confirm", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_deliver", Name: "deliver", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_get", Name: "get", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_get_no_ack", Name: "get_no_ack", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_publish", Name: "publish", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_redeliver", Name: "redeliver", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_message_stats_return_unroutable", Name: "return_unroutable", Algo: module.Incremental},
		},
	}
)

func (r *RabbitMQ) addVhostCharts(name string) {
	charts := chartsTmplVhost.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, forbiddenCharsReplacer.Replace(name))
		chart.Labels = []module.Label{
			{Key: "vhost", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warning(err)
	}
}

func (r *RabbitMQ) removeVhostCharts(vhost string) {
	px := fmt.Sprintf("vhost_%s_", forbiddenCharsReplacer.Replace(vhost))
	for _, chart := range *r.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (r *RabbitMQ) addQueueCharts(queue, vhost string) {
	charts := chartsTmplQueue.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, forbiddenCharsReplacer.Replace(queue), forbiddenCharsReplacer.Replace(vhost))
		chart.Labels = []module.Label{
			{Key: "queue", Value: queue},
			{Key: "vhost", Value: vhost},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, queue, vhost)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warning(err)
	}
}

func (r *RabbitMQ) removeQueueCharts(queue, vhost string) {
	px := fmt.Sprintf("queue_%s_vhost_%s_", forbiddenCharsReplacer.Replace(queue), forbiddenCharsReplacer.Replace(vhost))
	for _, chart := range *r.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

var forbiddenCharsReplacer = strings.NewReplacer(" ", "_", ".", "_")
