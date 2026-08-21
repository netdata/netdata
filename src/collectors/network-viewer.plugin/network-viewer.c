// SPDX-License-Identifier: GPL-3.0-or-later

// Function registration ownership split. NOTE: this file is compiled for Linux,
// FreeBSD and macOS only; Windows builds network-viewer-windows.c instead (see
// NETWORK_VIEWER_FILES in CMakeLists.txt), so "all OSes" below means "every OS
// that builds this file", plus Windows only where that file registers its own.
//   - network-connections         : all OSes -- here, and separately in
//                                   network-viewer-windows.c for Windows
//   - topology:network-connections: every OS -- served by the shared
//                                   network-viewer-topology.c
//   - dns-queries                 : Linux only, served by this plugin
//   - network-protocols           : FreeBSD only, served by this plugin
//                                   (on Linux, network-protocols is served by
//                                   ebpfgo.plugin via its stdin dispatcher;
//                                   adding a Linux registration here would
//                                   collide at runtime. The guard below
//                                   excludes the Linux registration at compile time.)
#include "network-viewer-topology.h"

#include "libnetdata/os/system-maps/system-services.h"

#if defined(OS_LINUX)
#include "network_viewer_ebpf_shared_memory.h"
#include "network_viewer_dns_shared_memory.h"
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
// BSD kernel protocol counters for the network-protocols Function: struct
// tcpstat / struct udpstat and the sysctl(3) API reading net.inet.*.stats.
#include <sys/sysctl.h>
#include <netinet/tcp_var.h>
#include <netinet/udp_var.h>
#endif

// Spawn server used by local-sockets for network-namespace walking (Linux
// only). Owned by this POSIX main TU; referenced by network-viewer-topology.c.
SPAWN_SERVER *spawn_srv = NULL;

#define ENABLE_DETAILED_VIEW

#define NETWORK_CONNECTIONS_VIEWER_FUNCTION "network-connections"
#define NETWORK_CONNECTIONS_VIEWER_HELP "Shows active network connections with protocol details, states, addresses, ports, and performance metrics."
#define NETWORK_PROTOCOLS_FUNCTION      "network-protocols"
#define NETWORK_PROTOCOLS_FUNCTION_HELP "TCP and UDP statistics (IPv4 and IPv6 combined)"
#define NETWORK_DNS_QUERIES_FUNCTION      "dns-queries"
#define NETWORK_DNS_QUERIES_FUNCTION_HELP "Linux eBPF DNS query and response statistics (UDP/TCP, IPv4/IPv6)"

#define NETWORK_VIEWER_TEST_DEFAULT_TIMEOUT_SECONDS 60ULL
#define NETWORK_VIEWER_TEST_TIMEOUT_DISABLED_SECONDS (100ULL * 365ULL * 24ULL * 60ULL * 60ULL)
#define NETWORK_VIEWER_TEST_MAX_REQUEST_BYTES (16ULL * 1024ULL * 1024ULL)



#define SIMPLE_HASHTABLE_VALUE_TYPE LOCAL_SOCKET *
#define SIMPLE_HASHTABLE_NAME _AGGREGATED_SOCKETS
#include "libnetdata/simple_hashtable/simple_hashtable.h"

netdata_mutex_t stdout_mutex;
#if defined(OS_FREEBSD) || defined(OS_MACOS)
static netdata_mutex_t nv_proto_mutex;
#endif

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
#if defined(OS_FREEBSD) || defined(OS_MACOS)
    netdata_mutex_init(&nv_proto_mutex);
#endif
}

static void __attribute__((destructor)) destroy_mutex(void) {
#if defined(OS_FREEBSD) || defined(OS_MACOS)
    netdata_mutex_destroy(&nv_proto_mutex);
#endif
    netdata_mutex_destroy(&stdout_mutex);
}
static bool plugin_should_exit = false;
static SERVICENAMES_CACHE *sc;



struct sockets_stats {
    BUFFER *wb;
    uint32_t *pids;
    size_t pid_count;
    size_t pid_capacity;
    DICTIONARY *container_field_snapshot;
    DICTIONARY *pid_starttime_cache;

    struct {
        uint32_t tcpi_rtt;
        uint32_t tcpi_rcv_rtt;
        uint32_t tcpi_total_retrans;
    } max;
};









static void network_viewer_add_enrichment_value(BUFFER *wb, const char *value)
{
    buffer_json_add_array_item_string(wb, value && *value ? value : NULL);
}

static void network_viewer_add_socket_enrichment_values(
    BUFFER *wb,
    DICTIONARY *container_field_snapshot,
    DICTIONARY *pid_starttime_cache,
    pid_t pid,
    uid_t uid,
    const char *process)
{
    NV_TOPOLOGY_CONTAINER_FIELDS fields;
    uint64_t starttime = topology_starttime_cache_get_or_load(pid_starttime_cache, (uint64_t)pid);
    topology_container_fields_snapshot_from_cache(
        container_field_snapshot, (uint32_t)pid, starttime, uid, process, &fields);

    network_viewer_add_enrichment_value(wb, fields.cgroup_status);
    network_viewer_add_enrichment_value(wb, fields.cgroup_path);
    network_viewer_add_enrichment_value(wb, fields.cgroup_name);
    network_viewer_add_enrichment_value(wb, fields.container_name);
    network_viewer_add_enrichment_value(wb, fields.orchestrator);
    network_viewer_add_enrichment_value(wb, fields.k8s_pod_name);
    network_viewer_add_enrichment_value(wb, fields.k8s_namespace);
    network_viewer_add_enrichment_value(wb, fields.k8s_workload);
    network_viewer_add_enrichment_value(wb, fields.docker_container_name);
    network_viewer_add_enrichment_value(wb, fields.docker_image);
    network_viewer_add_enrichment_value(wb, fields.systemd_unit_name);
    network_viewer_add_enrichment_value(wb, fields.systemd_unit_kind);
    network_viewer_add_enrichment_value(wb, fields.actor_kind);
}


