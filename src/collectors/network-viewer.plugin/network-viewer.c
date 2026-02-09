// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#include "libnetdata/required_dummies.h"

static SPAWN_SERVER *spawn_srv = NULL;

#define ENABLE_DETAILED_VIEW

#define LOCAL_SOCKETS_EXTENDED_MEMBERS struct { \
        size_t count;                           \
        struct {                                \
            pid_t pid;                          \
            uid_t uid;                          \
            SOCKET_DIRECTION direction;         \
            int state;                          \
            uint64_t net_ns_inode;              \
            struct socket_endpoint server;      \
            const char *local_address_space;    \
            const char *remote_address_space;   \
        } aggregated_key;                       \
    } network_viewer;

#include "libnetdata/local-sockets/local-sockets.h"
#include "libnetdata/os/system-maps/system-services.h"

#define NETWORK_CONNECTIONS_VIEWER_FUNCTION "network-connections"
#define NETWORK_CONNECTIONS_VIEWER_HELP "Shows active network connections with protocol details, states, addresses, ports, and performance metrics."
#define NETWORK_TOPOLOGY_VIEWER_FUNCTION "topology:network-viewer"
#define NETWORK_TOPOLOGY_VIEWER_HELP "Shows live L7 topology from observed local sockets, including process and endpoint relations."
#define NETWORK_TOPOLOGY_SCHEMA_VERSION "2.0"
#define NETWORK_TOPOLOGY_SOURCE "network-viewer"
#define NETWORK_TOPOLOGY_LAYER "l7"

#define NV_TOPOLOGY_USERNAME_MAX 128
#define NV_TOPOLOGY_CMDLINE_MAX 512
#define NV_TOPOLOGY_KEY_MAX 1024

typedef struct {
    pid_t pid;
    uid_t uid;
    uint64_t net_ns_inode;
    uint64_t sockets;
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char local_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
} NV_PROCESS_ACTOR;

typedef struct {
    uint64_t sockets;
    char ip[INET6_ADDRSTRLEN];
    char address_space[16];
} NV_REMOTE_ACTOR;

typedef struct {
    uint64_t sockets;
    char ip[INET6_ADDRSTRLEN];
    char address_space[16];
} NV_LOCAL_IP;

typedef struct {
    uint64_t pid;
    uint64_t uid;
    uint64_t net_ns_inode;
    uint64_t sockets;
    uint64_t retransmissions;
    uint32_t max_rtt_usec;
    uint32_t max_rcv_rtt_usec;
    uint16_t local_port;
    uint16_t remote_port;
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char protocol[8];
    char protocol_family[8];
    char direction[16];
    char state[32];
    char local_ip[INET6_ADDRSTRLEN];
    char remote_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char remote_address_space[16];
    char port_name[64];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
} NV_TOPOLOGY_LINK;

typedef struct {
    DICTIONARY *process_actors;
    DICTIONARY *remote_actors;
    DICTIONARY *local_ips;
    DICTIONARY *links;
    usec_t now_ut;
    uint64_t sockets_total;
    uint64_t skipped_sockets;
    char hostname[256];
    char machine_guid[128];
} NV_TOPOLOGY_CONTEXT;

#define SIMPLE_HASHTABLE_VALUE_TYPE LOCAL_SOCKET *
#define SIMPLE_HASHTABLE_NAME _AGGREGATED_SOCKETS
#include "libnetdata/simple_hashtable/simple_hashtable.h"

netdata_mutex_t stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}
static bool plugin_should_exit = false;
static SERVICENAMES_CACHE *sc;

ENUM_STR_MAP_DEFINE(SOCKET_DIRECTION) = {
    { .id = SOCKET_DIRECTION_LISTEN, .name = "listen" },
    { .id = SOCKET_DIRECTION_LOCAL_INBOUND, .name = "local" },
    { .id = SOCKET_DIRECTION_LOCAL_OUTBOUND, .name = "local" },
    { .id = SOCKET_DIRECTION_INBOUND, .name = "inbound" },
    { .id = SOCKET_DIRECTION_OUTBOUND, .name = "outbound" },

    // terminator
    { . id = 0, .name = NULL }
};
ENUM_STR_DEFINE_FUNCTIONS(SOCKET_DIRECTION, SOCKET_DIRECTION_LISTEN, "unknown");

typedef int TCP_STATE;
ENUM_STR_MAP_DEFINE(TCP_STATE) = {
    { .id = TCP_ESTABLISHED, .name = "established" },
    { .id = TCP_SYN_SENT, .name = "syn-sent" },
    { .id = TCP_SYN_RECV, .name = "syn-received" },
    { .id = TCP_FIN_WAIT1, .name = "fin1-wait1" },
    { .id = TCP_FIN_WAIT2, .name = "fin1-wait2" },
    { .id = TCP_TIME_WAIT, .name = "time-wait" },
    { .id = TCP_CLOSE, .name = "close" },
    { .id = TCP_CLOSE_WAIT, .name = "close-wait" },
    { .id = TCP_LAST_ACK, .name = "last-ack" },
    { .id = TCP_LISTEN, .name = "listen" },
    { .id = TCP_CLOSING, .name = "closing" },

    // terminator
    { . id = 0, .name = NULL }
};
ENUM_STR_DEFINE_FUNCTIONS(TCP_STATE, 0, "unknown");

