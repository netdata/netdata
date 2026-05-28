// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioNodeSockets = collectorapi.Priority + iota
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

var nodeChartsTmpl = collectorapi.Charts{
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

var nodeMqtt5ChartsTmpl = collectorapi.Charts{
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

var nodeMqtt4ChartsTmpl = collectorapi.Charts{
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
	nodeOpenSocketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_sockets",
		Title:    "Open Sockets",
		Units:    "sockets",
		Fam:      "sockets",
		Ctx:      "vernemq.node_sockets",
		Priority: prioNodeSockets,
		Dims: collectorapi.Dims{
			{ID: dimNode("open_sockets"), Name: "open"},
		},
	}
	nodeSocketEventsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_socket_events",
		Title:    "Open and Close Socket Events",
		Units:    "events/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_operations",
		Priority: prioNodeSocketEvents,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSocketOpen), Name: "open", Algo: collectorapi.Incremental},
			{ID: dimNode(metricSocketClose), Name: "close", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeClientKeepaliveExpiredChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_client_keepalive_expired",
		Title:    "Closed Sockets due to Keepalive Time Expired",
		Units:    "sockets/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_client_keepalive_expired",
		Priority: prioNodeClientKeepaliveExpired,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricClientKeepaliveExpired), Name: "closed", Algo: collectorapi.Incremental},
		},
	}
	nodeSocketCloseTimeoutChartTmpl = collectorapi.Chart{
		ID:       "node_%s_socket_close_timeout",
		Title:    "Closed Sockets due to no CONNECT Frame On Time",
		Units:    "sockets/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_close_timeout",
		Priority: prioNodeSocketCloseTimeout,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSocketCloseTimeout), Name: "closed", Algo: collectorapi.Incremental},
		},
	}
	nodeSocketErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_socket_errors",
		Title:    "Socket Errors",
		Units:    "errors/s",
		Fam:      "sockets",
		Ctx:      "vernemq.node_socket_errors",
		Priority: prioNodeSocketErrors,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSocketError), Name: "errors", Algo: collectorapi.Incremental},
		},
	}
)

// Queues
var (
	nodeQueueProcessesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queue_processes",
		Title:    "Living Queues in an Online or an Offline State",
		Units:    "queue processes",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_processes",
		Priority: prioNodeQueueProcesses,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricQueueProcesses), Name: "queue_processes"},
		},
	}
	nodeQueueProcessesEventsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queue_processes_events",
		Title:    "Queue Processes Setup and Teardown Events",
		Units:    "events/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_processes_operations",
		Priority: prioNodeQueueProcessesEvents,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricQueueSetup), Name: "setup", Algo: collectorapi.Incremental},
			{ID: dimNode(metricQueueTeardown), Name: "teardown", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeQueueProcessesOfflineStorageChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queue_process_init_from_storage",
		Title:    "Queue Processes Initialized from Offline Storage",
		Units:    "queue processes/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_process_init_from_storage",
		Priority: prioNodeQueueProcessesOfflineStorage,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricQueueInitializedFromStorage), Name: "queue processes", Algo: collectorapi.Incremental},
		},
	}
	nodeQueueMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queue_messages",
		Title:    "Received and Sent PUBLISH Messages",
		Units:    "messages/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_messages",
		Type:     collectorapi.Area,
		Priority: prioNodeQueueMessages,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricQueueMessageIn), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimNode(metricQueueMessageOut), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeQueuedMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queued_messages",
		Title:    "Queued PUBLISH Messages",
		Units:    "messages",
		Fam:      "queues",
		Ctx:      "vernemq.node_queued_messages",
		Type:     collectorapi.Line,
		Priority: prioNodeQueuedMessages,
		Dims: collectorapi.Dims{
			{ID: dimNode("queued_messages"), Name: "queued"},
		},
	}
	nodeQueueUndeliveredMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_queue_undelivered_messages",
		Title:    "Undelivered PUBLISH Messages",
		Units:    "messages/s",
		Fam:      "queues",
		Ctx:      "vernemq.node_queue_undelivered_messages",
		Type:     collectorapi.Stacked,
		Priority: prioNodeQueueUndeliveredMessages,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricQueueMessageDrop), Name: "dropped", Algo: collectorapi.Incremental},
			{ID: dimNode(metricQueueMessageExpired), Name: "expired", Algo: collectorapi.Incremental},
			{ID: dimNode(metricQueueMessageUnhandled), Name: "unhandled", Algo: collectorapi.Incremental},
		},
	}
)

