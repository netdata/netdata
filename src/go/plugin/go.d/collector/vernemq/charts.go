// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioNodeSockets = module.Priority + iota
	prioNodeSocketEvents
	prioNodeClientKeepaliveExpired
	prioNodeSocketCloseTimeout
	prioNodeSocketErrors

	prioNodeQueueProcesses
	prioNodeQueueProcessesEvents
	prioNodeQueueProcessesOfflineStorage
	prioNodeQueueMessages
	prioNodeQueuedMessages
	prioNodeQueueUndeliveredMessages

	prioNodeRouterSubscriptions
	prioNodeRouterMatchedSubscriptions
	prioNodeRouterMemory

	prioNodeAverageSchedulerUtilization
	prioNodeSystemProcesses
	prioNodeSystemReductions
	prioNodeSystemContext
	prioNodeSystemIO
	prioNodeSystemRunQueue
	prioNodeSystemGCCount
	prioNodeSystemGCWordsReclaimed
	prioNodeSystemMemoryAllocated

	prioNodeTraffic

	prioNodeRetainMessages
	prioNodeRetainMemoryUsage

	prioNodeClusterCommunicationTraffic
	prioNodeClusterCommunicationDropped
	prioNodeNetSplitUnresolved
	prioNodeNetSplits

	prioMqttPublishPackets
	prioMqttPublishErrors
	prioMqttPublishAuthPackets

	prioMqttPubAckPackets
	prioMqttPubAckReceivedReason
	prioMqttPubAckSentReason
	prioMqttPubAckUnexpectedMessages

	prioMqttPubRecPackets
	prioMqttPubRecReceivedReason
	prioMqttPubRecSentReason
	prioMqttPubRecUnexpectedMessages

	prioMqttPubRelPackets
	prioMqttPubRelReceivedReason
	prioMqttPubRelSentReason

	prioMqttPubCompPackets
	prioMqttPubCompReceivedReason
	prioMqttPubCompSentReason
	prioMqttPubCompUnexpectedMessages

	prioMqttConnectPackets
	prioMqttConnectSentReason

	prioMqttDisconnectPackets
	prioMqttDisconnectReceivedReason
	prioMqttDisconnectSentReason

	prioMqttSubscribePackets
	prioMqttSubscribeErrors
	prioMqttSubscribeAuthPackets

	prioMqttUnsubscribePackets
	prioMqttUnsubscribeErrors

	prioMqttAuthPackets
	prioMqttAuthReceivedReason
	prioMqttAuthSentReason

	prioMqttPingPackets

	prioNodeUptime
)

var nodeChartsTmpl = module.Charts{
	nodeOpenSocketsChartTmpl.Copy(),
	nodeSocketEventsChartTmpl.Copy(),
	nodeSocketCloseTimeoutChartTmpl.Copy(),
	nodeSocketErrorsChartTmpl.Copy(),

	nodeQueueProcessesChartTmpl.Copy(),
	nodeQueueProcessesEventsChartTmpl.Copy(),
	nodeQueueProcessesOfflineStorageChartTmpl.Copy(),
	nodeQueueMessagesChartTmpl.Copy(),
	nodeQueuedMessagesChartTmpl.Copy(),
	nodeQueueUndeliveredMessagesChartTmpl.Copy(),

	nodeRouterSubscriptionsChartTmpl.Copy(),
	nodeRouterMatchedSubscriptionsChartTmpl.Copy(),
	nodeRouterMemoryChartTmpl.Copy(),

	nodeAverageSchedulerUtilizationChartTmpl.Copy(),
	nodeSystemProcessesChartTmpl.Copy(),
	nodeSystemReductionsChartTmpl.Copy(),
	nodeSystemContextSwitches.Copy(),
	nodeSystemIOChartTmpl.Copy(),
	nodeSystemRunQueueChartTmpl.Copy(),
	nodeSystemGCCountChartTmpl.Copy(),
	nodeSystemGCWordsReclaimedChartTmpl.Copy(),
	nodeSystemMemoryAllocatedChartTmpl.Copy(),

	nodeTrafficChartTmpl.Copy(),

	nodeRetainMessagesChartsTmpl.Copy(),
	nodeRetainMemoryUsageChartTmpl.Copy(),

	nodeClusterCommunicationTrafficChartTmpl.Copy(),
	nodeClusterCommunicationDroppedChartTmpl.Copy(),
	nodeNetSplitUnresolvedChartTmpl.Copy(),
	nodeNetSplitsChartTmpl.Copy(),

	nodeUptimeChartTmpl.Copy(),
}