struct sockets_stats {
    BUFFER *wb;

    struct {
        uint32_t tcpi_rtt;
        uint32_t tcpi_rcv_rtt;
        uint32_t tcpi_total_retrans;
    } max;
};

static inline const char *network_viewer_machine_guid(void) {
    const char *guid = getenv("NETDATA_REGISTRY_UNIQUE_ID");
    return (guid && *guid) ? guid : NULL;
}

static inline const char *socket_protocol_name(uint16_t protocol) {
    return (protocol == IPPROTO_UDP) ? "udp" : "tcp";
}

static inline const char *socket_protocol_family_name(const LOCAL_SOCKET *n) {
    if(is_local_socket_ipv46(n))
        return "ipv46";

    if(n->local.family == AF_INET)
        return "ipv4";

    if(n->local.family == AF_INET6)
        return "ipv6";

    return "unknown";
}

static bool socket_endpoint_to_ip_text(const struct socket_endpoint *ep, char *dst) {
    if(ep->family == AF_INET) {
        ipv4_address_to_txt(ep->ip.ipv4, dst);
        return true;
    }

    if(ep->family == AF_INET6) {
        ipv6_address_to_txt(&ep->ip.ipv6, dst);
        return true;
    }

    dst[0] = '\0';
    return false;
}

static void topology_add_single_item_string_array(BUFFER *wb, const char *key, const char *value) {
    if(!value || !*value)
        return;

    if(strcmp(key, "ip_addresses") == 0 && strcmp(value, "*") == 0)
        return;

    buffer_json_member_add_array(wb, key);
    {
        buffer_json_add_array_item_string(wb, value);
    }
    buffer_json_array_close(wb);
}

static void topology_add_process_match(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx, const NV_PROCESS_ACTOR *pa) {
    buffer_json_member_add_object(wb, "match");
    {
        if(ctx->machine_guid[0])
            buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);

        topology_add_single_item_string_array(wb, "hostnames", ctx->hostname);
        topology_add_single_item_string_array(wb, "ip_addresses", pa->local_ip);
    }
    buffer_json_object_close(wb);
}

