// SPDX-License-Identifier: GPL-3.0-or-later

#include <arpa/inet.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <linux/if_vlan.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <sys/time.h>

#include "ebpf.h"
#include "ebpf_functions.h"
#include "libbpf_api/ebpf_library.h"

#ifdef LIBBPF_MAJOR_VERSION
#include "dns.skel.h"
#endif

/*****************************************************************
 *  EBPF FUNCTION COMMON
 *****************************************************************/

typedef struct ebpf_function_thread_start {
    ebpf_module_t *em;
    void (*start_routine)(void *);
    bool ready;
    bool run;
} ebpf_function_thread_start_t;

static void ebpf_function_thread_start(void *ptr)
{
    ebpf_function_thread_start_t *ctx = ptr;

    // Keep the new thread parked until its owner publishes the module state.
    while (!__atomic_load_n(&ctx->ready, __ATOMIC_ACQUIRE))
        tinysleep();

    bool run = __atomic_load_n(&ctx->run, __ATOMIC_ACQUIRE);
    ebpf_module_t *em = ctx->em;
    void (*start_routine)(void *) = ctx->start_routine;
    freez(ctx);

    if (run)
        start_routine(em);
}

/**
 * Function Start thread
 *
 * Start a specific thread after user request.
 *
 * @param em           The structure with thread information
 * @param period
 * @return
 */
static int ebpf_function_start_thread(ebpf_module_t *em, int period)
{
    struct netdata_static_thread *st = em->thread;
    // another request for thread that already ran, cleanup and restart
    if (period <= 0)
        period = EBPF_DEFAULT_LIFETIME;

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("Starting thread %s with lifetime = %d", em->info.thread_name, period);
#endif

    ebpf_function_thread_start_t *ctx = callocz(1, sizeof(*ctx));
    ctx->em = em;
    ctx->start_routine = st->start_routine;

    ND_THREAD *thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_function_thread_start, ctx);
    if (!thread) {
        freez(ctx);
        return 1;
    }

    bool run = true;
    netdata_mutex_lock(&ebpf_exit_cleanup);
    if (ebpf_plugin_stop())
        run = false;
    else {
        st->thread = thread;
        ebpf_module_enabled_set(em, NETDATA_THREAD_EBPF_FUNCTION_RUNNING);
        em->lifetime = period;
    }
    __atomic_store_n(&ctx->run, run, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->ready, true, __ATOMIC_RELEASE);
    netdata_mutex_unlock(&ebpf_exit_cleanup);

    if (!run) {
        nd_thread_signal_cancel(thread);
        nd_thread_join(thread);
        return 1;
    }

    return 0;
}

/*****************************************************************
 *  EBPF ERROR FUNCTIONS
 *****************************************************************/

/**
 * Function error
 *
 * Show error when a wrong function is given
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param code         the error code to show with the message.
 * @param msg          the error message
 */
static inline void ebpf_function_error(const char *transaction, int code, const char *msg)
{
    pluginsd_function_json_error_to_stdout(transaction, code, msg);
}

/**
 * Thread Help
 *
 * Shows help with all options accepted by thread function.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
*/
static inline void ebpf_function_help(const char *transaction, const char *message)
{
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout, "%s", message);
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

/*****************************************************************
 *  EBPF DNS FUNCTION
 *****************************************************************/

#define NETDATA_DNS_MAX_PORTS 32
#define NETDATA_DNS_DEFAULT_PORT 53
#define NETDATA_DNS_CAPTURE_INTERVAL 5
#define NETDATA_DNS_TIMEOUT_USEC (5ULL * USEC_PER_SEC)
#define NETDATA_DNS_MAX_DOMAIN_LENGTH 256
#define NETDATA_DNS_PACKET_BUFFER 65536
#define NETDATA_DNS_IPV4_MIN_HEADER 20
#define NETDATA_DNS_IPV6_HEADER 40
#define NETDATA_DNS_UDP_HEADER 8
#define NETDATA_DNS_TCP_MIN_HEADER 20
#define NETDATA_DNS_RCODE_LENGTH 256
#define NETDATA_DNS_LEGACY_KERNELS (NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14)

typedef struct netdata_dns_flow_key {
    uint8_t family;
    uint8_t protocol;
    uint16_t client_port;
    uint8_t server_ip[16];
    uint8_t client_ip[16];
} netdata_dns_flow_key_t;

typedef struct netdata_dns_rcode_counter {
    uint32_t code;
    uint32_t count;
    struct netdata_dns_rcode_counter *next;
} netdata_dns_rcode_counter_t;

typedef struct netdata_dns_stats {
    netdata_dns_flow_key_t key;
    uint16_t query_type;
    char domain[NETDATA_DNS_MAX_DOMAIN_LENGTH];
    uint32_t timeouts;
    uint64_t success_latency_sum;
    uint64_t failure_latency_sum;
    netdata_dns_rcode_counter_t *rcodes;
    struct netdata_dns_stats *next;
} netdata_dns_stats_t;

typedef struct netdata_dns_state {
    netdata_dns_flow_key_t key;
    uint16_t transaction_id;
    uint16_t query_type;
    uint64_t timestamp_usec;
    char domain[NETDATA_DNS_MAX_DOMAIN_LENGTH];
    struct netdata_dns_state *next;
} netdata_dns_state_t;

typedef struct netdata_dns_collector {
    netdata_dns_stats_t *stats;
    netdata_dns_state_t *state;
    size_t pending_queries;
    size_t total_results;
} netdata_dns_collector_t;

typedef struct netdata_dns_packet {
    netdata_dns_flow_key_t key;
    uint16_t transaction_id;
    uint16_t query_type;
    uint8_t response;
    uint8_t rcode;
    char domain[NETDATA_DNS_MAX_DOMAIN_LENGTH];
} netdata_dns_packet_t;

typedef struct ebpf_dns_config {
    uint16_t ports[NETDATA_DNS_MAX_PORTS];
    size_t port_count;
    int iterations;
} ebpf_dns_config_t;

typedef struct ebpf_dns_runtime {
    const char *source;
    const char *stage;
    const char *operation;
    int error_code;
    char object_path[FILENAME_MAX + 1];
} ebpf_dns_runtime_t;

typedef struct ebpf_dns_handle {
    struct bpf_object *obj;
#ifdef LIBBPF_MAJOR_VERSION
    struct dns_bpf *core;
#endif
    const char *source;
    char object_path[FILENAME_MAX + 1];
} ebpf_dns_handle_t;

static uint16_t ebpf_dns_read_u16(const uint8_t *src)
{
    return ((uint16_t)src[0] << 8) | src[1];
}

static size_t ebpf_dns_ip_size(uint8_t family)
{
    return (family == AF_INET6) ? 16 : 4;
}

static int ebpf_dns_flow_key_equal(const netdata_dns_flow_key_t *a, const netdata_dns_flow_key_t *b)
{
    size_t length;

    if (a->family != b->family || a->protocol != b->protocol || a->client_port != b->client_port)
        return 0;

    length = ebpf_dns_ip_size(a->family);
    if (memcmp(a->server_ip, b->server_ip, length))
        return 0;

    if (memcmp(a->client_ip, b->client_ip, length))
        return 0;

    return 1;
}

static void ebpf_dns_format_ip(char *dst, size_t len, uint8_t family, const uint8_t *src)
{
    if (!inet_ntop(family, src, dst, len))
        snprintfz(dst, len, "%s", "invalid");
}

static const char *ebpf_dns_protocol_name(uint8_t protocol)
{
    switch (protocol) {
        case IPPROTO_TCP:
            return "TCP";
        case IPPROTO_UDP:
            return "UDP";
        default:
            return "UNKNOWN";
    }
}

static void ebpf_dns_set_error(ebpf_dns_runtime_t *runtime, const char *stage, const char *operation, int error_code)
{
    runtime->stage = stage;
    runtime->operation = operation;
    runtime->error_code = error_code;
}

static inline void ebpf_dns_reset_config(ebpf_dns_config_t *cfg)
{
    memset(cfg, 0, sizeof(*cfg));
    cfg->ports[0] = NETDATA_DNS_DEFAULT_PORT;
    cfg->port_count = 1;
    cfg->iterations = 1;
}