static void local_socket_to_json_array(struct sockets_stats *st, const LOCAL_SOCKET *n, uint64_t proc_self_net_ns_inode, bool aggregated) {
    if(n->direction == SOCKET_DIRECTION_NONE)
        return;

    if(!aggregated)
        nv_pid_append(&st->pids, &st->pid_count, &st->pid_capacity, n->pid);

    BUFFER *wb = st->wb;

    char local_address[INET6_ADDRSTRLEN];
    char remote_address[INET6_ADDRSTRLEN];
    char *protocol;

    if(n->local.family == AF_INET) {
        ipv4_address_to_txt(n->local.ip.ipv4, local_address);

        if(local_sockets_is_zero_address(&n->remote))
            remote_address[0] = '\0';
        else
            ipv4_address_to_txt(n->remote.ip.ipv4, remote_address);

        protocol = n->local.protocol == IPPROTO_TCP ? "tcp4" : "udp4";
    }
    else if(is_local_socket_ipv46(n)) {
        strncpyz(local_address, "*", sizeof(local_address) - 1);
        remote_address[0] = '\0';
        protocol = n->local.protocol == IPPROTO_TCP ? "tcp46" : "udp46";
    }
    else if(n->local.family == AF_INET6) {
        ipv6_address_to_txt(&n->local.ip.ipv6, local_address);

        if(local_sockets_is_zero_address(&n->remote))
            remote_address[0] = '\0';
        else
            ipv6_address_to_txt(&n->remote.ip.ipv6, remote_address);

        protocol = n->local.protocol == IPPROTO_TCP ? "tcp6" : "udp6";
    }
    else
        return;

    const char *type;
    if(n->net_ns_inode == proc_self_net_ns_inode)
        type = "system";
    else if(n->net_ns_inode == 0)
        type = "[unknown]";
    else
        type = "container";

    buffer_json_add_array_item_array(wb);
    {
        buffer_json_add_array_item_string(wb, SOCKET_DIRECTION_2str(n->direction));
        buffer_json_add_array_item_string(wb, protocol);
        buffer_json_add_array_item_string(wb, type); // system or container
        if(n->local.protocol == IPPROTO_TCP)
            buffer_json_add_array_item_string(wb, TCP_STATE_2str(n->state));
        else
            buffer_json_add_array_item_string(wb, "stateless");

        buffer_json_add_array_item_uint64(wb, n->pid);

        if(!n->comm[0])
            buffer_json_add_array_item_string(wb, "[unknown]");
        else
            buffer_json_add_array_item_string(wb, n->comm);

        // buffer_json_add_array_item_string(wb, string2str(n->cmdline));

        if(n->uid == UID_UNSET) {
            // buffer_json_add_array_item_uint64(wb, n->uid);
            buffer_json_add_array_item_string(wb, "[unknown]");
        }
        else {
            // buffer_json_add_array_item_uint64(wb, n->uid);
            CACHED_USERNAME cu = cached_username_get_by_uid(n->uid);
            buffer_json_add_array_item_string(wb, string2str(cu.username));
            cached_username_release(cu);
        }

        network_viewer_add_socket_enrichment_values(
            wb,
            st->container_field_snapshot,
            st->pid_starttime_cache,
            n->pid,
            n->uid,
            n->comm[0] ? n->comm : "[unknown]");

        const struct socket_endpoint *server_endpoint;
        const char *server_address;
        const char *client_address_space;
        const char *server_address_space;
        switch (n->direction) {
            case SOCKET_DIRECTION_LISTEN:
            case SOCKET_DIRECTION_INBOUND:
            case SOCKET_DIRECTION_LOCAL_INBOUND:
                server_address = local_address;
                server_address_space = n->network_viewer.aggregated_key.local_address_space;
                client_address_space = n->network_viewer.aggregated_key.remote_address_space;
                server_endpoint = &n->local;
                break;

            case SOCKET_DIRECTION_OUTBOUND:
            case SOCKET_DIRECTION_LOCAL_OUTBOUND:
                server_address = remote_address;
                server_address_space = n->network_viewer.aggregated_key.remote_address_space;
                client_address_space = n->network_viewer.aggregated_key.local_address_space;
                server_endpoint = &n->remote;
                break;

            default:
            case SOCKET_DIRECTION_NONE:
                server_address = NULL;
                client_address_space = NULL;
                server_address_space = NULL;
                server_endpoint = NULL;
                break;
        }

        if(server_endpoint) {
            STRING *serv = system_servicenames_cache_lookup(sc, server_endpoint->port, server_endpoint->protocol);
            buffer_json_add_array_item_string(wb, string2str(serv));
        }
        else
            buffer_json_add_array_item_string(wb, "[unknown]");

        if(!aggregated) {
            buffer_json_add_array_item_string(wb, local_address);
            buffer_json_add_array_item_uint64(wb, n->local.port);
        }
        buffer_json_add_array_item_string(wb, n->network_viewer.aggregated_key.local_address_space);

        if(!aggregated) {
            buffer_json_add_array_item_string(wb, remote_address);
            buffer_json_add_array_item_uint64(wb, n->remote.port);
        }
        buffer_json_add_array_item_string(wb, n->network_viewer.aggregated_key.remote_address_space);

        if(aggregated) {
            buffer_json_add_array_item_string(wb, server_address);
        }

        buffer_json_add_array_item_uint64(wb, n->network_viewer.aggregated_key.server.port);

        if(aggregated) {
            buffer_json_add_array_item_string(wb, client_address_space);
            buffer_json_add_array_item_string(wb, server_address_space);
        }

        // buffer_json_add_array_item_uint64(wb, n->inode);
        // buffer_json_add_array_item_uint64(wb, n->net_ns_inode);

#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
        // RTT
        buffer_json_add_array_item_double(wb, (double)n->info.tcp.tcpi_rtt / (double)USEC_PER_MS);
        if(st->max.tcpi_rtt < n->info.tcp.tcpi_rtt)
            st->max.tcpi_rtt = n->info.tcp.tcpi_rtt;

        // Receiver RTT
#if defined(OS_LINUX)
        buffer_json_add_array_item_double(wb, (double)n->info.tcp.tcpi_rcv_rtt / (double)USEC_PER_MS);
        if(st->max.tcpi_rcv_rtt < n->info.tcp.tcpi_rcv_rtt)
            st->max.tcpi_rcv_rtt = n->info.tcp.tcpi_rcv_rtt;
#else
        buffer_json_add_array_item_double(wb, 0.0);
#endif

        // Retransmissions
#if defined(OS_LINUX)
        buffer_json_add_array_item_uint64(wb, n->info.tcp.tcpi_total_retrans);
        if(st->max.tcpi_total_retrans < n->info.tcp.tcpi_total_retrans)
            st->max.tcpi_total_retrans = n->info.tcp.tcpi_total_retrans;
#else
        buffer_json_add_array_item_uint64(wb, 0);
#endif
#endif

        // eBPF per-PID socket counters from ebpfgo.plugin shared memory.
        // Aggregated rows use the pre-summed accumulators built during aggregation;
        // detailed rows do a live lookup for the individual PID.
#if defined(OS_LINUX)
        {
            bool have_es;
            uint64_t ebpf_bytes_sent = 0, ebpf_bytes_received = 0;
            uint64_t ebpf_call_tcp_sent = 0, ebpf_call_tcp_received = 0;
            uint64_t ebpf_retransmit = 0, ebpf_call_udp_sent = 0, ebpf_call_udp_received = 0;
            uint64_t ebpf_call_close = 0, ebpf_call_tcp_v4 = 0, ebpf_call_tcp_v6 = 0;

            if(aggregated) {
                have_es = n->network_viewer.ebpf_valid;
                if(have_es) {
                    ebpf_bytes_sent         = n->network_viewer.ebpf_bytes_sent;
                    ebpf_bytes_received     = n->network_viewer.ebpf_bytes_received;
                    ebpf_call_tcp_sent      = n->network_viewer.ebpf_call_tcp_sent;
                    ebpf_call_tcp_received  = n->network_viewer.ebpf_call_tcp_received;
                    ebpf_retransmit         = n->network_viewer.ebpf_retransmit;
                    ebpf_call_udp_sent      = n->network_viewer.ebpf_call_udp_sent;
                    ebpf_call_udp_received  = n->network_viewer.ebpf_call_udp_received;
                    ebpf_call_close         = n->network_viewer.ebpf_call_close;
                    ebpf_call_tcp_v4        = n->network_viewer.ebpf_call_tcp_v4_conn;
                    ebpf_call_tcp_v6        = n->network_viewer.ebpf_call_tcp_v6_conn;
                }
            }
            else {
                struct ebpf_pid_stat es;
                have_es = network_viewer_ebpf_shared_memory_lookup((pid_t)n->pid, &es);
                if(have_es) {
                    /* SHM values are per-interval deltas; divide by the collection
                     * interval to produce rates/s comparable across hosts. */
                    uint32_t d = (es.socket.socket_update_every_s > 0) ?
                                 es.socket.socket_update_every_s : 1;
                    ebpf_bytes_sent         = (es.socket.bytes_sent        + d/2) / d;
                    ebpf_bytes_received     = (es.socket.bytes_received    + d/2) / d;
                    ebpf_call_tcp_sent      = (es.socket.call_tcp_sent     + d/2) / d;
                    ebpf_call_tcp_received  = (es.socket.call_tcp_received + d/2) / d;
                    ebpf_retransmit         = (es.socket.retransmit        + d/2) / d;
                    ebpf_call_udp_sent      = (es.socket.call_udp_sent     + d/2) / d;
                    ebpf_call_udp_received  = (es.socket.call_udp_received + d/2) / d;
                    ebpf_call_close         = (es.socket.call_close        + d/2) / d;
                    ebpf_call_tcp_v4        = (es.socket.call_tcp_v4_connection + d/2) / d;
                    ebpf_call_tcp_v6        = (es.socket.call_tcp_v6_connection + d/2) / d;
                }
            }
            buffer_json_add_array_item_uint64(wb, ebpf_bytes_sent);
            buffer_json_add_array_item_uint64(wb, ebpf_bytes_received);
            buffer_json_add_array_item_uint64(wb, ebpf_call_tcp_sent);
            buffer_json_add_array_item_uint64(wb, ebpf_call_tcp_received);
            buffer_json_add_array_item_uint64(wb, ebpf_retransmit);
            buffer_json_add_array_item_uint64(wb, ebpf_call_udp_sent);
            buffer_json_add_array_item_uint64(wb, ebpf_call_udp_received);
            buffer_json_add_array_item_uint64(wb, ebpf_call_close);
            buffer_json_add_array_item_uint64(wb, ebpf_call_tcp_v4);
            buffer_json_add_array_item_uint64(wb, ebpf_call_tcp_v6);
        }
#else
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
        buffer_json_add_array_item_uint64(wb, 0);
#endif

        // count
        buffer_json_add_array_item_uint64(wb, n->network_viewer.count);
    }
    buffer_json_array_close(wb);
}

static void populate_aggregated_key(const LOCAL_SOCKET *nn) {
    LOCAL_SOCKET *n = (LOCAL_SOCKET *)nn;

    n->network_viewer.count = 1;

    n->network_viewer.aggregated_key.pid = n->pid;
    n->network_viewer.aggregated_key.uid = n->uid;
    n->network_viewer.aggregated_key.direction = n->direction;
    n->network_viewer.aggregated_key.net_ns_inode = n->net_ns_inode;
    n->network_viewer.aggregated_key.state = n->state;

    switch(n->direction) {
        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
        case SOCKET_DIRECTION_LISTEN:
            n->network_viewer.aggregated_key.server = n->local;
            break;

        case SOCKET_DIRECTION_OUTBOUND:
        case SOCKET_DIRECTION_LOCAL_OUTBOUND:
            n->network_viewer.aggregated_key.server = n->remote;
            break;

        case SOCKET_DIRECTION_NONE:
            break;
    }

    n->network_viewer.aggregated_key.local_address_space = local_sockets_address_space(&n->local);
    n->network_viewer.aggregated_key.remote_address_space = local_sockets_address_space(&n->remote);
}

static void local_sockets_cb_to_json(LS_STATE *ls, const LOCAL_SOCKET *n, void *data) {
    struct sockets_stats *st = data;
    populate_aggregated_key(n);
    local_socket_to_json_array(st, n, ls->proc_self_net_ns_inode, false);
}

#define KEEP_THE_BIGGER(a, b) (a) = ((a) < (b)) ? (b) : (a)
#define KEEP_THE_SMALLER(a, b) (a) = ((a) > (b)) ? (b) : (a)
#define SUM_THEM_ALL(a, b) (a) += (b)
#define OR_THEM_ALL(a, b) (a) |= (b)

