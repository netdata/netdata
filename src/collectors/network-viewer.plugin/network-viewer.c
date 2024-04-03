// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
ebpf_module_t ebpf_nv_module;

static ebpf_local_maps_t nv_maps[] = {{.name = "tbl_nv_socket",
                                       .internal_input = 65536,
                                       .user_input = 65536, .type = NETDATA_EBPF_MAP_RESIZABLE,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                      },
                                      {.name = "nv_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                       .user_input = NETDATA_CONTROLLER_END,
                                       .type = NETDATA_EBPF_MAP_CONTROLLER,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                      },
                                      {.name = NULL, .internal_input = 0, .user_input = 0,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                      }};

netdata_ebpf_targets_t nv_targets[] = { {.name = "inet_csk_accept", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_retransmit_skb", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_cleanup_rbuf", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_close", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "udp_recvmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_sendmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "udp_sendmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_v4_connect", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_v6_connect", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "tcp_set_state", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};


#ifdef LIBBPF_MAJOR_VERSION // BTF code
#include "libnetdata/ebpf/includes/networkviewer.skel.h"
struct networkviewer_bpf *networkviewer_bpf_obj = NULL;
struct btf *common_btf = NULL;
#else
void *common_btf = NULL;
#endif

#endif // defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)

#define ENABLE_DETAILED_VIEW

#define LOCAL_SOCKETS_EXTENDED_MEMBERS struct { \
        size_t count;                           \
        const char *local_address_space;        \
        const char *remote_address_space;       \
    } network_viewer;

#include "libnetdata/maps/local-sockets.h"
#include "libnetdata/maps/system-users.h"

#define NETWORK_CONNECTIONS_VIEWER_FUNCTION "network-connections"
#define NETWORK_CONNECTIONS_VIEWER_HELP "Network connections explorer"

#define SIMPLE_HASHTABLE_VALUE_TYPE LOCAL_SOCKET
#define SIMPLE_HASHTABLE_NAME _AGGREGATED_SOCKETS
#include "libnetdata/simple_hashtable.h"

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;
static USERNAMES_CACHE *uc;

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

typedef struct networkviewer_opt {
    bool debug;
    bool ebpf;
    int  level;
} networkviewer_opt_t;

static void local_socket_to_json_array(BUFFER *wb, LOCAL_SOCKET *n, uint64_t proc_self_net_ns_inode, bool aggregated) {
    char local_address[INET6_ADDRSTRLEN];
    char remote_address[INET6_ADDRSTRLEN];
    char *protocol;

    if(n->local.family == AF_INET) {
        ipv4_address_to_txt(n->local.ip.ipv4, local_address);
        ipv4_address_to_txt(n->remote.ip.ipv4, remote_address);
        protocol = n->local.protocol == IPPROTO_TCP ? "tcp4" : "udp4";
    }
    else if(n->local.family == AF_INET6) {
        ipv6_address_to_txt(&n->local.ip.ipv6, local_address);
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
            STRING *u = system_usernames_cache_lookup_uid(uc, n->uid);
            buffer_json_add_array_item_string(wb, string2str(u));
            string_freez(u);
        }

        if(!aggregated) {
            buffer_json_add_array_item_string(wb, local_address);
            buffer_json_add_array_item_uint64(wb, n->local.port);
        }
        buffer_json_add_array_item_string(wb, n->network_viewer.local_address_space);

        if(!aggregated) {
            buffer_json_add_array_item_string(wb, remote_address);
            buffer_json_add_array_item_uint64(wb, n->remote.port);
        }
        buffer_json_add_array_item_string(wb, n->network_viewer.remote_address_space);

        uint16_t server_port = 0;
        const char *server_address = NULL;
        const char *client_address_space = NULL;
        const char *server_address_space = NULL;
        switch (n->direction) {
            case SOCKET_DIRECTION_LISTEN:
            case SOCKET_DIRECTION_INBOUND:
            case SOCKET_DIRECTION_LOCAL_INBOUND:
                server_port = n->local.port;
                server_address = local_address;
                server_address_space = n->network_viewer.local_address_space;
                client_address_space = n->network_viewer.remote_address_space;
                break;

            case SOCKET_DIRECTION_OUTBOUND:
            case SOCKET_DIRECTION_LOCAL_OUTBOUND:
                server_port = n->remote.port;
                server_address = remote_address;
                server_address_space = n->network_viewer.remote_address_space;
                client_address_space = n->network_viewer.local_address_space;
                break;

            case SOCKET_DIRECTION_NONE:
                break;
        }
        if(aggregated)
            buffer_json_add_array_item_string(wb, server_address);

        buffer_json_add_array_item_uint64(wb, server_port);

        if(aggregated) {
            buffer_json_add_array_item_string(wb, client_address_space);
            buffer_json_add_array_item_string(wb, server_address_space);
        }

        // buffer_json_add_array_item_uint64(wb, n->inode);
        // buffer_json_add_array_item_uint64(wb, n->net_ns_inode);
        buffer_json_add_array_item_uint64(wb, n->network_viewer.count);
    }
    buffer_json_array_close(wb);
}