static int ebpf_dns_add_port(ebpf_dns_config_t *cfg, uint16_t port, char *error, size_t error_size)
{
    size_t i;

    for (i = 0; i < cfg->port_count; i++) {
        if (cfg->ports[i] == port)
            return 0;
    }

    if (cfg->port_count >= NETDATA_DNS_MAX_PORTS) {
        snprintfz(error, error_size, "Maximum number of DNS ports (%d) reached.", NETDATA_DNS_MAX_PORTS);
        return -1;
    }

    cfg->ports[cfg->port_count++] = port;
    return 0;
}

static int ebpf_dns_parse_port_list(ebpf_dns_config_t *cfg, const char *input, char *error, size_t error_size)
{
    char *copy = strdupz(input);
    char *cursor = NULL;
    char *token;

    cfg->port_count = 0;
    token = strtok_r(copy, ",", &cursor);
    while (token) {
        char *endptr = NULL;
        unsigned long port;

        if (*token) {
            port = strtoul(token, &endptr, 10);
            if (*endptr || port == 0 || port > UINT16_MAX) {
                snprintfz(error, error_size, "DNS port value (%s) is not valid.", token);
                freez(copy);
                return -1;
            }

            if (ebpf_dns_add_port(cfg, (uint16_t)port, error, error_size)) {
                freez(copy);
                return -1;
            }
        }

        token = strtok_r(NULL, ",", &cursor);
    }

    freez(copy);

    if (!cfg->port_count)
        return ebpf_dns_add_port(cfg, NETDATA_DNS_DEFAULT_PORT, error, error_size);

    return 0;
}

static int ebpf_dns_parse_iterations(ebpf_dns_config_t *cfg, const char *input, char *error, size_t error_size)
{
    char *endptr = NULL;
    long value = strtol(input, &endptr, 10);

    if (*endptr) {
        snprintfz(error, error_size, "Iteration value (%s) is not valid.", input);
        return -1;
    }

    if (value < 1)
        value = 1;

    cfg->iterations = (int)value;
    return 0;
}

static int ebpf_dns_read_name(
    const uint8_t *data, size_t length, size_t offset, char *dst, size_t dst_len, size_t *next)
{
    size_t current = offset;
    size_t out = 0;
    size_t jumps = 0;
    int jumped = 0;

    if (!dst_len)
        return 0;

    while (current < length && jumps < 32) {
        uint8_t label = data[current];

        if ((label & 0xC0) == 0xC0) {
            size_t pointer;

            if (current + 1 >= length)
                return 0;

            pointer = ((size_t)(label & 0x3F) << 8) | data[current + 1];
            if (!jumped) {
                *next = current + 2;
                jumped = 1;
            }

            current = pointer;
            jumps++;
            continue;
        }

        current++;
        if (label == 0) {
            if (!jumped)
                *next = current;

            if (!out) {
                if (dst_len < 2)
                    return 0;

                dst[0] = '.';
                dst[1] = '\0';
            } else {
                dst[out] = '\0';
            }

            return 1;
        }

        if (label > 63 || current + label > length)
            return 0;

        if (out && out + 1 >= dst_len)
            return 0;

        if (out)
            dst[out++] = '.';

        if (out + label >= dst_len)
            return 0;

        while (label--) {
            unsigned char ch = data[current++];

            if (ch >= 'A' && ch <= 'Z')
                ch = (unsigned char)(ch - 'A' + 'a');

            dst[out++] = (char)ch;
        }

        jumps++;
    }

    return 0;
}

static int ebpf_dns_parse_payload(
    const uint8_t *payload, size_t payload_len, uint8_t protocol, netdata_dns_packet_t *packet)
{
    const uint8_t *message = payload;
    size_t message_len = payload_len;
    uint16_t flags;
    uint16_t qdcount;
    uint16_t qclass;
    size_t offset = 12;

    if (protocol == IPPROTO_TCP) {
        uint16_t dns_length;

        if (payload_len < 2)
            return 0;

        dns_length = ebpf_dns_read_u16(payload);
        if (!dns_length || (size_t)dns_length + 2 > payload_len)
            return 0;

        message = payload + 2;
        message_len = dns_length;
    }

    if (message_len < 12)
        return 0;

    packet->transaction_id = ebpf_dns_read_u16(message);
    flags = ebpf_dns_read_u16(message + 2);
    qdcount = ebpf_dns_read_u16(message + 4);

    if (qdcount != 1)
        return 0;

    if (!ebpf_dns_read_name(message, message_len, offset, packet->domain, sizeof(packet->domain), &offset))
        return 0;

    if (offset + 4 > message_len)
        return 0;

    packet->query_type = ebpf_dns_read_u16(message + offset);
    qclass = ebpf_dns_read_u16(message + offset + 2);
    if (qclass != 1)
        return 0;

    packet->response = (flags & 0x8000U) ? 1 : 0;
    packet->rcode = (uint8_t)(flags & 0x000FU);

    return 1;
}

static int ebpf_dns_parse_ipv4(
    const uint8_t *packet, size_t length, size_t offset, netdata_dns_packet_t *dns_packet)
{
    size_t ihl;
    size_t l4_offset;
    size_t l4_length;
    uint16_t total_length;
    uint16_t frag_off;
    uint8_t protocol;
    uint16_t src_port;
    uint16_t dst_port;
    const uint8_t *payload;
    size_t payload_length;

    if (offset + NETDATA_DNS_IPV4_MIN_HEADER > length)
        return 0;

    if ((packet[offset] >> 4) != 4)
        return 0;

    ihl = (packet[offset] & 0x0FU) * 4U;
    if (ihl < NETDATA_DNS_IPV4_MIN_HEADER || offset + ihl > length)
        return 0;

    total_length = ebpf_dns_read_u16(packet + offset + 2);
    if (total_length < ihl)
        return 0;

    frag_off = ebpf_dns_read_u16(packet + offset + 6);
    if (frag_off & 0x1FFFU)
        return 0;

    protocol = packet[offset + 9];
    if (protocol != IPPROTO_UDP && protocol != IPPROTO_TCP)
        return 0;

    l4_offset = offset + ihl;
    if (offset + total_length < l4_offset)
        return 0;

    l4_length = (offset + total_length <= length) ? (offset + total_length - l4_offset) : (length - l4_offset);
    if (!l4_length)
        return 0;

    if (protocol == IPPROTO_UDP) {
        if (l4_length < NETDATA_DNS_UDP_HEADER)
            return 0;

        src_port = ebpf_dns_read_u16(packet + l4_offset);
        dst_port = ebpf_dns_read_u16(packet + l4_offset + 2);
        payload = packet + l4_offset + NETDATA_DNS_UDP_HEADER;
        payload_length = l4_length - NETDATA_DNS_UDP_HEADER;
    } else {
        size_t tcp_header_length;

        if (l4_length < NETDATA_DNS_TCP_MIN_HEADER)
            return 0;

        src_port = ebpf_dns_read_u16(packet + l4_offset);
        dst_port = ebpf_dns_read_u16(packet + l4_offset + 2);
        tcp_header_length = (size_t)((packet[l4_offset + 12] >> 4) & 0x0FU) * 4U;
        if (tcp_header_length < NETDATA_DNS_TCP_MIN_HEADER || tcp_header_length > l4_length)
            return 0;

        payload = packet + l4_offset + tcp_header_length;
        payload_length = l4_length - tcp_header_length;
    }

    memset(dns_packet, 0, sizeof(*dns_packet));
    if (!ebpf_dns_parse_payload(payload, payload_length, protocol, dns_packet))
        return 0;

    dns_packet->key.family = AF_INET;
    dns_packet->key.protocol = protocol;
    if (!dns_packet->response) {
        memcpy(dns_packet->key.client_ip, packet + offset + 12, 4);
        memcpy(dns_packet->key.server_ip, packet + offset + 16, 4);
        dns_packet->key.client_port = src_port;
    } else {
        memcpy(dns_packet->key.server_ip, packet + offset + 12, 4);
        memcpy(dns_packet->key.client_ip, packet + offset + 16, 4);
        dns_packet->key.client_port = dst_port;
    }

    return 1;
}