static void topology_add_host_match(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx) {
    buffer_json_member_add_object(wb, "match");
    {
        if(ctx->machine_guid[0])
            buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);

        topology_add_single_item_string_array(wb, "hostnames", ctx->hostname);
        buffer_json_member_add_array(wb, "ip_addresses");
        {
            NV_LOCAL_IP *lip;
            dfe_start_read(ctx->local_ips, lip) {
                if(!lip->ip[0]) continue;
                buffer_json_add_array_item_string(wb, lip->ip);
            }
            dfe_done(lip);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_add_remote_match(BUFFER *wb, const char *ip) {
    buffer_json_member_add_object(wb, "match");
    {
        topology_add_single_item_string_array(wb, "ip_addresses", ip);
    }
    buffer_json_object_close(wb);
}

static void local_socket_to_json_array(struct sockets_stats *st, const LOCAL_SOCKET *n, uint64_t proc_self_net_ns_inode, bool aggregated) {
    if(n->direction == SOCKET_DIRECTION_NONE)
        return;

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

        // RTT
        buffer_json_add_array_item_double(wb, (double)n->info.tcp.tcpi_rtt / (double)USEC_PER_MS);
        if(st->max.tcpi_rtt < n->info.tcp.tcpi_rtt)
            st->max.tcpi_rtt = n->info.tcp.tcpi_rtt;

        // Receiver RTT
        buffer_json_add_array_item_double(wb, (double)n->info.tcp.tcpi_rcv_rtt / (double)USEC_PER_MS);
        if(st->max.tcpi_rcv_rtt < n->info.tcp.tcpi_rcv_rtt)
            st->max.tcpi_rcv_rtt = n->info.tcp.tcpi_rcv_rtt;

        // Retransmissions
        buffer_json_add_array_item_uint64(wb, n->info.tcp.tcpi_total_retrans);
        if(st->max.tcpi_total_retrans < n->info.tcp.tcpi_total_retrans)
            st->max.tcpi_total_retrans = n->info.tcp.tcpi_total_retrans;

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

        // The current number of consecutive retransmissions that have occurred for the most recently transmitted segment.
        SUM_THEM_ALL(t->info.tcp.tcpi_retransmits, n->info.tcp.tcpi_retransmits);

        // The total number of retransmissions that have occurred for the entire connection since it was established.
        SUM_THEM_ALL(t->info.tcp.tcpi_total_retrans, n->info.tcp.tcpi_total_retrans);

        // The total number of segments that have been retransmitted since the connection was established.
        SUM_THEM_ALL(t->info.tcp.tcpi_retrans, n->info.tcp.tcpi_retrans);

        // The number of keepalive probes sent
        SUM_THEM_ALL(t->info.tcp.tcpi_probes, n->info.tcp.tcpi_probes);

        // The number of times the retransmission timeout has been backed off.
        SUM_THEM_ALL(t->info.tcp.tcpi_backoff, n->info.tcp.tcpi_backoff);

        // A bitmask representing the TCP options currently enabled for the connection, such as SACK and Timestamps.
        OR_THEM_ALL(t->info.tcp.tcpi_options, n->info.tcp.tcpi_options);

        // The send window scale value used for this connection
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_wscale, n->info.tcp.tcpi_snd_wscale);

        // The receive window scale value used for this connection
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_wscale, n->info.tcp.tcpi_rcv_wscale);

        // Retransmission timeout in milliseconds
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rto, n->info.tcp.tcpi_rto);

        // The delayed acknowledgement timeout in milliseconds.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_ato, n->info.tcp.tcpi_ato);

        // The maximum segment size for sending.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_mss, n->info.tcp.tcpi_snd_mss);

        // The maximum segment size for receiving.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_mss, n->info.tcp.tcpi_rcv_mss);

        // The number of unacknowledged segments
        SUM_THEM_ALL(t->info.tcp.tcpi_unacked, n->info.tcp.tcpi_unacked);

        // The number of segments that have been selectively acknowledged
        SUM_THEM_ALL(t->info.tcp.tcpi_sacked, n->info.tcp.tcpi_sacked);

        // The number of segments that have been selectively acknowledged
        SUM_THEM_ALL(t->info.tcp.tcpi_sacked, n->info.tcp.tcpi_sacked);

        // The number of lost segments.
        SUM_THEM_ALL(t->info.tcp.tcpi_lost, n->info.tcp.tcpi_lost);

        // The number of forward acknowledgment segments.
        SUM_THEM_ALL(t->info.tcp.tcpi_fackets, n->info.tcp.tcpi_fackets);

        // The time in milliseconds since the last data was sent.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_data_sent, n->info.tcp.tcpi_last_data_sent);

        // The time in milliseconds since the last acknowledgment was sent (not tracked in Linux, hence often zero).
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_ack_sent, n->info.tcp.tcpi_last_ack_sent);

        // The time in milliseconds since the last data was received.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_data_recv, n->info.tcp.tcpi_last_data_recv);

        // The time in milliseconds since the last acknowledgment was received.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_last_ack_recv, n->info.tcp.tcpi_last_ack_recv);

        // The path MTU for this connection
        KEEP_THE_SMALLER(t->info.tcp.tcpi_pmtu, n->info.tcp.tcpi_pmtu);

        // The slow start threshold for receiving
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_ssthresh, n->info.tcp.tcpi_rcv_ssthresh);

        // The slow start threshold for sending
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_ssthresh, n->info.tcp.tcpi_snd_ssthresh);

        // The round trip time in milliseconds
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rtt, n->info.tcp.tcpi_rtt);

        // The round trip time variance in milliseconds.
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rttvar, n->info.tcp.tcpi_rttvar);

        // The size of the sending congestion window.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_snd_cwnd, n->info.tcp.tcpi_snd_cwnd);

        // The maximum segment size that could be advertised.
        KEEP_THE_BIGGER(t->info.tcp.tcpi_advmss, n->info.tcp.tcpi_advmss);

        // The reordering metric
        KEEP_THE_SMALLER(t->info.tcp.tcpi_reordering, n->info.tcp.tcpi_reordering);

        // The receive round trip time in milliseconds.
        KEEP_THE_BIGGER(t->info.tcp.tcpi_rcv_rtt, n->info.tcp.tcpi_rcv_rtt);

        // The available space in the receive buffer.
        KEEP_THE_SMALLER(t->info.tcp.tcpi_rcv_space, n->info.tcp.tcpi_rcv_space);
    }
    else {
        t = mallocz(sizeof(*t));
        memcpy(t, n, sizeof(*t));
        t->cmdline = string_dup(t->cmdline);
        simple_hashtable_set_slot_AGGREGATED_SOCKETS(ht, sl, hash, t);
    }
}

