// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

// https://www.rabbitmq.com/monitoring.html#cluster-wide-metrics
type overviewStats struct {
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
	MessageStats messageStats `json:"message_stats" stm:"message_stats"`
	Node         string
}

// https://www.rabbitmq.com/monitoring.html#node-metrics
type nodeStats struct {
	FDTotal      int64 `json:"fd_total" stm:"fd_total"`
	FDUsed       int64 `json:"fd_used" stm:"fd_used"`
	MemLimit     int64 `json:"mem_limit" stm:"mem_limit"`
	MemUsed      int64 `json:"mem_used" stm:"mem_used"`
	SocketsTotal int64 `json:"sockets_total" stm:"sockets_total"`
	SocketsUsed  int64 `json:"sockets_used" stm:"sockets_used"`
	ProcTotal    int64 `json:"proc_total" stm:"proc_total"`
	ProcUsed     int64 `json:"proc_used" stm:"proc_used"`
	DiskFree     int64 `json:"disk_free" stm:"disk_free"`
	RunQueue     int64 `json:"run_queue" stm:"run_queue"`
}

type vhostStats struct {
	Name                   string       `json:"name"`
	Messages               int64        `json:"messages" stm:"messages"`
	MessagesReady          int64        `json:"messages_ready" stm:"messages_ready"`
	MessagesUnacknowledged int64        `json:"messages_unacknowledged" stm:"messages_unacknowledged"`
	MessageStats           messageStats `json:"message_stats" stm:"message_stats"`
}

// https://www.rabbitmq.com/monitoring.html#queue-metrics
type queueStats struct {
	Name                   string       `json:"name"`
	Vhost                  string       `json:"vhost"`
	State                  string       `json:"state"`
	Type                   string       `json:"type"`
	Messages               int64        `json:"messages" stm:"messages"`
	MessagesReady          int64        `json:"messages_ready" stm:"messages_ready"`
	MessagesUnacknowledged int64        `json:"messages_unacknowledged" stm:"messages_unacknowledged"`
	MessagesPagedOut       int64        `json:"messages_paged_out" stm:"messages_paged_out"`
	MessagesPersistent     int64        `json:"messages_persistent" stm:"messages_persistent"`
	MessageStats           messageStats `json:"message_stats" stm:"message_stats"`
}

// https://rawcdn.githack.com/rabbitmq/rabbitmq-server/v3.11.5/deps/rabbitmq_management/priv/www/api/index.html
type messageStats struct {
	Ack              int64 `json:"ack" stm:"ack"`
	Publish          int64 `json:"publish" stm:"publish"`
	PublishIn        int64 `json:"publish_in" stm:"publish_in"`
	PublishOut       int64 `json:"publish_out" stm:"publish_out"`
	Confirm          int64 `json:"confirm" stm:"confirm"`
	Deliver          int64 `json:"deliver" stm:"deliver"`
	DeliverNoAck     int64 `json:"deliver_no_ack" stm:"deliver_no_ack"`
	Get              int64 `json:"get" stm:"get"`
	GetNoAck         int64 `json:"get_no_ack" stm:"get_no_ack"`
	DeliverGet       int64 `json:"deliver_get" stm:"deliver_get"`
	Redeliver        int64 `json:"redeliver" stm:"redeliver"`
	ReturnUnroutable int64 `json:"return_unroutable" stm:"return_unroutable"`
}
