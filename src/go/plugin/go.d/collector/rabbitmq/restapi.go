// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

const (
	urlPathAPIWhoami      = "/api/whoami"
	urlPathAPIDefinitions = "/api/definitions"
	urlPathAPIOverview    = "/api/overview"
	urlPathAPINodes       = "/api/nodes"
	urlPathAPIVhosts      = "/api/vhosts"
	urlPathAPIQueues      = "/api/queues"
)

type apiWhoamiResp struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type apiDefinitionsResp struct {
	RabbitmqVersion string `json:"rabbitmq_version"`
	GlobalParams    []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"global_parameters"`
}

// https://www.rabbitmq.com/monitoring.html#cluster-wide-metrics
type apiOverviewResp struct {
	ObjectTotals struct {
		Consumers   int64 `json:"consumers" stm:"consumers"`
		Queues      int64 `json:"queues" stm:"queues"`
		Exchanges   int64 `json:"exchanges" stm:"exchanges"`
		Connections int64 `json:"connections" stm:"connections"`
		Channels    int64 `json:"channels" stm:"channels"`
	} `json:"object_totals" stm:"object_totals"`
	ChurnRates struct {
		ChannelClosed     int64 `json:"channel_closed" stm:"channel_closed"`
		ChannelCreated    int64 `json:"channel_created" stm:"channel_created"`
		ConnectionClosed  int64 `json:"connection_closed" stm:"connection_closed"`
		ConnectionCreated int64 `json:"connection_created" stm:"connection_created"`
		QueueCreated      int64 `json:"queue_created" stm:"queue_created"`
		QueueDeclared     int64 `json:"queue_declared" stm:"queue_declared"`
		QueueDeleted      int64 `json:"queue_deleted" stm:"queue_deleted"`
	} `json:"churn_rates" stm:"churn_rates"`
	QueueTotals struct {
		Messages               int64 `json:"messages" stm:"messages"`
		MessagesReady          int64 `json:"messages_ready" stm:"messages_ready"`
		MessagesUnacknowledged int64 `json:"messages_unacknowledged" stm:"messages_unacknowledged"`
	} `json:"queue_totals" stm:"queue_totals"`
	MessageStats apiMessageStats `json:"message_stats" stm:"message_stats"`
}

// https://www.rabbitmq.com/monitoring.html#node-metrics
type (
	apiNodeResp struct {
		Name          string           `json:"name"`
		OsPid         string           `json:"os_pid"`
		Partitions    []string         `json:"partitions"` // network partitions https://www.rabbitmq.com/docs/partitions#detecting
		FDTotal       int64            `json:"fd_total"`
		FDUsed        int64            `json:"fd_used"`
		MemLimit      int64            `json:"mem_limit"`
		MemUsed       int64            `json:"mem_used"`
		SocketsTotal  int64            `json:"sockets_total"`
		SocketsUsed   int64            `json:"sockets_used"`
		ProcTotal     int64            `json:"proc_total"`
		ProcUsed      int64            `json:"proc_used"`
		DiskFree      int64            `json:"disk_free"`
		RunQueue      int64            `json:"run_queue"`
		Uptime        int64            `json:"uptime"`
		Running       bool             `json:"running"`
		MemAlarm      bool             `json:"mem_alarm"`
		DiskFreeAlarm bool             `json:"disk_free_alarm"`
		BeingDrained  bool             `json:"being_drained"`
		ClusterLinks  []apiClusterPeer `json:"cluster_links"`
	}
	apiClusterPeer struct {
		Name      string `json:"name"`
		RecvBytes int64  `json:"recv_bytes"`
		SendBytes int64  `json:"send_bytes"`
	}
)

type apiVhostResp struct {
	Name                   string            `json:"name"`
	ClusterState           map[string]string `json:"cluster_state"`
	Messages               int64             `json:"messages" stm:"messages"`
	MessagesReady          int64             `json:"messages_ready" stm:"messages_ready"`
	MessagesUnacknowledged int64             `json:"messages_unacknowledged" stm:"messages_unacknowledged"`
	MessageStats           apiMessageStats   `json:"message_stats" stm:"message_stats"`
}

// https://www.rabbitmq.com/monitoring.html#queue-metrics
type apiQueueResp struct {
	Name                   string          `json:"name"`
	Node                   string          `json:"node"`
	Vhost                  string          `json:"vhost"`
	Type                   string          `json:"type"`
	State                  string          `json:"state"`
	IdleSince              *any            `json:"idle_since"`
	Messages               int64           `json:"messages" stm:"messages"`
	MessagesReady          int64           `json:"messages_ready" stm:"messages_ready"`
	MessagesUnacknowledged int64           `json:"messages_unacknowledged" stm:"messages_unacknowledged"`
	MessagesPagedOut       int64           `json:"messages_paged_out" stm:"messages_paged_out"`
	MessagesPersistent     int64           `json:"messages_persistent" stm:"messages_persistent"`
	MessageStats           apiMessageStats `json:"message_stats" stm:"message_stats"`
}

// https://rawcdn.githack.com/rabbitmq/rabbitmq-server/v3.11.5/deps/rabbitmq_management/priv/www/api/index.html
type apiMessageStats struct {
	Ack              int64 `json:"ack" stm:"ack"`
	Publish          int64 `json:"publish" stm:"publish"`
	PublishIn        int64 `json:"publish_in" stm:"publish_in"`
	PublishOut       int64 `json:"publish_out" stm:"publish_out"`
	Confirm          int64 `json:"confirm" stm:"confirm"`
	Deliver          int64 `json:"deliver" stm:"deliver"`
	DeliverNoAck     int64 `json:"deliver_no_ack" stm:"deliver_no_ack"`
	Get              int64 `json:"get" stm:"get"`
	GetEmpty         int64 `json:"get_empty" stm:"get_empty"`
	GetNoAck         int64 `json:"get_no_ack" stm:"get_no_ack"`
	DeliverGet       int64 `json:"deliver_get" stm:"deliver_get"`
	Redeliver        int64 `json:"redeliver" stm:"redeliver"`
	ReturnUnroutable int64 `json:"return_unroutable" stm:"return_unroutable"`
}
