// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	Charts = module.Charts
	Chart  = module.Chart
	Dims   = module.Dims
	Dim    = module.Dim
)

var charts = Charts{
	chartOpenSockets.Copy(),
	chartSocketEvents.Copy(),
	chartClientKeepaliveExpired.Copy(),
	chartSocketErrors.Copy(),
	chartSocketCloseTimeout.Copy(),

	chartQueueProcesses.Copy(),
	chartQueueProcessesEvents.Copy(),
	chartQueueProcessesOfflineStorage.Copy(),
	chartQueueMessages.Copy(),
	chartQueueUndeliveredMessages.Copy(),

	chartRouterSubscriptions.Copy(),
	chartRouterMatchedSubscriptions.Copy(),
	chartRouterMemory.Copy(),

	chartAverageSchedulerUtilization.Copy(),
	chartSchedulerUtilization.Copy(),
	chartSystemProcesses.Copy(),
	chartSystemReductions.Copy(),
	chartSystemContextSwitches.Copy(),
	chartSystemIO.Copy(),
	chartSystemRunQueue.Copy(),
	chartSystemGCCount.Copy(),
	chartSystemGCWordsReclaimed.Copy(),
	chartSystemMemoryAllocated.Copy(),

	chartBandwidth.Copy(),

	chartRetainMessages.Copy(),
	chartRetainMemoryUsage.Copy(),

	chartClusterCommunicationBandwidth.Copy(),
	chartClusterCommunicationDropped.Copy(),
	chartNetSplitUnresolved.Copy(),
	chartNetSplits.Copy(),

	chartMQTTv5AUTH.Copy(),
	chartMQTTv5AUTHReceivedReason.Copy(),
	chartMQTTv5AUTHSentReason.Copy(),

	chartMQTTv3v5CONNECT.Copy(),
	chartMQTTv3v5CONNACKSentReason.Copy(),

	chartMQTTv3v5DISCONNECT.Copy(),
	chartMQTTv5DISCONNECTReceivedReason.Copy(),
	chartMQTTv5DISCONNECTSentReason.Copy(),

	chartMQTTv3v5SUBSCRIBE.Copy(),
	chartMQTTv3v5SUBSCRIBEError.Copy(),
	chartMQTTv3v5SUBSCRIBEAuthError.Copy(),

	chartMQTTv3v5UNSUBSCRIBE.Copy(),
	chartMQTTv3v5UNSUBSCRIBEError.Copy(),

	chartMQTTv3v5PUBLISH.Copy(),
	chartMQTTv3v5PUBLISHErrors.Copy(),
	chartMQTTv3v5PUBLISHAuthErrors.Copy(),
	chartMQTTv3v5PUBACK.Copy(),
	chartMQTTv5PUBACKReceivedReason.Copy(),
	chartMQTTv5PUBACKSentReason.Copy(),
	chartMQTTv3v5PUBACKUnexpected.Copy(),
	chartMQTTv3v5PUBREC.Copy(),
	chartMQTTv5PUBRECReceivedReason.Copy(),
	chartMQTTv5PUBRECSentReason.Copy(),
	chartMQTTv3PUBRECUnexpected.Copy(),
	chartMQTTv3v5PUBREL.Copy(),
	chartMQTTv5PUBRELReceivedReason.Copy(),
	chartMQTTv5PUBRELSentReason.Copy(),
	chartMQTTv3v5PUBCOMP.Copy(),
	chartMQTTv5PUBCOMPReceivedReason.Copy(),
	chartMQTTv5PUBCOMPSentReason.Copy(),
	chartMQTTv3v5PUBCOMPUnexpected.Copy(),

	chartMQTTv3v5PING.Copy(),

	chartUptime.Copy(),
}