static int ebpf_dns_parse_ipv6(
    const uint8_t *packet, size_t length, size_t offset, netdata_dns_packet_t *dns_packet)
{
    size_t l4_offset;
    size_t l4_length;
    uint16_t payload_size;
    uint8_t protocol;
    uint16_t src_port;
    uint16_t dst_port;
    const uint8_t *payload;
    size_t payload_length;

    if (offset + NETDATA_DNS_IPV6_HEADER > length)
        return 0;

    if ((packet[offset] >> 4) != 6)
        return 0;

    payload_size = ebpf_dns_read_u16(packet + offset + 4);
    protocol = packet[offset + 6];
    if (protocol != IPPROTO_UDP && protocol != IPPROTO_TCP)
        return 0;

    l4_offset = offset + NETDATA_DNS_IPV6_HEADER;
    if (l4_offset > length)
        return 0;

    l4_length = (l4_offset + payload_size <= length) ? payload_size : (length - l4_offset);
    if (!l4_length)
        return 0;

    if (protocol == IPPROTO_UDP) {
        if (l4_length < NETDATA_DNS_UDP_HEADER)
            return 0;

        src_port = ebpf_dns_read_u16(packet + l4_offset);
        dst_port = ebpf_dns_read_u16(packet + l4_offset + 2);
        payload = packet + l4_offset + NETDATA_DNS_UDP_HEADER;
        payload_length = l4_length - NETDATA_DNS_UDP_HEADER;
    } else {
        size_t tcp_header_length;

        if (l4_length < NETDATA_DNS_TCP_MIN_HEADER)
            return 0;

        src_port = ebpf_dns_read_u16(packet + l4_offset);
        dst_port = ebpf_dns_read_u16(packet + l4_offset + 2);
        tcp_header_length = (size_t)((packet[l4_offset + 12] >> 4) & 0x0FU) * 4U;
        if (tcp_header_length < NETDATA_DNS_TCP_MIN_HEADER || tcp_header_length > l4_length)
            return 0;

        payload = packet + l4_offset + tcp_header_length;
        payload_length = l4_length - tcp_header_length;
    }

    memset(dns_packet, 0, sizeof(*dns_packet));
    if (!ebpf_dns_parse_payload(payload, payload_length, protocol, dns_packet))
        return 0;

    dns_packet->key.family = AF_INET6;
    dns_packet->key.protocol = protocol;
    if (!dns_packet->response) {
        memcpy(dns_packet->key.client_ip, packet + offset + 8, 16);
        memcpy(dns_packet->key.server_ip, packet + offset + 24, 16);
        dns_packet->key.client_port = src_port;
    } else {
        memcpy(dns_packet->key.server_ip, packet + offset + 8, 16);
        memcpy(dns_packet->key.client_ip, packet + offset + 24, 16);
        dns_packet->key.client_port = dst_port;
    }

    return 1;
}

static int ebpf_dns_parse_packet(const uint8_t *packet, size_t length, netdata_dns_packet_t *dns_packet)
{
    size_t offset = ETH_HLEN;
    uint16_t protocol;

    if (length < ETH_HLEN)
        return 0;

    protocol = ebpf_dns_read_u16(packet + 12);
    while (protocol == ETH_P_8021Q || protocol == ETH_P_8021AD) {
        if (offset + 4 > length)
            return 0;

        protocol = ebpf_dns_read_u16(packet + offset + 2);
        offset += 4;
    }

    if (protocol == ETH_P_IP)
        return ebpf_dns_parse_ipv4(packet, length, offset, dns_packet);

    if (protocol == ETH_P_IPV6)
        return ebpf_dns_parse_ipv6(packet, length, offset, dns_packet);

    return 0;
}

static netdata_dns_stats_t *ebpf_dns_find_stats(
    netdata_dns_collector_t *collector, const netdata_dns_flow_key_t *key, const char *domain, uint16_t query_type)
{
    netdata_dns_stats_t *current = collector->stats;

    while (current) {
        if (current->query_type == query_type && !strcmp(current->domain, domain) &&
            ebpf_dns_flow_key_equal(&current->key, key))
            return current;

        current = current->next;
    }

    return NULL;
}

static netdata_dns_stats_t *ebpf_dns_get_stats(
    netdata_dns_collector_t *collector, const netdata_dns_flow_key_t *key, const char *domain, uint16_t query_type)
{
    netdata_dns_stats_t *stats = ebpf_dns_find_stats(collector, key, domain, query_type);

    if (stats)
        return stats;

    stats = callocz(1, sizeof(*stats));
    memcpy(&stats->key, key, sizeof(*key));
    stats->query_type = query_type;
    strncpy(stats->domain, domain, sizeof(stats->domain) - 1);
    stats->next = collector->stats;
    collector->stats = stats;
    collector->total_results++;

    return stats;
}

static void ebpf_dns_increment_rcode(netdata_dns_stats_t *stats, uint8_t rcode)
{
    netdata_dns_rcode_counter_t *current = stats->rcodes;

    while (current) {
        if (current->code == rcode) {
            current->count++;
            return;
        }

        current = current->next;
    }

    current = callocz(1, sizeof(*current));
    current->code = rcode;
    current->count = 1;
    current->next = stats->rcodes;
    stats->rcodes = current;
}

static void ebpf_dns_timeout_state(netdata_dns_collector_t *collector, netdata_dns_state_t *state)
{
    netdata_dns_stats_t *stats = ebpf_dns_get_stats(collector, &state->key, state->domain, state->query_type);

    if (stats)
        stats->timeouts++;
}

static void ebpf_dns_expire_states(netdata_dns_collector_t *collector, uint64_t now_usec)
{
    netdata_dns_state_t **current = &collector->state;

    while (*current) {
        netdata_dns_state_t *state = *current;

        if (now_usec - state->timestamp_usec > NETDATA_DNS_TIMEOUT_USEC) {
            ebpf_dns_timeout_state(collector, state);
            *current = state->next;
            freez(state);
            collector->pending_queries--;
            continue;
        }

        current = &state->next;
    }
}

static void ebpf_dns_process_query(
    netdata_dns_collector_t *collector, const netdata_dns_packet_t *packet, uint64_t now_usec)
{
    netdata_dns_state_t *current = collector->state;

    while (current) {
        if (current->transaction_id == packet->transaction_id && ebpf_dns_flow_key_equal(&current->key, &packet->key))
            return;

        current = current->next;
    }

    current = callocz(1, sizeof(*current));
    memcpy(&current->key, &packet->key, sizeof(packet->key));
    current->transaction_id = packet->transaction_id;
    current->query_type = packet->query_type;
    current->timestamp_usec = now_usec;
    strncpy(current->domain, packet->domain, sizeof(current->domain) - 1);
    current->next = collector->state;
    collector->state = current;
    collector->pending_queries++;
}

static void ebpf_dns_process_response(
    netdata_dns_collector_t *collector, const netdata_dns_packet_t *packet, uint64_t now_usec)
{
    netdata_dns_state_t **current = &collector->state;

    while (*current) {
        netdata_dns_state_t *state = *current;

        if (state->transaction_id == packet->transaction_id && ebpf_dns_flow_key_equal(&state->key, &packet->key)) {
            uint64_t latency = now_usec - state->timestamp_usec;
            netdata_dns_stats_t *stats = ebpf_dns_get_stats(collector, &state->key, state->domain, state->query_type);

            if (stats) {
                if (latency > NETDATA_DNS_TIMEOUT_USEC) {
                    stats->timeouts++;
                } else {
                    ebpf_dns_increment_rcode(stats, packet->rcode);
                    if (packet->rcode == 0)
                        stats->success_latency_sum += latency;
                    else
                        stats->failure_latency_sum += latency;
                }
            }

            *current = state->next;
            freez(state);
            collector->pending_queries--;
            return;
        }

        current = &state->next;
    }
}

static void ebpf_dns_free_collector(netdata_dns_collector_t *collector)
{
    netdata_dns_state_t *state = collector->state;
    netdata_dns_stats_t *stats = collector->stats;

    while (state) {
        netdata_dns_state_t *next = state->next;

        freez(state);
        state = next;
    }

    while (stats) {
        netdata_dns_rcode_counter_t *rcode = stats->rcodes;
        netdata_dns_stats_t *next = stats->next;

        while (rcode) {
            netdata_dns_rcode_counter_t *rcode_next = rcode->next;

            freez(rcode);
            rcode = rcode_next;
        }

        freez(stats);
        stats = next;
    }
}

static inline uint32_t ebpf_dns_rcode_success_count(const netdata_dns_stats_t *stats)
{
    uint32_t total = 0;
    netdata_dns_rcode_counter_t *rcode = stats->rcodes;

    while (rcode) {
        if (rcode->code == 0)
            total += rcode->count;

        rcode = rcode->next;
    }

    return total;
}

