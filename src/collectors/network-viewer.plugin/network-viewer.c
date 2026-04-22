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
#define NETWORK_TOPOLOGY_VIEWER_FUNCTION "topology:network-connections"
#define NETWORK_TOPOLOGY_VIEWER_HELP "Shows live network-connections topology with self/process/endpoint actors and ownership/socket links."
#define NETWORK_VIEWER_RESPONSE_UPDATE_EVERY 5
// Keep in sync with the topology schema contract used across topology producers.
#define NETWORK_TOPOLOGY_SCHEMA_VERSION "2.0"
#define NETWORK_TOPOLOGY_SOURCE "network-connections"
#define NETWORK_TOPOLOGY_LAYER "l7"
#define NV_TOPOLOGY_MAX_PPID_DEPTH 64

#define NV_TOPOLOGY_USERNAME_MAX 128
#define NV_TOPOLOGY_CMDLINE_MAX 512
#define NV_TOPOLOGY_KEY_MAX 1024

typedef struct {
    pid_t pid;
    pid_t ppid;
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
    uint64_t ppid;
    uint64_t uid;
    uint64_t net_ns_inode;
    char process[TASK_COMM_LEN + 1];
} NV_ENDPOINT_OWNER;

typedef struct {
    uint64_t pid;
    uint64_t ppid;
    uint64_t uid;
    uint64_t net_ns_inode;
    uint64_t sockets;
    uint64_t retransmissions;
    uint32_t max_rtt_usec;
    uint32_t max_rcv_rtt_usec;
    uint16_t local_port;
    uint16_t remote_port;
    uint16_t peer_port;
    uint16_t protocol_id;
    uint8_t direction_id;
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char protocol[8];
    char protocol_family[8];
    char direction[16];
    char state[32];
    char local_ip[INET6_ADDRSTRLEN];
    char remote_ip[INET6_ADDRSTRLEN];
    char peer_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char remote_address_space[16];
    char port_name[64];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
} NV_TOPOLOGY_LINK;

typedef struct {
    bool info_only;
    bool processes_by_pid;
    bool sockets_listening;
    bool sockets_local;
    bool sockets_inbound;
    bool sockets_outbound;
    bool protocols_ipv4_tcp;
    bool protocols_ipv6_tcp;
    bool protocols_ipv4_udp;
    bool protocols_ipv6_udp;
} NV_TOPOLOGY_OPTIONS;

typedef struct {
    DICTIONARY *process_actors;
    DICTIONARY *remote_actors;
    DICTIONARY *local_ips;
    DICTIONARY *endpoint_owners_exact;
    DICTIONARY *endpoint_owners_exact_any_ns;
    DICTIONARY *endpoint_owners_service;
    DICTIONARY *endpoint_owners_service_any_ns;
    DICTIONARY *links;
    usec_t now_ut;
    uint64_t sockets_total;
    uint64_t skipped_sockets;
    char hostname[256];
    char machine_guid[128];
    NV_TOPOLOGY_OPTIONS options;
} NV_TOPOLOGY_CONTEXT;

typedef struct {
    uint64_t ppid;
} NV_PPID_CACHE_ENTRY;

typedef struct {
    size_t process_actor_count;
    size_t socket_link_count;
    size_t local_ip_count;
    size_t endpoint_actor_count;
    size_t ownership_link_count;
    char host_actor_id[NV_TOPOLOGY_KEY_MAX];
} NV_TOPOLOGY_RENDER_STATE;

typedef struct nv_process_socket_row {
    const NV_TOPOLOGY_LINK *link;
    struct nv_process_socket_row *next;
} NV_PROCESS_SOCKET_ROW;

typedef struct {
    NV_PROCESS_SOCKET_ROW *head;
    NV_PROCESS_SOCKET_ROW *tail;
} NV_PROCESS_SOCKET_ROWS;

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
    { .id = TCP_FIN_WAIT1, .name = "fin-wait1" },
    { .id = TCP_FIN_WAIT2, .name = "fin-wait2" },
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

static inline void topology_options_defaults(NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return;

    memset(opts, 0, sizeof(*opts));
    opts->processes_by_pid = false; // default: by_name
    opts->sockets_listening = false;
    opts->sockets_local = false;
    opts->sockets_inbound = true;
    opts->sockets_outbound = true;
    opts->protocols_ipv4_tcp = true;
    opts->protocols_ipv6_tcp = true;
    opts->protocols_ipv4_udp = true;
    opts->protocols_ipv6_udp = true;
}

static inline bool topology_sockets_any_enabled(const NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return false;

    return (opts->sockets_listening ||
            opts->sockets_local ||
            opts->sockets_inbound ||
            opts->sockets_outbound);
}

static inline bool topology_protocols_any_enabled(const NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return false;

    return (opts->protocols_ipv4_tcp ||
            opts->protocols_ipv6_tcp ||
            opts->protocols_ipv4_udp ||
            opts->protocols_ipv6_udp);
}

static void topology_parse_options(const char *function, NV_TOPOLOGY_OPTIONS *opts) {
    topology_options_defaults(opts);
    if(!function || !*function || !opts)
        return;

    bool protocols_selected_explicitly = false;
    bool sockets_selected_explicitly = false;

    char *function_copy = strdupz(function);
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    for(size_t i = 1; i < num_words; i++) {
        char *param = get_word(words, num_words, i);
        if(!param || !*param)
            continue;

        if(strcmp(param, "info") == 0) {
            opts->info_only = true;
            continue;
        }

        if(strcmp(param, "processes:by_name") == 0 || strcmp(param, "processes:by-name") == 0) {
            opts->processes_by_pid = false;
            continue;
        }
        if(strcmp(param, "processes:by_pid") == 0 || strcmp(param, "processes:by-pid") == 0) {
            opts->processes_by_pid = true;
            continue;
        }

        if(strcmp(param, "endpoints:by_ip") == 0 || strcmp(param, "endpoints:by-ip") == 0)
            continue;

        if(strncmp(param, "sockets:", 8) == 0) {
            if(!sockets_selected_explicitly) {
                opts->sockets_listening = false;
                opts->sockets_local = false;
                opts->sockets_inbound = false;
                opts->sockets_outbound = false;
                sockets_selected_explicitly = true;
            }

            char *sockets_copy = strdupz(&param[8]);
            char *sockets_remaining = sockets_copy;
            char *socket_kind;
            while(sockets_remaining && *sockets_remaining &&
                  (socket_kind = strsep_skip_consecutive_separators(&sockets_remaining, ","))) {
                socket_kind = trim(socket_kind);
                if(!socket_kind || !*socket_kind)
                    continue;

                if(strcmp(socket_kind, "listening") == 0)
                    opts->sockets_listening = true;
                else if(strcmp(socket_kind, "local") == 0)
                    opts->sockets_local = true;
                else if(strcmp(socket_kind, "inbound") == 0)
                    opts->sockets_inbound = true;
                else if(strcmp(socket_kind, "outbound") == 0)
                    opts->sockets_outbound = true;
            }
            freez(sockets_copy);
            continue;
        }

        if(strncmp(param, "protocols:", 10) == 0) {
            if(!protocols_selected_explicitly) {
                opts->protocols_ipv4_tcp = false;
                opts->protocols_ipv6_tcp = false;
                opts->protocols_ipv4_udp = false;
                opts->protocols_ipv6_udp = false;
                protocols_selected_explicitly = true;
            }

            char *protocols_copy = strdupz(&param[10]);
            char *protocols_remaining = protocols_copy;
            char *protocol;
            while(protocols_remaining && *protocols_remaining &&
                  (protocol = strsep_skip_consecutive_separators(&protocols_remaining, ","))) {
                protocol = trim(protocol);
                if(!protocol || !*protocol)
                    continue;

                if(strcmp(protocol, "ipv4_tcp") == 0 || strcmp(protocol, "ipv4-tcp") == 0)
                    opts->protocols_ipv4_tcp = true;
                else if(strcmp(protocol, "ipv6_tcp") == 0 || strcmp(protocol, "ipv6-tcp") == 0)
                    opts->protocols_ipv6_tcp = true;
                else if(strcmp(protocol, "ipv4_udp") == 0 || strcmp(protocol, "ipv4-udp") == 0)
                    opts->protocols_ipv4_udp = true;
                else if(strcmp(protocol, "ipv6_udp") == 0 || strcmp(protocol, "ipv6-udp") == 0)
                    opts->protocols_ipv6_udp = true;
            }
            freez(protocols_copy);
        }
    }
    freez(function_copy);

    if(!topology_sockets_any_enabled(opts)) {
        opts->sockets_listening = false;
        opts->sockets_local = false;
        opts->sockets_inbound = true;
        opts->sockets_outbound = true;
    }

    if(!topology_protocols_any_enabled(opts)) {
        opts->protocols_ipv4_tcp = true;
        opts->protocols_ipv6_tcp = true;
        opts->protocols_ipv4_udp = true;
        opts->protocols_ipv6_udp = true;
    }
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

static inline void topology_format_ip_port(const char *ip, uint16_t port, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    if(!ip)
        ip = "";

    if(strchr(ip, ':'))
        snprintf(dst, dst_size, "[%s]:%u", ip, port);
    else
        snprintf(dst, dst_size, "%s:%u", ip, port);
}

static inline bool topology_ip_is_unspecified(const char *ip) {
    if(!ip || !*ip)
        return true;

    return (strcmp(ip, "*") == 0 || strcmp(ip, "0.0.0.0") == 0 || strcmp(ip, "::") == 0);
}

static inline bool topology_ip_is_self_range(const char *ip) {
    if(!ip || !*ip)
        return false;

    if(strncmp(ip, "127.", 4) == 0)
        return true;
    if(strncmp(ip, "0.", 2) == 0)
        return true;
    if(strcmp(ip, "::1") == 0)
        return true;
    if(strncmp(ip, "::ffff:127.", 10) == 0)
        return true;

    return false;
}

static inline bool topology_ip_belongs_to_self(const NV_TOPOLOGY_CONTEXT *ctx, const char *ip, const char *address_space) {
    if(topology_ip_is_unspecified(ip))
        return true;

    if(topology_ip_is_self_range(ip))
        return true;

    if(address_space && *address_space) {
        if(strcmp(address_space, "loopback") == 0 || strcmp(address_space, "zero") == 0)
            return true;
    }

    if(ctx && ctx->local_ips && ip && *ip && dictionary_get(ctx->local_ips, ip))
        return true;

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
        buffer_json_member_add_string(wb, "process_name", pa->process);
        if(ctx && ctx->options.processes_by_pid) {
            buffer_json_member_add_uint64(wb, "pid", pa->pid);
            buffer_json_member_add_uint64(wb, "uid", pa->uid);
            buffer_json_member_add_uint64(wb, "net_ns_inode", pa->net_ns_inode);
        }
    }
    buffer_json_object_close(wb);
}

