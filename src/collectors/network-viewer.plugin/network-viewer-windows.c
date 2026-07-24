// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/os/windows-perflib/perflib.h"
#include "libnetdata/os/system-maps/system-services.h"
#include "libnetdata/os/system-maps/cached-sid-username.h"

// Minimal IP Helper API forward declarations.
// <winsock2.h> and <ws2tcpip.h> cannot be included here: libnetdata.h already
// pulls in POSIX socket headers (via uv.h), and the Windows headers redefine
// hostent, sockaddr, pollfd etc. causing compile errors on Cygwin/MSYS2.
// Base types (DWORD, ULONG, UCHAR, BOOL, PVOID, PDWORD) come from <windows.h>
// which is included for OS_WINDOWS by libnetdata/common.h.
// inet_ntop / struct in_addr / AF_INET* / INET6_ADDRSTRLEN come from the
// POSIX headers already included by libnetdata.h.

#define MIB_TCP_STATE_CLOSED     1
#define MIB_TCP_STATE_LISTEN     2
#define MIB_TCP_STATE_SYN_SENT   3
#define MIB_TCP_STATE_SYN_RCVD   4
#define MIB_TCP_STATE_ESTAB      5
#define MIB_TCP_STATE_FIN_WAIT1  6
#define MIB_TCP_STATE_FIN_WAIT2  7
#define MIB_TCP_STATE_CLOSE_WAIT 8
#define MIB_TCP_STATE_CLOSING    9
#define MIB_TCP_STATE_LAST_ACK  10
#define MIB_TCP_STATE_TIME_WAIT 11
#define MIB_TCP_STATE_DELETE_TCB 12

typedef enum { TCP_TABLE_OWNER_PID_ALL = 5 } TCP_TABLE_CLASS;
typedef enum { UDP_TABLE_OWNER_PID = 1 }    UDP_TABLE_CLASS;

typedef struct {
    DWORD dwState;
    DWORD dwLocalAddr;
    DWORD dwLocalPort;
    DWORD dwRemoteAddr;
    DWORD dwRemotePort;
    DWORD dwOwningPid;
} MIB_TCPROW_OWNER_PID;

typedef struct {
    UCHAR ucLocalAddr[16];
    DWORD dwLocalScopeId;
    DWORD dwLocalPort;
    UCHAR ucRemoteAddr[16];
    DWORD dwRemoteScopeId;
    DWORD dwRemotePort;
    DWORD dwState;
    DWORD dwOwningPid;
} MIB_TCP6ROW_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_TCPROW_OWNER_PID table[];
} MIB_TCPTABLE_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_TCP6ROW_OWNER_PID table[];
} MIB_TCP6TABLE_OWNER_PID;

typedef struct {
    DWORD dwLocalAddr;
    DWORD dwLocalPort;
    DWORD dwOwningPid;
} MIB_UDPROW_OWNER_PID;

typedef struct {
    UCHAR ucLocalAddr[16];
    DWORD dwLocalScopeId;
    DWORD dwLocalPort;
    DWORD dwOwningPid;
} MIB_UDP6ROW_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_UDPROW_OWNER_PID table[];
} MIB_UDPTABLE_OWNER_PID;

typedef struct {
    DWORD dwNumEntries;
    MIB_UDP6ROW_OWNER_PID table[];
} MIB_UDP6TABLE_OWNER_PID;

DWORD WINAPI GetExtendedTcpTable(PVOID pTcpTable, PDWORD pdwSize, BOOL bOrder,
                                 ULONG ulAf, TCP_TABLE_CLASS TableClass, ULONG Reserved);
DWORD WINAPI GetExtendedUdpTable(PVOID pUdpTable, PDWORD pdwSize, BOOL bOrder,
                                 ULONG ulAf, UDP_TABLE_CLASS TableClass, ULONG Reserved);

// Windows-native AF_ values for IP Helper API calls.
// Cygwin POSIX headers define AF_INET6=10; Windows APIs expect 23.
// AF_INET=2 happens to be the same on both.
#define NV_WIN_AF_INET  2
#define NV_WIN_AF_INET6 23

#define PLUGIN_NETWORK_VIEWER_NAME   "network-viewer.plugin"
#define NV_WIN_FUNCTION_PROTO        "network-protocols"
#define NV_WIN_FUNCTION_PROTO_HELP   "Windows TCP, UDP, and SMB Server Shares statistics grouped by transport and IP family"
#define NV_WIN_FUNCTION_UPDATE_EVERY 5
#define NV_WIN_FUNCTION_PRIORITY     100
#define NV_WIN_FUNCTION_CONN         "network-connections"
#define NV_WIN_FUNCTION_CONN_HELP    "Shows active network connections with protocol details, states, addresses, ports, and process information."

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