static inline uint32_t ebpf_dns_rcode_failure_count(const netdata_dns_stats_t *stats)
{
    uint32_t total = 0;
    netdata_dns_rcode_counter_t *rcode = stats->rcodes;

    while (rcode) {
        if (rcode->code != 0)
            total += rcode->count;

        rcode = rcode->next;
    }

    return total;
}

static void ebpf_dns_format_rcodes(const netdata_dns_stats_t *stats, char *buffer, size_t length)
{
    size_t used = 0;
    netdata_dns_rcode_counter_t *rcode = stats->rcodes;

    if (!length)
        return;

    if (!stats->rcodes) {
        snprintfz(buffer, length, "%s", "none");
        return;
    }

    buffer[0] = '\0';
    while (rcode && used + 1 < length) {
        char chunk[32];
        size_t available = length - used - 1;
        int written = snprintf(chunk, sizeof(chunk), "%s%u=%u", used ? ", " : "", rcode->code, rcode->count);
        if (written < 0)
            break;

        if ((size_t)written > available) {
            memcpy(buffer + used, chunk, available);
            used += available;
            buffer[used] = '\0';
            break;
        }

        memcpy(buffer + used, chunk, (size_t)written);
        used += (size_t)written;
        buffer[used] = '\0';
        rcode = rcode->next;
    }
}

static void ebpf_dns_fill_data_row(BUFFER *wb, const netdata_dns_stats_t *stats)
{
    uint32_t success = ebpf_dns_rcode_success_count(stats);
    uint32_t failure = ebpf_dns_rcode_failure_count(stats);
    NETDATA_DOUBLE avg_success_ms = success ? ((NETDATA_DOUBLE)stats->success_latency_sum / success) / 1000.0 : 0.0;
    NETDATA_DOUBLE avg_failure_ms = failure ? ((NETDATA_DOUBLE)stats->failure_latency_sum / failure) / 1000.0 : 0.0;
    char server_ip[INET6_ADDRSTRLEN];
    char client_ip[INET6_ADDRSTRLEN];
    char rcodes[NETDATA_DNS_RCODE_LENGTH];

    ebpf_dns_format_ip(server_ip, sizeof(server_ip), stats->key.family, stats->key.server_ip);
    ebpf_dns_format_ip(client_ip, sizeof(client_ip), stats->key.family, stats->key.client_ip);
    ebpf_dns_format_rcodes(stats, rcodes, sizeof(rcodes));

    buffer_json_add_array_item_array(wb);
    buffer_json_add_array_item_string(wb, stats->domain);
    buffer_json_add_array_item_uint64(wb, stats->query_type);
    buffer_json_add_array_item_string(wb, ebpf_dns_protocol_name(stats->key.protocol));
    buffer_json_add_array_item_string(wb, server_ip);
    buffer_json_add_array_item_string(wb, client_ip);
    buffer_json_add_array_item_uint64(wb, stats->key.client_port);
    buffer_json_add_array_item_uint64(wb, success + failure);
    buffer_json_add_array_item_uint64(wb, success);
    buffer_json_add_array_item_uint64(wb, failure);
    buffer_json_add_array_item_uint64(wb, stats->timeouts);
    buffer_json_add_array_item_double(wb, avg_success_ms);
    buffer_json_add_array_item_double(wb, avg_failure_ms);
    buffer_json_add_array_item_string(wb, rcodes);
    buffer_json_array_close(wb);
}

static void ebpf_dns_fill_data_array(BUFFER *wb, const netdata_dns_collector_t *collector)
{
    netdata_dns_stats_t *stats = collector->stats;

    while (stats) {
        ebpf_dns_fill_data_row(wb, stats);
        stats = stats->next;
    }
}

static struct bpf_program *ebpf_dns_find_socket_filter_program(struct bpf_object *obj)
{
    struct bpf_program *prog;

    bpf_object__for_each_program(prog, obj) {
        if (bpf_program__get_type(prog) == BPF_PROG_TYPE_SOCKET_FILTER)
            return prog;
    }

    return NULL;
}

static int ebpf_dns_configure_ports(
    struct bpf_object *obj, const ebpf_dns_config_t *cfg, ebpf_dns_runtime_t *runtime)
{
    struct bpf_map *map;
    const char *name = "dns_ports";

    bpf_object__for_each_map(map, obj) {
        const char *map_name = bpf_map__name(map);
        int fd;
        size_t i;
        uint8_t enabled = 1;

        if (strcmp(map_name, name))
            continue;

        fd = bpf_map__fd(map);
        for (i = 0; i < cfg->port_count; i++) {
            if (bpf_map_update_elem(fd, &cfg->ports[i], &enabled, BPF_ANY)) {
                ebpf_dns_set_error(runtime, "configure_ports", "bpf_map_update_elem", errno ? -errno : -1);
                return -1;
            }
        }

        return 0;
    }

    ebpf_dns_set_error(runtime, "configure_ports", "find_dns_ports_map", -ENOENT);
    return -1;
}

static int ebpf_dns_open_capture_socket(int program_fd, ebpf_dns_runtime_t *runtime)
{
    struct sockaddr_ll bind_addr = { .sll_family = AF_PACKET, .sll_protocol = htons(ETH_P_ALL) };
    int sockfd;
    struct timeval timeout = { .tv_sec = 1, .tv_usec = 0 };

    sockfd = socket(AF_PACKET, SOCK_RAW, htons(ETH_P_ALL));
    if (sockfd < 0) {
        ebpf_dns_set_error(runtime, "open_capture_socket", "socket", errno ? -errno : -1);
        return -1;
    }

    if (bind(sockfd, (struct sockaddr *)&bind_addr, sizeof(bind_addr))) {
        ebpf_dns_set_error(runtime, "open_capture_socket", "bind", errno ? -errno : -1);
        close(sockfd);
        return -1;
    }

    if (setsockopt(sockfd, SOL_SOCKET, SO_ATTACH_BPF, &program_fd, sizeof(program_fd))) {
        ebpf_dns_set_error(runtime, "open_capture_socket", "setsockopt(SO_ATTACH_BPF)", errno ? -errno : -1);
        close(sockfd);
        return -1;
    }

    if (setsockopt(sockfd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout))) {
        ebpf_dns_set_error(runtime, "open_capture_socket", "setsockopt(SO_RCVTIMEO)", errno ? -errno : -1);
        close(sockfd);
        return -1;
    }

    return sockfd;
}

static void ebpf_dns_collect_packets(
    int sockfd,
    netdata_dns_collector_t *collector,
    int capture_seconds,
    usec_t *stop_monotonic_ut,
    bool *cancelled)
{
    uint8_t packet[NETDATA_DNS_PACKET_BUFFER];
    usec_t end_ut = now_monotonic_usec() + ((usec_t)capture_seconds * USEC_PER_SEC);

    if (stop_monotonic_ut && *stop_monotonic_ut && *stop_monotonic_ut < end_ut)
        end_ut = *stop_monotonic_ut;

    while (now_monotonic_usec() < end_ut) {
        ssize_t received;
        uint64_t now_usec;

        if (cancelled && *cancelled)
            break;

        received = recv(sockfd, packet, sizeof(packet), 0);
        now_usec = now_monotonic_usec();

        ebpf_dns_expire_states(collector, now_usec);

        if (received <= 0) {
            if (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK)
                continue;

            break;
        }

        {
            netdata_dns_packet_t dns_packet;

            if (!ebpf_dns_parse_packet(packet, (size_t)received, &dns_packet))
                continue;

            if (!dns_packet.response)
                ebpf_dns_process_query(collector, &dns_packet, now_usec);
            else
                ebpf_dns_process_response(collector, &dns_packet, now_usec);
        }
    }

    ebpf_dns_expire_states(collector, now_monotonic_usec());
}

