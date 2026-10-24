// SPDX-License-Identifier: GPL-3.0-or-later

// Shared topology:network-connections contract for network-viewer.plugin.
//
// network-viewer-topology.c is compiled on every OS that builds the plugin:
//   - POSIX  : together with network-viewer.c (connections/protocols/dns + main);
//   - Windows: together with network-viewer-windows.c (connections/protocols +
//              main).
// Both translation units MUST see the same LOCAL_SOCKET layout, so this header
// owns the LOCAL_SOCKETS_EXTENDED_MEMBERS definition and includes the
// local-sockets umbrella.

#ifndef NETDATA_NETWORK_VIEWER_TOPOLOGY_H
#define NETDATA_NETWORK_VIEWER_TOPOLOGY_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "network-viewer-apps-lookup-client.h"
#include "network-viewer-topology-containers.h"

#ifndef LOCAL_SOCKETS_EXTENDED_MEMBERS
#define LOCAL_SOCKETS_EXTENDED_MEMBERS struct {                  \
        size_t count;                                            \
        struct {                                                 \
            pid_t pid;                                           \
            uid_t uid;                                           \
            SOCKET_DIRECTION direction;                          \
            int state;                                           \
            uint64_t net_ns_inode;                               \
            struct socket_endpoint server;                       \
            const char *local_address_space;                     \
            const char *remote_address_space;                    \
        } aggregated_key;                                        \
        /* eBPF per-group sums, valid only when ebpf_valid=true. \
         * Accumulated during aggregation so the serialiser does \
         * not perform a per-PID lookup for each aggregated row. \
         * Mirrors the uint64_t fields of ebpf_socket_publish_apps, \
         * using primitive types to avoid an include-order dependency. */ \
        uint64_t ebpf_bytes_sent;                                \
        uint64_t ebpf_bytes_received;                            \
        uint64_t ebpf_call_tcp_sent;                             \
        uint64_t ebpf_call_tcp_received;                         \
        uint64_t ebpf_retransmit;                                \
        uint64_t ebpf_call_udp_sent;                             \
        uint64_t ebpf_call_udp_received;                         \
        uint64_t ebpf_call_close;                                \
        uint64_t ebpf_call_tcp_v4_conn;                          \
        uint64_t ebpf_call_tcp_v6_conn;                          \
        bool ebpf_valid;                                         \
    } network_viewer;
#endif

#include "libnetdata/local-sockets/local-sockets.h"

// stdout_mutex is defined by the plugin's main TU (network-viewer.c on POSIX,
// network-viewer-windows.c on Windows).
extern netdata_mutex_t stdout_mutex;

#if defined(LOCAL_SOCKETS_USE_SETNS)
// spawn server used for network-namespace walking; defined by the POSIX main.
extern SPAWN_SERVER *spawn_srv;
#endif

#define NETWORK_TOPOLOGY_VIEWER_FUNCTION "topology:network-connections"
#define NETWORK_TOPOLOGY_VIEWER_HELP "Shows live network-connections topology with self/process/endpoint actors and ownership/socket links."
#define NETWORK_VIEWER_RESPONSE_UPDATE_EVERY 5

// Linux-compatible TCP state type used by the local-sockets enum map.
typedef int TCP_STATE;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(SOCKET_DIRECTION);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(TCP_STATE);

// Response helpers shared with the other Functions served from
// network-viewer.c (network-connections, dns-queries, network-protocols);
// defined in network-viewer-topology.c (the only topology TU compiled on
// Windows too).
BUFFER *nv_response_preamble(void);
void network_viewer_finalize_response_buffer(BUFFER *wb, time_t now_s, time_t expires_delta_s);
BUFFER *network_viewer_json_error_response(int code, const char *message);
void network_viewer_emit_response(const char *transaction, BUFFER *wb);

// NV_DISPATCH calls the given result expression, emits the response for the
// current transaction, then frees the buffer.  Requires `transaction` in scope.
#define NV_DISPATCH(wb_expr) do { \
    BUFFER *_nv_wb = (wb_expr); \
    network_viewer_emit_response(transaction, _nv_wb); \
    buffer_free(_nv_wb); \
} while(0)

// Starttime cache entry shared with the connections code in network-viewer.c
typedef struct {
    uint64_t starttime;
} NV_PID_STARTTIME_CACHE_ENTRY;

// Helpers the connections code in network-viewer.c still needs from the
// topology translation unit.
void nv_pid_append(uint32_t **pids, size_t *count, size_t *capacity, pid_t pid);
size_t nv_pid_sort_unique(uint32_t *pids, size_t count);
void nv_warm_cache_from_aggregated_sockets(LOCAL_SOCKET **sockets, size_t count);
uint64_t topology_starttime_cache_get_or_load(DICTIONARY *pid_starttime_cache, uint64_t pid);
void topology_container_fields_snapshot_from_cache(
    DICTIONARY *container_field_snapshot, uint32_t pid, uint64_t starttime,
    uid_t uid, const char *process, NV_TOPOLOGY_CONTAINER_FIELDS *fields);

// topology:network-connections Function dispatch (functions_evloop signature).
void network_viewer_topology_function(
    const char *transaction, char *function, usec_t *stop_monotonic_ut,
    bool *cancelled, BUFFER *payload, HTTP_ACCESS access,
    const char *source, void *data);

// Topology payload renderer (exposed for the POSIX --test path in
// network-viewer.c).
BUFFER *network_viewer_topology_result(
    char *function, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *payload);

#endif /* NETDATA_NETWORK_VIEWER_TOPOLOGY_H */