static void local_sockets_cb_to_topology(LS_STATE *ls, const LOCAL_SOCKET *n, void *data) {
    if(n->direction == SOCKET_DIRECTION_NONE)
        return;

    NV_TOPOLOGY_CONTEXT *ctx = data;
    ctx->sockets_total++;

    char local_ip[INET6_ADDRSTRLEN] = "";
    char remote_ip[INET6_ADDRSTRLEN] = "";

    if(is_local_socket_ipv46(n))
        strncpyz(local_ip, "*", sizeof(local_ip) - 1);
    else if(!socket_endpoint_to_ip_text(&n->local, local_ip))
        return;

    if(!local_sockets_is_zero_address(&n->remote))
        socket_endpoint_to_ip_text(&n->remote, remote_ip);

    const char *namespace_type;
    if(n->net_ns_inode == ls->proc_self_net_ns_inode)
        namespace_type = "system";
    else if(n->net_ns_inode == 0)
        namespace_type = "unknown";
    else
        namespace_type = "container";

    const char *local_address_space = local_sockets_address_space(&n->local);
    const char *remote_address_space = local_sockets_address_space(&n->remote);
    const char *process_name = n->comm[0] ? n->comm : "[unknown]";
    const char *cmdline = string2str(n->cmdline);

    char username[NV_TOPOLOGY_USERNAME_MAX] = "[unknown]";
    if(n->uid != UID_UNSET) {
        CACHED_USERNAME cu = cached_username_get_by_uid(n->uid);
        const char *cached_username = string2str(cu.username);
        if(cached_username && *cached_username)
            snprintf(username, sizeof(username), "%s", cached_username);
        cached_username_release(cu);
    }

    if(local_ip[0] && strcmp(local_ip, "*") != 0) {
        NV_LOCAL_IP *local_actor = dictionary_get(ctx->local_ips, local_ip);
        if(!local_actor) {
            NV_LOCAL_IP tmp = { 0 };
            snprintf(tmp.ip, sizeof(tmp.ip), "%s", local_ip);
            snprintf(tmp.address_space, sizeof(tmp.address_space), "%s", local_address_space);
            local_actor = dictionary_set(ctx->local_ips, local_ip, &tmp, sizeof(tmp));
        }
        local_actor->sockets++;
    }

    char process_key[NV_TOPOLOGY_KEY_MAX];
    snprintf(process_key, sizeof(process_key), "pid=%d|uid=%u|ns=%llu|comm=%s|local=%s",
             n->pid,
             (unsigned)n->uid,
             (unsigned long long)n->net_ns_inode,
             process_name,
             local_ip);

    NV_PROCESS_ACTOR *pa = dictionary_get(ctx->process_actors, process_key);
    if(!pa) {
        NV_PROCESS_ACTOR tmp = { 0 };
        tmp.pid = n->pid;
        tmp.uid = n->uid;
        tmp.net_ns_inode = n->net_ns_inode;
        snprintf(tmp.process, sizeof(tmp.process), "%s", process_name);
        snprintf(tmp.username, sizeof(tmp.username), "%s", username);
        snprintf(tmp.namespace_type, sizeof(tmp.namespace_type), "%s", namespace_type);
        snprintf(tmp.local_ip, sizeof(tmp.local_ip), "%s", local_ip);
        snprintf(tmp.local_address_space, sizeof(tmp.local_address_space), "%s", local_address_space);
        if(cmdline && *cmdline)
            snprintf(tmp.cmdline, sizeof(tmp.cmdline), "%s", cmdline);
        pa = dictionary_set(ctx->process_actors, process_key, &tmp, sizeof(tmp));
    }
    pa->sockets++;

    if(!remote_ip[0]) {
        ctx->skipped_sockets++;
        return;
    }

    NV_REMOTE_ACTOR *ra = dictionary_get(ctx->remote_actors, remote_ip);
    if(!ra) {
        NV_REMOTE_ACTOR tmp = { 0 };
        snprintf(tmp.ip, sizeof(tmp.ip), "%s", remote_ip);
        snprintf(tmp.address_space, sizeof(tmp.address_space), "%s", remote_address_space);
        ra = dictionary_set(ctx->remote_actors, remote_ip, &tmp, sizeof(tmp));
    }
    ra->sockets++;

    const struct socket_endpoint *server_endpoint = NULL;
    switch(n->direction) {
        case SOCKET_DIRECTION_LISTEN:
        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
            server_endpoint = &n->local;
            break;

        case SOCKET_DIRECTION_OUTBOUND:
        case SOCKET_DIRECTION_LOCAL_OUTBOUND:
            server_endpoint = &n->remote;
            break;

        default:
            break;
    }

    char port_name[64] = "[unknown]";
    if(server_endpoint) {
        STRING *serv = system_servicenames_cache_lookup(sc, server_endpoint->port, server_endpoint->protocol);
        const char *tmp_name = string2str(serv);
        if(tmp_name && *tmp_name)
            snprintf(port_name, sizeof(port_name), "%s", tmp_name);
    }

    char link_key[NV_TOPOLOGY_KEY_MAX];
    snprintf(link_key, sizeof(link_key), "pid=%d|uid=%u|ns=%llu|local=%s|remote=%s|proto=%u|dir=%u|state=%u|lport=%u|rport=%u",
             n->pid,
             (unsigned)n->uid,
             (unsigned long long)n->net_ns_inode,
             local_ip,
             remote_ip,
             (unsigned)n->local.protocol,
             (unsigned)n->direction,
             (unsigned)n->state,
             n->local.port,
             n->remote.port);

    NV_TOPOLOGY_LINK *link = dictionary_get(ctx->links, link_key);
    if(!link) {
        NV_TOPOLOGY_LINK tmp = { 0 };
        tmp.pid = n->pid;
        tmp.uid = n->uid;
        tmp.net_ns_inode = n->net_ns_inode;
        tmp.local_port = n->local.port;
        tmp.remote_port = n->remote.port;
        snprintf(tmp.process, sizeof(tmp.process), "%s", process_name);
        snprintf(tmp.username, sizeof(tmp.username), "%s", username);
        snprintf(tmp.namespace_type, sizeof(tmp.namespace_type), "%s", namespace_type);
        snprintf(tmp.protocol, sizeof(tmp.protocol), "%s", socket_protocol_name(n->local.protocol));
        snprintf(tmp.protocol_family, sizeof(tmp.protocol_family), "%s", socket_protocol_family_name(n));
        snprintf(tmp.direction, sizeof(tmp.direction), "%s", SOCKET_DIRECTION_2str(n->direction));
        snprintf(tmp.state, sizeof(tmp.state), "%s",
                 n->local.protocol == IPPROTO_TCP ? TCP_STATE_2str(n->state) : "stateless");
        snprintf(tmp.local_ip, sizeof(tmp.local_ip), "%s", local_ip);
        snprintf(tmp.remote_ip, sizeof(tmp.remote_ip), "%s", remote_ip);
        snprintf(tmp.local_address_space, sizeof(tmp.local_address_space), "%s", local_address_space);
        snprintf(tmp.remote_address_space, sizeof(tmp.remote_address_space), "%s", remote_address_space);
        snprintf(tmp.port_name, sizeof(tmp.port_name), "%s", port_name);
        if(cmdline && *cmdline)
            snprintf(tmp.cmdline, sizeof(tmp.cmdline), "%s", cmdline);
        link = dictionary_set(ctx->links, link_key, &tmp, sizeof(tmp));
    }

    link->sockets++;
    link->retransmissions += n->info.tcp.tcpi_total_retrans;
    if(link->max_rtt_usec < n->info.tcp.tcpi_rtt)
        link->max_rtt_usec = n->info.tcp.tcpi_rtt;
    if(link->max_rcv_rtt_usec < n->info.tcp.tcpi_rcv_rtt)
        link->max_rcv_rtt_usec = n->info.tcp.tcpi_rcv_rtt;
}

