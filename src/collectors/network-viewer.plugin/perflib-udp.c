// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#define PLUGIN_NETWORK_VIEWER_NAME   "network-viewer.plugin"
#define NV_WIN_FUNCTION_UDP_HELP     "Windows UDP statistics by IP family (datagrams)"
#define NV_WIN_FUNCTION_UPDATE_EVERY 5

// Defined in perflib-tcp.c
extern netdata_mutex_t stdout_mutex;
extern bool plugin_should_exit;

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

static void initialize(void)
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
        initialize();
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
