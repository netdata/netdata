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
