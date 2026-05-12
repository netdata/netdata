// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#define PLUGIN_NETWORK_VIEWER_NAME   "network-viewer.plugin"
#define NV_WIN_FUNCTION_TCP          "tcp-stats"
#define NV_WIN_FUNCTION_TCP_HELP     "Windows TCP statistics by IP family (connections, segments, states)"
#define NV_WIN_FUNCTION_UDP          "udp-stats"
#define NV_WIN_FUNCTION_UDP_HELP     "Windows UDP statistics by IP family (datagrams)"
#define NV_WIN_FUNCTION_UPDATE_EVERY 5
#define NV_WIN_FUNCTION_PRIORITY     100

netdata_mutex_t stdout_mutex;
static bool plugin_should_exit = false;

// ============================================================
// TCP
// ============================================================

typedef struct {
    const char *af;
    const char *object_name;

    COUNTER_DATA connection_failures;
    COUNTER_DATA connections_active;
    COUNTER_DATA connections_established;
    COUNTER_DATA connections_passive;
    COUNTER_DATA connections_reset;
    COUNTER_DATA segments_total;
    COUNTER_DATA segments_received;
    COUNTER_DATA segments_retransmitted;
    COUNTER_DATA segments_sent;

    bool have_states;
    uint32_t state_counts[NETDATA_WIN_TCP_STATE_COUNT];
} TCP_FAMILY;

static TCP_FAMILY tcp_ipv4 = {
    .af = "IPv4",
    .object_name = "TCPv4",
};

static TCP_FAMILY tcp_ipv6 = {
    .af = "IPv6",
    .object_name = "TCPv6",
};

static void initialize_tcp_keys(TCP_FAMILY *tcp)
{
    tcp->connection_failures.key     = "Connection Failures";
    tcp->connections_active.key      = "Connections Active";
    tcp->connections_established.key = "Connections Established";
    tcp->connections_passive.key     = "Connections Passive";
    tcp->connections_reset.key       = "Connections Reset";
    tcp->segments_total.key          = "Segments/sec";
    tcp->segments_received.key       = "Segments Received/sec";
    tcp->segments_retransmitted.key  = "Segments Retransmitted/sec";
    tcp->segments_sent.key           = "Segments Sent/sec";
}

static void tcp_initialize(void)
{
    initialize_tcp_keys(&tcp_ipv4);
    initialize_tcp_keys(&tcp_ipv6);
}

static bool tcp_collect_family(TCP_FAMILY *tcp)
{
    DWORD id = RegistryFindIDByName(tcp->object_name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return false;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, tcp->object_name);
    if (!pObjectType)
        return false;

    bool have_any = false;
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connection_failures);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_active);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_established);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_passive);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->connections_reset);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_total);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_received);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_retransmitted);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &tcp->segments_sent);

    uint32_t af = (strcmp(tcp->af, "IPv4") == 0) ? AF_INET : AF_INET6;
    tcp->have_states = netdata_win_collect_tcp_state_counts(af, tcp->state_counts);

    return have_any;
}

static void tcp_emit_row(BUFFER *wb, const TCP_FAMILY *tcp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, tcp->af);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connection_failures.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_active.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_established.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_passive.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_reset.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_total.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_received.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_retransmitted.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_sent.current.Data);

        if (tcp->have_states) {
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_CLOSED]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_LISTENING]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_SYN_SENT]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_SYN_RECEIVED]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_ESTABLISHED]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT1]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_FIN_WAIT2]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_CLOSE_WAIT]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_CLOSING]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_LAST_ACK]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_TIME_WAIT]);
            buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->state_counts[NETDATA_WIN_TCP_STATE_DELETE_TCB]);
        } else {
            for (int i = 0; i < NETDATA_WIN_TCP_STATE_COUNT; i++)
                buffer_json_add_array_item_uint64(wb, 0);
        }
    }
    buffer_json_array_close(wb);
}

