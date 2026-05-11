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
#define NETWORK_TOPOLOGY_SCHEMA_VERSION "netdata.topology.v1"
#define NETWORK_TOPOLOGY_SOURCE "network-connections"
#define NETWORK_TOPOLOGY_LAYER "network"
#define NV_TOPOLOGY_MAX_PPID_DEPTH 64

#define NV_TOPOLOGY_USERNAME_MAX 128
#define NV_TOPOLOGY_CMDLINE_MAX 512
#define NV_TOPOLOGY_LABEL_KEY_MAX 96
#define NV_TOPOLOGY_LABEL_VALUE_MAX 512
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
    bool detailed;
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
    opts->detailed = false; // default: aggregated graph view
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

        if(strcmp(param, "aggregated") == 0 ||
           strcmp(param, "mode:aggregated") == 0 ||
           strcmp(param, "__topology_mode:aggregated") == 0 ||
           strcmp(param, "__topology_mode=aggregated") == 0 ||
           strcmp(param, "view:aggregated") == 0) {
            opts->detailed = false;
            continue;
        }
        if(strcmp(param, "detailed") == 0 ||
           strcmp(param, "mode:detailed") == 0 ||
           strcmp(param, "__topology_mode:detailed") == 0 ||
           strcmp(param, "__topology_mode=detailed") == 0 ||
           strcmp(param, "view:detailed") == 0) {
            opts->detailed = true;
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
    bool hidden_listen_socket = (n->direction == SOCKET_DIRECTION_LISTEN && !ctx->options.sockets_listening);
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

    if(hidden_listen_socket) {
        NV_PROCESS_ACTOR hidden_owner = {
            .pid = n->pid,
            .ppid = n->ppid,
            .uid = n->uid,
            .net_ns_inode = n->net_ns_inode,
        };
        snprintf(hidden_owner.process, sizeof(hidden_owner.process), "%s", process_name);
        topology_register_endpoint_owner(ctx, n->net_ns_inode, n->local.protocol, local_ip, n->local.port, &hidden_owner, true);
        ctx->skipped_sockets++;
        return;
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
    bool self_owned_listen_socket = (n->direction == SOCKET_DIRECTION_LISTEN && remote_is_self);
    bool create_endpoint_actor = !remote_is_self;
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

    if(self_owned_listen_socket) {
        ctx->skipped_sockets++;
        return;
    }

    const struct socket_endpoint *server_endpoint = NULL;
    uint16_t endpoint_port = n->remote.port;
    switch(n->direction) {
        case SOCKET_DIRECTION_LISTEN:
            server_endpoint = &n->local;
            endpoint_port = n->local.port;
            break;

        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
            server_endpoint = &n->local;
            endpoint_port = n->remote.port;
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
            // Always collect listeners so inbound/outbound socket classification
            // remains stable across topology socket filters.
            .listening = true,
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
        buffer_json_add_array_item_string(wb, "__topology_mode");
        buffer_json_add_array_item_string(wb, "mode");
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
            buffer_json_member_add_string(wb, "id", "__topology_mode");
            buffer_json_member_add_string(wb, "name", "Mode");
            buffer_json_member_add_string(wb, "help", "Return an aggregated graph by default, or include lossless socket evidence for detailed correlation.");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "aggregated");
                    buffer_json_member_add_string(wb, "name", "Aggregated");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "detailed");
                    buffer_json_member_add_string(wb, "name", "Detailed");
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

typedef struct {
    char id[NV_TOPOLOGY_KEY_MAX];
    char type[16];
    char machine_guid[128];
    char hostname[256];
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char local_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char ip[INET6_ADDRSTRLEN];
    char address_space[16];
    char display_name[NV_TOPOLOGY_KEY_MAX];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
    uint64_t pid;
    uint64_t ppid;
    uint64_t uid;
    uint64_t net_ns_inode;
    uint64_t sockets;
    uint64_t local_ip_count;
    bool has_pid;
    bool has_ppid;
    bool has_uid;
    bool has_net_ns_inode;
    bool has_local_ip_count;
} NV_TOPOLOGY_V1_ACTOR;

typedef struct {
    uint64_t src_actor;
    uint64_t dst_actor;
    char type[32];
    char protocol[16];
    char direction[16];
    char state[32];
    uint64_t evidence_count;
    uint64_t socket_count;
    uint64_t retransmissions;
    uint32_t max_rtt_usec;
    uint32_t max_rcv_rtt_usec;
} NV_TOPOLOGY_V1_GRAPH_LINK;

typedef struct {
    uint64_t link;
    uint64_t src_actor;
    uint64_t dst_actor;
    const NV_TOPOLOGY_LINK *source;
} NV_TOPOLOGY_V1_SOCKET_EVIDENCE;

typedef struct {
    uint64_t src_actor;
    uint64_t dst_actor;
    const NV_TOPOLOGY_LINK *source;
} NV_TOPOLOGY_V1_CONNECTION_ROW;

typedef struct {
    uint64_t actor;
    uint64_t port;
    uint64_t socket_count;
    char protocol[16];
    char direction[16];
} NV_TOPOLOGY_V1_PORT_ROW;

typedef struct {
    uint64_t actor;
    uint64_t port;
    char protocol[16];
    char address_space[16];
    char ip[INET6_ADDRSTRLEN];
} NV_TOPOLOGY_V1_CORRELATION_ROW;

typedef struct {
    uint64_t actor;
    uint64_t value_index;
    bool has_value_index;
    char key[NV_TOPOLOGY_LABEL_KEY_MAX];
    char value[NV_TOPOLOGY_LABEL_VALUE_MAX];
    char source[32];
    char kind[32];
} NV_TOPOLOGY_V1_ACTOR_LABEL;

typedef struct {
    NV_TOPOLOGY_V1_ACTOR *actors;
    size_t actors_used;
    size_t actors_size;

    NV_TOPOLOGY_V1_GRAPH_LINK *links;
    size_t links_used;
    size_t links_size;

    NV_TOPOLOGY_V1_SOCKET_EVIDENCE *evidence;
    size_t evidence_used;
    size_t evidence_size;

    NV_TOPOLOGY_V1_CONNECTION_ROW *connections;
    size_t connections_used;
    size_t connections_size;

    NV_TOPOLOGY_V1_PORT_ROW *ports;
    size_t ports_used;
    size_t ports_size;

    NV_TOPOLOGY_V1_CORRELATION_ROW *correlation_points;
    size_t correlation_points_used;
    size_t correlation_points_size;

    NV_TOPOLOGY_V1_CORRELATION_ROW *correlation_claims;
    size_t correlation_claims_used;
    size_t correlation_claims_size;

    NV_TOPOLOGY_V1_ACTOR_LABEL *labels;
    size_t labels_used;
    size_t labels_size;

    DICTIONARY *actor_index;
    DICTIONARY *graph_link_index;
    DICTIONARY *port_index;
    DICTIONARY *correlation_point_index;
    DICTIONARY *correlation_claim_index;
} NV_TOPOLOGY_V1_PAYLOAD;

typedef struct {
    const char **values;
    uint64_t *indexes;
    size_t rows;
    size_t rows_used;
    size_t values_used;
    size_t values_size;
    size_t values_json_size;
    size_t unique_json_size;
    size_t indexes_json_size;
    DICTIONARY *index;
} NV_TOPOLOGY_V1_STRING_COLUMN;

static void topology_v1_strncpy(char *dst, size_t dst_size, const char *src) {
    if(!dst || !dst_size)
        return;

    strncpyz(dst, src ? src : "", dst_size - 1);
}

static void topology_v1_actor_index_set(NV_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id, uint64_t index) {
    dictionary_set(payload->actor_index, actor_id, &index, sizeof(index));
}

static bool topology_v1_actor_index_get(NV_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id, uint64_t *index) {
    uint64_t *stored = dictionary_get(payload->actor_index, actor_id);
    if(!stored)
        return false;

    if(index)
        *index = *stored;

    return true;
}

static NV_TOPOLOGY_V1_ACTOR *topology_v1_add_actor(NV_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id) {
    if(payload->actors_used == payload->actors_size) {
        size_t new_size = payload->actors_size ? payload->actors_size * 2 : 32;
        payload->actors = reallocz(payload->actors, new_size * sizeof(*payload->actors));
        payload->actors_size = new_size;
    }

    NV_TOPOLOGY_V1_ACTOR *actor = &payload->actors[payload->actors_used];
    *actor = (NV_TOPOLOGY_V1_ACTOR){ 0 };
    topology_v1_strncpy(actor->id, sizeof(actor->id), actor_id);
    topology_v1_actor_index_set(payload, actor_id, payload->actors_used);
    payload->actors_used++;
    return actor;
}

static NV_TOPOLOGY_V1_GRAPH_LINK *topology_v1_add_graph_link(NV_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->links_used == payload->links_size) {
        size_t new_size = payload->links_size ? payload->links_size * 2 : 64;
        payload->links = reallocz(payload->links, new_size * sizeof(*payload->links));
        payload->links_size = new_size;
    }

    NV_TOPOLOGY_V1_GRAPH_LINK *link = &payload->links[payload->links_used++];
    *link = (NV_TOPOLOGY_V1_GRAPH_LINK){ 0 };
    return link;
}