static void local_sockets_cb_to_aggregation(LS_STATE *ls __maybe_unused, const LOCAL_SOCKET *n, void *data) {
    SIMPLE_HASHTABLE_AGGREGATED_SOCKETS *ht = data;

    populate_aggregated_key(n);
    XXH64_hash_t hash = XXH3_64bits(&n->network_viewer.aggregated_key, sizeof(n->network_viewer.aggregated_key));
    SIMPLE_HASHTABLE_SLOT_AGGREGATED_SOCKETS *sl = simple_hashtable_get_slot_AGGREGATED_SOCKETS(ht, hash, (LOCAL_SOCKET *)n, true);
    LOCAL_SOCKET *t = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(t) {
        t->network_viewer.count++;

        KEEP_THE_BIGGER(t->timer, n->timer);
        KEEP_THE_BIGGER(t->retransmits, n->retransmits);
        KEEP_THE_SMALLER(t->expires, n->expires);
        KEEP_THE_BIGGER(t->rqueue, n->rqueue);
        KEEP_THE_BIGGER(t->wqueue, n->wqueue);

#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
        // The current number of consecutive retransmissions that have occurred for the most recently transmitted segment.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_retransmits, n->info.tcp.tcpi_retransmits);
#endif

        // The total number of retransmissions that have occurred for the entire connection since it was established.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_total_retrans, n->info.tcp.tcpi_total_retrans);
#endif

        // The total number of segments that have been retransmitted since the connection was established.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_retrans, n->info.tcp.tcpi_retrans);
#endif

        // The number of keepalive probes sent
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_probes, n->info.tcp.tcpi_probes);
#endif

        // The number of times the retransmission timeout has been backed off.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_backoff, n->info.tcp.tcpi_backoff);
#endif

        // A bitmask representing the TCP options currently enabled for the connection, such as SACK and Timestamps.
        OR_THEM_ALL(t->info.tcp.tcpi_options, n->info.tcp.tcpi_options);

        // The send window scale value used for this connection
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_wscale, n->info.tcp.tcpi_snd_wscale);

        // The receive window scale value used for this connection
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_wscale, n->info.tcp.tcpi_rcv_wscale);

        // Retransmission timeout in milliseconds
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rto, n->info.tcp.tcpi_rto);

        // The delayed acknowledgement timeout in milliseconds.
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_ato, n->info.tcp.tcpi_ato);
#endif

        // The maximum segment size for sending.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_mss, n->info.tcp.tcpi_snd_mss);

        // The maximum segment size for receiving.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_mss, n->info.tcp.tcpi_rcv_mss);

        // The number of unacknowledged segments
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_unacked, n->info.tcp.tcpi_unacked);
#endif

        // The number of segments that have been selectively acknowledged
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_sacked, n->info.tcp.tcpi_sacked);
#endif

        // The number of lost segments.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_lost, n->info.tcp.tcpi_lost);
#endif

        // The number of forward acknowledgment segments.
#if defined(OS_LINUX)
        SUM_THEM_ALL(t->info.tcp.tcpi_fackets, n->info.tcp.tcpi_fackets);
#endif

        // The time in milliseconds since the last data was sent.
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_data_sent, n->info.tcp.tcpi_last_data_sent);
#endif

        // The time in milliseconds since the last acknowledgment was sent (not tracked in Linux, hence often zero).
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_ack_sent, n->info.tcp.tcpi_last_ack_sent);
#endif

        // The time in milliseconds since the last data was received.
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_data_recv, n->info.tcp.tcpi_last_data_recv);
#endif

        // The time in milliseconds since the last acknowledgment was received.
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_ack_recv, n->info.tcp.tcpi_last_ack_recv);
#endif

        // The path MTU for this connection
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_pmtu, n->info.tcp.tcpi_pmtu);
#endif

        // The slow start threshold for receiving
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_ssthresh, n->info.tcp.tcpi_rcv_ssthresh);
#endif

        // The slow start threshold for sending
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_ssthresh, n->info.tcp.tcpi_snd_ssthresh);

        // The round trip time in milliseconds
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rtt, n->info.tcp.tcpi_rtt);

        // The round trip time variance in milliseconds.
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rttvar, n->info.tcp.tcpi_rttvar);

        // The size of the sending congestion window.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_cwnd, n->info.tcp.tcpi_snd_cwnd);

        // The maximum segment size that could be advertised.
#if defined(OS_LINUX)
        KEEP_THE_BIGGER(t->info.tcp.tcpi_advmss, n->info.tcp.tcpi_advmss);
#endif

        // The reordering metric
#if defined(OS_LINUX)
        KEEP_THE_SMALLER(t->info.tcp.tcpi_reordering, n->info.tcp.tcpi_reordering);
#endif

        // The receive round trip time in milliseconds.
#if defined(OS_LINUX)
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rcv_rtt, n->info.tcp.tcpi_rcv_rtt);
#endif

        // The available space in the receive buffer.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_space, n->info.tcp.tcpi_rcv_space);
#endif

        /* eBPF data is indexed by PID, and aggregated_key.pid is part of the group
         * key, so all sockets that merge here share the same PID as the group's
         * first socket; the else-branch below already looked it up. */
    }
    else {
        t = mallocz(sizeof(*t));
        memcpy(t, n, sizeof(*t));
        t->cmdline = string_dup(t->cmdline);
        simple_hashtable_set_slot_AGGREGATED_SOCKETS(ht, sl, hash, t);
#if defined(OS_LINUX)
        /* Initialise eBPF sums for the first socket in the new aggregation
         * group.  The memcpy above left network_viewer.ebpf_* unset. */
        {
            struct ebpf_pid_stat _ebpf_t;
            t->network_viewer.ebpf_valid = network_viewer_ebpf_shared_memory_lookup((pid_t)t->pid, &_ebpf_t);
            if(t->network_viewer.ebpf_valid) {
                /* SHM values are per-interval deltas; divide by the collection
                 * interval to produce rates/s comparable across hosts. */
                uint32_t d = (_ebpf_t.socket.socket_update_every_s > 0) ?
                             _ebpf_t.socket.socket_update_every_s : 1;
                t->network_viewer.ebpf_bytes_sent        = (_ebpf_t.socket.bytes_sent        + d/2) / d;
                t->network_viewer.ebpf_bytes_received    = (_ebpf_t.socket.bytes_received    + d/2) / d;
                t->network_viewer.ebpf_call_tcp_sent     = (_ebpf_t.socket.call_tcp_sent     + d/2) / d;
                t->network_viewer.ebpf_call_tcp_received = (_ebpf_t.socket.call_tcp_received + d/2) / d;
                t->network_viewer.ebpf_retransmit        = (_ebpf_t.socket.retransmit        + d/2) / d;
                t->network_viewer.ebpf_call_udp_sent     = (_ebpf_t.socket.call_udp_sent     + d/2) / d;
                t->network_viewer.ebpf_call_udp_received = (_ebpf_t.socket.call_udp_received + d/2) / d;
                t->network_viewer.ebpf_call_close        = (_ebpf_t.socket.call_close        + d/2) / d;
                t->network_viewer.ebpf_call_tcp_v4_conn  = (_ebpf_t.socket.call_tcp_v4_connection + d/2) / d;
                t->network_viewer.ebpf_call_tcp_v6_conn  = (_ebpf_t.socket.call_tcp_v6_connection + d/2) / d;
            }
            else {
                t->network_viewer.ebpf_bytes_sent        = 0;
                t->network_viewer.ebpf_bytes_received    = 0;
                t->network_viewer.ebpf_call_tcp_sent     = 0;
                t->network_viewer.ebpf_call_tcp_received = 0;
                t->network_viewer.ebpf_retransmit        = 0;
                t->network_viewer.ebpf_call_udp_sent     = 0;
                t->network_viewer.ebpf_call_udp_received = 0;
                t->network_viewer.ebpf_call_close        = 0;
                t->network_viewer.ebpf_call_tcp_v4_conn  = 0;
                t->network_viewer.ebpf_call_tcp_v6_conn  = 0;
            }
        }
#endif
    }
}







static int local_sockets_compar(const void *a, const void *b) {
    LOCAL_SOCKET *n1 = *(LOCAL_SOCKET **)a, *n2 = *(LOCAL_SOCKET **)b;
    return strcmp(n1->comm, n2->comm);
}

static BUFFER *network_viewer_result(char *function) {

    time_t now_s = now_realtime_sec();
    bool aggregated = false;

#if defined(OS_LINUX)
    network_viewer_ebpf_shared_memory_refresh();
#endif

    BUFFER *wb = nv_response_preamble();

    struct sockets_stats st = {
        .wb = wb,
    };
    st.container_field_snapshot = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(NV_TOPOLOGY_CONTAINER_FIELDS));
    st.pid_starttime_cache = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(NV_PID_STARTTIME_CACHE_ENTRY));

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_CONNECTIONS_VIEWER_HELP);

