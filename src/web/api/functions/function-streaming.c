// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-streaming.h"

#define GROUP_BY_COLUMN(name, descr) \
    buffer_json_member_add_object(wb, name);\
    {\
        buffer_json_member_add_string(wb, "name", descr);\
        buffer_json_member_add_array(wb, "columns");\
        {\
            buffer_json_add_array_item_string(wb, name);\
        }\
        buffer_json_array_close(wb);\
    }\
    buffer_json_object_close(wb);


int function_streaming(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {

    time_t now = now_realtime_sec();

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_STREAMING_HELP);
    buffer_json_member_add_array(wb, "data");

    size_t max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX] = { 0 };
    size_t max_db_metrics = 0, max_db_instances = 0, max_db_contexts = 0;
    size_t max_collection_replication_instances = 0, max_streaming_replication_instances = 0;
    size_t max_ml_anomalous = 0, max_ml_normal = 0, max_ml_trained = 0, max_ml_pending = 0, max_ml_silenced = 0;

    time_t
        max_db_duration = 0,
        max_db_from = 0,
        max_db_to = 0,
        max_in_age = 0,
        max_out_age = 0,
        max_out_attempt_age = 0;

    uint64_t
        max_in_since = 0,
        max_out_since = 0,
        max_out_attempt_since = 0;

    int16_t
        max_in_hops = -1,
        max_out_hops = -1;

    int
        max_in_local_port = 0,
        max_in_remote_port = 0,
        max_out_local_port = 0,
        max_out_remote_port = 0;

    uint32_t
        max_in_connections = 0,
        max_out_connections = 0;

    {
        RRDHOST *host;
        dfe_start_read(rrdhost_root_index, host) {
            RRDHOST_STATUS s;
            rrdhost_status(host, now, &s, RRDHOST_STATUS_ALL);
            buffer_json_add_array_item_array(wb);

            if(s.db.metrics > max_db_metrics)
                max_db_metrics = s.db.metrics;

            if(s.db.instances > max_db_instances)
                max_db_instances = s.db.instances;

            if(s.db.contexts > max_db_contexts)
                max_db_contexts = s.db.contexts;

            if(s.ingest.replication.instances > max_collection_replication_instances)
                max_collection_replication_instances = s.ingest.replication.instances;

            if(s.stream.replication.instances > max_streaming_replication_instances)
                max_streaming_replication_instances = s.stream.replication.instances;

            for(int i = 0; i < STREAM_TRAFFIC_TYPE_MAX ;i++) {
                if (s.stream.sent_bytes_on_this_connection_per_type[i] >
                    max_sent_bytes_on_this_connection_per_type[i])
                    max_sent_bytes_on_this_connection_per_type[i] =
                        s.stream.sent_bytes_on_this_connection_per_type[i];
            }

            // Node
            buffer_json_add_array_item_string(wb, rrdhost_hostname(s.host));

            // rowOptions
            buffer_json_add_array_item_object(wb);
            {
                const char *severity = NULL; // normal, debug, notice, warning, critical
                if(!rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST)) {
                    switch(s.ingest.status) {
                        case RRDHOST_INGEST_STATUS_OFFLINE:
                        case RRDHOST_INGEST_STATUS_ARCHIVED:
                            severity = "critical";
                            break;

                        default:
                        case RRDHOST_INGEST_STATUS_INITIALIZING:
                        case RRDHOST_INGEST_STATUS_ONLINE:
                        case RRDHOST_INGEST_STATUS_REPLICATING:
                            break;
                    }

                    switch(s.stream.status) {
                        case RRDHOST_STREAM_STATUS_OFFLINE:
                            if(!severity && s.stream.reason != STREAM_HANDSHAKE_SP_NO_DESTINATION)
                                severity = "warning";
                            break;

                        default:
                        case RRDHOST_STREAM_STATUS_REPLICATING:
                        case RRDHOST_STREAM_STATUS_ONLINE:
                            break;
                    }
                }
                buffer_json_member_add_string(wb, "severity", severity ? severity : "normal");
            }
            buffer_json_object_close(wb); // rowOptions

            // Ephemerality
            buffer_json_add_array_item_string(wb, rrdhost_option_check(s.host, RRDHOST_OPTION_EPHEMERAL_HOST) ? "ephemeral" : "permanent");

            // AgentName and AgentVersion
            buffer_json_add_array_item_string(wb, rrdhost_program_name(host));
            buffer_json_add_array_item_string(wb, rrdhost_program_version(host));

            // System Info
            rrdhost_system_info_to_streaming_function_array(wb, s.host->system_info);

            // retention
            buffer_json_add_array_item_uint64(wb, s.db.first_time_s * MSEC_PER_SEC); // dbFrom
            if(s.db.first_time_s > max_db_from) max_db_from = s.db.first_time_s;

            buffer_json_add_array_item_uint64(wb, s.db.last_time_s * MSEC_PER_SEC); // dbTo
            if(s.db.last_time_s > max_db_to) max_db_to = s.db.last_time_s;

            if(s.db.first_time_s && s.db.last_time_s && s.db.last_time_s > s.db.first_time_s) {
                time_t db_duration = s.db.last_time_s - s.db.first_time_s;
                buffer_json_add_array_item_uint64(wb, db_duration); // dbDuration
                if(db_duration > max_db_duration) max_db_duration = db_duration;
            }
            else
                buffer_json_add_array_item_string(wb, NULL); // dbDuration

            buffer_json_add_array_item_uint64(wb, s.db.metrics); // dbMetrics
            buffer_json_add_array_item_uint64(wb, s.db.instances); // dbInstances
            buffer_json_add_array_item_uint64(wb, s.db.contexts); // dbContexts

            // statuses
            buffer_json_add_array_item_string(wb, rrdhost_ingest_status_to_string(s.ingest.status)); // InStatus
            buffer_json_add_array_item_string(wb, rrdhost_streaming_status_to_string(s.stream.status)); // OutStatus
            buffer_json_add_array_item_string(wb, rrdhost_ml_status_to_string(s.ml.status)); // MLStatus

            // collection

            // InConnections
            buffer_json_add_array_item_uint64(wb, s.host->stream.rcv.status.connections);
            if(s.host->stream.rcv.status.connections > max_in_connections)
                max_in_connections = s.host->stream.rcv.status.connections;

            if(s.ingest.since) {
                uint64_t in_since = s.ingest.since * MSEC_PER_SEC;
                buffer_json_add_array_item_uint64(wb, in_since); // InSince
                if(in_since > max_in_since) max_in_since = in_since;

                time_t in_age = s.now - s.ingest.since;
                buffer_json_add_array_item_time_t(wb, in_age); // InAge
                if(in_age > max_in_age) max_in_age = in_age;
            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // InSince
                buffer_json_add_array_item_string(wb, NULL); // InAge
            }

            // InReason
            if(s.ingest.type == RRDHOST_INGEST_TYPE_LOCALHOST)
                buffer_json_add_array_item_string(wb, "LOCALHOST");
            else if(s.ingest.type == RRDHOST_INGEST_TYPE_VIRTUAL)
                buffer_json_add_array_item_string(wb, "VIRTUAL NODE");
            else
                buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.ingest.reason));

            buffer_json_add_array_item_int64(wb, s.ingest.hops); // InHops
            if(s.ingest.hops > max_in_hops) max_in_hops = s.ingest.hops;

            buffer_json_add_array_item_double(wb, s.ingest.replication.completion); // InReplCompletion
            buffer_json_add_array_item_uint64(wb, s.ingest.replication.instances); // InReplInstances
            buffer_json_add_array_item_string(wb, s.ingest.type == RRDHOST_INGEST_TYPE_LOCALHOST || s.ingest.type == RRDHOST_INGEST_TYPE_VIRTUAL ? "localhost" : s.ingest.peers.local.ip); // InLocalIP

            buffer_json_add_array_item_uint64(wb, s.ingest.peers.local.port); // InLocalPort
            if(s.ingest.peers.local.port > max_in_local_port) max_in_local_port = s.ingest.peers.local.port;

            buffer_json_add_array_item_string(wb, s.ingest.peers.peer.ip); // InRemoteIP
            buffer_json_add_array_item_uint64(wb, s.ingest.peers.peer.port); // InRemotePort
            if(s.ingest.peers.peer.port > max_in_remote_port) max_in_remote_port = s.ingest.peers.peer.port;

            buffer_json_add_array_item_string(wb, s.ingest.ssl ? "SSL" : "PLAIN"); // InSSL
            stream_capabilities_to_json_array(wb, s.ingest.capabilities, NULL); // InCapabilities

            buffer_json_add_array_item_uint64(wb, s.ingest.collected.metrics); // CollectedMetrics
            buffer_json_add_array_item_uint64(wb, s.ingest.collected.instances); // CollectedInstances
            buffer_json_add_array_item_uint64(wb, s.ingest.collected.contexts); // CollectedContexts

            // streaming

            // OutConnections
            buffer_json_add_array_item_uint64(wb, s.host->stream.snd.status.connections);
            if(s.host->stream.snd.status.connections > max_out_connections)
                max_out_connections = s.host->stream.snd.status.connections;

            if(s.stream.since) {
                uint64_t out_since = s.stream.since * MSEC_PER_SEC;
                buffer_json_add_array_item_uint64(wb, out_since); // OutSince
                if(out_since > max_out_since) max_out_since = out_since;

                time_t out_age = s.now - s.stream.since;
                buffer_json_add_array_item_time_t(wb, out_age); // OutAge
                if(out_age > max_out_age) max_out_age = out_age;
            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // OutSince
                buffer_json_add_array_item_string(wb, NULL); // OutAge
            }
            buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.stream.reason)); // OutReason

            buffer_json_add_array_item_int64(wb, s.stream.hops); // OutHops
            if(s.stream.hops > max_out_hops) max_out_hops = s.stream.hops;

            buffer_json_add_array_item_double(wb, s.stream.replication.completion); // OutReplCompletion
            buffer_json_add_array_item_uint64(wb, s.stream.replication.instances); // OutReplInstances
            buffer_json_add_array_item_string(wb, s.stream.peers.local.ip); // OutLocalIP
            buffer_json_add_array_item_uint64(wb, s.stream.peers.local.port); // OutLocalPort
            if(s.stream.peers.local.port > max_out_local_port) max_out_local_port = s.stream.peers.local.port;

            buffer_json_add_array_item_string(wb, s.stream.peers.peer.ip); // OutRemoteIP
            buffer_json_add_array_item_uint64(wb, s.stream.peers.peer.port); // OutRemotePort
            if(s.stream.peers.peer.port > max_out_remote_port) max_out_remote_port = s.stream.peers.peer.port;

            buffer_json_add_array_item_string(wb, s.stream.ssl ? "SSL" : "PLAIN"); // OutSSL
            buffer_json_add_array_item_string(wb, s.stream.compression ? "COMPRESSED" : "UNCOMPRESSED"); // OutCompression
            stream_capabilities_to_json_array(wb, s.stream.capabilities, NULL); // OutCapabilities
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);

            buffer_json_add_array_item_array(wb); // OutAttemptHandshake
            usec_t last_attempt = stream_parent_handshake_error_to_json(wb, host);
            buffer_json_array_close(wb); // // OutAttemptHandshake

            if(!last_attempt) {
                buffer_json_add_array_item_string(wb, NULL); // OutAttemptSince
                buffer_json_add_array_item_string(wb, NULL); // OutAttemptAge
            }
            else {
                uint64_t out_attempt_since = last_attempt / USEC_PER_MS;
                buffer_json_add_array_item_uint64(wb, out_attempt_since); // OutAttemptSince
                if(out_attempt_since > max_out_attempt_since) max_out_attempt_since = out_attempt_since;

                time_t out_attempt_age = s.now - (time_t)(last_attempt / USEC_PER_SEC);
                buffer_json_add_array_item_time_t(wb, out_attempt_age); // OutAttemptAge
                if(out_attempt_age > max_out_attempt_age) max_out_attempt_age = out_attempt_age;
            }

            // ML
            if(s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.anomalous); // MlAnomalous
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.normal); // MlNormal
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.trained); // MlTrained
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.pending); // MlPending
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.silenced); // MlSilenced

                if(s.ml.metrics.anomalous > max_ml_anomalous)
                    max_ml_anomalous = s.ml.metrics.anomalous;

                if(s.ml.metrics.normal > max_ml_normal)
                    max_ml_normal = s.ml.metrics.normal;

                if(s.ml.metrics.trained > max_ml_trained)
                    max_ml_trained = s.ml.metrics.trained;

                if(s.ml.metrics.pending > max_ml_pending)
                    max_ml_pending = s.ml.metrics.pending;

                if(s.ml.metrics.silenced > max_ml_silenced)
                    max_ml_silenced = s.ml.metrics.silenced;

            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // MlAnomalous
                buffer_json_add_array_item_string(wb, NULL); // MlNormal
                buffer_json_add_array_item_string(wb, NULL); // MlTrained
                buffer_json_add_array_item_string(wb, NULL); // MlPending
                buffer_json_add_array_item_string(wb, NULL); // MlSilenced
            }

            // close
            buffer_json_array_close(wb);
        }
        dfe_done(host);
    }
    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Node", "Node's Hostname",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "rowOptions", "rowOptions",
                                    RRDF_FIELD_TYPE_NONE, RRDR_FIELD_VISUAL_ROW_OPTIONS, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_FIXED, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_DUMMY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Ephemerality", "The type of ephemerality for the node",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "AgentName", "The name of the Netdata agent",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "AgentVersion", "The version of the Netdata agent",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSName", "The name of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSId", "The identifier of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSIdLike", "The ID-like string for the host's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSVersion", "The version of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSVersionId", "The version identifier of the host's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSDetection", "Details about host OS detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CPUCores", "The number of CPU cores in the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DiskSpace", "The total disk space available on the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CPUFreq", "The CPU frequency of the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "RAMTotal", "The total RAM available on the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSName", "The name of the container's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSId", "The identifier of the container's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSIdLike", "The ID-like string for the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSVersion", "The version of the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSVersionId", "The version identifier of the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSDetection", "Details about container OS detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "IsK8sNode", "Whether this node is part of a Kubernetes cluster",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "KernelName", "The kernel name",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "KernelVersion", "The kernel version",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Architecture", "The system architecture",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Virtualization", "The virtualization technology in use",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "VirtDetection", "Details about virtualization detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Container", "Container type information",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerDetection", "Details about container detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudProviderType", "The type of cloud provider",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudInstanceType", "The type of cloud instance",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudInstanceRegion", "The region of the cloud instance",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbFrom", "DB Data Retention From",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_db_from * MSEC_PER_SEC, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbTo", "DB Data Retention To",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_db_to * MSEC_PER_SEC, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbDuration", "DB Data Retention Duration",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_db_duration, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbMetrics", "Time-series Metrics in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbInstances", "Instances in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbContexts", "Contexts in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_contexts, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- statuses ---

        buffer_rrdf_table_add_field(wb, field_id++, "InStatus", "Data Collection Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);


        buffer_rrdf_table_add_field(wb, field_id++, "OutStatus", "Streaming Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlStatus", "ML Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- collection ---

        buffer_rrdf_table_add_field(wb, field_id++, "InConnections", "Number of times this child connected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_in_connections, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InSince", "Last Data Collection Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_in_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InAge", "Last Data Collection Online Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_in_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReason", "Data Collection Online Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InHops", "Data Collection Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_in_hops, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplCompletion", "Inbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplInstances", "Inbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max_collection_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalIP", "Inbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalPort", "Inbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_in_local_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemoteIP", "Inbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemotePort", "Inbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_in_remote_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InSSL", "Inbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InCapabilities", "Inbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedMetrics", "Time-series Metrics Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedInstances", "Instances Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedContexts", "Contexts Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_contexts, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- streaming ---

        buffer_rrdf_table_add_field(wb, field_id++, "OutConnections", "Number of times connected to a parent",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_out_connections, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutSince", "Last Streaming Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_out_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAge", "Last Streaming Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_out_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReason", "Streaming Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutHops", "Streaming Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_out_hops, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplCompletion", "Outbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplInstances", "Outbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max_streaming_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalIP", "Outbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalPort", "Outbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemoteIP", "Outbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemotePort", "Outbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_out_remote_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutSSL", "Outbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCompression", "Outbound Compressed Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCapabilities", "Outbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficData", "Outbound Metric Data Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes", (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficMetadata", "Outbound Metric Metadata Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficReplication", "Outbound Metric Replication Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficFunctions", "Outbound Metric Functions Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptHandshake",
                                    "Outbound Connection Attempt Handshake Status",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptSince",
                                    "Last Outbound Connection Attempt Status Change Time",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_out_attempt_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptAge",
                                    "Last Outbound Connection Attempt Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_out_attempt_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- ML ---

        buffer_rrdf_table_add_field(wb, field_id++, "MlAnomalous", "Number of Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_anomalous,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlNormal", "Number of Not Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_normal,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlTrained", "Number of Trained Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_trained,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlPending", "Number of Pending Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_pending,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlSilenced", "Number of Silenced Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_silenced,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Node");
    buffer_json_member_add_object(wb, "charts");
    {
        // Data Collection Age chart
        buffer_json_member_add_object(wb, "InAge");
        {
            buffer_json_member_add_string(wb, "name", "Data Collection Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "InAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Streaming Age chart
        buffer_json_member_add_object(wb, "OutAge");
        {
            buffer_json_member_add_string(wb, "name", "Streaming Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "OutAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // DB Duration
        buffer_json_member_add_object(wb, "dbDuration");
        {
            buffer_json_member_add_string(wb, "name", "Retention Duration");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "dbDuration");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "InAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "OutAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        GROUP_BY_COLUMN("OSName", "O/S Name");
        GROUP_BY_COLUMN("OSId", "O/S ID");
        GROUP_BY_COLUMN("OSIdLike", "O/S ID Like");
        GROUP_BY_COLUMN("OSVersion", "O/S Version");
        GROUP_BY_COLUMN("OSVersionId", "O/S Version ID");
        GROUP_BY_COLUMN("OSDetection", "O/S Detection");
        GROUP_BY_COLUMN("CPUCores", "CPU Cores");
        GROUP_BY_COLUMN("ContainerOSName", "Container O/S Name");
        GROUP_BY_COLUMN("ContainerOSId", "Container O/S ID");
        GROUP_BY_COLUMN("ContainerOSIdLike", "Container O/S ID Like");
        GROUP_BY_COLUMN("ContainerOSVersion", "Container O/S Version");
        GROUP_BY_COLUMN("ContainerOSVersionId", "Container O/S Version ID");
        GROUP_BY_COLUMN("ContainerOSDetection", "Container O/S Detection");
        GROUP_BY_COLUMN("IsK8sNode", "Kubernetes Nodes");
        GROUP_BY_COLUMN("KernelName", "Kernel Name");
        GROUP_BY_COLUMN("KernelVersion", "Kernel Version");
        GROUP_BY_COLUMN("Architecture", "Architecture");
        GROUP_BY_COLUMN("Virtualization", "Virtualization Technology");
        GROUP_BY_COLUMN("VirtDetection", "Virtualization Detection");
        GROUP_BY_COLUMN("Container", "Container");
        GROUP_BY_COLUMN("ContainerDetection", "Container Detection");
        GROUP_BY_COLUMN("CloudProviderType", "Cloud Provider Type");
        GROUP_BY_COLUMN("CloudInstanceType", "Cloud Instance Type");
        GROUP_BY_COLUMN("CloudInstanceRegion", "Cloud Instance Region");

        GROUP_BY_COLUMN("InStatus", "Collection Status");
        GROUP_BY_COLUMN("OutStatus", "Streaming Status");
        GROUP_BY_COLUMN("MlStatus", "ML Status");
        GROUP_BY_COLUMN("InRemoteIP", "Inbound IP");
        GROUP_BY_COLUMN("OutRemoteIP", "Outbound IP");
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}