static NV_TOPOLOGY_V1_SOCKET_EVIDENCE *topology_v1_add_socket_evidence(NV_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->evidence_used == payload->evidence_size) {
        size_t new_size = payload->evidence_size ? payload->evidence_size * 2 : 256;
        payload->evidence = reallocz(payload->evidence, new_size * sizeof(*payload->evidence));
        payload->evidence_size = new_size;
    }

    NV_TOPOLOGY_V1_SOCKET_EVIDENCE *row = &payload->evidence[payload->evidence_used++];
    *row = (NV_TOPOLOGY_V1_SOCKET_EVIDENCE){ 0 };
    return row;
}

static void topology_v1_add_connection_row(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t src_actor,
    uint64_t dst_actor,
    const NV_TOPOLOGY_LINK *source) {
    if(!payload || !source)
        return;

    if(payload->connections_used == payload->connections_size) {
        size_t new_size = payload->connections_size ? payload->connections_size * 2 : 256;
        payload->connections = reallocz(payload->connections, new_size * sizeof(*payload->connections));
        payload->connections_size = new_size;
    }

    NV_TOPOLOGY_V1_CONNECTION_ROW *row = &payload->connections[payload->connections_used++];
    *row = (NV_TOPOLOGY_V1_CONNECTION_ROW){ 0 };
    row->src_actor = src_actor;
    row->dst_actor = dst_actor;
    row->source = source;
}

static void topology_v1_add_port_row(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *protocol,
    const char *direction,
    uint64_t port,
    uint64_t socket_count) {
    if(!payload || !payload->port_index || !port || !socket_count)
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%s|%s|%"PRIu64,
              actor,
              protocol ? protocol : "",
              direction ? direction : "",
              port);

    uint64_t *stored = dictionary_get(payload->port_index, key);
    if(stored) {
        payload->ports[*stored].socket_count += socket_count;
        return;
    }

    if(payload->ports_used == payload->ports_size) {
        size_t new_size = payload->ports_size ? payload->ports_size * 2 : 256;
        payload->ports = reallocz(payload->ports, new_size * sizeof(*payload->ports));
        payload->ports_size = new_size;
    }

    uint64_t index = payload->ports_used;
    NV_TOPOLOGY_V1_PORT_ROW *row = &payload->ports[payload->ports_used++];
    *row = (NV_TOPOLOGY_V1_PORT_ROW){ 0 };
    row->actor = actor;
    row->port = port;
    row->socket_count = socket_count;
    topology_v1_strncpy(row->protocol, sizeof(row->protocol), protocol);
    topology_v1_strncpy(row->direction, sizeof(row->direction), direction);

    dictionary_set(payload->port_index, key, &index, sizeof(index));
}

static void topology_v1_add_actor_label_ex(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    const char *value,
    const char *source,
    const char *kind,
    bool has_value_index,
    uint64_t value_index) {
    if(!payload || !key || !*key || !value || !*value)
        return;

    if(payload->labels_used == payload->labels_size) {
        size_t new_size = payload->labels_size ? payload->labels_size * 2 : 128;
        payload->labels = reallocz(payload->labels, new_size * sizeof(*payload->labels));
        payload->labels_size = new_size;
    }

    NV_TOPOLOGY_V1_ACTOR_LABEL *row = &payload->labels[payload->labels_used++];
    *row = (NV_TOPOLOGY_V1_ACTOR_LABEL){ 0 };
    row->actor = actor;
    row->has_value_index = has_value_index;
    row->value_index = value_index;
    topology_v1_strncpy(row->key, sizeof(row->key), key);
    topology_v1_strncpy(row->value, sizeof(row->value), value);
    topology_v1_strncpy(row->source, sizeof(row->source), source ? source : NETWORK_TOPOLOGY_SOURCE);
    topology_v1_strncpy(row->kind, sizeof(row->kind), kind ? kind : "attribute");
}

static void topology_v1_add_actor_label(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    const char *value,
    const char *kind) {
    topology_v1_add_actor_label_ex(payload, actor, key, value, NETWORK_TOPOLOGY_SOURCE, kind, false, 0);
}

static void topology_v1_add_actor_label_uint(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    uint64_t value,
    const char *kind) {
    char buffer[32];
    snprintfz(buffer, sizeof(buffer), "%"PRIu64, value);
    topology_v1_add_actor_label(payload, actor, key, buffer, kind);
}

static void topology_v1_add_correlation_row(
    NV_TOPOLOGY_V1_CORRELATION_ROW **rows,
    size_t *rows_used,
    size_t *rows_size,
    DICTIONARY *index,
    uint64_t actor,
    const char *protocol,
    const char *address_space,
    const char *ip,
    uint64_t port) {
    if(!rows || !rows_used || !rows_size || !index || !port || topology_ip_is_unspecified(ip))
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%s|%s|%s|%"PRIu64,
              actor,
              protocol ? protocol : "",
              address_space ? address_space : "",
              ip ? ip : "",
              port);

    if(dictionary_get(index, key))
        return;

    if(*rows_used == *rows_size) {
        size_t new_size = *rows_size ? *rows_size * 2 : 256;
        *rows = reallocz(*rows, new_size * sizeof(**rows));
        *rows_size = new_size;
    }

    NV_TOPOLOGY_V1_CORRELATION_ROW *row = &(*rows)[(*rows_used)++];
    *row = (NV_TOPOLOGY_V1_CORRELATION_ROW){ 0 };
    row->actor = actor;
    row->port = port;
    topology_v1_strncpy(row->protocol, sizeof(row->protocol), protocol);
    topology_v1_strncpy(row->address_space, sizeof(row->address_space), address_space);
    topology_v1_strncpy(row->ip, sizeof(row->ip), ip);

    bool stored = true;
    dictionary_set(index, key, &stored, sizeof(stored));
}

static void topology_v1_add_correlation_claim(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const NV_TOPOLOGY_LINK *source) {
    if(!payload || !source)
        return;

    topology_v1_add_correlation_row(
        &payload->correlation_claims,
        &payload->correlation_claims_used,
        &payload->correlation_claims_size,
        payload->correlation_claim_index,
        actor,
        source->protocol,
        source->local_address_space,
        source->local_ip,
        source->local_port);
}

static void topology_v1_add_correlation_point(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const NV_TOPOLOGY_LINK *source) {
    if(!payload || !source)
        return;

    topology_v1_add_correlation_row(
        &payload->correlation_points,
        &payload->correlation_points_used,
        &payload->correlation_points_size,
        payload->correlation_point_index,
        actor,
        source->protocol,
        source->remote_address_space,
        source->remote_ip,
        source->remote_port);
}

static void topology_v1_free(NV_TOPOLOGY_V1_PAYLOAD *payload) {
    if(!payload)
        return;

    freez(payload->actors);
    freez(payload->links);
    freez(payload->evidence);
    freez(payload->connections);
    freez(payload->ports);
    freez(payload->correlation_points);
    freez(payload->correlation_claims);
    freez(payload->labels);
    if(payload->actor_index)
        dictionary_destroy(payload->actor_index);
    if(payload->graph_link_index)
        dictionary_destroy(payload->graph_link_index);
    if(payload->port_index)
        dictionary_destroy(payload->port_index);
    if(payload->correlation_point_index)
        dictionary_destroy(payload->correlation_point_index);
    if(payload->correlation_claim_index)
        dictionary_destroy(payload->correlation_claim_index);
    *payload = (NV_TOPOLOGY_V1_PAYLOAD){ 0 };
}

static bool topology_v1_should_emit_endpoint_actor(
    const NV_TOPOLOGY_CONTEXT *ctx,
    const NV_REMOTE_ACTOR *actor) {
    if(!actor || !actor->ip[0])
        return false;

    return !topology_ip_belongs_to_self(ctx, actor->ip, actor->address_space);
}

