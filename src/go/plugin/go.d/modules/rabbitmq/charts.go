// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"
	"maps"
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

	prioNodeAvailStatus
	prioNodeMemAlarmStatus
	prioNodeDiskFreeAlarmStatus
	prioNodeFileDescriptorsUsage
	prioNodeSocketsUsage
	prioNodeErlangProcessesUsage
	prioNodeErlangRunQueueProcessesCount
	prioNodeMemoryUsage
	prioNodeDiskSpaceFreeSize
	prioNodeClusterLinkPeerTraffic
	prioNodeUptime

	prioVhostMessagesCount
	prioVhostMessagesRate
	prioVhostStatus

	prioQueueMessagesCount
	prioQueueMessagesRate
)

var overviewCharts = module.Charts{
	chartMessagesCount.Copy(),
	chartMessagesRate.Copy(),
	chartObjectsCount.Copy(),
	chartConnectionChurnRate.Copy(),
	chartChannelChurnRate.Copy(),
	chartQueueChurnRate.Copy(),
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
			{ID: "message_stats_get_empty", Name: "get_empty", Algo: module.Incremental},
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

var nodeChartsTmpl = module.Charts{
	nodeAvailStatusChartTmpl.Copy(),
	nodeMemAlarmStatusChartTmpl.Copy(),
	nodeDiskFreeAlarmStatusChartTmpl.Copy(),
	nodeFileDescriptorsUsageChartTmpl.Copy(),
	nodeSocketsUsageChartTmpl.Copy(),
	nodeErlangProcessesUsageChartTmpl.Copy(),
	nodeErlangRunQueueProcessesCountChartTmpl.Copy(),
	nodeMemoryUsageChartTmpl.Copy(),
	nodeDiskSpaceFreeSizeChartTmpl.Copy(),
	nodeUptimeChartTmpl.Copy(),
}

var (
	nodeAvailStatusChartTmpl = module.Chart{
		ID:       "node_%s_avail_status",
		Title:    "Node Availability Status",
		Units:    "status",
		Fam:      "node status",
		Ctx:      "rabbitmq.node_avail_status",
		Type:     module.Line,
		Priority: prioNodeAvailStatus,
		Dims: module.Dims{
			{ID: "node_%s_avail_status_running", Name: "running"},
			{ID: "node_%s_avail_status_down", Name: "down"},
		},
	}
	nodeMemAlarmStatusChartTmpl = module.Chart{
		ID:       "node_%s_mem_alarm_status",
		Title:    "Node Memory Alarm Status",
		Units:    "status",
		Fam:      "node status",
		Ctx:      "rabbitmq.node_mem_alarm_status",
		Type:     module.Line,
		Priority: prioNodeMemAlarmStatus,
		Dims: module.Dims{
			{ID: "node_%s_mem_alarm_status_clear", Name: "clear"},
			{ID: "node_%s_mem_alarm_status_triggered", Name: "triggered"},
		},
	}
	nodeDiskFreeAlarmStatusChartTmpl = module.Chart{
		ID:       "node_%s_disk_free_alarm_status",
		Title:    "Node Disk Free Alarm Status",
		Units:    "status",
		Fam:      "node status",
		Ctx:      "rabbitmq.node_disk_free_alarm_status",
		Type:     module.Line,
		Priority: prioNodeDiskFreeAlarmStatus,
		Dims: module.Dims{
			{ID: "node_%s_disk_free_alarm_status_clear", Name: "clear"},
			{ID: "node_%s_disk_free_alarm_status_triggered", Name: "triggered"},
		},
	}
	nodeFileDescriptorsUsageChartTmpl = module.Chart{
		ID:       "node_%s_file_descriptors_usage",
		Title:    "Node File Descriptors Usage",
		Units:    "fd",
		Fam:      "node fds",
		Ctx:      "rabbitmq.node_file_descriptors_usage",
		Type:     module.Stacked,
		Priority: prioNodeFileDescriptorsUsage,
		Dims: module.Dims{
			{ID: "node_%s_fds_used", Name: "used"},
		},
	}
	nodeSocketsUsageChartTmpl = module.Chart{
		ID:       "node_%s_sockets_used_usage",
		Title:    "Node Sockets Usage",
		Units:    "sockets",
		Fam:      "node sockets",
		Ctx:      "rabbitmq.node_sockets_usage",
		Type:     module.Stacked,
		Priority: prioNodeSocketsUsage,
		Dims: module.Dims{
			{ID: "node_%s_sockets_used", Name: "used"},
		},
	}
	nodeErlangProcessesUsageChartTmpl = module.Chart{
		ID:       "node_%s_erlang_processes_usage",
		Title:    "Node Erlang Processes Usage",
		Units:    "processes",
		Fam:      "node erlang",
		Ctx:      "rabbitmq.node_erlang_processes_usage",
		Type:     module.Stacked,
		Priority: prioNodeErlangProcessesUsage,
		Dims: module.Dims{
			{ID: "node_%s_procs_used", Name: "used"},
		},
	}
	nodeErlangRunQueueProcessesCountChartTmpl = module.Chart{
		ID:       "node_%s_erlang_run_queue_processes_count",
		Title:    "Node Erlang Run Queue",
		Units:    "processes",
		Fam:      "node erlang",
		Ctx:      "rabbitmq.node_erlang_run_queue_processes_count",
		Priority: prioNodeErlangRunQueueProcessesCount,
		Dims: module.Dims{
			{ID: "node_%s_run_queue", Name: "length"},
		},
	}
	nodeMemoryUsageChartTmpl = module.Chart{
		ID:       "node_%s_memory_usage",
		Title:    "Node Memory Usage",
		Units:    "bytes",
		Fam:      "node mem",
		Ctx:      "rabbitmq.node_memory_usage",
		Priority: prioNodeMemoryUsage,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "node_%s_mem_used", Name: "used"},
		},
	}
	nodeDiskSpaceFreeSizeChartTmpl = module.Chart{
		ID:       "node_%s_disk_space_free_size",
		Title:    "Node Disk Free Space",
		Units:    "bytes",
		Fam:      "node disk",
		Ctx:      "rabbitmq.node_disk_space_free_size",
		Type:     module.Area,
		Priority: prioNodeDiskSpaceFreeSize,
		Dims: module.Dims{
			{ID: "node_%s_disk_free_bytes", Name: "free"},
		},
	}
	nodeUptimeChartTmpl = module.Chart{
		ID:       "node_%s_uptime",
		Title:    "Node Uptime",
		Units:    "seconds",
		Fam:      "node uptime",
		Ctx:      "rabbitmq.node_uptime",
		Type:     module.Line,
		Priority: prioNodeUptime,
		Dims: module.Dims{
			{ID: "node_%s_uptime", Name: "uptime"},
		},
	}
)