static void network_viewer_topology_function(
    const char *transaction, char *function, usec_t *stop_monotonic_ut __maybe_unused,
    bool *cancelled __maybe_unused, BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {

    time_t now_s = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();
    bool info_only = false;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "topology");
    buffer_json_member_add_time_t(wb, "update_every", 5);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_TOPOLOGY_VIEWER_HELP);
    buffer_json_member_add_array(wb, "accepted_params");
    buffer_json_array_close(wb);
    buffer_json_member_add_array(wb, "required_params");
    buffer_json_array_close(wb);

    char function_copy[strlen(function) + 1];
    memcpy(function_copy, function, sizeof(function_copy));
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    for(size_t i = 1; i < num_words ;i++) {
        char *param = get_word(words, num_words, i);
        if(strcmp(param, "info") == 0) {
            info_only = true;
            break;
        }
    }

    NV_TOPOLOGY_CONTEXT ctx = {
        .now_ut = now_ut,
    };

    if(!info_only) {
        ctx.process_actors = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_PROCESS_ACTOR));
        ctx.remote_actors = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_REMOTE_ACTOR));
        ctx.local_ips = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_LOCAL_IP));
        ctx.links = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_TOPOLOGY_LINK));

        if(ctx.process_actors && ctx.remote_actors && ctx.local_ips && ctx.links) {
            if(!os_hostname(ctx.hostname, sizeof(ctx.hostname), netdata_configured_host_prefix))
                snprintf(ctx.hostname, sizeof(ctx.hostname), "%s", "localhost");

            const char *machine_guid = network_viewer_machine_guid();
            if(machine_guid)
                snprintf(ctx.machine_guid, sizeof(ctx.machine_guid), "%s", machine_guid);

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
                    .cb = local_sockets_cb_to_topology,
                    .data = &ctx,
                },
#if defined(LOCAL_SOCKETS_USE_SETNS)
                .spawn_server = spawn_srv,