static void topology_add_process_identity_match(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx, uint64_t pid, uint64_t uid, uint64_t net_ns_inode, const char *process_name) {
    buffer_json_member_add_object(wb, "match");
    {
        buffer_json_member_add_string(wb, "process_name", process_name && *process_name ? process_name : "[unknown]");
        if(ctx && ctx->options.processes_by_pid) {
            buffer_json_member_add_uint64(wb, "pid", pid);
            buffer_json_member_add_uint64(wb, "uid", uid);
            buffer_json_member_add_uint64(wb, "net_ns_inode", net_ns_inode);
        }
    }
    buffer_json_object_close(wb);
}

static inline void topology_process_parent_lookup_key(
    char *dst,
    size_t dst_size,
    uint64_t pid,
    uint64_t net_ns_inode,
    bool include_ns
) {
    if(!dst || !dst_size)
        return;

    if(include_ns)
        snprintf(dst, dst_size, "ns=%llu|pid=%llu",
                 (unsigned long long)net_ns_inode,
                 (unsigned long long)pid);
    else
        snprintf(dst, dst_size, "pid=%llu",
                 (unsigned long long)pid);
}

static inline void topology_pid_lookup_key(char *dst, size_t dst_size, uint64_t pid) {
    if(!dst || !dst_size)
        return;

    snprintf(dst, dst_size, "pid=%llu", (unsigned long long)pid);
}

static bool topology_read_proc_ppid(uint64_t pid, uint64_t *ppid) {
    if(!pid || !ppid)
        return false;

    char filename[FILENAME_MAX + 1];
    char status_buf[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%llu/status",
              netdata_configured_host_prefix,
              (unsigned long long)pid);

    if(read_txt_file(filename, status_buf, sizeof(status_buf)))
        return false;

    char *p = strstr(status_buf, "PPid:");
    if(!p)
        return false;

    p += 5;
    while(isspace((unsigned char)*p))
        p++;

    if(*p < '0' || *p > '9')
        return false;

    uint64_t parent = strtoull(p, NULL, 10);
    if(parent == pid)
        parent = 0;

    *ppid = parent;
    return true;
}

static uint64_t topology_ppid_cache_get_or_load(DICTIONARY *ppid_cache, uint64_t pid) {
    if(!ppid_cache || !pid)
        return 0;

    char pid_key[64];
    topology_pid_lookup_key(pid_key, sizeof(pid_key), pid);

    NV_PPID_CACHE_ENTRY *cached = dictionary_get(ppid_cache, pid_key);
    if(cached)
        return cached->ppid;

    NV_PPID_CACHE_ENTRY tmp = { .ppid = 0 };
    uint64_t ppid = 0;
    if(topology_read_proc_ppid(pid, &ppid))
        tmp.ppid = ppid;

    cached = dictionary_set(ppid_cache, pid_key, &tmp, sizeof(tmp));
    return cached ? cached->ppid : tmp.ppid;
}

static NV_ENDPOINT_OWNER *topology_find_process_parent_actor(
    uint64_t ppid,
    uint64_t net_ns_inode,
    DICTIONARY *process_parent_ns_lookup,
    DICTIONARY *process_parent_any_lookup,
    DICTIONARY *ppid_cache
) {
    if(!ppid || !process_parent_ns_lookup || !process_parent_any_lookup)
        return NULL;

    uint64_t current_pid = ppid;
    for(size_t depth = 0; depth < NV_TOPOLOGY_MAX_PPID_DEPTH && current_pid; depth++) {
        char parent_key_ns[NV_TOPOLOGY_KEY_MAX];
        char parent_key_any[NV_TOPOLOGY_KEY_MAX];

        topology_process_parent_lookup_key(parent_key_ns, sizeof(parent_key_ns), current_pid, net_ns_inode, true);
        NV_ENDPOINT_OWNER *parent = dictionary_get(process_parent_ns_lookup, parent_key_ns);
        if(!parent) {
            topology_process_parent_lookup_key(parent_key_any, sizeof(parent_key_any), current_pid, 0, false);
            parent = dictionary_get(process_parent_any_lookup, parent_key_any);
        }

        if(parent)
            return parent;

        if(!ppid_cache)
            break;

        uint64_t next_ppid = topology_ppid_cache_get_or_load(ppid_cache, current_pid);
        if(!next_ppid || next_ppid == current_pid)
            break;

        current_pid = next_ppid;
    }

    return NULL;
}

static void topology_endpoint_owner_exact_key(char *dst, size_t dst_size, uint64_t net_ns_inode, uint16_t protocol, const char *ip, uint16_t port, bool include_ns) {
    if(!dst || !dst_size)
        return;

    if(include_ns)
        snprintf(dst, dst_size, "ns=%llu|proto=%u|ip=%s|port=%u",
                 (unsigned long long)net_ns_inode, (unsigned)protocol, ip, (unsigned)port);
    else
        snprintf(dst, dst_size, "proto=%u|ip=%s|port=%u", (unsigned)protocol, ip, (unsigned)port);
}

static void topology_endpoint_owner_service_key(char *dst, size_t dst_size, uint64_t net_ns_inode, uint16_t protocol, uint16_t port, bool include_ns) {
    if(!dst || !dst_size)
        return;

    if(include_ns)
        snprintf(dst, dst_size, "ns=%llu|proto=%u|port=%u",
                 (unsigned long long)net_ns_inode, (unsigned)protocol, (unsigned)port);
    else
        snprintf(dst, dst_size, "proto=%u|port=%u", (unsigned)protocol, (unsigned)port);
}

static void topology_register_endpoint_owner(
    NV_TOPOLOGY_CONTEXT *ctx,
    uint64_t net_ns_inode,
    uint16_t protocol,
    const char *ip,
    uint16_t port,
    const NV_PROCESS_ACTOR *pa,
    bool service_candidate
) {
    if(!ctx || !pa || !port)
        return;

    NV_ENDPOINT_OWNER owner = {
        .pid = pa->pid,
        .ppid = pa->ppid,
        .uid = pa->uid,
        .net_ns_inode = pa->net_ns_inode,
    };
    snprintf(owner.process, sizeof(owner.process), "%s", pa->process);

    char key[NV_TOPOLOGY_KEY_MAX];
    if(ip && *ip && strcmp(ip, "*") != 0) {
        if(ctx->endpoint_owners_exact) {
            topology_endpoint_owner_exact_key(key, sizeof(key), net_ns_inode, protocol, ip, port, true);
            dictionary_set(ctx->endpoint_owners_exact, key, &owner, sizeof(owner));
        }
        if(ctx->endpoint_owners_exact_any_ns) {
            topology_endpoint_owner_exact_key(key, sizeof(key), 0, protocol, ip, port, false);
            dictionary_set(ctx->endpoint_owners_exact_any_ns, key, &owner, sizeof(owner));
        }
    }

    if(service_candidate) {
        if(ctx->endpoint_owners_service) {
            topology_endpoint_owner_service_key(key, sizeof(key), net_ns_inode, protocol, port, true);
            dictionary_set(ctx->endpoint_owners_service, key, &owner, sizeof(owner));
        }
        if(ctx->endpoint_owners_service_any_ns) {
            topology_endpoint_owner_service_key(key, sizeof(key), 0, protocol, port, false);
            dictionary_set(ctx->endpoint_owners_service_any_ns, key, &owner, sizeof(owner));
        }
    }
}