// Serialize pluginsd JSON errors the same way as success responses.
static void nv_send_error(const char *transaction, int code, const char *message)
{
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_json_error_to_stdout(transaction, code, message);
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

// Add a two-column stacked-bar chart entry to the charts object.
static void nv_add_stacked_bar_chart(BUFFER *wb, const char *key,
                                     const char *col1, const char *col2)
{
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_string(wb, "name", key);
        buffer_json_member_add_string(wb, "type", "stacked-bar");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, col1);
            buffer_json_add_array_item_string(wb, col2);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

// Add one entry to the default_charts array: [chart_key, groupby_column].
static void nv_add_default_chart(BUFFER *wb, const char *chart_key, const char *groupby)
{
    buffer_json_add_array_item_array(wb);
    buffer_json_add_array_item_string(wb, chart_key);
    buffer_json_add_array_item_string(wb, groupby);
    buffer_json_array_close(wb);
}

// Add one entry to the group_by object: a single-column grouping keyed by name.
static void nv_add_group_by(BUFFER *wb, const char *name)
{
    buffer_json_member_add_object(wb, name);
    {
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_array(wb, "columns");
        buffer_json_add_array_item_string(wb, name);
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

// Helpers for network-connections column definitions (all use SORT_ASCENDING, SUMMARY_COUNT).
static void nv_conn_str_field(BUFFER *wb, size_t *field_id,
                               const char *id, const char *label,
                               RRDF_FIELD_FILTER filter, RRDF_FIELD_OPTIONS opts)
{
    buffer_rrdf_table_add_field(wb, (*field_id)++, id, label,
        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
        RRDF_FIELD_SUMMARY_COUNT, filter, opts, NULL);
}

static void nv_conn_int_field(BUFFER *wb, size_t *field_id,
                               const char *id, const char *label,
                               RRDF_FIELD_FILTER filter, RRDF_FIELD_OPTIONS opts)
{
    buffer_rrdf_table_add_field(wb, (*field_id)++, id, label,
        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
        RRDF_FIELD_SUMMARY_COUNT, filter, opts, NULL);
}

// Convert Perflib rate counters to per-second values and keep raw gauges unchanged.
static uint64_t nv_perflib_value(const COUNTER_DATA *cd)
{
    if(unlikely(!cd->updated))
        return 0;

    switch(cd->current.CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT: {
            if(unlikely(!cd->previous.Time || !cd->current.Frequency))
                return 0;

            ULONGLONG data1 = cd->current.Data;
            ULONGLONG data0 = cd->previous.Data;
            LONGLONG time1 = cd->current.Time;
            LONGLONG time0 = cd->previous.Time;
            LONGLONG dt = time1 - time0;

            if(unlikely(dt <= 0 || data1 < data0))
                return 0;

            return (uint64_t)(((double)(data1 - data0) * (double)cd->current.Frequency) / (double)dt);
        }

        default:
            return (uint64_t)cd->current.Data;
    }
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

static netdata_mutex_t nv_collect_mutex;
static SERVICENAMES_CACHE *sc = NULL;

static void initialize_udp_keys(UDP_FAMILY *udp)
{
    udp->datagrams_no_port.key         = "Datagrams No Port/sec";
    udp->datagrams_received_errors.key = "Datagrams Received Errors";
    udp->datagrams_received.key        = "Datagrams Received/sec";
    udp->datagrams_sent.key            = "Datagrams Sent/sec";
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
// SMB Server Shares — per-share traffic and connection metrics
// ============================================================

typedef struct {
    usec_t       last_collected;
    COUNTER_DATA receivedBytes;
    COUNTER_DATA sentBytes;
    COUNTER_DATA treeConnectCount;
} NV_SMB_SHARE;

static DICTIONARY *nv_smb_shares = NULL;

static void nv_smb_share_insert_cb(const DICTIONARY_ITEM *item __maybe_unused,
                                   void *value, void *data __maybe_unused)
{
    NV_SMB_SHARE *s = value;
    s->receivedBytes.key    = "Received Bytes/sec";
    s->sentBytes.key        = "Sent Bytes/sec";
    s->treeConnectCount.key = "Tree Connect Count";
}

// Enumerate "SMB Server Shares" instances and update per-share COUNTER_DATA.
// Must be called with nv_collect_mutex held when worker threads may run concurrently.
// Returns true if at least one share was collected in this pass.
static bool nv_smb_collect(void)
{
    PERF_DATA_BLOCK  *pDataBlock;
    PERF_OBJECT_TYPE *pObjectType;
    if (!perflib_get_object("SMB Server Shares", &pDataBlock, &pObjectType))
        return false;

    char name[1024];
    usec_t now_ut = now_monotonic_usec();
    LONG collected = 0;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, name, sizeof(name)))
            strncpyz(name, "[unknown]", sizeof(name) - 1);

        NV_SMB_SHARE *s = dictionary_set(nv_smb_shares, name, NULL, sizeof(*s));
        // Mark the share as seen in this pass regardless of counter success.
        // GC only removes entries absent from the Perflib instance list; transient
        // counter failures should not destroy the entry or its rate-calculation state.
        s->last_collected = now_ut;

        bool share_ok = false;
        share_ok |= perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &s->receivedBytes);
        share_ok |= perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &s->sentBytes);
        share_ok |= perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &s->treeConnectCount);
        if (share_ok)
            collected++;
    }

    // Remove shares that were not seen in this collection pass.
    {
        NV_SMB_SHARE *s;
        dfe_start_write(nv_smb_shares, s)
        {
            if (s->last_collected < now_ut)
                dictionary_del(nv_smb_shares, s_dfe.name);
        }
        dfe_done(s);
        dictionary_garbage_collect(nv_smb_shares);
    }

    return collected > 0;
}

// Emit one network-protocols table row per SMB share.
// Must be called under nv_collect_mutex, after nv_smb_collect().
// Column order matches function_network_protocols:
//   Transport, Family, Share, Received, Sent, Errors,
//   ConnActive, ConnEstablished, ConnPassive, ConnReset,
//   SegsTotal, SegsRetransmitted, DatagramsNoPort, TreeConnects,
//   ReceivedBytes, SentBytes.
static void proto_emit_smb_rows(BUFFER *wb)
{
    NV_SMB_SHARE *s;
    dfe_start_read(nv_smb_shares, s)
    {
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, "SMB");
            buffer_json_add_array_item_string(wb, "");      // Family — not applicable
            buffer_json_add_array_item_string(wb, s_dfe.name); // Share name
            buffer_json_add_array_item_uint64(wb, 0); // Received          — TCP/UDP only
            buffer_json_add_array_item_uint64(wb, 0); // Sent              — TCP/UDP only
            buffer_json_add_array_item_uint64(wb, 0); // Errors
            buffer_json_add_array_item_uint64(wb, 0); // ConnActive        — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnEstablished   — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnPassive       — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnReset         — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // SegsTotal         — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // SegsRetransmitted — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // DatagramsNoPort   — UDP only
            buffer_json_add_array_item_uint64(wb, nv_perflib_value(&s->treeConnectCount));
            buffer_json_add_array_item_uint64(wb, nv_perflib_value(&s->receivedBytes));
            buffer_json_add_array_item_uint64(wb, nv_perflib_value(&s->sentBytes));
        }
        buffer_json_array_close(wb);
    }
    dfe_done(s);
}