// Subscriptions
var (
	nodeRouterSubscriptionsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_router_subscriptions",
		Title:    "Subscriptions in the Routing Table",
		Units:    "subscriptions",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_subscriptions",
		Priority: prioNodeRouterSubscriptions,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricRouterSubscriptions), Name: "subscriptions"},
		},
	}
	nodeRouterMatchedSubscriptionsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_router_matched_subscriptions",
		Title:    "Matched Subscriptions",
		Units:    "subscriptions/s",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_matched_subscriptions",
		Priority: prioNodeRouterMatchedSubscriptions,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricRouterMatchesLocal), Name: "local", Algo: collectorapi.Incremental},
			{ID: dimNode(metricRouterMatchesRemote), Name: "remote", Algo: collectorapi.Incremental},
		},
	}
	nodeRouterMemoryChartTmpl = collectorapi.Chart{
		ID:       "node_%s_router_memory",
		Title:    "Routing Table Memory Usage",
		Units:    "bytes",
		Fam:      "subscriptions",
		Ctx:      "vernemq.node_router_memory",
		Type:     collectorapi.Area,
		Priority: prioNodeRouterMemory,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricRouterMemory), Name: "used"},
		},
	}
)

// Erlang VM
var (
	nodeAverageSchedulerUtilizationChartTmpl = collectorapi.Chart{
		ID:       "node_%s_average_scheduler_utilization",
		Title:    "Average Scheduler Utilization",
		Units:    "percentage",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_average_scheduler_utilization",
		Type:     collectorapi.Area,
		Priority: prioNodeAverageSchedulerUtilization,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemUtilization), Name: "utilization"},
		},
	}
	nodeSystemProcessesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_processes",
		Title:    "Erlang Processes",
		Units:    "processes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_processes",
		Priority: prioNodeSystemProcesses,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemProcessCount), Name: "processes"},
		},
	}
	nodeSystemReductionsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_reductions",
		Title:    "Reductions",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_reductions",
		Priority: prioNodeSystemReductions,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemReductions), Name: "reductions", Algo: collectorapi.Incremental},
		},
	}
	nodeSystemContextSwitches = collectorapi.Chart{
		ID:       "node_%s_system_context_switches",
		Title:    "Context Switches",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_context_switches",
		Priority: prioNodeSystemContext,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemContextSwitches), Name: "context switches", Algo: collectorapi.Incremental},
		},
	}
	nodeSystemIOChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_io",
		Title:    "Received and Sent Traffic through Ports",
		Units:    "bytes/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_io",
		Type:     collectorapi.Area,
		Priority: prioNodeSystemIO,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemIOIn), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimNode(metricSystemIOOut), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeSystemRunQueueChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_run_queue",
		Title:    "Processes that are Ready to Run on All Run-Queues",
		Units:    "processes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_run_queue",
		Priority: prioNodeSystemRunQueue,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemRunQueue), Name: "ready"},
		},
	}
	nodeSystemGCCountChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_gc_count",
		Title:    "GC Count",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_gc_count",
		Priority: prioNodeSystemGCCount,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemGCCount), Name: "gc", Algo: collectorapi.Incremental},
		},
	}
	nodeSystemGCWordsReclaimedChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_gc_words_reclaimed",
		Title:    "GC Words Reclaimed",
		Units:    "ops/s",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_gc_words_reclaimed",
		Priority: prioNodeSystemGCWordsReclaimed,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemWordsReclaimedByGC), Name: "words reclaimed", Algo: collectorapi.Incremental},
		},
	}
	nodeSystemMemoryAllocatedChartTmpl = collectorapi.Chart{
		ID:       "node_%s_system_allocated_memory",
		Title:    "Memory Allocated by the Erlang Processes and by the Emulator",
		Units:    "bytes",
		Fam:      "erlang vm",
		Ctx:      "vernemq.node_system_allocated_memory",
		Type:     collectorapi.Stacked,
		Priority: prioNodeSystemMemoryAllocated,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricVMMemoryProcesses), Name: "processes"},
			{ID: dimNode(metricVMMemorySystem), Name: "system"},
		},
	}
)