static int ebpf_dns_select_max_index(int is_rhf, uint32_t kver)
{
    if (is_rhf > 0) {
        if (kver >= NETDATA_EBPF_KERNEL_5_14)
            return NETDATA_IDX_V5_14;
        if (kver >= NETDATA_EBPF_KERNEL_5_4 && kver < NETDATA_EBPF_KERNEL_5_5)
            return NETDATA_IDX_V5_4;
        if (kver >= NETDATA_EBPF_KERNEL_4_11)
            return NETDATA_IDX_V4_18;
    } else {
        if (kver >= NETDATA_EBPF_KERNEL_6_8)
            return NETDATA_IDX_V6_8;
        if (kver >= NETDATA_EBPF_KERNEL_5_16)
            return NETDATA_IDX_V5_16;
        if (kver >= NETDATA_EBPF_KERNEL_5_15)
            return NETDATA_IDX_V5_15;
        if (kver >= NETDATA_EBPF_KERNEL_5_11)
            return NETDATA_IDX_V5_11;
        if (kver >= NETDATA_EBPF_KERNEL_5_10)
            return NETDATA_IDX_V5_10;
        if (kver >= NETDATA_EBPF_KERNEL_4_17)
            return NETDATA_IDX_V5_4;
        if (kver >= NETDATA_EBPF_KERNEL_4_15)
            return NETDATA_IDX_V4_16;
        if (kver >= NETDATA_EBPF_KERNEL_4_11)
            return NETDATA_IDX_V4_14;
    }

    return NETDATA_IDX_V3_10;
}

static const char *ebpf_dns_kernel_name(uint32_t selector)
{
    static const char *kernel_names[] = {
        NETDATA_IDX_STR_V3_10,
        NETDATA_IDX_STR_V4_14,
        NETDATA_IDX_STR_V4_16,
        NETDATA_IDX_STR_V4_18,
        NETDATA_IDX_STR_V5_4,
        NETDATA_IDX_STR_V5_10,
        NETDATA_IDX_STR_V5_11,
        NETDATA_IDX_STR_V5_14,
        NETDATA_IDX_STR_V5_15,
        NETDATA_IDX_STR_V5_16,
        NETDATA_IDX_STR_V6_8};

    return kernel_names[selector];
}

static uint32_t ebpf_dns_select_legacy_index(uint32_t kernels, int is_rhf, uint32_t kver)
{
    uint32_t start = ebpf_dns_select_max_index(is_rhf, kver);

    if (is_rhf == -1)
        kernels &= ~NETDATA_V5_14;

    for (uint32_t idx = start + 1; idx > 0; idx--) {
        uint32_t current = idx - 1;
        if (kernels & (1U << current))
            return current;
    }

    return NETDATA_IDX_V3_10;
}

static void ebpf_dns_mount_legacy_path(char *buffer, size_t length)
{
    uint32_t selector = ebpf_dns_select_legacy_index(NETDATA_DNS_LEGACY_KERNELS, isrh, running_on_kernel);

    snprintfz(
        buffer,
        length,
        "%s/ebpf.d/pnetdata_ebpf_dns.%s%s.o",
        ebpf_plugin_dir,
        ebpf_dns_kernel_name(selector),
        (isrh != -1) ? ".rhf" : "");
}

static int ebpf_dns_open_legacy(ebpf_dns_handle_t *handle, ebpf_dns_runtime_t *runtime)
{
    ebpf_dns_mount_legacy_path(handle->object_path, sizeof(handle->object_path));
    runtime->source = "legacy";
    snprintfz(runtime->object_path, sizeof(runtime->object_path), "%s", handle->object_path);

    handle->obj = bpf_object__open_file(handle->object_path, NULL);
    if (!handle->obj) {
        ebpf_dns_set_error(runtime, "open_legacy_object", "bpf_object__open_file", errno ? -errno : -1);
        return -1;
    }

    if (libbpf_get_error(handle->obj)) {
        int err = (int)libbpf_get_error(handle->obj);

        bpf_object__close(handle->obj);
        handle->obj = NULL;
        ebpf_dns_set_error(runtime, "open_legacy_object", "libbpf_get_error", err);
        return -1;
    }

    handle->source = "legacy";
    return 0;
}

static void ebpf_dns_close_handle(ebpf_dns_handle_t *handle)
{
#ifdef LIBBPF_MAJOR_VERSION
    if (handle->core) {
        dns_bpf__destroy(handle->core);
        handle->core = NULL;
        handle->obj = NULL;
        handle->source = NULL;
        return;
    }
#endif

    if (handle->obj) {
        bpf_object__close(handle->obj);
        handle->obj = NULL;
    }

    handle->source = NULL;
}

static int ebpf_dns_open_handle(ebpf_dns_handle_t *handle, ebpf_dns_runtime_t *runtime)
{
#ifdef LIBBPF_MAJOR_VERSION
    runtime->source = "CO-RE";
    handle->core = dns_bpf__open();
    if (handle->core) {
        handle->obj = handle->core->obj;
        handle->source = "CO-RE";
        runtime->source = handle->source;
        runtime->object_path[0] = '\0';
        return 0;
    }

    ebpf_dns_set_error(runtime, "open_core_object", "dns_bpf__open", errno ? -errno : -1);
#endif

    return ebpf_dns_open_legacy(handle, runtime);
}

static int ebpf_dns_capture(
    const ebpf_dns_config_t *cfg,
    netdata_dns_collector_t *collector,
    ebpf_dns_runtime_t *runtime,
    usec_t *stop_monotonic_ut,
    bool *cancelled)
{
    ebpf_dns_handle_t handle = { 0 };
    struct bpf_program *prog;
    int sockfd;

    if (ebpf_dns_open_handle(&handle, runtime))
        return -1;

    if (bpf_object__load(handle.obj)) {
        int err = errno ? -errno : -1;

#ifdef LIBBPF_MAJOR_VERSION
        if (handle.core) {
            ebpf_dns_close_handle(&handle);
            if (!ebpf_dns_open_legacy(&handle, runtime) && !bpf_object__load(handle.obj))
                goto dns_object_loaded;

            err = errno ? -errno : -1;
        }
#endif

        ebpf_dns_set_error(runtime, "load_object", "bpf_object__load", err);
        ebpf_dns_close_handle(&handle);
        return -1;
    }

dns_object_loaded:
    runtime->source = handle.source;
    if (handle.object_path[0])
        snprintfz(runtime->object_path, sizeof(runtime->object_path), "%s", handle.object_path);

    prog = ebpf_dns_find_socket_filter_program(handle.obj);
    if (!prog) {
        ebpf_dns_set_error(runtime, "find_socket_filter_program", "bpf_object__for_each_program", -ENOENT);
        ebpf_dns_close_handle(&handle);
        return -1;
    }

    if (ebpf_dns_configure_ports(handle.obj, cfg, runtime)) {
        ebpf_dns_close_handle(&handle);
        return -1;
    }

    sockfd = ebpf_dns_open_capture_socket(bpf_program__fd(prog), runtime);
    if (sockfd < 0) {
        ebpf_dns_close_handle(&handle);
        return -1;
    }

    ebpf_dns_collect_packets(sockfd, collector, cfg->iterations * NETDATA_DNS_CAPTURE_INTERVAL, stop_monotonic_ut, cancelled);

    close(sockfd);
    ebpf_dns_close_handle(&handle);
    return 0;
}

static void ebpf_dns_write_columns(BUFFER *wb)
{
    int fields_id = 0;

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Domain",
        "Domain",
        RRDF_FIELD_TYPE_STRING,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Type",
        "Query Type",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Protocol",
        "Transport Protocol",
        RRDF_FIELD_TYPE_STRING,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Server",
        "Server IP",
        RRDF_FIELD_TYPE_STRING,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Client",
        "Client IP",
        RRDF_FIELD_TYPE_STRING,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "ClientPort",
        "Client Port",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Responses",
        "Responses",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        "responses",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_SUM,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Success",
        "Successful Responses",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        "responses",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_SUM,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Failure",
        "Failed Responses",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        "responses",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_SUM,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "Timeouts",
        "Timed Out Queries",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        0,
        "queries",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_SUM,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "AvgSuccessMs",
        "Average Success Latency",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        3,
        "ms",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_MEAN,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id++,
        "AvgFailureMs",
        "Average Failure Latency",
        RRDF_FIELD_TYPE_INTEGER,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER,
        3,
        "ms",
        NAN,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_MEAN,
        RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);

    buffer_rrdf_table_add_field(
        wb,
        fields_id,
        "Rcodes",
        "Response Codes",
        RRDF_FIELD_TYPE_DETAIL_STRING,
        RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE,
        0,
        NULL,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH,
        NULL);
}