#ifdef ENABLE_DETAILED_VIEW
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "sockets");
    }
    buffer_json_array_close(wb); // accepted_params
    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "sockets");
            buffer_json_member_add_string(wb, "name", "Sockets");
            buffer_json_member_add_string(wb, "help", "Select the source type to query");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "aggregated");
                    buffer_json_member_add_string(wb, "name", "Aggregated view of sockets");
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "detailed");
                    buffer_json_member_add_string(wb, "name", "Detailed view of all sockets");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb); // options array
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb); // required_params
#endif

    char *function_copy = strdupz(function ? function : NETWORK_CONNECTIONS_VIEWER_FUNCTION);
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    for(size_t i = 1; i < num_words ;i++) {
        char *param = get_word(words, num_words, i);
        if(!param || !*param) continue;
        if(strcmp(param, "sockets:aggregated") == 0) {
            aggregated = true;
        }
        else if(strcmp(param, "sockets:detailed") == 0) {
            aggregated = false;
        }
        else if(strcmp(param, "info") == 0) {
            freez(function_copy);
            goto close_and_send;
        }
    }

    freez(function_copy);

    if(aggregated) {
        buffer_json_member_add_object(wb, "aggregated_view");
        {
            buffer_json_member_add_string(wb, "column", "Count");
            buffer_json_member_add_string(wb, "results_label", "unique combinations");
            buffer_json_member_add_string(wb, "aggregated_label", "sockets");
        }
        buffer_json_object_close(wb);
    }

    {
        buffer_json_member_add_array(wb, "data");

        LS_STATE ls = {
            .config = {
                .listening = true,
                .inbound = true,
                .outbound = true,
                .local = true,
                .tcp4 = true,
                .tcp6 = true,
                .udp4 = true,
                .udp6 = true,
                .pid = true,
                .uid = true,
                .cmdline = true,
                .comm = true,
                .namespaces = true,
                .tcp_info = true,

                .max_errors = 10,
                .max_concurrent_namespaces = 5,
            },
#if defined(LOCAL_SOCKETS_USE_SETNS)
            .spawn_server = spawn_srv,
#endif
            .stats = { 0 },
            .sockets_hashtable = { 0 },
            .local_ips_hashtable = { 0 },
            .listening_ports_hashtable = { 0 },
        };

        SIMPLE_HASHTABLE_AGGREGATED_SOCKETS ht = { 0 };
        if(aggregated) {
            simple_hashtable_init_AGGREGATED_SOCKETS(&ht, 1024);
            ls.config.cb = local_sockets_cb_to_aggregation;
            ls.config.data = &ht;
        }
        else {
            ls.config.cb = local_sockets_cb_to_json;
            ls.config.data = &st;
        }

        local_sockets_process(&ls);

        if(!aggregated) {
            st.pid_count = nv_pid_sort_unique(st.pids, st.pid_count);
            nv_apps_lookup_warm_pids(st.pids, st.pid_count);
        }

        if(aggregated) {
            size_t added = 0;
            uint64_t proc_self_net_ns_inode = ls.proc_self_net_ns_inode;

            if(ht.used) {
                LOCAL_SOCKET **array = mallocz(ht.used * sizeof(LOCAL_SOCKET *));
                for(SIMPLE_HASHTABLE_SLOT_AGGREGATED_SOCKETS *sl = simple_hashtable_first_read_only_AGGREGATED_SOCKETS(&ht);
                     sl;
                     sl = simple_hashtable_next_read_only_AGGREGATED_SOCKETS(&ht, sl)) {
                    LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
                    if(!n || added >= ht.used) continue;

                    array[added++] = n;
                }

                nv_warm_cache_from_aggregated_sockets(array, added);
                qsort(array, added, sizeof(LOCAL_SOCKET *), local_sockets_compar);

                for(size_t i = 0; i < added ;i++) {
                    local_socket_to_json_array(&st, array[i], proc_self_net_ns_inode, true);
                    string_freez(array[i]->cmdline);
                    freez(array[i]);
                }

                freez(array);
            }

            simple_hashtable_destroy_AGGREGATED_SOCKETS(&ht);
        }

        freez(st.pids);
        if(st.container_field_snapshot) {
            dictionary_destroy(st.container_field_snapshot);
            st.container_field_snapshot = NULL;
        }
        if(st.pid_starttime_cache) {
            dictionary_destroy(st.pid_starttime_cache);
            st.pid_starttime_cache = NULL;
        }

        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "columns");
        {
            size_t field_id = 0;

            // Direction
            buffer_rrdf_table_add_field(wb, field_id++, "Direction", "Socket Direction",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE|RRDF_FIELD_OPTS_STICKY,
                                        NULL);

            // Protocol
            buffer_rrdf_table_add_field(wb, field_id++, "Protocol", "Socket Protocol",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // Type
            buffer_rrdf_table_add_field(wb, field_id++, "Namespace", "Namespace",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // State
            buffer_rrdf_table_add_field(wb, field_id++, "State", "Socket State",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // Pid
            buffer_rrdf_table_add_field(wb, field_id++, "PID", "Process ID",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // Comm
            buffer_rrdf_table_add_field(wb, field_id++, "Process", "Process Name",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE|RRDF_FIELD_OPTS_FULL_WIDTH,
                                        NULL);

//            // Cmdline
//            buffer_rrdf_table_add_field(wb, field_id++, "CommandLine", "Command Line",
//                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
//                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
//                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
//                                        RRDF_FIELD_OPTS_NONE|RRDF_FIELD_OPTS_FULL_WIDTH,
//                                        NULL);

//            // Uid
//            buffer_rrdf_table_add_field(wb, field_id++, "UID", "User ID",
//                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
//                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
//                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
//                                        RRDF_FIELD_OPTS_NONE,
//                                        NULL);

            // Username
            buffer_rrdf_table_add_field(wb, field_id++, "User", "Username",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            cgroup_topology_emit_rrdf_table_fields(wb, &field_id, false);

            // Portname
            buffer_rrdf_table_add_field(wb, field_id++, "Portname", "Server Port Name",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            if(!aggregated) {
                // Local Address
                buffer_rrdf_table_add_field(wb, field_id++, "LocalIP", "Local IP Address",
                                            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                            RRDF_FIELD_OPTS_VISIBLE|RRDF_FIELD_OPTS_FULL_WIDTH,
                                            NULL);

                // Local Port
                buffer_rrdf_table_add_field(wb, field_id++, "LocalPort", "Local Port",
                                            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                            RRDF_FIELD_OPTS_VISIBLE,
                                            NULL);
            }

            // Local Address Space
            buffer_rrdf_table_add_field(wb, field_id++, "LocalAddressSpace", "Local IP Address Space",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_NONE,
                                        NULL);

            if(!aggregated) {
                // Remote Address
                buffer_rrdf_table_add_field(wb, field_id++, "RemoteIP", "Remote IP Address",
                                            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                            RRDF_FIELD_OPTS_VISIBLE|RRDF_FIELD_OPTS_FULL_WIDTH,
                                            NULL);

                // Remote Port
                buffer_rrdf_table_add_field(wb, field_id++, "RemotePort", "Remote Port",
                                            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                            RRDF_FIELD_OPTS_VISIBLE,
                                            NULL);
            }

            // Remote Address Space
            buffer_rrdf_table_add_field(wb, field_id++, "RemoteAddressSpace", "Remote IP Address Space",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_NONE,
                                        NULL);

            if(aggregated) {
                // Server IP
                buffer_rrdf_table_add_field(wb, field_id++, "ServerIP", "Server IP Address",
                                            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                            RRDF_FIELD_OPTS_FULL_WIDTH | (aggregated ? RRDF_FIELD_OPTS_VISIBLE : RRDF_FIELD_OPTS_NONE),
                                            NULL);
            }

            // Server Port
            buffer_rrdf_table_add_field(wb, field_id++, "ServerPort", "Server Port",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                        aggregated ? RRDF_FIELD_OPTS_VISIBLE : RRDF_FIELD_OPTS_NONE,
                                        NULL);

            if(aggregated) {
                // Client Address Space
                buffer_rrdf_table_add_field(wb, field_id++, "ClientAddressSpace", "Client IP Address Space",
                                            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                            RRDF_FIELD_OPTS_VISIBLE,
                                            NULL);

                // Server Address Space
                buffer_rrdf_table_add_field(wb, field_id++, "ServerAddressSpace", "Server IP Address Space",
                                            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                            RRDF_FIELD_OPTS_VISIBLE,
                                            NULL);
            }

//            // inode
//            buffer_rrdf_table_add_field(wb, field_id++, "Inode", "Socket Inode",
//                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
//                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
//                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
//                                        RRDF_FIELD_OPTS_NONE,
//                                        NULL);

//            // Namespace inode
//            buffer_rrdf_table_add_field(wb, field_id++, "Namespace Inode", "Namespace Inode",
//                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
//                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
//                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
//                                        RRDF_FIELD_OPTS_NONE,
//                                        NULL);

#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
            // RTT
            buffer_rrdf_table_add_field(wb, field_id++, "RTT", aggregated ? "Max Smoothed Round Trip Time" : "Smoothed Round Trip Time",
                                        RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                        2, "ms", st.max.tcpi_rtt / USEC_PER_MS, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // Asymmetry RTT
            buffer_rrdf_table_add_field(wb, field_id++, "RecvRTT", aggregated ? "Max Receiver ACKs RTT" : "Receiver ACKs RTT",
                                        RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                        2, "ms", st.max.tcpi_rcv_rtt / USEC_PER_MS, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            // Rentrasmissions
            buffer_rrdf_table_add_field(wb, field_id++, "Retrans", "Total Retransmissions",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, "packets", st.max.tcpi_total_retrans, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);
#endif

            // eBPF per-PID socket counters (populated on Linux when ebpfgo.plugin is running)
#define NV_EBPF_FIELD(id, label, unit)                                             \
            buffer_rrdf_table_add_field(wb, field_id++, id, label,                \
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,                  \
                RRDF_FIELD_TRANSFORM_NONE, 0, unit, NAN,                           \
                RRDF_FIELD_SORT_DESCENDING, NULL,                                  \
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,                   \
                RRDF_FIELD_OPTS_NONE, NULL)
            NV_EBPF_FIELD("eBPFBytesSent",  "eBPF TCP Bytes Sent by PID",       "bytes/s");
            NV_EBPF_FIELD("eBPFBytesRecv",  "eBPF TCP Bytes Received by PID",   "bytes/s");
            NV_EBPF_FIELD("eBPFTCPSent",    "eBPF TCP Send Calls by PID",       "calls/s");
            NV_EBPF_FIELD("eBPFTCPRecv",    "eBPF TCP Receive Calls by PID",    "calls/s");
            NV_EBPF_FIELD("eBPFRetransmit", "eBPF TCP Retransmissions by PID",  "calls/s");
            NV_EBPF_FIELD("eBPFUDPSent",    "eBPF UDP Send Calls by PID",       "calls/s");
            NV_EBPF_FIELD("eBPFUDPRecv",    "eBPF UDP Receive Calls by PID",    "calls/s");
            NV_EBPF_FIELD("eBPFClose",      "eBPF TCP Close Calls by PID",      "calls/s");
            NV_EBPF_FIELD("eBPFConnV4",     "eBPF IPv4 TCP Connections by PID", "connections/s");
            NV_EBPF_FIELD("eBPFConnV6",     "eBPF IPv6 TCP Connections by PID", "connections/s");
#undef NV_EBPF_FIELD

            // Count
            buffer_rrdf_table_add_field(wb, field_id++, "Count", "Number of sockets like this",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, "sockets", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                                        aggregated ? (RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY) : RRDF_FIELD_OPTS_NONE,
                                        NULL);
        }

        buffer_json_object_close(wb); // columns
        buffer_json_member_add_string(wb, "default_sort_column", aggregated ? "Count" : "Direction");

        buffer_json_member_add_object(wb, "custom_charts");
        {
            buffer_json_member_add_object(wb, "Network Map");
            {
                buffer_json_member_add_string(wb, "type", "network-viewer");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb); // custom_charts

        buffer_json_member_add_object(wb, "charts");
        {
#define NV_STACKED_BAR_CHART(name_, col_)                                          \
            buffer_json_member_add_object(wb, name_);                              \
            {                                                                      \
                buffer_json_member_add_string(wb, "type", "stacked-bar");          \
                buffer_json_member_add_array(wb, "columns");                       \
                { buffer_json_add_array_item_string(wb, col_); }                   \
                buffer_json_array_close(wb);                                       \
            }                                                                      \
            buffer_json_object_close(wb)
            NV_STACKED_BAR_CHART("Count by Direction", "Direction");
            NV_STACKED_BAR_CHART("Count by Process",   "Process");
            NV_STACKED_BAR_CHART("Count by Protocol",  "Protocol");
#undef NV_STACKED_BAR_CHART
        }
        buffer_json_object_close(wb); // charts

        buffer_json_member_add_array(wb, "default_charts");
        {
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, "Count");
            buffer_json_add_array_item_string(wb, "Direction");
            buffer_json_array_close(wb);

            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, "Count");
            buffer_json_add_array_item_string(wb, "Process");
            buffer_json_array_close(wb);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "group_by");
        {
#define NV_GROUP_BY_SINGLE(key_, display_name_)                                    \
            buffer_json_member_add_object(wb, key_);                               \
            {                                                                      \
                buffer_json_member_add_string(wb, "name", display_name_);          \
                buffer_json_member_add_array(wb, "columns");                       \
                { buffer_json_add_array_item_string(wb, key_); }                   \
                buffer_json_array_close(wb);                                       \
            }                                                                      \
            buffer_json_object_close(wb)
            NV_GROUP_BY_SINGLE("Direction", "Direction");
            NV_GROUP_BY_SINGLE("Protocol",  "Protocol");
            NV_GROUP_BY_SINGLE("Namespace", "Namespace");
            NV_GROUP_BY_SINGLE("Process",   "Process");
            if(!aggregated) {
                NV_GROUP_BY_SINGLE("LocalIP",    "Local IP");
                NV_GROUP_BY_SINGLE("LocalPort",  "Local Port");
                NV_GROUP_BY_SINGLE("RemoteIP",   "Remote IP");
                NV_GROUP_BY_SINGLE("RemotePort", "Remote Port");
            }
#undef NV_GROUP_BY_SINGLE
        }
        buffer_json_object_close(wb); // group_by
    }

close_and_send:
    if(st.container_field_snapshot)
        dictionary_destroy(st.container_field_snapshot);
    if(st.pid_starttime_cache)
        dictionary_destroy(st.pid_starttime_cache);
    network_viewer_finalize_response_buffer(wb, now_s, NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    return wb;
}

void network_viewer_function(
    const char *transaction, char *function,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {

    NV_DISPATCH(network_viewer_result(function));
}

// ----------------------------------------------------------------------------------------------------------------
// Linux: dns-queries function (data from ebpf-go.plugin via shared memory)

#if defined(OS_LINUX)

/* DNS query type name (QTYPE) — most common types for fast lookup. */
static const char *nv_dns_qtype_name(uint16_t qtype)
{
    switch (qtype) {
    case   1: return "A";
    case   2: return "NS";
    case   5: return "CNAME";
    case   6: return "SOA";
    case  12: return "PTR";
    case  15: return "MX";
    case  16: return "TXT";
    case  28: return "AAAA";
    case  33: return "SRV";
    case  65: return "HTTPS";
    case 255: return "ANY";
    default:  return NULL;  /* caller formats numeric string */
    }
}

/* DNS RCODE name — RFC 2929 / RFC 6895. */
static const char *nv_dns_rcode_name(uint16_t rcode)
{
    switch (rcode) {
    case 0: return "NOERROR";
    case 1: return "FORMERR";
    case 2: return "SERVFAIL";
    case 3: return "NXDOMAIN";
    case 4: return "NOTIMP";
    case 5: return "REFUSED";
    case 6: return "YXDOMAIN";
    case 7: return "YXRRSET";
    case 8: return "NXRRSET";
    case 9: return "NOTAUTH";
    default: return NULL;   /* caller formats numeric string */
    }
}

/* MUST equal DNS_FLOW_TTL_US (20 s) so consecutive NV snapshots cover
 * non-overlapping time windows — each DNS query counted exactly once.
 * The ebpf-go publisher runs at half this interval (dnsDefaultUpdateEvery=10)
 * to keep SHM fresh mid-window; that cadence does not affect snapshot overlap. */
#define NV_DNS_UPDATE_EVERY 20

static BUFFER *network_viewer_dns_result(void)
{
    struct ebpfgo_dns_shared *dns = mallocz(sizeof(*dns));
    if (!network_viewer_dns_shared_memory_snapshot(dns)) {
        freez(dns);
        return network_viewer_json_error_response(
            HTTP_RESP_SERVICE_UNAVAILABLE,
            "dns-queries: data not yet available (enable dns = yes in ebpf.d.conf)");
    }

    uint32_t flow_count = dns->hdr.live_count;
    if (flow_count > NETDATA_EBPFGO_DNS_FLOW_RING_CAP)
        flow_count = NETDATA_EBPFGO_DNS_FLOW_RING_CAP;

    time_t now_s = now_realtime_sec();
    BUFFER *wb = nv_response_preamble();

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NV_DNS_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_DNS_QUERIES_FUNCTION_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        char ip_buf[INET6_ADDRSTRLEN];
        char num_buf[32];

        for (uint32_t i = 0; i < flow_count; i++) {
            const struct ebpfgo_dns_flow_record *r = &dns->ring[i];

            /* Format server IP.  inet_ntop returns NULL on malformed input
             * (e.g. ip_version lies about the family); fall back to a safe
             * placeholder so ip_buf is never passed uninitialised to the
             * JSON writer. */
            const char *ip_str;
            if (r->ip_version == 4) {
                ip_str = inet_ntop(AF_INET, &r->server_ip[0], ip_buf, sizeof(ip_buf));
            } else {
                ip_str = inet_ntop(AF_INET6, r->server_ip, ip_buf, sizeof(ip_buf));
            }
            if (!ip_str)
                ip_str = "(invalid)";

            /* Format query type */
            const char *qtype_name = nv_dns_qtype_name(r->query_type);
            if (!qtype_name) {
                snprintfz(num_buf, sizeof(num_buf), "%u", (unsigned)r->query_type);
                qtype_name = num_buf;
            }

            /* Format rcode */
            const char *rcode_name = nv_dns_rcode_name(r->rcode);
            char rcode_buf[16];
            if (!rcode_name) {
                snprintfz(rcode_buf, sizeof(rcode_buf), "%u", (unsigned)r->rcode);
                rcode_name = rcode_buf;
            }

            char domain_buf[NETDATA_EBPFGO_DNS_DOMAIN_MAX];
            memcpy(domain_buf, r->domain, sizeof(domain_buf));
            domain_buf[sizeof(domain_buf) - 1] = '\0';

            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, domain_buf[0] ? domain_buf : "(unknown)");
            buffer_json_add_array_item_string(wb, qtype_name);
            buffer_json_add_array_item_string(wb, (r->protocol == 17) ? "UDP" : "TCP");
            buffer_json_add_array_item_string(wb, (r->ip_version == 4) ? "IPv4" : "IPv6");
            buffer_json_add_array_item_string(wb, ip_str);
            buffer_json_add_array_item_double(wb, (double)r->latency_us / 1e6);
            buffer_json_add_array_item_string(wb, r->timed_out ? "Timeout" : "OK");
            buffer_json_add_array_item_string(wb, rcode_name);
            /* Synthetic columns for chart: 1 query per row; responses = non-timed-out */
            buffer_json_add_array_item_uint64(wb, 1);
            buffer_json_add_array_item_uint64(wb, r->timed_out ? 0 : 1);
            buffer_json_array_close(wb);
        }
    }
    buffer_json_array_close(wb); // data

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        buffer_rrdf_table_add_field(wb, field_id++, "Domain", "Queried Domain Name",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_FACET,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "QueryType", "DNS Query Type",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Transport", "Transport Protocol",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "IPFamily", "IP Protocol Family",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ServerIP", "DNS Server IP Address",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Latency", "Query-to-Response Latency",
            RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
            0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_MAX,
            RRDF_FIELD_FILTER_RANGE,
            RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Status", "Query Completion Status",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "RCode", "DNS Response Code",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE, NULL);

        /* Synthetic columns used by the aggregate chart only — hidden from table view. */
        buffer_rrdf_table_add_field(wb, field_id++, "Queries", "DNS Queries (count)",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "queries", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Responses", "DNS Responses (count)",
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
            0, "responses", NAN, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Latency");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Queries");
                buffer_json_add_array_item_string(wb, "Responses");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb); // Traffic
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
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

        buffer_json_member_add_object(wb, "IPFamily");
        {
            buffer_json_member_add_string(wb, "name", "IP Family");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "IPFamily");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Status");
        {
            buffer_json_member_add_string(wb, "name", "Status");
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Status");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    network_viewer_finalize_response_buffer(wb, now_s, NV_DNS_UPDATE_EVERY);
    freez(dns);
    return wb;
}

static void network_viewer_dns_function(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    NV_DISPATCH(network_viewer_dns_result());
}

#endif /* OS_LINUX */

// ----------------------------------------------------------------------------------------------------------------
// FreeBSD/macOS: network-protocols function

#if defined(OS_FREEBSD) || defined(OS_MACOS)

typedef struct {
    struct tcpstat tcp;
    struct udpstat udp;
    usec_t         last_ut;
    bool           initialized;
} NV_PROTO_STATE;

static NV_PROTO_STATE nv_proto_prev = { .initialized = false };

static uint64_t nv_proto_delta(uint64_t cur, uint64_t prev, double elapsed_s) {
    if (elapsed_s <= 0.0 || cur < prev)
        return 0;
    return (uint64_t)((double)(cur - prev) / elapsed_s + 0.5);
}

#if defined(OS_FREEBSD)
static uint64_t nv_proto_count_established(void) {
    uint64_t tcp_states[TCP_NSTATES] = { 0 };
    size_t len = sizeof(tcp_states);
    if(sysctlbyname("net.inet.tcp.states", tcp_states, &len, NULL, 0) == 0)
        return tcp_states[TCPS_ESTABLISHED];

    return 0;
}
#elif defined(OS_MACOS)
struct nv_proto_established_count {
    uint64_t established;
};

static void nv_proto_established_count_cb(LS_STATE *ls __maybe_unused, const LOCAL_SOCKET *n, void *data) {
    struct nv_proto_established_count *count = data;

    if(n && n->local.protocol == IPPROTO_TCP && n->state == TCP_ESTABLISHED)
        count->established++;
}

static uint64_t nv_proto_count_established(void) {
    struct nv_proto_established_count count = { 0 };
    LS_STATE ls = {
        .config = {
            .listening = true,
            .inbound = true,
            .outbound = true,
            .local = true,
            .tcp4 = true,
            .tcp6 = true,
            .udp4 = false,
            .udp6 = false,
            .pid = false,
            .uid = false,
            .cmdline = false,
            .comm = false,
            .namespaces = false,
            .tcp_info = false,
            .max_errors = 0,
            .max_concurrent_namespaces = 0,
            .cb = nv_proto_established_count_cb,
            .data = &count,
        },
    };

    local_sockets_process(&ls);
    return count.established;
}

typedef struct {
    uint64_t established;
    usec_t last_ut;
} NV_PROTO_ESTABLISHED_CACHE;

static NV_PROTO_ESTABLISHED_CACHE nv_proto_established_cache = { 0 };

// Called with nv_proto_mutex held; the cache shares nv_proto_prev's sampling window.
static uint64_t nv_proto_count_established_cached(usec_t now_ut) {
    const usec_t cache_ttl_ut = (usec_t)NETWORK_VIEWER_RESPONSE_UPDATE_EVERY * USEC_PER_SEC;

    if(nv_proto_established_cache.last_ut && now_ut - nv_proto_established_cache.last_ut < cache_ttl_ut)
        return nv_proto_established_cache.established;

    // macOS has no cheap net.inet.tcp.states equivalent; avoid repeated full FD walks in one response window.
    nv_proto_established_cache.established = nv_proto_count_established();
    nv_proto_established_cache.last_ut = now_ut;
    return nv_proto_established_cache.established;
}
#endif

static BUFFER *network_protocols_result(void) {
    // Sampling and delta computation must be atomic: acquiring the mutex first
    // prevents two concurrent requests from each sampling stale counters and
    // then computing deltas against the same nv_proto_prev with a near-zero
    // elapsed time, which would produce wildly inflated per-second rates.
    netdata_mutex_lock(&nv_proto_mutex);

    struct tcpstat tcp_cur = { 0 };
    struct udpstat udp_cur = { 0 };
    size_t len;

    len = sizeof(tcp_cur);
    if (sysctlbyname("net.inet.tcp.stats", &tcp_cur, &len, NULL, 0) < 0) {
        netdata_mutex_unlock(&nv_proto_mutex);
        return network_viewer_json_error_response(HTTP_RESP_INTERNAL_SERVER_ERROR, "failed to read net.inet.tcp.stats");
    }

    len = sizeof(udp_cur);
    if (sysctlbyname("net.inet.udp.stats", &udp_cur, &len, NULL, 0) < 0) {
        netdata_mutex_unlock(&nv_proto_mutex);
        return network_viewer_json_error_response(HTTP_RESP_INTERNAL_SERVER_ERROR, "failed to read net.inet.udp.stats");
    }

    usec_t now_ut = now_monotonic_usec();

    bool first = !nv_proto_prev.initialized;
    double elapsed_s = first ? 0.0 : (double)(now_ut - nv_proto_prev.last_ut) / (double)USEC_PER_SEC;

#define TCP_DELTA(f) nv_proto_delta((uint64_t)tcp_cur.f, (uint64_t)nv_proto_prev.tcp.f, elapsed_s)
#define UDP_DELTA(f) nv_proto_delta((uint64_t)udp_cur.f, (uint64_t)nv_proto_prev.udp.f, elapsed_s)

    uint64_t tcp_received   = first ? 0 : TCP_DELTA(tcps_rcvtotal);
    uint64_t tcp_sent       = first ? 0 : TCP_DELTA(tcps_sndtotal);
    uint64_t tcp_errors     = first ? 0 : TCP_DELTA(tcps_conndrops);
    uint64_t tcp_active     = first ? 0 : TCP_DELTA(tcps_connattempt);
    uint64_t tcp_passive    = first ? 0 : TCP_DELTA(tcps_accepts);
    uint64_t tcp_resets     = first ? 0 : TCP_DELTA(tcps_drops);
    uint64_t tcp_segs_total = first ? 0 : tcp_received + tcp_sent;
    uint64_t tcp_retrans    = first ? 0 : TCP_DELTA(tcps_sndrexmitpack);

    uint64_t udp_received   = first ? 0 : UDP_DELTA(udps_ipackets);
    uint64_t udp_sent       = first ? 0 : UDP_DELTA(udps_opackets);
    uint64_t udp_errors     = first ? 0 : (
                                  UDP_DELTA(udps_hdrops) +
                                  UDP_DELTA(udps_badlen) +
                                  UDP_DELTA(udps_badsum) +
                                  UDP_DELTA(udps_nosum));
    uint64_t udp_no_port    = first ? 0 : UDP_DELTA(udps_noport);

#undef TCP_DELTA
#undef UDP_DELTA

    nv_proto_prev.tcp         = tcp_cur;
    nv_proto_prev.udp         = udp_cur;
    nv_proto_prev.last_ut     = now_ut;
    nv_proto_prev.initialized = true;

#if defined(OS_MACOS)
    uint64_t established = nv_proto_count_established_cached(now_ut);
#else
    uint64_t established = nv_proto_count_established();
#endif

    netdata_mutex_unlock(&nv_proto_mutex);

    time_t now_s = now_realtime_sec();
    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_PROTOCOLS_FUNCTION_HELP);

    buffer_json_member_add_array(wb, "data");
    {
        // TCP row: OS counters cover both IPv4 and IPv6 in a single counter set.
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, "TCP");
            buffer_json_add_array_item_string(wb, "IPv4+IPv6");
            buffer_json_add_array_item_uint64(wb, tcp_received);
            buffer_json_add_array_item_uint64(wb, tcp_sent);
            buffer_json_add_array_item_uint64(wb, tcp_errors);
            buffer_json_add_array_item_uint64(wb, tcp_active);
            buffer_json_add_array_item_uint64(wb, established);
            buffer_json_add_array_item_uint64(wb, tcp_passive);
            buffer_json_add_array_item_uint64(wb, tcp_resets);
            buffer_json_add_array_item_uint64(wb, tcp_segs_total);
            buffer_json_add_array_item_uint64(wb, tcp_retrans);
            buffer_json_add_array_item_uint64(wb, 0); // DatagramsNoPort — UDP only
        }
        buffer_json_array_close(wb);

        // UDP row
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, "UDP");
            buffer_json_add_array_item_string(wb, "IPv4+IPv6");
            buffer_json_add_array_item_uint64(wb, udp_received);
            buffer_json_add_array_item_uint64(wb, udp_sent);
            buffer_json_add_array_item_uint64(wb, udp_errors);
            buffer_json_add_array_item_uint64(wb, 0); // ConnActive        — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnEstablished   — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnPassive       — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // ConnReset         — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // SegsTotal         — TCP only
            buffer_json_add_array_item_uint64(wb, 0); // SegsRetransmitted — TCP only
            buffer_json_add_array_item_uint64(wb, udp_no_port);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb); // data

    size_t field_id = 0;
    buffer_json_member_add_object(wb, "columns");
    {
        buffer_rrdf_table_add_field(wb, field_id++, "Transport", "Transport Protocol",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Family", "IP Protocol Family",
            RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

#define NV_INT_FIELD(id, label, unit)                                              \
        buffer_rrdf_table_add_field(wb, field_id++, id, label,                    \
            RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,                     \
            RRDF_FIELD_TRANSFORM_NUMBER, 0, unit, NAN,                            \
            RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,             \
            RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_VISIBLE, NULL)

        NV_INT_FIELD("Received",          "Received (Segments/Datagrams)",     "segments/datagrams/s");
        NV_INT_FIELD("Sent",              "Sent (Segments/Datagrams)",          "segments/datagrams/s");
        NV_INT_FIELD("Errors",            "Errors (Failures/Rx Errors)",        "errors");
        NV_INT_FIELD("ConnActive",        "Active Connections Opened",          "opens");
        NV_INT_FIELD("ConnEstablished",   "Currently Established Connections",  "connections");
        NV_INT_FIELD("ConnPassive",       "Passive Connections Opened",         "opens");
        NV_INT_FIELD("ConnReset",         "Reset Connections",                  "resets");
        NV_INT_FIELD("SegsTotal",         "Total Segments",                     "segments/s");
        NV_INT_FIELD("SegsRetransmitted", "Retransmitted Segments",             "segments/s");
        NV_INT_FIELD("DatagramsNoPort",   "Datagrams with No Port",             "datagrams/s");
#undef NV_INT_FIELD
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Received");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Received");
                buffer_json_add_array_item_string(wb, "Sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
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
    }
    buffer_json_object_close(wb); // group_by

    network_viewer_finalize_response_buffer(wb, now_s, NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    return wb;
}

void function_network_protocols(
    const char *transaction, char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused)
{
    BUFFER *wb = network_protocols_result();
    network_viewer_emit_response(transaction, wb);
    buffer_free(wb);
}

#endif // OS_FREEBSD || OS_MACOS

// ----------------------------------------------------------------------------------------------------------------
// test CLI

struct network_viewer_test_command {
    bool enabled;
    const char *function_name;
    uint64_t timeout_seconds;
    bool timeout_seconds_set;
};

static void network_viewer_test_usage(FILE *stream)
{
    fprintf(
        stream,
        "usage: network-viewer.plugin --test <network-connections|topology:network-connections|network-protocols> [--timeout <seconds>] < payload.json\n"
        "       network-protocols does not read a payload from stdin\n");
}

static bool network_viewer_test_option_present(int argc, char **argv)
{
    for(int i = 1; i < argc; i++) {
        if(strcmp(argv[i], "--test") == 0 || strncmp(argv[i], "--test=", strlen("--test=")) == 0)
            return true;
    }

    return false;
}

static int network_viewer_set_required_option_once(const char **slot, const char *value, const char *option)
{
    if(*slot) {
        fprintf(stderr, "duplicate %s\n", option);
        network_viewer_test_usage(stderr);
        return 2;
    }

    if(!value || !*value) {
        fprintf(stderr, "missing value for %s\n", option);
        network_viewer_test_usage(stderr);
        return 2;
    }

    *slot = value;
    return 0;
}

static int network_viewer_set_timeout_option_once(uint64_t *slot, bool *slot_set, const char *value)
{
    if(*slot_set) {
        fprintf(stderr, "duplicate --timeout\n");
        network_viewer_test_usage(stderr);
        return 2;
    }

    if(!value || !*value) {
        fprintf(stderr, "missing value for --timeout\n");
        network_viewer_test_usage(stderr);
        return 2;
    }

    for(const char *s = value; *s; s++) {
        if(*s < '0' || *s > '9') {
            fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
            network_viewer_test_usage(stderr);
            return 2;
        }
    }

    errno = 0;
    unsigned long long parsed = strtoull(value, NULL, 10);
    if(errno == ERANGE) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        network_viewer_test_usage(stderr);
        return 2;
    }

#if ULLONG_MAX > UINT64_MAX
    if(parsed > UINT64_MAX) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        network_viewer_test_usage(stderr);
        return 2;
    }
#endif

    *slot = (uint64_t)parsed;
    *slot_set = true;
    return 0;
}

static int network_viewer_reject_request_option(void)
{
    fprintf(stderr, "--request is no longer supported; pass the request payload on stdin\n");
    network_viewer_test_usage(stderr);
    return 2;
}

static int parse_network_viewer_test_command(int argc, char **argv, struct network_viewer_test_command *cmd)
{
    *cmd = (struct network_viewer_test_command){ 0 };
    if(!network_viewer_test_option_present(argc, argv))
        return 0;

    cmd->enabled = true;

    for(int i = 1; i < argc; i++) {
        const char *arg = argv[i];

        if(strcmp(arg, "--test") == 0) {
            if(++i >= argc)
                return network_viewer_set_required_option_once(&cmd->function_name, NULL, "--test");

            int rc = network_viewer_set_required_option_once(&cmd->function_name, argv[i], "--test");
            if(rc)
                return rc;
        }
        else if(strncmp(arg, "--test=", strlen("--test=")) == 0) {
            int rc = network_viewer_set_required_option_once(&cmd->function_name, arg + strlen("--test="), "--test");
            if(rc)
                return rc;
        }
        else if(strcmp(arg, "--request") == 0 || strncmp(arg, "--request=", strlen("--request=")) == 0) {
            return network_viewer_reject_request_option();
        }
        else if(strcmp(arg, "--timeout") == 0) {
            if(++i >= argc)
                return network_viewer_set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, NULL);

            int rc = network_viewer_set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, argv[i]);
            if(rc)
                return rc;
        }
        else if(strncmp(arg, "--timeout=", strlen("--timeout=")) == 0) {
            int rc = network_viewer_set_timeout_option_once(
                &cmd->timeout_seconds, &cmd->timeout_seconds_set, arg + strlen("--timeout="));
            if(rc)
                return rc;
        }
        else if(strcmp(arg, "-h") == 0 || strcmp(arg, "--help") == 0) {
            network_viewer_test_usage(stderr);
            return 2;
        }
        else {
            fprintf(stderr, "unsupported network-viewer test option '%s'\n", arg);
            network_viewer_test_usage(stderr);
            return 2;
        }
    }

    if(!cmd->function_name) {
        fprintf(stderr, "missing required --test\n");
        network_viewer_test_usage(stderr);
        return 2;
    }

    if(!cmd->timeout_seconds_set)
        cmd->timeout_seconds = NETWORK_VIEWER_TEST_DEFAULT_TIMEOUT_SECONDS;

    return 0;
}