var nodeClusterPeerChartsTmpl = module.Charts{
	nodeClusterLinkPeerTrafficChartTmpl.Copy(),
}

var (
	nodeClusterLinkPeerTrafficChartTmpl = module.Chart{
		ID:       "node_%s_peer_%s_cluster_link_traffic",
		Title:    "Node Cluster Link Peer Traffic",
		Units:    "bytes/s",
		Fam:      "node cluster link",
		Ctx:      "rabbitmq.node_peer_cluster_link_traffic",
		Type:     module.Area,
		Priority: prioNodeClusterLinkPeerTraffic,
		Dims: module.Dims{
			{ID: "node_%s_peer_%s_cluster_link_recv_bytes", Name: "received", Algo: module.Incremental},
			{ID: "node_%s_peer_%s_cluster_link_send_bytes", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}
)

var vhostChartsTmpl = module.Charts{
	vhostMessageCountChartTmpl.Copy(),
	vhostMessagesRateChartTmpl.Copy(),
	vhostStatusChartTmpl.Copy(),
}

var (
	vhostMessageCountChartTmpl = module.Chart{
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
	vhostMessagesRateChartTmpl = module.Chart{
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
	vhostStatusChartTmpl = module.Chart{
		ID:       "vhost_%s_status",
		Title:    "Vhost Status",
		Units:    "status",
		Fam:      "vhost status",
		Ctx:      "rabbitmq.vhost_status",
		Type:     module.Line,
		Priority: prioVhostStatus,
		Dims: module.Dims{
			{ID: "vhost_%s_status_running", Name: "running"},
			{ID: "vhost_%s_status_stopped", Name: "stopped"},
			{ID: "vhost_%s_status_partial", Name: "partial"},
		},
	}
)

var queueChartsTmpl = module.Charts{
	queueMessagesCountChartTmpl.Copy(),
	queueMessagesRateChartTmpl.Copy(),
}

var (
	queueMessagesCountChartTmpl = module.Chart{
		ID:       "queue_%s_vhost_%s_node_%s_message_count",
		Title:    "Queue messages",
		Units:    "messages",
		Fam:      "queue messages",
		Ctx:      "rabbitmq.queue_messages_count",
		Type:     module.Stacked,
		Priority: prioQueueMessagesCount,
		Dims: module.Dims{
			{ID: "queue_%s_vhost_%s_node_%s_messages_ready", Name: "ready"},
			{ID: "queue_%s_vhost_%s_node_%s_messages_unacknowledged", Name: "unacknowledged"},
			{ID: "queue_%s_vhost_%s_node_%s_messages_paged_out", Name: "paged_out"},
			{ID: "queue_%s_vhost_%s_node_%s_messages_persistent", Name: "persistent"},
		},
	}
	queueMessagesRateChartTmpl = module.Chart{
		ID:       "queue_%s_vhost_%s_node_%s_message_stats",
		Title:    "Queue messages rate",
		Units:    "messages/s",
		Fam:      "queue messages",
		Ctx:      "rabbitmq.queue_messages_rate",
		Type:     module.Stacked,
		Priority: prioQueueMessagesRate,
		Dims: module.Dims{
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_ack", Name: "ack", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_confirm", Name: "confirm", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_deliver", Name: "deliver", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_get", Name: "get", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_get_no_ack", Name: "get_no_ack", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_publish", Name: "publish", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_redeliver", Name: "redeliver", Algo: module.Incremental},
			{ID: "queue_%s_vhost_%s_node_%s_message_stats_return_unroutable", Name: "return_unroutable", Algo: module.Incremental},
		},
	}
)

func (r *RabbitMQ) updateCharts() {
	if !r.cache.overview.hasCharts {
		r.cache.overview.hasCharts = true
		r.addOverviewCharts()
	}

	maps.DeleteFunc(r.cache.nodes, func(_ string, node *nodeCacheItem) bool {
		if !node.seen {
			r.removeNodeCharts(node)
			return true
		}
		if !node.hasCharts {
			node.hasCharts = true
			r.addNodeCharts(node)
		}
		maps.DeleteFunc(node.peers, func(_ string, peer *peerCacheItem) bool {
			if !peer.seen {
				r.removeNodeClusterPeerCharts(peer)
				return true
			}
			if !peer.hasCharts {
				peer.hasCharts = true
				r.addNodeClusterPeerCharts(peer)
			}
			return false
		})
		return false
	})

	maps.DeleteFunc(r.cache.vhosts, func(_ string, vhost *vhostCacheItem) bool {
		if !vhost.seen {
			r.removeVhostCharts(vhost)
			return true
		}
		if !vhost.hasCharts {
			vhost.hasCharts = true
			r.addVhostCharts(vhost)
		}
		return false
	})

	maps.DeleteFunc(r.cache.queues, func(_ string, queue *queueCacheItem) bool {
		if !queue.seen {
			r.removeQueueCharts(queue)
			return true
		}
		if !queue.hasCharts {
			queue.hasCharts = true
			r.addQueueCharts(queue)
		}
		return false
	})
}

func (r *RabbitMQ) addOverviewCharts() {
	charts := overviewCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "cluster_id", Value: r.clusterId},
			{Key: "cluster_name", Value: r.clusterName},
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warningf("failed to add overview charts: %v", err)
	}
}

func (r *RabbitMQ) addNodeCharts(node *nodeCacheItem) {
	charts := nodeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, node.name))
		chart.Labels = []module.Label{
			{Key: "cluster_id", Value: r.clusterId},
			{Key: "cluster_name", Value: r.clusterName},
			{Key: "node", Value: node.name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, node.name)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warningf("failed to add node charts: %v", err)
	}
}

