// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

// Source Code Metrics:
//  - https://github.com/vernemq/vernemq/blob/master/apps/vmq_server/src/vmq_metrics.erl
//  - https://github.com/vernemq/vernemq/blob/master/apps/vmq_server/src/vmq_metrics.hrl

// Source Code FSM:
//  - https://github.com/vernemq/vernemq/blob/master/apps/vmq_server/src/vmq_mqtt_fsm.erl
//  - https://github.com/vernemq/vernemq/blob/master/apps/vmq_server/src/vmq_mqtt5_fsm.erl

// MQTT Packet Types:
//  - v4: http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/errata01/os/mqtt-v3.1.1-errata01-os-complete.html#_Toc442180834
//  - v5: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901019

// Erlang VM:
//  - http://erlang.org/documentation/doc-5.7.1/erts-5.7.1/doc/html/erlang.html

// Not used metrics (https://docs.vernemq.com/monitoring/introduction):
// - "mqtt_connack_accepted_sent"              // v4, not populated,  "mqtt_connack_sent" used instead
// - "mqtt_connack_unacceptable_protocol_sent" // v4, not populated,  "mqtt_connack_sent" used instead
// - "mqtt_connack_identifier_rejected_sent"   // v4, not populated,  "mqtt_connack_sent" used instead
// - "mqtt_connack_server_unavailable_sent"    // v4, not populated,  "mqtt_connack_sent" used instead
// - "mqtt_connack_bad_credentials_sent"       // v4, not populated,  "mqtt_connack_sent" used instead
// - "mqtt_connack_not_authorized_sent"        // v4, not populated,  "mqtt_connack_sent" used instead
// - "system_exact_reductions"
// - "system_runtime"
// - "vm_memory_atom"
// - "vm_memory_atom_used"
// - "vm_memory_binary"
// - "vm_memory_code"
// - "vm_memory_ets"
// - "vm_memory_processes_used"
// - "vm_memory_total"

// -----------------------------------------------MQTT------------------------------------------------------------------
const (
	// AUTH
	metricAUTHReceived = "mqtt_auth_received" // v5 has 'reason_code' label
	metricAUTHSent     = "mqtt_auth_sent"     // v5 has 'reason_code' label

	// CONNECT
	metricCONNECTReceived = "mqtt_connect_received" // v4, v5
	metricCONNACKSent     = "mqtt_connack_sent"     // v4 has 'return_code' label, v5 has 'reason_code'

	// SUBSCRIBE
	metricSUBSCRIBEReceived  = "mqtt_subscribe_received"   // v4, v5
	metricSUBACKSent         = "mqtt_suback_sent"          // v4, v5
	metricSUBSCRIBEError     = "mqtt_subscribe_error"      // v4, v5
	metricSUBSCRIBEAuthError = "mqtt_subscribe_auth_error" // v4, v5

	// UNSUBSCRIBE
	metricUNSUBSCRIBEReceived = "mqtt_unsubscribe_received" // v4, v5
	metricUNSUBACKSent        = "mqtt_unsuback_sent"        // v4, v5
	metricUNSUBSCRIBEError    = "mqtt_unsubscribe_error"    // v4, v5

	// PUBLISH
	metricPUBSLISHReceived = "mqtt_publish_received"   // v4, v5
	metricPUBSLIHSent      = "mqtt_publish_sent"       // v4, v5
	metricPUBLISHError     = "mqtt_publish_error"      // v4, v5
	metricPUBLISHAuthError = "mqtt_publish_auth_error" // v4, v5

	// Publish acknowledgment (QoS 1)
	metricPUBACKReceived = "mqtt_puback_received"      // v4, v5 has 'reason_code' label
	metricPUBACKSent     = "mqtt_puback_sent"          // v4, v5 has 'reason_code' label
	metricPUBACKInvalid  = "mqtt_puback_invalid_error" // v4, v5

	// Publish received (QoS 2 delivery part 1)
	metricPUBRECReceived = "mqtt_pubrec_received"      // v4, v5 has 'reason_code' label
	metricPUBRECSent     = "mqtt_pubrec_sent"          // v4, v5 has 'reason_code' label
	metricPUBRECInvalid  = "mqtt_pubrec_invalid_error" // v4

	// Publish release (QoS 2 delivery part 2)
	metricPUBRELReceived = "mqtt_pubrel_received" // v4, v5 has 'reason_code' label
	metricPUBRELSent     = "mqtt_pubrel_sent"     // v4, v5 has 'reason_code' label

	// Publish complete (QoS 2 delivery part 3)
	metricPUBCOMPReceived = "mqtt_pubcomp_received"      // v4, v5 has 'reason_code' label
	metricPUBCOMPSent     = "mqtt_pubcomp_sent"          // v4, v5 has 'reason_code' label
	metricPUNCOMPInvalid  = "mqtt_pubcomp_invalid_error" // v4, v5

	// PING
	metricPINGREQReceived = "mqtt_pingreq_received" // v4, v5
	metricPINGRESPSent    = "mqtt_pingresp_sent"    // v4, v5

	// DISCONNECT
	metricDISCONNECTReceived = "mqtt_disconnect_received" // v4, v5 has 'reason_code' label
	metricDISCONNECTSent     = "mqtt_disconnect_sent"     // v5 has 'reason_code' label

	// Misc
	metricMQTTInvalidMsgSizeError = "mqtt_invalid_msg_size_error" // v4, v5
)