static bool network_viewer_test_function_matches(const char *function, const char *expected, size_t expected_len)
{
    return function && expected && expected_len &&
           strncmp(function, expected, expected_len) == 0 &&
           (function[expected_len] == '\0' || isspace((unsigned char)function[expected_len]));
}

static bool network_viewer_test_function_supported(const char *function)
{
    return network_viewer_test_function_matches(
               function, NETWORK_CONNECTIONS_VIEWER_FUNCTION, sizeof(NETWORK_CONNECTIONS_VIEWER_FUNCTION) - 1) ||
           network_viewer_test_function_matches(
               function, NETWORK_TOPOLOGY_VIEWER_FUNCTION, sizeof(NETWORK_TOPOLOGY_VIEWER_FUNCTION) - 1)
#if defined(OS_FREEBSD) || defined(OS_MACOS)
           || network_viewer_test_function_matches(
               function, NETWORK_PROTOCOLS_FUNCTION, sizeof(NETWORK_PROTOCOLS_FUNCTION) - 1)
#endif
           ;
}

static uint64_t network_viewer_effective_test_timeout_seconds(uint64_t timeout_seconds)
{
    return timeout_seconds ? timeout_seconds : NETWORK_VIEWER_TEST_TIMEOUT_DISABLED_SECONDS;
}