// Traffic
var (
	nodeTrafficChartTmpl = collectorapi.Chart{
		ID:       "node_%s_traffic",
		Title:    "Node Traffic",
		Units:    "bytes/s",
		Fam:      "traffic",
		Ctx:      "vernemq.node_traffic",
		Type:     collectorapi.Area,
		Priority: prioNodeTraffic,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricBytesReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimNode(metricBytesSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)

// Retain
var (
	nodeRetainMessagesChartsTmpl = collectorapi.Chart{
		ID:       "node_%s_retain_messages",
		Title:    "Stored Retained Messages",
		Units:    "messages",
		Fam:      "retain",
		Ctx:      "vernemq.node_retain_messages",
		Priority: prioNodeRetainMessages,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricRetainMessages), Name: "messages"},
		},
	}
	nodeRetainMemoryUsageChartTmpl = collectorapi.Chart{
		ID:       "node_%s_retain_memory",
		Title:    "Stored Retained Messages Memory Usage",
		Units:    "bytes",
		Fam:      "retain",
		Ctx:      "vernemq.node_retain_memory",
		Type:     collectorapi.Area,
		Priority: prioNodeRetainMemoryUsage,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricRetainMemory), Name: "used"},
		},
	}
)

// Cluster
var (
	nodeClusterCommunicationTrafficChartTmpl = collectorapi.Chart{
		ID:       "node_%s_cluster_traffic",
		Title:    "Communication with Other Cluster Nodes",
		Units:    "bytes/s",
		Fam:      "cluster",
		Ctx:      "vernemq.node_cluster_traffic",
		Type:     collectorapi.Area,
		Priority: prioNodeClusterCommunicationTraffic,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricClusterBytesReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimNode(metricClusterBytesSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeClusterCommunicationDroppedChartTmpl = collectorapi.Chart{
		ID:       "node_%s_cluster_dropped",
		Title:    "Traffic Dropped During Communication with Other Cluster Nodes",
		Units:    "bytes/s",
		Fam:      "cluster",
		Type:     collectorapi.Area,
		Ctx:      "vernemq.node_cluster_dropped",
		Priority: prioNodeClusterCommunicationDropped,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricClusterBytesDropped), Name: "dropped", Algo: collectorapi.Incremental},
		},
	}
	nodeNetSplitUnresolvedChartTmpl = collectorapi.Chart{
		ID:       "node_%s_netsplit_unresolved",
		Title:    "Unresolved Netsplits",
		Units:    "netsplits",
		Fam:      "cluster",
		Ctx:      "vernemq.node_netsplit_unresolved",
		Priority: prioNodeNetSplitUnresolved,
		Dims: collectorapi.Dims{
			{ID: dimNode("netsplit_unresolved"), Name: "unresolved"},
		},
	}
	nodeNetSplitsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_netsplit",
		Title:    "Netsplits",
		Units:    "netsplits/s",
		Fam:      "cluster",
		Ctx:      "vernemq.node_netsplits",
		Type:     collectorapi.Stacked,
		Priority: prioNodeNetSplits,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricNetSplitResolved), Name: "resolved", Algo: collectorapi.Incremental},
			{ID: dimNode(metricNetSplitDetected), Name: "detected", Algo: collectorapi.Incremental},
		},
	}
)