static void topology_v1_collect_actors(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_RENDER_STATE *state,
    NV_TOPOLOGY_V1_PAYLOAD *payload) {
    NV_TOPOLOGY_V1_ACTOR *self = topology_v1_add_actor(payload, state->host_actor_id);
    topology_v1_strncpy(self->type, sizeof(self->type), "self");
    topology_v1_strncpy(self->machine_guid, sizeof(self->machine_guid), ctx->machine_guid);
    topology_v1_strncpy(self->hostname, sizeof(self->hostname), ctx->hostname);
    topology_v1_strncpy(self->display_name, sizeof(self->display_name), ctx->hostname);
    self->sockets = ctx->sockets_total;
    self->local_ip_count = state->local_ip_count;
    self->has_local_ip_count = true;
    uint64_t self_index = payload->actors_used - 1;
    topology_v1_add_actor_label(payload, self_index, "display_name", self->display_name, "attribute");
    topology_v1_add_actor_label(payload, self_index, "type", self->type, "metadata");
    topology_v1_add_actor_label(payload, self_index, "hostname", self->hostname, "identity");
    topology_v1_add_actor_label(payload, self_index, "machine_guid", self->machine_guid, "identity");
    topology_v1_add_actor_label_uint(payload, self_index, "socket_count", self->sockets, "metric");
    topology_v1_add_actor_label_uint(payload, self_index, "local_ip_count", self->local_ip_count, "metric");

    NV_PROCESS_ACTOR *pa;
    dfe_start_read(ctx->process_actors, pa) {
        char actor_id[NV_TOPOLOGY_KEY_MAX];
        char display_name[NV_TOPOLOGY_KEY_MAX];
        topology_actor_id_for_process(ctx, pa->pid, pa->uid, pa->net_ns_inode, pa->process, actor_id, sizeof(actor_id));
        topology_process_display_name(ctx, pa->process, pa->pid, display_name, sizeof(display_name));

        NV_TOPOLOGY_V1_ACTOR *actor = topology_v1_add_actor(payload, actor_id);
        topology_v1_strncpy(actor->type, sizeof(actor->type), "process");
        topology_v1_strncpy(actor->machine_guid, sizeof(actor->machine_guid), ctx->machine_guid);
        topology_v1_strncpy(actor->hostname, sizeof(actor->hostname), ctx->hostname);
        topology_v1_strncpy(actor->process, sizeof(actor->process), pa->process);
        topology_v1_strncpy(actor->username, sizeof(actor->username), pa->username);
        topology_v1_strncpy(actor->namespace_type, sizeof(actor->namespace_type), pa->namespace_type);
        topology_v1_strncpy(actor->local_ip, sizeof(actor->local_ip), pa->local_ip);
        topology_v1_strncpy(actor->local_address_space, sizeof(actor->local_address_space), pa->local_address_space);
        topology_v1_strncpy(actor->display_name, sizeof(actor->display_name), display_name);
        topology_v1_strncpy(actor->cmdline, sizeof(actor->cmdline), pa->cmdline);
        actor->pid = (uint64_t)pa->pid;
        actor->ppid = (uint64_t)pa->ppid;
        actor->uid = (uint64_t)pa->uid;
        actor->net_ns_inode = pa->net_ns_inode;
        actor->sockets = pa->sockets;
        actor->has_pid = ctx->options.processes_by_pid;
        actor->has_ppid = ctx->options.processes_by_pid;
        actor->has_uid = ctx->options.processes_by_pid;
        actor->has_net_ns_inode = ctx->options.processes_by_pid;

        uint64_t actor_index = payload->actors_used - 1;
        topology_v1_add_actor_label(payload, actor_index, "display_name", actor->display_name, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "type", actor->type, "metadata");
        topology_v1_add_actor_label(payload, actor_index, "process", actor->process, "identity");
        topology_v1_add_actor_label(payload, actor_index, "username", actor->username, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "cmdline", actor->cmdline, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "namespace_type", actor->namespace_type, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "local_ip", actor->local_ip, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "local_address_space", actor->local_address_space, "attribute");
        topology_v1_add_actor_label_uint(payload, actor_index, "socket_count", actor->sockets, "metric");
        if(actor->has_pid)
            topology_v1_add_actor_label_uint(payload, actor_index, "pid", actor->pid, "identity");
        if(actor->has_uid)
            topology_v1_add_actor_label_uint(payload, actor_index, "uid", actor->uid, "attribute");
        if(actor->has_net_ns_inode)
            topology_v1_add_actor_label_uint(payload, actor_index, "net_ns_inode", actor->net_ns_inode, "identity");
    }
    dfe_done(pa);

    NV_REMOTE_ACTOR *ra;
    dfe_start_read(ctx->remote_actors, ra) {
        // Remote actors are discovered while sockets are scanned, before all
        // local IPs may be known. Recheck with the final local-IP set so actor
        // emission and link destination resolution use the same self test.
        if(!topology_v1_should_emit_endpoint_actor(ctx, ra))
            continue;

        char actor_id[NV_TOPOLOGY_KEY_MAX];
        topology_actor_id_for_remote_endpoint(ctx, ra->ip, ra->address_space, actor_id, sizeof(actor_id));

        NV_TOPOLOGY_V1_ACTOR *actor = topology_v1_add_actor(payload, actor_id);
        topology_v1_strncpy(actor->type, sizeof(actor->type), "endpoint");
        topology_v1_strncpy(actor->ip, sizeof(actor->ip), ra->ip);
        topology_v1_strncpy(actor->address_space, sizeof(actor->address_space), ra->address_space);
        topology_v1_strncpy(actor->display_name, sizeof(actor->display_name), ra->ip);
        actor->sockets = ra->sockets;
        uint64_t actor_index = payload->actors_used - 1;
        topology_v1_add_actor_label(payload, actor_index, "display_name", actor->display_name, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "type", actor->type, "metadata");
        topology_v1_add_actor_label(payload, actor_index, "ip", actor->ip, "identity");
        topology_v1_add_actor_label(payload, actor_index, "address_space", actor->address_space, "attribute");
        topology_v1_add_actor_label_uint(payload, actor_index, "socket_count", actor->sockets, "metric");
        state->endpoint_actor_count++;
    }
    dfe_done(ra);
}

static bool topology_v1_process_actor_index(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t pid,
    uint64_t uid,
    uint64_t net_ns_inode,
    const char *process,
    uint64_t *index) {
    char actor_id[NV_TOPOLOGY_KEY_MAX];
    topology_actor_id_for_process(ctx, pid, uid, net_ns_inode, process, actor_id, sizeof(actor_id));
    return topology_v1_actor_index_get(payload, actor_id, index);
}

static bool topology_v1_endpoint_actor_index(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    const char *ip,
    const char *address_space,
    uint64_t *index) {
    char actor_id[NV_TOPOLOGY_KEY_MAX];
    topology_actor_id_for_remote_endpoint(ctx, ip, address_space, actor_id, sizeof(actor_id));
    return topology_v1_actor_index_get(payload, actor_id, index);
}

static uint64_t topology_v1_graph_link_get_or_add(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t src_actor,
    uint64_t dst_actor,
    const char *type,
    const char *protocol,
    const char *direction,
    const char *state) {
    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%"PRIu64"|%s|%s|%s|%s",
              src_actor, dst_actor,
              type ? type : "",
              protocol ? protocol : "",
              direction ? direction : "",
              state ? state : "");

    uint64_t *stored = dictionary_get(payload->graph_link_index, key);
    if(stored)
        return *stored;

    uint64_t index = payload->links_used;
    NV_TOPOLOGY_V1_GRAPH_LINK *link = topology_v1_add_graph_link(payload);
    link->src_actor = src_actor;
    link->dst_actor = dst_actor;
    topology_v1_strncpy(link->type, sizeof(link->type), type);
    topology_v1_strncpy(link->protocol, sizeof(link->protocol), protocol);
    topology_v1_strncpy(link->direction, sizeof(link->direction), direction);
    topology_v1_strncpy(link->state, sizeof(link->state), state);
    dictionary_set(payload->graph_link_index, key, &index, sizeof(index));
    return index;
}

static bool topology_v1_socket_dst_actor_index(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    const NV_TOPOLOGY_LINK *link,
    uint64_t *dst_actor,
    bool *dst_is_correlation_point) {
    if(dst_is_correlation_point)
        *dst_is_correlation_point = false;

    bool remote_is_self = topology_ip_belongs_to_self(ctx, link->remote_ip, link->remote_address_space);
    if(remote_is_self && link->direction_id != SOCKET_DIRECTION_LISTEN) {
        const char *peer_ip = link->peer_ip[0] ? link->peer_ip : link->remote_ip;
        NV_ENDPOINT_OWNER *owner = topology_lookup_endpoint_owner(ctx, link->net_ns_inode, link->protocol_id, peer_ip, link->peer_port, true);

        if(owner) {
            return topology_v1_process_actor_index(ctx, payload,
                                                   owner->pid, owner->uid, owner->net_ns_inode,
                                                   owner->process, dst_actor);
        }

        return topology_v1_process_actor_index(ctx, payload,
                                               link->pid, link->uid, link->net_ns_inode,
                                               link->process, dst_actor);
    }

    if(dst_is_correlation_point)
        *dst_is_correlation_point = true;
    return topology_v1_endpoint_actor_index(ctx, payload, link->remote_ip, link->remote_address_space, dst_actor);
}

