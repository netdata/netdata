// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#define PLUGIN_NETWORK_VIEWER_NAME   "network-viewer.plugin"
#define NV_WIN_FUNCTION_PROTO        "network-protocols"
#define NV_WIN_FUNCTION_PROTO_HELP   "Windows TCP and UDP statistics by transport and IP family"
#define NV_WIN_FUNCTION_UPDATE_EVERY 5
#define NV_WIN_FUNCTION_PRIORITY     100

netdata_mutex_t stdout_mutex;
static bool plugin_should_exit = false;

// ============================================================
// Shared helpers
// ============================================================

// Resolve a perflib object by name; returns true and sets both out-pointers on success.
static bool perflib_get_object(const char *object_name,
                               PERF_DATA_BLOCK **pDataBlock_out,
                               PERF_OBJECT_TYPE **pObjectType_out)
{
    DWORD id = RegistryFindIDByName(object_name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return false;

    *pDataBlock_out = perflibGetPerformanceData(id);
    if (!*pDataBlock_out)
        return false;

    *pObjectType_out = perflibFindObjectTypeByName(*pDataBlock_out, object_name);
    return *pObjectType_out != NULL;
}

// Write the common JSON table response header fields into an already-created buffer.
static void nv_table_begin(BUFFER *wb, const char *help)
{
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", help);
}

// Finalize the JSON, then send it to pluginsd under the stdout mutex.
static void nv_send_result(const char *transaction, BUFFER *wb, time_t now_s)
{
    buffer_json_member_add_time_t(wb, "expires", now_s + NV_WIN_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);
    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NV_WIN_FUNCTION_UPDATE_EVERY;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

// Add a sticky string key column (Transport, Family, etc.).
static void nv_add_key_field(BUFFER *wb, size_t *field_id, const char *id, const char *label)
{
    buffer_rrdf_table_add_field(wb, (*field_id)++, id, label,
        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);
}

// Add a standard integer counter column (all counter columns share the same display/filter flags).
static void nv_add_int_field(BUFFER *wb, size_t *field_id,
                              const char *id, const char *label, const char *unit)
{
    buffer_rrdf_table_add_field(wb, (*field_id)++, id, label,
        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
        0, unit, NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
        RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL);
}

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
    PERF_DATA_BLOCK *pDataBlock;
    PERF_OBJECT_TYPE *pObjectType;
    if (!perflib_get_object(tcp->object_name, &pDataBlock, &pObjectType))
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

    return have_any;
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
    PERF_DATA_BLOCK *pDataBlock;
    PERF_OBJECT_TYPE *pObjectType;
    if (!perflib_get_object(udp->object_name, &pDataBlock, &pObjectType))
        return false;

    bool have_any = false;
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_no_port);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received_errors);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_sent);

    return have_any;
}

// ============================================================
// Network Protocols (combined TCP + UDP function)
// ============================================================

// Column order for all rows: Transport, Family, Received, Sent, Errors,
//   ConnActive, ConnEstablished, ConnPassive, ConnReset, SegsTotal, SegsRetransmitted,
//   DatagramsNoPort.

static void proto_emit_tcp_row(BUFFER *wb, const TCP_FAMILY *tcp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, "TCP");
        buffer_json_add_array_item_string(wb, tcp->af);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_received.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_sent.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connection_failures.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_active.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_established.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_passive.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->connections_reset.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_total.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)tcp->segments_retransmitted.current.Data);
        buffer_json_add_array_item_uint64(wb, 0); // DatagramsNoPort — UDP only
    }
    buffer_json_array_close(wb);
}

static void proto_emit_udp_row(BUFFER *wb, const UDP_FAMILY *udp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, "UDP");
        buffer_json_add_array_item_string(wb, udp->af);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_received.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_sent.current.Data);
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_received_errors.current.Data);
        buffer_json_add_array_item_uint64(wb, 0); // ConnActive        — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnEstablished   — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnPassive       — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnReset         — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // SegsTotal         — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // SegsRetransmitted — TCP only
        buffer_json_add_array_item_uint64(wb, (uint64_t)udp->datagrams_no_port.current.Data);
    }
    buffer_json_array_close(wb);
}