// ============================================================
// Network Protocols (combined TCP + UDP + SMB function)
// ============================================================

// Column order for all rows: Transport, Family, Share, Received, Sent, Errors,
//   ConnActive, ConnEstablished, ConnPassive, ConnReset, SegsTotal, SegsRetransmitted,
//   DatagramsNoPort, TreeConnects, ReceivedBytes, SentBytes.

static void proto_emit_tcp_row(BUFFER *wb, const TCP_FAMILY *tcp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, "TCP");
        buffer_json_add_array_item_string(wb, tcp->af);
        buffer_json_add_array_item_string(wb, ""); // Share — N/A for TCP
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->segments_received));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->segments_sent));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->connection_failures));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->connections_active));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->connections_established));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->connections_passive));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->connections_reset));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->segments_total));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&tcp->segments_retransmitted));
        buffer_json_add_array_item_uint64(wb, 0); // DatagramsNoPort — UDP only
        buffer_json_add_array_item_uint64(wb, 0); // TreeConnects    — SMB only
        buffer_json_add_array_item_uint64(wb, 0); // ReceivedBytes   — SMB only
        buffer_json_add_array_item_uint64(wb, 0); // SentBytes       — SMB only
    }
    buffer_json_array_close(wb);
}

static void proto_emit_udp_row(BUFFER *wb, const UDP_FAMILY *udp)
{
    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, "UDP");
        buffer_json_add_array_item_string(wb, udp->af);
        buffer_json_add_array_item_string(wb, ""); // Share — N/A for UDP
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&udp->datagrams_received));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&udp->datagrams_sent));
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&udp->datagrams_received_errors));
        buffer_json_add_array_item_uint64(wb, 0); // ConnActive        — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnEstablished   — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnPassive       — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // ConnReset         — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // SegsTotal         — TCP only
        buffer_json_add_array_item_uint64(wb, 0); // SegsRetransmitted — TCP only
        buffer_json_add_array_item_uint64(wb, nv_perflib_value(&udp->datagrams_no_port));
        buffer_json_add_array_item_uint64(wb, 0); // TreeConnects  — SMB only
        buffer_json_add_array_item_uint64(wb, 0); // ReceivedBytes — SMB only
        buffer_json_add_array_item_uint64(wb, 0); // SentBytes     — SMB only
    }
    buffer_json_array_close(wb);
}