static void ebpf_dns_write_charts(BUFFER *wb)
{
    buffer_json_member_add_object(wb, "Response Mix");
    {
        buffer_json_member_add_string(wb, "name", "Response Mix");
        buffer_json_member_add_string(wb, "type", "stacked-bar");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, "Success");
            buffer_json_add_array_item_string(wb, "Failure");
            buffer_json_add_array_item_string(wb, "Timeouts");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "Latency");
    {
        buffer_json_member_add_string(wb, "name", "Latency");
        buffer_json_member_add_string(wb, "type", "bar");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, "AvgSuccessMs");
            buffer_json_add_array_item_string(wb, "AvgFailureMs");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void ebpf_dns_write_group_by(BUFFER *wb)
{
    buffer_json_member_add_object(wb, "Domain");
    {
        buffer_json_member_add_string(wb, "name", "Domain");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, "Domain");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "Server");
    {
        buffer_json_member_add_string(wb, "name", "Server");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, "Server");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "Client");
    {
        buffer_json_member_add_string(wb, "name", "Client");
        buffer_json_member_add_array(wb, "columns");
        {
            buffer_json_add_array_item_string(wb, "Client");
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
}

static void ebpf_function_dns_manipulation(
    const char *transaction,
    char *function,
    usec_t *stop_monotonic_ut,
    bool *cancelled,
    BUFFER *payload __maybe_unused,
    HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused,
    void *data __maybe_unused)
{
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    size_t num_words = quoted_strings_splitter_whitespace(function, words, PLUGINSD_MAX_WORDS);
    ebpf_dns_config_t cfg = { 0 };
    ebpf_dns_runtime_t runtime = { .source = "unknown", .stage = "initializing", .operation = "initialize" };
    bool info = false;
    char error_message[256];
    time_t now_s = now_realtime_sec();

    static const char *dns_help = {
        "ebpf.plugin / dns\n"
        "\n"
        "Function `network-dns-tracing` captures DNS packets on demand with an eBPF socket filter.\n"
        "Each iteration captures 5 seconds of DNS traffic and returns an RRDF table summarizing observed queries.\n"
        "\n"
        "The following filters are supported:\n"
        "\n"
        "   port:PORT[,PORT...]\n"
        "      Comma separated list of DNS ports to monitor. Default is 53.\n"
        "\n"
        "   iteration:COUNT\n"
        "      Number of 5-second capture windows to run. Default is 1.\n"
        "\n"
        "   help\n"
        "      Show this message.\n"
        "\n"
        "   info\n"
        "      Return metadata without starting a capture.\n"};

    ebpf_dns_reset_config(&cfg);

    for (int i = 1; i < PLUGINSD_MAX_WORDS; i++) {
        const char *keyword = get_word(words, num_words, i);
        const char *value;

        if (!keyword)
            break;

        if (strncmp(keyword, EBPF_FUNCTION_DNS_PORT, sizeof(EBPF_FUNCTION_DNS_PORT) - 1) == 0) {
            value = &keyword[sizeof(EBPF_FUNCTION_DNS_PORT) - 1];
            if (ebpf_dns_parse_port_list(&cfg, value, error_message, sizeof(error_message))) {
                ebpf_function_error(transaction, HTTP_RESP_BAD_REQUEST, error_message);
                return;
            }
        } else if (strncmp(keyword, EBPF_FUNCTION_DNS_ITERATION, sizeof(EBPF_FUNCTION_DNS_ITERATION) - 1) == 0) {
            value = &keyword[sizeof(EBPF_FUNCTION_DNS_ITERATION) - 1];
            if (ebpf_dns_parse_iterations(&cfg, value, error_message, sizeof(error_message))) {
                ebpf_function_error(transaction, HTTP_RESP_BAD_REQUEST, error_message);
                return;
            }
        } else if (
            strncmp(keyword, "after:", sizeof("after:") - 1) == 0 ||
            strncmp(keyword, "before:", sizeof("before:") - 1) == 0 ||
            strncmp(keyword, "direction:", sizeof("direction:") - 1) == 0 ||
            strncmp(keyword, "last:", sizeof("last:") - 1) == 0 ||
            strncmp(keyword, "anchor:", sizeof("anchor:") - 1) == 0) {
            // Ignore common FUNCTION UI navigation arguments for this on-demand table.
            continue;
        } else if (strncmp(keyword, "help", 4) == 0) {
            ebpf_function_help(transaction, dns_help);
            return;
        } else if (strncmp(keyword, "info", 4) == 0) {
            info = true;
        } else {
            snprintfz(error_message, sizeof(error_message), "Unsupported DNS argument: %s", keyword);
            ebpf_function_error(transaction, HTTP_RESP_BAD_REQUEST, error_message);
            return;
        }
    }

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", NETDATA_DNS_CAPTURE_INTERVAL);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_DNS_FUNCTION_DESCRIPTION);
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "port");
        buffer_json_add_array_item_string(wb, "iteration");
        buffer_json_add_array_item_string(wb, "info");
        buffer_json_add_array_item_string(wb, "after");
        buffer_json_add_array_item_string(wb, "before");
        buffer_json_add_array_item_string(wb, "direction");
        buffer_json_add_array_item_string(wb, "last");
    }
    buffer_json_array_close(wb);
    buffer_json_member_add_array(wb, "required_params");
    buffer_json_array_close(wb);
    buffer_json_member_add_uint64(wb, "iterations", cfg.iterations);
    buffer_json_member_add_uint64(wb, "capture_seconds", cfg.iterations * NETDATA_DNS_CAPTURE_INTERVAL);

    buffer_json_member_add_array(wb, "ports");
    for (size_t i = 0; i < cfg.port_count; i++)
        buffer_json_add_array_item_uint64(wb, cfg.ports[i]);
    buffer_json_array_close(wb);

    if (!info) {
        netdata_dns_collector_t collector = { 0 };

        if (ebpf_dns_capture(&cfg, &collector, &runtime, stop_monotonic_ut, cancelled)) {
            snprintfz(
                error_message,
                sizeof(error_message),
                "DNS tracing failed via %s at %s/%s (%d)%s%s%s",
                runtime.source ? runtime.source : "unknown",
                runtime.stage ? runtime.stage : "unknown",
                runtime.operation ? runtime.operation : "unknown",
                runtime.error_code,
                runtime.object_path[0] ? " object=" : "",
                runtime.object_path[0] ? runtime.object_path : "",
                "");
            buffer_free(wb);
            ebpf_function_error(transaction, HTTP_RESP_INTERNAL_SERVER_ERROR, error_message);
            return;
        }

        buffer_json_member_add_string(wb, "loader", runtime.source ? runtime.source : "unknown");

        buffer_json_member_add_array(wb, "data");
        ebpf_dns_fill_data_array(wb, &collector);
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "columns");
        ebpf_dns_write_columns(wb);
        buffer_json_object_close(wb);

        buffer_json_member_add_string(wb, "default_sort_column", "Responses");

        buffer_json_member_add_object(wb, "charts");
        ebpf_dns_write_charts(wb);
        buffer_json_object_close(wb);

        buffer_json_member_add_array(wb, "default_charts");
        {
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, "Response Mix");
            buffer_json_add_array_item_string(wb, "Domain");
            buffer_json_array_close(wb);

            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, "Latency");
            buffer_json_add_array_item_string(wb, "Domain");
            buffer_json_array_close(wb);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "group_by");
        ebpf_dns_write_group_by(wb);
        buffer_json_object_close(wb);

        ebpf_dns_free_collector(&collector);
    } else {
        buffer_json_member_add_object(wb, "columns");
        ebpf_dns_write_columns(wb);
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_time_t(wb, "expires", now_s + NETDATA_DNS_CAPTURE_INTERVAL);
    buffer_json_finalize(wb);

    pluginsd_function_result_begin_to_stdout(
        transaction, HTTP_RESP_OK, "application/json", now_s + NETDATA_DNS_CAPTURE_INTERVAL);
    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);

    buffer_free(wb);
}

/*****************************************************************
 *  EBPF SOCKET FUNCTION
 *****************************************************************/

/**
 * Fill Fake socket
 *
 * Fill socket with an invalid request.
 *
 * @param fake_values is the structure where we are storing the value.
 */
static inline void ebpf_socket_fill_fake_socket(netdata_socket_plus_t *fake_values)
{
    snprintfz(fake_values->socket_string.src_ip, INET6_ADDRSTRLEN, "%s", "127.0.0.1");
    snprintfz(fake_values->socket_string.dst_ip, INET6_ADDRSTRLEN, "%s", "127.0.0.1");
    fake_values->pid = getpid();
    //fake_values->socket_string.src_port = 0;
    fake_values->socket_string.dst_port[0] = 0;
    snprintfz(fake_values->socket_string.dst_ip, NI_MAXSERV, "%s", "none");
    fake_values->data.family = AF_INET;
    fake_values->data.protocol = AF_UNSPEC;
}