static void topology_v1_collect_links(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_RENDER_STATE *state,
    NV_TOPOLOGY_V1_PAYLOAD *payload) {
    uint64_t host_actor = 0;
    topology_v1_actor_index_get(payload, state->host_actor_id, &host_actor);

    NV_PROCESS_ACTOR *pa;
    dfe_start_read(ctx->process_actors, pa) {
        uint64_t process_actor;
        if(!topology_v1_process_actor_index(ctx, payload, pa->pid, pa->uid, pa->net_ns_inode, pa->process, &process_actor))
            continue;

        uint64_t link_index = topology_v1_graph_link_get_or_add(
            payload, host_actor, process_actor, "ownership", "ownership", "contains", "active");
        NV_TOPOLOGY_V1_GRAPH_LINK *link = &payload->links[link_index];
        link->socket_count += pa->sockets;
        state->ownership_link_count++;
    }
    dfe_done(pa);

    NV_TOPOLOGY_LINK *source;
    dfe_start_read(ctx->links, source) {
        uint64_t src_actor;
        if(!topology_v1_process_actor_index(ctx, payload,
                                            source->pid, source->uid, source->net_ns_inode,
                                            source->process, &src_actor))
            continue;

        uint64_t dst_actor;
        bool dst_is_correlation_point = false;
        if(!topology_v1_socket_dst_actor_index(ctx, payload, source, &dst_actor, &dst_is_correlation_point))
            continue;

        topology_v1_add_correlation_claim(payload, src_actor, source);
        if(dst_is_correlation_point)
            topology_v1_add_correlation_point(payload, dst_actor, source);

        topology_v1_add_port_row(
            payload, src_actor, source->protocol, source->direction, source->local_port, source->sockets);
        if(!dst_is_correlation_point)
            topology_v1_add_port_row(
                payload, dst_actor, source->protocol, source->direction, source->remote_port, source->sockets);

        uint64_t link_index = topology_v1_graph_link_get_or_add(
            payload, src_actor, dst_actor,
            dst_is_correlation_point ? "endpoint_socket" : "socket",
            source->protocol, source->direction, source->state);
        NV_TOPOLOGY_V1_GRAPH_LINK *link = &payload->links[link_index];
        link->evidence_count++;
        link->socket_count += source->sockets;
        link->retransmissions += source->retransmissions;
        if(link->max_rtt_usec < source->max_rtt_usec)
            link->max_rtt_usec = source->max_rtt_usec;
        if(link->max_rcv_rtt_usec < source->max_rcv_rtt_usec)
            link->max_rcv_rtt_usec = source->max_rcv_rtt_usec;

        if(ctx->options.detailed) {
            NV_TOPOLOGY_V1_SOCKET_EVIDENCE *row = topology_v1_add_socket_evidence(payload);
            row->link = link_index;
            row->src_actor = src_actor;
            row->dst_actor = dst_actor;
            row->source = source;
        }
        else
            topology_v1_add_connection_row(payload, src_actor, dst_actor, source);
    }
    dfe_done(source);
}

static void topology_v1_emit_column(
    BUFFER *wb,
    const char *id,
    const char *type,
    const char *role,
    bool nullable,
    const char *aggregation) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "id", id);
    buffer_json_member_add_string(wb, "type", type);
    if(nullable)
        buffer_json_member_add_boolean(wb, "nullable", true);
    if(role)
        buffer_json_member_add_string(wb, "role", role);
    if(aggregation)
        buffer_json_member_add_string(wb, "aggregation", aggregation);
    buffer_json_object_close(wb);
}

static void topology_v1_values_start(BUFFER *wb) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "codec", "values");
    buffer_json_member_add_array(wb, "values");
}

static void topology_v1_const_string(BUFFER *wb, const char *value) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "codec", "const");
    buffer_json_member_add_string(wb, "value", value);
    buffer_json_object_close(wb);
}

static void topology_v1_values_end(BUFFER *wb) {
    buffer_json_array_close(wb);
    buffer_json_object_close(wb);
}

static void topology_v1_add_nullable_uint(BUFFER *wb, bool has_value, uint64_t value) {
    if(has_value)
        buffer_json_add_array_item_uint64(wb, value);
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static size_t topology_v1_uint_json_size(uint64_t value) {
    size_t size = 1;
    while(value >= 10) {
        value /= 10;
        size++;
    }

    return size;
}

static size_t topology_v1_string_json_size(const char *value) {
    if(!value || !*value)
        return 4; // null

    size_t size = 2; // quotes
    for(const unsigned char *s = (const unsigned char *)value; *s; s++) {
        if(*s == '"' || *s == '\\')
            size += 2;
        else if(*s <= 0x1f)
            size += 6;
        else
            size++;
    }

    return size;
}

static void topology_v1_string_column_init(NV_TOPOLOGY_V1_STRING_COLUMN *column, size_t rows) {
    *column = (NV_TOPOLOGY_V1_STRING_COLUMN){
        .rows = rows,
        .indexes = rows ? callocz(rows, sizeof(*column->indexes)) : NULL,
        .index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL,
            sizeof(uint64_t)),
    };
}

static void topology_v1_string_column_free(NV_TOPOLOGY_V1_STRING_COLUMN *column) {
    if(!column)
        return;

    freez(column->values);
    freez(column->indexes);
    if(column->index)
        dictionary_destroy(column->index);
    *column = (NV_TOPOLOGY_V1_STRING_COLUMN){ 0 };
}

static void topology_v1_string_column_add(NV_TOPOLOGY_V1_STRING_COLUMN *column, const char *value) {
    if(!column || column->rows_used >= column->rows)
        return;

    const char *stored_value = (value && *value) ? value : NULL;
    const char *key = stored_value ? stored_value : "\001";
    uint64_t index = 0;
    uint64_t *stored = column->index ? dictionary_get(column->index, key) : NULL;

    if(stored) {
        index = *stored;
    }
    else {
        index = column->values_used;
        if(column->values_used == column->values_size) {
            size_t new_size = column->values_size ? column->values_size * 2 : 16;
            column->values = reallocz(column->values, new_size * sizeof(*column->values));
            column->values_size = new_size;
        }

        column->values[column->values_used++] = stored_value;
        if(column->index)
            dictionary_set(column->index, key, &index, sizeof(index));

        column->unique_json_size += topology_v1_string_json_size(stored_value);
        if(column->values_used > 1)
            column->unique_json_size++;
    }

    column->indexes[column->rows_used] = index;
    column->indexes_json_size += topology_v1_uint_json_size(index);
    column->values_json_size += topology_v1_string_json_size(stored_value);
    if(column->rows_used)
        column->indexes_json_size++, column->values_json_size++;
    column->rows_used++;
}

static void topology_v1_emit_auto_string_column(BUFFER *wb, const NV_TOPOLOGY_V1_STRING_COLUMN *column) {
    size_t values_encoding_size = sizeof("{\"codec\":\"values\",\"values\":[]}") - 1 + column->values_json_size;
    size_t dict_encoding_size =
        sizeof("{\"codec\":\"dict\",\"values\":[],\"indexes\":[]}") - 1 +
        column->unique_json_size + column->indexes_json_size;
    bool use_dict = column->rows > 0 && column->values_used < column->rows && dict_encoding_size < values_encoding_size;

    buffer_json_add_array_item_object(wb);
    if(use_dict) {
        buffer_json_member_add_string(wb, "codec", "dict");
        buffer_json_member_add_array(wb, "values");
        for(size_t i = 0; i < column->values_used; i++)
            buffer_json_add_array_item_string(wb, column->values[i]);
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "indexes");
        for(size_t i = 0; i < column->rows_used; i++)
            buffer_json_add_array_item_uint64(wb, column->indexes[i]);
        buffer_json_array_close(wb);
    }
    else {
        buffer_json_member_add_string(wb, "codec", "values");
        buffer_json_member_add_array(wb, "values");
        for(size_t i = 0; i < column->rows_used; i++)
            buffer_json_add_array_item_string(wb, column->values[column->indexes[i]]);
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_actor_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "id", "string", "identity", false, NULL);
    topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "layer", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "machine_guid", "string", "merge_identity", true, NULL);
    topology_v1_emit_column(wb, "hostname", "string", "merge_identity", true, NULL);
    topology_v1_emit_column(wb, "process", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "username", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "cmdline", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "pid", "uint", "identity", true, NULL);
    topology_v1_emit_column(wb, "ppid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "uid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "net_ns_inode", "uint", "identity", true, NULL);
    topology_v1_emit_column(wb, "namespace_type", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "local_ip", "ip", "attribute", true, NULL);
    topology_v1_emit_column(wb, "local_address_space", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "ip", "ip", "identity", true, NULL);
    topology_v1_emit_column(wb, "address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "display_name", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "local_ip_count", "uint", "metric", true, "sum");
}

static void topology_v1_emit_link_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "direction", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "evidence_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
}

static void topology_v1_emit_socket_evidence_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "link", "link_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "local_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "local_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "remote_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "remote_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "peer_port", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol_family", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "direction", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "namespace_type", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "local_address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "remote_address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "pid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "uid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "net_ns_inode", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "process", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
}

static void topology_v1_emit_connection_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "local_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "local_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "remote_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "remote_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "peer_port", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "direction", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "service", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
}

static void topology_v1_emit_socket_port_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "direction", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
}

static void topology_v1_emit_actor_label_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "key", "string", "attribute", false, NULL);
    topology_v1_emit_column(wb, "value", "string", "attribute", false, NULL);
    topology_v1_emit_column(wb, "source", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "kind", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "value_index", "uint", "attribute", true, NULL);
}

static void topology_v1_emit_modal_direct_column_with_visibility(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *column,
    const char *cell,
    const char *visibility);