var nodeMqtt5ChartsTmpl = module.Charts{
	nodeClientKeepaliveExpiredChartTmpl.Copy(),

	nodeMqttPUBLISHPacketsChartTmpl.Copy(),
	nodeMqttPUBLISHErrorsChartTmpl.Copy(),
	nodeMqttPUBLISHAuthErrorsChartTmpl.Copy(),

	nodeMqttPUBACKPacketsChartTmpl.Copy(),
	nodeMqttPUBACKReceivedByReasonChartTmpl.Copy(),
	nodeMqttPUBACKSentByReasonChartTmpl.Copy(),
	nodeMqttPUBACKUnexpectedMessagesChartTmpl.Copy(),

	nodeMqttPUBRECPacketsChartTmpl.Copy(),
	nodeMqttPUBRECReceivedByReasonChartTmpl.Copy(),
	nodeMqttPUBRECSentByReasonChartTmpl.Copy(),

	nodeMqttPUBRELPacketsChartTmpl.Copy(),
	nodeMqttPUBRELReceivedByReasonChartTmpl.Copy(),
	nodeMqttPUBRELSentByReasonChartTmpl.Copy(),

	nodeMqttPUBCOMPPacketsChartTmpl.Copy(),
	nodeMqttPUBCOMPReceivedByReasonChartTmpl.Copy(),
	nodeMqttPUBCOMPSentByReasonChartTmpl.Copy(),
	nodeMqttPUBCOMPUnexpectedMessagesChartTmpl.Copy(),

	nodeMqttCONNECTPacketsChartTmpl.Copy(),
	nodeMqttCONNACKSentByReasonCodeChartTmpl.Copy(),

	nodeMqtt5DISCONNECTPacketsChartTmpl.Copy(),
	nodeMqttDISCONNECTReceivedByReasonChartTmpl.Copy(),
	nodeMqttDISCONNECTSentByReasonChartTmpl.Copy(),

	nodeMqttSUBSCRIBEPacketsChartTmpl.Copy(),
	modeMqttSUBSCRIBEErrorsChartTmpl.Copy(),
	nodeMqttSUBSCRIBEAuthErrorsChartTmpl.Copy(),

	nodeMqttUNSUBSCRIBEPacketsChartTmpl.Copy(),
	nodeMqttUNSUBSCRIBEErrorsChartTmpl.Copy(),

	nodeMqttAUTHPacketsChartTmpl.Copy(),
	nodeMqttAUTHReceivedByReasonChartTmpl.Copy(),
	nodeMqttAUTHSentByReasonChartTmpl.Copy(),

	nodeMqttPINGPacketsChartTmpl.Copy(),
}

var nodeMqtt4ChartsTmpl = module.Charts{
	nodeClientKeepaliveExpiredChartTmpl.Copy(),

	nodeMqttPUBLISHPacketsChartTmpl.Copy(),
	nodeMqttPUBLISHErrorsChartTmpl.Copy(),
	nodeMqttPUBLISHAuthErrorsChartTmpl.Copy(),

	nodeMqttPUBACKPacketsChartTmpl.Copy(),
	nodeMqttPUBACKUnexpectedMessagesChartTmpl.Copy(),

	nodeMqttPUBRECPacketsChartTmpl.Copy(),
	nodeMqttPUBRECUnexpectedMessagesChartTmpl.Copy(),

	nodeMqttPUBRELPacketsChartTmpl.Copy(),

	nodeMqttPUBCOMPPacketsChartTmpl.Copy(),
	nodeMqttPUBCOMPUnexpectedMessagesChartTmpl.Copy(),

	nodeMqttCONNECTPacketsChartTmpl.Copy(),
	nodeMqttCONNACKSentByReturnCodeChartTmpl.Copy(),

	nodeMqtt4DISCONNECTPacketsChartTmpl.Copy(),

	nodeMqttSUBSCRIBEPacketsChartTmpl.Copy(),
	modeMqttSUBSCRIBEErrorsChartTmpl.Copy(),

	nodeMqttSUBSCRIBEAuthErrorsChartTmpl.Copy(),
	nodeMqttUNSUBSCRIBEPacketsChartTmpl.Copy(),
	nodeMqttUNSUBSCRIBEErrorsChartTmpl.Copy(),

	nodeMqttPINGPacketsChartTmpl.Copy(),
}