var (
	nodeUptimeChartTmpl = collectorapi.Chart{
		ID:       "node_%s_uptime",
		Title:    "Node Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "vernemq.node_uptime",
		Priority: prioNodeUptime,
		Dims: collectorapi.Dims{
			{ID: dimNode(metricSystemWallClock), Name: "time", Div: 1000},
		},
	}
)

// PUBLISH
var (
	nodeMqttPUBLISHPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_publish",
		Title:    "MQTT QoS 0,1,2 PUBLISH",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish",
		Priority: prioMqttPublishPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBSLISHReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPUBSLIHSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBLISHErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_publish_errors",
		Title:    "MQTT Failed PUBLISH Operations due to a Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish_errors",
		Priority: prioMqttPublishErrors,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBLISHError), Name: "publish", Algo: collectorapi.Incremental},
		},
	}
	nodeMqttPUBLISHAuthErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_publish_auth_errors",
		Title:    "MQTT Unauthorized PUBLISH Attempts",
		Units:    "errors/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_publish_auth_errors",
		Type:     collectorapi.Area,
		Priority: prioMqttPublishAuthPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBLISHAuthError), Name: "publish_auth", Algo: collectorapi.Incremental},
		},
	}
	nodeMqttPUBACKPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_puback",
		Title:    "MQTT QoS 1 PUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_puback",
		Priority: prioMqttPubAckPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBACKReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPUBACKSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBACKReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_puback_received_by_reason_code",
			Title:    "MQTT PUBACK QoS 1 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_puback_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubAckReceivedReason,
		}
		for _, v := range mqtt5PUBACKReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBACKReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBACKSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_puback_sent_by_reason_code",
			Title:    "MQTT PUBACK QoS 1 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_puback_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubAckSentReason,
		}
		for _, v := range mqtt5PUBACKSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBACKSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBACKUnexpectedMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_puback_unexpected",
		Title:    "MQTT PUBACK QoS 1 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_puback_invalid_error",
		Priority: prioMqttPubAckUnexpectedMessages,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBACKInvalid), Name: "unexpected", Algo: collectorapi.Incremental},
		},
	}
	nodeMqttPUBRECPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_pubrec",
		Title:    "MQTT PUBREC QoS 2 Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrec",
		Priority: prioMqttPubRecPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBRECReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPUBRECSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBRECReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubrec_received_by_reason_code",
			Title:    "MQTT PUBREC QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrec_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubRecReceivedReason,
		}
		for _, v := range mqtt5PUBRECReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBRECReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRECSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubrec_sent_by_reason_code",
			Title:    "MQTT PUBREC QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrec_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubRecSentReason,
		}
		for _, v := range mqtt5PUBRECSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBRECSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRECUnexpectedMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_pubrec_unexpected",
		Title:    "MQTT PUBREC QoS 2 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrec_invalid_error",
		Priority: prioMqttPubRecUnexpectedMessages,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBRECInvalid), Name: "unexpected", Algo: collectorapi.Incremental},
		},
	}
	nodeMqttPUBRELPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_pubrel",
		Title:    "MQTT PUBREL QoS 2 Packets¬",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubrel",
		Priority: prioMqttPubRelPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBRELReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPUBRELSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBRELReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubrel_received_by_reason_code",
			Title:    "MQTT PUBREL QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrel_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubRelReceivedReason,
		}
		for _, v := range mqtt5PUBRELReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBRELReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBRELSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubrel_sent_by_reason_code",
			Title:    "MQTT PUBREL QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubrel_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubRelSentReason,
		}
		for _, v := range mqtt5PUBRELSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBRELSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_pubcomp",
		Title:    "MQTT PUBCOMP QoS 2 Packets",
		Units:    "packets/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubcomp",
		Priority: prioMqttPubCompPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUBCOMPReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPUBCOMPSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttPUBCOMPReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubcomp_received_by_reason_code",
			Title:    "MQTT PUBCOMP QoS 2 Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubcomp_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubCompReceivedReason,
		}
		for _, v := range mqtt5PUBCOMPReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBCOMPReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_pubcomp_sent_by_reason_code",
			Title:    "MQTT PUBCOMP QoS 2 Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt publish",
			Ctx:      "vernemq.node_mqtt_pubcomp_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttPubCompSentReason,
		}
		for _, v := range mqtt5PUBCOMPSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricPUBCOMPSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttPUBCOMPUnexpectedMessagesChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_pubcomp_unexpected",
		Title:    "MQTT PUBCOMP QoS 2 Received Unexpected Messages",
		Units:    "messages/s",
		Fam:      "mqtt publish",
		Ctx:      "vernemq.node_mqtt_pubcomp_invalid_error",
		Priority: prioMqttPubCompUnexpectedMessages,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPUNCOMPInvalid), Name: "unexpected", Algo: collectorapi.Incremental},
		},
	}
)