static NV_ENDPOINT_OWNER *topology_lookup_endpoint_owner(
    const NV_TOPOLOGY_CONTEXT *ctx,
    uint64_t net_ns_inode,
    uint16_t protocol,
    const char *ip,
    uint16_t port,
    bool allow_service_fallback
) {
    if(!ctx || !port)
        return NULL;

    char key[NV_TOPOLOGY_KEY_MAX];
    NV_ENDPOINT_OWNER *owner = NULL;

    if(ip && *ip && strcmp(ip, "*") != 0) {
        if(ctx->endpoint_owners_exact) {
            topology_endpoint_owner_exact_key(key, sizeof(key), net_ns_inode, protocol, ip, port, true);
            owner = dictionary_get(ctx->endpoint_owners_exact, key);
            if(owner)
                return owner;
        }

        if(ctx->endpoint_owners_exact_any_ns) {
            topology_endpoint_owner_exact_key(key, sizeof(key), 0, protocol, ip, port, false);
            owner = dictionary_get(ctx->endpoint_owners_exact_any_ns, key);
            if(owner)
                return owner;
        }
    }

    if(!allow_service_fallback)
        return NULL;

    if(ctx->endpoint_owners_service) {
        topology_endpoint_owner_service_key(key, sizeof(key), net_ns_inode, protocol, port, true);
        owner = dictionary_get(ctx->endpoint_owners_service, key);
        if(owner)
            return owner;
    }

    if(ctx->endpoint_owners_service_any_ns) {
        topology_endpoint_owner_service_key(key, sizeof(key), 0, protocol, port, false);
        owner = dictionary_get(ctx->endpoint_owners_service_any_ns, key);
        if(owner)
            return owner;
    }

    return NULL;
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
                if(topology_ip_is_unspecified(lip->ip)) continue;
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

static void topology_encode_identifier_component(char *dst, size_t dst_size, const char *src) {
    if(!dst || !dst_size)
        return;

    const uint8_t *s = (const uint8_t *)((src && *src) ? src : "[unknown]");
    size_t written = 0;

    while(*s && written + 1 < dst_size) {
        uint8_t ch = *s++;

        if(isalnum(ch) || ch == '-' || ch == '_' || ch == '.' || ch == '~') {
            dst[written++] = (char)ch;
            continue;
        }

        if(written + 3 >= dst_size)
            break;

        dst[written++] = '%';
        dst[written++] = hex_digits_lower[(ch >> 4) & 0x0F];
        dst[written++] = hex_digits_lower[ch & 0x0F];
    }

    dst[written] = '\0';
}

static void topology_actor_id_for_host(const NV_TOPOLOGY_CONTEXT *ctx, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    if(ctx && ctx->machine_guid[0])
        snprintf(dst, dst_size, "netdata-machine-guid:%s", ctx->machine_guid);
    else if(ctx && ctx->hostname[0])
        snprintf(dst, dst_size, "hostname:%s", ctx->hostname);
    else
        snprintf(dst, dst_size, "host:unknown");
}

static void topology_actor_id_for_process(
    const NV_TOPOLOGY_CONTEXT *ctx,
    uint64_t pid,
    uint64_t uid,
    uint64_t net_ns_inode,
    const char *process,
    char *dst,
    size_t dst_size
) {
    if(!dst || !dst_size)
        return;

    const char *node_identity = (ctx && ctx->machine_guid[0]) ? ctx->machine_guid : (ctx && ctx->hostname[0] ? ctx->hostname : "unknown");
    const char *safe_process = (process && *process) ? process : "[unknown]";
    if(ctx && !ctx->options.processes_by_pid) {
        char encoded_process[((TASK_COMM_LEN + 1) * 3) + 1];
        topology_encode_identifier_component(encoded_process, sizeof(encoded_process), safe_process);
        snprintf(dst, dst_size, "process:%s|comm=%s", node_identity, encoded_process);
    }
    else {
        snprintf(dst, dst_size, "process:%s|pid=%llu|uid=%llu|ns=%llu",
                 node_identity,
                 (unsigned long long)pid,
                 (unsigned long long)uid,
                 (unsigned long long)net_ns_inode);
    }
}

static bool topology_process_name_is_unknown(const char *process_name) {
    return !process_name || !*process_name || strcmp(process_name, "[unknown]") == 0;
}

static void topology_actor_id_for_remote_endpoint(const NV_TOPOLOGY_CONTEXT *ctx, const char *ip, const char *address_space, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    (void)ctx;
    (void)address_space;
    if(ip && *ip)
        snprintf(dst, dst_size, "ip:%s", ip);
    else
        snprintf(dst, dst_size, "ip:unknown");
}

static void topology_process_display_name(
    const NV_TOPOLOGY_CONTEXT *ctx,
    const char *process_name,
    uint64_t pid,
    char *dst,
    size_t dst_size
) {
    if(!dst || !dst_size)
        return;

    const char *safe_process = (process_name && *process_name) ? process_name : "[unknown]";
    if(ctx && !ctx->options.processes_by_pid)
        snprintf(dst, dst_size, "%s", safe_process);
    else
        snprintf(dst, dst_size, "%s[%llu]", safe_process, (unsigned long long)pid);
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
    char remote_peer_ip[INET6_ADDRSTRLEN] = "";

    if(is_local_socket_ipv46(n))
        strncpyz(local_ip, "*", sizeof(local_ip) - 1);
    else if(!socket_endpoint_to_ip_text(&n->local, local_ip))
        return;

    if(!local_sockets_is_zero_address(&n->remote)) {
        socket_endpoint_to_ip_text(&n->remote, remote_ip);
        snprintf(remote_peer_ip, sizeof(remote_peer_ip), "%s", remote_ip);
    }

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

    if(local_ip[0] && !topology_ip_is_unspecified(local_ip)) {
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
    if(ctx->options.processes_by_pid) {
        snprintf(process_key, sizeof(process_key), "pid=%d|uid=%u|ns=%llu",
                 n->pid,
                 (unsigned)n->uid,
                 (unsigned long long)n->net_ns_inode);
    }
    else {
        char encoded_process[((TASK_COMM_LEN + 1) * 3) + 1];
        topology_encode_identifier_component(encoded_process, sizeof(encoded_process), process_name);
        snprintf(process_key, sizeof(process_key), "comm=%s", encoded_process);
    }

    NV_PROCESS_ACTOR *pa = dictionary_get(ctx->process_actors, process_key);
    if(!pa) {
        NV_PROCESS_ACTOR tmp = { 0 };
        tmp.pid = n->pid;
        tmp.ppid = n->ppid;
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
    if(!pa->ppid && n->ppid)
        pa->ppid = n->ppid;
    if(topology_process_name_is_unknown(pa->process) && !topology_process_name_is_unknown(process_name))
        snprintf(pa->process, sizeof(pa->process), "%s", process_name);
    if(!pa->cmdline[0] && cmdline && *cmdline)
        snprintf(pa->cmdline, sizeof(pa->cmdline), "%s", cmdline);
    if((!pa->local_ip[0] || topology_ip_is_unspecified(pa->local_ip)) && local_ip[0] && !topology_ip_is_unspecified(local_ip))
        snprintf(pa->local_ip, sizeof(pa->local_ip), "%s", local_ip);

    bool service_candidate = (n->direction == SOCKET_DIRECTION_LISTEN ||
                              n->direction == SOCKET_DIRECTION_INBOUND ||
                              n->direction == SOCKET_DIRECTION_LOCAL_INBOUND);
    topology_register_endpoint_owner(ctx, n->net_ns_inode, n->local.protocol, local_ip, n->local.port, pa, service_candidate);

    if(!remote_ip[0]) {
        if(n->direction == SOCKET_DIRECTION_LISTEN || n->direction == SOCKET_DIRECTION_LOCAL_INBOUND) {
            if(strcmp(local_ip, "*") == 0 || local_sockets_is_zero_address(&n->local)) {
                ctx->skipped_sockets++;
                return;
            }
            snprintf(remote_ip, sizeof(remote_ip), "%s", local_ip);
            remote_address_space = local_address_space;
        }
        else {
            ctx->skipped_sockets++;
            return;
        }
    }

    if(topology_ip_is_unspecified(remote_ip)) {
        ctx->skipped_sockets++;
        return;
    }

    bool remote_is_self = topology_ip_belongs_to_self(ctx, remote_ip, remote_address_space);
    bool create_endpoint_actor = (!remote_is_self || n->direction == SOCKET_DIRECTION_LISTEN);
    if(create_endpoint_actor) {
        char endpoint_actor_key[NV_TOPOLOGY_KEY_MAX];
        topology_actor_id_for_remote_endpoint(ctx, remote_ip, remote_address_space, endpoint_actor_key, sizeof(endpoint_actor_key));
        NV_REMOTE_ACTOR *ra = dictionary_get(ctx->remote_actors, endpoint_actor_key);
        if(!ra) {
            NV_REMOTE_ACTOR tmp = { 0 };
            snprintf(tmp.ip, sizeof(tmp.ip), "%s", remote_ip);
            snprintf(tmp.address_space, sizeof(tmp.address_space), "%s", remote_address_space);
            ra = dictionary_set(ctx->remote_actors, endpoint_actor_key, &tmp, sizeof(tmp));
        }
        ra->sockets++;
    }

    const struct socket_endpoint *server_endpoint = NULL;
    uint16_t endpoint_port = n->remote.port;
    switch(n->direction) {
        case SOCKET_DIRECTION_LISTEN:
        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
            server_endpoint = &n->local;
            endpoint_port = n->local.port;
            break;

        case SOCKET_DIRECTION_OUTBOUND:
        case SOCKET_DIRECTION_LOCAL_OUTBOUND:
            server_endpoint = &n->remote;
            endpoint_port = n->remote.port;
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
             endpoint_port);

    NV_TOPOLOGY_LINK *link = dictionary_get(ctx->links, link_key);
    if(!link) {
        NV_TOPOLOGY_LINK tmp = { 0 };
        tmp.pid = n->pid;
        tmp.ppid = n->ppid;
        tmp.uid = n->uid;
        tmp.net_ns_inode = n->net_ns_inode;
        tmp.local_port = n->local.port;
        tmp.remote_port = endpoint_port;
        tmp.peer_port = n->remote.port;
        tmp.protocol_id = n->local.protocol;
        tmp.direction_id = (uint8_t)n->direction;
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
        snprintf(tmp.peer_ip, sizeof(tmp.peer_ip), "%s", remote_peer_ip);
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

static void topology_context_destroy(NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx)
        return;

    if(ctx->links)
        dictionary_destroy(ctx->links);
    if(ctx->endpoint_owners_service_any_ns)
        dictionary_destroy(ctx->endpoint_owners_service_any_ns);
    if(ctx->endpoint_owners_service)
        dictionary_destroy(ctx->endpoint_owners_service);
    if(ctx->endpoint_owners_exact_any_ns)
        dictionary_destroy(ctx->endpoint_owners_exact_any_ns);
    if(ctx->endpoint_owners_exact)
        dictionary_destroy(ctx->endpoint_owners_exact);
    if(ctx->local_ips)
        dictionary_destroy(ctx->local_ips);
    if(ctx->remote_actors)
        dictionary_destroy(ctx->remote_actors);
    if(ctx->process_actors)
        dictionary_destroy(ctx->process_actors);
}

static DICTIONARY *topology_build_process_socket_index(const NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx || !ctx->links)
        return NULL;

    DICTIONARY *index = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(NV_PROCESS_SOCKET_ROWS));
    if(!index)
        return NULL;

    NV_TOPOLOGY_LINK *link;
    dfe_start_read(ctx->links, link) {
        char process_actor_id[NV_TOPOLOGY_KEY_MAX];
        topology_actor_id_for_process(ctx, link->pid, link->uid, link->net_ns_inode, link->process, process_actor_id, sizeof(process_actor_id));

        NV_PROCESS_SOCKET_ROWS *rows = dictionary_get(index, process_actor_id);
        if(!rows) {
            NV_PROCESS_SOCKET_ROWS tmp = { 0 };
            rows = dictionary_set(index, process_actor_id, &tmp, sizeof(tmp));
        }

        if(!rows)
            continue;

        NV_PROCESS_SOCKET_ROW *row = callocz(1, sizeof(*row));
        row->link = link;

        if(rows->tail)
            rows->tail->next = row;
        else
            rows->head = row;
        rows->tail = row;
    }
    dfe_done(link);

    return index;
}

static void topology_destroy_process_socket_index(DICTIONARY *index) {
    if(!index)
        return;

    NV_PROCESS_SOCKET_ROWS *rows;
    dfe_start_read(index, rows) {
        NV_PROCESS_SOCKET_ROW *row = rows->head;
        while(row) {
            NV_PROCESS_SOCKET_ROW *next = row->next;
            freez(row);
            row = next;
        }
    }
    dfe_done(rows);

    dictionary_destroy(index);
}

static bool topology_prepare_context(NV_TOPOLOGY_CONTEXT *ctx, usec_t now_ut, const NV_TOPOLOGY_OPTIONS *options) {
    if(!ctx)
        return false;

    memset(ctx, 0, sizeof(*ctx));
    ctx->now_ut = now_ut;
    if(options)
        ctx->options = *options;

    if(ctx->options.info_only)
        return true;

    ctx->process_actors = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_PROCESS_ACTOR));
    ctx->remote_actors = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_REMOTE_ACTOR));
    ctx->local_ips = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_LOCAL_IP));
    ctx->endpoint_owners_exact = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_ENDPOINT_OWNER));
    ctx->endpoint_owners_exact_any_ns = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_ENDPOINT_OWNER));
    ctx->endpoint_owners_service = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_ENDPOINT_OWNER));
    ctx->endpoint_owners_service_any_ns = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_ENDPOINT_OWNER));
    ctx->links = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_TOPOLOGY_LINK));

    if(!(ctx->process_actors && ctx->remote_actors && ctx->local_ips &&
         ctx->endpoint_owners_exact && ctx->endpoint_owners_exact_any_ns &&
         ctx->endpoint_owners_service && ctx->endpoint_owners_service_any_ns &&
         ctx->links))
        return false;

    if(!os_hostname(ctx->hostname, sizeof(ctx->hostname), netdata_configured_host_prefix))
        snprintf(ctx->hostname, sizeof(ctx->hostname), "%s", "localhost");

    const char *machine_guid = network_viewer_machine_guid();
    if(machine_guid)
        snprintf(ctx->machine_guid, sizeof(ctx->machine_guid), "%s", machine_guid);

    LS_STATE ls = {
        .config = {
            .listening = ctx->options.sockets_listening,
            .local = ctx->options.sockets_local,
            .inbound = ctx->options.sockets_inbound,
            .outbound = ctx->options.sockets_outbound,
            .tcp4 = ctx->options.protocols_ipv4_tcp,
            .tcp6 = ctx->options.protocols_ipv6_tcp,
            .udp4 = ctx->options.protocols_ipv4_udp,
            .udp6 = ctx->options.protocols_ipv6_udp,
            .pid = true,
            .uid = true,
            .cmdline = true,
            .comm = true,
            .namespaces = true,
            .tcp_info = true,
            .max_errors = 10,
            .max_concurrent_namespaces = 5,
            .cb = local_sockets_cb_to_topology,
            .data = ctx,
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
    return true;
}

static void topology_render_state_init(NV_TOPOLOGY_RENDER_STATE *state, const NV_TOPOLOGY_CONTEXT *ctx) {
    if(!state)
        return;

    memset(state, 0, sizeof(*state));
    if(!ctx)
        return;

    state->process_actor_count = ctx->process_actors ? dictionary_entries(ctx->process_actors) : 0;
    state->socket_link_count = ctx->links ? dictionary_entries(ctx->links) : 0;
    state->local_ip_count = ctx->local_ips ? dictionary_entries(ctx->local_ips) : 0;
    topology_actor_id_for_host(ctx, state->host_actor_id, sizeof(state->host_actor_id));
}

static void topology_finalize_response(const char *transaction, BUFFER *wb, time_t now_s) {
    buffer_json_member_add_time_t(wb, "expires", now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY;
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

static void topology_write_response_metadata(BUFFER *wb) {
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "topology");
    buffer_json_member_add_time_t(wb, "update_every", NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_TOPOLOGY_VIEWER_HELP);
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "info");
        buffer_json_add_array_item_string(wb, "processes");
        buffer_json_add_array_item_string(wb, "sockets");
        buffer_json_add_array_item_string(wb, "protocols");
        buffer_json_add_array_item_string(wb, "endpoints");
    }
    buffer_json_array_close(wb);
    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "processes");
            buffer_json_member_add_string(wb, "name", "Processes");
            buffer_json_member_add_string(wb, "help", "Group process actors by process name or by PID.");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "by_name");
                    buffer_json_member_add_string(wb, "name", "Processes by Name");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "by_pid");
                    buffer_json_member_add_string(wb, "name", "Processes by PID");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "sockets");
            buffer_json_member_add_string(wb, "name", "Sockets");
            buffer_json_member_add_string(wb, "help", "Select one or more socket directions.");
            buffer_json_member_add_string(wb, "type", "multiselect");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "listening");
                    buffer_json_member_add_string(wb, "name", "Listening");
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "local");
                    buffer_json_member_add_string(wb, "name", "Local");
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "inbound");
                    buffer_json_member_add_string(wb, "name", "Inbound");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "outbound");
                    buffer_json_member_add_string(wb, "name", "Outbound");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "protocols");
            buffer_json_member_add_string(wb, "name", "Protocols");
            buffer_json_member_add_string(wb, "help", "Select one or more socket protocol families.");
            buffer_json_member_add_string(wb, "type", "multiselect");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "ipv4_tcp");
                    buffer_json_member_add_string(wb, "name", "IPv4 TCP");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "ipv6_tcp");
                    buffer_json_member_add_string(wb, "name", "IPv6 TCP");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "ipv4_udp");
                    buffer_json_member_add_string(wb, "name", "IPv4 UDP");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "ipv6_udp");
                    buffer_json_member_add_string(wb, "name", "IPv6 UDP");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "endpoints");
            buffer_json_member_add_string(wb, "name", "Endpoints");
            buffer_json_member_add_string(wb, "help", "Keep non-private endpoints by IP until AS grouping is available.");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "by_ip");
                    buffer_json_member_add_string(wb, "name", "Non-Private Endpoints by IP");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);
}