// Sockets
var (
	nodeOpenSocketsChartTmpl = module.Chart{
		ID:       "node_%s_sockets",
		Title:    "Open Sockets",
		Units:    "sockets",
		Fam:      "sockets",
		Ctx:      "vernemq.node_sockets",
		Priority: prioNodeSockets,
		Dims: module.Dims{
			{ID: dimNode("open_sockets"), Name: "open"},
		},
	}
	nodeSocketEventsChartTmpl = module.Chart{
		ID:       "node_%s_socket_events",
		Title:    "Open and Close Socket Events",
		Units:    "events/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_operations",
		Priority: prioNodeSocketEvents,
		Dims: module.Dims{
			{ID: dimNode(metricSocketOpen), Name: "open", Algo: module.Incremental},
			{ID: dimNode(metricSocketClose), Name: "close", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeClientKeepaliveExpiredChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_client_keepalive_expired",
		Title:    "Closed Sockets due to Keepalive Time Expired",
		Units:    "sockets/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_client_keepalive_expired",
		Priority: prioNodeClientKeepaliveExpired,
		Dims: module.Dims{
			{ID: dimMqttVer(metricClientKeepaliveExpired), Name: "closed", Algo: module.Incremental},
		},
	}
	nodeSocketCloseTimeoutChartTmpl = module.Chart{
		ID:       "node_%s_socket_close_timeout",
		Title:    "Closed Sockets due to no CONNECT Frame On Time",
		Units:    "sockets/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_close_timeout",
		Priority: prioNodeSocketCloseTimeout,
		Dims: module.Dims{
			{ID: dimNode(metricSocketCloseTimeout), Name: "closed", Algo: module.Incremental},
		},
	}
	nodeSocketErrorsChartTmpl = module.Chart{
		ID:       "node_%s_socket_errors",
		Title:    "Socket Errors",
		Units:    "errors/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_errors",
		Priority: prioNodeSocketErrors,
		Dims: module.Dims{
			{ID: dimNode(metricSocketError), Name: "errors", Algo: module.Incremental},
		},
	}
)

// Queues
var (
	nodeQueueProcessesChartTmpl = module.Chart{
		ID:       "node_%s_queue_processes",
		Title:    "Living Queues in an Online or an Offline State",
		Units:    "queue processes",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_processes",
		Priority: prioNodeQueueProcesses,
		Dims: module.Dims{
			{ID: dimNode(metricQueueProcesses), Name: "queue_processes"},
		},
	}
	nodeQueueProcessesEventsChartTmpl = module.Chart{
		ID:       "node_%s_queue_processes_events",
		Title:    "Queue Processes Setup and Teardown Events",
		Units:    "events/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_processes_operations",
		Priority: prioNodeQueueProcessesEvents,
		Dims: module.Dims{
			{ID: dimNode(metricQueueSetup), Name: "setup", Algo: module.Incremental},
			{ID: dimNode(metricQueueTeardown), Name: "teardown", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeQueueProcessesOfflineStorageChartTmpl = module.Chart{
		ID:       "node_%s_queue_process_init_from_storage",
		Title:    "Queue Processes Initialized from Offline Storage",
		Units:    "queue processes/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_process_init_from_storage",
		Priority: prioNodeQueueProcessesOfflineStorage,
		Dims: module.Dims{
			{ID: dimNode(metricQueueInitializedFromStorage), Name: "queue processes", Algo: module.Incremental},
		},
	}
	nodeQueueMessagesChartTmpl = module.Chart{
		ID:       "node_%s_queue_messages",
		Title:    "Received and Sent PUBLISH Messages",
		Units:    "messages/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_messages",
		Type:     module.Area,
		Priority: prioNodeQueueMessages,
		Dims: module.Dims{
			{ID: dimNode(metricQueueMessageIn), Name: "received", Algo: module.Incremental},
			{ID: dimNode(metricQueueMessageOut), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeQueuedMessagesChartTmpl = module.Chart{
		ID:       "node_%s_queued_messages",
		Title:    "Queued PUBLISH Messages",
		Units:    "messages",
		Fam:      "queues",
		Ctx:      "vernemq.node_queued_messages",
		Type:     module.Line,
		Priority: prioNodeQueuedMessages,
		Dims: module.Dims{
			{ID: dimNode("queued_messages"), Name: "queued"},
		},
	}
	nodeQueueUndeliveredMessagesChartTmpl = module.Chart{
		ID:       "node_%s_queue_undelivered_messages",
		Title:    "Undelivered PUBLISH Messages",
		Units:    "messages/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_undelivered_messages",
		Type:     module.Stacked,
		Priority: prioNodeQueueUndeliveredMessages,
		Dims: module.Dims{
			{ID: dimNode(metricQueueMessageDrop), Name: "dropped", Algo: module.Incremental},
			{ID: dimNode(metricQueueMessageExpired), Name: "expired", Algo: module.Incremental},
			{ID: dimNode(metricQueueMessageUnhandled), Name: "unhandled", Algo: module.Incremental},
		},
	}
)

// Subscriptions
var (
	nodeRouterSubscriptionsChartTmpl = module.Chart{
		ID:       "node_%s_router_subscriptions",
		Title:    "Subscriptions in the Routing Table",
		Units:    "subscriptions",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_subscriptions",
		Priority: prioNodeRouterSubscriptions,
		Dims: module.Dims{
			{ID: dimNode(metricRouterSubscriptions), Name: "subscriptions"},
		},
	}
	nodeRouterMatchedSubscriptionsChartTmpl = module.Chart{
		ID:       "node_%s_router_matched_subscriptions",
		Title:    "Matched Subscriptions",
		Units:    "subscriptions/s",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_matched_subscriptions",
		Priority: prioNodeRouterMatchedSubscriptions,
		Dims: module.Dims{
			{ID: dimNode(metricRouterMatchesLocal), Name: "local", Algo: module.Incremental},
			{ID: dimNode(metricRouterMatchesRemote), Name: "remote", Algo: module.Incremental},
		},
	}
	nodeRouterMemoryChartTmpl = module.Chart{
		ID:       "node_%s_router_memory",
		Title:    "Routing Table Memory Usage",
		Units:    "bytes",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_memory",
		Type:     module.Area,
		Priority: prioNodeRouterMemory,
		Dims: module.Dims{
			{ID: dimNode(metricRouterMemory), Name: "used"},
		},
	}
)

// Erlang VM
var (
	nodeAverageSchedulerUtilizationChartTmpl = module.Chart{
		ID:       "node_%s_average_scheduler_utilization",
		Title:    "Average Scheduler Utilization",
		Units:    "percentage",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_average_scheduler_utilization",
		Type:     module.Area,
		Priority: prioNodeAverageSchedulerUtilization,
		Dims: module.Dims{
			{ID: dimNode(metricSystemUtilization), Name: "utilization"},
		},
	}
	nodeSystemProcessesChartTmpl = module.Chart{
		ID:       "node_%s_system_processes",
		Title:    "Erlang Processes",
		Units:    "processes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_processes",
		Priority: prioNodeSystemProcesses,
		Dims: module.Dims{
			{ID: dimNode(metricSystemProcessCount), Name: "processes"},
		},
	}
	nodeSystemReductionsChartTmpl = module.Chart{
		ID:       "node_%s_system_reductions",
		Title:    "Reductions",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_reductions",
		Priority: prioNodeSystemReductions,
		Dims: module.Dims{
			{ID: dimNode(metricSystemReductions), Name: "reductions", Algo: module.Incremental},
		},
	}
	nodeSystemContextSwitches = module.Chart{
		ID:       "node_%s_system_context_switches",
		Title:    "Context Switches",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_context_switches",
		Priority: prioNodeSystemContext,
		Dims: module.Dims{
			{ID: dimNode(metricSystemContextSwitches), Name: "context switches", Algo: module.Incremental},
		},
	}
	nodeSystemIOChartTmpl = module.Chart{
		ID:       "node_%s_system_io",
		Title:    "Received and Sent Traffic through Ports",
		Units:    "bytes/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_io",
		Type:     module.Area,
		Priority: prioNodeSystemIO,
		Dims: module.Dims{
			{ID: dimNode(metricSystemIOIn), Name: "received", Algo: module.Incremental},
			{ID: dimNode(metricSystemIOOut), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeSystemRunQueueChartTmpl = module.Chart{
		ID:       "node_%s_system_run_queue",
		Title:    "Processes that are Ready to Run on All Run-Queues",
		Units:    "processes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_run_queue",
		Priority: prioNodeSystemRunQueue,
		Dims: module.Dims{
			{ID: dimNode(metricSystemRunQueue), Name: "ready"},
		},
	}
	nodeSystemGCCountChartTmpl = module.Chart{
		ID:       "node_%s_system_gc_count",
		Title:    "GC Count",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_gc_count",
		Priority: prioNodeSystemGCCount,
		Dims: module.Dims{
			{ID: dimNode(metricSystemGCCount), Name: "gc", Algo: module.Incremental},
		},
	}
	nodeSystemGCWordsReclaimedChartTmpl = module.Chart{
		ID:       "node_%s_system_gc_words_reclaimed",
		Title:    "GC Words Reclaimed",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_gc_words_reclaimed",
		Priority: prioNodeSystemGCWordsReclaimed,
		Dims: module.Dims{
			{ID: dimNode(metricSystemWordsReclaimedByGC), Name: "words reclaimed", Algo: module.Incremental},
		},
	}
	nodeSystemMemoryAllocatedChartTmpl = module.Chart{
		ID:       "node_%s_system_allocated_memory",
		Title:    "Memory Allocated by the Erlang Processes and by the Emulator",
		Units:    "bytes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_allocated_memory",
		Type:     module.Stacked,
		Priority: prioNodeSystemMemoryAllocated,
		Dims: module.Dims{
			{ID: dimNode(metricVMMemoryProcesses), Name: "processes"},
			{ID: dimNode(metricVMMemorySystem), Name: "system"},
		},
	}
)

// Traffic
var (
	nodeTrafficChartTmpl = module.Chart{
		ID:       "node_%s_traffic",
		Title:    "Node Traffic",
		Units:    "bytes/s",
		Fam:      "traffic",
		Ctx:      "vernemq.node_traffic",
		Type:     module.Area,
		Priority: prioNodeTraffic,
		Dims: module.Dims{
			{ID: dimNode(metricBytesReceived), Name: "received", Algo: module.Incremental},
			{ID: dimNode(metricBytesSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
)

// Retain
var (
	nodeRetainMessagesChartsTmpl = module.Chart{
		ID:       "node_%s_retain_messages",
		Title:    "Stored Retained Messages",
		Units:    "messages",
		Fam:      "retain",
		Ctx:      "vernemq.node_retain_messages",
		Priority: prioNodeRetainMessages,
		Dims: module.Dims{
			{ID: dimNode(metricRetainMessages), Name: "messages"},
		},
	}
	nodeRetainMemoryUsageChartTmpl = module.Chart{
		ID:       "node_%s_retain_memory",
		Title:    "Stored Retained Messages Memory Usage",
		Units:    "bytes",
		Fam:      "retain",
		Ctx:      "vernemq.node_retain_memory",
		Type:     module.Area,
		Priority: prioNodeRetainMemoryUsage,
		Dims: module.Dims{
			{ID: dimNode(metricRetainMemory), Name: "used"},
		},
	}
)

// Cluster
var (
	nodeClusterCommunicationTrafficChartTmpl = module.Chart{
		ID:       "node_%s_cluster_traffic",
		Title:    "Communication with Other Cluster Nodes",
		Units:    "bytes/s",
		Fam:      "cluster",
		Ctx:      "vernemq.node_cluster_traffic",
		Type:     module.Area,
		Priority: prioNodeClusterCommunicationTraffic,
		Dims: module.Dims{
			{ID: dimNode(metricClusterBytesReceived), Name: "received", Algo: module.Incremental},
			{ID: dimNode(metricClusterBytesSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeClusterCommunicationDroppedChartTmpl = module.Chart{
		ID:       "node_%s_cluster_dropped",
		Title:    "Traffic Dropped During Communication with Other Cluster Nodes",
		Units:    "bytes/s",
		Fam:      "cluster",
		Type:     module.Area,
		Ctx:      "vernemq.node_cluster_dropped",
		Priority: prioNodeClusterCommunicationDropped,
		Dims: module.Dims{
			{ID: dimNode(metricClusterBytesDropped), Name: "dropped", Algo: module.Incremental},
		},
	}
	nodeNetSplitUnresolvedChartTmpl = module.Chart{
		ID:       "node_%s_netsplit_unresolved",
		Title:    "Unresolved Netsplits",
		Units:    "netsplits",
		Fam:      "cluster",
		Ctx:      "vernemq.node_netsplit_unresolved",
		Priority: prioNodeNetSplitUnresolved,
		Dims: module.Dims{
			{ID: dimNode("netsplit_unresolved"), Name: "unresolved"},
		},
	}
	nodeNetSplitsChartTmpl = module.Chart{
		ID:       "node_%s_netsplit",
		Title:    "Netsplits",
		Units:    "netsplits/s",
		Fam:      "cluster",
		Ctx:      "vernemq.node_netsplits",
		Type:     module.Stacked,
		Priority: prioNodeNetSplits,
		Dims: module.Dims{
			{ID: dimNode(metricNetSplitResolved), Name: "resolved", Algo: module.Incremental},
			{ID: dimNode(metricNetSplitDetected), Name: "detected", Algo: module.Incremental},
		},
	}
)

var (
	nodeUptimeChartTmpl = module.Chart{
		ID:       "node_%s_uptime",
		Title:    "Node Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "vernemq.node_uptime",
		Priority: prioNodeUptime,
		Dims: module.Dims{
			{ID: dimNode(metricSystemWallClock), Name: "time", Div: 1000},
		},
	}
)

// PUBLISH
var (
	nodeMqttPUBLISHPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_publish",
		Title:    "MQTT QoS 0,1,2 PUBLISH",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish",
		Priority: prioMqttPublishPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBSLISHReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricPUBSLIHSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBLISHErrorsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_publish_errors",
		Title:    "MQTT Failed PUBLISH Operations due to a Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish_errors",
		Priority: prioMqttPublishErrors,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBLISHError), Name: "publish", Algo: module.Incremental},
		},
	}
	nodeMqttPUBLISHAuthErrorsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_publish_auth_errors",
		Title:    "MQTT Unauthorized PUBLISH Attempts",
		Units:    "errors/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish_auth_errors",
		Type:     module.Area,
		Priority: prioMqttPublishAuthPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBLISHAuthError), Name: "publish_auth", Algo: module.Incremental},
		},
	}
	nodeMqttPUBACKPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_puback",
		Title:    "MQTT QoS 1 PUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_puback",
		Priority: prioMqttPubAckPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBACKReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricPUBACKSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBACKReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_puback_received_by_reason_code",
			Title:    "MQTT PUBACK QoS 1 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_puback_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubAckReceivedReason,
		}
		for _, v := range mqtt5PUBACKReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBACKReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBACKSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_puback_sent_by_reason_code",
			Title:    "MQTT PUBACK QoS 1 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_puback_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubAckSentReason,
		}
		for _, v := range mqtt5PUBACKSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBACKSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBACKUnexpectedMessagesChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_puback_unexpected",
		Title:    "MQTT PUBACK QoS 1 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_puback_invalid_error",
		Priority: prioMqttPubAckUnexpectedMessages,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBACKInvalid), Name: "unexpected", Algo: module.Incremental},
		},
	}
	nodeMqttPUBRECPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_pubrec",
		Title:    "MQTT PUBREC QoS 2 Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrec",
		Priority: prioMqttPubRecPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBRECReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricPUBRECSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBRECReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubrec_received_by_reason_code",
			Title:    "MQTT PUBREC QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrec_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubRecReceivedReason,
		}
		for _, v := range mqtt5PUBRECReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBRECReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRECSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubrec_sent_by_reason_code",
			Title:    "MQTT PUBREC QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrec_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubRecSentReason,
		}
		for _, v := range mqtt5PUBRECSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBRECSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRECUnexpectedMessagesChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_pubrec_unexpected",
		Title:    "MQTT PUBREC QoS 2 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrec_invalid_error",
		Priority: prioMqttPubRecUnexpectedMessages,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBRECInvalid), Name: "unexpected", Algo: module.Incremental},
		},
	}
	nodeMqttPUBRELPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_pubrel",
		Title:    "MQTT PUBREL QoS 2 Packets¬",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrel",
		Priority: prioMqttPubRelPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBRELReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricPUBRELSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBRELReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubrel_received_by_reason_code",
			Title:    "MQTT PUBREL QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrel_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubRelReceivedReason,
		}
		for _, v := range mqtt5PUBRELReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBRELReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRELSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubrel_sent_by_reason_code",
			Title:    "MQTT PUBREL QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrel_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubRelSentReason,
		}
		for _, v := range mqtt5PUBRELSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBRELSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_pubcomp",
		Title:    "MQTT PUBCOMP QoS 2 Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubcomp",
		Priority: prioMqttPubCompPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUBCOMPReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricPUBCOMPSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBCOMPReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubcomp_received_by_reason_code",
			Title:    "MQTT PUBCOMP QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubcomp_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubCompReceivedReason,
		}
		for _, v := range mqtt5PUBCOMPReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBCOMPReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_pubcomp_sent_by_reason_code",
			Title:    "MQTT PUBCOMP QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubcomp_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttPubCompSentReason,
		}
		for _, v := range mqtt5PUBCOMPSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricPUBCOMPSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPUnexpectedMessagesChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_pubcomp_unexpected",
		Title:    "MQTT PUBCOMP QoS 2 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubcomp_invalid_error",
		Priority: prioMqttPubCompUnexpectedMessages,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPUNCOMPInvalid), Name: "unexpected", Algo: module.Incremental},
		},
	}
)

