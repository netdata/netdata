// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "network-viewer-apps-lookup-client.h"
#include "network-viewer-topology-containers.h"


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
#define NETWORK_PROTOCOLS_FUNCTION      "network-protocols"
#define NETWORK_PROTOCOLS_FUNCTION_HELP "TCP and UDP statistics (IPv4 and IPv6 combined)"
// Keep in sync with the topology schema contract used across topology producers.
#define NETWORK_TOPOLOGY_SCHEMA_VERSION "netdata.topology.v1"
#define NETWORK_TOPOLOGY_SOURCE "network-connections"
#define NETWORK_TOPOLOGY_LAYER "network"
#define NV_TOPOLOGY_MAX_PPID_DEPTH 64
#define NV_TOPOLOGY_RESPONSE_SIZE_LIMIT (64ULL * 1024ULL * 1024ULL)
#define NETWORK_VIEWER_TEST_DEFAULT_TIMEOUT_SECONDS 60ULL
#define NETWORK_VIEWER_TEST_TIMEOUT_DISABLED_SECONDS (100ULL * 365ULL * 24ULL * 60ULL * 60ULL)
#define NETWORK_VIEWER_TEST_MAX_REQUEST_BYTES (16ULL * 1024ULL * 1024ULL)

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
    uint64_t starttime;
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
    uint64_t starttime;
    char process[TASK_COMM_LEN + 1];
} NV_ENDPOINT_OWNER;

typedef enum {
    NV_TOPOLOGY_ENDPOINT_ROLE_NONE = 0,
    NV_TOPOLOGY_ENDPOINT_ROLE_CLIENT,
    NV_TOPOLOGY_ENDPOINT_ROLE_SERVER,
} NV_TOPOLOGY_ENDPOINT_ROLE;

typedef enum {
    NV_TOPOLOGY_GROUP_BY_PROCESS_NAME = 0,
    NV_TOPOLOGY_GROUP_BY_PID,
    NV_TOPOLOGY_GROUP_BY_CONTAINER,
} NV_TOPOLOGY_GROUP_BY;

typedef struct {
    uint64_t pid;
    uint64_t ppid;
    uint64_t uid;
    uint64_t net_ns_inode;
    uint64_t starttime;
    uint64_t sockets;
    uint64_t retransmissions;
    uint32_t max_rtt_usec;
    uint32_t max_rcv_rtt_usec;
    uint16_t client_port;
    uint16_t server_port;
    uint16_t process_port;
    uint16_t protocol_id;
    bool process_is_client;
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char protocol[8];
    char protocol_family[8];
    char state[32];
    char client_ip[INET6_ADDRSTRLEN];
    char server_ip[INET6_ADDRSTRLEN];
    char client_address_space[16];
    char server_address_space[16];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
} NV_TOPOLOGY_LINK;

typedef struct {
    bool info_only;
    bool detailed;
    NV_TOPOLOGY_GROUP_BY group_by;
    bool sockets_listening;
    bool sockets_inbound;
    bool sockets_outbound;
    bool protocols_ipv4_tcp;
    bool protocols_ipv6_tcp;
    bool protocols_ipv4_udp;
    bool protocols_ipv6_udp;
    SIMPLE_PATTERN *label_whitelist;
} NV_TOPOLOGY_OPTIONS;

typedef struct {
    bool protocols_selected_explicitly;
    bool sockets_selected_explicitly;
} NV_TOPOLOGY_PARSE_STATE;

typedef enum {
    NV_TOPOLOGY_ABORT_NONE = 0,
    NV_TOPOLOGY_ABORT_CANCELLED,
    NV_TOPOLOGY_ABORT_TIMED_OUT,
    NV_TOPOLOGY_ABORT_TOO_LARGE,
} NV_TOPOLOGY_ABORT_STATUS;

typedef struct {
    DICTIONARY *process_actors;
    DICTIONARY *remote_actors;
    DICTIONARY *local_ips;
    DICTIONARY *endpoint_owners_exact;
    DICTIONARY *endpoint_owners_exact_any_ns;
    DICTIONARY *endpoint_owners_service;
    DICTIONARY *endpoint_owners_service_any_ns;
    DICTIONARY *links;
    DICTIONARY *container_field_snapshot;
    DICTIONARY *pid_starttime_cache;
    usec_t now_ut;
    usec_t *stop_monotonic_ut;
    bool *cancelled;
    NV_TOPOLOGY_ABORT_STATUS abort_status;
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
    uint64_t starttime;
} NV_PID_STARTTIME_CACHE_ENTRY;

typedef struct {
    size_t process_actor_count;
    size_t socket_link_count;
    size_t local_ip_count;
    size_t endpoint_actor_count;
    size_t ownership_link_count;
    char host_actor_id[NV_TOPOLOGY_KEY_MAX];
} NV_TOPOLOGY_RENDER_STATE;

static NV_TOPOLOGY_ABORT_STATUS topology_function_abort_state(usec_t *stop_monotonic_ut, bool *cancelled)
{
    if(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED))
        return NV_TOPOLOGY_ABORT_CANCELLED;

    if(stop_monotonic_ut) {
        usec_t stop_ut = __atomic_load_n(stop_monotonic_ut, __ATOMIC_RELAXED);
        if(stop_ut && now_monotonic_usec() >= stop_ut)
            return NV_TOPOLOGY_ABORT_TIMED_OUT;
    }

    return NV_TOPOLOGY_ABORT_NONE;
}

static NV_TOPOLOGY_ABORT_STATUS topology_context_check_abort(NV_TOPOLOGY_CONTEXT *ctx)
{
    if(!ctx)
        return NV_TOPOLOGY_ABORT_NONE;

    if(ctx->abort_status != NV_TOPOLOGY_ABORT_NONE)
        return ctx->abort_status;

    ctx->abort_status = topology_function_abort_state(ctx->stop_monotonic_ut, ctx->cancelled);
    return ctx->abort_status;
}

static bool topology_abort_iteration_checkpoint(size_t iteration)
{
    return (iteration & 0xFF) == 0;
}

static NV_TOPOLOGY_ABORT_STATUS topology_response_check(BUFFER *wb, NV_TOPOLOGY_CONTEXT *ctx)
{
    NV_TOPOLOGY_ABORT_STATUS status = topology_context_check_abort(ctx);
    if(status != NV_TOPOLOGY_ABORT_NONE)
        return status;

    if(wb && buffer_strlen(wb) > NV_TOPOLOGY_RESPONSE_SIZE_LIMIT) {
        if(ctx)
            ctx->abort_status = NV_TOPOLOGY_ABORT_TOO_LARGE;
        return NV_TOPOLOGY_ABORT_TOO_LARGE;
    }

    return NV_TOPOLOGY_ABORT_NONE;
}

static int topology_abort_http_code(NV_TOPOLOGY_ABORT_STATUS status)
{
    if(status == NV_TOPOLOGY_ABORT_TOO_LARGE)
        return HTTP_RESP_CONTENT_TOO_LARGE;

    return status == NV_TOPOLOGY_ABORT_CANCELLED ?
        HTTP_RESP_CLIENT_CLOSED_REQUEST : HTTP_RESP_GATEWAY_TIMEOUT;
}

static const char *topology_abort_message(NV_TOPOLOGY_ABORT_STATUS status)
{
    switch(status) {
        case NV_TOPOLOGY_ABORT_CANCELLED:
            return "Request cancelled.";
        case NV_TOPOLOGY_ABORT_TOO_LARGE:
            return "Topology response exceeded the producer size budget.";
        case NV_TOPOLOGY_ABORT_TIMED_OUT:
        case NV_TOPOLOGY_ABORT_NONE:
        default:
            return "Request timed out.";
    }
}

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