static NETDATA_DOUBLE bytes_to_mb(uint64_t bytes)
{
    return (NETDATA_DOUBLE)bytes / (1024 * 1024);
}

/**
 * Fill function buffer
 *
 * Fill buffer with data to be shown on cloud.
 *
 * @param wb          buffer where we store data.
 * @param values      data read from hash table
 * @param name        the process name
 */
static void ebpf_fill_function_buffer(BUFFER *wb, netdata_socket_plus_t *values, char *name)
{
    buffer_json_add_array_item_array(wb);

    // IMPORTANT!
    // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

    // PID
    buffer_json_add_array_item_uint64(wb, (uint64_t)values->pid);

    // NAME
    if (!values->data.name[0])
        buffer_json_add_array_item_string(wb, (name) ? name : "unknown");
    else
        buffer_json_add_array_item_string(wb, values->data.name);

    // Origin
    buffer_json_add_array_item_string(wb, (values->data.external_origin) ? "in" : "out");

    // Source IP
    buffer_json_add_array_item_string(wb, values->socket_string.src_ip);

    // SRC Port
    //buffer_json_add_array_item_uint64(wb, (uint64_t) values->socket_string.src_port);

    // Destination IP
    buffer_json_add_array_item_string(wb, values->socket_string.dst_ip);

    // DST Port
    buffer_json_add_array_item_string(wb, values->socket_string.dst_port);

    uint64_t connections;
    if (values->data.protocol == IPPROTO_TCP) {
        buffer_json_add_array_item_string(wb, "TCP");
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.tcp.tcp_bytes_received));
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.tcp.tcp_bytes_sent));
        connections = values->data.tcp.ipv4_connect + values->data.tcp.ipv6_connect;
    } else if (values->data.protocol == IPPROTO_UDP) {
        buffer_json_add_array_item_string(wb, "UDP");
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.udp.udp_bytes_received));
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.udp.udp_bytes_sent));
        connections = values->data.udp.call_udp_sent + values->data.udp.call_udp_received;
    } else {
        buffer_json_add_array_item_string(wb, "UNSPEC");
        buffer_json_add_array_item_double(wb, 0);
        buffer_json_add_array_item_double(wb, 0);
        connections = 1;
    }

    // Connections
    if (values->flags & NETDATA_SOCKET_FLAGS_ALREADY_OPEN) {
        connections++;
    } else if (!connections) {
        // If no connections, this means that we lost when connection was opened
        values->flags |= NETDATA_SOCKET_FLAGS_ALREADY_OPEN;
        connections++;
    }
    buffer_json_add_array_item_uint64(wb, connections);

    buffer_json_array_close(wb);
}

/**
 * Clean Judy array unsafe
 *
 * Clean all Judy Array allocated to show table when a function is called.
 * Before to call this function it is necessary to lock `ebpf_judy_pid.index.rw_spinlock`.
 **/
static void ebpf_socket_clean_judy_array_unsafe()
{
    if (!ebpf_judy_pid.index.JudyLArray)
        return;

    Pvoid_t *pid_value, *socket_value;
    Word_t local_pid = 0, local_socket = 0;
    bool first_pid = true, first_socket = true;
    while ((pid_value = JudyLFirstThenNext(ebpf_judy_pid.index.JudyLArray, &local_pid, &first_pid))) {
        netdata_ebpf_judy_pid_stats_t *pid_ptr = (netdata_ebpf_judy_pid_stats_t *)*pid_value;
        rw_spinlock_write_lock(&pid_ptr->socket_stats.rw_spinlock);
        if (pid_ptr->socket_stats.JudyLArray) {
            while (
                (socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_socket, &first_socket))) {
                netdata_socket_plus_t *socket_clean = *socket_value;
                aral_freez(aral_socket_table, socket_clean);
            }
            JudyLFreeArray(&pid_ptr->socket_stats.JudyLArray, PJE0);
            pid_ptr->socket_stats.JudyLArray = NULL;
        }
        rw_spinlock_write_unlock(&pid_ptr->socket_stats.rw_spinlock);
    }
}

/**
 * Fill function buffer unsafe
 *
 * Fill the function buffer with socket information. Before to call this function it is necessary to lock
 * ebpf_judy_pid.index.rw_spinlock
 *
 * @param buf    buffer used to store data to be shown by function.
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static void ebpf_socket_fill_function_buffer_unsafe(BUFFER *buf)
{
    int counter = 0;

    Pvoid_t *pid_value, *socket_value;
    Word_t local_pid = 0;
    bool first_pid = true;
    while ((pid_value = JudyLFirstThenNext(ebpf_judy_pid.index.JudyLArray, &local_pid, &first_pid))) {
        netdata_ebpf_judy_pid_stats_t *pid_ptr = (netdata_ebpf_judy_pid_stats_t *)*pid_value;
        bool first_socket = true;
        Word_t local_timestamp = 0;
        rw_spinlock_read_lock(&pid_ptr->socket_stats.rw_spinlock);
        if (pid_ptr->socket_stats.JudyLArray) {
            while ((
                socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_timestamp, &first_socket))) {
                netdata_socket_plus_t *values = (netdata_socket_plus_t *)*socket_value;
                ebpf_fill_function_buffer(buf, values, pid_ptr->cmdline);
            }
            counter++;
        }
        rw_spinlock_read_unlock(&pid_ptr->socket_stats.rw_spinlock);
    }

    if (!counter) {
        netdata_socket_plus_t fake_values = {};
        ebpf_socket_fill_fake_socket(&fake_values);
        ebpf_fill_function_buffer(buf, &fake_values, NULL);
    }
}

/**
 * Socket read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data on very busy socket.
 *
 * @param buf the buffer to store data;
 * @param em  the module main structure.
 *
 * @return It always returns NULL.
 */
void ebpf_socket_read_open_connections(BUFFER *buf, struct ebpf_module *em)
{
    // thread was not initialized or Array was reset
    rw_spinlock_read_lock(&ebpf_judy_pid.index.rw_spinlock);
    if (!em->maps || (em->maps[NETDATA_SOCKET_OPEN_SOCKET].map_fd == ND_EBPF_MAP_FD_NOT_INITIALIZED) ||
        !ebpf_judy_pid.index.JudyLArray) {
        netdata_socket_plus_t fake_values = {};

        ebpf_socket_fill_fake_socket(&fake_values);

        ebpf_fill_function_buffer(buf, &fake_values, NULL);
        rw_spinlock_read_unlock(&ebpf_judy_pid.index.rw_spinlock);
        return;
    }

    rw_spinlock_read_lock(&network_viewer_opt.rw_spinlock);
    ebpf_socket_fill_function_buffer_unsafe(buf);
    rw_spinlock_read_unlock(&network_viewer_opt.rw_spinlock);
    rw_spinlock_read_unlock(&ebpf_judy_pid.index.rw_spinlock);
}

/**
 * Function: Socket
 *
 * Show information for sockets stored in hash tables.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param function     function name and arguments given to thread.
 * @param timeout      The function timeout
 * @param cancelled    Variable used to store function status.
 */