static void topology_write_presentation(BUFFER *wb) {
    buffer_json_member_add_object(wb, "presentation");
    {
        buffer_json_member_add_object(wb, "actor_types");
        {
            buffer_json_member_add_object(wb, "self");
            {
                buffer_json_member_add_string(wb, "label", "This host");
                buffer_json_member_add_string(wb, "color_slot", "self");
                buffer_json_member_add_boolean(wb, "border", true);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", true);

                buffer_json_member_add_array(wb, "summary_fields");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "hostname");
                    buffer_json_member_add_string(wb, "label", "Hostname");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.hostname");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "local_ip_count");
                    buffer_json_member_add_string(wb, "label", "Local IPs");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.local_ip_count");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "observed_sockets");
                    buffer_json_member_add_string(wb, "label", "Sockets");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.observed_sockets");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);

                buffer_json_member_add_object(wb, "tables");
                {
                    buffer_json_member_add_object(wb, "links");
                    {
                        buffer_json_member_add_string(wb, "label", "Connections");
                        buffer_json_member_add_string(wb, "source", "links");
                        buffer_json_member_add_array(wb, "columns");
                        {
                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "remoteLabel");
                            buffer_json_member_add_string(wb, "label", "Remote");
                            buffer_json_member_add_string(wb, "type", "actor_link");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "protocol");
                            buffer_json_member_add_string(wb, "label", "Protocol");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "direction");
                            buffer_json_member_add_string(wb, "label", "Direction");
                            buffer_json_object_close(wb);
                        }
                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_array(wb, "modal_tabs");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", "info");
                    buffer_json_member_add_string(wb, "label", "Info");
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "process");
            {
                buffer_json_member_add_string(wb, "label", "Process");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_boolean(wb, "border", true);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", true);
                buffer_json_member_add_boolean(wb, "show_port_bullets", true);

                buffer_json_member_add_array(wb, "summary_fields");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "display_name");
                    buffer_json_member_add_string(wb, "label", "Process");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.display_name");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "cmdline");
                    buffer_json_member_add_string(wb, "label", "Command");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.cmdline");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "socket_count");
                    buffer_json_member_add_string(wb, "label", "Sockets");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.socket_count");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "local_ip");
                    buffer_json_member_add_string(wb, "label", "Local IP");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.local_ip");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "user");
                    buffer_json_member_add_string(wb, "label", "User");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "labels.user");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);

                buffer_json_member_add_object(wb, "tables");
                {
                    buffer_json_member_add_object(wb, "sockets");
                    {
                        buffer_json_member_add_string(wb, "label", "Sockets");
                        buffer_json_member_add_string(wb, "source", "data");
                        buffer_json_member_add_boolean(wb, "bullet_source", true);
                        buffer_json_member_add_uint64(wb, "order", 0);
                        buffer_json_member_add_array(wb, "columns");
                        {
                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "remote");
                            buffer_json_member_add_string(wb, "label", "Remote");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "protocol");
                            buffer_json_member_add_string(wb, "label", "Protocol");
                            buffer_json_member_add_string(wb, "type", "badge");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "direction");
                            buffer_json_member_add_string(wb, "label", "Direction");
                            buffer_json_member_add_string(wb, "type", "badge");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "state");
                            buffer_json_member_add_string(wb, "label", "State");
                            buffer_json_member_add_string(wb, "type", "badge");
                            buffer_json_object_close(wb);
                        }
                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);

                    buffer_json_member_add_object(wb, "links");
                    {
                        buffer_json_member_add_string(wb, "label", "Connections");
                        buffer_json_member_add_string(wb, "source", "links");
                        buffer_json_member_add_uint64(wb, "order", 1);
                        buffer_json_member_add_array(wb, "columns");
                        {
                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "remoteLabel");
                            buffer_json_member_add_string(wb, "label", "Remote");
                            buffer_json_member_add_string(wb, "type", "actor_link");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "protocol");
                            buffer_json_member_add_string(wb, "label", "Protocol");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "direction");
                            buffer_json_member_add_string(wb, "label", "Direction");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "state");
                            buffer_json_member_add_string(wb, "label", "State");
                            buffer_json_object_close(wb);
                        }
                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_array(wb, "modal_tabs");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", "info");
                    buffer_json_member_add_string(wb, "label", "Info");
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "endpoint");
            {
                buffer_json_member_add_string(wb, "label", "Endpoint");
                buffer_json_member_add_string(wb, "color_slot", "derived");
                buffer_json_member_add_boolean(wb, "border", true);
                buffer_json_member_add_string(wb, "role", "endpoint");

                buffer_json_member_add_array(wb, "summary_fields");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "display_name");
                    buffer_json_member_add_string(wb, "label", "IP Address");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.display_name");
                    buffer_json_add_array_item_string(wb, "match.ip_addresses.0");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "socket_count");
                    buffer_json_member_add_string(wb, "label", "Sockets");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "attributes.socket_count");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);

                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "key", "address_space");
                    buffer_json_member_add_string(wb, "label", "Address Space");
                    buffer_json_member_add_array(wb, "sources");
                    buffer_json_add_array_item_string(wb, "labels.address_space");
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);

                buffer_json_member_add_object(wb, "tables");
                {
                    buffer_json_member_add_object(wb, "links");
                    {
                        buffer_json_member_add_string(wb, "label", "Connections");
                        buffer_json_member_add_string(wb, "source", "links");
                        buffer_json_member_add_array(wb, "columns");
                        {
                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "remoteLabel");
                            buffer_json_member_add_string(wb, "label", "Remote");
                            buffer_json_member_add_string(wb, "type", "actor_link");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "protocol");
                            buffer_json_member_add_string(wb, "label", "Protocol");
                            buffer_json_object_close(wb);

                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "key", "direction");
                            buffer_json_member_add_string(wb, "label", "Direction");
                            buffer_json_object_close(wb);
                        }
                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_array(wb, "modal_tabs");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", "info");
                    buffer_json_member_add_string(wb, "label", "Info");
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "link_types");
        {
            buffer_json_member_add_object(wb, "ownership");
            {
                buffer_json_member_add_string(wb, "label", "Ownership");
                buffer_json_member_add_string(wb, "color_slot", "muted");
                buffer_json_member_add_boolean(wb, "dash", true);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "socket");
            {
                buffer_json_member_add_string(wb, "label", "Socket");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "width", 1.5);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_array(wb, "port_fields");
        {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "key", "type");
            buffer_json_member_add_string(wb, "label", "Type");
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "port_types");
        {
            buffer_json_member_add_object(wb, "topology");
            {
                buffer_json_member_add_string(wb, "label", "Socket");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "opacity", 1.0);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "legend");
        {
            buffer_json_member_add_array(wb, "actors");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "self");
                buffer_json_member_add_string(wb, "label", "This host");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "process");
                buffer_json_member_add_string(wb, "label", "Process");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "endpoint");
                buffer_json_member_add_string(wb, "label", "Endpoint");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);

            buffer_json_member_add_array(wb, "links");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "ownership");
                buffer_json_member_add_string(wb, "label", "Ownership");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "socket");
                buffer_json_member_add_string(wb, "label", "Socket");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);

            buffer_json_member_add_array(wb, "ports");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "topology");
                buffer_json_member_add_string(wb, "label", "Socket");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_string(wb, "actor_click_behavior", "highlight_connections");
    }
    buffer_json_object_close(wb);
}