// CONNECT
var (
	nodeMqttCONNECTPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_connect",
		Title:    "MQTT CONNECT and CONNACK",
		Units:    "packets/s",
		Fam:      "mqtt connect",
		Ctx:      "vernemq.node_mqtt_connect",
		Priority: prioMqttConnectPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricCONNECTReceived), Name: "connect", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricCONNACKSent), Name: "connack", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttCONNACKSentByReturnCodeChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_connack_sent_by_return_code",
			Title:    "MQTT CONNACK Sent by Return Code",
			Units:    "packets/s",
			Fam:      "mqtt connect",
			Ctx:      "vernemq.node_mqtt_connack_sent_by_return_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttConnectSentReason,
		}
		for _, v := range mqtt4CONNACKSentReturnCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttRCode(metricCONNACKSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttCONNACKSentByReasonCodeChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_connack_sent_by_reason_code",
			Title:    "MQTT CONNACK Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt connect",
			Ctx:      "vernemq.node_mqtt_connack_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttConnectSentReason,
		}
		for _, v := range mqtt5CONNACKSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricCONNACKSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
)

// DISCONNECT
var (
	nodeMqtt5DISCONNECTPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_disconnect",
		Title:    "MQTT DISCONNECT Packets",
		Units:    "packets/s",
		Fam:      "mqtt disconnect",
		Ctx:      "vernemq.node_mqtt_disconnect",
		Priority: prioMqttDisconnectPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricDISCONNECTReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricDISCONNECTSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqtt4DISCONNECTPacketsChartTmpl = func() collectorapi.Chart {
		chart := nodeMqtt5DISCONNECTPacketsChartTmpl.Copy()
		_ = chart.RemoveDim(dimMqttVer(metricDISCONNECTSent))
		return *chart
	}()
	nodeMqttDISCONNECTReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_disconnect_received_by_reason_code",
			Title:    "MQTT DISCONNECT Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt disconnect",
			Ctx:      "vernemq.node_mqtt_disconnect_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttDisconnectReceivedReason,
		}
		for _, v := range mqtt5DISCONNECTReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricDISCONNECTReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttDISCONNECTSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_disconnect_sent_by_reason_code",
			Title:    "MQTT DISCONNECT Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt disconnect",
			Ctx:      "vernemq.node_mqtt_disconnect_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttDisconnectSentReason,
		}
		for _, v := range mqtt5DISCONNECTSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricDISCONNECTSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
)