static usec_t network_viewer_test_stop_monotonic_usec(uint64_t timeout_seconds)
{
    usec_t now_ut = now_monotonic_usec();
    uint64_t effective_timeout_seconds = network_viewer_effective_test_timeout_seconds(timeout_seconds);
    uint64_t max_timeout_seconds = (UINT64_MAX - now_ut) / USEC_PER_SEC;

    if(effective_timeout_seconds > max_timeout_seconds)
        return UINT64_MAX;

    return now_ut + effective_timeout_seconds * USEC_PER_SEC;
}

static BUFFER *network_viewer_read_request_payload_from_stdin(void)
{
    BUFFER *payload = buffer_create(8192, NULL);
    size_t total = 0;

    while(true) {
        char buffer[8192];
        ssize_t bytes_read = read(STDIN_FILENO, buffer, sizeof(buffer));
        if(bytes_read == -1) {
            if(errno == EINTR)
                continue;

            fprintf(stderr, "failed to read request payload from stdin: %s\n", strerror(errno));
            buffer_free(payload);
            return NULL;
        }

        if(bytes_read == 0)
            break;

        if((uint64_t)total + (uint64_t)bytes_read > NETWORK_VIEWER_TEST_MAX_REQUEST_BYTES) {
            fprintf(
                stderr,
                "request payload from stdin is too large: max %llu bytes\n",
                (unsigned long long)NETWORK_VIEWER_TEST_MAX_REQUEST_BYTES);
            buffer_free(payload);
            return NULL;
        }

        buffer_memcat(payload, buffer, (size_t)bytes_read);
        total += (size_t)bytes_read;
    }

    if(total == 0) {
        fprintf(stderr, "request payload from stdin is empty\n");
        buffer_free(payload);
        return NULL;
    }

    payload->content_type = CT_APPLICATION_JSON;
    return payload;
}