func (r *RabbitMQ) removeNodeCharts(node *nodeCacheItem) {
	px := fmt.Sprintf("node_%s_", node.name)
	r.removeCharts(px)
}

func (r *RabbitMQ) addNodeClusterPeerCharts(peer *peerCacheItem) {
	charts := nodeClusterPeerChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, peer.node, peer.name))
		chart.Labels = []module.Label{
			{Key: "cluster_id", Value: r.clusterId},
			{Key: "cluster_name", Value: r.clusterName},
			{Key: "node", Value: peer.node},
			{Key: "peer", Value: peer.name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, peer.node, peer.name)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warningf("failed to add node cluster peer charts: %v", err)
	}

}

func (r *RabbitMQ) removeNodeClusterPeerCharts(peer *peerCacheItem) {
	px := fmt.Sprintf("node_%s_peer_%s_", peer.node, peer.name)
	r.removeCharts(px)
}

func (r *RabbitMQ) addVhostCharts(vhost *vhostCacheItem) {
	charts := vhostChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, vhost.name))
		chart.Labels = []module.Label{
			{Key: "cluster_id", Value: r.clusterId},
			{Key: "cluster_name", Value: r.clusterName},
			{Key: "vhost", Value: vhost.name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, vhost.name)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warningf("failed to add vhost charts: %v", err)
	}
}

func (r *RabbitMQ) removeVhostCharts(vhost *vhostCacheItem) {
	px := fmt.Sprintf("vhost_%s_", vhost.name)
	r.removeCharts(px)
}

func (r *RabbitMQ) addQueueCharts(q *queueCacheItem) {
	charts := queueChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, q.name, q.vhost, q.node)
		chart.ID = cleanChartId(chart.ID)
		chart.Labels = []module.Label{
			{Key: "cluster_id", Value: r.clusterId},
			{Key: "cluster_name", Value: r.clusterName},
			{Key: "node", Value: q.node},
			{Key: "queue", Value: q.name},
			{Key: "vhost", Value: q.vhost},
			{Key: "type", Value: q.typ},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, q.name, q.vhost, q.node)
		}
	}

	if err := r.Charts().Add(*charts...); err != nil {
		r.Warning(err)
	}
}

func (r *RabbitMQ) removeQueueCharts(q *queueCacheItem) {
	px := fmt.Sprintf("queue_%s_vhost_%s_node_%s_", q.name, q.vhost, q.node)
	r.removeCharts(px)
}

func (r *RabbitMQ) removeCharts(prefix string) {
	prefix = cleanChartId(prefix)
	for _, chart := range *r.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanChartId(id string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return r.Replace(id)
}