// CONNECT
var (
	nodeMqttCONNECTPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_connect",
		Title:    "MQTT CONNECT and CONNACK",
		Units:    "packets/s",
		Fam:      "mqtt connect",
		Ctx:      "vernemq.node_mqtt_connect",
		Priority: prioMqttConnectPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricCONNECTReceived), Name: "connect", Algo: module.Incremental},
			{ID: dimMqttVer(metricCONNACKSent), Name: "connack", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttCONNACKSentByReturnCodeChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_connack_sent_by_return_code",
			Title:    "MQTT CONNACK Sent by Return Code",
			Units:    "packets/s",
			Fam:      "mqtt connect",
			Ctx:      "vernemq.node_mqtt_connack_sent_by_return_code",
			Type:     module.Stacked,
			Priority: prioMqttConnectSentReason,
		}
		for _, v := range mqtt4CONNACKSentReturnCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttRCode(metricCONNACKSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttCONNACKSentByReasonCodeChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_connack_sent_by_reason_code",
			Title:    "MQTT CONNACK Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt connect",
			Ctx:      "vernemq.node_mqtt_connack_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttConnectSentReason,
		}
		for _, v := range mqtt5CONNACKSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricCONNACKSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
)

// DISCONNECT
var (
	nodeMqtt5DISCONNECTPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_disconnect",
		Title:    "MQTT DISCONNECT Packets",
		Units:    "packets/s",
		Fam:      "mqtt disconnect",
		Ctx:      "vernemq.node_mqtt_disconnect",
		Priority: prioMqttDisconnectPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricDISCONNECTReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricDISCONNECTSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqtt4DISCONNECTPacketsChartTmpl = func() module.Chart {
		chart := nodeMqtt5DISCONNECTPacketsChartTmpl.Copy()
		_ = chart.RemoveDim(dimMqttVer(metricDISCONNECTSent))
		return *chart
	}()
	nodeMqttDISCONNECTReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_disconnect_received_by_reason_code",
			Title:    "MQTT DISCONNECT Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt disconnect",
			Ctx:      "vernemq.node_mqtt_disconnect_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttDisconnectReceivedReason,
		}
		for _, v := range mqtt5DISCONNECTReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricDISCONNECTReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttDISCONNECTSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_disconnect_sent_by_reason_code",
			Title:    "MQTT DISCONNECT Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt disconnect",
			Ctx:      "vernemq.node_mqtt_disconnect_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttDisconnectSentReason,
		}
		for _, v := range mqtt5DISCONNECTSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricDISCONNECTSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
)