static void topology_v1_emit_modal_direct_column(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *column,
    const char *cell) {
    topology_v1_emit_modal_direct_column_with_visibility(wb, id, label, column, cell, NULL);
}

static void topology_v1_emit_modal_direct_column_with_visibility(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *column,
    const char *cell,
    const char *visibility) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "direct");
            buffer_json_member_add_string(wb, "column", column);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", cell);
        if(visibility)
            buffer_json_member_add_string(wb, "visibility", visibility);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_modal_opposite_actor_column_labeled(BUFFER *wb, const char *id, const char *label) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "opposite_actor");
            buffer_json_member_add_string(wb, "src_actor_column", "src_actor");
            buffer_json_member_add_string(wb, "dst_actor_column", "dst_actor");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", "actor_link");
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_modal_selected_socket_endpoint_column(BUFFER *wb) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", "endpoint");
        buffer_json_member_add_string(wb, "label", "Endpoint");
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "selected_side_endpoint");
            buffer_json_member_add_string(wb, "src_actor_column", "src_actor");
            buffer_json_member_add_string(wb, "dst_actor_column", "dst_actor");
            buffer_json_member_add_string(wb, "local_ip_column", "local_ip");
            buffer_json_member_add_string(wb, "local_port_column", "local_port");
            buffer_json_member_add_string(wb, "remote_ip_column", "remote_ip");
            buffer_json_member_add_string(wb, "remote_port_column", "remote_port");
            buffer_json_member_add_string(wb, "protocol_column", "protocol");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", "endpoint");
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_modal_identification_field(BUFFER *wb, const char *key, const char *label) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "key", key);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_uint64(wb, "max_values", 1);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_modal_label_identification(BUFFER *wb, const char *actor_type) {
    buffer_json_member_add_object(wb, "identification");
    {
        buffer_json_member_add_boolean(wb, "enabled", true);
        buffer_json_member_add_array(wb, "fields");
        {
            if(strcmp(actor_type, "self") == 0) {
                topology_v1_emit_modal_identification_field(wb, "hostname", "Hostname");
                topology_v1_emit_modal_identification_field(wb, "socket_count", "Sockets");
                topology_v1_emit_modal_identification_field(wb, "local_ip_count", "Local IPs");
            }
            else if(strcmp(actor_type, "process") == 0) {
                topology_v1_emit_modal_identification_field(wb, "process", "Process");
                topology_v1_emit_modal_identification_field(wb, "username", "User");
                topology_v1_emit_modal_identification_field(wb, "namespace_type", "Namespace");
                topology_v1_emit_modal_identification_field(wb, "local_ip", "Local IP");
                topology_v1_emit_modal_identification_field(wb, "cmdline", "Command");
                topology_v1_emit_modal_identification_field(wb, "socket_count", "Sockets");
            }
            else if(strcmp(actor_type, "endpoint") == 0) {
                topology_v1_emit_modal_identification_field(wb, "ip", "IP");
                topology_v1_emit_modal_identification_field(wb, "address_space", "Address Space");
                topology_v1_emit_modal_identification_field(wb, "socket_count", "Sockets");
            }
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_actor_type(
    BUFFER *wb,
    const char *id,
    const char *merge_a,
    const char *merge_b,
    const char *scope,
    const char *label,
    const char *color_slot,
    const char *icon,
    const char *role,
    bool border,
    const char *size_mode,
    const char *size_metric_column,
    bool show_port_bullets,
    const char *port_table,
    bool detailed,
    const char *label_column_a,
    const char *label_column_b) {
    bool is_self = strcmp(id, "self") == 0;
    bool is_endpoint = strcmp(id, "endpoint") == 0;

    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "layer", NETWORK_TOPOLOGY_LAYER);
        buffer_json_member_add_array(wb, "identity");
        buffer_json_add_array_item_string(wb, "id");
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "merge_identity");
        if(merge_a)
            buffer_json_add_array_item_string(wb, merge_a);
        if(merge_b)
            buffer_json_add_array_item_string(wb, merge_b);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "aggregation_scopes");
        buffer_json_add_array_item_string(wb, scope);
        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "presentation");
        {
            buffer_json_member_add_string(wb, "label", label);
            buffer_json_member_add_string(wb, "role", role);
            buffer_json_member_add_string(wb, "icon", icon);
            buffer_json_member_add_string(wb, "color_slot", color_slot);
            buffer_json_member_add_object(wb, "border");
            {
                buffer_json_member_add_boolean(wb, "enabled", border);
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "size");
            {
                buffer_json_member_add_string(wb, "mode", size_mode ? size_mode : "fixed");
                if(size_metric_column)
                    buffer_json_member_add_string(wb, "metric_column", size_metric_column);
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "label_policy");
            {
                buffer_json_member_add_array(wb, "columns");
                if(label_column_a)
                    buffer_json_add_array_item_string(wb, label_column_a);
                if(label_column_b)
                    buffer_json_add_array_item_string(wb, label_column_b);
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "fallback", "type_label");
                buffer_json_member_add_uint64(wb, "max_length", 80);
                buffer_json_member_add_string(wb, "array", "reject");
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "ports");
            {
                buffer_json_member_add_boolean(wb, "show_bullets", show_port_bullets);
                if(show_port_bullets) {
                    buffer_json_member_add_array(wb, "sources");
                    {
                        buffer_json_add_array_item_object(wb);
                        buffer_json_member_add_string(wb, "source", "actor_table");
                        buffer_json_member_add_string(wb, "table", port_table ? port_table : "socket_ports");
                        buffer_json_member_add_string(wb, "actor_column", "actor");
                        buffer_json_member_add_string(wb, "name_column", "port");
                        buffer_json_member_add_string(wb, "value_column", "socket_count");
                        buffer_json_member_add_string(wb, "default_type", "topology");
                        buffer_json_object_close(wb);
                    }
                    buffer_json_array_close(wb);
                }
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "modal");
            {
                buffer_json_member_add_object(wb, "labels");
                {
                    buffer_json_member_add_string(wb, "table", "actor_labels");
                    topology_v1_emit_modal_label_identification(wb, id);
                }
                buffer_json_object_close(wb);
                buffer_json_member_add_object(wb, "mini_topology");
                {
                    buffer_json_member_add_uint64(wb, "depth", 1);
                    buffer_json_member_add_array(wb, "exclude_link_types");
                    if(!is_self)
                        buffer_json_add_array_item_string(wb, "ownership");
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
                buffer_json_member_add_array(wb, "sections");
                {
                    buffer_json_add_array_item_object(wb);
                    {
                        if(is_self) {
                            buffer_json_member_add_string(wb, "id", "processes");
                            buffer_json_member_add_string(wb, "label", "Processes");
                        }
                        else if(is_endpoint) {
                            buffer_json_member_add_string(wb, "id", "processes");
                            buffer_json_member_add_string(wb, "label", "Processes");
                        }
                        else {
                            buffer_json_member_add_string(wb, "id", detailed ? "sockets" : "connections");
                            buffer_json_member_add_string(wb, "label", detailed ? "Sockets" : "Connections");
                        }
                        buffer_json_member_add_uint64(wb, "order", 1);
                        buffer_json_member_add_object(wb, "source");
                        {
                            if(is_self)
                                buffer_json_member_add_string(wb, "kind", "links");
                            else
                                buffer_json_member_add_string(wb, "kind", detailed ? "evidence" : "relationship_table");

                            if(!is_self && detailed)
                                buffer_json_member_add_string(wb, "evidence", "socket");
                            else if(!is_self)
                                buffer_json_member_add_string(wb, "table", "connections");
                        }
                        buffer_json_object_close(wb);
                        buffer_json_member_add_object(wb, "owner_filter");
                        {
                            buffer_json_member_add_string(wb, "mode", (!is_self && detailed) ? "incident_evidence" : "incident_link");
                            buffer_json_member_add_string(wb, "src_actor_column", "src_actor");
                            buffer_json_member_add_string(wb, "dst_actor_column", "dst_actor");
                        }
                        buffer_json_object_close(wb);
                        if(is_self) {
                            buffer_json_member_add_array(wb, "row_filters");
                            {
                                buffer_json_add_array_item_object(wb);
                                {
                                    buffer_json_member_add_string(wb, "column", "type");
                                    buffer_json_member_add_string(wb, "op", "eq");
                                    buffer_json_member_add_string(wb, "value", "ownership");
                                }
                                buffer_json_object_close(wb);
                            }
                            buffer_json_array_close(wb);
                        }
                        buffer_json_member_add_array(wb, "columns");
                        {
                            if(is_self) {
                                topology_v1_emit_modal_opposite_actor_column_labeled(wb, "process", "Process");
                            }
                            else {
                                topology_v1_emit_modal_opposite_actor_column_labeled(
                                    wb, is_endpoint ? "process" : "remote", is_endpoint ? "Process" : "Remote");
                                topology_v1_emit_modal_selected_socket_endpoint_column(wb);
                                if(!detailed)
                                    topology_v1_emit_modal_direct_column(wb, "service", "Service", "service", "text");
                                topology_v1_emit_modal_direct_column(wb, "protocol", "Protocol", "protocol", "badge");
                                topology_v1_emit_modal_direct_column(wb, "direction", "Direction", "direction", "badge");
                                topology_v1_emit_modal_direct_column(wb, "state", "State", "state", "badge");
                            }
                            topology_v1_emit_modal_direct_column(wb, "sockets", "Sockets", "socket_count", "number");
                            if(is_self)
                                topology_v1_emit_modal_direct_column_with_visibility(
                                    wb, "evidence", "Evidence", "evidence_count", "number", "expanded");
                            else {
                                topology_v1_emit_modal_direct_column_with_visibility(
                                    wb, "retransmissions", "Retransmissions", "retransmissions", "number", "expanded");
                                topology_v1_emit_modal_direct_column(wb, "rtt", "RTT max", "rtt_ms_max", "number");
                                topology_v1_emit_modal_direct_column_with_visibility(
                                    wb, "recv_rtt", "Receiver RTT max", "recv_rtt_ms_max", "number", "expanded");
                            }
                        }
                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

        }
        buffer_json_object_close(wb);

    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_link_type(
    BUFFER *wb,
    const char *id,
    const char *orientation,
    const char *direction_role,
    const char *evidence_type,
    const char *label,
    const char *color_slot,
    const char *line_style,
    const char *width,
    const char *arrow,
    const char *opacity,
    const char *scale_key,
    const char *value_column,
    const char *layout_strength,
    const char *layout_distance) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "orientation", orientation);
        buffer_json_member_add_string(wb, "direction_role", direction_role);
        buffer_json_member_add_object(wb, "aggregation");
        {
            buffer_json_member_add_string(wb, "direction", "preserve");
            buffer_json_member_add_string(wb, "evidence", evidence_type ? "append" : "count");
            buffer_json_member_add_object(wb, "metrics");
            {
                buffer_json_member_add_string(wb, "evidence_count", "sum");
                buffer_json_member_add_string(wb, "socket_count", "sum");
                if(evidence_type) {
                    buffer_json_member_add_string(wb, "retransmissions", "sum");
                    buffer_json_member_add_string(wb, "rtt_ms_max", "max");
                    buffer_json_member_add_string(wb, "recv_rtt_ms_max", "max");
                }
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
        if(evidence_type) {
            buffer_json_member_add_array(wb, "evidence_types");
            buffer_json_add_array_item_string(wb, evidence_type);
            buffer_json_array_close(wb);
        }
        buffer_json_member_add_object(wb, "presentation");
        {
            buffer_json_member_add_string(wb, "label", label);
            buffer_json_member_add_string(wb, "color_slot", color_slot);
            if(opacity)
                buffer_json_member_add_string(wb, "opacity", opacity);
            buffer_json_member_add_string(wb, "line_style", line_style);
            buffer_json_member_add_string(wb, "width", width);
            buffer_json_member_add_string(wb, "curve", "auto");
            buffer_json_member_add_string(wb, "arrow", arrow);
            if(layout_strength || layout_distance) {
                buffer_json_member_add_object(wb, "layout");
                {
                    if(layout_strength)
                        buffer_json_member_add_string(wb, "strength", layout_strength);
                    if(layout_distance)
                        buffer_json_member_add_string(wb, "distance", layout_distance);
                }
                buffer_json_object_close(wb);
            }
            if(scale_key && value_column) {
                buffer_json_member_add_object(wb, "variable");
                {
                    buffer_json_member_add_string(wb, "channel", "width");
                    buffer_json_member_add_string(wb, "scale_key", scale_key);
                    buffer_json_member_add_string(wb, "value_column", value_column);
                    buffer_json_member_add_string(wb, "min", width);
                    buffer_json_member_add_string(wb, "max", "emphasis");
                }
                buffer_json_object_close(wb);
            }
        }
        buffer_json_object_close(wb);

    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_type_registry(BUFFER *wb, bool detailed __maybe_unused) {
    buffer_json_member_add_object(wb, "types");
    {
        buffer_json_member_add_object(wb, "actor_types");
        {
            topology_v1_emit_actor_type(
                wb, "self", "machine_guid", "hostname", "node",
                "This host", "self", "self", "actor", true, "fixed", NULL, false, NULL,
                detailed,
                "display_name", "hostname");
            topology_v1_emit_actor_type(
                wb, "process", "machine_guid", "process", "process_name",
                "Process", "primary", "process", "actor", true, "metric", "socket_count", true, "socket_ports",
                detailed,
                "display_name", "process");
            topology_v1_emit_actor_type(
                wb, "endpoint", "ip", "address_space", "endpoint",
                "Correlation endpoint", "derived", "remote-endpoint", "endpoint", true, "fixed", NULL, false, NULL,
                detailed,
                "display_name", "ip");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "link_types");
        {
            topology_v1_emit_link_type(
                wb, "socket", "directed", "flow", "socket",
                "Local socket", "gray", "solid", "thin", "forward", NULL,
                "sockets", "socket_count", "stronger", "farther");
            topology_v1_emit_link_type(
                wb, "endpoint_socket", "directed", "flow", "socket",
                "Endpoint connection", "primary", "solid", "thin", "forward", NULL,
                NULL, NULL, "weakest", "normal");
            topology_v1_emit_link_type(
                wb, "correlated_socket", "directed", "flow", "socket",
                "Correlated socket", "primary", "solid", "thin", "forward", NULL,
                "sockets", "socket_count", "weakest", "farthest");
            topology_v1_emit_link_type(
                wb, "ownership", "hierarchical", "ownership", NULL,
                "Process ownership", "dim", "dotted", "thin", "none", "faded",
                NULL, NULL, "normal", "normal");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "port_types");
        {
            buffer_json_member_add_object(wb, "topology");
            {
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Socket");
                    buffer_json_member_add_string(wb, "color_slot", "primary");
                    buffer_json_member_add_string(wb, "opacity", "normal");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "evidence_types");
        {
            buffer_json_member_add_object(wb, "socket");
            {
                buffer_json_member_add_string(wb, "link_type", "socket");
                buffer_json_member_add_string(wb, "role", "relationship_evidence");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_socket_evidence_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_array(wb, "match_columns");
                buffer_json_add_array_item_string(wb, "local_ip");
                buffer_json_add_array_item_string(wb, "local_port");
                buffer_json_add_array_item_string(wb, "remote_ip");
                buffer_json_add_array_item_string(wb, "remote_port");
                buffer_json_add_array_item_string(wb, "protocol");
                buffer_json_add_array_item_string(wb, "direction");
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "table_types");
        {
            buffer_json_member_add_object(wb, "socket_ports");
            {
                buffer_json_member_add_string(wb, "role", "actor_inventory");
                buffer_json_member_add_string(wb, "owner", "actor");
                buffer_json_member_add_string(wb, "aggregation", "sum");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_socket_port_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Ports");
                    buffer_json_member_add_uint64(wb, "order", 1);
                    buffer_json_member_add_array(wb, "columns");
                    {
                        topology_v1_emit_modal_direct_column(wb, "port", "Port", "port", "number");
                        topology_v1_emit_modal_direct_column(wb, "protocol", "Protocol", "protocol", "badge");
                        topology_v1_emit_modal_direct_column(wb, "direction", "Direction", "direction", "badge");
                        topology_v1_emit_modal_direct_column(wb, "sockets", "Sockets", "socket_count", "number");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "connections");
            {
                buffer_json_member_add_string(wb, "role", "relationship_summary");
                buffer_json_member_add_string(wb, "owner", "link");
                buffer_json_member_add_string(wb, "aggregation", "merge_metrics");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_connection_columns(wb);
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "actor_labels");
            {
                buffer_json_member_add_string(wb, "role", "actor_inventory");
                buffer_json_member_add_string(wb, "owner", "actor");
                buffer_json_member_add_string(wb, "aggregation", "set");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_actor_label_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Labels");
                    buffer_json_member_add_uint64(wb, "order", 0);
                    buffer_json_member_add_array(wb, "columns");
                    {
                        topology_v1_emit_modal_direct_column(wb, "key", "Label", "key", "text");
                        topology_v1_emit_modal_direct_column(wb, "value", "Value", "value", "text");
                        topology_v1_emit_modal_direct_column(wb, "source", "Source", "source", "badge");
                        topology_v1_emit_modal_direct_column(wb, "kind", "Kind", "kind", "badge");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "aggregation_scopes");
        {
            buffer_json_member_add_object(wb, "node");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "machine_guid");
                buffer_json_add_array_item_string(wb, "hostname");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "evidence_policy", "preserve");
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "process_name");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "process");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "evidence_policy", "preserve");
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "pid");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "pid");
                buffer_json_add_array_item_string(wb, "net_ns_inode");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "evidence_policy", "preserve");
            }
            buffer_json_object_close(wb);

        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_presentation(BUFFER *wb) {
    buffer_json_member_add_object(wb, "presentation");
    {
        buffer_json_member_add_string(wb, "profile_version", "network-connections.v1");
        buffer_json_member_add_object(wb, "selection");
        {
            buffer_json_member_add_object(wb, "actor_click");
            {
                buffer_json_member_add_string(wb, "mode", "highlight_connections");
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
                buffer_json_member_add_string(wb, "label", "Correlation endpoint");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);

            buffer_json_member_add_array(wb, "links");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "ownership");
                buffer_json_member_add_string(wb, "label", "Process ownership");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "socket");
                buffer_json_member_add_string(wb, "label", "Local socket");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "endpoint_socket");
                buffer_json_member_add_string(wb, "label", "Endpoint connection");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "correlated_socket");
                buffer_json_member_add_string(wb, "label", "Correlated socket");
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

        buffer_json_member_add_array(wb, "port_fields");
        {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "key", "type");
            buffer_json_member_add_string(wb, "label", "Type");
            buffer_json_object_close(wb);

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "key", "socket_count");
            buffer_json_member_add_string(wb, "label", "Sockets");
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "scale_keys");
        {
            buffer_json_member_add_object(wb, "sockets");
            {
                buffer_json_member_add_string(wb, "label", "Sockets");
                buffer_json_member_add_string(wb, "unit", "count");
            }
            buffer_json_object_close(wb);

        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_actor_table(BUFFER *wb, NV_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "actors");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->actors_used);
        buffer_json_member_add_array(wb, "columns");
        topology_v1_emit_actor_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) \
                buffer_json_add_array_item_string(wb, payload->actors[i].member[0] ? payload->actors[i].member : NULL); \
            topology_v1_values_end(wb); \
        } while(0)