static void topology_write_actors(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx, NV_TOPOLOGY_RENDER_STATE *state) {
    DICTIONARY *process_socket_index = topology_build_process_socket_index(ctx);

    buffer_json_member_add_array(wb, "actors");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "actor_id", state->host_actor_id);
            buffer_json_member_add_string(wb, "actor_type", "self");
            buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
            buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
            topology_add_host_match(wb, ctx);

            buffer_json_member_add_object(wb, "attributes");
            {
                buffer_json_member_add_string(wb, "hostname", ctx->hostname);
                buffer_json_member_add_uint64(wb, "local_ip_count", state->local_ip_count);
                buffer_json_member_add_uint64(wb, "observed_sockets", ctx->sockets_total);
                buffer_json_member_add_string(wb, "display_name", ctx->hostname);
                buffer_json_member_add_string(wb, "actor_class", "self");
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "labels");
            {
                buffer_json_member_add_string(wb, "hostname", ctx->hostname);
                if(ctx->machine_guid[0])
                    buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);
                buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                buffer_json_member_add_string(wb, "display_name", ctx->hostname);
                buffer_json_member_add_string(wb, "actor_class", "self");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        NV_PROCESS_ACTOR *pa;
        dfe_start_read(ctx->process_actors, pa) {
            char process_actor_id[NV_TOPOLOGY_KEY_MAX];
            char process_display_name[NV_TOPOLOGY_KEY_MAX];
            topology_actor_id_for_process(ctx, pa->pid, pa->uid, pa->net_ns_inode, pa->process, process_actor_id, sizeof(process_actor_id));
            topology_process_display_name(ctx, pa->process, pa->pid, process_display_name, sizeof(process_display_name));
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "actor_id", process_actor_id);
                buffer_json_member_add_string(wb, "actor_type", "process");
                buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                topology_add_process_match(wb, ctx, pa);

                buffer_json_member_add_object(wb, "parent_match");
                {
                    if(ctx->machine_guid[0])
                        buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);
                    topology_add_single_item_string_array(wb, "hostnames", ctx->hostname);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "attributes");
                {
                    if(ctx->options.processes_by_pid) {
                        buffer_json_member_add_uint64(wb, "pid", pa->pid);
                        buffer_json_member_add_uint64(wb, "ppid", pa->ppid);
                        buffer_json_member_add_uint64(wb, "uid", pa->uid);
                        buffer_json_member_add_uint64(wb, "net_ns_inode", pa->net_ns_inode);
                    }
                    buffer_json_member_add_uint64(wb, "socket_count", pa->sockets);
                    buffer_json_member_add_string(wb, "local_ip", pa->local_ip);
                    buffer_json_member_add_string(wb, "local_address_space", pa->local_address_space);
                    buffer_json_member_add_string(wb, "display_name", process_display_name);
                    buffer_json_member_add_string(wb, "actor_class", "process");
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
                    buffer_json_member_add_string(wb, "display_name", process_display_name);
                    buffer_json_member_add_string(wb, "actor_class", "process");
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "tables");
                {
                    buffer_json_member_add_array(wb, "sockets");
                    {
                        NV_PROCESS_SOCKET_ROWS *rows = process_socket_index ? dictionary_get(process_socket_index, process_actor_id) : NULL;
                        for(NV_PROCESS_SOCKET_ROW *row = rows ? rows->head : NULL; row; row = row->next) {
                            char remote_endpoint[128];
                            topology_format_ip_port(row->link->remote_ip, row->link->remote_port, remote_endpoint, sizeof(remote_endpoint));
                            buffer_json_add_array_item_object(wb);
                            buffer_json_member_add_string(wb, "remote", remote_endpoint);
                            buffer_json_member_add_string(wb, "protocol", row->link->protocol);
                            buffer_json_member_add_string(wb, "direction", row->link->direction);
                            buffer_json_member_add_string(wb, "state", row->link->state);
                            buffer_json_object_close(wb);
                        }
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        dfe_done(pa);

        NV_REMOTE_ACTOR *ra;
        dfe_start_read(ctx->remote_actors, ra) {
            bool endpoint_is_self = topology_ip_belongs_to_self(ctx, ra->ip, ra->address_space);

            char endpoint_actor_id[NV_TOPOLOGY_KEY_MAX];
            topology_actor_id_for_remote_endpoint(ctx, ra->ip, ra->address_space, endpoint_actor_id, sizeof(endpoint_actor_id));
            state->endpoint_actor_count++;
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "actor_id", endpoint_actor_id);
                buffer_json_member_add_string(wb, "actor_type", "endpoint");
                buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
                topology_add_remote_match(wb, ra->ip);

                if(endpoint_is_self) {
                    buffer_json_member_add_object(wb, "parent_match");
                    {
                        if(ctx->machine_guid[0])
                            buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);
                        topology_add_single_item_string_array(wb, "hostnames", ctx->hostname);
                    }
                    buffer_json_object_close(wb);
                }

                buffer_json_member_add_object(wb, "attributes");
                {
                    buffer_json_member_add_uint64(wb, "socket_count", ra->sockets);
                    buffer_json_member_add_uint64(wb, "local_socket_count", endpoint_is_self ? ra->sockets : 0);
                    buffer_json_member_add_uint64(wb, "remote_socket_count", endpoint_is_self ? 0 : ra->sockets);
                    buffer_json_member_add_string(wb, "endpoint_scope", endpoint_is_self ? "self" : "remote");
                    buffer_json_member_add_string(wb, "display_name", ra->ip);
                    buffer_json_member_add_string(wb, "actor_class", "endpoint");
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "labels");
                {
                    buffer_json_member_add_string(wb, "address_space", ra->address_space);
                    buffer_json_member_add_string(wb, "endpoint_scope", endpoint_is_self ? "self" : "remote");
                    buffer_json_member_add_string(wb, "display_name", ra->ip);
                    buffer_json_member_add_string(wb, "actor_class", "endpoint");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        dfe_done(ra);
    }
    buffer_json_array_close(wb);

    topology_destroy_process_socket_index(process_socket_index);
}