static void local_sockets_cb_to_json(LS_STATE *ls, LOCAL_SOCKET *n, void *data) {
    n->network_viewer.count = 1;
    n->network_viewer.local_address_space = local_sockets_address_space(&n->local);
    n->network_viewer.remote_address_space = local_sockets_address_space(&n->remote);
    local_socket_to_json_array(data, n, ls->proc_self_net_ns_inode, false);
}

static void local_sockets_cb_to_aggregation(LS_STATE *ls __maybe_unused, LOCAL_SOCKET *n, void *data) {
    SIMPLE_HASHTABLE_AGGREGATED_SOCKETS *ht = data;
    n->network_viewer.count = 1;
    n->network_viewer.local_address_space = local_sockets_address_space(&n->local);
    n->network_viewer.remote_address_space = local_sockets_address_space(&n->remote);

    switch(n->direction) {
        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
        case SOCKET_DIRECTION_LISTEN:
            memset(&n->remote.ip, 0, sizeof(n->remote.ip));
            n->remote.port = 0;
            break;

        case SOCKET_DIRECTION_OUTBOUND:
        case SOCKET_DIRECTION_LOCAL_OUTBOUND:
            memset(&n->local.ip, 0, sizeof(n->local.ip));
            n->local.port = 0;
            break;

        case SOCKET_DIRECTION_NONE:
            return;
    }

    n->inode = 0;
    n->local_ip_hash = 0;
    n->remote_ip_hash = 0;
    n->local_port_hash = 0;
    n->timer = 0;
    n->retransmits = 0;
    n->expires = 0;
    n->rqueue = 0;
    n->wqueue = 0;
    memset(&n->local_port_key, 0, sizeof(n->local_port_key));

    XXH64_hash_t hash = XXH3_64bits(n, sizeof(*n));
    SIMPLE_HASHTABLE_SLOT_AGGREGATED_SOCKETS *sl = simple_hashtable_get_slot_AGGREGATED_SOCKETS(ht, hash, n, true);
    LOCAL_SOCKET *t = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(t) {
        t->network_viewer.count++;
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
    size_t num_words = quoted_strings_splitter_pluginsd(function_copy, words, 1024);
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

                .max_errors = 10,
            },
            .stats = { 0 },
#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
            .use_ebpf = false,
            .ebpf_module = &ebpf_nv_module,
#endif
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
            ls.config.data = wb;
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
                local_socket_to_json_array(wb, array[i], proc_self_net_ns_inode, true);
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

            // Count
            buffer_rrdf_table_add_field(wb, field_id++, "Count", "Number of sockets like this",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
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
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_s + 1, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
// ----------------------------------------------------------------------------------------------------------------
// eBPF

#ifdef LIBBPF_MAJOR_VERSION
static inline void ebpf_networkviewer_disable_probes(struct networkviewer_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_nv_inet_csk_accept_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v4_connect_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v6_connect_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_retransmit_skb_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_cleanup_rbuf_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_close_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_udp_recvmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_sendmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_udp_sendmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_set_state_kprobe, false);
}

static inline void ebpf_networkviewer_disable_trampoline(struct networkviewer_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_nv_inet_csk_accept_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v4_connect_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v6_connect_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_retransmit_skb_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_cleanup_rbuf_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_close_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_udp_recvmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_sendmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_udp_sendmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_nv_tcp_set_state_fentry, false);
}

static void ebpf_networkviewer_set_trampoline_target(struct networkviewer_bpf *obj, netdata_ebpf_targets_t *targets)
{
    bpf_program__set_attach_target(obj->progs.netdata_nv_inet_csk_accept_fexit, 0,
                                   targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_v4_connect_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_V4_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_v6_connect_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_V6_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_retransmit_skb_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_RETRANSMIT].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_cleanup_rbuf_fentry, 0,
                                   targets[NETDATA_FCNT_CLEANUP_RBUF].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_close_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_CLOSE].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_udp_recvmsg_fentry, 0,
                                   targets[NETDATA_FCNT_UDP_RECEVMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_sendmsg_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_udp_sendmsg_fentry, 0,
                                   targets[NETDATA_FCNT_UDP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_nv_tcp_set_state_fentry, 0,
                                   targets[NETDATA_FCNT_TCP_SET_STATE].name);
}