void function_network_protocols(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    bool have_tcp_ipv4 = false;
    bool have_tcp_ipv6 = false;
    bool have_udp_ipv4 = false;
    bool have_udp_ipv6 = false;
    bool have_smb      = false;

    if(unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED))) {
        nv_send_error(transaction, HTTP_RESP_CLIENT_CLOSED_REQUEST, "Request cancelled.");
        goto cleanup;
    }

    // Serialize access to the shared COUNTER_DATA state so previous/current
    // deltas are consistent regardless of which worker thread handles the request.
    // Hold the mutex across collection AND data-row emission: proto_emit_*_row
    // reads previous/current fields from the same shared structs that another
    // worker would overwrite during its own collection pass.
    netdata_mutex_lock(&nv_collect_mutex);
    have_tcp_ipv4 = tcp_collect_family(&tcp_ipv4);
    have_tcp_ipv6 = tcp_collect_family(&tcp_ipv6);
    have_udp_ipv4 = udp_collect_family(&udp_ipv4);
    have_udp_ipv6 = udp_collect_family(&udp_ipv6);
    have_smb      = nv_smb_collect();

    if(unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED))) {
        netdata_mutex_unlock(&nv_collect_mutex);
        nv_send_error(transaction, HTTP_RESP_CLIENT_CLOSED_REQUEST, "Request cancelled.");
        goto cleanup;
    }

    if(unlikely(!have_tcp_ipv4 && !have_tcp_ipv6 && !have_udp_ipv4 && !have_udp_ipv6 && !have_smb)) {
        netdata_mutex_unlock(&nv_collect_mutex);
        nv_send_error(transaction, HTTP_RESP_INTERNAL_SERVER_ERROR,
                      "failed to collect Windows network stack statistics");
        goto cleanup;
    }

    time_t now_s = now_realtime_sec();
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    nv_table_begin(wb, NV_WIN_FUNCTION_PROTO_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        if(have_tcp_ipv4)
            proto_emit_tcp_row(wb, &tcp_ipv4);
        if(have_tcp_ipv6)
            proto_emit_tcp_row(wb, &tcp_ipv6);
        if(have_udp_ipv4)
            proto_emit_udp_row(wb, &udp_ipv4);
        if(have_udp_ipv6)
            proto_emit_udp_row(wb, &udp_ipv6);
        if(have_smb)
            proto_emit_smb_rows(wb);
    }
    buffer_json_array_close(wb); // data
    netdata_mutex_unlock(&nv_collect_mutex);

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        nv_add_key_field(wb, &field_id, "Transport", "Transport Protocol");
        nv_add_key_field(wb, &field_id, "Family",    "IP Protocol Family");
        nv_add_key_field(wb, &field_id, "Share",     "SMB Share Name");

        // TCP/UDP traffic columns — segments for TCP, datagrams for UDP; SMB rows carry 0
        nv_add_int_field(wb, &field_id, "Received", "Received (Segments/Datagrams)", "segments/datagrams/s");
        nv_add_int_field(wb, &field_id, "Sent",     "Sent (Segments/Datagrams)",     "segments/datagrams/s");
        nv_add_int_field(wb, &field_id, "Errors",   "Errors (Failures/Rx Errors)",   "errors");

        // TCP-only columns (UDP rows carry 0)
        // ConnActive/ConnPassive/ConnReset are PERF_COUNTER_RAWCOUNT cumulative totals,
        // not per-second rates, so units have no /s suffix.
        nv_add_int_field(wb, &field_id, "ConnActive",        "Active Connections Opened",         "opens");
        nv_add_int_field(wb, &field_id, "ConnEstablished",   "Currently Established Connections",  "connections");
        nv_add_int_field(wb, &field_id, "ConnPassive",       "Passive Connections Opened",         "opens");
        nv_add_int_field(wb, &field_id, "ConnReset",         "Reset Connections",                  "resets");
        nv_add_int_field(wb, &field_id, "SegsTotal",         "Total Segments",                     "segments/s");
        nv_add_int_field(wb, &field_id, "SegsRetransmitted", "Retransmitted Segments",             "segments/s");

        // UDP-only column (TCP/SMB rows carry 0)
        nv_add_int_field(wb, &field_id, "DatagramsNoPort", "Datagrams with No Port", "datagrams/s");

        // SMB-only columns (TCP/UDP rows carry 0)
        nv_add_int_field(wb, &field_id, "TreeConnects",   "Active SMB Tree Connections", "connections");
        nv_add_int_field(wb, &field_id, "ReceivedBytes",  "Received Bytes",              "bytes/s");
        nv_add_int_field(wb, &field_id, "SentBytes",      "Sent Bytes",                  "bytes/s");
    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Received");

    // charts.columns = metric columns for the Y axis (NOT the groupby column)
    // Traffic: TCP/UDP segment/datagram counts — incompatible unit with SMB bytes.
    // SMB Traffic: per-share byte throughput — incompatible unit with segment counts.
    buffer_json_member_add_object(wb, "charts");
    {
        nv_add_stacked_bar_chart(wb, "Traffic",     "Received",      "Sent");
        nv_add_stacked_bar_chart(wb, "SMB Traffic", "ReceivedBytes", "SentBytes");
    }
    buffer_json_object_close(wb); // charts

    // default_charts: [chart_key, groupby_column]
    // Traffic groups TCP/UDP rows; SMB Traffic groups SMB rows.
    buffer_json_member_add_array(wb, "default_charts");
    {
        nv_add_default_chart(wb, "Traffic",     "Transport");
        nv_add_default_chart(wb, "Traffic",     "Family");
        nv_add_default_chart(wb, "SMB Traffic", "Share");
    }
    buffer_json_array_close(wb); // default_charts

    buffer_json_member_add_object(wb, "group_by");
    {
        nv_add_group_by(wb, "Transport");
        nv_add_group_by(wb, "Family");
        nv_add_group_by(wb, "Share");
    }
    buffer_json_object_close(wb); // group_by

    nv_send_result(transaction, wb, now_s);

cleanup:
    // Release the thread-local perflib buffer once per request to avoid retaining
    // the largest query size for the lifetime of the worker thread.
    perflibFreePerformanceData();
}

// ============================================================
// Network Connections — per-socket view via IP Helper API
// ============================================================

// --- Address space classification -----------------------------------------------

static const uint8_t nv_ipv6_zero_addr[16]    = {0};
static const uint8_t nv_ipv6_loopback_addr[16] = {0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1};

static const char *nv_ipv4_address_space(DWORD ip_nbo)
{
    uint32_t ip = ntohl(ip_nbo);
    if (!ip) return "zero";
    if ((ip >> 24) == 127) return "loopback";
    if ((ip >> 28) == 0xEU) return "multicast";  // 224.0.0.0/4
    uint32_t o1 = ip >> 24;
    uint32_t o2 = (ip >> 16) & 0xFF;
    if (o1 == 10 ||
        (o1 == 172 && o2 >= 16 && o2 <= 31) ||
        (o1 == 192 && o2 == 168) ||
        (o1 == 169 && o2 == 254))
        return "private";
    return "public";
}

static const char *nv_ipv6_address_space(const UCHAR *b)
{
    static const uint8_t v4map[12]  = {0,0,0,0,0,0,0,0,0,0,0xFF,0xFF};

    if (!memcmp(b, nv_ipv6_zero_addr, 16)) return "zero";
    if (!memcmp(b, nv_ipv6_loopback_addr, 16)) return "loopback";
    if (!memcmp(b, v4map, 12)) {
        DWORD v4;
        memcpy(&v4, b + 12, 4);
        return nv_ipv4_address_space(v4);
    }
    if (b[0] == 0xFF) return "multicast";
    if ((b[0] & 0xFE) == 0xFC) return "private";              // ULA fc00::/7
    if (b[0] == 0xFE && (b[1] & 0xC0) == 0x80) return "private"; // link-local fe80::/10
    return "public";
}

// --- Loopback checks for direction detection ------------------------------------

static bool nv_is_ipv4_loopback(DWORD ip_nbo)
{
    return (ntohl(ip_nbo) >> 24) == 127;
}

static bool nv_is_ipv6_loopback(const UCHAR *b)
{
    return !memcmp(b, nv_ipv6_loopback_addr, 16);
}

// --- TCP state number → string --------------------------------------------------