// Sockets
var (
	chartOpenSockets = Chart{
		ID:    "sockets",
		Title: "Open Sockets",
		Units: "sockets",
		Fam:   "sockets",
		Ctx:   "vernemq.sockets",
		Dims: Dims{
			{ID: "open_sockets", Name: "open"},
		},
	}
	chartSocketEvents = Chart{
		ID:    "socket_events",
		Title: "Socket Open and Close Events",
		Units: "events/s",
		Fam:   "sockets",
		Ctx:   "vernemq.socket_operations",
		Dims: Dims{
			{ID: metricSocketOpen, Name: "open", Algo: module.Incremental},
			{ID: metricSocketClose, Name: "close", Algo: module.Incremental, Mul: -1},
		},
	}
	chartClientKeepaliveExpired = Chart{
		ID:    "client_keepalive_expired",
		Title: "Closed Sockets due to Keepalive Time Expired",
		Units: "sockets/s",
		Fam:   "sockets",
		Ctx:   "vernemq.client_keepalive_expired",
		Dims: Dims{
			{ID: metricClientKeepaliveExpired, Name: "closed", Algo: module.Incremental},
		},
	}
	chartSocketCloseTimeout = Chart{
		ID:    "socket_close_timeout",
		Title: "Closed Sockets due to no CONNECT Frame On Time",
		Units: "sockets/s",
		Fam:   "sockets",
		Ctx:   "vernemq.socket_close_timeout",
		Dims: Dims{
			{ID: metricSocketCloseTimeout, Name: "closed", Algo: module.Incremental},
		},
	}
	chartSocketErrors = Chart{
		ID:    "socket_errors",
		Title: "Socket Errors",
		Units: "errors/s",
		Fam:   "sockets",
		Ctx:   "vernemq.socket_errors",
		Dims: Dims{
			{ID: metricSocketError, Name: "errors", Algo: module.Incremental},
		},
	}
)

// Queues
var (
	chartQueueProcesses = Chart{
		ID:    "queue_processes",
		Title: "Living Queues in an Online or an Offline State",
		Units: "queue processes",
		Fam:   "queues",
		Ctx:   "vernemq.queue_processes",
		Dims: Dims{
			{ID: metricQueueProcesses, Name: "queue_processes"},
		},
	}
	chartQueueProcessesEvents = Chart{
		ID:    "queue_processes_events",
		Title: "Queue Processes Setup and Teardown Events",
		Units: "events/s",
		Fam:   "queues",
		Ctx:   "vernemq.queue_processes_operations",
		Dims: Dims{
			{ID: metricQueueSetup, Name: "setup", Algo: module.Incremental},
			{ID: metricQueueTeardown, Name: "teardown", Algo: module.Incremental, Mul: -1},
		},
	}
	chartQueueProcessesOfflineStorage = Chart{
		ID:    "queue_process_init_from_storage",
		Title: "Queue Processes Initialized from Offline Storage",
		Units: "queue processes/s",
		Fam:   "queues",
		Ctx:   "vernemq.queue_process_init_from_storage",
		Dims: Dims{
			{ID: metricQueueInitializedFromStorage, Name: "queue processes", Algo: module.Incremental},
		},
	}
	chartQueueMessages = Chart{
		ID:    "queue_messages",
		Title: "Received and Sent PUBLISH Messages",
		Units: "messages/s",
		Fam:   "queues",
		Ctx:   "vernemq.queue_messages",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricQueueMessageIn, Name: "received", Algo: module.Incremental},
			{ID: metricQueueMessageOut, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartQueueUndeliveredMessages = Chart{
		ID:    "queue_undelivered_messages",
		Title: "Undelivered PUBLISH Messages",
		Units: "messages/s",
		Fam:   "queues",
		Ctx:   "vernemq.queue_undelivered_messages",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: metricQueueMessageDrop, Name: "dropped", Algo: module.Incremental},
			{ID: metricQueueMessageExpired, Name: "expired", Algo: module.Incremental},
			{ID: metricQueueMessageUnhandled, Name: "unhandled", Algo: module.Incremental},
		},
	}
)