static void ebpf_function_socket_manipulation(
    const char *transaction,
    char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused,
    bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused,
    HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused,
    void *data __maybe_unused)
{
    ebpf_module_t *em = &ebpf_modules[EBPF_MODULE_SOCKET_IDX];

    char *words[PLUGINSD_MAX_WORDS] = {NULL};
    size_t num_words = quoted_strings_splitter_whitespace(function, words, PLUGINSD_MAX_WORDS);
    const char *name;
    int period = -1;
    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    network_viewer_opt.enabled = CONFIG_BOOLEAN_YES;
    uint32_t previous;
    bool info = false;
    time_t now_s = now_realtime_sec();

    static const char *socket_help = {
        "ebpf.plugin / socket\n"
        "\n"
        "Function `socket` display information for all open sockets during ebpf.plugin runtime.\n"
        "During thread runtime the plugin is always collecting data, but when an option is modified, the plugin\n"
        "resets completely the previous table and can show a clean data for the first request before to bring the\n"
        "modified request.\n"
        "\n"
        "The following filters are supported:\n"
        "\n"
        "   family:FAMILY\n"
        "      Shows information for the FAMILY specified. Option accepts IPV4, IPV6 and all, that is the default.\n"
        "\n"
        "   period:PERIOD\n"
        "      Enable socket to run a specific PERIOD in seconds. When PERIOD is not\n"
        "      specified plugin will use the default 300 seconds\n"
        "\n"
        "   resolve:BOOL\n"
        "      Resolve service name, default value is YES.\n"
        "\n"
        "   range:CIDR\n"
        "      Show sockets that have only a specific destination. Default all addresses.\n"
        "\n"
        "   port:range\n"
        "      Show sockets that have only a specific destination.\n"
        "\n"
        "   reset\n"
        "      Send a reset to collector. When a collector receives this command, it uses everything defined in configuration file.\n"
        "\n"
        "   interfaces\n"
        "      When the collector receives this command, it read all available interfaces on host.\n"
        "\n"
        "Filters can be combined. Each filter can be given only one time. Default all ports\n"};

    for (int i = 1; i < PLUGINSD_MAX_WORDS; i++) {
        const char *keyword = get_word(words, num_words, i);
        if (!keyword)
            break;

        if (strncmp(keyword, EBPF_FUNCTION_SOCKET_FAMILY, sizeof(EBPF_FUNCTION_SOCKET_FAMILY) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_FAMILY) - 1];
            previous = network_viewer_opt.family;
            uint32_t family = AF_UNSPEC;
            if (!strcmp(name, "IPV4"))
                family = AF_INET;
            else if (!strcmp(name, "IPV6"))
                family = AF_INET6;

            if (family != previous) {
                rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
                network_viewer_opt.family = family;
                rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
                ebpf_socket_clean_judy_array_unsafe();
            }
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_PERIOD, sizeof(EBPF_FUNCTION_SOCKET_PERIOD) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_PERIOD) - 1];
            netdata_mutex_lock(&ebpf_exit_cleanup);
            period = str2i(name);
            if (period > 0) {
                em->lifetime = period;
            } else
                em->lifetime = EBPF_NON_FUNCTION_LIFE_TIME;

#ifdef NETDATA_DEV_MODE
            collector_info("Lifetime modified for %u", em->lifetime);
#endif
            netdata_mutex_unlock(&ebpf_exit_cleanup);
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RESOLVE, sizeof(EBPF_FUNCTION_SOCKET_RESOLVE) - 1) == 0) {
            previous = network_viewer_opt.service_resolution_enabled;
            uint32_t resolution;
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_RESOLVE) - 1];
            resolution = (!strcasecmp(name, "YES")) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

            if (previous != resolution) {
                rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
                network_viewer_opt.service_resolution_enabled = resolution;
                rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

                ebpf_socket_clean_judy_array_unsafe();
            }
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RANGE, sizeof(EBPF_FUNCTION_SOCKET_RANGE) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_RANGE) - 1];
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_ip_structure(&network_viewer_opt.included_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.excluded_ips);
            ebpf_parse_ips_unsafe((char *)name);
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

            ebpf_socket_clean_judy_array_unsafe();
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_PORT, sizeof(EBPF_FUNCTION_SOCKET_PORT) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_PORT) - 1];
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_port_structure(&network_viewer_opt.included_port);
            ebpf_clean_port_structure(&network_viewer_opt.excluded_port);
            ebpf_parse_ports((char *)name);
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

            ebpf_socket_clean_judy_array_unsafe();
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RESET, sizeof(EBPF_FUNCTION_SOCKET_RESET) - 1) == 0) {
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_port_structure(&network_viewer_opt.included_port);
            ebpf_clean_port_structure(&network_viewer_opt.excluded_port);

            ebpf_clean_ip_structure(&network_viewer_opt.included_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.excluded_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.ipv4_local_ip);
            ebpf_clean_ip_structure(&network_viewer_opt.ipv6_local_ip);

            parse_network_viewer_section(&socket_config);
            ebpf_read_local_addresses_unsafe();
            network_viewer_opt.enabled = CONFIG_BOOLEAN_YES;
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_INTERFACES, sizeof(EBPF_FUNCTION_SOCKET_INTERFACES) - 1) == 0) {
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_read_local_addresses_unsafe();
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
        } else if (strncmp(keyword, "help", 4) == 0) {
            ebpf_function_help(transaction, socket_help);
            rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);
            return;
        } else if (strncmp(keyword, "info", 4) == 0)
            info = true;
    }
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    if (ebpf_module_enabled_get(em) > NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        // Cleanup when we already had a thread running
        rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
        ebpf_socket_clean_judy_array_unsafe();
        rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

        collect_pids |= 1 << EBPF_MODULE_SOCKET_IDX;
        if (ebpf_function_start_thread(em, period)) {
            ebpf_function_error(transaction, HTTP_RESP_INTERNAL_SERVER_ERROR, "Cannot start thread.");
            return;
        }
    } else {
        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (period < 0)
            em->lifetime = (ebpf_module_enabled_get(em) != NETDATA_THREAD_EBPF_FUNCTION_RUNNING) ?
                               EBPF_NON_FUNCTION_LIFE_TIME :
                               EBPF_DEFAULT_LIFETIME;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", em->update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION);

    if (info)
        goto close_and_send;

    // Collect data
    buffer_json_member_add_array(wb, "data");
    ebpf_socket_read_open_connections(wb, em);
    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        int fields_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "PID",
            "Process ID",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Name",
            "Process Name",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Origin",
            "Connection Origin",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Src",
            "Source IP Address",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        /*
        buffer_rrdf_table_add_field(wb, fields_id++, "SrcPort", "Source Port", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);
                                    */

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Dst",
            "Destination IP Address",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "DstPort",
            "Destination Port",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Protocol",
            "Transport Layer Protocol",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_NONE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Rcvd",
            "Traffic Received",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            3,
            "MB",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Sent",
            "Traffic Sent",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            3,
            "MB",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id,
            "Conns",
            "Connections",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            0,
            "connections",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Rcvd");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Rcvd");
                buffer_json_add_array_item_string(wb, "Sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Connections");
        {
            buffer_json_member_add_string(wb, "name", "Connections");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Conns");
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
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Connections");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Name");
        {
            buffer_json_member_add_string(wb, "name", "Process Name");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Name");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Origin");
        {
            buffer_json_member_add_string(wb, "name", "Origin");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Origin");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Src");
        {
            buffer_json_member_add_string(wb, "name", "Source IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Src");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Dst");
        {
            buffer_json_member_add_string(wb, "name", "Destination IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Dst");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "DstPort");
        {
            buffer_json_member_add_string(wb, "name", "Destination Port");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "DstPort");
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
    }
    buffer_json_object_close(wb); // group_by

close_and_send:
    buffer_json_member_add_time_t(wb, "expires", now_s + em->update_every);
    buffer_json_finalize(wb);

    // Lock necessary to avoid race condition
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_s + em->update_every);

    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);

    pluginsd_function_result_end_to_stdout();
    fflush(stdout);

    buffer_free(wb);
}

/*****************************************************************
 *  EBPF FUNCTION THREAD
 *****************************************************************/

/**
 * FUNCTION thread.
 *
 * @param ptr a `ebpf_module_t *`.
 *
 * @return always NULL.
 */
void ebpf_function_thread(void *ptr)
{
    (void)ptr;

    struct functions_evloop_globals *wg = functions_evloop_init(1, "EBPF", &lock, &ebpf_plugin_exit, NULL);

    functions_evloop_add_function(
        wg, EBPF_FUNCTION_DNS, ebpf_function_dns_manipulation, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);
    functions_evloop_add_function(
        wg, EBPF_FUNCTION_SOCKET, ebpf_function_socket_manipulation, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    netdata_mutex_lock(&lock);
    EBPF_PLUGIN_FUNCTIONS(EBPF_FUNCTION_DNS, EBPF_PLUGIN_DNS_FUNCTION_DESCRIPTION, NETDATA_DNS_CAPTURE_INTERVAL);
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *em = &ebpf_modules[i];
        if (!em->functions.fnct_routine)
            continue;

        EBPF_PLUGIN_FUNCTIONS(em->functions.fcnt_name, em->functions.fcnt_desc, em->update_every);
    }
    netdata_mutex_unlock(&lock);

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop()) {
        if (ebpf_plugin_stop()) {
            break;
        }

        heartbeat_next(&hb);

        if (ebpf_plugin_stop()) {
            break;
        }
    }
}
