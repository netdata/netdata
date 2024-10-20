// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

func newNodeStats() *nodeStats {
	return &nodeStats{
		stats: make(map[string]int64),
		mqtt4: make(map[string]int64),
		mqtt5: make(map[string]int64),
	}
}

type nodeStats struct {
	stats map[string]int64
	mqtt4 map[string]int64
	mqtt5 map[string]int64
}

// Source code metrics: https://github.com/vernemq/vernemq/blob/master/apps/vmq_server/src/vmq_metrics.erl
// Not used metrics: https://docs.vernemq.com/monitoring/introduction

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

var (
	mqtt5AUTHReceivedReasonCodes = []string{
		"success",
		"continue_authentication",
		"reauthenticate",
	}
	mqtt5AUTHSentReasonCodes = mqtt5AUTHReceivedReasonCodes

	mqtt4CONNACKSentReturnCodes = []string{
		"success",
		"unsupported_protocol_version",
		"client_identifier_not_valid",
		"server_unavailable",
		"bad_username_or_password",
		"not_authorized",
	}
	mqtt5CONNACKSentReasonCodes = []string{
		"success",
		"unspecified_error",
		"malformed_packet",
		"protocol_error",
		"impl_specific_error",
		"unsupported_protocol_version",
		"client_identifier_not_valid",
		"bad_username_or_password",
		"not_authorized",
		"server_unavailable",
		"server_busy",
		"banned",
		"bad_authentication_method",
		"topic_name_invalid",
		"packet_too_large",
		"quota_exceeded",
		"payload_format_invalid",
		"retain_not_supported",
		"qos_not_supported",
		"use_another_server",
		"server_moved",
		"connection_rate_exceeded",
	}

	mqtt5DISCONNECTReceivedReasonCodes = []string{
		"normal_disconnect",
		"disconnect_with_will_msg",
		"unspecified_error",
		"malformed_packet",
		"protocol_error",
		"impl_specific_error",
		"topic_name_invalid",
		"receive_max_exceeded",
		"topic_alias_invalid",
		"packet_too_large",
		"message_rate_too_high",
		"quota_exceeded",
		"administrative_action",
		"payload_format_invalid",
	}
	mqtt5DISCONNECTSentReasonCodes = []string{
		"normal_disconnect",
		"unspecified_error",
		"malformed_packet",
		"protocol_error",
		"impl_specific_error",
		"not_authorized",
		"server_busy",
		"server_shutting_down",
		"keep_alive_timeout",
		"session_taken_over",
		"topic_filter_invalid",
		"topic_name_invalid",
		"receive_max_exceeded",
		"topic_alias_invalid",
		"packet_too_large",
		"message_rate_too_high",
		"quota_exceeded",
		"administrative_action",
		"payload_format_invalid",
		"retain_not_supported",
		"qos_not_supported",
		"use_another_server",
		"server_moved",
		"shared_subs_not_supported",
		"connection_rate_exceeded",
		"max_connect_time",
		"subscription_ids_not_supported",
		"wildcard_subs_not_supported",
	}

	mqtt5PUBACKReceivedReasonCodes = []string{
		"success",
		"no_matching_subscribers",
		"unspecified_error",
		"impl_specific_error",
		"not_authorized",
		"topic_name_invalid",
		"packet_id_in_use",
		"quota_exceeded",
		"payload_format_invalid",
	}
	mqtt5PUBACKSentReasonCodes = mqtt5PUBACKReceivedReasonCodes

	mqtt5PUBRECReceivedReasonCodes = mqtt5PUBACKReceivedReasonCodes
	mqtt5PUBRECSentReasonCodes     = mqtt5PUBACKReceivedReasonCodes

	mqtt5PUBRELReceivedReasonCodes = []string{
		"success",
		"packet_id_not_found",
	}
	mqtt5PUBRELSentReasonCodes = mqtt5PUBRELReceivedReasonCodes

	mqtt5PUBCOMPReceivedReasonCodes = mqtt5PUBRELReceivedReasonCodes
	mqtt5PUBCOMPSentReasonCodes     = mqtt5PUBRELReceivedReasonCodes
)