// Subscriptions
var (
	chartRouterSubscriptions = Chart{
		ID:    "router_subscriptions",
		Title: "Subscriptions in the Routing Table",
		Units: "subscriptions",
		Fam:   "subscriptions",
		Ctx:   "vernemq.router_subscriptions",
		Dims: Dims{
			{ID: metricRouterSubscriptions, Name: "subscriptions"},
		},
	}
	chartRouterMatchedSubscriptions = Chart{
		ID:    "router_matched_subscriptions",
		Title: "Matched Subscriptions",
		Units: "subscriptions/s",
		Fam:   "subscriptions",
		Ctx:   "vernemq.router_matched_subscriptions",
		Dims: Dims{
			{ID: metricRouterMatchesLocal, Name: "local", Algo: module.Incremental},
			{ID: metricRouterMatchesRemote, Name: "remote", Algo: module.Incremental},
		},
	}
	chartRouterMemory = Chart{
		ID:    "router_memory",
		Title: "Routing Table Memory Usage",
		Units: "KiB",
		Fam:   "subscriptions",
		Ctx:   "vernemq.router_memory",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricRouterMemory, Name: "used", Div: 1024},
		},
	}
)

// Erlang VM
var (
	chartAverageSchedulerUtilization = Chart{
		ID:    "average_scheduler_utilization",
		Title: "Average Scheduler Utilization",
		Units: "percentage",
		Fam:   "erlang vm",
		Ctx:   "vernemq.average_scheduler_utilization",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricSystemUtilization, Name: "utilization"},
		},
	}
	chartSchedulerUtilization = Chart{
		ID:    "scheduler_utilization",
		Title: "Scheduler Utilization",
		Units: "percentage",
		Fam:   "erlang vm",
		Type:  module.Stacked,
		Ctx:   "vernemq.system_utilization_scheduler",
	}
	chartSystemProcesses = Chart{
		ID:    "system_processes",
		Title: "Erlang Processes",
		Units: "processes",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_processes",
		Dims: Dims{
			{ID: metricSystemProcessCount, Name: "processes"},
		},
	}
	chartSystemReductions = Chart{
		ID:    "system_reductions",
		Title: "Reductions",
		Units: "ops/s",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_reductions",
		Dims: Dims{
			{ID: metricSystemReductions, Name: "reductions", Algo: module.Incremental},
		},
	}
	chartSystemContextSwitches = Chart{
		ID:    "system_context_switches",
		Title: "Context Switches",
		Units: "ops/s",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_context_switches",
		Dims: Dims{
			{ID: metricSystemContextSwitches, Name: "context switches", Algo: module.Incremental},
		},
	}
	chartSystemIO = Chart{
		ID:    "system_io",
		Title: "Received and Sent Traffic through Ports",
		Units: "kilobits/s",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_io",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricSystemIOIn, Name: "received", Algo: module.Incremental, Mul: 8, Div: 1024},
			{ID: metricSystemIOOut, Name: "sent", Algo: module.Incremental, Mul: 8, Div: -1024},
		},
	}
	chartSystemRunQueue = Chart{
		ID:    "system_run_queue",
		Title: "Processes that are Ready to Run on All Run-Queues",
		Units: "processes",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_run_queue",
		Dims: Dims{
			{ID: metricSystemRunQueue, Name: "ready"},
		},
	}
	chartSystemGCCount = Chart{
		ID:    "system_gc_count",
		Title: "GC Count",
		Units: "ops/s",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_gc_count",
		Dims: Dims{
			{ID: metricSystemGCCount, Name: "gc", Algo: module.Incremental},
		},
	}
	chartSystemGCWordsReclaimed = Chart{
		ID:    "system_gc_words_reclaimed",
		Title: "GC Words Reclaimed",
		Units: "ops/s",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_gc_words_reclaimed",
		Dims: Dims{
			{ID: metricSystemWordsReclaimedByGC, Name: "words reclaimed", Algo: module.Incremental},
		},
	}
	chartSystemMemoryAllocated = Chart{
		ID:    "system_allocated_memory",
		Title: "Memory Allocated by the Erlang Processes and by the Emulator",
		Units: "KiB",
		Fam:   "erlang vm",
		Ctx:   "vernemq.system_allocated_memory",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: metricVMMemoryProcesses, Name: "processes", Div: 1024},
			{ID: metricVMMemorySystem, Name: "system", Div: 1024},
		},
	}
)

// Bandwidth
var (
	chartBandwidth = Chart{
		ID:    "bandwidth",
		Title: "Bandwidth",
		Units: "kilobits/s",
		Fam:   "bandwidth",
		Ctx:   "vernemq.bandwidth",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricBytesReceived, Name: "received", Algo: module.Incremental, Mul: 8, Div: 1024},
			{ID: metricBytesSent, Name: "sent", Algo: module.Incremental, Mul: 8, Div: -1024},
		},
	}
)