// SUBSCRIBE
var (
	nodeMqttSUBSCRIBEPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_subscribe",
		Title:    "MQTT SUBSCRIBE and SUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe",
		Priority: prioMqttSubscribePackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEReceived), Name: "subscribe", Algo: module.Incremental},
			{ID: dimMqttVer(metricSUBACKSent), Name: "suback", Algo: module.Incremental, Mul: -1},
		},
	}
	modeMqttSUBSCRIBEErrorsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_subscribe_error",
		Title:    "MQTT Failed SUBSCRIBE Operations due to Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe_error",
		Priority: prioMqttSubscribeErrors,
		Dims: module.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEError), Name: "subscribe", Algo: module.Incremental},
		},
	}
	nodeMqttSUBSCRIBEAuthErrorsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_subscribe_auth_error",
		Title:    "MQTT Unauthorized SUBSCRIBE Attempts",
		Units:    "errors/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe_auth_error",
		Priority: prioMqttSubscribeAuthPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEAuthError), Name: "subscribe_auth", Algo: module.Incremental},
		},
	}
)

// UNSUBSCRIBE
var (
	nodeMqttUNSUBSCRIBEPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_unsubscribe",
		Title:    "MQTT UNSUBSCRIBE and UNSUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt unsubscribe",
		Ctx:      "vernemq.node_mqtt_unsubscribe",
		Priority: prioMqttUnsubscribePackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricUNSUBSCRIBEReceived), Name: "unsubscribe", Algo: module.Incremental},
			{ID: dimMqttVer(metricUNSUBACKSent), Name: "unsuback", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttUNSUBSCRIBEErrorsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_unsubscribe_error",
		Title:    "MQTT Failed UNSUBSCRIBE Operations due to Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt unsubscribe",
		Ctx:      "vernemq.node_mqtt_unsubscribe_error",
		Priority: prioMqttUnsubscribeErrors,
		Dims: module.Dims{
			{ID: dimMqttVer(metricUNSUBSCRIBEError), Name: "unsubscribe", Algo: module.Incremental},
		},
	}
)