// SUBSCRIBE
var (
	nodeMqttSUBSCRIBEPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_subscribe",
		Title:    "MQTT SUBSCRIBE and SUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe",
		Priority: prioMqttSubscribePackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEReceived), Name: "subscribe", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricSUBACKSent), Name: "suback", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	modeMqttSUBSCRIBEErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_subscribe_error",
		Title:    "MQTT Failed SUBSCRIBE Operations due to Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe_error",
		Priority: prioMqttSubscribeErrors,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEError), Name: "subscribe", Algo: collectorapi.Incremental},
		},
	}
	nodeMqttSUBSCRIBEAuthErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_subscribe_auth_error",
		Title:    "MQTT Unauthorized SUBSCRIBE Attempts",
		Units:    "errors/s",
		Fam:      "mqtt subscribe",
		Ctx:      "vernemq.node_mqtt_subscribe_auth_error",
		Priority: prioMqttSubscribeAuthPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricSUBSCRIBEAuthError), Name: "subscribe_auth", Algo: collectorapi.Incremental},
		},
	}
)

// UNSUBSCRIBE
var (
	nodeMqttUNSUBSCRIBEPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_unsubscribe",
		Title:    "MQTT UNSUBSCRIBE and UNSUBACK Packets",
		Units:    "packets/s",
		Fam:      "mqtt unsubscribe",
		Ctx:      "vernemq.node_mqtt_unsubscribe",
		Priority: prioMqttUnsubscribePackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricUNSUBSCRIBEReceived), Name: "unsubscribe", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricUNSUBACKSent), Name: "unsuback", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttUNSUBSCRIBEErrorsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_unsubscribe_error",
		Title:    "MQTT Failed UNSUBSCRIBE Operations due to Netsplit",
		Units:    "errors/s",
		Fam:      "mqtt unsubscribe",
		Ctx:      "vernemq.node_mqtt_unsubscribe_error",
		Priority: prioMqttUnsubscribeErrors,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricUNSUBSCRIBEError), Name: "unsubscribe", Algo: collectorapi.Incremental},
		},
	}
)

// AUTH
var (
	nodeMqttAUTHPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt%s_auth",
		Title:    "MQTT AUTH Packets",
		Units:    "packets/s",
		Fam:      "mqtt auth",
		Ctx:      "vernemq.node_mqtt_auth",
		Priority: prioMqttAuthPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricAUTHReceived), Name: "received", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricAUTHSent), Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	nodeMqttAUTHReceivedByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_auth_received_by_reason_code",
			Title:    "MQTT AUTH Received by Reason",
			Units:    "packets/s",
			Fam:      "mqtt auth",
			Ctx:      "vernemq.node_mqtt_auth_received_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttAuthReceivedReason,
		}
		for _, v := range mqtt5AUTHReceivedReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricAUTHReceived, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	nodeMqttAUTHSentByReasonChartTmpl = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "node_%s_mqtt%s_auth_sent_by_reason_code",
			Title:    "MQTT AUTH Sent by Reason",
			Units:    "packets/s",
			Fam:      "mqtt auth",
			Ctx:      "vernemq.node_mqtt_auth_sent_by_reason_code",
			Type:     collectorapi.Stacked,
			Priority: prioMqttAuthSentReason,
		}
		for _, v := range mqtt5AUTHSentReasonCodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID: dimMqttReason(metricAUTHSent, v), Name: v, Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
)

// PING
var (
	nodeMqttPINGPacketsChartTmpl = collectorapi.Chart{
		ID:       "node_%s_mqtt_ver_%s_ping",
		Title:    "MQTT PING Packets",
		Units:    "packets/s",
		Fam:      "mqtt ping",
		Ctx:      "vernemq.node_mqtt_ping",
		Priority: prioMqttPingPackets,
		Dims: collectorapi.Dims{
			{ID: dimMqttVer(metricPINGREQReceived), Name: "pingreq", Algo: collectorapi.Incremental},
			{ID: dimMqttVer(metricPINGRESPSent), Name: "pingresp", Algo: collectorapi.Incremental, Mul: -1},
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

func newNodeCharts(node string) *collectorapi.Charts {
	charts := nodeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, node))
		chart.Labels = []collectorapi.Label{
			{Key: "node", Value: node},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, node)
		}
	}

	return charts
}

func newNodeMqttCharts(node, mqttVer string) *collectorapi.Charts {
	var charts *collectorapi.Charts

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
		chart.Labels = []collectorapi.Label{
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