// Retain
var (
	chartRetainMessages = Chart{
		ID:    "retain_messages",
		Title: "Stored Retained Messages",
		Units: "messages",
		Fam:   "retain",
		Ctx:   "vernemq.retain_messages",
		Dims: Dims{
			{ID: metricRetainMessages, Name: "messages"},
		},
	}
	chartRetainMemoryUsage = Chart{
		ID:    "retain_memory",
		Title: "Stored Retained Messages Memory Usage",
		Units: "KiB",
		Fam:   "retain",
		Ctx:   "vernemq.retain_memory",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricRetainMemory, Name: "used", Div: 1024},
		},
	}
)

// Cluster
var (
	chartClusterCommunicationBandwidth = Chart{
		ID:    "cluster_bandwidth",
		Title: "Communication with Other Cluster Nodes",
		Units: "kilobits/s",
		Fam:   "cluster",
		Ctx:   "vernemq.cluster_bandwidth",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricClusterBytesReceived, Name: "received", Algo: module.Incremental, Mul: 8, Div: 1024},
			{ID: metricClusterBytesSent, Name: "sent", Algo: module.Incremental, Mul: 8, Div: -1024},
		},
	}
	chartClusterCommunicationDropped = Chart{
		ID:    "cluster_dropped",
		Title: "Traffic Dropped During Communication with Other Cluster Nodes",
		Units: "kilobits/s",
		Fam:   "cluster",
		Type:  module.Area,
		Ctx:   "vernemq.cluster_dropped",
		Dims: Dims{
			{ID: metricClusterBytesDropped, Name: "dropped", Algo: module.Incremental, Mul: 8, Div: 1024},
		},
	}
	chartNetSplitUnresolved = Chart{
		ID:    "netsplit_unresolved",
		Title: "Unresolved Netsplits",
		Units: "netsplits",
		Fam:   "cluster",
		Ctx:   "vernemq.netsplit_unresolved",
		Dims: Dims{
			{ID: "netsplit_unresolved", Name: "unresolved"},
		},
	}
	chartNetSplits = Chart{
		ID:    "netsplit",
		Title: "Netsplits",
		Units: "netsplits/s",
		Fam:   "cluster",
		Ctx:   "vernemq.netsplits",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: metricNetSplitResolved, Name: "resolved", Algo: module.Incremental},
			{ID: metricNetSplitDetected, Name: "detected", Algo: module.Incremental},
		},
	}
)