void function_tcp_stats(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        tcp_initialize();
        initialized = true;
    }

    tcp_collect_family(&tcp_ipv4);
    tcp_collect_family(&tcp_ipv6);

    time_t now_s = now_realtime_sec();
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NV_WIN_FUNCTION_TCP_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        tcp_emit_row(wb, &tcp_ipv4);
        tcp_emit_row(wb, &tcp_ipv6);
    }
    buffer_json_array_close(wb); // data

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        buffer_rrdf_table_add_field(wb, field_id++, "Protocol", "IP Protocol Family",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ConnFailures", "Connection Failures",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ConnActive", "Active Connections Opened",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ConnEstablished", "Currently Established Connections",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ConnPassive", "Passive Connections Opened",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ConnReset", "Reset Connections",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "SegsTotal", "Total Segments",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "segments", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "SegsReceived", "Received Segments",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "segments", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "SegsRetransmitted", "Retransmitted Segments",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "segments", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "SegsSent", "Sent Segments",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "segments", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateClosed", "Connections in CLOSED state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateListening", "Connections in LISTENING state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateSynSent", "Connections in SYN_SENT state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateSynReceived", "Connections in SYN_RECEIVED state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateEstablished", "Connections in ESTABLISHED state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateFinWait1", "Connections in FIN_WAIT1 state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateFinWait2", "Connections in FIN_WAIT2 state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateCloseWait", "Connections in CLOSE_WAIT state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateClosing", "Connections in CLOSING state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateLastAck", "Connections in LAST_ACK state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateTimeWait", "Connections in TIME_WAIT state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "StateDeleteTcb", "Connections in DELETE_TCB state",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "connections", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_time_t(wb, "expires", now_s + NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NV_WIN_FUNCTION_UPDATE_EVERY;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

// ============================================================
// UDP
// ============================================================

typedef struct {
    const char *af;
    const char *object_name;

    COUNTER_DATA datagrams_no_port;
    COUNTER_DATA datagrams_received_errors;
    COUNTER_DATA datagrams_received;
    COUNTER_DATA datagrams_sent;
} UDP_FAMILY;

static UDP_FAMILY udp_ipv4 = {
    .af = "IPv4",
    .object_name = "UDPv4",
};

static UDP_FAMILY udp_ipv6 = {
    .af = "IPv6",
    .object_name = "UDPv6",
};

static void initialize_udp_keys(UDP_FAMILY *udp)
{
    udp->datagrams_no_port.key         = "Datagrams No Port/sec";
    udp->datagrams_received_errors.key = "Datagrams Received Errors";
    udp->datagrams_received.key        = "Datagrams Received/sec";
    udp->datagrams_sent.key            = "Datagrams Sent/sec";
}

static void udp_initialize(void)
{
    initialize_udp_keys(&udp_ipv4);
    initialize_udp_keys(&udp_ipv6);
}

static bool udp_collect_family(UDP_FAMILY *udp)
{
    DWORD id = RegistryFindIDByName(udp->object_name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return false;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, udp->object_name);
    if (!pObjectType)
        return false;

    bool have_any = false;
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_no_port);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received_errors);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_sent);

    return have_any;
}

static void udp_emit_row(BUFFER *wb, const UDP_FAMILY *udp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, udp->af);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_no_port.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_received_errors.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_received.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_sent.current.Data);
    }
    buffer_json_array_close(wb);
}

void function_udp_stats(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        udp_initialize();
        initialized = true;
    }

    udp_collect_family(&udp_ipv4);
    udp_collect_family(&udp_ipv6);

    time_t now_s = now_realtime_sec();
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NV_WIN_FUNCTION_UDP_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        udp_emit_row(wb, &udp_ipv4);
        udp_emit_row(wb, &udp_ipv6);
    }
    buffer_json_array_close(wb); // data

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        buffer_rrdf_table_add_field(wb, field_id++, "Protocol", "IP Protocol Family",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DatagramsNoPort", "Datagrams with No Port",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "datagrams", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DatagramsErrors", "Datagrams Received with Errors",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "datagrams", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DatagramsReceived", "Received Datagrams",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "datagrams", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DatagramsSent", "Sent Datagrams",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "datagrams", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_time_t(wb, "expires", now_s + NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NV_WIN_FUNCTION_UPDATE_EVERY;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

// ============================================================
// main
// ============================================================

int main(int argc, char **argv)
{
    netdata_mutex_init(&stdout_mutex);
    nd_log_initialize_for_external_plugins("network-viewer.plugin");
    netdata_threads_init_for_external_plugins(0);

    int update_every = 1;
    if (argc >= 2) {
        update_every = atoi(argv[1]);
        if (update_every < 1)
            update_every = 1;
    }
    (void)update_every;

    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
            NV_WIN_FUNCTION_TCP, 60, NV_WIN_FUNCTION_TCP_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE),
            NV_WIN_FUNCTION_PRIORITY);
    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
            NV_WIN_FUNCTION_UDP, 60, NV_WIN_FUNCTION_UDP_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE),
            NV_WIN_FUNCTION_PRIORITY);
    fflush(stdout);

    struct functions_evloop_globals *wg =
        functions_evloop_init(5, "NV-WIN", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, NV_WIN_FUNCTION_TCP, function_tcp_stats,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);
    functions_evloop_add_function(wg, NV_WIN_FUNCTION_UDP, function_udp_stats,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    usec_t send_newline_ut = 0;
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        usec_t dt_ut = heartbeat_next(&hb);
        send_newline_ut += dt_ut;

        if (!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    return 0;
}