static const char *nv_tcp_state_str(DWORD state)
{
    switch (state) {
        case MIB_TCP_STATE_CLOSED:     return "close";
        case MIB_TCP_STATE_LISTEN:     return "listen";
        case MIB_TCP_STATE_SYN_SENT:   return "syn-sent";
        case MIB_TCP_STATE_SYN_RCVD:   return "syn-received";
        case MIB_TCP_STATE_ESTAB:      return "established";
        case MIB_TCP_STATE_FIN_WAIT1:  return "fin-wait1";
        case MIB_TCP_STATE_FIN_WAIT2:  return "fin-wait2";
        case MIB_TCP_STATE_CLOSE_WAIT: return "close-wait";
        case MIB_TCP_STATE_CLOSING:    return "closing";
        case MIB_TCP_STATE_LAST_ACK:   return "last-ack";
        case MIB_TCP_STATE_TIME_WAIT:  return "time-wait";
        case MIB_TCP_STATE_DELETE_TCB: return "delete";
        default:                       return "unknown";
    }
}

// --- Process info ---------------------------------------------------------------

static void nv_get_comm(DWORD pid, char *comm, size_t comm_size)
{
    comm[0] = '\0';
    HANDLE h = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!h) return;

    char path[MAX_PATH];
    DWORD sz = (DWORD)sizeof(path);
    if (QueryFullProcessImageNameA(h, 0, path, &sz)) {
        const char *base = strrchr(path, '\\');
        base = base ? base + 1 : path;
        strncpyz(comm, base, comm_size - 1);
    }
    CloseHandle(h);
}

static STRING *nv_get_username(DWORD pid)
{
    HANDLE hp = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!hp) return NULL;

    STRING *result = NULL;
    HANDLE ht = NULL;
    if (OpenProcessToken(hp, TOKEN_QUERY, &ht)) {
        DWORD sz = 0;
        GetTokenInformation(ht, TokenUser, NULL, 0, &sz);
        if (sz) {
            TOKEN_USER *tu = mallocz(sz);
            if (GetTokenInformation(ht, TokenUser, tu, sz, &sz))
                result = cached_sid_fullname_or_sid_str(tu->User.Sid);
            freez(tu);
        }
        CloseHandle(ht);
    }
    CloseHandle(hp);
    return result;
}

// --- Per-request PID cache ------------------------------------------------------
// Resolving process name and username requires OpenProcess + kernel work.
// Many sockets share the same PID (e.g. svchost.exe), so caching per request
// eliminates redundant syscalls for every repeated PID.

typedef struct {
    DWORD   pid;
    char    comm[256];
    STRING *username; // ref-counted; freed by nv_pid_cache_free
} NV_PID_CACHE_ENTRY;

typedef struct {
    NV_PID_CACHE_ENTRY *entries;
    size_t              used;
    size_t              capacity;
} NV_PID_CACHE;

static void nv_pid_cache_init(NV_PID_CACHE *c)
{
    c->entries  = NULL;
    c->used     = 0;
    c->capacity = 0;
}

static void nv_pid_cache_lookup(NV_PID_CACHE *c, DWORD pid,
                                const char **comm_out, const char **username_out)
{
    for (size_t i = 0; i < c->used; i++) {
        if (c->entries[i].pid == pid) {
            *comm_out     = c->entries[i].comm;
            *username_out = string2str(c->entries[i].username);
            return;
        }
    }

    if (c->used >= c->capacity) {
        c->capacity = c->capacity ? c->capacity * 2 : 32;
        c->entries  = reallocz(c->entries, c->capacity * sizeof(*c->entries));
    }

    NV_PID_CACHE_ENTRY *e = &c->entries[c->used++];
    e->pid     = pid;
    e->comm[0] = '\0';
    e->username = NULL;

    nv_get_comm(pid, e->comm, sizeof(e->comm));
    e->username = nv_get_username(pid);

    *comm_out     = e->comm;
    *username_out = string2str(e->username);
}

static void nv_pid_cache_free(NV_PID_CACHE *c)
{
    for (size_t i = 0; i < c->used; i++)
        string_freez(c->entries[i].username);
    freez(c->entries);
    c->entries  = NULL;
    c->used     = 0;
    c->capacity = 0;
}

// --- Listening-port set for direction classification ----------------------------
// Key is {family, port} so that a TCP6 listener on port N does not cause TCP4
// connections whose ephemeral port happens to be N to be misclassified as inbound.
// Mirrors the Linux approach (local-sockets.h:772-774) where family and protocol
// are both part of the listening-port key.

typedef struct {
    ULONG  family;   // NV_WIN_AF_INET or NV_WIN_AF_INET6
    DWORD  port;     // host byte order
} NV_LISTEN_PORT;

typedef struct {
    NV_LISTEN_PORT *ports;
    size_t          used;
    size_t          capacity;
} NV_LISTEN_SET;

static void nv_listen_set_init(NV_LISTEN_SET *s)
{
    s->ports    = NULL;
    s->used     = 0;
    s->capacity = 0;
}

static void nv_listen_set_add(NV_LISTEN_SET *s, DWORD port_hbo, ULONG family)
{
    if (s->used >= s->capacity) {
        s->capacity = s->capacity ? s->capacity * 2 : 64;
        s->ports = reallocz(s->ports, s->capacity * sizeof(NV_LISTEN_PORT));
    }
    s->ports[s->used].port   = port_hbo;
    s->ports[s->used].family = family;
    s->used++;
}

static int nv_listen_port_compar(const void *a, const void *b)
{
    const NV_LISTEN_PORT *pa = a, *pb = b;
    if (pa->family != pb->family)
        return (pa->family > pb->family) - (pa->family < pb->family);
    return (pa->port > pb->port) - (pa->port < pb->port);
}

static void nv_listen_set_sort(NV_LISTEN_SET *s)
{
    if (s->used > 1)
        qsort(s->ports, s->used, sizeof(NV_LISTEN_PORT), nv_listen_port_compar);
}

static bool nv_listen_set_contains(const NV_LISTEN_SET *s, DWORD port_hbo, ULONG family)
{
    if (!s->used) return false;
    NV_LISTEN_PORT key = { .family = family, .port = port_hbo };
    return bsearch(&key, s->ports, s->used, sizeof(NV_LISTEN_PORT), nv_listen_port_compar) != NULL;
}