// AUTH
var (
	chartMQTTv5AUTH = Chart{
		ID:    "mqtt_auth",
		Title: "v5 AUTH",
		Units: "packets/s",
		Fam:   "mqtt auth",
		Ctx:   "vernemq.mqtt_auth",
		Dims: Dims{
			{ID: metricAUTHReceived, Name: "received", Algo: module.Incremental},
			{ID: metricAUTHSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5AUTHReceivedReason = Chart{
		ID:    "mqtt_auth_received_reason",
		Title: "v5 AUTH Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt auth",
		Ctx:   "vernemq.mqtt_auth_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricAUTHReceived, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv5AUTHSentReason = Chart{
		ID:    "mqtt_auth_sent_reason",
		Title: "v5 AUTH Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt auth",
		Ctx:   "vernemq.mqtt_auth_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricAUTHSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
)

// CONNECT
var (
	chartMQTTv3v5CONNECT = Chart{
		ID:    "mqtt_connect",
		Title: "v3/v5 CONNECT and CONNACK",
		Units: "packets/s",
		Fam:   "mqtt connect",
		Ctx:   "vernemq.mqtt_connect",
		Dims: Dims{
			{ID: metricCONNECTReceived, Name: "CONNECT", Algo: module.Incremental},
			{ID: metricCONNACKSent, Name: "CONNACK", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv3v5CONNACKSentReason = Chart{
		ID:    "mqtt_connack_sent_reason",
		Title: "v3/v5 CONNACK Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt connect",
		Ctx:   "vernemq.mqtt_connack_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricCONNACKSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
)

// DISCONNECT
var (
	chartMQTTv3v5DISCONNECT = Chart{
		ID:    "mqtt_disconnect",
		Title: "v3/v5 DISCONNECT",
		Units: "packets/s",
		Fam:   "mqtt disconnect",
		Ctx:   "vernemq.mqtt_disconnect",
		Dims: Dims{
			{ID: metricDISCONNECTReceived, Name: "received", Algo: module.Incremental},
			{ID: metricDISCONNECTSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5DISCONNECTReceivedReason = Chart{
		ID:    "mqtt_disconnect_received_reason",
		Title: "v5 DISCONNECT Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt disconnect",
		Ctx:   "vernemq.mqtt_disconnect_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricDISCONNECTReceived, "normal_disconnect"), Name: "normal_disconnect", Algo: module.Incremental},
		},
	}
	chartMQTTv5DISCONNECTSentReason = Chart{
		ID:    "mqtt_disconnect_sent_reason",
		Title: "v5 DISCONNECT Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt disconnect",
		Ctx:   "vernemq.mqtt_disconnect_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricDISCONNECTSent, "normal_disconnect"), Name: "normal_disconnect", Algo: module.Incremental},
		},
	}
)

// SUBSCRIBE
var (
	chartMQTTv3v5SUBSCRIBE = Chart{
		ID:    "mqtt_subscribe",
		Title: "v3/v5 SUBSCRIBE and SUBACK",
		Units: "packets/s",
		Fam:   "mqtt subscribe",
		Ctx:   "vernemq.mqtt_subscribe",
		Dims: Dims{
			{ID: metricSUBSCRIBEReceived, Name: "SUBSCRIBE", Algo: module.Incremental},
			{ID: metricSUBACKSent, Name: "SUBACK", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv3v5SUBSCRIBEError = Chart{
		ID:    "mqtt_subscribe_error",
		Title: "v3/v5 Failed SUBSCRIBE Operations due to a Netsplit",
		Units: "ops/s",
		Fam:   "mqtt subscribe",
		Ctx:   "vernemq.mqtt_subscribe_error",
		Dims: Dims{
			{ID: metricSUBSCRIBEError, Name: "failed", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5SUBSCRIBEAuthError = Chart{
		ID:    "mqtt_subscribe_auth_error",
		Title: "v3/v5 Unauthorized SUBSCRIBE Attempts",
		Units: "attempts/s",
		Fam:   "mqtt subscribe",
		Ctx:   "vernemq.mqtt_subscribe_auth_error",
		Dims: Dims{
			{ID: metricSUBSCRIBEAuthError, Name: "unauth", Algo: module.Incremental},
		},
	}
)

// UNSUBSCRIBE
var (
	chartMQTTv3v5UNSUBSCRIBE = Chart{
		ID:    "mqtt_unsubscribe",
		Title: "v3/v5 UNSUBSCRIBE and UNSUBACK",
		Units: "packets/s",
		Fam:   "mqtt unsubscribe",
		Ctx:   "vernemq.mqtt_unsubscribe",
		Dims: Dims{
			{ID: metricUNSUBSCRIBEReceived, Name: "UNSUBSCRIBE", Algo: module.Incremental},
			{ID: metricUNSUBACKSent, Name: "UNSUBACK", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv3v5UNSUBSCRIBEError = Chart{
		ID:    "mqtt_unsubscribe_error",
		Title: "v3/v5 Failed UNSUBSCRIBE Operations due to a Netsplit",
		Units: "ops/s",
		Fam:   "mqtt unsubscribe",
		Ctx:   "vernemq.mqtt_unsubscribe_error",
		Dims: Dims{
			{ID: metricUNSUBSCRIBEError, Name: "failed", Algo: module.Incremental},
		},
	}
)

// PUBLISH
var (
	chartMQTTv3v5PUBLISH = Chart{
		ID:    "mqtt_publish",
		Title: "v3/v5 QoS 0,1,2 PUBLISH",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_publish",
		Dims: Dims{
			{ID: metricPUBSLISHReceived, Name: "received", Algo: module.Incremental},
			{ID: metricPUBSLIHSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv3v5PUBLISHErrors = Chart{
		ID:    "mqtt_publish_errors",
		Title: "v3/v5 Failed PUBLISH Operations due to a Netsplit",
		Units: "ops/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_publish_errors",
		Dims: Dims{
			{ID: metricPUBLISHError, Name: "failed", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBLISHAuthErrors = Chart{
		ID:    "mqtt_publish_auth_errors",
		Title: "v3/v5 Unauthorized PUBLISH Attempts",
		Units: "attempts/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_publish_auth_errors",
		Type:  module.Area,
		Dims: Dims{
			{ID: metricPUBLISHAuthError, Name: "unauth", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBACK = Chart{
		ID:    "mqtt_puback",
		Title: "v3/v5 QoS 1 PUBACK",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_puback",
		Dims: Dims{
			{ID: metricPUBACKReceived, Name: "received", Algo: module.Incremental},
			{ID: metricPUBACKSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5PUBACKReceivedReason = Chart{
		ID:    "mqtt_puback_received_reason",
		Title: "v5 PUBACK QoS 1 Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_puback_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBACKReceived, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv5PUBACKSentReason = Chart{
		ID:    "mqtt_puback_sent_reason",
		Title: "v5 PUBACK QoS 1 Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_puback_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBACKSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBACKUnexpected = Chart{
		ID:    "mqtt_puback_unexpected",
		Title: "v3/v5 PUBACK QoS 1 Received Unexpected Messages",
		Units: "messages/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_puback_invalid_error",
		Dims: Dims{
			{ID: metricPUBACKInvalid, Name: "unexpected", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBREC = Chart{
		ID:    "mqtt_pubrec",
		Title: "v3/v5 PUBREC QoS 2",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrec",
		Dims: Dims{
			{ID: metricPUBRECReceived, Name: "received", Algo: module.Incremental},
			{ID: metricPUBRECSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5PUBRECReceivedReason = Chart{
		ID:    "mqtt_pubrec_received_reason",
		Title: "v5 PUBREC QoS 2 Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrec_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBRECReceived, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv5PUBRECSentReason = Chart{
		ID:    "mqtt_pubrec_sent_reason",
		Title: "v5 PUBREC QoS 2 Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrec_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBRECSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv3PUBRECUnexpected = Chart{
		ID:    "mqtt_pubrec_unexpected",
		Title: "v3 PUBREC QoS 2 Received Unexpected Messages",
		Units: "messages/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrec_invalid_error",
		Dims: Dims{
			{ID: metricPUBRECInvalid, Name: "unexpected", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBREL = Chart{
		ID:    "mqtt_pubrel",
		Title: "v3/v5 PUBREL QoS 2",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrel",
		Dims: Dims{
			{ID: metricPUBRELReceived, Name: "received", Algo: module.Incremental},
			{ID: metricPUBRELSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5PUBRELReceivedReason = Chart{
		ID:    "mqtt_pubrel_received_reason",
		Title: "v5 PUBREL QoS 2 Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrel_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBRELReceived, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv5PUBRELSentReason = Chart{
		ID:    "mqtt_pubrel_sent_reason",
		Title: "v5 PUBREL QoS 2 Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubrel_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBRELSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBCOMP = Chart{
		ID:    "mqtt_pubcomp",
		Title: "v3/v5 PUBCOMP QoS 2",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubcom",
		Dims: Dims{
			{ID: metricPUBCOMPReceived, Name: "received", Algo: module.Incremental},
			{ID: metricPUBCOMPSent, Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMQTTv5PUBCOMPReceivedReason = Chart{
		ID:    "mqtt_pubcomp_received_reason",
		Title: "v5 PUBCOMP QoS 2 Received by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubcomp_received_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBCOMPReceived, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv5PUBCOMPSentReason = Chart{
		ID:    "mqtt_pubcomp_sent_reason",
		Title: "v5 PUBCOMP QoS 2 Sent by Reason",
		Units: "packets/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubcomp_sent_reason",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: join(metricPUBCOMPSent, "success"), Name: "success", Algo: module.Incremental},
		},
	}
	chartMQTTv3v5PUBCOMPUnexpected = Chart{
		ID:    "mqtt_pubcomp_unexpected",
		Title: "v3/v5 PUBCOMP QoS 2 Received Unexpected Messages",
		Units: "messages/s",
		Fam:   "mqtt publish",
		Ctx:   "vernemq.mqtt_pubcomp_invalid_error",
		Dims: Dims{
			{ID: metricPUNCOMPInvalid, Name: "unexpected", Algo: module.Incremental},
		},
	}
)

// PING
var (
	chartMQTTv3v5PING = Chart{
		ID:    "mqtt_ping",
		Title: "v3/v5 PING",
		Units: "packets/s",
		Fam:   "mqtt ping",
		Ctx:   "vernemq.mqtt_ping",
		Dims: Dims{
			{ID: metricPINGREQReceived, Name: "PINGREQ", Algo: module.Incremental},
			{ID: metricPINGRESPSent, Name: "PINGRESP", Algo: module.Incremental, Mul: -1},
		},
	}
)

var (
	chartUptime = Chart{
		ID:    "node_uptime",
		Title: "Node Uptime",
		Units: "seconds",
		Fam:   "uptime",
		Ctx:   "vernemq.node_uptime",
		Dims: Dims{
			{ID: metricSystemWallClock, Name: "time", Div: 1000},
		},
	}
)

func (v *VerneMQ) notifyNewScheduler(name string) {
	if v.cache[name] {
		return
	}
	v.cache[name] = true

	id := chartSchedulerUtilization.ID
	num := name[len("system_utilization_scheduler_"):]

	v.addAbsDimToChart(id, name, num)
}

func (v *VerneMQ) notifyNewReason(name, reason string) {
	if reason == "success" || reason == "normal_disconnect" {
		return
	}
	key := join(name, reason)
	if v.cache[key] {
		return
	}
	v.cache[key] = true

	var chart Chart
	switch name {
	case metricAUTHReceived:
		chart = chartMQTTv5AUTHReceivedReason
	case metricAUTHSent:
		chart = chartMQTTv5AUTHSentReason
	case metricCONNACKSent:
		chart = chartMQTTv3v5CONNACKSentReason
	case metricDISCONNECTReceived:
		chart = chartMQTTv5DISCONNECTReceivedReason
	case metricDISCONNECTSent:
		chart = chartMQTTv5DISCONNECTSentReason
	case metricPUBACKReceived:
		chart = chartMQTTv5PUBACKReceivedReason
	case metricPUBACKSent:
		chart = chartMQTTv5PUBACKSentReason
	case metricPUBRECReceived:
		chart = chartMQTTv5PUBRECReceivedReason
	case metricPUBRECSent:
		chart = chartMQTTv5PUBRECSentReason
	case metricPUBRELReceived:
		chart = chartMQTTv5PUBRELReceivedReason
	case metricPUBRELSent:
		chart = chartMQTTv5PUBRELSentReason
	case metricPUBCOMPReceived:
		chart = chartMQTTv5PUBCOMPReceivedReason
	case metricPUBCOMPSent:
		chart = chartMQTTv5PUBCOMPSentReason
	default:
		v.Warningf("unknown metric name, wont be added to the charts: '%s'", name)
		return
	}

	v.addIncDimToChart(chart.ID, key, reason)
}

func (v *VerneMQ) addAbsDimToChart(chartID, dimID, dimName string) {
	v.addDimToChart(chartID, dimID, dimName, false)
}

func (v *VerneMQ) addIncDimToChart(chartID, dimID, dimName string) {
	v.addDimToChart(chartID, dimID, dimName, true)
}

func (v *VerneMQ) addDimToChart(chartID, dimID, dimName string, inc bool) {
	chart := v.Charts().Get(chartID)
	if chart == nil {
		v.Warningf("add '%s' dim: couldn't find '%s' chart", dimID, chartID)
		return
	}

	dim := &Dim{ID: dimID, Name: dimName}
	if inc {
		dim.Algo = module.Incremental
	}

	if err := chart.AddDim(dim); err != nil {
		v.Warningf("add '%s' dim: %v", dimID, err)
		return
	}
	chart.MarkNotCreated()
}