ENUM_STR_MAP_DEFINE(SOCKET_DIRECTION) = {
    { .id = SOCKET_DIRECTION_LISTEN, .name = "listen" },
    { .id = SOCKET_DIRECTION_LOCAL_INBOUND, .name = "inbound" },
    { .id = SOCKET_DIRECTION_LOCAL_OUTBOUND, .name = "outbound" },
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

static void nv_pid_append(uint32_t **pids, size_t *count, size_t *capacity, pid_t pid);

static inline const char *network_viewer_machine_guid(void) {
    const char *guid = getenv("NETDATA_REGISTRY_UNIQUE_ID");
    return (guid && *guid) ? guid : NULL;
}

static inline void topology_options_defaults(NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return;

    memset(opts, 0, sizeof(*opts));
    opts->detailed = false; // default: aggregated graph view
    opts->group_by = NV_TOPOLOGY_GROUP_BY_PROCESS_NAME;
    opts->sockets_listening = true;
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

static void topology_options_finalize(NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return;

    if(!topology_sockets_any_enabled(opts)) {
        opts->sockets_listening = true;
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

static void topology_apply_option_param(
    NV_TOPOLOGY_OPTIONS *opts,
    NV_TOPOLOGY_PARSE_STATE *state,
    const char *param)
{
    if(!opts || !state || !param || !*param)
        return;

    if(strcmp(param, "info") == 0) {
        opts->info_only = true;
        return;
    }

    if(strcmp(param, "aggregated") == 0 ||
       strcmp(param, "mode:aggregated") == 0 ||
       strcmp(param, "__topology_mode:aggregated") == 0 ||
       strcmp(param, "__topology_mode=aggregated") == 0 ||
       strcmp(param, "view:aggregated") == 0) {
        opts->detailed = false;
        return;
    }
    if(strcmp(param, "detailed") == 0 ||
       strcmp(param, "mode:detailed") == 0 ||
       strcmp(param, "__topology_mode:detailed") == 0 ||
       strcmp(param, "__topology_mode=detailed") == 0 ||
       strcmp(param, "view:detailed") == 0) {
        opts->detailed = true;
        return;
    }

    if(strcmp(param, "processes:by_name") == 0 || strcmp(param, "processes:by-name") == 0 ||
       strcmp(param, "group_by:process_name") == 0 || strcmp(param, "group-by:process-name") == 0 ||
       strcmp(param, "group_by=process_name") == 0 || strcmp(param, "group-by=process-name") == 0) {
        opts->group_by = NV_TOPOLOGY_GROUP_BY_PROCESS_NAME;
        return;
    }
    if(strcmp(param, "processes:by_pid") == 0 || strcmp(param, "processes:by-pid") == 0 ||
       strcmp(param, "group_by:pid") == 0 || strcmp(param, "group-by:pid") == 0 ||
       strcmp(param, "group_by=pid") == 0 || strcmp(param, "group-by=pid") == 0) {
        opts->group_by = NV_TOPOLOGY_GROUP_BY_PID;
        return;
    }
    if(strcmp(param, "group_by:container") == 0 || strcmp(param, "group-by:container") == 0 ||
       strcmp(param, "group_by=container") == 0 || strcmp(param, "group-by=container") == 0 ||
       strcmp(param, "group_by:container_name") == 0 || strcmp(param, "group-by:container-name") == 0 ||
       strcmp(param, "group_by=container_name") == 0 || strcmp(param, "group-by=container-name") == 0) {
        opts->group_by = NV_TOPOLOGY_GROUP_BY_CONTAINER;
        return;
    }

    if(strcmp(param, "endpoints:by_ip") == 0 || strcmp(param, "endpoints:by-ip") == 0)
        return;

    if(strncmp(param, "labels:", 7) == 0) {
        simple_pattern_free(opts->label_whitelist);
        opts->label_whitelist = nv_label_whitelist_parse(&param[7]);
        return;
    }

    if(strncmp(param, "sockets:", 8) == 0) {
        if(!state->sockets_selected_explicitly) {
            opts->sockets_listening = false;
            opts->sockets_inbound = false;
            opts->sockets_outbound = false;
            state->sockets_selected_explicitly = true;
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
            else if(strcmp(socket_kind, "inbound") == 0)
                opts->sockets_inbound = true;
            else if(strcmp(socket_kind, "outbound") == 0)
                opts->sockets_outbound = true;
        }
        freez(sockets_copy);
        return;
    }

    if(strncmp(param, "protocols:", 10) == 0) {
        if(!state->protocols_selected_explicitly) {
            opts->protocols_ipv4_tcp = false;
            opts->protocols_ipv6_tcp = false;
            opts->protocols_ipv4_udp = false;
            opts->protocols_ipv6_udp = false;
            state->protocols_selected_explicitly = true;
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

static void topology_parse_options(const char *function, NV_TOPOLOGY_OPTIONS *opts) {
    if(!opts)
        return;

    topology_options_defaults(opts);
    if(!function || !*function)
        return;

    char *function_copy = strdupz(function);
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    NV_TOPOLOGY_PARSE_STATE state = { 0 };
    for(size_t i = 1; i < num_words; i++) {
        char *param = get_word(words, num_words, i);
        topology_apply_option_param(opts, &state, param);
    }
    freez(function_copy);

    topology_options_finalize(opts);
}

static bool topology_payload_string_is_true(const char *value) {
    return value &&
           (strcmp(value, "1") == 0 ||
            strcasecmp(value, "true") == 0 ||
            strcasecmp(value, "yes") == 0 ||
            strcasecmp(value, "on") == 0);
}

static void topology_apply_payload_key_value(
    NV_TOPOLOGY_OPTIONS *opts,
    NV_TOPOLOGY_PARSE_STATE *state,
    const char *key,
    const char *value)
{
    if(!opts || !state || !key || !*key || !value || !*value)
        return;

    if(strcmp(key, "info") == 0) {
        if(topology_payload_string_is_true(value))
            opts->info_only = true;
        return;
    }

    char param[PLUGINSD_LINE_MAX + 1];
    if(strcmp(key, "group_by") == 0 || strcmp(key, "group-by") == 0 ||
       strcmp(key, "group_by[0]") == 0 || strcmp(key, "group-by[0]") == 0)
        snprintfz(param, sizeof(param) - 1, "group_by:%s", value);
    else if(strcmp(key, "__topology_mode") == 0 || strcmp(key, "mode") == 0)
        snprintfz(param, sizeof(param) - 1, "__topology_mode:%s", value);
    else if(strcmp(key, "sockets") == 0 || strcmp(key, "protocols") == 0 ||
            strcmp(key, "endpoints") == 0 || strcmp(key, "labels") == 0)
        snprintfz(param, sizeof(param) - 1, "%s:%s", key, value);
    else
        snprintfz(param, sizeof(param) - 1, "%s", value);

    topology_apply_option_param(opts, state, param);
}

static const char *topology_json_scalar_to_string(json_object *value, char *dst, size_t dst_size) {
    if(!value || !dst || !dst_size)
        return NULL;

    switch(json_object_get_type(value)) {
        case json_type_string:
            return json_object_get_string(value);
        case json_type_boolean:
            return json_object_get_boolean(value) ? "true" : "false";
        case json_type_int:
            snprintfz(dst, dst_size - 1, "%lld", (long long)json_object_get_int64(value));
            return dst;
        case json_type_double:
            snprintfz(dst, dst_size - 1, "%f", json_object_get_double(value));
            return dst;
        default:
            return NULL;
    }
}

static bool topology_apply_payload_member(
    NV_TOPOLOGY_OPTIONS *opts,
    NV_TOPOLOGY_PARSE_STATE *state,
    const char *key,
    json_object *value,
    BUFFER *error)
{
    if(!key || !value)
        return true;

    if(strcmp(key, "selections") == 0) {
        if(json_object_get_type(value) != json_type_object) {
            buffer_sprintf(error, "member 'selections' is not an object");
            return false;
        }

        json_object_object_foreach(value, selection_key, selection_value) {
            if(!topology_apply_payload_member(opts, state, selection_key, selection_value, error))
                return false;
        }
        return true;
    }

    if(json_object_get_type(value) == json_type_array) {
        size_t values = json_object_array_length(value);
        for(size_t i = 0; i < values; i++) {
            char scalar[DOUBLE_MAX_LENGTH];
            const char *selection = topology_json_scalar_to_string(json_object_array_get_idx(value, i), scalar, sizeof(scalar));
            if(!selection) {
                buffer_sprintf(error, "selection '%s' array item %zu is not a scalar", key, i);
                return false;
            }
            topology_apply_payload_key_value(opts, state, key, selection);
        }
        return true;
    }

    char scalar[DOUBLE_MAX_LENGTH];
    const char *selection = topology_json_scalar_to_string(value, scalar, sizeof(scalar));
    if(selection)
        topology_apply_payload_key_value(opts, state, key, selection);

    return true;
}

static bool topology_parse_payload_options(BUFFER *payload, NV_TOPOLOGY_OPTIONS *opts, BUFFER *error) {
    if(!payload || !buffer_strlen(payload) || !opts)
        return true;

    struct json_tokener *tokener = json_tokener_new();
    if(!tokener) {
        buffer_strcat(error, "failed to initialize json parser");
        return false;
    }

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse_ex(tokener, buffer_tostring(payload), (int)buffer_strlen(payload));
    enum json_tokener_error json_error = json_tokener_get_error(tokener);
    json_tokener_free(tokener);
    if(json_error != json_tokener_success) {
        buffer_sprintf(error, "invalid json payload: %s", json_tokener_error_desc(json_error));
        return false;
    }

    if(!jobj || json_object_get_type(jobj) != json_type_object) {
        buffer_strcat(error, "payload is not a json object");
        return false;
    }

    NV_TOPOLOGY_PARSE_STATE state = { 0 };
    json_object_object_foreach(jobj, key, value) {
        if(!topology_apply_payload_member(opts, &state, key, value, error))
            return false;
    }

    topology_options_finalize(opts);
    return true;
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

#if defined(OS_LINUX)
static bool topology_read_proc_ppid(uint64_t pid, uint64_t *ppid) {
    if(!pid || !ppid)
        return false;

    char filename[FILENAME_MAX + 1];
    char status_buf[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%llu/status",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "",
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

static bool topology_read_proc_starttime(uint64_t pid, uint64_t *starttime) {
    if(!pid || !starttime)
        return false;

    *starttime = 0;

    char filename[FILENAME_MAX + 1];
    char stat_buf[4096];
    snprintfz(filename, sizeof(filename), "%s/proc/%llu/stat",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "",
              (unsigned long long)pid);

    if(read_txt_file(filename, stat_buf, sizeof(stat_buf)))
        return false;

    char *fields = strrchr(stat_buf, ')');
    if(!fields)
        return false;

    fields++;
    for(uint32_t field = 3; field <= 22; field++) {
        while(isspace((unsigned char)*fields))
            fields++;

        if(!*fields)
            return false;

        if(field == 22) {
            char *end = NULL;
            uint64_t value = strtoull(fields, &end, 10);
            if(end == fields || value == 0)
                return false;

            *starttime = value;
            return true;
        }

        while(*fields && !isspace((unsigned char)*fields))
            fields++;
    }

    return false;
}
#elif defined(OS_FREEBSD)
#include <sys/types.h>
#include <sys/sysctl.h>
#include <sys/user.h>
#include <netinet/tcp_var.h>
#include <netinet/udp.h>
#include <netinet/udp_var.h>
static bool topology_read_proc_ppid(uint64_t pid, uint64_t *ppid) {
    if(!pid || !ppid)
        return false;

    struct kinfo_proc kp;
    size_t size = sizeof(kp);
    int mib[4] = { CTL_KERN, KERN_PROC, KERN_PROC_PID, (int)pid };

    if(sysctl(mib, 4, &kp, &size, NULL, 0) != 0 || size == 0)
        return false;

    uint64_t parent = (uint64_t)kp.ki_ppid;
    if(parent == pid)
        parent = 0;

    *ppid = parent;
    return true;
}

static bool topology_read_proc_starttime(uint64_t pid __maybe_unused, uint64_t *starttime) {
    if(starttime)
        *starttime = 0;

    return false;
}
#elif defined(OS_MACOS)
#include <libproc.h>
#include <netinet/tcp_var.h>
#include <netinet/udp_var.h>
#include <sys/proc_info.h>
#include <sys/sysctl.h>

static bool topology_read_proc_ppid(uint64_t pid, uint64_t *ppid) {
    if(!pid || !ppid || pid > (uint64_t)INT_MAX)
        return false;

    struct proc_bsdinfo bsd;
    int rc = proc_pidinfo((pid_t)pid, PROC_PIDTBSDINFO, 0, &bsd, sizeof(bsd));
    if(rc != (int)sizeof(bsd))
        return false;

    uint64_t parent = (uint64_t)bsd.pbi_ppid;
    if(parent == pid)
        parent = 0;

    *ppid = parent;
    return true;
}

static bool topology_read_proc_starttime(uint64_t pid, uint64_t *starttime) {
    if(!pid || !starttime || pid > (uint64_t)INT_MAX)
        return false;

    struct proc_bsdinfo bsd;
    int rc = proc_pidinfo((pid_t)pid, PROC_PIDTBSDINFO, 0, &bsd, sizeof(bsd));
    if(rc != (int)sizeof(bsd))
        return false;

    if(!bsd.pbi_start_tvsec && !bsd.pbi_start_tvusec)
        return false;

    *starttime = bsd.pbi_start_tvsec * USEC_PER_SEC + bsd.pbi_start_tvusec;
    return true;
}
#else
static bool topology_read_proc_ppid(uint64_t pid __maybe_unused, uint64_t *ppid) {
    if(ppid)
        *ppid = 0;

    return false;
}

static bool topology_read_proc_starttime(uint64_t pid __maybe_unused, uint64_t *starttime) {
    if(starttime)
        *starttime = 0;

    return false;
}
#endif

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

static uint64_t topology_starttime_cache_get_or_load(DICTIONARY *pid_starttime_cache, uint64_t pid) {
    if(!pid_starttime_cache || !pid)
        return 0;

    char pid_key[64];
    topology_pid_lookup_key(pid_key, sizeof(pid_key), pid);

    NV_PID_STARTTIME_CACHE_ENTRY *cached = dictionary_get(pid_starttime_cache, pid_key);
    if(cached)
        return cached->starttime;

    NV_PID_STARTTIME_CACHE_ENTRY tmp = { .starttime = 0 };
    uint64_t starttime = 0;
    if(topology_read_proc_starttime(pid, &starttime))
        tmp.starttime = starttime;

    cached = dictionary_set(pid_starttime_cache, pid_key, &tmp, sizeof(tmp));
    return cached ? cached->starttime : tmp.starttime;
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
        .starttime = pa->starttime,
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
    const char *container_name,
    char *dst,
    size_t dst_size
) {
    if(!dst || !dst_size)
        return;

    const char *node_identity = (ctx && ctx->machine_guid[0]) ? ctx->machine_guid : (ctx && ctx->hostname[0] ? ctx->hostname : "unknown");
    const char *safe_process = (process && *process) ? process : "[unknown]";
    if(ctx && ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER) {
        char encoded_container[(NV_TOPOLOGY_CONTAINER_NAME_MAX * 3) + 1];
        topology_encode_identifier_component(
            encoded_container, sizeof(encoded_container),
            (container_name && *container_name) ? container_name : safe_process);
        snprintf(dst, dst_size, "container:%s|name=%s", node_identity, encoded_container);
    }
    else if(ctx && ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PROCESS_NAME) {
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

static void topology_user_actor_name(uid_t uid, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    if(uid == UID_UNSET)
        return;

    CACHED_USERNAME cu = cached_username_get_by_uid(uid);
    const char *username = string2str(cu.username);
    if(username && *username && strcmp(username, "[unknown]") != 0)
        strncpyz(dst, username, dst_size - 1);
    cached_username_release(cu);

    if(!dst[0])
        snprintfz(dst, dst_size, "user%u", (unsigned)uid);
}

static bool topology_user_slice_actor_name(
    const char *cgroup_path,
    uid_t process_uid,
    char *dst,
    size_t dst_size)
{
    if(!dst || !dst_size)
        return false;

    dst[0] = '\0';

    uint32_t path_uid = NIPC_UID_UNSET;
    if(!cgroup_topology_parse_user_slice_uid(cgroup_path, &path_uid))
        return false;

    uid_t uid = process_uid != UID_UNSET ? process_uid : (uid_t)path_uid;
    topology_user_actor_name(uid, dst, dst_size);

    return dst[0] != '\0';
}

static bool topology_container_fields_apply_user_slice(
    const char *cgroup_path,
    uid_t process_uid,
    NV_TOPOLOGY_CONTAINER_FIELDS *fields)
{
    if(!fields)
        return false;

    char name[NV_TOPOLOGY_USERNAME_MAX];
    if(!topology_user_slice_actor_name(cgroup_path, process_uid, name, sizeof(name)))
        return false;

    strncpyz(fields->container_name, name, sizeof(fields->container_name) - 1);
    strncpyz(fields->orchestrator, "systemd", sizeof(fields->orchestrator) - 1);
    strncpyz(fields->actor_type, "user", sizeof(fields->actor_type) - 1);
    strncpyz(fields->actor_kind, "user", sizeof(fields->actor_kind) - 1);

    return true;
}

static void topology_container_fields_fill(
    uint32_t pid,
    uint64_t starttime,
    uid_t uid,
    const char *process,
    NV_TOPOLOGY_CONTAINER_FIELDS *fields)
{
    if(!fields)
        return;

    const char *fallback = (process && *process) ? process : "[unknown]";
    nv_container_fields_set_process_fallback(fields, fallback);

    if(!pid || !starttime)
        return;

    NV_APPS_LOOKUP_FIELDS cached;
    if(!nv_cache_lookup_pid(pid, starttime, &cached))
        return;

    fields->from_cache = true;
    strncpyz(fields->cgroup_status, nv_cgroup_status_name(cached.cgroup_status), sizeof(fields->cgroup_status) - 1);
    strncpyz(fields->cgroup_path, cached.cgroup_path ? cached.cgroup_path : "", sizeof(fields->cgroup_path) - 1);
    strncpyz(fields->cgroup_name, cached.cgroup_name ? cached.cgroup_name : "", sizeof(fields->cgroup_name) - 1);
    if(nv_cgroup_retry_later_without_path(cached.cgroup_status, cached.cgroup_path)) {
        nv_cache_lookup_fields_free(&cached);
        return;
    }

    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    cgroup_topology_classify(cached.cgroup_status, cached.orchestrator, cached.cgroup_path, &classification);
    strncpyz(fields->orchestrator, classification.effective_orchestrator, sizeof(fields->orchestrator) - 1);
    strncpyz(fields->systemd_unit_name, classification.systemd_unit_name, sizeof(fields->systemd_unit_name) - 1);
    strncpyz(fields->systemd_unit_kind, classification.systemd_unit_kind, sizeof(fields->systemd_unit_kind) - 1);
    strncpyz(fields->actor_kind, classification.actor_kind, sizeof(fields->actor_kind) - 1);
    strncpyz(fields->actor_type, classification.actor_type, sizeof(fields->actor_type) - 1);
    bool use_user_slice_name =
        !nv_cgroup_fields_have_container_identity(&cached) &&
        topology_container_fields_apply_user_slice(cached.cgroup_path, uid, fields);
    bool use_systemd_unit_name = !use_user_slice_name && strncmp(fields->actor_type, "systemd_", strlen("systemd_")) == 0;

    if(cached.cgroup_status != NIPC_APPS_CGROUP_KNOWN) {
        if(use_systemd_unit_name && fields->systemd_unit_name[0])
            strncpyz(fields->container_name, fields->systemd_unit_name, sizeof(fields->container_name) - 1);
        else if(cached.cgroup_status == NIPC_APPS_CGROUP_HOST_ROOT ||
           cached.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT)
            strncpyz(fields->container_name, fallback, sizeof(fields->container_name) - 1);

        nv_cache_lookup_fields_free(&cached);
        return;
    }

    nv_derive_k8s_pod_name(&cached, fields->k8s_pod_name, sizeof(fields->k8s_pod_name));
    nv_derive_k8s_namespace(&cached, fields->k8s_namespace, sizeof(fields->k8s_namespace));
    nv_derive_k8s_workload(&cached, fields->k8s_workload, sizeof(fields->k8s_workload));
    nv_derive_docker_container_name(
        &cached, fields->cgroup_name, fields->docker_container_name, sizeof(fields->docker_container_name));
    nv_derive_docker_image(&cached, fields->docker_image, sizeof(fields->docker_image));

    if(!use_user_slice_name) {
        if(use_systemd_unit_name && fields->systemd_unit_name[0])
            strncpyz(fields->container_name, fields->systemd_unit_name, sizeof(fields->container_name) - 1);
        else if(fields->docker_container_name[0])
            strncpyz(fields->container_name, fields->docker_container_name, sizeof(fields->container_name) - 1);
        else if(fields->cgroup_name[0])
            strncpyz(fields->container_name, fields->cgroup_name, sizeof(fields->container_name) - 1);
    }

    nv_cache_lookup_fields_free(&cached);
}

static void topology_container_fields_snapshot_from_cache(
    DICTIONARY *container_field_snapshot,
    uint32_t pid,
    uint64_t starttime,
    uid_t uid,
    const char *process,
    NV_TOPOLOGY_CONTAINER_FIELDS *fields)
{
    if(!fields)
        return;

    if(!container_field_snapshot || !pid || !starttime) {
        topology_container_fields_fill(pid, starttime, uid, process, fields);
        return;
    }

    char key[64];
    snprintfz(key, sizeof(key), "%u|%"PRIu64, pid, starttime);
    NV_TOPOLOGY_CONTAINER_FIELDS *cached = dictionary_get(container_field_snapshot, key);
    if(cached) {
        *fields = *cached;
        return;
    }

    NV_TOPOLOGY_CONTAINER_FIELDS tmp;
    topology_container_fields_fill(pid, starttime, uid, process, &tmp);
    dictionary_set(container_field_snapshot, key, &tmp, sizeof(tmp));
    *fields = tmp;
}

static void topology_container_fields_snapshot(
    const NV_TOPOLOGY_CONTEXT *ctx,
    uint32_t pid,
    uint64_t starttime,
    uid_t uid,
    const char *process,
    NV_TOPOLOGY_CONTAINER_FIELDS *fields)
{
    topology_container_fields_snapshot_from_cache(
        ctx ? ctx->container_field_snapshot : NULL, pid, starttime, uid, process, fields);
}

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
    if(ctx && ctx->options.group_by != NV_TOPOLOGY_GROUP_BY_PID)
        snprintf(dst, dst_size, "%s", safe_process);
    else
        snprintf(dst, dst_size, "%s[%llu]", safe_process, (unsigned long long)pid);
}

static const char *topology_group_by_id(NV_TOPOLOGY_GROUP_BY group_by)
{
    switch(group_by) {
        case NV_TOPOLOGY_GROUP_BY_PID:
            return "pid";
        case NV_TOPOLOGY_GROUP_BY_CONTAINER:
            return "container";
        case NV_TOPOLOGY_GROUP_BY_PROCESS_NAME:
        default:
            return "process_name";
    }
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
    }
    else {
        t = mallocz(sizeof(*t));
        memcpy(t, n, sizeof(*t));
        t->cmdline = string_dup(t->cmdline);
        simple_hashtable_set_slot_AGGREGATED_SOCKETS(ht, sl, hash, t);
    }
}

static bool topology_socket_direction_selected(const NV_TOPOLOGY_CONTEXT *ctx, SOCKET_DIRECTION direction) {
    if(!ctx)
        return false;

    switch(direction) {
        case SOCKET_DIRECTION_LISTEN:
            return ctx->options.sockets_listening;
        case SOCKET_DIRECTION_INBOUND:
        case SOCKET_DIRECTION_LOCAL_INBOUND:
            return ctx->options.sockets_inbound;
        case SOCKET_DIRECTION_OUTBOUND:
        case SOCKET_DIRECTION_LOCAL_OUTBOUND:
            return ctx->options.sockets_outbound;
        default:
            return false;
    }
}

static void local_sockets_cb_to_topology(LS_STATE *ls, const LOCAL_SOCKET *n, void *data) {
    NV_TOPOLOGY_CONTEXT *ctx = data;
    if(!ctx)
        return;

    if(topology_abort_iteration_checkpoint(ctx->sockets_total) &&
       topology_context_check_abort(ctx) != NV_TOPOLOGY_ABORT_NONE)
        return;

    if(n->direction == SOCKET_DIRECTION_NONE)
        return;

    ctx->sockets_total++;
    bool selected_socket = topology_socket_direction_selected(ctx, n->direction);
    bool hidden_listen_socket = (n->direction == SOCKET_DIRECTION_LISTEN && !ctx->options.sockets_listening);

    char local_ip[INET6_ADDRSTRLEN] = "";
    char remote_ip[INET6_ADDRSTRLEN] = "";

    if(is_local_socket_ipv46(n))
        strncpyz(local_ip, "*", sizeof(local_ip) - 1);
    else if(!socket_endpoint_to_ip_text(&n->local, local_ip))
        return;

    if(!local_sockets_is_zero_address(&n->remote)) {
        socket_endpoint_to_ip_text(&n->remote, remote_ip);
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
    uint64_t starttime = topology_starttime_cache_get_or_load(ctx->pid_starttime_cache, (uint64_t)n->pid);

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
            .starttime = starttime,
        };
        snprintf(hidden_owner.process, sizeof(hidden_owner.process), "%s", process_name);
        topology_register_endpoint_owner(ctx, n->net_ns_inode, n->local.protocol, local_ip, n->local.port, &hidden_owner, true);
        ctx->skipped_sockets++;
        return;
    }

    if(!selected_socket) {
        ctx->skipped_sockets++;
        return;
    }

    char process_key[NV_TOPOLOGY_KEY_MAX];
    snprintf(process_key, sizeof(process_key), "pid=%d|uid=%u|ns=%llu",
             n->pid,
             (unsigned)n->uid,
             (unsigned long long)n->net_ns_inode);

    NV_PROCESS_ACTOR *pa = dictionary_get(ctx->process_actors, process_key);
    if(!pa) {
        NV_PROCESS_ACTOR tmp = { 0 };
        tmp.pid = n->pid;
        tmp.ppid = n->ppid;
        tmp.uid = n->uid;
        tmp.net_ns_inode = n->net_ns_inode;
        tmp.starttime = starttime;
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
    if(!pa->starttime && starttime)
        pa->starttime = starttime;
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

    if(n->direction == SOCKET_DIRECTION_LISTEN) {
        ctx->skipped_sockets++;
        return;
    }

    if(!remote_ip[0] || topology_ip_is_unspecified(remote_ip)) {
        ctx->skipped_sockets++;
        return;
    }

    bool process_is_client = (n->direction == SOCKET_DIRECTION_OUTBOUND ||
                              n->direction == SOCKET_DIRECTION_LOCAL_OUTBOUND);

    char client_ip[INET6_ADDRSTRLEN] = "";
    char server_ip[INET6_ADDRSTRLEN] = "";
    const char *client_address_space = NULL;
    const char *server_address_space = NULL;
    uint16_t client_port = 0;
    uint16_t server_port = 0;

    if(process_is_client) {
        snprintf(client_ip, sizeof(client_ip), "%s", local_ip);
        snprintf(server_ip, sizeof(server_ip), "%s", remote_ip);
        client_address_space = local_address_space;
        server_address_space = remote_address_space;
        client_port = n->local.port;
        server_port = n->remote.port;
    }
    else {
        snprintf(client_ip, sizeof(client_ip), "%s", remote_ip);
        snprintf(server_ip, sizeof(server_ip), "%s", local_ip);
        client_address_space = remote_address_space;
        server_address_space = local_address_space;
        client_port = n->remote.port;
        server_port = n->local.port;
    }

    const char *endpoint_ip = process_is_client ? server_ip : client_ip;
    const char *endpoint_address_space = process_is_client ? server_address_space : client_address_space;
    bool endpoint_is_self = topology_ip_belongs_to_self(ctx, endpoint_ip, endpoint_address_space);
    if(!endpoint_is_self) {
        char endpoint_actor_key[NV_TOPOLOGY_KEY_MAX];
        topology_actor_id_for_remote_endpoint(ctx, endpoint_ip, endpoint_address_space, endpoint_actor_key, sizeof(endpoint_actor_key));
        NV_REMOTE_ACTOR *ra = dictionary_get(ctx->remote_actors, endpoint_actor_key);
        if(!ra) {
            NV_REMOTE_ACTOR tmp = { 0 };
            snprintf(tmp.ip, sizeof(tmp.ip), "%s", endpoint_ip);
            snprintf(tmp.address_space, sizeof(tmp.address_space), "%s", endpoint_address_space);
            ra = dictionary_set(ctx->remote_actors, endpoint_actor_key, &tmp, sizeof(tmp));
        }
        ra->sockets++;
    }

    char link_key[NV_TOPOLOGY_KEY_MAX];
    snprintf(link_key, sizeof(link_key), "pid=%d|uid=%u|ns=%llu|client=%s:%u|server=%s:%u|proto=%u|state=%u",
             n->pid,
             (unsigned)n->uid,
             (unsigned long long)n->net_ns_inode,
             client_ip,
             (unsigned)client_port,
             server_ip,
             (unsigned)server_port,
             (unsigned)n->local.protocol,
             (unsigned)n->state);

    NV_TOPOLOGY_LINK *link = dictionary_get(ctx->links, link_key);
    if(!link) {
        NV_TOPOLOGY_LINK tmp = { 0 };
        tmp.pid = n->pid;
        tmp.ppid = n->ppid;
        tmp.uid = n->uid;
        tmp.net_ns_inode = n->net_ns_inode;
        tmp.starttime = starttime;
        tmp.client_port = client_port;
        tmp.server_port = server_port;
        tmp.process_port = n->local.port;
        tmp.protocol_id = n->local.protocol;
        tmp.process_is_client = process_is_client;
        snprintf(tmp.process, sizeof(tmp.process), "%s", process_name);
        snprintf(tmp.username, sizeof(tmp.username), "%s", username);
        snprintf(tmp.namespace_type, sizeof(tmp.namespace_type), "%s", namespace_type);
        snprintf(tmp.protocol, sizeof(tmp.protocol), "%s", socket_protocol_name(n->local.protocol));
        snprintf(tmp.protocol_family, sizeof(tmp.protocol_family), "%s", socket_protocol_family_name(n));
        snprintf(tmp.state, sizeof(tmp.state), "%s",
                 n->local.protocol == IPPROTO_TCP ? TCP_STATE_2str(n->state) : "stateless");
        snprintf(tmp.client_ip, sizeof(tmp.client_ip), "%s", client_ip);
        snprintf(tmp.server_ip, sizeof(tmp.server_ip), "%s", server_ip);
        snprintf(tmp.client_address_space, sizeof(tmp.client_address_space), "%s", client_address_space);
        snprintf(tmp.server_address_space, sizeof(tmp.server_address_space), "%s", server_address_space);
        if(cmdline && *cmdline)
            snprintf(tmp.cmdline, sizeof(tmp.cmdline), "%s", cmdline);
        link = dictionary_set(ctx->links, link_key, &tmp, sizeof(tmp));
    }

    if(!link->starttime && starttime)
        link->starttime = starttime;
    link->sockets++;
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO) && defined(OS_LINUX)
    link->retransmissions += n->info.tcp.tcpi_total_retrans;
#endif
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
    if(link->max_rtt_usec < n->info.tcp.tcpi_rtt)
        link->max_rtt_usec = n->info.tcp.tcpi_rtt;
#if defined(OS_LINUX)
    if(link->max_rcv_rtt_usec < n->info.tcp.tcpi_rcv_rtt)
        link->max_rcv_rtt_usec = n->info.tcp.tcpi_rcv_rtt;
#endif
#endif
}

static void topology_context_destroy(NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx)
        return;

    if(ctx->links)
        dictionary_destroy(ctx->links);
    if(ctx->container_field_snapshot)
        dictionary_destroy(ctx->container_field_snapshot);
    if(ctx->pid_starttime_cache)
        dictionary_destroy(ctx->pid_starttime_cache);
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

static int nv_uint32_compar(const void *a, const void *b)
{
    uint32_t av = *(const uint32_t *)a;
    uint32_t bv = *(const uint32_t *)b;

    return (av > bv) - (av < bv);
}

static size_t nv_pid_sort_unique(uint32_t *pids, size_t count)
{
    if (!pids || count < 2)
        return count;

    qsort(pids, count, sizeof(*pids), nv_uint32_compar);

    size_t unique = 1;
    for (size_t i = 1; i < count; i++) {
        if (pids[i] != pids[unique - 1])
            pids[unique++] = pids[i];
    }

    return unique;
}

static void nv_pid_append(uint32_t **pids, size_t *count, size_t *capacity, pid_t pid)
{
    if (!pids || !count || !capacity || pid <= 0)
        return;

    if (*count == *capacity) {
        size_t new_capacity = *capacity ? *capacity * 2 : 64;
        *pids = reallocz(*pids, new_capacity * sizeof(**pids));
        *capacity = new_capacity;
    }

    (*pids)[(*count)++] = (uint32_t)pid;
}

static NV_TOPOLOGY_ABORT_STATUS nv_warm_cache_from_topology_actors(NV_TOPOLOGY_CONTEXT *ctx)
{
    if (!ctx || !ctx->process_actors)
        return NV_TOPOLOGY_ABORT_NONE;

    uint32_t *pids = NULL;
    size_t count = 0, capacity = 0;
    NV_PROCESS_ACTOR *pa;
    dfe_start_read(ctx->process_actors, pa) {
        if(topology_abort_iteration_checkpoint(count) &&
           topology_context_check_abort(ctx) != NV_TOPOLOGY_ABORT_NONE)
            break;

        nv_pid_append(&pids, &count, &capacity, pa->pid);
    }
    dfe_done(pa);

    if(topology_context_check_abort(ctx) != NV_TOPOLOGY_ABORT_NONE) {
        freez(pids);
        return ctx->abort_status;
    }

    count = nv_pid_sort_unique(pids, count);
    nv_apps_lookup_warm_pids(pids, count);
    freez(pids);
    return topology_context_check_abort(ctx);
}

static void nv_warm_cache_from_aggregated_sockets(LOCAL_SOCKET **sockets, size_t count)
{
    if (!sockets || count == 0)
        return;

    uint32_t *pids = NULL;
    size_t pid_count = 0, capacity = 0;
    for (size_t i = 0; i < count; i++)
        nv_pid_append(&pids, &pid_count, &capacity, sockets[i] ? sockets[i]->pid : 0);

    pid_count = nv_pid_sort_unique(pids, pid_count);
    nv_apps_lookup_warm_pids(pids, pid_count);
    freez(pids);
}

static bool topology_prepare_context(
    NV_TOPOLOGY_CONTEXT *ctx,
    usec_t now_ut,
    const NV_TOPOLOGY_OPTIONS *options,
    usec_t *stop_monotonic_ut,
    bool *cancelled) {
    if(!ctx)
        return false;

    memset(ctx, 0, sizeof(*ctx));
    ctx->now_ut = now_ut;
    ctx->stop_monotonic_ut = stop_monotonic_ut;
    ctx->cancelled = cancelled;
    if(options)
        ctx->options = *options;

    if(topology_context_check_abort(ctx) != NV_TOPOLOGY_ABORT_NONE)
        return false;

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
    ctx->container_field_snapshot = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_TOPOLOGY_CONTAINER_FIELDS));
    ctx->pid_starttime_cache = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(NV_PID_STARTTIME_CACHE_ENTRY));

    if(!(ctx->process_actors && ctx->remote_actors && ctx->local_ips &&
         ctx->endpoint_owners_exact && ctx->endpoint_owners_exact_any_ns &&
         ctx->endpoint_owners_service && ctx->endpoint_owners_service_any_ns &&
         ctx->links && ctx->container_field_snapshot && ctx->pid_starttime_cache))
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
            .local = ctx->options.sockets_inbound || ctx->options.sockets_outbound,
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
    return topology_context_check_abort(ctx) == NV_TOPOLOGY_ABORT_NONE;
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

static void network_viewer_finalize_response_buffer(BUFFER *wb, time_t now_s) {
    buffer_json_member_add_time_t(wb, "expires", now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_finalize(wb);

    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + NETWORK_VIEWER_RESPONSE_UPDATE_EVERY;
}

static BUFFER *network_viewer_json_error_response(int code, const char *message)
{
    BUFFER *wb = buffer_create(0, NULL);
    char escaped[PLUGINSD_LINE_MAX + 1];
    json_escape_string(escaped, message ? message : "", PLUGINSD_LINE_MAX);
    buffer_sprintf(wb, "{\"status\":%d,\"error_message\":\"%s\"}", code, escaped);
    wb->response_code = code;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_realtime_sec();
    return wb;
}

static void network_viewer_emit_response(const char *transaction, BUFFER *wb)
{
    if(!wb)
        return;

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);
}

static void topology_write_response_metadata(BUFFER *wb) {
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "topology");
    buffer_json_member_add_uint64(wb, "v", 3);
    buffer_json_member_add_time_t(wb, "update_every", NETWORK_VIEWER_RESPONSE_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", NETWORK_TOPOLOGY_VIEWER_HELP);
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "info");
        buffer_json_add_array_item_string(wb, "group_by");
        buffer_json_add_array_item_string(wb, "__topology_mode");
        buffer_json_add_array_item_string(wb, "mode");
        buffer_json_add_array_item_string(wb, "sockets");
        buffer_json_add_array_item_string(wb, "protocols");
        buffer_json_add_array_item_string(wb, "endpoints");
        buffer_json_add_array_item_string(wb, "labels");
    }
    buffer_json_array_close(wb);
    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "group_by");
            buffer_json_member_add_string(wb, "name", "Group By");
            buffer_json_member_add_string(wb, "help", "Choose the topology actor grouping level.");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "process_name");
                    buffer_json_member_add_string(wb, "name", "Process Name");
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "pid");
                    buffer_json_member_add_string(wb, "name", "PID");
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "container");
                    buffer_json_member_add_string(wb, "name", "Container Name");
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
                    buffer_json_member_add_boolean(wb, "defaultSelected", true);
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

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "labels");
            buffer_json_member_add_string(wb, "name", "Labels");
            buffer_json_member_add_string(wb, "help", "Pipe-separated label-key whitelist for optional actor labels. Omit to hide free-form labels.");
            buffer_json_member_add_string(wb, "type", "text");
        }
        buffer_json_object_close(wb);

    }
    buffer_json_array_close(wb);
}