void function_network_protocols(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        tcp_initialize();
        udp_initialize();
        initialized = true;
    }

    tcp_collect_family(&tcp_ipv4);
    tcp_collect_family(&tcp_ipv6);
    udp_collect_family(&udp_ipv4);
    udp_collect_family(&udp_ipv6);

    time_t now_s = now_realtime_sec();
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    nv_table_begin(wb, NV_WIN_FUNCTION_PROTO_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        proto_emit_tcp_row(wb, &tcp_ipv4);
        proto_emit_tcp_row(wb, &tcp_ipv6);
        proto_emit_udp_row(wb, &udp_ipv4);
        proto_emit_udp_row(wb, &udp_ipv6);
    }
    buffer_json_array_close(wb); // data

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        nv_add_key_field(wb, &field_id, "Transport", "Transport Protocol");
        nv_add_key_field(wb, &field_id, "Family",    "IP Protocol Family");

        // Normalized columns — TCP: segments, UDP: datagrams
        nv_add_int_field(wb, &field_id, "Received", "Received (Segments/Datagrams)", "packets");
        nv_add_int_field(wb, &field_id, "Sent",     "Sent (Segments/Datagrams)",     "packets");
        nv_add_int_field(wb, &field_id, "Errors",   "Errors (Failures/Rx Errors)",   "errors");

        // TCP-only columns (UDP rows carry 0)
        nv_add_int_field(wb, &field_id, "ConnActive",        "Active Connections Opened",         "connections");
        nv_add_int_field(wb, &field_id, "ConnEstablished",   "Currently Established Connections",  "connections");
        nv_add_int_field(wb, &field_id, "ConnPassive",       "Passive Connections Opened",         "connections");
        nv_add_int_field(wb, &field_id, "ConnReset",         "Reset Connections",                  "connections");
        nv_add_int_field(wb, &field_id, "SegsTotal",         "Total Segments",                     "segments");
        nv_add_int_field(wb, &field_id, "SegsRetransmitted", "Retransmitted Segments",             "segments");

        // UDP-only column (TCP rows carry 0)
        nv_add_int_field(wb, &field_id, "DatagramsNoPort", "Datagrams with No Port", "datagrams");
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Received by Transport");
        {
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Transport");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Sent by Transport");
        {
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Transport");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Received by Family");
        {
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Family");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Sent by Family");
        {
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Family");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Received");
        buffer_json_add_array_item_string(wb, "Transport");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Sent");
        buffer_json_add_array_item_string(wb, "Transport");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb); // default_charts

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Transport");
        {
            buffer_json_member_add_string(wb, "name", "Transport");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Transport");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Family");
        {
            buffer_json_member_add_string(wb, "name", "Family");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Family");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    nv_send_result(transaction, wb, now_s);
}

// ============================================================
// main
// ============================================================

int main(int argc, char **argv)
{
    netdata_mutex_init(&stdout_mutex);
    nd_log_initialize_for_external_plugins("network-viewer.plugin");
    netdata_threads_init_for_external_plugins(0);

    PerflibNamesRegistryInitialize();

    int update_every = 1;
    if (argc >= 2) {
        update_every = atoi(argv[1]);
        if (update_every < 1)
            update_every = 1;
    }
    (void)update_every;

    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
            NV_WIN_FUNCTION_PROTO, 60, NV_WIN_FUNCTION_PROTO_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE),
            NV_WIN_FUNCTION_PRIORITY);
    fflush(stdout);

    struct functions_evloop_globals *wg =
        functions_evloop_init(5, "NV-WIN", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, NV_WIN_FUNCTION_PROTO, function_network_protocols,
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

        PerflibNamesRegistryUpdate();
    }

    PerflibNamesRegistryCleanup();

    return 0;
}