static int network_viewer_write_test_result(BUFFER *result)
{
    if(!result) {
        fprintf(stderr, "network-viewer test function returned no result\n");
        return 1;
    }

    if(buffer_strlen(result))
        fwrite(buffer_tostring(result), buffer_strlen(result), 1, stdout);
    fprintf(stdout, "\n");
    fflush(stdout);

    return (result->response_code >= HTTP_RESP_OK && result->response_code < 300) ? 0 : 1;
}

static int run_network_viewer_test_command(const struct network_viewer_test_command *cmd)
{
    if(!network_viewer_test_function_supported(cmd->function_name)) {
        fprintf(
            stderr,
            "unsupported network-viewer test function '%s' (expected '%s', '%s'"
#if defined(OS_FREEBSD) || defined(OS_MACOS)
            ", or '%s'"
#endif
            ")\n",
            cmd->function_name,
            NETWORK_CONNECTIONS_VIEWER_FUNCTION,
            NETWORK_TOPOLOGY_VIEWER_FUNCTION
#if defined(OS_FREEBSD) || defined(OS_MACOS)
            , NETWORK_PROTOCOLS_FUNCTION
#endif
            );
        return 2;
    }

    bool cancelled = false;
    usec_t stop_monotonic_ut = network_viewer_test_stop_monotonic_usec(cmd->timeout_seconds);
    char *function = strdupz(cmd->function_name);

    BUFFER *result;
#if defined(OS_FREEBSD) || defined(OS_MACOS)
    if(network_viewer_test_function_matches(
           function,
           NETWORK_PROTOCOLS_FUNCTION,
           sizeof(NETWORK_PROTOCOLS_FUNCTION) - 1))
        result = network_protocols_result();
    else
#endif
    if(network_viewer_test_function_matches(
           function,
           NETWORK_TOPOLOGY_VIEWER_FUNCTION,
           sizeof(NETWORK_TOPOLOGY_VIEWER_FUNCTION) - 1)) {
        BUFFER *payload = network_viewer_read_request_payload_from_stdin();
        if(!payload) {
            freez(function);
            return 1;
        }
        result = network_viewer_topology_result(function, &stop_monotonic_ut, &cancelled, payload);
        buffer_free(payload);
    }
    else
        result = network_viewer_result(function);

    freez(function);

    int rc = network_viewer_write_test_result(result);
    buffer_free(result);
    return rc;
}