static void nv_listen_set_free(NV_LISTEN_SET *s)
{
    freez(s->ports);
    s->ports    = NULL;
    s->used     = 0;
    s->capacity = 0;
}

// --- Table fetch helpers --------------------------------------------------------

// Common Windows API signature for GetExtended{Tcp,Udp}Table.
// Both functions have identical prototypes except for the TableClass enum type;
// since all Windows enum types are int-sized, a single typedef covers both.
typedef DWORD (WINAPI *NV_GET_TABLE_FN)(PVOID, PDWORD, BOOL, ULONG, int, ULONG);

static void *nv_fetch_ip_table(NV_GET_TABLE_FN get_fn, ULONG af, int table_class)
{
    DWORD size = 0;
    get_fn(NULL, &size, FALSE, af, table_class, 0);
    if (!size) return NULL;

    // Add headroom to tolerate connections added between size-probe and actual fetch.
    size += 4096;
    void *buf = mallocz(size);
    DWORD ret = get_fn(buf, &size, FALSE, af, table_class, 0);
    if (ret == ERROR_INSUFFICIENT_BUFFER) {
        buf = reallocz(buf, size);
        ret = get_fn(buf, &size, FALSE, af, table_class, 0);
    }
    if (ret != NO_ERROR) {
        freez(buf);
        return NULL;
    }
    return buf;
}

static void *nv_fetch_tcp_table(ULONG af)
{
    return nv_fetch_ip_table((NV_GET_TABLE_FN)GetExtendedTcpTable, af, TCP_TABLE_OWNER_PID_ALL);
}

static void *nv_fetch_udp_table(ULONG af)
{
    return nv_fetch_ip_table((NV_GET_TABLE_FN)GetExtendedUdpTable, af, UDP_TABLE_OWNER_PID);
}

// --- Row emitter ----------------------------------------------------------------

// Column order (matches the columns declared in function_network_connections):
//   Direction, Protocol, State, PID, Process, User, Portname,
//   LocalIP, LocalPort, LocalAddressSpace,
//   RemoteIP, RemotePort, RemoteAddressSpace,
//   ServerPort, Count

static void nv_emit_row(BUFFER *wb,
                        NV_PID_CACHE *pid_cache,
                        const char *direction,
                        const char *protocol,
                        const char *state,
                        DWORD pid,
                        const char *local_ip,  DWORD local_port_hbo,  const char *local_as,
                        const char *remote_ip, DWORD remote_port_hbo, const char *remote_as)
{
    DWORD server_port = (strcmp(direction, "outbound") == 0) ? remote_port_hbo : local_port_hbo;

    uint16_t ipproto = (strncmp(protocol, "tcp", 3) == 0) ? IPPROTO_TCP : IPPROTO_UDP;
    STRING *portname = system_servicenames_cache_lookup(sc, (uint16_t)server_port, (uint16_t)ipproto);

    const char *comm, *username;
    nv_pid_cache_lookup(pid_cache, pid, &comm, &username);

    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, direction);
        buffer_json_add_array_item_string(wb, protocol);
        buffer_json_add_array_item_string(wb, state);
        buffer_json_add_array_item_uint64(wb, pid);
        buffer_json_add_array_item_string(wb, comm[0] ? comm : "[unknown]");
        buffer_json_add_array_item_string(wb, (username && username[0]) ? username : "[unknown]");
        buffer_json_add_array_item_string(wb, string2str(portname));
        buffer_json_add_array_item_string(wb, local_ip);
        buffer_json_add_array_item_uint64(wb, local_port_hbo);
        buffer_json_add_array_item_string(wb, local_as);
        buffer_json_add_array_item_string(wb, remote_ip);
        buffer_json_add_array_item_uint64(wb, remote_port_hbo);
        buffer_json_add_array_item_string(wb, remote_as);
        buffer_json_add_array_item_uint64(wb, server_port);
        buffer_json_add_array_item_uint64(wb, 1); // Count — always 1 (detailed view)
    }
    buffer_json_array_close(wb);

    string_freez(portname);
    // comm and username are owned by pid_cache; not freed here
}

// --- Main function handler -------------------------------------------------------

void function_network_connections(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED))) {
        nv_send_error(transaction, HTTP_RESP_CLIENT_CLOSED_REQUEST, "Request cancelled.");
        return;
    }

    // Fetch all four tables upfront to minimise the enumeration window.
    MIB_TCPTABLE_OWNER_PID  *tcp4 = nv_fetch_tcp_table(NV_WIN_AF_INET);
    MIB_TCP6TABLE_OWNER_PID *tcp6 = nv_fetch_tcp_table(NV_WIN_AF_INET6);
    MIB_UDPTABLE_OWNER_PID  *udp4 = nv_fetch_udp_table(NV_WIN_AF_INET);
    MIB_UDP6TABLE_OWNER_PID *udp6 = nv_fetch_udp_table(NV_WIN_AF_INET6);

    // A NULL return from nv_fetch_*_table means the API call failed entirely
    // (not merely an empty table — zero-entry tables return a non-NULL buffer).
    // If every fetch failed we cannot distinguish "no sockets" from a broken
    // driver, so report an error instead of silently returning an empty table.
    if (unlikely(!tcp4 && !tcp6 && !udp4 && !udp6)) {
        nv_send_error(transaction, HTTP_RESP_INTERNAL_SERVER_ERROR,
                      "failed to collect Windows network connections");
        return;
    }

    NV_PID_CACHE pid_cache;
    nv_pid_cache_init(&pid_cache);

    // --- First pass: collect all TCP LISTEN ports for direction classification ---
    NV_LISTEN_SET listen_set;
    nv_listen_set_init(&listen_set);