// AUTH
var (
	nodeMqttAUTHPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt%s_auth",
		Title:    "MQTT AUTH Packets",
		Units:    "packets/s",
		Fam:      "mqtt auth",
		Ctx:      "vernemq.node_mqtt_auth",
		Priority: prioMqttAuthPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricAUTHReceived), Name: "received", Algo: module.Incremental},
			{ID: dimMqttVer(metricAUTHSent), Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nodeMqttAUTHReceivedByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_auth_received_by_reason_code",
			Title:    "MQTT AUTH Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt auth",
			Ctx:      "vernemq.node_mqtt_auth_received_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttAuthReceivedReason,
		}
		for _, v := range mqtt5AUTHReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricAUTHReceived, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
	nodeMqttAUTHSentByReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "node_%s_mqtt%s_auth_sent_by_reason_code",
			Title:    "MQTT AUTH Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt auth",
			Ctx:      "vernemq.node_mqtt_auth_sent_by_reason_code",
			Type:     module.Stacked,
			Priority: prioMqttAuthSentReason,
		}
		for _, v := range mqtt5AUTHSentReasonCodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID: dimMqttReason(metricAUTHSent, v), Name: v, Algo: module.Incremental,
			})
		}
		return chart
	}()
)

// PING
var (
	nodeMqttPINGPacketsChartTmpl = module.Chart{
		ID:       "node_%s_mqtt_ver_%s_ping",
		Title:    "MQTT PING Packets",
		Units:    "packets/s",
		Fam:      "mqtt ping",
		Ctx:      "vernemq.node_mqtt_ping",
		Priority: prioMqttPingPackets,
		Dims: module.Dims{
			{ID: dimMqttVer(metricPINGREQReceived), Name: "pingreq", Algo: module.Incremental},
			{ID: dimMqttVer(metricPINGRESPSent), Name: "pingresp", Algo: module.Incremental, Mul: -1},
		},
	}
)