static int ebpf_networkviewer_attach_probes(struct networkviewer_bpf *obj, netdata_ebpf_targets_t *targets)
{
    obj->links.netdata_nv_inet_csk_accept_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_inet_csk_accept_kretprobe,
                                                                                 true,
                                                                                 targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);
    int ret = libbpf_get_error(obj->links.netdata_nv_inet_csk_accept_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_tcp_v4_connect_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_v4_connect_kprobe,
                                                                             false,
                                                                             targets[NETDATA_FCNT_TCP_V4_CONNECT].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_v4_connect_kprobe);
    if (ret)
        return -1;

    /*
    obj->links.netdata_nv_tcp_v6_connect_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_v6_connect_kprobe,
                                                                             false,
                                                                             targets[NETDATA_FCNT_TCP_V6_CONNECT].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_v6_connect_kprobe);
    if (ret)
        return -1;
        */

    obj->links.netdata_nv_tcp_retransmit_skb_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_retransmit_skb_kprobe,
                                                                                 false,
                                                                                 targets[NETDATA_FCNT_TCP_RETRANSMIT].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_retransmit_skb_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_tcp_cleanup_rbuf_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_cleanup_rbuf_kprobe,
                                                                               false,
                                                                               targets[NETDATA_FCNT_CLEANUP_RBUF].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_cleanup_rbuf_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_tcp_close_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_close_kprobe,
                                                                        false,
                                                                        targets[NETDATA_FCNT_TCP_CLOSE].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_close_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_udp_recvmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_udp_recvmsg_kprobe,
                                                                          false,
                                                                          targets[NETDATA_FCNT_UDP_RECEVMSG].name);
    ret = libbpf_get_error(obj->links.netdata_nv_udp_recvmsg_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_tcp_sendmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_sendmsg_kprobe,
                                                                          false,
                                                                          targets[NETDATA_FCNT_TCP_SENDMSG].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_sendmsg_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_udp_sendmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_udp_sendmsg_kprobe,
                                                                          false,
                                                                          targets[NETDATA_FCNT_UDP_SENDMSG].name);
    ret = libbpf_get_error(obj->links.netdata_nv_udp_sendmsg_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_nv_tcp_set_state_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_nv_tcp_set_state_kprobe,
                                                                            false,
                                                                            targets[NETDATA_FCNT_TCP_SET_STATE].name);
    ret = libbpf_get_error(obj->links.netdata_nv_tcp_set_state_kprobe);
    if (ret)
        return -1;

    return 0;
}

static inline int ebpf_networkviewer_load_and_attach(struct networkviewer_bpf *obj, ebpf_module_t *em)
{
    if (!em->maps_per_core) {
        // Added to be compatible with all kernels
        bpf_map__set_type(obj->maps.tbl_nv_socket, BPF_MAP_TYPE_HASH);
        bpf_map__set_type(obj->maps.nv_ctrl, BPF_MAP_TYPE_ARRAY);
        bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v6_connect_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_nv_tcp_v6_connect_fentry, false);
    }

    int ret;
    if (em->targets[0].mode == EBPF_LOAD_TRAMPOLINE) { // trampoline
        ebpf_networkviewer_disable_probes(obj);

        ebpf_networkviewer_set_trampoline_target(obj, em->targets);

        ret = networkviewer_bpf__load(obj);
        if (!ret)
            ret = networkviewer_bpf__attach(obj);
    } else { // kprobe
        ebpf_networkviewer_disable_trampoline(obj);

        ret = networkviewer_bpf__load(obj);
        if (!ret)
            ret = ebpf_networkviewer_attach_probes(obj, em->targets);
    }

    return ret;
}
#endif

static inline void network_viwer_set_module(ebpf_module_t *em, networkviewer_opt_t *args)
{
    static char *binary_name = {"network_viewer"};

    em->info.thread_name = em->info.config_name = binary_name;
    em->maps = nv_maps;
    em->probe_links = NULL;
    em->objects = NULL;
    em->kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14;
    em->maps_per_core = 0;
    em->mode = MODE_ENTRY;
    em->targets = nv_targets;
    em->apps_charts = NETDATA_EBPF_APPS_FLAG_YES;
    em->apps_level = args->level;

#ifdef LIBBPF_MAJOR_VERSION
    em->load |= EBPF_LOAD_CORE; // Prefer CO-RE avoiding kprobes
    ebpf_adjust_thread_load(em, common_btf);
#else
    em->load |= EBPF_LOAD_LEGACY;
#endif
}