const (
	// Sockets
	metricSocketOpen             = "socket_open"
	metricSocketClose            = "socket_close"
	metricSocketError            = "socket_error"
	metricSocketCloseTimeout     = "socket_close_timeout"
	metricClientKeepaliveExpired = "client_keepalive_expired" // v4, v5

	// Queues
	metricQueueProcesses              = "queue_processes"
	metricQueueSetup                  = "queue_setup"
	metricQueueTeardown               = "queue_teardown"
	metricQueueMessageIn              = "queue_message_in"
	metricQueueMessageOut             = "queue_message_out"
	metricQueueMessageDrop            = "queue_message_drop"
	metricQueueMessageExpired         = "queue_message_expired"
	metricQueueMessageUnhandled       = "queue_message_unhandled"
	metricQueueInitializedFromStorage = "queue_initialized_from_storage"

	// Subscriptions
	metricRouterMatchesLocal  = "router_matches_local"
	metricRouterMatchesRemote = "router_matches_remote"
	metricRouterMemory        = "router_memory"
	metricRouterSubscriptions = "router_subscriptions"

	// Erlang VM
	metricSystemUtilization        = "system_utilization"
	metricSystemProcessCount       = "system_process_count"
	metricSystemReductions         = "system_reductions"
	metricSystemContextSwitches    = "system_context_switches"
	metricSystemIOIn               = "system_io_in"
	metricSystemIOOut              = "system_io_out"
	metricSystemRunQueue           = "system_run_queue"
	metricSystemGCCount            = "system_gc_count"
	metricSystemWordsReclaimedByGC = "system_words_reclaimed_by_gc"
	metricVMMemoryProcesses        = "vm_memory_processes"
	metricVMMemorySystem           = "vm_memory_system"

	// Bandwidth
	metricBytesReceived = "bytes_received"
	metricBytesSent     = "bytes_sent"

	// Retain
	metricRetainMemory   = "retain_memory"
	metricRetainMessages = "retain_messages"

	// Cluster
	metricClusterBytesDropped  = "cluster_bytes_dropped"
	metricClusterBytesReceived = "cluster_bytes_received"
	metricClusterBytesSent     = "cluster_bytes_sent"
	metricNetSplitDetected     = "netsplit_detected"
	metricNetSplitResolved     = "netsplit_resolved"

	// Uptime
	metricSystemWallClock = "system_wallclock"
)