func (c *Collector) addNodeCharts(node string, nst *nodeStats) {
	if err := c.Charts().Add(*newNodeCharts(node)...); err != nil {
		c.Warningf("error on adding node '%s' charts: %v", node, err)
	}
	if len(nst.mqtt4) > 0 {
		if err := c.Charts().Add(*newNodeMqttCharts(node, "4")...); err != nil {
			c.Warningf("error on adding node '%s' mqtt v4 charts: %v", node, err)
		}
	}
	if len(nst.mqtt5) > 0 {
		if err := c.Charts().Add(*newNodeMqttCharts(node, "5")...); err != nil {
			c.Warningf("error on adding node '%s' mqtt 5 charts: %v", node, err)
		}
	}
}

func (c *Collector) removeNodeCharts(node string) {
	px := cleanChartID(fmt.Sprintf("node_%s_", node))

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func newNodeCharts(node string) *module.Charts {
	charts := nodeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, node))
		chart.Labels = []module.Label{
			{Key: "node", Value: node},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, node)
		}
	}

	return charts
}

func newNodeMqttCharts(node, mqttVer string) *module.Charts {
	var charts *module.Charts

	switch mqttVer {
	case "4":
		charts = nodeMqtt4ChartsTmpl.Copy()
	case "5":
		charts = nodeMqtt5ChartsTmpl.Copy()
	default:
		return nil
	}

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, node, mqttVer))
		chart.Labels = []module.Label{
			{Key: "node", Value: node},
			{Key: "mqtt_version", Value: mqttVer},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, node, mqttVer)
		}
	}

	return charts
}

func dimNode(name string) string {
	return join("node_%s", name)
}

func dimMqttVer(name string) string {
	return join("node_%s_mqtt%s", name)
}

func dimMqttReason(name, reason string) string {
	return join("node_%s_mqtt%s", name, "reason_code", reason)
}

func dimMqttRCode(name, rcode string) string {
	return join("node_%s_mqtt%s", name, "return_code", rcode)
}

func cleanChartID(id string) string {
	r := strings.NewReplacer(".", "_", "'", "_", " ", "_")
	return r.Replace(id)
}