#endif
                .stats = { 0 },
                .sockets_hashtable = { 0 },
                .local_ips_hashtable = { 0 },
                .listening_ports_hashtable = { 0 },
            };

            local_sockets_process(&ls);

            buffer_json_member_add_object(wb, "data");
            {
                size_t process_actor_count = dictionary_entries(ctx.process_actors);
                size_t remote_actor_count = dictionary_entries(ctx.remote_actors);
                size_t link_count = dictionary_entries(ctx.links);
                size_t local_ip_count = dictionary_entries(ctx.local_ips);

                buffer_json_member_add_string(wb, "schema_version", NETWORK_TOPOLOGY_SCHEMA_VERSION);
                buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                buffer_json_member_add_string(wb, "agent_id", ctx.machine_guid[0] ? ctx.machine_guid : ctx.hostname);
                buffer_json_member_add_datetime_rfc3339(wb, "collected_at", ctx.now_ut, true);

                buffer_json_member_add_array(wb, "actors");
                {
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_string(wb, "actor_type", "host");
                        buffer_json_member_add_string(wb, "layer", "l3");
                        buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                        topology_add_host_match(wb, &ctx);

                        buffer_json_member_add_object(wb, "attributes");
                        {
                            buffer_json_member_add_string(wb, "hostname", ctx.hostname);
                            buffer_json_member_add_uint64(wb, "local_ip_count", local_ip_count);
                            buffer_json_member_add_uint64(wb, "observed_sockets", ctx.sockets_total);
                        }
                        buffer_json_object_close(wb);

                        buffer_json_member_add_object(wb, "labels");
                        {
                            buffer_json_member_add_string(wb, "hostname", ctx.hostname);
                            if(ctx.machine_guid[0])
                                buffer_json_member_add_string(wb, "machine_guid", ctx.machine_guid);
                            buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                        }
                        buffer_json_object_close(wb);
                    }
                    buffer_json_object_close(wb);

                    NV_PROCESS_ACTOR *pa;
                    dfe_start_read(ctx.process_actors, pa) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "actor_type", "process");
                            buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                            buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                            topology_add_process_match(wb, &ctx, pa);

                            buffer_json_member_add_object(wb, "parent_match");
                            {
                                if(ctx.machine_guid[0])
                                    buffer_json_member_add_string(wb, "netdata_machine_guid", ctx.machine_guid);
                                topology_add_single_item_string_array(wb, "hostnames", ctx.hostname);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "attributes");
                            {
                                buffer_json_member_add_uint64(wb, "pid", pa->pid);
                                buffer_json_member_add_uint64(wb, "uid", pa->uid);
                                buffer_json_member_add_uint64(wb, "net_ns_inode", pa->net_ns_inode);
                                buffer_json_member_add_uint64(wb, "socket_count", pa->sockets);
                                buffer_json_member_add_string(wb, "local_ip", pa->local_ip);
                                buffer_json_member_add_string(wb, "local_address_space", pa->local_address_space);
                                if(pa->cmdline[0])
                                    buffer_json_member_add_string(wb, "cmdline", pa->cmdline);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "labels");
                            {
                                buffer_json_member_add_string(wb, "process", pa->process);
                                buffer_json_member_add_string(wb, "user", pa->username);
                                buffer_json_member_add_string(wb, "namespace", pa->namespace_type);
                                buffer_json_member_add_string(wb, "local_address_space", pa->local_address_space);
                            }
                            buffer_json_object_close(wb);
                        }
                        buffer_json_object_close(wb);
                    }
                    dfe_done(pa);

                    NV_REMOTE_ACTOR *ra;
                    dfe_start_read(ctx.remote_actors, ra) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "actor_type", "ip");
                            buffer_json_member_add_string(wb, "layer", "l3");
                            buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                            topology_add_remote_match(wb, ra->ip);

                            buffer_json_member_add_object(wb, "attributes");
                            {
                                buffer_json_member_add_uint64(wb, "socket_count", ra->sockets);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "labels");
                            {
                                buffer_json_member_add_string(wb, "address_space", ra->address_space);
                            }
                            buffer_json_object_close(wb);
                        }
                        buffer_json_object_close(wb);
                    }
                    dfe_done(ra);
                }
                buffer_json_array_close(wb);

                buffer_json_member_add_array(wb, "links");
                {
                    NV_TOPOLOGY_LINK *link;
                    dfe_start_read(ctx.links, link) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                            buffer_json_member_add_string(wb, "protocol", link->protocol);
                            buffer_json_member_add_string(wb, "direction", link->direction);
                            buffer_json_member_add_string(wb, "state", link->state);
                            buffer_json_member_add_datetime_rfc3339(wb, "discovered_at", ctx.now_ut, true);
                            buffer_json_member_add_datetime_rfc3339(wb, "last_seen", ctx.now_ut, true);

                            buffer_json_member_add_object(wb, "src");
                            {
                                buffer_json_member_add_object(wb, "match");
                                {
                                    if(ctx.machine_guid[0])
                                        buffer_json_member_add_string(wb, "netdata_machine_guid", ctx.machine_guid);
                                    topology_add_single_item_string_array(wb, "hostnames", ctx.hostname);
                                    topology_add_single_item_string_array(wb, "ip_addresses", link->local_ip);
                                }
                                buffer_json_object_close(wb);

                                buffer_json_member_add_object(wb, "attributes");
                                {
                                    buffer_json_member_add_string(wb, "actor_type", "process");
                                    buffer_json_member_add_uint64(wb, "pid", link->pid);
                                    buffer_json_member_add_uint64(wb, "uid", link->uid);
                                    buffer_json_member_add_uint64(wb, "net_ns_inode", link->net_ns_inode);
                                    buffer_json_member_add_string(wb, "process", link->process);
                                    buffer_json_member_add_string(wb, "user", link->username);
                                    buffer_json_member_add_string(wb, "namespace", link->namespace_type);
                                    buffer_json_member_add_string(wb, "address_space", link->local_address_space);
                                    buffer_json_member_add_uint64(wb, "port", link->local_port);
                                    buffer_json_member_add_string(wb, "port_name", link->port_name);
                                    buffer_json_member_add_string(wb, "protocol_family", link->protocol_family);
                                    if(link->cmdline[0])
                                        buffer_json_member_add_string(wb, "cmdline", link->cmdline);
                                }
                                buffer_json_object_close(wb);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "dst");
                            {
                                topology_add_remote_match(wb, link->remote_ip);

                                buffer_json_member_add_object(wb, "attributes");
                                {
                                    buffer_json_member_add_string(wb, "address_space", link->remote_address_space);
                                    buffer_json_member_add_uint64(wb, "port", link->remote_port);
                                }
                                buffer_json_object_close(wb);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "metrics");
                            {
                                buffer_json_member_add_uint64(wb, "socket_count", link->sockets);
                                buffer_json_member_add_uint64(wb, "retransmissions", link->retransmissions);
                                buffer_json_member_add_double(wb, "rtt_ms_max", (double)link->max_rtt_usec / (double)USEC_PER_MS);
                                buffer_json_member_add_double(wb, "recv_rtt_ms_max", (double)link->max_rcv_rtt_usec / (double)USEC_PER_MS);
                            }
                            buffer_json_object_close(wb);

                            buffer_json_member_add_object(wb, "labels");
                            {
                                buffer_json_member_add_string(wb, "protocol", link->protocol);
                                buffer_json_member_add_string(wb, "direction", link->direction);
                                buffer_json_member_add_string(wb, "state", link->state);
                                buffer_json_member_add_string(wb, "process", link->process);
                                buffer_json_member_add_string(wb, "user", link->username);
                                buffer_json_member_add_string(wb, "namespace", link->namespace_type);
                                buffer_json_member_add_string(wb, "protocol_family", link->protocol_family);
                                buffer_json_member_add_string(wb, "local_address_space", link->local_address_space);
                                buffer_json_member_add_string(wb, "remote_address_space", link->remote_address_space);
                                buffer_json_member_add_string(wb, "port_name", link->port_name);
                            }
                            buffer_json_object_close(wb);
                        }
                        buffer_json_object_close(wb);
                    }
                    dfe_done(link);
                }
                buffer_json_array_close(wb);

                buffer_json_member_add_object(wb, "stats");
                {
                    buffer_json_member_add_uint64(wb, "sockets_total", ctx.sockets_total);
                    buffer_json_member_add_uint64(wb, "sockets_without_remote_endpoint", ctx.skipped_sockets);
                    buffer_json_member_add_uint64(wb, "local_process_actors", process_actor_count);
                    buffer_json_member_add_uint64(wb, "remote_ip_actors", remote_actor_count);
                    buffer_json_member_add_uint64(wb, "links_total", link_count);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }

    if(ctx.links)
        dictionary_destroy(ctx.links);
    if(ctx.local_ips)
        dictionary_destroy(ctx.local_ips);
    if(ctx.remote_actors)
        dictionary_destroy(ctx.remote_actors);
    if(ctx.process_actors)
        dictionary_destroy(ctx.process_actors);

    buffer_json_member_add_time_t(wb, "expires", now_s + 1);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + 1;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