// Collect all LISTEN ports from a TCP table into the listen set.
// Works for both MIB_TCPTABLE_OWNER_PID and MIB_TCP6TABLE_OWNER_PID because
// the macro expands with the concrete type at each call site.
#define NV_COLLECT_TCP_LISTEN(tbl, family, set)                                         \
    do {                                                                                 \
        for (DWORD _i = 0; _i < (tbl)->dwNumEntries; _i++) {                            \
            if ((tbl)->table[_i].dwState == MIB_TCP_STATE_LISTEN)                       \
                nv_listen_set_add((set), ntohs((uint16_t)(tbl)->table[_i].dwLocalPort), \
                                  (family));                                             \
        }                                                                                \
    } while (0)

    if (tcp4) NV_COLLECT_TCP_LISTEN(tcp4, NV_WIN_AF_INET,  &listen_set);
    if (tcp6) NV_COLLECT_TCP_LISTEN(tcp6, NV_WIN_AF_INET6, &listen_set);
    nv_listen_set_sort(&listen_set);

    // --- Second pass: emit rows --------------------------------------------------
    time_t now_s = now_realtime_sec();
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    nv_table_begin(wb, NV_WIN_FUNCTION_CONN_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        char local_ip [INET6_ADDRSTRLEN];
        char remote_ip[INET6_ADDRSTRLEN];

        // TCP IPv4
        if (tcp4) {
            for (DWORD i = 0; i < tcp4->dwNumEntries; i++) {
                if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
                    goto emit_done;
                MIB_TCPROW_OWNER_PID *r = &tcp4->table[i];

                struct in_addr la = { .s_addr = r->dwLocalAddr };
                if (!inet_ntop(AF_INET, &la, local_ip, sizeof(local_ip)))
                    continue;

                DWORD local_port = ntohs((uint16_t)r->dwLocalPort);
                const char *local_as = nv_ipv4_address_space(r->dwLocalAddr);

                const char *direction;
                if (r->dwState == MIB_TCP_STATE_LISTEN) {
                    direction = "listen";
                } else if (nv_is_ipv4_loopback(r->dwLocalAddr) || nv_is_ipv4_loopback(r->dwRemoteAddr)) {
                    direction = nv_listen_set_contains(&listen_set, local_port, NV_WIN_AF_INET) ? "inbound" : "outbound";
                } else if (nv_listen_set_contains(&listen_set, local_port, NV_WIN_AF_INET)) {
                    direction = "inbound";
                } else {
                    direction = "outbound";
                }

                // LISTEN rows have no remote endpoint; defaults cover inet_ntop failure too.
                const char *remote_ip_s      = "";
                const char *remote_as        = "";
                DWORD remote_port_emit       = 0;
                if (r->dwState != MIB_TCP_STATE_LISTEN && r->dwRemoteAddr) {
                    struct in_addr ra = { .s_addr = r->dwRemoteAddr };
                    if (inet_ntop(AF_INET, &ra, remote_ip, sizeof(remote_ip))) {
                        remote_ip_s      = remote_ip;
                        remote_as        = nv_ipv4_address_space(r->dwRemoteAddr);
                        remote_port_emit = ntohs((uint16_t)r->dwRemotePort);
                    }
                }

                nv_emit_row(wb, &pid_cache, direction, "tcp4", nv_tcp_state_str(r->dwState), r->dwOwningPid,
                            local_ip, local_port, local_as,
                            remote_ip_s, remote_port_emit, remote_as);
            }
        }

        if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
            goto emit_done;

        // TCP IPv6
        if (tcp6) {
            for (DWORD i = 0; i < tcp6->dwNumEntries; i++) {
                if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
                    goto emit_done;
                MIB_TCP6ROW_OWNER_PID *r = &tcp6->table[i];

                if (!inet_ntop(AF_INET6, r->ucLocalAddr, local_ip, sizeof(local_ip)))
                    continue;
                DWORD local_port = ntohs((uint16_t)r->dwLocalPort);
                const char *local_as = nv_ipv6_address_space(r->ucLocalAddr);

                const char *direction;
                if (r->dwState == MIB_TCP_STATE_LISTEN) {
                    direction = "listen";
                } else if (nv_is_ipv6_loopback(r->ucLocalAddr) || nv_is_ipv6_loopback(r->ucRemoteAddr)) {
                    direction = nv_listen_set_contains(&listen_set, local_port, NV_WIN_AF_INET6) ? "inbound" : "outbound";
                } else if (nv_listen_set_contains(&listen_set, local_port, NV_WIN_AF_INET6)) {
                    direction = "inbound";
                } else {
                    direction = "outbound";
                }

                bool remote_zero = !memcmp(r->ucRemoteAddr, nv_ipv6_zero_addr, 16) && !r->dwRemotePort;

                const char *remote_ip_s      = "";
                const char *remote_as        = "";
                DWORD remote_port_emit       = 0;
                if (r->dwState != MIB_TCP_STATE_LISTEN && !remote_zero) {
                    if (inet_ntop(AF_INET6, r->ucRemoteAddr, remote_ip, sizeof(remote_ip))) {
                        remote_ip_s      = remote_ip;
                        remote_as        = nv_ipv6_address_space(r->ucRemoteAddr);
                        remote_port_emit = ntohs((uint16_t)r->dwRemotePort);
                    }
                }

                nv_emit_row(wb, &pid_cache, direction, "tcp6", nv_tcp_state_str(r->dwState), r->dwOwningPid,
                            local_ip, local_port, local_as,
                            remote_ip_s, remote_port_emit, remote_as);
            }
        }

        if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
            goto emit_done;

        // UDP IPv4 — endpoints have no remote address; direction is always "listen".
        if (udp4) {
            for (DWORD i = 0; i < udp4->dwNumEntries; i++) {
                if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
                    goto emit_done;
                MIB_UDPROW_OWNER_PID *r = &udp4->table[i];

                struct in_addr la = { .s_addr = r->dwLocalAddr };
                if (!inet_ntop(AF_INET, &la, local_ip, sizeof(local_ip)))
                    continue;
                DWORD local_port = ntohs((uint16_t)r->dwLocalPort);
                const char *local_as = nv_ipv4_address_space(r->dwLocalAddr);

                nv_emit_row(wb, &pid_cache, "listen", "udp4", "stateless", r->dwOwningPid,
                            local_ip, local_port, local_as,
                            "", 0, "");
            }
        }

        if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
            goto emit_done;

        // UDP IPv6
        if (udp6) {
            for (DWORD i = 0; i < udp6->dwNumEntries; i++) {
                if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)))
                    goto emit_done;
                MIB_UDP6ROW_OWNER_PID *r = &udp6->table[i];

                if (!inet_ntop(AF_INET6, r->ucLocalAddr, local_ip, sizeof(local_ip)))
                    continue;
                DWORD local_port = ntohs((uint16_t)r->dwLocalPort);
                const char *local_as = nv_ipv6_address_space(r->ucLocalAddr);

                nv_emit_row(wb, &pid_cache, "listen", "udp6", "stateless", r->dwOwningPid,
                            local_ip, local_port, local_as,
                            "", 0, "");
            }
        }