static void topology_write_links_and_stats(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx, NV_TOPOLOGY_RENDER_STATE *state) {
    buffer_json_member_add_array(wb, "links");
    {
        DICTIONARY *process_parent_ns_lookup = NULL;
        DICTIONARY *process_parent_any_lookup = NULL;
        DICTIONARY *ppid_cache = NULL;
        if(ctx->options.processes_by_pid) {
            process_parent_ns_lookup = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                NULL, sizeof(NV_ENDPOINT_OWNER));
            process_parent_any_lookup = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                NULL, sizeof(NV_ENDPOINT_OWNER));
            ppid_cache = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                NULL, sizeof(NV_PPID_CACHE_ENTRY));

            if(process_parent_ns_lookup && process_parent_any_lookup) {
                char parent_key_ns[NV_TOPOLOGY_KEY_MAX];
                char parent_key_any[NV_TOPOLOGY_KEY_MAX];
                char pid_key[64];
                NV_PROCESS_ACTOR *pa_index;
                dfe_start_read(ctx->process_actors, pa_index) {
                    if(!pa_index->pid)
                        continue;

                    NV_ENDPOINT_OWNER owner = {
                        .pid = pa_index->pid,
                        .ppid = pa_index->ppid,
                        .uid = pa_index->uid,
                        .net_ns_inode = pa_index->net_ns_inode,
                    };
                    snprintf(owner.process, sizeof(owner.process), "%s", pa_index->process);

                    topology_process_parent_lookup_key(parent_key_ns, sizeof(parent_key_ns),
                                                       (uint64_t)pa_index->pid, pa_index->net_ns_inode, true);
                    dictionary_set(process_parent_ns_lookup, parent_key_ns, &owner, sizeof(owner));

                    topology_process_parent_lookup_key(parent_key_any, sizeof(parent_key_any),
                                                       (uint64_t)pa_index->pid, 0, false);
                    dictionary_set(process_parent_any_lookup, parent_key_any, &owner, sizeof(owner));

                    if(ppid_cache) {
                        NV_PPID_CACHE_ENTRY ppid_entry = { .ppid = pa_index->ppid };
                        topology_pid_lookup_key(pid_key, sizeof(pid_key), (uint64_t)pa_index->pid);
                        dictionary_set(ppid_cache, pid_key, &ppid_entry, sizeof(ppid_entry));
                    }
                }
                dfe_done(pa_index);
            }
        }

        NV_PROCESS_ACTOR *pa;
        dfe_start_read(ctx->process_actors, pa) {
            bool src_is_process = false;
            NV_ENDPOINT_OWNER *parent_pa = NULL;
            NV_ENDPOINT_OWNER parent_resolved = { 0 };
            const char *ownership_kind = "self_root";
            char src_actor_id[NV_TOPOLOGY_KEY_MAX];
            char src_display_name[NV_TOPOLOGY_KEY_MAX];
            char process_actor_id[NV_TOPOLOGY_KEY_MAX];
            char process_display_name[NV_TOPOLOGY_KEY_MAX];
            char ownership_display_name[NV_TOPOLOGY_KEY_MAX * 2 + 7];
            topology_actor_id_for_process(ctx, pa->pid, pa->uid, pa->net_ns_inode, pa->process, process_actor_id, sizeof(process_actor_id));
            topology_process_display_name(ctx, pa->process, pa->pid, process_display_name, sizeof(process_display_name));

            if(ctx->options.processes_by_pid &&
               process_parent_ns_lookup && process_parent_any_lookup &&
               pa->ppid && pa->ppid != pa->pid) {
                parent_pa = topology_find_process_parent_actor(pa->ppid, pa->net_ns_inode,
                                                               process_parent_ns_lookup,
                                                               process_parent_any_lookup,
                                                               ppid_cache);

                if(parent_pa) {
                    src_is_process = true;
                    ownership_kind = "process_parent";
                    parent_resolved = *parent_pa;
                    parent_pa = &parent_resolved;
                    topology_actor_id_for_process(ctx, parent_pa->pid, parent_pa->uid, parent_pa->net_ns_inode, parent_pa->process, src_actor_id, sizeof(src_actor_id));
                    topology_process_display_name(ctx, parent_pa->process, parent_pa->pid, src_display_name, sizeof(src_display_name));
                }
            }

            if(!src_is_process) {
                snprintf(src_actor_id, sizeof(src_actor_id), "%s", state->host_actor_id);
                snprintf(src_display_name, sizeof(src_display_name), "%s", ctx->hostname);
            }

            snprintf(ownership_display_name, sizeof(ownership_display_name), "%s owns %s", src_display_name, process_display_name);
            state->ownership_link_count++;

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                buffer_json_member_add_string(wb, "protocol", "ownership");
                buffer_json_member_add_string(wb, "link_type", "ownership");
                buffer_json_member_add_string(wb, "direction", "contains");
                buffer_json_member_add_string(wb, "state", "active");
                buffer_json_member_add_string(wb, "src_actor_id", src_actor_id);
                buffer_json_member_add_string(wb, "dst_actor_id", process_actor_id);
                buffer_json_member_add_datetime_rfc3339(wb, "discovered_at", ctx->now_ut, true);
                buffer_json_member_add_datetime_rfc3339(wb, "last_seen", ctx->now_ut, true);

                buffer_json_member_add_object(wb, "src");
                {
                    if(src_is_process) {
                        topology_add_process_identity_match(wb, ctx, parent_pa->pid, parent_pa->uid, parent_pa->net_ns_inode, parent_pa->process);

                        buffer_json_member_add_object(wb, "attributes");
                        {
                            buffer_json_member_add_string(wb, "actor_type", "process");
                            if(ctx->options.processes_by_pid) {
                                buffer_json_member_add_uint64(wb, "pid", parent_pa->pid);
                                buffer_json_member_add_uint64(wb, "ppid", parent_pa->ppid);
                                buffer_json_member_add_uint64(wb, "uid", parent_pa->uid);
                                buffer_json_member_add_uint64(wb, "net_ns_inode", parent_pa->net_ns_inode);
                            }
                            buffer_json_member_add_string(wb, "process", parent_pa->process);
                            buffer_json_member_add_string(wb, "display_name", src_display_name);
                        }
                        buffer_json_object_close(wb);
                    }
                    else {
                        topology_add_host_match(wb, ctx);

                        buffer_json_member_add_object(wb, "attributes");
                        {
                            buffer_json_member_add_string(wb, "actor_type", "self");
                            buffer_json_member_add_string(wb, "display_name", ctx->hostname);
                        }
                        buffer_json_object_close(wb);
                    }
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "dst");
                {
                    topology_add_process_match(wb, ctx, pa);

                    buffer_json_member_add_object(wb, "attributes");
                    {
                        buffer_json_member_add_string(wb, "actor_type", "process");
                        if(ctx->options.processes_by_pid) {
                            buffer_json_member_add_uint64(wb, "pid", pa->pid);
                            buffer_json_member_add_uint64(wb, "ppid", pa->ppid);
                            buffer_json_member_add_uint64(wb, "uid", pa->uid);
                            buffer_json_member_add_uint64(wb, "net_ns_inode", pa->net_ns_inode);
                        }
                        buffer_json_member_add_string(wb, "process", pa->process);
                        buffer_json_member_add_string(wb, "display_name", process_display_name);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "metrics");
                {
                    buffer_json_member_add_uint64(wb, "socket_count", pa->sockets);
                    buffer_json_member_add_string(wb, "display_name", ownership_display_name);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "labels");
                {
                    buffer_json_member_add_string(wb, "link_class", "ownership");
                    buffer_json_member_add_string(wb, "render_intent", "dark");
                    buffer_json_member_add_string(wb, "ownership_kind", ownership_kind);
                    buffer_json_member_add_string(wb, "display_name", ownership_display_name);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        dfe_done(pa);

        if(process_parent_ns_lookup)
            dictionary_destroy(process_parent_ns_lookup);
        if(process_parent_any_lookup)
            dictionary_destroy(process_parent_any_lookup);
        if(ppid_cache)
            dictionary_destroy(ppid_cache);

        NV_TOPOLOGY_LINK *link;
        dfe_start_read(ctx->links, link) {
            char process_actor_id[NV_TOPOLOGY_KEY_MAX];
            char dst_actor_id[NV_TOPOLOGY_KEY_MAX];
            char endpoint_actor_id[NV_TOPOLOGY_KEY_MAX];
            char local_bind_port[128];
            char remote_endpoint_port[128];
            char process_display_name[NV_TOPOLOGY_KEY_MAX];
            char dst_process_display_name[NV_TOPOLOGY_KEY_MAX];
            char dst_process_port_name[64] = "";
            char link_display_name[NV_TOPOLOGY_KEY_MAX * 2 + 256 + 7];
            bool remote_is_self = topology_ip_belongs_to_self(ctx, link->remote_ip, link->remote_address_space);
            bool dst_is_process = false;
            bool dst_local_peer_unresolved = false;
            uint64_t dst_pid = link->pid;
            uint64_t dst_ppid = link->ppid;
            uint64_t dst_uid = link->uid;
            uint64_t dst_net_ns_inode = link->net_ns_inode;
            const char *dst_process_name = link->process;
            uint16_t dst_process_port = 0;

            topology_actor_id_for_process(ctx, link->pid, link->uid, link->net_ns_inode, link->process, process_actor_id, sizeof(process_actor_id));
            topology_actor_id_for_remote_endpoint(ctx, link->remote_ip, link->remote_address_space, endpoint_actor_id, sizeof(endpoint_actor_id));
            snprintf(local_bind_port, sizeof(local_bind_port), "%u", link->local_port);
            topology_format_ip_port(link->remote_ip, link->remote_port, remote_endpoint_port, sizeof(remote_endpoint_port));
            topology_process_display_name(ctx, link->process, link->pid, process_display_name, sizeof(process_display_name));

            if(remote_is_self && link->direction_id != SOCKET_DIRECTION_LISTEN) {
                const char *peer_ip = link->peer_ip[0] ? link->peer_ip : link->remote_ip;
                bool allow_service_fallback = true;
                NV_ENDPOINT_OWNER *owner = topology_lookup_endpoint_owner(ctx, link->net_ns_inode, link->protocol_id, peer_ip, link->peer_port, allow_service_fallback);

                if(owner) {
                    dst_pid = owner->pid;
                    dst_ppid = owner->ppid;
                    dst_uid = owner->uid;
                    dst_net_ns_inode = owner->net_ns_inode;
                    dst_process_name = owner->process;
                    dst_process_port = link->peer_port;
                }
                else {
                    dst_local_peer_unresolved = (link->direction_id != SOCKET_DIRECTION_LISTEN);
                    dst_process_port = (link->direction_id == SOCKET_DIRECTION_LISTEN) ? link->local_port : (link->peer_port ? link->peer_port : link->remote_port);
                }

                dst_is_process = true;
                topology_actor_id_for_process(ctx, dst_pid, dst_uid, dst_net_ns_inode, dst_process_name, dst_actor_id, sizeof(dst_actor_id));
                topology_process_display_name(ctx, dst_process_name, dst_pid, dst_process_display_name, sizeof(dst_process_display_name));
                if(dst_process_port)
                    snprintf(dst_process_port_name, sizeof(dst_process_port_name), "%u", dst_process_port);
                else
                    snprintf(dst_process_port_name, sizeof(dst_process_port_name), "unknown");

                snprintf(link_display_name, sizeof(link_display_name), "%s:%s -> %s:%s",
                         process_display_name, local_bind_port, dst_process_display_name, dst_process_port_name);
            }
            else {
                snprintf(dst_actor_id, sizeof(dst_actor_id), "%s", endpoint_actor_id);
                snprintf(link_display_name, sizeof(link_display_name), "%s:%s -> %s",
                         process_display_name, local_bind_port, remote_endpoint_port);
            }

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
                buffer_json_member_add_string(wb, "protocol", link->protocol);
                buffer_json_member_add_string(wb, "link_type", "socket");
                buffer_json_member_add_string(wb, "direction", link->direction);
                buffer_json_member_add_string(wb, "state", link->state);
                buffer_json_member_add_string(wb, "src_actor_id", process_actor_id);
                buffer_json_member_add_string(wb, "dst_actor_id", dst_actor_id);
                buffer_json_member_add_datetime_rfc3339(wb, "discovered_at", ctx->now_ut, true);
                buffer_json_member_add_datetime_rfc3339(wb, "last_seen", ctx->now_ut, true);

                buffer_json_member_add_object(wb, "src");
                {
                    buffer_json_member_add_object(wb, "match");
                    {
                        if(ctx->machine_guid[0])
                            buffer_json_member_add_string(wb, "netdata_machine_guid", ctx->machine_guid);
                        topology_add_single_item_string_array(wb, "hostnames", ctx->hostname);
                        topology_add_single_item_string_array(wb, "ip_addresses", link->local_ip);
                    }
                    buffer_json_object_close(wb);

                    buffer_json_member_add_object(wb, "attributes");
                    {
                        buffer_json_member_add_string(wb, "actor_type", "process");
                        if(ctx->options.processes_by_pid) {
                            buffer_json_member_add_uint64(wb, "pid", link->pid);
                            buffer_json_member_add_uint64(wb, "ppid", link->ppid);
                            buffer_json_member_add_uint64(wb, "uid", link->uid);
                            buffer_json_member_add_uint64(wb, "net_ns_inode", link->net_ns_inode);
                        }
                        buffer_json_member_add_string(wb, "process", link->process);
                        buffer_json_member_add_string(wb, "user", link->username);
                        buffer_json_member_add_string(wb, "namespace", link->namespace_type);
                        buffer_json_member_add_string(wb, "address_space", link->local_address_space);
                        buffer_json_member_add_uint64(wb, "port", link->local_port);
                        buffer_json_member_add_string(wb, "port_name", local_bind_port);
                        buffer_json_member_add_string(wb, "bind_ip", link->local_ip);
                        buffer_json_member_add_string(wb, "service_name", link->port_name);
                        buffer_json_member_add_string(wb, "display_name", process_display_name);
                        buffer_json_member_add_string(wb, "protocol_family", link->protocol_family);
                        if(link->cmdline[0])
                            buffer_json_member_add_string(wb, "cmdline", link->cmdline);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);

                buffer_json_member_add_object(wb, "dst");
                {
                    if(dst_is_process)
                        topology_add_process_identity_match(wb, ctx, dst_pid, dst_uid, dst_net_ns_inode, dst_process_name);
                    else
                        topology_add_remote_match(wb, link->remote_ip);

                    buffer_json_member_add_object(wb, "attributes");
                    {
                        if(dst_is_process) {
                            buffer_json_member_add_string(wb, "actor_type", "process");
                            if(ctx->options.processes_by_pid) {
                                buffer_json_member_add_uint64(wb, "pid", dst_pid);
                                buffer_json_member_add_uint64(wb, "ppid", dst_ppid);
                                buffer_json_member_add_uint64(wb, "uid", dst_uid);
                                buffer_json_member_add_uint64(wb, "net_ns_inode", dst_net_ns_inode);
                            }
                            buffer_json_member_add_string(wb, "process", dst_process_name);
                            buffer_json_member_add_string(wb, "address_space", "self");
                            if(dst_process_port) {
                                buffer_json_member_add_uint64(wb, "port", dst_process_port);
                                buffer_json_member_add_string(wb, "port_name", dst_process_port_name);
                            }
                            buffer_json_member_add_string(wb, "display_name", dst_process_display_name);
                            if(dst_local_peer_unresolved)
                                buffer_json_member_add_boolean(wb, "unresolved_local_peer", true);
                        }
                        else {
                            buffer_json_member_add_string(wb, "actor_type", "endpoint");
                            buffer_json_member_add_string(wb, "address_space", link->remote_address_space);
                            buffer_json_member_add_uint64(wb, "port", link->remote_port);
                            buffer_json_member_add_string(wb, "port_name", remote_endpoint_port);
                            buffer_json_member_add_string(wb, "display_name", link->remote_ip);
                        }
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
                    buffer_json_member_add_string(wb, "display_name", link_display_name);
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
                    buffer_json_member_add_string(wb, "port_name", local_bind_port);
                    buffer_json_member_add_string(wb, "bind_ip", link->local_ip);
                    buffer_json_member_add_string(wb, "service_name", link->port_name);
                    buffer_json_member_add_string(wb, "link_class", "socket");
                    buffer_json_member_add_string(wb, "socket_kind", link->direction);
                    buffer_json_member_add_string(wb, "render_intent", "socket");
                    buffer_json_member_add_string(wb, "display_name", link_display_name);
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
        size_t links_total = state->socket_link_count + state->ownership_link_count;
        buffer_json_member_add_string(wb, "processes_mode", ctx->options.processes_by_pid ? "by_pid" : "by_name");
        buffer_json_member_add_boolean(wb, "sockets_listening", ctx->options.sockets_listening);
        buffer_json_member_add_boolean(wb, "sockets_local", ctx->options.sockets_local);
        buffer_json_member_add_boolean(wb, "sockets_inbound", ctx->options.sockets_inbound);
        buffer_json_member_add_boolean(wb, "sockets_outbound", ctx->options.sockets_outbound);
        buffer_json_member_add_string(wb, "endpoints_mode_selected", "by_ip");
        buffer_json_member_add_string(wb, "endpoints_mode_effective", "by_ip");
        buffer_json_member_add_boolean(wb, "protocol_ipv4_tcp", ctx->options.protocols_ipv4_tcp);
        buffer_json_member_add_boolean(wb, "protocol_ipv6_tcp", ctx->options.protocols_ipv6_tcp);
        buffer_json_member_add_boolean(wb, "protocol_ipv4_udp", ctx->options.protocols_ipv4_udp);
        buffer_json_member_add_boolean(wb, "protocol_ipv6_udp", ctx->options.protocols_ipv6_udp);
        buffer_json_member_add_uint64(wb, "sockets_total", ctx->sockets_total);
        buffer_json_member_add_uint64(wb, "sockets_without_remote_endpoint", ctx->skipped_sockets);
        buffer_json_member_add_uint64(wb, "local_process_actors", state->process_actor_count);
        buffer_json_member_add_uint64(wb, "endpoint_actors", state->endpoint_actor_count);
        buffer_json_member_add_uint64(wb, "socket_links", state->socket_link_count);
        buffer_json_member_add_uint64(wb, "ownership_links", state->ownership_link_count);
        buffer_json_member_add_uint64(wb, "links_total", links_total);
    }
    buffer_json_object_close(wb);
}

static void topology_write_data(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx || ctx->options.info_only || !ctx->process_actors || !ctx->remote_actors || !ctx->local_ips || !ctx->links)
        return;

    NV_TOPOLOGY_RENDER_STATE state;
    topology_render_state_init(&state, ctx);

    buffer_json_member_add_object(wb, "data");
    {
        buffer_json_member_add_string(wb, "schema_version", NETWORK_TOPOLOGY_SCHEMA_VERSION);
        buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
        buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
        buffer_json_member_add_string(wb, "agent_id", ctx->machine_guid[0] ? ctx->machine_guid : ctx->hostname);
        buffer_json_member_add_datetime_rfc3339(wb, "collected_at", ctx->now_ut, true);

        topology_write_actors(wb, ctx, &state);
        topology_write_links_and_stats(wb, ctx, &state);
    }
    buffer_json_object_close(wb);
}

static void network_viewer_topology_function(
    const char *transaction, char *function, usec_t *stop_monotonic_ut __maybe_unused,
    bool *cancelled __maybe_unused, BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {

    time_t now_s = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();
    NV_TOPOLOGY_OPTIONS options = { 0 };
    topology_parse_options(function, &options);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    topology_write_response_metadata(wb);
    topology_write_presentation(wb);

    NV_TOPOLOGY_CONTEXT ctx;
    bool ctx_ready = topology_prepare_context(&ctx, now_ut, &options);
    if(ctx_ready)
        topology_write_data(wb, &ctx);

    topology_context_destroy(&ctx);
    topology_finalize_response(transaction, wb, now_s);
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

    char *function_copy = strdupz(function);
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
            buffer_json_member_add_object(wb, "Count by Direction");
            {
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Direction");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "Count by Process");
            {
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Process");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "Count by Protocol");
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
    buffer_json_member_add_time_t(wb, "expires", now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY;
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