static int local_sockets_compar(const void *a, const void *b) {
    LOCAL_SOCKET *n1 = *(LOCAL_SOCKET **)a, *n2 = *(LOCAL_SOCKET **)b;
    return strcmp(n1->comm, n2->comm);
}

void network_viewer_function(const char *transaction, char *function __maybe_unused, usec_t *stop_monotonic_ut __maybe_unused,
                             bool *cancelled __maybe_unused, BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
                             const char *source __maybe_unused, void *data __maybe_unused) {

    time_t now_s = now_realtime_sec();
    bool aggregated = false;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    struct sockets_stats st = {
        .wb = wb,
    };

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 5);
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

    char function_copy[strlen(function) + 1];
    memcpy(function_copy, function, sizeof(function_copy));
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    for(size_t i = 1; i < num_words ;i++) {
        char *param = get_word(words, num_words, i);
        if(strcmp(param, "sockets:aggregated") == 0) {
            aggregated = true;
        }
        else if(strcmp(param, "sockets:detailed") == 0) {
            aggregated = false;
        }
        else if(strcmp(param, "info") == 0) {
            goto close_and_send;
        }
    }

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

        if(aggregated) {
            LOCAL_SOCKET *array[ht.used];
            size_t added = 0;
            uint64_t proc_self_net_ns_inode = ls.proc_self_net_ns_inode;
            for(SIMPLE_HASHTABLE_SLOT_AGGREGATED_SOCKETS *sl = simple_hashtable_first_read_only_AGGREGATED_SOCKETS(&ht);
                 sl;
                 sl = simple_hashtable_next_read_only_AGGREGATED_SOCKETS(&ht, sl)) {
                LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
                if(!n || added >= ht.used) continue;

                array[added++] = n;
            }

            qsort(array, added, sizeof(LOCAL_SOCKET *), local_sockets_compar);

            for(size_t i = 0; i < added ;i++) {
                local_socket_to_json_array(&st, array[i], proc_self_net_ns_inode, true);
                string_freez(array[i]->cmdline);
                freez(array[i]);
            }

            simple_hashtable_destroy_AGGREGATED_SOCKETS(&ht);
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
            // Data Collection Age chart
            buffer_json_member_add_object(wb, "Count");
            {
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Direction");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            // Streaming Age chart
            buffer_json_member_add_object(wb, "Count");
            {
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Process");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            // DB Duration
            buffer_json_member_add_object(wb, "Count");
            {
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Protocol");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
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
            buffer_json_member_add_object(wb, "Direction");
            {
                buffer_json_member_add_string(wb, "name", "Direction");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Direction");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "Protocol");
            {
                buffer_json_member_add_string(wb, "name", "Protocol");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Protocol");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "Namespace");
            {
                buffer_json_member_add_string(wb, "name", "Namespace");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Namespace");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "Process");
            {
                buffer_json_member_add_string(wb, "name", "Process");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Process");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            if(!aggregated) {
                buffer_json_member_add_object(wb, "LocalIP");
                {
                    buffer_json_member_add_string(wb, "name", "Local IP");
                    buffer_json_member_add_array(wb, "columns");
                    {
                        buffer_json_add_array_item_string(wb, "LocalIP");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "LocalPort");
                {
                    buffer_json_member_add_string(wb, "name", "Local Port");
                    buffer_json_member_add_array(wb, "columns");
                    {
                        buffer_json_add_array_item_string(wb, "LocalPort");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "RemoteIP");
                {
                    buffer_json_member_add_string(wb, "name", "Remote IP");
                    buffer_json_member_add_array(wb, "columns");
                    {
                        buffer_json_add_array_item_string(wb, "RemoteIP");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "RemotePort");
                {
                    buffer_json_member_add_string(wb, "name", "Remote Port");
                    buffer_json_member_add_array(wb, "columns");
                    {
                        buffer_json_add_array_item_string(wb, "RemotePort");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
        }
        buffer_json_object_close(wb); // group_by
    }

close_and_send:
    buffer_json_member_add_time_t(wb, "expires", now_s + 1);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + 1;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

// ----------------------------------------------------------------------------------------------------------------
// main

int main(int argc __maybe_unused, char **argv __maybe_unused) {
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

    // ----------------------------------------------------------------------------------------------------------------

    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
//        for(int i = 0; i < 100; i++) {
            bool cancelled = false;
            usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
            char topo_buf[] = "topology:network-viewer";
            network_viewer_topology_function("123", topo_buf, &stop_monotonic_ut, &cancelled,
                                             NULL, HTTP_ACCESS_ALL, NULL, NULL);

            char buf[] = "network-connections sockets:aggregated";
            network_viewer_function("123", buf, &stop_monotonic_ut, &cancelled,
                                     NULL, HTTP_ACCESS_ALL, NULL, NULL);

            char buf2[] = "network-connections sockets:detailed";
            network_viewer_function("123", buf2, &stop_monotonic_ut, &cancelled,
                                    NULL, HTTP_ACCESS_ALL, NULL, NULL);
//        }

        spawn_server_destroy(spawn_srv);
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

    // ----------------------------------------------------------------------------------------------------------------

    usec_t send_newline_ut = 0;
    bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        usec_t dt_ut = heartbeat_next(&hb);
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    spawn_server_destroy(spawn_srv);
    spawn_srv = NULL;

    return 0;
}