// ----------------------------------------------------------------------------------------------------------------
// main

int main(int argc, char **argv) {
    struct network_viewer_test_command test_command = { 0 };
    int test_parse_rc = parse_network_viewer_test_command(argc, argv, &test_command);
    if(test_parse_rc)
        exit(test_parse_rc);

    nd_thread_tag_set("NETWORK-VIEWER");
    nd_log_initialize_for_external_plugins("network-viewer.plugin");
    netdata_threads_init_for_external_plugins(0);

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(true) == -1) exit(1);

#if defined(LOCAL_SOCKETS_USE_SETNS)
    spawn_srv = spawn_server_create(SPAWN_SERVER_OPTION_CALLBACK, "setns", local_sockets_spawn_server_callback, argc, (const char **)argv);
    if(spawn_srv == NULL) {
        fprintf(stderr, "Cannot create spawn server.\n");
        exit(1);
    }
#endif

    cached_usernames_init();
    update_cached_host_users();
    sc = system_servicenames_cache_init();

    if(test_command.enabled) {
        nv_apps_lookup_init(&plugin_should_exit);
        nv_apps_lookup_start();

        int rc = run_network_viewer_test_command(&test_command);

        __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
        nv_apps_lookup_stop();

#if defined(OS_LINUX)
        network_viewer_ebpf_shared_memory_close();
        network_viewer_dns_shared_memory_close();
#endif

#if defined(LOCAL_SOCKETS_USE_SETNS)
        spawn_server_destroy(spawn_srv);
        spawn_srv = NULL;
#endif
        return rc;
    }

    // ----------------------------------------------------------------------------------------------------------------

    // Manual debug mode only; normal plugins.d execution never takes this path.
    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
//        for(int i = 0; i < 100; i++) {
            bool cancelled = false;
            usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
            char topo_buf[] = "topology:network-connections";
            network_viewer_topology_function("123", topo_buf, &stop_monotonic_ut, &cancelled,
                                             NULL, HTTP_ACCESS_ALL, NULL, NULL);

            char buf[] = "network-connections sockets:aggregated";
            network_viewer_function("123", buf, &stop_monotonic_ut, &cancelled,
                                     NULL, HTTP_ACCESS_ALL, NULL, NULL);

            char buf2[] = "network-connections sockets:detailed";
            network_viewer_function("123", buf2, &stop_monotonic_ut, &cancelled,
                                    NULL, HTTP_ACCESS_ALL, NULL, NULL);
//        }

#if defined(LOCAL_SOCKETS_USE_SETNS)
        spawn_server_destroy(spawn_srv);
#endif
#if defined(OS_LINUX)
        network_viewer_ebpf_shared_memory_close();
        network_viewer_dns_shared_memory_close();
#endif
        exit(1);
    }

    // ----------------------------------------------------------------------------------------------------------------

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
            NETWORK_TOPOLOGY_VIEWER_FUNCTION, 60,
            NETWORK_TOPOLOGY_VIEWER_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
        NETWORK_CONNECTIONS_VIEWER_FUNCTION, 60,
        NETWORK_CONNECTIONS_VIEWER_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

#if defined(OS_LINUX)
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
            NETWORK_DNS_QUERIES_FUNCTION, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
            NETWORK_DNS_QUERIES_FUNCTION_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
            NETWORK_PROTOCOLS_FUNCTION, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
            NETWORK_PROTOCOLS_FUNCTION_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE),
            RRDFUNCTIONS_PRIORITY_DEFAULT);
#endif

    // ----------------------------------------------------------------------------------------------------------------

    struct functions_evloop_globals *wg =
        functions_evloop_init(5, "Network-Viewer", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, NETWORK_CONNECTIONS_VIEWER_FUNCTION,
                                  network_viewer_function,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);

    functions_evloop_add_function(wg, NETWORK_TOPOLOGY_VIEWER_FUNCTION,
                                  network_viewer_topology_function,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);

#if defined(OS_LINUX)
    functions_evloop_add_function(wg, NETWORK_DNS_QUERIES_FUNCTION,
                                  network_viewer_dns_function,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
    functions_evloop_add_function(wg, NETWORK_PROTOCOLS_FUNCTION,
                                  function_network_protocols,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);
#endif

    // ----------------------------------------------------------------------------------------------------------------

    nv_apps_lookup_init(&plugin_should_exit);
    nv_apps_lookup_start();

    usec_t send_newline_ut = 0;
    bool tty = isatty(fileno(stdout)) == 1;
    int exit_code = 0;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        usec_t dt_ut = heartbeat_next(&hb);
        send_newline_ut += dt_ut;

        if(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE) && nv_apps_lookup_worker_exited()) {
            netdata_log_error("FATAL: network-viewer APPS_LOOKUP worker exited unexpectedly; requesting daemon respawn");
            __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
            exit_code = 1;
            break;
        }

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            netdata_mutex_lock(&stdout_mutex);
            nv_apps_lookup_send_charts_to_netdata(send_newline_ut);
            fprintf(stdout, "\n");
            fflush(stdout);
            netdata_mutex_unlock(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    functions_evloop_cancel_threads(wg);
    functions_evloop_join_threads(wg);
    nv_apps_lookup_stop();

#if defined(OS_LINUX)
    network_viewer_ebpf_shared_memory_close();
    network_viewer_dns_shared_memory_close();
#endif

#if defined(LOCAL_SOCKETS_USE_SETNS)
    spawn_server_destroy(spawn_srv);
    spawn_srv = NULL;
#endif

    return exit_code;
}