static inline int network_viewer_load_ebpf_to_kernel(ebpf_module_t *em, int kver)
{
    int isrh = get_redhat_release();

    int ret = 0;
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(PLUGINS_DIR, em, kver, isrh, &em->objects);
        if (!em->probe_links)
            ret = -1;
        else {
            struct bpf_map *map;
            bpf_object__for_each_map(map, em->objects)
            {
                const char *name = bpf_map__name(map);
                int fd = bpf_map__fd(map);
                if (!strcmp(name, nv_maps[0].name))
                    em->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd = fd;
                else
                    em->maps[NETWORK_VIEWER_EBPF_NV_CONTROL].map_fd = fd;
            }
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        ebpf_define_map_type(em->maps, em->maps_per_core, kver);
        networkviewer_bpf_obj = networkviewer_bpf__open();
        ret = (!networkviewer_bpf_obj) ? -1 : ebpf_networkviewer_load_and_attach(networkviewer_bpf_obj, em);
        if (!ret) {
            em->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd = bpf_map__fd(networkviewer_bpf_obj->maps.tbl_nv_socket);
            em->maps[NETWORK_VIEWER_EBPF_NV_CONTROL].map_fd = bpf_map__fd(networkviewer_bpf_obj->maps.nv_ctrl);
        }
    }
#endif

    if (!ret) {
        ebpf_update_controller(nv_maps[1].map_fd, em);
        // We are going to use the optional value to determine when data is loaded in kernel
        ebpf_nv_module.optional = NETWORK_VIEWER_EBPF_NV_NOT_RUNNING;
        ebpf_nv_module.running_time = now_realtime_sec();

        rw_spinlock_init(&ebpf_nv_module.rw_spinlock);
    }

    return ret;
}

static inline void network_viewer_unload_ebpf()
{
#ifdef LIBBPF_MAJOR_VERSION
    if (common_btf) {
        btf__free(common_btf);
        common_btf = NULL;
    }

    if (networkviewer_bpf_obj) {
        networkviewer_bpf__destroy(networkviewer_bpf_obj);
        networkviewer_bpf_obj = NULL;
    }
#else
    if (ebpf_nv_module.objects)
        ebpf_unload_legacy_code(ebpf_nv_module.objects, ebpf_nv_module.probe_links);
#endif
    ebpf_local_maps_t *maps = ebpf_nv_module.maps;
    if (maps)
        maps[0].map_fd = maps[1].map_fd = -1;
}

static inline void network_viewer_load_ebpf(networkviewer_opt_t *args)
{
    memset(&ebpf_nv_module, 0, sizeof(ebpf_module_t));

    if (ebpf_can_plugin_load_code(ebpf_get_kernel_version(), "networkviewer.plugin"))
        return;

    if (ebpf_adjust_memory_limit())
        return;

#ifdef LIBBPF_MAJOR_VERSION
    common_btf = ebpf_load_btf_file(EBPF_DEFAULT_BTF_PATH, EBPF_DEFAULT_BTF_FILE);
#endif

    int kver = ebpf_get_kernel_version();
    network_viwer_set_module(&ebpf_nv_module, args);

    if (network_viewer_load_ebpf_to_kernel(&ebpf_nv_module, kver))
        network_viewer_unload_ebpf();
}

void *network_viewer_ebpf_worker(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t max = 5 * NETWORK_VIEWER_EBPF_ACTION_LIMIT;
    uint64_t removed = 0;
    while (!plugin_should_exit) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);

        rw_spinlock_write_lock(&em->rw_spinlock);
        uint32_t curr = now_realtime_sec() - em->running_time;
        if (em->optional != NETWORK_VIEWER_EBPF_NV_NOT_RUNNING || curr < max) {
            rw_spinlock_write_unlock(&em->rw_spinlock);
            continue;
        }
        em->running_time = now_realtime_sec();
        em->optional = NETWORK_VIEWER_EBPF_NV_NOT_RUNNING;
        rw_spinlock_write_unlock(&em->rw_spinlock);

        ebpf_nv_idx_t key = {};
        ebpf_nv_idx_t next_key = {};
        ebpf_nv_data_t stored = {};
        int fd = em->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd;
        while (!bpf_map_get_next_key(fd, &key, &next_key)) {
            if (!bpf_map_lookup_elem(fd, &key, &stored)) {
                if (stored.closed) {
                    bpf_map_delete_elem(fd, &key);
                    removed++;
                }
            }

            stored.closed = 0;
            key = next_key;
        }
    }

    if (removed)
        local_sockets_reset_ebpf_value(em, removed);

    return NULL;
}
#endif // defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)