emit_done:;
    }
    buffer_json_array_close(wb); // data

    nv_listen_set_free(&listen_set);
    nv_pid_cache_free(&pid_cache);
    freez(tcp4);
    freez(tcp6);
    freez(udp4);
    freez(udp6);

    if (unlikely(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED))) {
        nv_send_error(transaction, HTTP_RESP_CLIENT_CLOSED_REQUEST, "Request cancelled.");
        return;
    }

    // --- Column definitions ---
    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        nv_conn_str_field(wb, &field_id, "Direction",          "Socket Direction",          RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY);
        nv_conn_str_field(wb, &field_id, "Protocol",           "Socket Protocol",           RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "State",              "Socket State",              RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_int_field(wb, &field_id, "PID",                "Process ID",                RRDF_FIELD_FILTER_NONE,        RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "Process",            "Process Name",              RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH);
        nv_conn_str_field(wb, &field_id, "User",               "Username",                  RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "Portname",           "Server Port Name",          RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "LocalIP",            "Local IP Address",          RRDF_FIELD_FILTER_NONE,        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH);
        nv_conn_int_field(wb, &field_id, "LocalPort",          "Local Port",                RRDF_FIELD_FILTER_NONE,        RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "LocalAddressSpace",  "Local IP Address Space",    RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_NONE);
        nv_conn_str_field(wb, &field_id, "RemoteIP",           "Remote IP Address",         RRDF_FIELD_FILTER_NONE,        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH);
        nv_conn_int_field(wb, &field_id, "RemotePort",         "Remote Port",               RRDF_FIELD_FILTER_NONE,        RRDF_FIELD_OPTS_VISIBLE);
        nv_conn_str_field(wb, &field_id, "RemoteAddressSpace", "Remote IP Address Space",   RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_NONE);
        nv_conn_int_field(wb, &field_id, "ServerPort",         "Server Port",               RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_NONE);
        buffer_rrdf_table_add_field(wb, field_id++, "Count", "Number of sockets",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, "sockets", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
            RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_NONE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Direction");

    buffer_json_member_add_object(wb, "custom_charts");
    {
        buffer_json_member_add_object(wb, "Network Map");
        {
            buffer_json_member_add_string(wb, "type", "network-viewer");
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // custom_charts

    buffer_json_member_add_object(wb, "group_by");
    {
        nv_add_group_by(wb, "Direction");
        nv_add_group_by(wb, "Protocol");
        nv_add_group_by(wb, "State");
        nv_add_group_by(wb, "Process");
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
    if(nd_environment_freeze_process() != 0)
        fatal("Cannot freeze the process environment: %s", strerror(errno));
    netdata_threads_init_for_external_plugins(0);

    PerflibNamesRegistryInitialize();
    netdata_mutex_init(&nv_collect_mutex);

    nv_smb_shares = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_SMB_SHARE));
    dictionary_register_insert_callback(nv_smb_shares, nv_smb_share_insert_cb, NULL);

    // Prime each family's COUNTER_DATA so the first real request has a valid
    // previous baseline and rate counters return non-zero values immediately.
    initialize_tcp_keys(&tcp_ipv4);
    initialize_tcp_keys(&tcp_ipv6);
    initialize_udp_keys(&udp_ipv4);
    initialize_udp_keys(&udp_ipv6);
    tcp_collect_family(&tcp_ipv4);
    tcp_collect_family(&tcp_ipv6);
    udp_collect_family(&udp_ipv4);
    udp_collect_family(&udp_ipv6);
    nv_smb_collect();
    perflibFreePerformanceData();

    cached_sid_username_init();
    sc = system_servicenames_cache_init();

    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
            NV_WIN_FUNCTION_PROTO, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NV_WIN_FUNCTION_PROTO_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE),
            NV_WIN_FUNCTION_PRIORITY);

    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
            NV_WIN_FUNCTION_CONN, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NV_WIN_FUNCTION_CONN_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            NV_WIN_FUNCTION_PRIORITY);

    fflush(stdout);

    struct functions_evloop_globals *wg =
        functions_evloop_init(5, "NV-WIN", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, NV_WIN_FUNCTION_PROTO, function_network_protocols,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    functions_evloop_add_function(wg, NV_WIN_FUNCTION_CONN, function_network_connections,
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

    functions_evloop_cancel_threads(wg);
    PerflibNamesRegistryCleanup();
    system_servicenames_cache_destroy(sc);
    dictionary_destroy(nv_smb_shares);

    return 0;
}