#define NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(member, has_member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) \
                topology_v1_add_nullable_uint(wb, payload->actors[i].has_member, payload->actors[i].member); \
            topology_v1_values_end(wb); \
        } while(0)

        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(id);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(type);
        topology_v1_const_string(wb, NETWORK_TOPOLOGY_LAYER);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(machine_guid);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(hostname);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(process);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(username);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(cmdline);
        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(pid, has_pid);
        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(ppid, has_ppid);
        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(uid, has_uid);
        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(net_ns_inode, has_net_ns_inode);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(namespace_type);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(local_ip);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(local_address_space);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(ip);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(address_space);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(display_name);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->actors_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->actors[i].sockets);
        topology_v1_values_end(wb);

        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(local_ip_count, has_local_ip_count);

#undef NV_TOPOLOGY_V1_ACTOR_STRING_VALUES
#undef NV_TOPOLOGY_V1_ACTOR_UINT_VALUES

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_link_table(BUFFER *wb, NV_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "links");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->links_used);
        buffer_json_member_add_array(wb, "columns");
        topology_v1_emit_link_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_LINK_UINT_VALUES(member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->links_used; i++) \
                buffer_json_add_array_item_uint64(wb, payload->links[i].member); \
            topology_v1_values_end(wb); \
        } while(0)