typedef struct {
    char id[NV_TOPOLOGY_KEY_MAX];
    char type[NV_TOPOLOGY_ACTOR_TYPE_MAX];
    char machine_guid[128];
    char hostname[256];
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char local_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char ip[INET6_ADDRSTRLEN];
    char address_space[16];
    char cgroup_path[NV_TOPOLOGY_CGROUP_PATH_MAX];
    char cgroup_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char container_name[NV_TOPOLOGY_CONTAINER_NAME_MAX];
    char cgroup_status[32];
    char orchestrator[NV_TOPOLOGY_ORCHESTRATOR_MAX];
    char k8s_pod_name[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_namespace[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_workload[NV_TOPOLOGY_K8S_NAME_MAX];
    char docker_container_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char docker_image[NV_TOPOLOGY_DOCKER_IMAGE_MAX];
    char systemd_unit_name[NV_TOPOLOGY_SYSTEMD_UNIT_MAX];
    char systemd_unit_kind[NV_TOPOLOGY_SYSTEMD_KIND_MAX];
    char actor_kind[NV_TOPOLOGY_ACTOR_KIND_MAX];
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
    uint64_t socket_count;
    uint64_t retransmissions;
    uint32_t max_rtt_usec;
    uint32_t max_rcv_rtt_usec;
    char client_ip[INET6_ADDRSTRLEN];
    char server_ip[INET6_ADDRSTRLEN];
    char protocol[16];
    char state[32];
} NV_TOPOLOGY_V1_CONNECTION_ROW;

typedef struct {
    uint64_t actor;
    uint64_t port;
    uint64_t socket_count;
    char protocol[16];
} NV_TOPOLOGY_V1_PORT_ROW;

typedef struct {
    uint64_t actor;
    uint64_t pid;
    uint64_t ppid;
    uint64_t uid;
    uint64_t net_ns_inode;
    char process[TASK_COMM_LEN + 1];
    char username[NV_TOPOLOGY_USERNAME_MAX];
    char namespace_type[16];
    char local_ip[INET6_ADDRSTRLEN];
    char local_address_space[16];
    char cmdline[NV_TOPOLOGY_CMDLINE_MAX];
} NV_TOPOLOGY_V1_PROCESS_ROW;

typedef struct {
    uint64_t actor;
    uint64_t pid;
    char cgroup_status[32];
    char cgroup_path[NV_TOPOLOGY_CGROUP_PATH_MAX];
    char cgroup_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char container_name[NV_TOPOLOGY_CONTAINER_NAME_MAX];
    char orchestrator[NV_TOPOLOGY_ORCHESTRATOR_MAX];
    char actor_kind[NV_TOPOLOGY_ACTOR_KIND_MAX];
    char k8s_pod_name[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_namespace[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_workload[NV_TOPOLOGY_K8S_NAME_MAX];
    char docker_container_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char docker_image[NV_TOPOLOGY_DOCKER_IMAGE_MAX];
    char systemd_unit_name[NV_TOPOLOGY_SYSTEMD_UNIT_MAX];
    char systemd_unit_kind[NV_TOPOLOGY_SYSTEMD_KIND_MAX];
} NV_TOPOLOGY_V1_CGROUP_ROW;

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

    NV_TOPOLOGY_V1_PROCESS_ROW *processes;
    size_t processes_used;
    size_t processes_size;

    NV_TOPOLOGY_V1_CGROUP_ROW *cgroups;
    size_t cgroups_used;
    size_t cgroups_size;

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
    DICTIONARY *connection_index;
    DICTIONARY *port_index;
    DICTIONARY *process_index;
    DICTIONARY *cgroup_index;
    DICTIONARY *correlation_point_index;
    DICTIONARY *correlation_claim_index;
    DICTIONARY *label_index;
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
    if(!payload || !payload->connection_index || !source)
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%"PRIu64"|%s|%s",
              src_actor,
              dst_actor,
              source->protocol,
              source->state);

    uint64_t *stored = dictionary_get(payload->connection_index, key);
    if(stored) {
        NV_TOPOLOGY_V1_CONNECTION_ROW *row = &payload->connections[*stored];
        row->socket_count += source->sockets;
        row->retransmissions += source->retransmissions;
        if(row->max_rtt_usec < source->max_rtt_usec)
            row->max_rtt_usec = source->max_rtt_usec;
        if(row->max_rcv_rtt_usec < source->max_rcv_rtt_usec)
            row->max_rcv_rtt_usec = source->max_rcv_rtt_usec;
        return;
    }

    if(payload->connections_used == payload->connections_size) {
        size_t new_size = payload->connections_size ? payload->connections_size * 2 : 256;
        payload->connections = reallocz(payload->connections, new_size * sizeof(*payload->connections));
        payload->connections_size = new_size;
    }

    uint64_t index = payload->connections_used;
    NV_TOPOLOGY_V1_CONNECTION_ROW *row = &payload->connections[payload->connections_used++];
    *row = (NV_TOPOLOGY_V1_CONNECTION_ROW){ 0 };
    row->src_actor = src_actor;
    row->dst_actor = dst_actor;
    row->socket_count = source->sockets;
    row->retransmissions = source->retransmissions;
    row->max_rtt_usec = source->max_rtt_usec;
    row->max_rcv_rtt_usec = source->max_rcv_rtt_usec;
    topology_v1_strncpy(row->client_ip, sizeof(row->client_ip), source->client_ip);
    topology_v1_strncpy(row->server_ip, sizeof(row->server_ip), source->server_ip);
    topology_v1_strncpy(row->protocol, sizeof(row->protocol), source->protocol);
    topology_v1_strncpy(row->state, sizeof(row->state), source->state);
    dictionary_set(payload->connection_index, key, &index, sizeof(index));
}

static void topology_v1_add_port_row(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *protocol,
    uint64_t port,
    uint64_t socket_count) {
    if(!payload || !payload->port_index || !port || !socket_count)
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%s|%"PRIu64,
              actor,
              protocol ? protocol : "",
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

    dictionary_set(payload->port_index, key, &index, sizeof(index));
}

static void topology_v1_add_process_row(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const NV_PROCESS_ACTOR *source)
{
    if(!payload || !payload->process_index || !source || !source->pid)
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%"PRIu64"|%"PRIu64,
              actor, (uint64_t)source->pid, source->net_ns_inode);

    if(dictionary_get(payload->process_index, key))
        return;

    if(payload->processes_used == payload->processes_size) {
        size_t new_size = payload->processes_size ? payload->processes_size * 2 : 128;
        payload->processes = reallocz(payload->processes, new_size * sizeof(*payload->processes));
        payload->processes_size = new_size;
    }

    uint64_t index = payload->processes_used;
    NV_TOPOLOGY_V1_PROCESS_ROW *row = &payload->processes[payload->processes_used++];
    *row = (NV_TOPOLOGY_V1_PROCESS_ROW){ 0 };
    row->actor = actor;
    row->pid = source->pid;
    row->ppid = source->ppid;
    row->uid = source->uid;
    row->net_ns_inode = source->net_ns_inode;
    topology_v1_strncpy(row->process, sizeof(row->process), source->process);
    topology_v1_strncpy(row->username, sizeof(row->username), source->username);
    topology_v1_strncpy(row->namespace_type, sizeof(row->namespace_type), source->namespace_type);
    topology_v1_strncpy(row->local_ip, sizeof(row->local_ip), source->local_ip);
    topology_v1_strncpy(row->local_address_space, sizeof(row->local_address_space), source->local_address_space);
    topology_v1_strncpy(row->cmdline, sizeof(row->cmdline), source->cmdline);

    dictionary_set(payload->process_index, key, &index, sizeof(index));
}

static void topology_v1_add_cgroup_row(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    uint64_t pid,
    const NV_TOPOLOGY_CONTAINER_FIELDS *fields)
{
    if(!payload || !payload->cgroup_index || !fields || !pid)
        return;

    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%"PRIu64"|%s|%s|%s",
              actor, pid, fields->cgroup_status, fields->cgroup_path, fields->container_name);

    if(dictionary_get(payload->cgroup_index, key))
        return;

    if(payload->cgroups_used == payload->cgroups_size) {
        size_t new_size = payload->cgroups_size ? payload->cgroups_size * 2 : 128;
        payload->cgroups = reallocz(payload->cgroups, new_size * sizeof(*payload->cgroups));
        payload->cgroups_size = new_size;
    }

    uint64_t index = payload->cgroups_used;
    NV_TOPOLOGY_V1_CGROUP_ROW *row = &payload->cgroups[payload->cgroups_used++];
    *row = (NV_TOPOLOGY_V1_CGROUP_ROW){ 0 };
    row->actor = actor;
    row->pid = pid;
    topology_v1_strncpy(row->cgroup_status, sizeof(row->cgroup_status), fields->cgroup_status);
    topology_v1_strncpy(row->cgroup_path, sizeof(row->cgroup_path), fields->cgroup_path);
    topology_v1_strncpy(row->cgroup_name, sizeof(row->cgroup_name), fields->cgroup_name);
    topology_v1_strncpy(row->container_name, sizeof(row->container_name), fields->container_name);
    topology_v1_strncpy(row->orchestrator, sizeof(row->orchestrator), fields->orchestrator);
    topology_v1_strncpy(row->actor_kind, sizeof(row->actor_kind), fields->actor_kind);
    topology_v1_strncpy(row->k8s_pod_name, sizeof(row->k8s_pod_name), fields->k8s_pod_name);
    topology_v1_strncpy(row->k8s_namespace, sizeof(row->k8s_namespace), fields->k8s_namespace);
    topology_v1_strncpy(row->k8s_workload, sizeof(row->k8s_workload), fields->k8s_workload);
    topology_v1_strncpy(row->docker_container_name, sizeof(row->docker_container_name), fields->docker_container_name);
    topology_v1_strncpy(row->docker_image, sizeof(row->docker_image), fields->docker_image);
    topology_v1_strncpy(row->systemd_unit_name, sizeof(row->systemd_unit_name), fields->systemd_unit_name);
    topology_v1_strncpy(row->systemd_unit_kind, sizeof(row->systemd_unit_kind), fields->systemd_unit_kind);

    dictionary_set(payload->cgroup_index, key, &index, sizeof(index));
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

    const char *label_source = source ? source : NETWORK_TOPOLOGY_SOURCE;
    const char *label_kind = kind ? kind : "attribute";
    if(payload->label_index) {
        char index_key[NV_TOPOLOGY_KEY_MAX];
        snprintfz(index_key, sizeof(index_key), "%"PRIu64"|%zu:%s|%zu:%s|%zu:%s|%zu:%s",
                  actor,
                  strlen(key), key,
                  strlen(value), value,
                  strlen(label_source), label_source,
                  strlen(label_kind), label_kind);
        if(dictionary_get(payload->label_index, index_key))
            return;

        bool stored = true;
        dictionary_set(payload->label_index, index_key, &stored, sizeof(stored));
    }

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
    topology_v1_strncpy(row->source, sizeof(row->source), label_source);
    topology_v1_strncpy(row->kind, sizeof(row->kind), label_kind);
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

static void topology_v1_add_actor_label_if_set(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    const char *value,
    const char *kind)
{
    if(value && *value)
        topology_v1_add_actor_label(payload, actor, key, value, kind);
}

static void topology_v1_enrich_process_actor_from_cache(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor_index,
    NV_TOPOLOGY_V1_ACTOR *actor,
    uint32_t pid,
    uint64_t starttime,
    uid_t uid,
    bool fill_actor_fields)
{
    if(!ctx || !payload || !actor || !pid)
        return;

    NV_TOPOLOGY_CONTAINER_FIELDS fields = { 0 };
    topology_container_fields_snapshot(ctx, pid, starttime, uid, actor->process, &fields);

    bool have_cached = false;
    NV_APPS_LOOKUP_FIELDS cached;

    if(ctx->options.label_whitelist && nv_cache_lookup_pid(pid, starttime, &cached))
        have_cached = true;

    if(fill_actor_fields) {
        topology_v1_strncpy(actor->cgroup_status, sizeof(actor->cgroup_status), fields.cgroup_status);
        topology_v1_strncpy(actor->cgroup_path, sizeof(actor->cgroup_path), fields.cgroup_path);
        topology_v1_strncpy(actor->cgroup_name, sizeof(actor->cgroup_name), fields.cgroup_name);
        topology_v1_strncpy(actor->container_name, sizeof(actor->container_name), fields.container_name);
        topology_v1_strncpy(actor->orchestrator, sizeof(actor->orchestrator), fields.orchestrator);
        topology_v1_strncpy(actor->k8s_pod_name, sizeof(actor->k8s_pod_name), fields.k8s_pod_name);
        topology_v1_strncpy(actor->k8s_namespace, sizeof(actor->k8s_namespace), fields.k8s_namespace);
        topology_v1_strncpy(actor->k8s_workload, sizeof(actor->k8s_workload), fields.k8s_workload);
        topology_v1_strncpy(actor->docker_container_name, sizeof(actor->docker_container_name), fields.docker_container_name);
        topology_v1_strncpy(actor->docker_image, sizeof(actor->docker_image), fields.docker_image);
        topology_v1_strncpy(actor->systemd_unit_name, sizeof(actor->systemd_unit_name), fields.systemd_unit_name);
        topology_v1_strncpy(actor->systemd_unit_kind, sizeof(actor->systemd_unit_kind), fields.systemd_unit_kind);
        topology_v1_strncpy(actor->actor_kind, sizeof(actor->actor_kind), fields.actor_kind);
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
            topology_v1_strncpy(actor->type, sizeof(actor->type), fields.actor_type);
    }

    topology_v1_add_actor_label_if_set(payload, actor_index, "cgroup_status", fields.cgroup_status, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "cgroup_path", fields.cgroup_path, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "cgroup_name", fields.cgroup_name, "attribute");
    if(!(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER &&
         strcmp(actor->container_name, fields.container_name) == 0))
        topology_v1_add_actor_label_if_set(payload, actor_index, "container_name", fields.container_name, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "orchestrator", fields.orchestrator, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "k8s_pod_name", fields.k8s_pod_name, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "k8s_namespace", fields.k8s_namespace, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "k8s_workload", fields.k8s_workload, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "docker_container_name", fields.docker_container_name, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "docker_image", fields.docker_image, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "systemd_unit_name", fields.systemd_unit_name, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "systemd_unit_kind", fields.systemd_unit_kind, "attribute");
    topology_v1_add_actor_label_if_set(payload, actor_index, "actor_kind", fields.actor_kind, "attribute");

    if(have_cached && ctx->options.label_whitelist) {
        for(uint16_t i = 0; i < cached.cgroup_label_count; i++) {
            const char *key = cached.cgroup_labels[i].key;
            const char *value = cached.cgroup_labels[i].value;
            if(key && value && simple_pattern_matches(ctx->options.label_whitelist, key))
                topology_v1_add_actor_label_ex(payload, actor_index, key, value, "cgroups", "label", false, 0);
        }
    }

    if(have_cached)
        nv_cache_lookup_fields_free(&cached);
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

static void topology_v1_add_correlation_claim_endpoint(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *protocol,
    const char *address_space,
    const char *ip,
    uint64_t port) {
    if(!payload)
        return;

    topology_v1_add_correlation_row(
        &payload->correlation_claims,
        &payload->correlation_claims_used,
        &payload->correlation_claims_size,
        payload->correlation_claim_index,
        actor,
        protocol,
        address_space,
        ip,
        port);
}

static void topology_v1_add_correlation_point_endpoint(
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *protocol,
    const char *address_space,
    const char *ip,
    uint64_t port) {
    if(!payload)
        return;

    topology_v1_add_correlation_row(
        &payload->correlation_points,
        &payload->correlation_points_used,
        &payload->correlation_points_size,
        payload->correlation_point_index,
        actor,
        protocol,
        address_space,
        ip,
        port);
}

static void topology_v1_free(NV_TOPOLOGY_V1_PAYLOAD *payload) {
    if(!payload)
        return;

    freez(payload->actors);
    freez(payload->links);
    freez(payload->evidence);
    freez(payload->connections);
    freez(payload->ports);
    freez(payload->processes);
    freez(payload->cgroups);
    freez(payload->correlation_points);
    freez(payload->correlation_claims);
    freez(payload->labels);
    if(payload->actor_index)
        dictionary_destroy(payload->actor_index);
    if(payload->graph_link_index)
        dictionary_destroy(payload->graph_link_index);
    if(payload->connection_index)
        dictionary_destroy(payload->connection_index);
    if(payload->port_index)
        dictionary_destroy(payload->port_index);
    if(payload->process_index)
        dictionary_destroy(payload->process_index);
    if(payload->cgroup_index)
        dictionary_destroy(payload->cgroup_index);
    if(payload->correlation_point_index)
        dictionary_destroy(payload->correlation_point_index);
    if(payload->correlation_claim_index)
        dictionary_destroy(payload->correlation_claim_index);
    if(payload->label_index)
        dictionary_destroy(payload->label_index);
    *payload = (NV_TOPOLOGY_V1_PAYLOAD){ 0 };
}

static bool topology_v1_should_emit_endpoint_actor(
    const NV_TOPOLOGY_CONTEXT *ctx,
    const NV_REMOTE_ACTOR *actor) {
    if(!actor || !actor->ip[0])
        return false;

    return !topology_ip_belongs_to_self(ctx, actor->ip, actor->address_space);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_collect_actors(
    NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_RENDER_STATE *state,
    NV_TOPOLOGY_V1_PAYLOAD *payload) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_context_check_abort(ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

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
    size_t actor_iteration = 0;
    dfe_start_read(ctx->process_actors, pa) {
        if(topology_abort_iteration_checkpoint(actor_iteration++) &&
           (abort_status = topology_context_check_abort(ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;

        char actor_id[NV_TOPOLOGY_KEY_MAX];
        char display_name[NV_TOPOLOGY_KEY_MAX];
        NV_TOPOLOGY_CONTAINER_FIELDS container_fields = { 0 };
        topology_container_fields_snapshot(ctx, (uint32_t)pa->pid, pa->starttime, pa->uid, pa->process, &container_fields);

        topology_actor_id_for_process(
            ctx,
            pa->pid,
            pa->uid,
            pa->net_ns_inode,
            pa->process,
            ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER ? container_fields.container_name : NULL,
            actor_id,
            sizeof(actor_id));

        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
            topology_v1_strncpy(display_name, sizeof(display_name), container_fields.container_name);
        else
            topology_process_display_name(ctx, pa->process, pa->pid, display_name, sizeof(display_name));

        const char *pid_label_kind = ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID ? "identity" : "attribute";
        uint64_t actor_index = 0;
        if(topology_v1_actor_index_get(payload, actor_id, &actor_index)) {
            payload->actors[actor_index].sockets += pa->sockets;
            topology_v1_add_process_row(payload, actor_index, pa);
            topology_v1_add_cgroup_row(payload, actor_index, pa->pid, &container_fields);
            topology_v1_enrich_process_actor_from_cache(
                ctx, payload, actor_index, &payload->actors[actor_index], (uint32_t)pa->pid, pa->starttime, pa->uid, false);
            if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
                topology_v1_add_actor_label_if_set(payload, actor_index, "process", pa->process, "attribute");
            topology_v1_add_actor_label_if_set(payload, actor_index, "username", pa->username, "attribute");
            topology_v1_add_actor_label_if_set(payload, actor_index, "cmdline", pa->cmdline, "attribute");
            topology_v1_add_actor_label_if_set(payload, actor_index, "namespace_type", pa->namespace_type, "attribute");
            topology_v1_add_actor_label_if_set(payload, actor_index, "local_ip", pa->local_ip, "attribute");
            topology_v1_add_actor_label_if_set(payload, actor_index, "local_address_space", pa->local_address_space, "attribute");
            topology_v1_add_actor_label_uint(payload, actor_index, "pid", (uint64_t)pa->pid, pid_label_kind);
            topology_v1_add_actor_label_uint(payload, actor_index, "ppid", (uint64_t)pa->ppid, "attribute");
            topology_v1_add_actor_label_uint(payload, actor_index, "uid", (uint64_t)pa->uid, "attribute");
            topology_v1_add_actor_label_uint(payload, actor_index, "net_ns_inode", pa->net_ns_inode, pid_label_kind);
            continue;
        }

        NV_TOPOLOGY_V1_ACTOR *actor = topology_v1_add_actor(payload, actor_id);
        topology_v1_strncpy(
            actor->type, sizeof(actor->type),
            ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER ? container_fields.actor_type : "process");
        topology_v1_strncpy(actor->machine_guid, sizeof(actor->machine_guid), ctx->machine_guid);
        topology_v1_strncpy(actor->hostname, sizeof(actor->hostname), ctx->hostname);
        topology_v1_strncpy(actor->display_name, sizeof(actor->display_name), display_name);
        actor_index = payload->actors_used - 1;

        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID) {
            topology_v1_strncpy(actor->process, sizeof(actor->process), pa->process);
            topology_v1_strncpy(actor->username, sizeof(actor->username), pa->username);
            topology_v1_strncpy(actor->namespace_type, sizeof(actor->namespace_type), pa->namespace_type);
            topology_v1_strncpy(actor->local_ip, sizeof(actor->local_ip), pa->local_ip);
            topology_v1_strncpy(actor->local_address_space, sizeof(actor->local_address_space), pa->local_address_space);
            topology_v1_strncpy(actor->cmdline, sizeof(actor->cmdline), pa->cmdline);
            topology_v1_enrich_process_actor_from_cache(
                ctx, payload, actor_index, actor, (uint32_t)pa->pid, pa->starttime, pa->uid, true);
            actor->pid = (uint64_t)pa->pid;
            actor->ppid = (uint64_t)pa->ppid;
            actor->uid = (uint64_t)pa->uid;
            actor->net_ns_inode = pa->net_ns_inode;
            actor->has_pid = true;
            actor->has_ppid = true;
            actor->has_uid = true;
            actor->has_net_ns_inode = true;
        }
        else if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PROCESS_NAME) {
            topology_v1_strncpy(actor->process, sizeof(actor->process), pa->process);
        }
        else {
            topology_v1_strncpy(actor->container_name, sizeof(actor->container_name), container_fields.container_name);
            topology_v1_strncpy(actor->cgroup_status, sizeof(actor->cgroup_status), container_fields.cgroup_status);
            topology_v1_strncpy(actor->cgroup_path, sizeof(actor->cgroup_path), container_fields.cgroup_path);
            topology_v1_strncpy(actor->cgroup_name, sizeof(actor->cgroup_name), container_fields.cgroup_name);
            topology_v1_strncpy(actor->orchestrator, sizeof(actor->orchestrator), container_fields.orchestrator);
            topology_v1_strncpy(actor->k8s_pod_name, sizeof(actor->k8s_pod_name), container_fields.k8s_pod_name);
            topology_v1_strncpy(actor->k8s_namespace, sizeof(actor->k8s_namespace), container_fields.k8s_namespace);
            topology_v1_strncpy(actor->k8s_workload, sizeof(actor->k8s_workload), container_fields.k8s_workload);
            topology_v1_strncpy(actor->docker_container_name, sizeof(actor->docker_container_name), container_fields.docker_container_name);
            topology_v1_strncpy(actor->docker_image, sizeof(actor->docker_image), container_fields.docker_image);
            topology_v1_strncpy(actor->systemd_unit_name, sizeof(actor->systemd_unit_name), container_fields.systemd_unit_name);
            topology_v1_strncpy(actor->systemd_unit_kind, sizeof(actor->systemd_unit_kind), container_fields.systemd_unit_kind);
            topology_v1_strncpy(actor->actor_kind, sizeof(actor->actor_kind), container_fields.actor_kind);
        }

        actor->sockets = pa->sockets;
        topology_v1_add_process_row(payload, actor_index, pa);
        topology_v1_add_cgroup_row(payload, actor_index, pa->pid, &container_fields);

        topology_v1_add_actor_label(payload, actor_index, "display_name", actor->display_name, "attribute");
        topology_v1_add_actor_label(payload, actor_index, "type", actor->type, "metadata");
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
            topology_v1_add_actor_label(payload, actor_index, "container_name", actor->container_name, "identity");
        else
            topology_v1_add_actor_label(payload, actor_index, "process", actor->process, "identity");

        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
            topology_v1_add_actor_label_if_set(payload, actor_index, "process", pa->process, "attribute");
        topology_v1_add_actor_label_if_set(payload, actor_index, "username", pa->username, "attribute");
        topology_v1_add_actor_label_if_set(payload, actor_index, "cmdline", pa->cmdline, "attribute");
        topology_v1_add_actor_label_if_set(payload, actor_index, "namespace_type", pa->namespace_type, "attribute");
        topology_v1_add_actor_label_if_set(payload, actor_index, "local_ip", pa->local_ip, "attribute");
        topology_v1_add_actor_label_if_set(payload, actor_index, "local_address_space", pa->local_address_space, "attribute");
        topology_v1_add_actor_label_uint(payload, actor_index, "pid", (uint64_t)pa->pid, pid_label_kind);
        topology_v1_add_actor_label_uint(payload, actor_index, "ppid", (uint64_t)pa->ppid, "attribute");
        topology_v1_add_actor_label_uint(payload, actor_index, "uid", (uint64_t)pa->uid, "attribute");
        topology_v1_add_actor_label_uint(payload, actor_index, "net_ns_inode", pa->net_ns_inode, pid_label_kind);

        if(ctx->options.group_by != NV_TOPOLOGY_GROUP_BY_PID)
            topology_v1_enrich_process_actor_from_cache(
                ctx, payload, actor_index, actor, (uint32_t)pa->pid, pa->starttime, pa->uid, false);

        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID) {
            topology_v1_add_actor_label(payload, actor_index, "username", actor->username, "attribute");
            topology_v1_add_actor_label(payload, actor_index, "cmdline", actor->cmdline, "attribute");
            topology_v1_add_actor_label(payload, actor_index, "namespace_type", actor->namespace_type, "attribute");
            topology_v1_add_actor_label(payload, actor_index, "local_ip", actor->local_ip, "attribute");
            topology_v1_add_actor_label(payload, actor_index, "local_address_space", actor->local_address_space, "attribute");
        }
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID)
            topology_v1_add_actor_label_uint(payload, actor_index, "pid", actor->pid, "identity");
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID)
            topology_v1_add_actor_label_uint(payload, actor_index, "ppid", actor->ppid, "attribute");
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID)
            topology_v1_add_actor_label_uint(payload, actor_index, "uid", actor->uid, "attribute");
        if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_PID)
            topology_v1_add_actor_label_uint(payload, actor_index, "net_ns_inode", actor->net_ns_inode, "identity");
    }
    dfe_done(pa);

    for(uint64_t i = 0; i < payload->actors_used; i++) {
        if(topology_abort_iteration_checkpoint((size_t)i) &&
           (abort_status = topology_context_check_abort(ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;

        if(strcmp(payload->actors[i].type, "process") == 0 || strcmp(payload->actors[i].type, "container") == 0)
            topology_v1_add_actor_label_uint(payload, i, "socket_count", payload->actors[i].sockets, "metric");
    }

    NV_REMOTE_ACTOR *ra;
    size_t remote_iteration = 0;
    dfe_start_read(ctx->remote_actors, ra) {
        if(topology_abort_iteration_checkpoint(remote_iteration++) &&
           (abort_status = topology_context_check_abort(ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;

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

    return topology_context_check_abort(ctx);
}

static bool topology_v1_process_actor_index(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t pid,
    uint64_t starttime,
    uint64_t uid,
    uint64_t net_ns_inode,
    const char *process,
    uint64_t *index) {
    char actor_id[NV_TOPOLOGY_KEY_MAX];
    NV_TOPOLOGY_CONTAINER_FIELDS container_fields = { 0 };
    if(ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER)
        topology_container_fields_snapshot(ctx, (uint32_t)pid, starttime, (uid_t)uid, process, &container_fields);

    topology_actor_id_for_process(
        ctx,
        pid,
        uid,
        net_ns_inode,
        process,
        ctx->options.group_by == NV_TOPOLOGY_GROUP_BY_CONTAINER ? container_fields.container_name : NULL,
        actor_id,
        sizeof(actor_id));
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
    const char *state) {
    char key[NV_TOPOLOGY_KEY_MAX];
    snprintfz(key, sizeof(key), "%"PRIu64"|%"PRIu64"|%s|%s|%s",
              src_actor, dst_actor,
              type ? type : "",
              protocol ? protocol : "",
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
    topology_v1_strncpy(link->state, sizeof(link->state), state);
    dictionary_set(payload->graph_link_index, key, &index, sizeof(index));
    return index;
}

static bool topology_v1_socket_peer_actor_index(
    const NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    const NV_TOPOLOGY_LINK *link,
    NV_TOPOLOGY_ENDPOINT_ROLE role,
    uint64_t *actor,
    bool *is_correlation_point) {
    if(is_correlation_point)
        *is_correlation_point = false;

    const char *ip = NULL;
    const char *address_space = NULL;
    uint16_t port = 0;

    if(role == NV_TOPOLOGY_ENDPOINT_ROLE_CLIENT) {
        ip = link->client_ip;
        address_space = link->client_address_space;
        port = link->client_port;
    }
    else if(role == NV_TOPOLOGY_ENDPOINT_ROLE_SERVER) {
        ip = link->server_ip;
        address_space = link->server_address_space;
        port = link->server_port;
    }
    else
        return false;

    bool endpoint_is_self = topology_ip_belongs_to_self(ctx, ip, address_space);
    if(endpoint_is_self) {
        NV_ENDPOINT_OWNER *owner = topology_lookup_endpoint_owner(ctx, link->net_ns_inode, link->protocol_id, ip, port, true);
        if(owner) {
            return topology_v1_process_actor_index(ctx, payload,
                                                   owner->pid, owner->starttime, owner->uid, owner->net_ns_inode,
                                                   owner->process, actor);
        }

        return false;
    }

    if(is_correlation_point)
        *is_correlation_point = true;
    return topology_v1_endpoint_actor_index(ctx, payload, ip, address_space, actor);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_collect_links(
    NV_TOPOLOGY_CONTEXT *ctx,
    NV_TOPOLOGY_RENDER_STATE *state,
    NV_TOPOLOGY_V1_PAYLOAD *payload) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_context_check_abort(ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

    uint64_t host_actor = 0;
    topology_v1_actor_index_get(payload, state->host_actor_id, &host_actor);

    NV_PROCESS_ACTOR *pa;
    size_t process_iteration = 0;
    dfe_start_read(ctx->process_actors, pa) {
        if(topology_abort_iteration_checkpoint(process_iteration++) &&
           (abort_status = topology_context_check_abort(ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;

        uint64_t process_actor;
        if(!topology_v1_process_actor_index(
               ctx, payload, pa->pid, pa->starttime, pa->uid, pa->net_ns_inode, pa->process, &process_actor))
            continue;

        uint64_t link_index = topology_v1_graph_link_get_or_add(
            payload, host_actor, process_actor, "ownership", "ownership", "active");
        NV_TOPOLOGY_V1_GRAPH_LINK *link = &payload->links[link_index];
        link->socket_count += pa->sockets;
        state->ownership_link_count++;
    }
    dfe_done(pa);

    NV_TOPOLOGY_LINK *source;
    size_t link_iteration = 0;
    dfe_start_read(ctx->links, source) {
        if(topology_abort_iteration_checkpoint(link_iteration++) &&
           (abort_status = topology_context_check_abort(ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;

        uint64_t src_actor;
        if(!topology_v1_process_actor_index(ctx, payload,
                                            source->pid, source->starttime, source->uid, source->net_ns_inode,
                                            source->process, &src_actor))
            continue;

        uint64_t client_actor = 0;
        uint64_t server_actor = 0;
        bool client_is_correlation_point = false;
        bool server_is_correlation_point = false;

        if(source->process_is_client) {
            client_actor = src_actor;
            if(!topology_v1_socket_peer_actor_index(
                   ctx, payload, source, NV_TOPOLOGY_ENDPOINT_ROLE_SERVER, &server_actor, &server_is_correlation_point))
                continue;
        }
        else {
            server_actor = src_actor;
            if(!topology_v1_socket_peer_actor_index(
                   ctx, payload, source, NV_TOPOLOGY_ENDPOINT_ROLE_CLIENT, &client_actor, &client_is_correlation_point))
                continue;
        }

        if(!client_actor || !server_actor)
            continue;

        if(source->process_is_client) {
            topology_v1_add_correlation_claim_endpoint(
                payload, client_actor, source->protocol, source->client_address_space, source->client_ip, source->client_port);
            if(server_is_correlation_point)
                topology_v1_add_correlation_point_endpoint(
                    payload, server_actor, source->protocol, source->server_address_space, source->server_ip, source->server_port);
        }
        else {
            topology_v1_add_correlation_claim_endpoint(
                payload, server_actor, source->protocol, source->server_address_space, source->server_ip, source->server_port);
            if(client_is_correlation_point)
                topology_v1_add_correlation_point_endpoint(
                    payload, client_actor, source->protocol, source->client_address_space, source->client_ip, source->client_port);
        }

        topology_v1_add_port_row(
            payload, src_actor, source->protocol, source->process_port, source->sockets);
        if(!source->process_is_client && !client_is_correlation_point)
            topology_v1_add_port_row(
                payload, client_actor, source->protocol, source->client_port, source->sockets);
        else if(source->process_is_client && !server_is_correlation_point)
            topology_v1_add_port_row(
                payload, server_actor, source->protocol, source->server_port, source->sockets);

        uint64_t link_index = topology_v1_graph_link_get_or_add(
            payload, client_actor, server_actor,
            (client_is_correlation_point || server_is_correlation_point) ? "endpoint_socket" : "socket",
            source->protocol, source->state);
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
            row->src_actor = client_actor;
            row->dst_actor = server_actor;
            row->source = source;
        }
        else
            topology_v1_add_connection_row(payload, client_actor, server_actor, source);
    }
    dfe_done(source);

    return topology_context_check_abort(ctx);
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
    topology_v1_emit_column(wb, "machine_guid", "string", "merge_identity", true, "set");
    topology_v1_emit_column(wb, "hostname", "string", "merge_identity", true, "set");
    topology_v1_emit_column(wb, "process", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "username", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cmdline", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "pid", "uint", "identity", true, NULL);
    topology_v1_emit_column(wb, "ppid", "uint", "attribute", true, "set");
    topology_v1_emit_column(wb, "uid", "uint", "attribute", true, "set");
    topology_v1_emit_column(wb, "net_ns_inode", "uint", "identity", true, NULL);
    topology_v1_emit_column(wb, "namespace_type", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "local_ip", "ip", "attribute", true, "set");
    topology_v1_emit_column(wb, "local_address_space", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "ip", "ip", "identity", true, NULL);
    topology_v1_emit_column(wb, "address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "cgroup_status", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cgroup_path", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cgroup_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "container_name", "string", "group_key", true, "set");
    topology_v1_emit_column(wb, "orchestrator", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_pod_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_namespace", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_workload", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "docker_container_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "docker_image", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "systemd_unit_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "systemd_unit_kind", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "actor_kind", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "display_name", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "local_ip_count", "uint", "metric", true, "sum");
}

static void topology_v1_emit_link_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "evidence_count", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
#endif
}

static void topology_v1_emit_socket_evidence_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "link", "link_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "client_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "client_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "server_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "server_port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol_family", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "namespace_type", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "client_address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "server_address_space", "string", "group_key", true, NULL);
    topology_v1_emit_column(wb, "pid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "uid", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "net_ns_inode", "uint", "attribute", true, NULL);
    topology_v1_emit_column(wb, "process", "string", "attribute", true, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
#endif
}

static void topology_v1_emit_connection_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "client_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "server_ip", "ip", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "state", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
    topology_v1_emit_column(wb, "retransmissions", "uint", "metric", false, "sum");
    topology_v1_emit_column(wb, "rtt_ms_max", "float", "metric", false, "max");
    topology_v1_emit_column(wb, "recv_rtt_ms_max", "float", "metric", false, "max");
#endif
}

static void topology_v1_emit_socket_port_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "port", "uint", "group_key", false, NULL);
    topology_v1_emit_column(wb, "protocol", "string", "group_key", false, NULL);
    topology_v1_emit_column(wb, "socket_count", "uint", "metric", false, "sum");
}

static void topology_v1_emit_process_detail_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "pid", "uint", "identity", false, NULL);
    topology_v1_emit_column(wb, "ppid", "uint", "attribute", true, "set");
    topology_v1_emit_column(wb, "uid", "uint", "attribute", true, "set");
    topology_v1_emit_column(wb, "net_ns_inode", "uint", "identity", true, NULL);
    topology_v1_emit_column(wb, "process", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "username", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "namespace_type", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "local_ip", "ip", "attribute", true, "set");
    topology_v1_emit_column(wb, "local_address_space", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cmdline", "string", "attribute", true, "set");
}

static void topology_v1_emit_cgroup_detail_columns(BUFFER *wb) {
    topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    topology_v1_emit_column(wb, "pid", "uint", "identity", false, NULL);
    topology_v1_emit_column(wb, "cgroup_status", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cgroup_path", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "cgroup_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "container_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "orchestrator", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "actor_kind", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_pod_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_namespace", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "k8s_workload", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "docker_container_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "docker_image", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "systemd_unit_name", "string", "attribute", true, "set");
    topology_v1_emit_column(wb, "systemd_unit_kind", "string", "attribute", true, "set");
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

static void topology_v1_emit_modal_formatted_endpoint_column(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *ip_column,
    const char *port_column) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "formatted_endpoint");
            if(ip_column)
                buffer_json_member_add_string(wb, "ip_column", ip_column);
            if(port_column)
                buffer_json_member_add_string(wb, "port_column", port_column);
            buffer_json_member_add_string(wb, "protocol_column", "protocol");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", "endpoint");
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_actor_processes_modal_section(BUFFER *wb, uint64_t order) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", "processes");
        buffer_json_member_add_string(wb, "label", "Processes");
        buffer_json_member_add_uint64(wb, "order", order);
        buffer_json_member_add_object(wb, "source");
        {
            buffer_json_member_add_string(wb, "kind", "actor_table");
            buffer_json_member_add_string(wb, "table", "processes");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_object(wb, "owner_filter");
        {
            buffer_json_member_add_string(wb, "mode", "actor_column");
            buffer_json_member_add_string(wb, "actor_column", "actor");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_array(wb, "columns");
        {
            topology_v1_emit_modal_direct_column(wb, "pid", "PID", "pid", "number");
            topology_v1_emit_modal_direct_column(wb, "process", "Process", "process", "text");
            topology_v1_emit_modal_direct_column(wb, "user", "User", "username", "text");
            topology_v1_emit_modal_direct_column(wb, "namespace", "Namespace", "namespace_type", "badge");
            topology_v1_emit_modal_direct_column(wb, "local_ip", "Local IP", "local_ip", "text");
            topology_v1_emit_modal_direct_column_with_visibility(wb, "cmdline", "Command", "cmdline", "text", "expanded");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_actor_cgroups_modal_section(BUFFER *wb, uint64_t order) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", "cgroups");
        buffer_json_member_add_string(wb, "label", "CGroups");
        buffer_json_member_add_uint64(wb, "order", order);
        buffer_json_member_add_object(wb, "source");
        {
            buffer_json_member_add_string(wb, "kind", "actor_table");
            buffer_json_member_add_string(wb, "table", "cgroups");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_object(wb, "owner_filter");
        {
            buffer_json_member_add_string(wb, "mode", "actor_column");
            buffer_json_member_add_string(wb, "actor_column", "actor");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_array(wb, "columns");
        {
            topology_v1_emit_modal_direct_column(wb, "pid", "PID", "pid", "number");
            topology_v1_emit_modal_direct_column(wb, "status", "Status", "cgroup_status", "badge");
            topology_v1_emit_modal_direct_column(wb, "orchestrator", "Orchestrator", "orchestrator", "badge");
            topology_v1_emit_modal_direct_column(wb, "kind", "Kind", "actor_kind", "badge");
            topology_v1_emit_modal_direct_column(wb, "container", "Container", "container_name", "text");
            topology_v1_emit_modal_direct_column(wb, "systemd_unit", "Systemd Unit", "systemd_unit_name", "text");
            topology_v1_emit_modal_direct_column(wb, "systemd_kind", "Systemd Kind", "systemd_unit_kind", "badge");
            topology_v1_emit_modal_direct_column_with_visibility(wb, "cgroup_name", "Cgroup", "cgroup_name", "text", "expanded");
            topology_v1_emit_modal_direct_column_with_visibility(wb, "cgroup_path", "Cgroup Path", "cgroup_path", "text", "expanded");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_network_connection_modal_section(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *selected_actor_column,
    const char *peer_label,
    const char *endpoint_label,
    const char *endpoint_ip_column,
    const char *endpoint_port_column,
    bool detailed,
    uint64_t order) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_uint64(wb, "order", order);
        buffer_json_member_add_object(wb, "source");
        {
            buffer_json_member_add_string(wb, "kind", detailed ? "evidence" : "relationship_table");
            if(detailed)
                buffer_json_member_add_string(wb, "evidence", "socket");
            else
                buffer_json_member_add_string(wb, "table", "connections");
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_object(wb, "owner_filter");
        {
            buffer_json_member_add_string(wb, "mode", "actor_column");
            buffer_json_member_add_string(wb, "actor_column", selected_actor_column);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_array(wb, "columns");
        {
            topology_v1_emit_modal_opposite_actor_column_labeled(wb, "actor", peer_label);
            topology_v1_emit_modal_formatted_endpoint_column(
                wb, "endpoint", endpoint_label, endpoint_ip_column, detailed ? endpoint_port_column : NULL);
            topology_v1_emit_modal_direct_column(wb, "protocol", "Protocol", "protocol", "badge");
            topology_v1_emit_modal_direct_column(wb, "state", "State", "state", "badge");
            topology_v1_emit_modal_direct_column(wb, "sockets", "Sockets", "socket_count", "number");
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
            topology_v1_emit_modal_direct_column_with_visibility(
                wb, "retransmissions", "Retransmissions", "retransmissions", "number", "expanded");
            topology_v1_emit_modal_direct_column(wb, "rtt", "RTT max", "rtt_ms_max", "number");
            topology_v1_emit_modal_direct_column_with_visibility(
                wb, "recv_rtt", "Receiver RTT max", "recv_rtt_ms_max", "number", "expanded");
#endif
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void topology_v1_emit_modal_identification_field(BUFFER *wb, const char *key, const char *label) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "key", key);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_uint64(wb, "max_values", 5);
    }
    buffer_json_object_close(wb);
}

static bool topology_v1_actor_type_has_process_enrichment(const char *actor_type)
{
    return actor_type &&
           strcmp(actor_type, "self") != 0 &&
           strcmp(actor_type, "endpoint") != 0;
}

static bool topology_v1_actor_type_is_grouped_runtime(const char *actor_type)
{
    return actor_type && strcmp(actor_type, "process") != 0 && topology_v1_actor_type_has_process_enrichment(actor_type);
}

static void topology_v1_emit_modal_label_identification(
    BUFFER *wb,
    const char *actor_type,
    NV_TOPOLOGY_GROUP_BY group_by) {
    (void)group_by;

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
                topology_v1_emit_modal_identification_field(wb, "pid", "PID");
                topology_v1_emit_modal_identification_field(wb, "username", "User");
                topology_v1_emit_modal_identification_field(wb, "namespace_type", "Namespace");
                topology_v1_emit_modal_identification_field(wb, "local_ip", "Local IP");
                topology_v1_emit_modal_identification_field(wb, "container_name", "Container");
                topology_v1_emit_modal_identification_field(wb, "cgroup_status", "Cgroup Status");
                topology_v1_emit_modal_identification_field(wb, "orchestrator", "Orchestrator");
                topology_v1_emit_modal_identification_field(wb, "cgroup_name", "Cgroup");
                topology_v1_emit_modal_identification_field(wb, "cgroup_path", "Cgroup Path");
                topology_v1_emit_modal_identification_field(wb, "k8s_pod_name", "K8s Pod");
                topology_v1_emit_modal_identification_field(wb, "k8s_namespace", "K8s Namespace");
                topology_v1_emit_modal_identification_field(wb, "k8s_workload", "K8s Workload");
                topology_v1_emit_modal_identification_field(wb, "docker_container_name", "Docker Container");
                topology_v1_emit_modal_identification_field(wb, "docker_image", "Docker Image");
                topology_v1_emit_modal_identification_field(wb, "systemd_unit_name", "Systemd Unit");
                topology_v1_emit_modal_identification_field(wb, "systemd_unit_kind", "Systemd Kind");
                topology_v1_emit_modal_identification_field(wb, "actor_kind", "Kind");
                topology_v1_emit_modal_identification_field(wb, "cmdline", "Command");
                topology_v1_emit_modal_identification_field(wb, "socket_count", "Sockets");
            }
            else if(strcmp(actor_type, "user") == 0) {
                topology_v1_emit_modal_identification_field(wb, "container_name", "User");
                topology_v1_emit_modal_identification_field(wb, "process", "Process");
                topology_v1_emit_modal_identification_field(wb, "pid", "PID");
                topology_v1_emit_modal_identification_field(wb, "uid", "UID");
                topology_v1_emit_modal_identification_field(wb, "cgroup_status", "Cgroup Status");
                topology_v1_emit_modal_identification_field(wb, "systemd_unit_name", "Systemd Unit");
                topology_v1_emit_modal_identification_field(wb, "socket_count", "Sockets");
            }
            else if(topology_v1_actor_type_is_grouped_runtime(actor_type)) {
                topology_v1_emit_modal_identification_field(wb, "container_name", "Container");
                topology_v1_emit_modal_identification_field(wb, "process", "Process");
                topology_v1_emit_modal_identification_field(wb, "pid", "PID");
                topology_v1_emit_modal_identification_field(wb, "cgroup_status", "Cgroup Status");
                topology_v1_emit_modal_identification_field(wb, "orchestrator", "Orchestrator");
                topology_v1_emit_modal_identification_field(wb, "actor_kind", "Kind");
                topology_v1_emit_modal_identification_field(wb, "k8s_pod_name", "K8s Pod");
                topology_v1_emit_modal_identification_field(wb, "k8s_namespace", "K8s Namespace");
                topology_v1_emit_modal_identification_field(wb, "k8s_workload", "K8s Workload");
                topology_v1_emit_modal_identification_field(wb, "docker_container_name", "Docker Container");
                topology_v1_emit_modal_identification_field(wb, "docker_image", "Docker Image");
                topology_v1_emit_modal_identification_field(wb, "systemd_unit_name", "Systemd Unit");
                topology_v1_emit_modal_identification_field(wb, "systemd_unit_kind", "Systemd Kind");
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

static int topology_v1_string_ptr_cmp(const void *left, const void *right) {
    const char *left_string = *(const char * const *)left;
    const char *right_string = *(const char * const *)right;
    return strcmp(left_string ? left_string : "", right_string ? right_string : "");
}

static bool topology_v1_actor_search_label_key_is_builtin(const char *key) {
    static const char *const keys[] = {
        "pid",
        "ppid",
        "uid",
        "net_ns_inode",
        "username",
        "cmdline",
        "namespace_type",
        "local_ip",
        "local_address_space",
        "process",
        "container_name",
        "cgroup_status",
        "orchestrator",
        "cgroup_name",
        "cgroup_path",
        "k8s_pod_name",
        "k8s_namespace",
        "k8s_workload",
        "docker_container_name",
        "docker_image",
        "systemd_unit_name",
        "systemd_unit_kind",
        "actor_kind",
    };

    if(!key || !*key)
        return true;

    for(size_t i = 0; i < _countof(keys); i++) {
        if(strcmp(keys[i], key) == 0)
            return true;
    }

    return false;
}

static bool topology_v1_label_matches_actor_type(
    const NV_TOPOLOGY_V1_PAYLOAD *payload,
    const NV_TOPOLOGY_V1_ACTOR_LABEL *label,
    const char *actor_type) {
    if(!payload || !label || !actor_type || label->actor >= payload->actors_used)
        return false;

    return strcmp(payload->actors[label->actor].type, actor_type) == 0;
}

static bool topology_v1_search_label_key_exists(const char **keys, size_t keys_used, const char *key) {
    if(topology_v1_actor_search_label_key_is_builtin(key))
        return true;

    for(size_t i = 0; i < keys_used; i++) {
        if(strcmp(keys[i], key) == 0)
            return true;
    }

    return false;
}

static void topology_v1_emit_actor_search_label_keys(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_PAYLOAD *payload,
    const char *actor_type)
{
    const char **dynamic_keys = NULL;
    size_t dynamic_keys_used = 0;
    size_t dynamic_keys_size = 0;

    if(topology_v1_actor_type_has_process_enrichment(actor_type) && payload) {
        for(size_t i = 0; i < payload->labels_used; i++) {
            const NV_TOPOLOGY_V1_ACTOR_LABEL *label = &payload->labels[i];
            if(strcmp(label->source, "cgroups") != 0 || strcmp(label->kind, "label") != 0)
                continue;

            if(!topology_v1_label_matches_actor_type(payload, label, actor_type))
                continue;

            if(topology_v1_search_label_key_exists(dynamic_keys, dynamic_keys_used, label->key))
                continue;

            if(dynamic_keys_used == dynamic_keys_size) {
                dynamic_keys_size = dynamic_keys_size ? dynamic_keys_size * 2 : 16;
                dynamic_keys = reallocz(dynamic_keys, dynamic_keys_size * sizeof(*dynamic_keys));
            }
            dynamic_keys[dynamic_keys_used++] = label->key;
        }
    }

    if(dynamic_keys_used > 1)
        qsort(dynamic_keys, dynamic_keys_used, sizeof(*dynamic_keys), topology_v1_string_ptr_cmp);

    buffer_json_member_add_array(wb, "label_keys");
    if(topology_v1_actor_type_has_process_enrichment(actor_type)) {
        buffer_json_add_array_item_string(wb, "pid");
        buffer_json_add_array_item_string(wb, "ppid");
        buffer_json_add_array_item_string(wb, "uid");
        buffer_json_add_array_item_string(wb, "net_ns_inode");
        buffer_json_add_array_item_string(wb, "username");
        buffer_json_add_array_item_string(wb, "cmdline");
        buffer_json_add_array_item_string(wb, "namespace_type");
        buffer_json_add_array_item_string(wb, "local_ip");
        buffer_json_add_array_item_string(wb, "local_address_space");
        buffer_json_add_array_item_string(wb, "process");
        buffer_json_add_array_item_string(wb, "container_name");
        buffer_json_add_array_item_string(wb, "cgroup_status");
        buffer_json_add_array_item_string(wb, "orchestrator");
        buffer_json_add_array_item_string(wb, "cgroup_name");
        buffer_json_add_array_item_string(wb, "cgroup_path");
        buffer_json_add_array_item_string(wb, "k8s_pod_name");
        buffer_json_add_array_item_string(wb, "k8s_namespace");
        buffer_json_add_array_item_string(wb, "k8s_workload");
        buffer_json_add_array_item_string(wb, "docker_container_name");
        buffer_json_add_array_item_string(wb, "docker_image");
        buffer_json_add_array_item_string(wb, "systemd_unit_name");
        buffer_json_add_array_item_string(wb, "systemd_unit_kind");
        buffer_json_add_array_item_string(wb, "actor_kind");
    }
    for(size_t i = 0; i < dynamic_keys_used; i++)
        buffer_json_add_array_item_string(wb, dynamic_keys[i]);
    buffer_json_array_close(wb);

    freez(dynamic_keys);
}

static void topology_v1_emit_actor_type(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_PAYLOAD *payload,
    const char *id,
    const char *merge_a,
    const char *merge_b,
    const char *const *scopes,
    size_t scopes_count,
    const char *label,
    const char *color_slot,
    const char *icon,
    const char *role,
    bool border,
    const char *size_mode,
    const char *size_metric_column,
    const char *size_scale,
    const char *layout_repulsion,
    bool show_port_bullets,
    const char *port_table,
    NV_TOPOLOGY_GROUP_BY group_by,
    bool detailed,
    const char *label_column_a,
    const char *label_column_b) {
    bool is_self = strcmp(id, "self") == 0;

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
        for(size_t i = 0; i < scopes_count; i++)
            buffer_json_add_array_item_string(wb, scopes[i]);
        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "search");
        {
            buffer_json_member_add_array(wb, "columns");
            if(strcmp(id, "self") == 0) {
                buffer_json_add_array_item_string(wb, "display_name");
                buffer_json_add_array_item_string(wb, "hostname");
                buffer_json_add_array_item_string(wb, "machine_guid");
            }
            else if(strcmp(id, "process") == 0) {
                buffer_json_add_array_item_string(wb, "display_name");
                buffer_json_add_array_item_string(wb, "machine_guid");
                buffer_json_add_array_item_string(wb, "hostname");
                buffer_json_add_array_item_string(wb, "process");
                buffer_json_add_array_item_string(wb, "username");
                buffer_json_add_array_item_string(wb, "cmdline");
                buffer_json_add_array_item_string(wb, "local_ip");
                buffer_json_add_array_item_string(wb, "container_name");
                buffer_json_add_array_item_string(wb, "cgroup_status");
                buffer_json_add_array_item_string(wb, "orchestrator");
                buffer_json_add_array_item_string(wb, "cgroup_name");
                buffer_json_add_array_item_string(wb, "cgroup_path");
                buffer_json_add_array_item_string(wb, "k8s_pod_name");
                buffer_json_add_array_item_string(wb, "k8s_namespace");
                buffer_json_add_array_item_string(wb, "k8s_workload");
                buffer_json_add_array_item_string(wb, "docker_container_name");
                buffer_json_add_array_item_string(wb, "docker_image");
                buffer_json_add_array_item_string(wb, "systemd_unit_name");
                buffer_json_add_array_item_string(wb, "systemd_unit_kind");
                buffer_json_add_array_item_string(wb, "actor_kind");
            }
            else if(topology_v1_actor_type_is_grouped_runtime(id)) {
                buffer_json_add_array_item_string(wb, "display_name");
                buffer_json_add_array_item_string(wb, "machine_guid");
                buffer_json_add_array_item_string(wb, "hostname");
                buffer_json_add_array_item_string(wb, "container_name");
                buffer_json_add_array_item_string(wb, "process");
                buffer_json_add_array_item_string(wb, "cgroup_status");
                buffer_json_add_array_item_string(wb, "orchestrator");
                buffer_json_add_array_item_string(wb, "actor_kind");
                buffer_json_add_array_item_string(wb, "k8s_pod_name");
                buffer_json_add_array_item_string(wb, "k8s_namespace");
                buffer_json_add_array_item_string(wb, "k8s_workload");
                buffer_json_add_array_item_string(wb, "docker_container_name");
                buffer_json_add_array_item_string(wb, "docker_image");
                buffer_json_add_array_item_string(wb, "systemd_unit_name");
                buffer_json_add_array_item_string(wb, "systemd_unit_kind");
            }
            else if(strcmp(id, "endpoint") == 0) {
                buffer_json_add_array_item_string(wb, "display_name");
                buffer_json_add_array_item_string(wb, "ip");
                buffer_json_add_array_item_string(wb, "address_space");
            }
            buffer_json_array_close(wb);
            topology_v1_emit_actor_search_label_keys(wb, payload, id);
        }
        buffer_json_object_close(wb);
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
                if(size_scale)
                    buffer_json_member_add_string(wb, "scale", size_scale);
            }
            buffer_json_object_close(wb);
            if(layout_repulsion) {
                buffer_json_member_add_object(wb, "layout");
                {
                    buffer_json_member_add_string(wb, "repulsion", layout_repulsion);
                }
                buffer_json_object_close(wb);
            }
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
                    topology_v1_emit_modal_label_identification(wb, id, group_by);
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
                    if(is_self) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "id", "actors");
                            buffer_json_member_add_string(wb, "label", "Actors");
                            buffer_json_member_add_uint64(wb, "order", 1);
                            buffer_json_member_add_object(wb, "source");
                            {
                                buffer_json_member_add_string(wb, "kind", "links");
                            }
                            buffer_json_object_close(wb);
                            buffer_json_member_add_object(wb, "owner_filter");
                            {
                                buffer_json_member_add_string(wb, "mode", "incident_link");
                                buffer_json_member_add_string(wb, "src_actor_column", "src_actor");
                                buffer_json_member_add_string(wb, "dst_actor_column", "dst_actor");
                            }
                            buffer_json_object_close(wb);
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
                            buffer_json_member_add_array(wb, "columns");
                            {
                                topology_v1_emit_modal_opposite_actor_column_labeled(wb, "actor", "Actor");
                                topology_v1_emit_modal_direct_column(wb, "sockets", "Sockets", "socket_count", "number");
                                topology_v1_emit_modal_direct_column_with_visibility(
                                    wb, "evidence", "Evidence", "evidence_count", "number", "expanded");
                            }
                            buffer_json_array_close(wb);
                        }
                        buffer_json_object_close(wb);
                    }
                    else {
                        bool is_endpoint = strcmp(id, "endpoint") == 0;
                        uint64_t dependency_order = 1;
                        if(!is_endpoint) {
                            topology_v1_emit_actor_processes_modal_section(wb, 1);
                            topology_v1_emit_actor_cgroups_modal_section(wb, 2);
                            dependency_order = 3;
                        }
                        topology_v1_emit_network_connection_modal_section(
                            wb,
                            detailed ? "dependency_sockets" : "dependencies",
                            "Dependencies",
                            "src_actor",
                            "Service",
                            "Server",
                            "server_ip",
                            "server_port",
                            detailed,
                            dependency_order);
                        topology_v1_emit_network_connection_modal_section(
                            wb,
                            detailed ? "dependant_sockets" : "dependants",
                            "Dependants",
                            "dst_actor",
                            "Client",
                            "Client",
                            "client_ip",
                            "client_port",
                            detailed,
                            dependency_order + 1);
                    }
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);

        }
        buffer_json_object_close(wb);

    }
    buffer_json_object_close(wb);
}

typedef struct {
    const char *id;
    const char *label;
    const char *icon;
} NV_TOPOLOGY_RUNTIME_ACTOR_TYPE;

static const NV_TOPOLOGY_RUNTIME_ACTOR_TYPE topology_runtime_actor_types[] = {
    { "process_group", "Process", "process" },
    { "container", "Container", "container" },
    { "user", "User", "user" },
    { "docker_container", "Docker container", "docker" },
    { "k8s_container", "Kubernetes container", "kubernetes" },
    { "podman_container", "Podman container", "podman" },
    { "lxc_container", "LXC container", "lxc" },
    { "nspawn_container", "systemd-nspawn container", "nspawn" },
    { "vm", "Virtual machine", "vm" },
    { "systemd_service", "Systemd service", "systemd" },
    { "systemd_scope", "Systemd scope", "systemd" },
    { "systemd_slice", "Systemd slice", "systemd" },
    { "systemd_socket", "Systemd socket", "systemd" },
    { "systemd_target", "Systemd target", "systemd" },
    { "systemd_timer", "Systemd timer", "systemd" },
    { "systemd_mount", "Systemd mount", "storage" },
    { "systemd_path", "Systemd path", "systemd" },
    { "systemd_swap", "Systemd swap", "storage" },
    { "systemd_device", "Systemd device", "device" },
    { "systemd_unit", "Systemd unit", "systemd" },
};

static void topology_v1_emit_runtime_actor_type(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_PAYLOAD *payload,
    const NV_TOPOLOGY_RUNTIME_ACTOR_TYPE *type,
    const char *const *container_scopes,
    size_t container_scopes_count,
    NV_TOPOLOGY_GROUP_BY group_by,
    bool detailed)
{
    topology_v1_emit_actor_type(
        wb, payload, type->id, "container_name", NULL, container_scopes, container_scopes_count,
        type->label, "primary", type->icon, "actor", true, "metric", "socket_count", "normal", "normal", true,
        "socket_ports", group_by, detailed, "display_name", "container_name");
}

static void topology_v1_emit_link_type(
    BUFFER *wb,
    const char *id,
    const char *orientation,
    const char *direction_role,
    const char *semantic_role,
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
        if(semantic_role)
            buffer_json_member_add_string(wb, "semantic_role", semantic_role);
        buffer_json_member_add_object(wb, "aggregation");
        {
            buffer_json_member_add_string(wb, "direction", "preserve");
            buffer_json_member_add_string(wb, "evidence", evidence_type ? "append" : "count");
            buffer_json_member_add_object(wb, "metrics");
            {
                buffer_json_member_add_string(wb, "evidence_count", "sum");
                buffer_json_member_add_string(wb, "socket_count", "sum");
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
                if(evidence_type) {
                    buffer_json_member_add_string(wb, "retransmissions", "sum");
                    buffer_json_member_add_string(wb, "rtt_ms_max", "max");
                    buffer_json_member_add_string(wb, "recv_rtt_ms_max", "max");
                }
#endif
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

static void topology_v1_emit_type_registry(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_GROUP_BY group_by,
    bool detailed __maybe_unused) {
    static const char *const self_scopes[] = { "node" };
    static const char *const process_scopes[] = { "process_name", "pid" };
    static const char *const container_scopes[] = { "container" };
    static const char *const endpoint_scopes[] = { "endpoint" };
    const char *process_merge_a = "process";
    const char *process_merge_b = NULL;

    if(group_by == NV_TOPOLOGY_GROUP_BY_PID) {
        process_merge_a = "machine_guid";
        process_merge_b = "pid";
    }

    buffer_json_member_add_object(wb, "types");
    {
        buffer_json_member_add_object(wb, "actor_types");
        {
            topology_v1_emit_actor_type(
                wb, payload, "self", "machine_guid", "hostname", self_scopes, _countof(self_scopes),
                "This host", "self", "self", "actor", true, "fixed", NULL, "emphasized", "strongest", false, NULL,
                group_by,
                detailed,
                "display_name", "hostname");
            topology_v1_emit_actor_type(
                wb, payload, "process", process_merge_a, process_merge_b, process_scopes, _countof(process_scopes),
                "Process", "primary", "process", "actor", true, "metric", "socket_count", "normal", "normal", true, "socket_ports",
                group_by,
                detailed,
                "display_name", "process");
            for(size_t i = 0; i < _countof(topology_runtime_actor_types); i++)
                topology_v1_emit_runtime_actor_type(
                    wb, payload, &topology_runtime_actor_types[i], container_scopes, _countof(container_scopes), group_by, detailed);
            topology_v1_emit_actor_type(
                wb, payload, "endpoint", "ip", "address_space", endpoint_scopes, _countof(endpoint_scopes),
                "Correlation endpoint", "derived", "remote-endpoint", "endpoint", true, "fixed", NULL, "compact", "weaker", false, NULL,
                group_by,
                detailed,
                "display_name", "ip");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "link_types");
        {
            topology_v1_emit_link_type(
                wb, "socket", "directed", "dependency", "traffic", "socket",
                "Local socket", "gray", "solid", "thin", "forward", NULL,
                "sockets", "socket_count", "normal", "normal");
            topology_v1_emit_link_type(
                wb, "endpoint_socket", "directed", "dependency", "traffic", "socket",
                "Endpoint connection", "primary", "solid", "thin", "forward", NULL,
                NULL, NULL, "normal", "normal");
            topology_v1_emit_link_type(
                wb, "correlated_socket", "directed", "dependency", "traffic", "socket",
                "Correlated socket", "primary", "solid", "thin", "forward", NULL,
                "sockets", "socket_count", "normal", "farthest");
            topology_v1_emit_link_type(
                wb, "ownership", "hierarchical", "ownership", "ownership", NULL,
                "Actor ownership", "dim", "dotted", "thin", "none", "faded",
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
                buffer_json_add_array_item_string(wb, "client_ip");
                buffer_json_add_array_item_string(wb, "client_port");
                buffer_json_add_array_item_string(wb, "server_ip");
                buffer_json_add_array_item_string(wb, "server_port");
                buffer_json_add_array_item_string(wb, "protocol");
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

            buffer_json_member_add_object(wb, "processes");
            {
                buffer_json_member_add_string(wb, "role", "actor_detail");
                buffer_json_member_add_string(wb, "owner", "actor");
                buffer_json_member_add_string(wb, "aggregation", "set");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_process_detail_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Processes");
                    buffer_json_member_add_uint64(wb, "order", 1);
                    buffer_json_member_add_array(wb, "columns");
                    {
                        topology_v1_emit_modal_direct_column(wb, "pid", "PID", "pid", "number");
                        topology_v1_emit_modal_direct_column(wb, "process", "Process", "process", "text");
                        topology_v1_emit_modal_direct_column(wb, "user", "User", "username", "text");
                        topology_v1_emit_modal_direct_column_with_visibility(wb, "cmdline", "Command", "cmdline", "text", "expanded");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "cgroups");
            {
                buffer_json_member_add_string(wb, "role", "actor_detail");
                buffer_json_member_add_string(wb, "owner", "actor");
                buffer_json_member_add_string(wb, "aggregation", "set");
                buffer_json_member_add_array(wb, "columns");
                topology_v1_emit_cgroup_detail_columns(wb);
                buffer_json_array_close(wb);
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "CGroups");
                    buffer_json_member_add_uint64(wb, "order", 2);
                    buffer_json_member_add_array(wb, "columns");
                    {
                        topology_v1_emit_modal_direct_column(wb, "pid", "PID", "pid", "number");
                        topology_v1_emit_modal_direct_column(wb, "status", "Status", "cgroup_status", "badge");
                        topology_v1_emit_modal_direct_column(wb, "orchestrator", "Orchestrator", "orchestrator", "badge");
                        topology_v1_emit_modal_direct_column(wb, "kind", "Kind", "actor_kind", "badge");
                        topology_v1_emit_modal_direct_column(wb, "container", "Container", "container_name", "text");
                        topology_v1_emit_modal_direct_column_with_visibility(wb, "cgroup_path", "Cgroup Path", "cgroup_path", "text", "expanded");
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
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

            buffer_json_member_add_object(wb, "container");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "container_name");
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

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_actor_table(
    BUFFER *wb,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

    buffer_json_member_add_object(wb, "actors");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->actors_used);
        buffer_json_member_add_array(wb, "columns");
        topology_v1_emit_actor_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) { \
                if(topology_abort_iteration_checkpoint(i) && \
                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                    return abort_status; \
                buffer_json_add_array_item_string(wb, payload->actors[i].member[0] ? payload->actors[i].member : NULL); \
            } \
            topology_v1_values_end(wb); \
        } while(0)
#define NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(member, has_member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) { \
                if(topology_abort_iteration_checkpoint(i) && \
                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                    return abort_status; \
                topology_v1_add_nullable_uint(wb, payload->actors[i].has_member, payload->actors[i].member); \
            } \
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
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(cgroup_status);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(cgroup_path);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(cgroup_name);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(container_name);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(orchestrator);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(k8s_pod_name);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(k8s_namespace);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(k8s_workload);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(docker_container_name);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(docker_image);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(systemd_unit_name);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(systemd_unit_kind);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(actor_kind);
        NV_TOPOLOGY_V1_ACTOR_STRING_VALUES(display_name);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->actors_used; i++) {
            if(topology_abort_iteration_checkpoint(i) &&
               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                return abort_status;
            buffer_json_add_array_item_uint64(wb, payload->actors[i].sockets);
        }
        topology_v1_values_end(wb);

        NV_TOPOLOGY_V1_ACTOR_UINT_VALUES(local_ip_count, has_local_ip_count);

#undef NV_TOPOLOGY_V1_ACTOR_STRING_VALUES
#undef NV_TOPOLOGY_V1_ACTOR_UINT_VALUES

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_link_table(
    BUFFER *wb,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

    buffer_json_member_add_object(wb, "links");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->links_used);
        buffer_json_member_add_array(wb, "columns");
        topology_v1_emit_link_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_LINK_UINT_VALUES(member) do { \
            topology_v1_values_start(wb); \
            for(size_t i = 0; i < payload->links_used; i++) { \
                if(topology_abort_iteration_checkpoint(i) && \
                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                    return abort_status; \
                buffer_json_add_array_item_uint64(wb, payload->links[i].member); \
            } \
            topology_v1_values_end(wb); \
        } while(0)
#define NV_TOPOLOGY_V1_LINK_STRING_VALUES(member) do { \
            NV_TOPOLOGY_V1_STRING_COLUMN column; \
            topology_v1_string_column_init(&column, payload->links_used); \
            for(size_t i = 0; i < payload->links_used; i++) { \
                if(topology_abort_iteration_checkpoint(i) && \
                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                    break; \
                topology_v1_string_column_add(&column, payload->links[i].member); \
            } \
            if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                topology_v1_emit_auto_string_column(wb, &column); \
            topology_v1_string_column_free(&column); \
            if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                return abort_status; \
        } while(0)

        NV_TOPOLOGY_V1_LINK_UINT_VALUES(src_actor);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(dst_actor);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(type);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(protocol);
        NV_TOPOLOGY_V1_LINK_STRING_VALUES(state);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(evidence_count);
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(socket_count);
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
        NV_TOPOLOGY_V1_LINK_UINT_VALUES(retransmissions);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++) {
            if(topology_abort_iteration_checkpoint(i) &&
               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                return abort_status;
            buffer_json_add_array_item_double(wb, (double)payload->links[i].max_rtt_usec / (double)USEC_PER_MS);
        }
        topology_v1_values_end(wb);

        topology_v1_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++) {
            if(topology_abort_iteration_checkpoint(i) &&
               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                return abort_status;
            buffer_json_add_array_item_double(wb, (double)payload->links[i].max_rcv_rtt_usec / (double)USEC_PER_MS);
        }
        topology_v1_values_end(wb);
#endif

#undef NV_TOPOLOGY_V1_LINK_UINT_VALUES
#undef NV_TOPOLOGY_V1_LINK_STRING_VALUES

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_socket_port_table(
    BUFFER *wb,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

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
                    for(size_t i = 0; i < payload->ports_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            return abort_status; \
                        buffer_json_add_array_item_uint64(wb, payload->ports[i].member); \
                    } \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_PORT_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->ports_used); \
                    for(size_t i = 0; i < payload->ports_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            break; \
                        topology_v1_string_column_add(&column, payload->ports[i].member); \
                    } \
                    if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                        topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                    if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                } while(0)

                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(port);
                    NV_TOPOLOGY_V1_PORT_STRING_VALUES(protocol);
                    NV_TOPOLOGY_V1_PORT_UINT_VALUES(socket_count);

#undef NV_TOPOLOGY_V1_PORT_UINT_VALUES
#undef NV_TOPOLOGY_V1_PORT_STRING_VALUES

                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "processes");
            {
                buffer_json_member_add_string(wb, "type", "processes");
                buffer_json_member_add_object(wb, "table");
                {
                    buffer_json_member_add_uint64(wb, "rows", payload->processes_used);
                    buffer_json_member_add_array(wb, "columns");
                    topology_v1_emit_process_detail_columns(wb);
                    buffer_json_array_close(wb);
                    buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(member) do { \
                    topology_v1_values_start(wb); \
                    for(size_t i = 0; i < payload->processes_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            return abort_status; \
                        buffer_json_add_array_item_uint64(wb, payload->processes[i].member); \
                    } \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->processes_used); \
                    for(size_t i = 0; i < payload->processes_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            break; \
                        topology_v1_string_column_add(&column, payload->processes[i].member); \
                    } \
                    if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                        topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                    if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                } while(0)

                    NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(pid);
                    NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(ppid);
                    NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(uid);
                    NV_TOPOLOGY_V1_PROCESS_UINT_VALUES(net_ns_inode);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(process);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(username);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(namespace_type);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(local_ip);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(local_address_space);
                    NV_TOPOLOGY_V1_PROCESS_STRING_VALUES(cmdline);

#undef NV_TOPOLOGY_V1_PROCESS_UINT_VALUES
#undef NV_TOPOLOGY_V1_PROCESS_STRING_VALUES

                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "cgroups");
            {
                buffer_json_member_add_string(wb, "type", "cgroups");
                buffer_json_member_add_object(wb, "table");
                {
                    buffer_json_member_add_uint64(wb, "rows", payload->cgroups_used);
                    buffer_json_member_add_array(wb, "columns");
                    topology_v1_emit_cgroup_detail_columns(wb);
                    buffer_json_array_close(wb);
                    buffer_json_member_add_array(wb, "values");

#define NV_TOPOLOGY_V1_CGROUP_UINT_VALUES(member) do { \
                    topology_v1_values_start(wb); \
                    for(size_t i = 0; i < payload->cgroups_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            return abort_status; \
                        buffer_json_add_array_item_uint64(wb, payload->cgroups[i].member); \
                    } \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->cgroups_used); \
                    for(size_t i = 0; i < payload->cgroups_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            break; \
                        topology_v1_string_column_add(&column, payload->cgroups[i].member); \
                    } \
                    if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                        topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                    if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                } while(0)

                    NV_TOPOLOGY_V1_CGROUP_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_CGROUP_UINT_VALUES(pid);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(cgroup_status);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(cgroup_path);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(cgroup_name);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(container_name);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(orchestrator);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(actor_kind);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(k8s_pod_name);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(k8s_namespace);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(k8s_workload);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(docker_container_name);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(docker_image);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(systemd_unit_name);
                    NV_TOPOLOGY_V1_CGROUP_STRING_VALUES(systemd_unit_kind);

#undef NV_TOPOLOGY_V1_CGROUP_UINT_VALUES
#undef NV_TOPOLOGY_V1_CGROUP_STRING_VALUES

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
                    for(size_t i = 0; i < payload->labels_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            return abort_status; \
                        buffer_json_add_array_item_uint64(wb, payload->labels[i].member); \
                    } \
                    topology_v1_values_end(wb); \
                } while(0)
#define NV_TOPOLOGY_V1_LABEL_STRING_VALUES(member) do { \
                    NV_TOPOLOGY_V1_STRING_COLUMN column; \
                    topology_v1_string_column_init(&column, payload->labels_used); \
                    for(size_t i = 0; i < payload->labels_used; i++) { \
                        if(topology_abort_iteration_checkpoint(i) && \
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                            break; \
                        topology_v1_string_column_add(&column, payload->labels[i].member); \
                    } \
                    if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                        topology_v1_emit_auto_string_column(wb, &column); \
                    topology_v1_string_column_free(&column); \
                    if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                } while(0)

                    NV_TOPOLOGY_V1_LABEL_UINT_VALUES(actor);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(key);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(value);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(source);
                    NV_TOPOLOGY_V1_LABEL_STRING_VALUES(kind);

                    topology_v1_values_start(wb);
                    for(size_t i = 0; i < payload->labels_used; i++) {
                        if(topology_abort_iteration_checkpoint(i) &&
                           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                            return abort_status;
                        topology_v1_add_nullable_uint(wb, payload->labels[i].has_value_index, payload->labels[i].value_index);
                    }
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
                            for(size_t i = 0; i < payload->connections_used; i++) { \
                                if(topology_abort_iteration_checkpoint(i) && \
                                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                                    return abort_status; \
                                buffer_json_add_array_item_uint64(wb, payload->connections[i].member); \
                            } \
                            topology_v1_values_end(wb); \
                        } while(0)
#define NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES(member) do { \
                            NV_TOPOLOGY_V1_STRING_COLUMN column; \
                            topology_v1_string_column_init(&column, payload->connections_used); \
                            for(size_t i = 0; i < payload->connections_used; i++) { \
                                if(topology_abort_iteration_checkpoint(i) && \
                                   (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                                    break; \
                                topology_v1_string_column_add(&column, payload->connections[i].member); \
                            } \
                            if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                                topology_v1_emit_auto_string_column(wb, &column); \
                            topology_v1_string_column_free(&column); \
                            if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                                return abort_status; \
                        } while(0)

                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(src_actor);
                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(dst_actor);
                        NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES(client_ip);
                        NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES(server_ip);
                        NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES(protocol);
                        NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES(state);
                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(socket_count);
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
                        NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES(retransmissions);

                        topology_v1_values_start(wb);
                        for(size_t i = 0; i < payload->connections_used; i++) {
                            if(topology_abort_iteration_checkpoint(i) &&
                               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                                return abort_status;
                            buffer_json_add_array_item_double(wb, (double)payload->connections[i].max_rtt_usec / (double)USEC_PER_MS);
                        }
                        topology_v1_values_end(wb);

                        topology_v1_values_start(wb);
                        for(size_t i = 0; i < payload->connections_used; i++) {
                            if(topology_abort_iteration_checkpoint(i) &&
                               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                                return abort_status;
                            buffer_json_add_array_item_double(wb, (double)payload->connections[i].max_rcv_rtt_usec / (double)USEC_PER_MS);
                        }
                        topology_v1_values_end(wb);
#endif

#undef NV_TOPOLOGY_V1_CONNECTION_UINT_VALUES
#undef NV_TOPOLOGY_V1_CONNECTION_STRING_VALUES

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
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_socket_evidence_table(
    BUFFER *wb,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

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
                for(size_t i = 0; i < payload->evidence_used; i++) { \
                    if(topology_abort_iteration_checkpoint(i) && \
                       (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                    buffer_json_add_array_item_uint64(wb, payload->evidence[i].member); \
                } \
                topology_v1_values_end(wb); \
            } while(0)
#define NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(member) do { \
                topology_v1_values_start(wb); \
                for(size_t i = 0; i < payload->evidence_used; i++) { \
                    if(topology_abort_iteration_checkpoint(i) && \
                       (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                        return abort_status; \
                    buffer_json_add_array_item_uint64(wb, payload->evidence[i].source->member); \
                } \
                topology_v1_values_end(wb); \
            } while(0)
#define NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(member) do { \
                NV_TOPOLOGY_V1_STRING_COLUMN column; \
                topology_v1_string_column_init(&column, payload->evidence_used); \
                for(size_t i = 0; i < payload->evidence_used; i++) { \
                    if(topology_abort_iteration_checkpoint(i) && \
                       (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                        break; \
                    topology_v1_string_column_add(&column, payload->evidence[i].source->member); \
                } \
                if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
                    topology_v1_emit_auto_string_column(wb, &column); \
                topology_v1_string_column_free(&column); \
                if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
                    return abort_status; \
            } while(0)

                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(link);
                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(src_actor);
                NV_TOPOLOGY_V1_EVIDENCE_UINT_VALUES(dst_actor);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(client_ip);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(client_port);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(server_ip);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(server_port);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(protocol);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(protocol_family);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(state);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(namespace_type);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(client_address_space);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(server_address_space);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(pid);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(uid);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(net_ns_inode);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_STRING_VALUES(process);
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(sockets);
#if defined(LOCAL_SOCKETS_HAVE_TCP_INFO)
                NV_TOPOLOGY_V1_EVIDENCE_SOURCE_UINT_VALUES(retransmissions);

                topology_v1_values_start(wb);
                for(size_t i = 0; i < payload->evidence_used; i++) {
                    if(topology_abort_iteration_checkpoint(i) &&
                       (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                        return abort_status;
                    buffer_json_add_array_item_double(wb, (double)payload->evidence[i].source->max_rtt_usec / (double)USEC_PER_MS);
                }
                topology_v1_values_end(wb);

                topology_v1_values_start(wb);
                for(size_t i = 0; i < payload->evidence_used; i++) {
                    if(topology_abort_iteration_checkpoint(i) &&
                       (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
                        return abort_status;
                    buffer_json_add_array_item_double(wb, (double)payload->evidence[i].source->max_rcv_rtt_usec / (double)USEC_PER_MS);
                }
                topology_v1_values_end(wb);
#endif

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
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_correlation_table(
    BUFFER *wb,
    const NV_TOPOLOGY_V1_CORRELATION_ROW *rows,
    size_t rows_used,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

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
    for(size_t i = 0; i < rows_used; i++) {
        if(topology_abort_iteration_checkpoint(i) &&
           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;
        buffer_json_add_array_item_uint64(wb, rows[i].actor);
    }
    topology_v1_values_end(wb);

    topology_v1_const_string(wb, "socket_exact");

#define NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(member) do { \
        NV_TOPOLOGY_V1_STRING_COLUMN column; \
        topology_v1_string_column_init(&column, rows_used); \
        for(size_t i = 0; i < rows_used; i++) { \
            if(topology_abort_iteration_checkpoint(i) && \
               (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE) \
                break; \
            topology_v1_string_column_add(&column, rows[i].member); \
        } \
        if(abort_status == NV_TOPOLOGY_ABORT_NONE) \
            topology_v1_emit_auto_string_column(wb, &column); \
        topology_v1_string_column_free(&column); \
        if(abort_status != NV_TOPOLOGY_ABORT_NONE) \
            return abort_status; \
    } while(0)

    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(protocol);
    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(address_space);
    NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES(ip);

#undef NV_TOPOLOGY_V1_CORRELATION_STRING_VALUES

    topology_v1_values_start(wb);
    for(size_t i = 0; i < rows_used; i++) {
        if(topology_abort_iteration_checkpoint(i) &&
           (abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;
        buffer_json_add_array_item_uint64(wb, rows[i].port);
    }
    topology_v1_values_end(wb);

    buffer_json_array_close(wb);
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_v1_emit_correlation(
    BUFFER *wb,
    NV_TOPOLOGY_V1_PAYLOAD *payload,
    NV_TOPOLOGY_CONTEXT *ctx) {
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

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
                buffer_json_add_array_item_string(wb, "container");
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
        abort_status = topology_v1_emit_correlation_table(
            wb, payload->correlation_points, payload->correlation_points_used, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "claims");
        abort_status = topology_v1_emit_correlation_table(
            wb, payload->correlation_claims, payload->correlation_claims_used, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            return abort_status;
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
    return topology_response_check(wb, ctx);
}

static NV_TOPOLOGY_ABORT_STATUS topology_write_data(BUFFER *wb, NV_TOPOLOGY_CONTEXT *ctx) {
    if(!ctx || ctx->options.info_only || !ctx->process_actors || !ctx->remote_actors || !ctx->local_ips || !ctx->links)
        return NV_TOPOLOGY_ABORT_NONE;

    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_response_check(wb, ctx);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return abort_status;

    NV_TOPOLOGY_RENDER_STATE state;
    topology_render_state_init(&state, ctx);

    NV_TOPOLOGY_V1_PAYLOAD topology = {
        .actor_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .graph_link_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .connection_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .port_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .process_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .cgroup_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .correlation_point_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(bool)),
        .correlation_claim_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(bool)),
        .label_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(bool)),
    };

    if(!topology.actor_index || !topology.graph_link_index || !topology.connection_index || !topology.port_index ||
       !topology.process_index || !topology.cgroup_index ||
       !topology.correlation_point_index || !topology.correlation_claim_index || !topology.label_index) {
        topology_v1_free(&topology);
        return NV_TOPOLOGY_ABORT_NONE;
    }

    abort_status = topology_v1_collect_actors(ctx, &state, &topology);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        goto cleanup;

    abort_status = topology_v1_collect_links(ctx, &state, &topology);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        goto cleanup;

    buffer_json_member_add_object(wb, "data");
    {
        if((abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

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
            buffer_json_member_add_string(wb, "scope", topology_group_by_id(ctx->options.group_by));
            buffer_json_member_add_string(wb, "mode", ctx->options.detailed ? "detailed" : "aggregated");
            buffer_json_member_add_array(wb, "supported_modes");
            buffer_json_add_array_item_string(wb, "aggregated");
            buffer_json_add_array_item_string(wb, "detailed");
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "group_by");
            buffer_json_add_array_item_string(wb, "process_name");
            buffer_json_add_array_item_string(wb, "pid");
            buffer_json_add_array_item_string(wb, "container");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "dictionaries");
        {
            buffer_json_member_add_array(wb, "strings");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        if((abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        topology_v1_emit_type_registry(wb, &topology, ctx->options.group_by, ctx->options.detailed);
        if((abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        topology_v1_emit_presentation(wb);
        if((abort_status = topology_response_check(wb, ctx)) != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        abort_status = topology_v1_emit_correlation(wb, &topology, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        abort_status = topology_v1_emit_actor_table(wb, &topology, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        abort_status = topology_v1_emit_link_table(wb, &topology, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        abort_status = topology_v1_emit_socket_port_table(wb, &topology, ctx);
        if(abort_status != NV_TOPOLOGY_ABORT_NONE)
            goto cleanup;

        if(ctx->options.detailed) {
            abort_status = topology_v1_emit_socket_evidence_table(wb, &topology, ctx);
            if(abort_status != NV_TOPOLOGY_ABORT_NONE)
                goto cleanup;
        }

        buffer_json_member_add_object(wb, "stats");
        {
            buffer_json_member_add_string(wb, "group_by", topology_group_by_id(ctx->options.group_by));
            buffer_json_member_add_string(wb, "mode", ctx->options.detailed ? "detailed" : "aggregated");
            buffer_json_member_add_boolean(wb, "sockets_listening", ctx->options.sockets_listening);
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
            buffer_json_member_add_uint64(wb, "process_rows", topology.processes_used);
            buffer_json_member_add_uint64(wb, "cgroup_rows", topology.cgroups_used);
            buffer_json_member_add_uint64(wb, "correlation_points", topology.correlation_points_used);
            buffer_json_member_add_uint64(wb, "correlation_claims", topology.correlation_claims_used);
            buffer_json_member_add_uint64(wb, "ownership_links", state.ownership_link_count);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

cleanup:
    topology_v1_free(&topology);
    return abort_status;
}

static BUFFER *network_viewer_topology_result(
    char *function, usec_t *stop_monotonic_ut,
    bool *cancelled, BUFFER *payload) {

    time_t now_s = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();
    NV_TOPOLOGY_OPTIONS options = { 0 };
    NV_TOPOLOGY_ABORT_STATUS abort_status = topology_function_abort_state(stop_monotonic_ut, cancelled);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE)
        return network_viewer_json_error_response(
            topology_abort_http_code(abort_status),
            topology_abort_message(abort_status));

    topology_parse_options(function, &options);
    CLEAN_BUFFER *error = buffer_create(0, NULL);
    if(!topology_parse_payload_options(payload, &options, error)) {
        simple_pattern_free(options.label_whitelist);
        return network_viewer_json_error_response(HTTP_RESP_BAD_REQUEST, buffer_tostring(error));
    }

    abort_status = topology_function_abort_state(stop_monotonic_ut, cancelled);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE) {
        simple_pattern_free(options.label_whitelist);
        return network_viewer_json_error_response(
            topology_abort_http_code(abort_status),
            topology_abort_message(abort_status));
    }

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    topology_write_response_metadata(wb);

    NV_TOPOLOGY_CONTEXT ctx = { 0 };
    bool ctx_ready = topology_prepare_context(&ctx, now_ut, &options, stop_monotonic_ut, cancelled);
    abort_status = ctx.abort_status;
    if(ctx_ready) {
        abort_status = nv_warm_cache_from_topology_actors(&ctx);
        if(abort_status == NV_TOPOLOGY_ABORT_NONE)
            abort_status = topology_write_data(wb, &ctx);
    }

    topology_context_destroy(&ctx);
    simple_pattern_free(options.label_whitelist);
    if(abort_status != NV_TOPOLOGY_ABORT_NONE) {
        buffer_free(wb);
        return network_viewer_json_error_response(
            topology_abort_http_code(abort_status),
            topology_abort_message(abort_status));
    }

    network_viewer_finalize_response_buffer(wb, now_s);
    return wb;
}

static void network_viewer_topology_function(
    const char *transaction, char *function, usec_t *stop_monotonic_ut,
    bool *cancelled, BUFFER *payload, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {

    BUFFER *wb = network_viewer_topology_result(function, stop_monotonic_ut, cancelled, payload);
    network_viewer_emit_response(transaction, wb);
    buffer_free(wb);
}

static int local_sockets_compar(const void *a, const void *b) {
    LOCAL_SOCKET *n1 = *(LOCAL_SOCKET **)a, *n2 = *(LOCAL_SOCKET **)b;
    return strcmp(n1->comm, n2->comm);
}

static BUFFER *network_viewer_result(char *function) {

    time_t now_s = now_realtime_sec();
    bool aggregated = false;

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

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
    if(st.container_field_snapshot)
        dictionary_destroy(st.container_field_snapshot);
    if(st.pid_starttime_cache)
        dictionary_destroy(st.pid_starttime_cache);
    network_viewer_finalize_response_buffer(wb, now_s);
    return wb;
}

void network_viewer_function(
    const char *transaction, char *function,
    usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {

    BUFFER *wb = network_viewer_result(function);
    network_viewer_emit_response(transaction, wb);
    buffer_free(wb);
}

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

    network_viewer_finalize_response_buffer(wb, now_s);
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

#if defined(LOCAL_SOCKETS_USE_SETNS)
    spawn_server_destroy(spawn_srv);
    spawn_srv = NULL;
#endif

    return exit_code;
}