// ----------------------------------------------------------------------------------------------------------------
// Parse Args

static void networkviewer_parse_args(networkviewer_opt_t *args, int argc, char **argv)
{
    int i;
    for(i = 1; i < argc; i++) {
        if (strcmp("debug", argv[i]) == 0)
            args->debug = true;
        else if (strcmp("ebpf", argv[i]) == 0) {
            args->ebpf = true;
            args->level = NETDATA_APPS_LEVEL_PARENT;
        }
        else if (strcmp("apps-level", argv[i]) == 0) {
            if(argc <= i + 1) {
                nd_log(NDLS_COLLECTORS, NDLP_INFO, "Parameter 'apps-level' requires either %u or %u as argument.",
                       NETDATA_APPS_LEVEL_REAL_PARENT, NETDATA_APPS_LEVEL_PARENT);
                exit(1);
            }

            i++;
            args->level = str2i(argv[i]);
            // We avoid the highest level (NETDATA_APPS_LEVEL_ALL), because charts can become unreadable
            if (args->level < NETDATA_APPS_LEVEL_REAL_PARENT || args->level > NETDATA_APPS_LEVEL_PARENT)
                args->level = NETDATA_APPS_LEVEL_PARENT;
        }
    }
}

// ----------------------------------------------------------------------------------------------------------------
// main

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    clocks_init();
    nd_thread_tag_set("NETWORK-VIEWER");
    nd_log_initialize_for_external_plugins("network-viewer.plugin");

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(true) == -1) exit(1);

    uc = system_usernames_cache_init();

    // ----------------------------------------------------------------------------------------------------------------

    networkviewer_opt_t args =  {};
    networkviewer_parse_args(&args, argc, argv);
    if(args.debug) {
#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
        if(args.ebpf)
            network_viewer_load_ebpf(&args);

        if (ebpf_nv_module.maps && ebpf_nv_module.maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd > 0)
            nd_log(NDLS_COLLECTORS, NDLP_INFO, "PLUGIN: the plugin will use eBPF %s to monitor sockets.",
                   (nv_targets[0].mode == EBPF_LOAD_TRAMPOLINE) ? "trampoline" : "kprobe");
#endif

        bool cancelled = false;
        usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
        char buf[] = "network-connections sockets:aggregated";
        network_viewer_function("123", buf, &stop_monotonic_ut, &cancelled,
                                 NULL, HTTP_ACCESS_ALL, NULL, NULL);

        char buf2[] = "network-connections sockets:detailed";
        network_viewer_function("123", buf2, &stop_monotonic_ut, &cancelled,
                                NULL, HTTP_ACCESS_ALL, NULL, NULL);

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
        if (args.ebpf)
            network_viewer_unload_ebpf();
#endif
        exit(1);
    }

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    static netdata_thread_t *nv_clean_thread = NULL;
    if(args.ebpf) {
        network_viewer_load_ebpf(&args);

        if (ebpf_nv_module.maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd > 0) {
            nv_clean_thread = mallocz(sizeof(netdata_thread_t));
            netdata_thread_create(nv_clean_thread,
                                  "P[networkviewer ebpf]",
                                  NETDATA_THREAD_OPTION_JOINABLE,
                                  network_viewer_ebpf_worker,
                                  &ebpf_nv_module);
        }
    }
#endif

    // ----------------------------------------------------------------------------------------------------------------

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
        NETWORK_CONNECTIONS_VIEWER_FUNCTION, 60,
        NETWORK_CONNECTIONS_VIEWER_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

    // ----------------------------------------------------------------------------------------------------------------

    struct functions_evloop_globals *wg =
        functions_evloop_init(5, "Network-Viewer", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg, NETWORK_CONNECTIONS_VIEWER_FUNCTION,
                                  network_viewer_function,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);

    // ----------------------------------------------------------------------------------------------------------------

    usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    if (nv_clean_thread) {
        netdata_thread_join(*nv_clean_thread, NULL);
        freez(nv_clean_thread);
    }
    if (args.ebpf)
        network_viewer_unload_ebpf();
#endif

    return 0;
}