#define NV_TOPOLOGY_V1_LINK_STRING_VALUES(member) do { \
            NV_TOPOLOGY_V1_STRING_COLUMN column; \
            topology_v1_string_column_init(&column, payload->links_used); \
            for(size_t i = 0; i < payload->links_used; i++) \
                topology_v1_string_column_add(&column, payload->links[i].member); \
            topology_v1_emit_auto_string_column(wb, &column); \
            topology_v1_string_column_free(&column); \
        } while(0)

        NV_TOPOLOGY_V1_LINK_UINT_VALUES(src_actor);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(dst_actor);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(type);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(protocol);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(direction);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(state);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(evidence_count);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(socket_count);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(retransmissions);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_double(wb, (double)payload->links[i].max_rtt_usec / (double)USEC_PER_MS);
        topology_v1_values_end(wb);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_double(wb, (double)payload->links[i].max_rcv_rtt_usec / (double)USEC_PER_MS);
        topology_v1_values_end(wb);

#undef NV_TOPOLOGY_V1_LINK_UINT_VALUES
#undef NV_TOPOLOGY_V1_LINK_STRING_VALUES

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_socket_port_table(BUFFER *wb, NV_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "tables");
    {
        buffer_json_member_add_object(wb, "actor");
        {
            buffer_json_member_add_object(wb, "socket_ports");
            {
                buffer_json_member_add_string(wb, "type", "socket_ports");
                buffer_json_member_add_object(wb, "table");
                {
                    buffer_json_member_add_uint64(wb, "rows", payload->ports_used);
                    buffer_json_member_add_array(wb, "columns");
                    topology_v1_emit_socket_port_columns(wb);
                    buffer_json_array_close(wb);
                    buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_PORT_UINT_VALUES(member) do { \
                    topology_v1_values_start(wb); \
                    for(size_t i = 0; i < payload->ports_used; i++) \
                        buffer_json_add_array_item_uint64(wb, payload->ports[i].member); \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_PORT_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->ports_used); \
                    for(size_t i = 0; i < payload->ports_used; i++) \
                        topology_v1_string_column_add(&column, payload->ports[i].member); \
                    topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                } while(0)

                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(port);
                    NV_TOPOLOGY_V1_PORT_STRING_VALUES(protocol);
                    NV_TOPOLOGY_V1_PORT_STRING_VALUES(direction);
                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(socket_count);

#undef NV_TOPOLOGY_V1_PORT_UINT_VALUES
#undef NV_TOPOLOGY_V1_PORT_STRING_VALUES

                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "actor_labels");
            {
                buffer_json_member_add_string(wb, "type", "actor_labels");
                buffer_json_member_add_object(wb, "table");
                {
                    buffer_json_member_add_uint64(wb, "rows", payload->labels_used);
                    buffer_json_member_add_array(wb, "columns");
                    topology_v1_emit_actor_label_columns(wb);
                    buffer_json_array_close(wb);
                    buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_LABEL_UINT_VALUES(member) do { \
                    topology_v1_values_start(wb); \
                    for(size_t i = 0; i < payload->labels_used; i++) \
                        buffer_json_add_array_item_uint64(wb, payload->labels[i].member); \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_LABEL_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->labels_used); \
                    for(size_t i = 0; i < payload->labels_used; i++) \
                        topology_v1_string_column_add(&column, payload->labels[i].member); \
                    topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                } while(0)

                    NV_TOPOLOGY_V1_LABEL_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(key);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(value);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(source);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(kind);

                    topology_v1_values_start(wb);
                    for(size_t i = 0; i < payload->labels_used; i++)
                        topology_v1_add_nullable_uint(wb, payload->labels[i].has_value_index, payload->labels[i].value_index);
                    topology_v1_values_end(wb);

#undef NV_TOPOLOGY_V1_LABEL_UINT_VALUES
#undef NV_TOPOLOGY_V1_LABEL_STRING_VALUES

                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        if(payload->connections_used) {
            buffer_json_member_add_object(wb, "relationship");
            {
                buffer_json_member_add_object(wb, "connections");
                {
                    buffer_json_member_add_string(wb, "type", "connections");
                    buffer_json_member_add_object(wb, "table");
                    {
                        buffer_json_member_add_uint64(wb, "rows", payload->connections_used);
                        buffer_json_member_add_array(wb, "columns");
                        topology_v1_emit_connection_columns(wb);
                        buffer_json_array_close(wb);
                        buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(member) do { \
                            topology_v1_values_start(wb); \
                            for(size_t i = 0; i < payload->connections_used; i++) \
                                buffer_json_add_array_item_uint64(wb, payload->connections[i].member); \
                            topology_v1_values_end(wb); \
                        } while(0)
#define NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(member) do { \
                            topology_v1_values_start(wb); \
                            for(size_t i = 0; i < payload->connections_used; i++) \
                                buffer_json_add_array_item_uint64(wb, payload->connections[i].source->member); \
                            topology_v1_values_end(wb); \
                        } while(0)
#define NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(member) do { \
                            NV_TOPOLOGY_V1_STRING_COLUMN column; \
                            topology_v1_string_column_init(&column, payload->connections_used); \
                            for(size_t i = 0; i < payload->connections_used; i++) \
                                topology_v1_string_column_add(&column, payload->connections[i].source->member); \
                            topology_v1_emit_auto_string_column(wb, &column); \
                            topology_v1_string_column_free(&column); \
                        } while(0)

                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(src_actor);
                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(dst_actor);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(local_ip);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(local_port);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(remote_ip);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(remote_port);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(peer_port);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(protocol);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(direction);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(state);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES(port_name);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(sockets);
                        NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES(retransmissions);

                        topology_v1_values_start(wb);
                        for(size_t i = 0; i < payload->connections_used; i++)
                            buffer_json_add_array_item_double(wb, (double)payload->connections[i].source->max_rtt_usec / (double)USEC_PER_MS);
                        topology_v1_values_end(wb);

                        topology_v1_values_start(wb);
                        for(size_t i = 0; i < payload->connections_used; i++)
                            buffer_json_add_array_item_double(wb, (double)payload->connections[i].source->max_rcv_rtt_usec / (double)USEC_PER_MS);
                        topology_v1_values_end(wb);

#undef NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES
#undef NV_TOPOLOGY_V1_CONNECTION_SOURCE_UINT_VALUES
#undef NV_TOPOLOGY_V1_CONNECTION_SOURCE_STRING_VALUES

                        buffer_json_array_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_socket_evidence_table(BUFFER *wb, NV_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "evidence");
    {
        buffer_json_member_add_object(wb, "socket");
        {
            buffer_json_member_add_string(wb, "type", "socket");
            buffer_json_member_add_object(wb, "table");
            {
                buffer_json_member_add_uint64(wb, "rows", payload->evidence_used);
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_socket_evidence_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(member) do { \
                topology_v1_values_start(wb); \
                for(size_t i = 0; i < payload->evidence_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->evidence[i].member); \
                topology_v1_values_end(wb); \
            } while(0)
#define NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(member) do { \
                topology_v1_values_start(wb); \
                for(size_t i = 0; i < payload->evidence_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->evidence[i].source->member); \
                topology_v1_values_end(wb); \
            } while(0)
#define NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(member) do { \
                NV_TOPOLOGY_V1_STRING_COLUMN column; \
                topology_v1_string_column_init(&column, payload->evidence_used); \
                for(size_t i = 0; i < payload->evidence_used; i++) \
                    topology_v1_string_column_add(&column, payload->evidence[i].source->member); \
                topology_v1_emit_auto_string_column(wb, &column); \
                topology_v1_string_column_free(&column); \
            } while(0)

                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(link);
                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(src_actor);
                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(dst_actor);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(local_ip);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(local_port);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(remote_ip);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(remote_port);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(peer_port);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(protocol);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(protocol_family);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(direction);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(state);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(namespace_type);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(local_address_space);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(remote_address_space);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(pid);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(uid);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(net_ns_inode);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(process);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(sockets);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(retransmissions);

                topology_v1_values_start(wb);
                for(size_t i = 0; i < payload->evidence_used; i++)
                    buffer_json_add_array_item_double(wb, (double)payload->evidence[i].source->max_rtt_usec / (double)USEC_PER_MS);
                topology_v1_values_end(wb);

                topology_v1_values_start(wb);
                for(size_t i = 0; i < payload->evidence_used; i++)
                    buffer_json_add_array_item_double(wb, (double)payload->evidence[i].source->max_rcv_rtt_usec / (double)USEC_PER_MS);
                topology_v1_values_end(wb);

#undef NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES
#undef NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES
#undef NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES

                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_correlation_table(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_CORRELATION_ROW *rows,
    size_t rows_used) {
    buffer_json_member_add_uint64(wb, "rows", rows_used);

    buffer_json_member_add_array(wb, "columns");
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "rule", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "address_space", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "port", "uint", "group_key", false, NULL);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "values");

    topology_v1_values_start(wb);
    for(size_t i = 0; i < rows_used; i++)
        buffer_json_add_array_item_uint64(wb, rows[i].actor);
    topology_v1_values_end(wb);

    topology_v1_const_string(wb, "socket_exact");

#define NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(member) do { \
        NV_TOPOLOGY_V1_STRING_COLUMN column; \
        topology_v1_string_column_init(&column, rows_used); \
        for(size_t i = 0; i < rows_used; i++) \
            topology_v1_string_column_add(&column, rows[i].member); \
        topology_v1_emit_auto_string_column(wb, &column); \
        topology_v1_string_column_free(&column); \
    } while(0)

    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(protocol);
    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(address_space);
    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(ip);

#undef NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES

    topology_v1_values_start(wb);
    for(size_t i = 0; i < rows_used; i++)
        buffer_json_add_array_item_uint64(wb, rows[i].port);
    topology_v1_values_end(wb);

    buffer_json_array_close(wb);
}

static void topology_v1_emit_correlation(BUFFER *wb, NV_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "correlation");
    {
        buffer_json_member_add_object(wb, "rules");
        {
            buffer_json_member_add_object(wb, "socket_exact");
            {
                buffer_json_member_add_string(wb, "action", "absorb");
                buffer_json_member_add_string(wb, "class", "resolve_loose_side");
                buffer_json_member_add_uint64(wb, "priority", 1);
                buffer_json_member_add_string(wb, "key_space", "network_socket");
                buffer_json_member_add_array(wb, "key");
                {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "column", "protocol");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "literal", ":");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "column", "address_space");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "literal", ":");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "column", "ip");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "literal", ":");
                    buffer_json_object_close(wb);
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "column", "port");
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb);
                buffer_json_member_add_array(wb, "point_actor_types");
                buffer_json_add_array_item_string(wb, "endpoint");
                buffer_json_array_close(wb);
                buffer_json_member_add_array(wb, "claim_actor_types");
                buffer_json_add_array_item_string(wb, "process");
                buffer_json_array_close(wb);
                buffer_json_member_add_array(wb, "correlation_link_types");
                buffer_json_add_array_item_string(wb, "endpoint_socket");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "output_link_type", "correlated_socket");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "points");
        topology_v1_emit_correlation_table(wb, payload->correlation_points, payload->correlation_points_used);
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "claims");
        topology_v1_emit_correlation_table(wb, payload->correlation_claims, payload->correlation_claims_used);
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_write_data(BUFFER *wb, const NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx || ctx->options.info_only || !ctx->process_actors || !ctx->remote_actors || !ctx->local_ips || !ctx->links)
        return;

    NV_TOPOLOGY_RENDER_STATE state;
    topology_render_state_init(&state, ctx);

    NV_TOPOLOGY_V1_PAYLOAD topology = {
        .actor_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .graph_link_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .port_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .correlation_point_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(bool)),
        .correlation_claim_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(bool)),
    };

    if(!topology.actor_index || !topology.graph_link_index || !topology.port_index ||
       !topology.correlation_point_index || !topology.correlation_claim_index) {
        topology_v1_free(&topology);
        return;
    }

    topology_v1_collect_actors(ctx, &state, &topology);
    topology_v1_collect_links(ctx, &state, &topology);

    buffer_json_member_add_object(wb, "data");
    {
        buffer_json_member_add_string(wb, "schema_version", NETWORK_TOPOLOGY_SCHEMA_VERSION);
        buffer_json_member_add_object(wb, "producer");
        {
            buffer_json_member_add_string(wb, "source", NETWORK_TOPOLOGY_SOURCE);
            buffer_json_member_add_string(wb, "instance", ctx->machine_guid[0] ? ctx->machine_guid : ctx->hostname);
            if(ctx->machine_guid[0])
                buffer_json_member_add_string(wb, "machine_guid", ctx->machine_guid);
            buffer_json_member_add_string(wb, "plugin", "network-viewer.plugin");
            buffer_json_member_add_array(wb, "capabilities");
            buffer_json_add_array_item_string(wb, "topology-v1");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_datetime_rfc3339(wb, "collected_at", ctx->now_ut, true);

        buffer_json_member_add_object(wb, "view");
        {
            buffer_json_member_add_string(wb, "id", "network-connections");
            buffer_json_member_add_string(wb, "scope", ctx->options.processes_by_pid ? "pid" : "process_name");
            buffer_json_member_add_string(wb, "mode", ctx->options.detailed ? "detailed" : "aggregated");
            buffer_json_member_add_array(wb, "supported_modes");
            buffer_json_add_array_item_string(wb, "aggregated");
            buffer_json_add_array_item_string(wb, "detailed");
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "group_by");
            buffer_json_add_array_item_string(wb, ctx->options.processes_by_pid ? "pid" : "process_name");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "dictionaries");
        {
            buffer_json_member_add_array(wb, "strings");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        topology_v1_emit_type_registry(wb, ctx->options.detailed);
        topology_v1_emit_presentation(wb);
        topology_v1_emit_correlation(wb, &topology);
        topology_v1_emit_actor_table(wb, &topology);
        topology_v1_emit_link_table(wb, &topology);
        topology_v1_emit_socket_port_table(wb, &topology);
        if(ctx->options.detailed)
            topology_v1_emit_socket_evidence_table(wb, &topology);

        buffer_json_member_add_object(wb, "stats");
        {
            buffer_json_member_add_string(wb, "processes_mode", ctx->options.processes_by_pid ? "by_pid" : "by_name");
            buffer_json_member_add_string(wb, "mode", ctx->options.detailed ? "detailed" : "aggregated");
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
            buffer_json_member_add_uint64(wb, "actors", topology.actors_used);
            buffer_json_member_add_uint64(wb, "local_process_actors", state.process_actor_count);
            buffer_json_member_add_uint64(wb, "endpoint_actors", state.endpoint_actor_count);
            buffer_json_member_add_uint64(wb, "links", topology.links_used);
            buffer_json_member_add_uint64(wb, "socket_evidence_rows", topology.evidence_used);
            buffer_json_member_add_uint64(wb, "socket_port_rows", topology.ports_used);
            buffer_json_member_add_uint64(wb, "correlation_points", topology.correlation_points_used);
            buffer_json_member_add_uint64(wb, "correlation_claims", topology.correlation_claims_used);
            buffer_json_member_add_uint64(wb, "ownership_links", state.ownership_link_count);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    topology_v1_free(&topology);
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
