//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <arpa/inet.h>
#include <errno.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

/*
 * ring_buffer API was added in libbpf 0.2 (LIBBPF_MAJOR_VERSION defined).
 * Provide a compile-time gate so the base flavor still compiles without it.
 */
#ifdef LIBBPF_MAJOR_VERSION
#  define DNS_HAS_RINGBUF 1
#else
#  define DNS_HAS_RINGBUF 0
#endif

/* -------------------------------------------------------------------------
 * Protocol / Ethernet constants (manual definitions avoid linux/ header issues)
 * ---------------------------------------------------------------------- */
#ifndef AF_PACKET
#  define AF_PACKET 17
#endif
#ifndef SOCK_RAW
#  define SOCK_RAW 3
#endif
#ifndef SOL_SOCKET
#  define SOL_SOCKET 1
#endif
#ifndef SO_ATTACH_BPF
#  define SO_ATTACH_BPF 50
#endif
/* Classic BPF socket filter (cBPF), distinct from eBPF SO_ATTACH_BPF */
#ifndef SO_ATTACH_FILTER
#  define SO_ATTACH_FILTER 26
#endif

/* Ethernet protocol IDs (network byte order values) */
#define DNS_ETH_P_ALL   0x0003u
#define DNS_ETH_P_IP    0x0800u
#define DNS_ETH_P_IPV6  0x86DDu
#define DNS_ETH_P_8021Q 0x8100u
#define DNS_ETH_P_8021AD 0x88A8u

/* IP protocol numbers */
#define DNS_IPPROTO_UDP 17u
#define DNS_IPPROTO_TCP  6u

/* Direction constants extracted from BPF bytecode (dns_buffer.skel.h):
 *   insn [191] MOV r1 = 1  (NETDATA_DNS_QUERY)
 *   insn [193] MOV r1 = 0  (NETDATA_DNS_RESPONSE)
 * so QUERY = 1, RESPONSE = 0. */
#define DNS_DIRECTION_QUERY    1u
#define DNS_DIRECTION_RESPONSE 0u

/* DNS ports to monitor (decision 2-B: 53 standard + 5353 mDNS). */
static const uint16_t dns_monitored_ports[] = {53, 5353};
#define DNS_PORT_COUNT (sizeof(dns_monitored_ports) / sizeof(dns_monitored_ports[0]))

/* Portable classic-BPF structs — avoids depending on <linux/filter.h> which
 * is not always in the CGo include path. Uses a unique prefix to prevent
 * collisions if a kernel header defining struct sock_filter is in scope. */
struct netdata_sock_filter { uint16_t code; uint8_t jt; uint8_t jf; uint32_t k; };
/* Mirror the kernel's sock_fprog layout without importing <linux/filter.h>.
 * Natural alignment produces the correct padding on every architecture:
 *   64-bit: 2-byte len + 6-byte pad + 8-byte ptr = 16 bytes
 *   32-bit: 2-byte len + 2-byte pad + 4-byte ptr =  8 bytes */
struct netdata_sock_fprog {
    uint16_t                   len;
    struct netdata_sock_filter *filter;
};

/* -------------------------------------------------------------------------
 * Per-query tracking limits
 * ---------------------------------------------------------------------- */
#define DNS_PENDING_CAP         512         /* max concurrent in-flight queries  */
#define DNS_PENDING_TIMEOUT_US  5000000ULL  /* 5 s: unmatched query → timeout    */
#define DNS_FLOW_RING_CAP       1000        /* ring capacity — matches SHM       */
#define DNS_FLOW_TTL_US         (20ULL * 1000000ULL) /* 20 s live window         */
#define DNS_DOMAIN_MAX          256
#define DNS_PACKET_BUF          65536

/* -------------------------------------------------------------------------
 * Shared flow record type — identical layout to ebpfgo_dns_flow_record in
 * apps_ebpf_shared_dns_row.h so the Go layer can copy fields 1:1.
 * ---------------------------------------------------------------------- */
struct netdata_dns_flow_record {
    uint64_t timestamp_us;
    uint64_t latency_us;
    uint32_t server_ip[4];
    uint32_t client_ip[4];
    char     domain[DNS_DOMAIN_MAX];
    uint16_t client_port;
    uint16_t query_type;
    uint16_t rcode;
    uint8_t  protocol;
    uint8_t  ip_version;
    uint8_t  timed_out;
    uint8_t  _pad[7];   /* explicit pad → sizeof == 320 */
};

/* Pending (in-flight) query slot */
struct netdata_dns_pending {
    uint64_t timestamp_us;
    uint32_t server_ip[4];
    uint32_t client_ip[4];
    char     domain[DNS_DOMAIN_MAX];
    uint16_t tx_id;
    uint16_t query_type;
    uint16_t client_port;
    uint8_t  protocol;
    uint8_t  ip_version;
    uint8_t  in_use;
    uint8_t  _pad[3];
};

/* Circular ring of completed flow records */
struct netdata_dns_flow_ring {
    struct netdata_dns_flow_record records[DNS_FLOW_RING_CAP];
    uint32_t head;   /* monotonically increasing next-write position */
};

/* -------------------------------------------------------------------------
 * Runtime types
 * ---------------------------------------------------------------------- */
struct netdata_dns_snapshot {
    uint64_t queries_udp_ipv4;
    uint64_t queries_udp_ipv6;
    uint64_t queries_tcp_ipv4;
    uint64_t queries_tcp_ipv6;
    uint64_t responses_udp_ipv4;
    uint64_t responses_udp_ipv6;
    uint64_t responses_tcp_ipv4;
    uint64_t responses_tcp_ipv6;
};

enum netdata_dns_runtime_kind {
    DNS_RUNTIME_BASE    = 0,
    DNS_RUNTIME_RINGBUF = 1,
};

/* netdata_dns_event_t mirrors the BPF event layout confirmed from BPF bytecode:
 *   insn [180] MOV r2 = 56  → bpf_ringbuf_reserve size = 56 bytes */
struct netdata_dns_event_t {
    uint64_t ct;
    uint32_t saddr[4];
    uint32_t daddr[4];
    uint32_t pkt_len;
    uint16_t sport;
    uint16_t dport;
    uint8_t  protocol;
    uint8_t  ip_version;
    uint8_t  direction;
    uint8_t  pad;
    uint32_t _align_pad;
};

struct netdata_dns_runtime {
    int kind;
    struct bpf_object *obj;
    int sock_fd;   /* BPF-attached socket (base: carries packets; ringbuf: empty) */
    int flow_fd;   /* dedicated AF_PACKET + classic-BPF socket (ringbuf mode only) */
    int per_query; /* when false, skip the flow socket and per-query tracking */
#if DNS_HAS_RINGBUF
    struct ring_buffer *rb;
#else
    void *rb;
#endif
    struct netdata_dns_snapshot   acc;
    struct netdata_dns_pending    pending[DNS_PENDING_CAP];
    struct netdata_dns_flow_ring  flows;
};

/* -------------------------------------------------------------------------
 * Shared counter accumulator
 * ---------------------------------------------------------------------- */
static void dns_acc_event(struct netdata_dns_snapshot *a,
                          int is_query, int is_udp, int is_ipv4)
{
    if (is_query) {
        if      (is_udp &&  is_ipv4) a->queries_udp_ipv4++;
        else if (is_udp && !is_ipv4) a->queries_udp_ipv6++;
        else if (!is_udp &&  is_ipv4) a->queries_tcp_ipv4++;
        else                          a->queries_tcp_ipv6++;
    } else {
        if      (is_udp &&  is_ipv4) a->responses_udp_ipv4++;
        else if (is_udp && !is_ipv4) a->responses_udp_ipv6++;
        else if (!is_udp &&  is_ipv4) a->responses_tcp_ipv4++;
        else                          a->responses_tcp_ipv6++;
    }
}

/* -------------------------------------------------------------------------
 * Ring buffer callback (buffer / arena variants only)
 * ---------------------------------------------------------------------- */
#if DNS_HAS_RINGBUF
static int dns_rb_callback(void *ctx, void *data, size_t data_sz)
{
    if (data_sz < offsetof(struct netdata_dns_event_t, direction) + 1)
        return 0;

    struct netdata_dns_runtime *rt = ctx;
    const struct netdata_dns_event_t *ev = data;

    dns_acc_event(&rt->acc,
                  ev->direction == DNS_DIRECTION_QUERY,
                  ev->protocol  == DNS_IPPROTO_UDP,
                  ev->ip_version == 4);
    return 0;
}
#endif /* DNS_HAS_RINGBUF */

/* -------------------------------------------------------------------------
 * Port helpers
 * ---------------------------------------------------------------------- */
static int dns_is_monitored_port(uint16_t port)
{
    for (size_t i = 0; i < DNS_PORT_COUNT; i++) {
        if (port == dns_monitored_ports[i])
            return 1;
    }
    return 0;
}

static uint16_t dns_read_u16be(const char *buf, int off)
{
    return (uint16_t)(((uint8_t)buf[off] << 8) | (uint8_t)buf[off + 1]);
}

/* -------------------------------------------------------------------------
 * Time helpers
 * ---------------------------------------------------------------------- */
static uint64_t dns_now_us(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)(ts.tv_nsec / 1000);
}

/* -------------------------------------------------------------------------
 * DNS name decompression (ported from kernel-collector/gotests/dns.go:dnsReadName)
 *
 * Reads from msg[0..msg_len-1] starting at offset off.
 * Fills out[0..max_out-1] (NUL-terminated).
 * Returns bytes consumed at the START offset (before any pointer jumps),
 * or 0 on parse error.
 * ---------------------------------------------------------------------- */
static int dns_read_name(const char *msg, int msg_len, int off,
                         char *out, int max_out)
{
    int  current   = off;
    int  out_len   = 0;
    int  jumps     = 0;
    int  first_end = -1;
    bool jumped    = false;

    while (current < msg_len && jumps < 32) {
        uint8_t label = (uint8_t)msg[current];

        if ((label & 0xC0u) == 0xC0u) {
            /* Pointer: 2-byte offset into the message */
            if (current + 1 >= msg_len)
                return 0;
            if (!jumped)
                first_end = current + 2;
            int ptr = ((label & 0x3Fu) << 8) | (uint8_t)msg[current + 1];
            current = ptr;
            jumped  = true;
            jumps++;
            continue;
        }

        current++;

        if (label == 0) {
            /* Root label — end of name */
            if (!jumped)
                first_end = current;
            out[out_len] = '\0';
            return (first_end >= 0) ? (first_end - off) : (current - off);
        }

        if (label > 63 || current + (int)label > msg_len)
            return 0;

        if (out_len > 0) {
            if (out_len + 1 >= max_out)
                return 0;
            out[out_len++] = '.';
        }

        if (out_len + (int)label >= max_out)
            return 0;

        for (int i = 0; i < (int)label; i++) {
            uint8_t ch = (uint8_t)msg[current + i];
            if (ch >= 'A' && ch <= 'Z')
                ch = (uint8_t)(ch - 'A' + 'a');
            out[out_len++] = (char)ch;
        }

        current += (int)label;
        jumps++;
    }

    return 0;
}

/* -------------------------------------------------------------------------
 * DNS payload parser
 *
 * buf[0..n-1] is the raw Ethernet frame.
 * dns_off is the offset to the DNS payload (after L4 header).
 * For TCP, a 2-byte DNS message length prefix sits at dns_off.
 * Fills tx_id, domain, query_type, *is_response, rcode on success.
 * Returns true on success.
 * ---------------------------------------------------------------------- */
static bool dns_parse_payload(const char *buf, int dns_off, int n,
                               uint8_t protocol,
                               uint16_t *tx_id_out,
                               char *domain_out, int domain_max,
                               uint16_t *query_type_out,
                               int *is_response_out,
                               uint16_t *rcode_out)
{
    const char *msg;
    int         msg_len;

    if (protocol == DNS_IPPROTO_TCP) {
        /* DNS-over-TCP prepends a 2-byte message length */
        if (dns_off + 2 > n)
            return false;
        int dns_len = (int)dns_read_u16be(buf, dns_off);
        if (dns_len < 12 || dns_off + 2 + dns_len > n)
            return false;
        msg     = buf + dns_off + 2;
        msg_len = dns_len;
    } else {
        msg     = buf + dns_off;
        msg_len = n - dns_off;
    }

    if (msg_len < 12)
        return false;

    uint16_t tx_id   = dns_read_u16be(msg, 0);
    uint16_t flags   = dns_read_u16be(msg, 2);
    uint16_t qdcount = dns_read_u16be(msg, 4);

    /* Only process standard single-question DNS messages */
    if (qdcount != 1)
        return false;

    char domain[DNS_DOMAIN_MAX];
    int consumed = dns_read_name(msg, msg_len, 12, domain, DNS_DOMAIN_MAX);
    if (consumed <= 0)
        return false;

    int offset = 12 + consumed;
    if (offset + 4 > msg_len)
        return false;

    uint16_t query_type = dns_read_u16be(msg, offset);
    uint16_t qclass     = dns_read_u16be(msg, offset + 2);

    /* Only Internet class queries */
    if (qclass != 1)
        return false;

    *tx_id_out       = tx_id;
    snprintf(domain_out, (size_t)domain_max, "%s", domain);
    *query_type_out  = query_type;
    *is_response_out = (flags & 0x8000u) ? 1 : 0;
    *rcode_out       = flags & 0x000Fu;

    return true;
}

/* -------------------------------------------------------------------------
 * Pending query table operations
 * ---------------------------------------------------------------------- */
static struct netdata_dns_pending *dns_pending_find(
    struct netdata_dns_runtime *rt,
    uint16_t tx_id,
    const uint32_t server_ip[4],
    const uint32_t client_ip[4],
    uint16_t client_port,
    uint8_t  protocol,
    uint8_t  ip_version)
{
    int ip_words = (ip_version == 6) ? 4 : 1;

    for (int i = 0; i < DNS_PENDING_CAP; i++) {
        struct netdata_dns_pending *p = &rt->pending[i];
        if (!p->in_use)
            continue;
        if (p->tx_id != tx_id || p->protocol != protocol ||
            p->ip_version != ip_version || p->client_port != client_port)
            continue;
        if (memcmp(p->server_ip, server_ip, (size_t)ip_words * sizeof(uint32_t)) != 0)
            continue;
        if (memcmp(p->client_ip, client_ip, (size_t)ip_words * sizeof(uint32_t)) != 0)
            continue;
        return p;
    }
    return NULL;
}

/* Find a free slot; if table is full, evict the oldest entry. */
static struct netdata_dns_pending *dns_pending_alloc(
    struct netdata_dns_runtime *rt)
{
    struct netdata_dns_pending *oldest = NULL;

    for (int i = 0; i < DNS_PENDING_CAP; i++) {
        if (!rt->pending[i].in_use)
            return &rt->pending[i];
        if (!oldest || rt->pending[i].timestamp_us < oldest->timestamp_us)
            oldest = &rt->pending[i];
    }

    /* Table full — reuse the oldest (evict without a timeout record) */
    return oldest;
}

/* Expire pending entries older than DNS_PENDING_TIMEOUT_US and add
 * timed-out records to the flow ring. */
static void dns_expire_pending(struct netdata_dns_runtime *rt, uint64_t now_us)
{
    for (int i = 0; i < DNS_PENDING_CAP; i++) {
        struct netdata_dns_pending *p = &rt->pending[i];
        if (!p->in_use)
            continue;
        if (now_us - p->timestamp_us <= DNS_PENDING_TIMEOUT_US)
            continue;

        struct netdata_dns_flow_record *r =
            &rt->flows.records[rt->flows.head % DNS_FLOW_RING_CAP];
        memset(r, 0, sizeof(*r));
        r->timestamp_us = now_us;
        r->latency_us   = 0;
        memcpy(r->server_ip, p->server_ip, sizeof(r->server_ip));
        memcpy(r->client_ip, p->client_ip, sizeof(r->client_ip));
        snprintf(r->domain, sizeof(r->domain), "%s", p->domain);
        r->client_port = p->client_port;
        r->query_type  = p->query_type;
        r->rcode       = 0;
        r->protocol    = p->protocol;
        r->ip_version  = p->ip_version;
        r->timed_out   = 1;
        rt->flows.head++;

        p->in_use = 0;
    }
}

/* -------------------------------------------------------------------------
 * Per-packet DNS processing
 *
 * Called after L4 header parsing. is_query==1 for outgoing queries,
 * is_query==0 for incoming responses.
 * server_ip / client_ip / client_port are already in canonical form.
 * ---------------------------------------------------------------------- */
static void dns_process_packet(
    struct netdata_dns_runtime *rt,
    uint64_t now_us,
    int      is_query,
    uint8_t  protocol,
    uint8_t  ip_version,
    const uint32_t server_ip[4],
    const uint32_t client_ip[4],
    uint16_t client_port,
    uint16_t tx_id,
    const char *domain,
    uint16_t query_type,
    uint16_t rcode)
{
    if (is_query) {
        /* Ignore duplicate queries (retransmissions) */
        if (dns_pending_find(rt, tx_id, server_ip, client_ip,
                             client_port, protocol, ip_version))
            return;

        struct netdata_dns_pending *p = dns_pending_alloc(rt);
        p->timestamp_us = now_us;
        memcpy(p->server_ip, server_ip, sizeof(p->server_ip));
        memcpy(p->client_ip, client_ip, sizeof(p->client_ip));
        snprintf(p->domain, sizeof(p->domain), "%s", domain);
        p->tx_id       = tx_id;
        p->query_type  = query_type;
        p->client_port = client_port;
        p->protocol    = protocol;
        p->ip_version  = ip_version;
        p->in_use      = 1;
        return;
    }

    /* Response: match and consume a pending entry */
    struct netdata_dns_pending *p =
        dns_pending_find(rt, tx_id, server_ip, client_ip,
                         client_port, protocol, ip_version);
    if (!p)
        return;   /* unsolicited response */

    uint64_t latency_us =
        (now_us > p->timestamp_us) ? (now_us - p->timestamp_us) : 0;

    struct netdata_dns_flow_record *r =
        &rt->flows.records[rt->flows.head % DNS_FLOW_RING_CAP];
    memset(r, 0, sizeof(*r));
    r->timestamp_us = now_us;
    r->latency_us   = latency_us;
    memcpy(r->server_ip, p->server_ip, sizeof(r->server_ip));
    memcpy(r->client_ip, p->client_ip, sizeof(r->client_ip));
    snprintf(r->domain, sizeof(r->domain), "%s", p->domain);
    r->client_port = p->client_port;
    r->query_type  = p->query_type;
    r->rcode       = rcode;
    r->protocol    = p->protocol;
    r->ip_version  = p->ip_version;
    r->timed_out   = 0;
    rt->flows.head++;

    p->in_use = 0;
}

/* -------------------------------------------------------------------------
 * Raw Ethernet frame parser — used by both drain paths
 *
 * Parses headers, identifies DNS direction, parses DNS payload, and
 * calls dns_process_packet for per-query tracking.
 *
 * out_is_query / out_is_udp / out_is_ipv4 are filled for aggregate counting;
 * pass NULL for any you do not need (flow-only drain path).
 *
 * Returns true if the packet is a valid DNS packet (aggregate counter should
 * be incremented). Returns false if not a DNS packet.
 * ---------------------------------------------------------------------- */
static bool dns_parse_raw_packet(
    struct netdata_dns_runtime *rt,
    const char *buf, ssize_t n,
    uint64_t now_us,
    int *out_is_query,
    int *out_is_udp,
    int *out_is_ipv4)
{
    if (n < 14)
        return false;

    uint16_t ethertype = dns_read_u16be(buf, 12);
    int off = 14;

    while (ethertype == DNS_ETH_P_8021Q || ethertype == DNS_ETH_P_8021AD) {
        if (off + 4 > (int)n)
            return false;
        ethertype = dns_read_u16be(buf, off + 2);
        off += 4;
    }

    uint8_t  ip_version;
    uint8_t  proto;
    uint32_t saddr[4] = {0}, daddr[4] = {0};
    int      transport_off;

    if (ethertype == DNS_ETH_P_IP) {
        if (off + 20 > (int)n)
            return false;
        if ((buf[off] >> 4) != 4)
            return false;
        int ihl = (buf[off] & 0x0f) * 4;
        if (ihl < 20 || off + ihl > (int)n)
            return false;
        ip_version    = 4;
        proto         = (uint8_t)buf[off + 9];
        memcpy(saddr, buf + off + 12, 4);
        memcpy(daddr, buf + off + 16, 4);
        transport_off = off + ihl;
    } else if (ethertype == DNS_ETH_P_IPV6) {
        if (off + 40 > (int)n)
            return false;
        if ((buf[off] >> 4) != 6)
            return false;
        ip_version    = 6;
        proto         = (uint8_t)buf[off + 6];
        memcpy(saddr, buf + off + 8,  16);
        memcpy(daddr, buf + off + 24, 16);
        transport_off = off + 40;
    } else {
        return false;
    }

    if (proto != DNS_IPPROTO_UDP && proto != DNS_IPPROTO_TCP)
        return false;

    uint16_t sport = 0, dport = 0;
    int      dns_payload_off;

    if (proto == DNS_IPPROTO_UDP) {
        if (transport_off + 8 > (int)n)
            return false;
        sport          = dns_read_u16be(buf, transport_off);
        dport          = dns_read_u16be(buf, transport_off + 2);
        dns_payload_off = transport_off + 8;
    } else {
        if (transport_off + 20 > (int)n)
            return false;
        sport = dns_read_u16be(buf, transport_off);
        dport = dns_read_u16be(buf, transport_off + 2);
        int tcp_hdr = ((buf[transport_off + 12] >> 4) & 0x0f) * 4;
        if (tcp_hdr < 20 || transport_off + tcp_hdr > (int)n)
            return false;
        dns_payload_off = transport_off + tcp_hdr;
    }

    int is_query    = dns_is_monitored_port(dport);
    int is_response = !is_query && dns_is_monitored_port(sport);
    if (!is_query && !is_response)
        return false;

    if (out_is_query)  *out_is_query  = is_query;
    if (out_is_udp)    *out_is_udp    = (proto == DNS_IPPROTO_UDP);
    if (out_is_ipv4)   *out_is_ipv4   = (ip_version == 4);

    /* DNS payload parsing for per-query tracking */
    uint16_t tx_id = 0, query_type = 0, rcode = 0;
    char     domain[DNS_DOMAIN_MAX] = {0};
    int      is_resp_flag = 0;

    if (!dns_parse_payload(buf, dns_payload_off, (int)n, proto,
                           &tx_id, domain, DNS_DOMAIN_MAX,
                           &query_type, &is_resp_flag, &rcode))
        return true;   /* valid transport DNS packet; still count aggregate */

    /* Canonical form: query → client=src, server=dst; response → reversed */
    uint32_t server_ip[4] = {0}, client_ip[4] = {0};
    uint16_t client_port;

    if (!is_resp_flag) {
        memcpy(client_ip, saddr, sizeof(saddr));
        memcpy(server_ip, daddr, sizeof(daddr));
        client_port = sport;
    } else {
        memcpy(server_ip, saddr, sizeof(saddr));
        memcpy(client_ip, daddr, sizeof(daddr));
        client_port = dport;
    }

    dns_expire_pending(rt, now_us);
    dns_process_packet(rt, now_us, !is_resp_flag, proto, ip_version,
                       server_ip, client_ip, client_port,
                       tx_id, domain, query_type, rcode);

    return true;
}

/* -------------------------------------------------------------------------
 * Base variant: drain raw AF_PACKET socket with manual header parsing.
 * Also does per-query DNS payload parsing.
 * ---------------------------------------------------------------------- */
static void dns_drain_socket(struct netdata_dns_runtime *rt)
{
    char    buf[DNS_PACKET_BUF];
    ssize_t n;
    uint64_t now_us = dns_now_us();

    while ((n = recv(rt->sock_fd, buf, sizeof(buf), MSG_DONTWAIT)) > 0) {
        int is_query, is_udp, is_ipv4;
        if (dns_parse_raw_packet(rt, buf, n, now_us,
                                 &is_query, &is_udp, &is_ipv4))
            dns_acc_event(&rt->acc, is_query, is_udp, is_ipv4);
    }
}

/* -------------------------------------------------------------------------
 * Ring buffer mode: drain dedicated flow-capture socket.
 * Aggregate counting is done by the ring buffer callback; this path
 * exists only to feed the per-query DNS payload parser.
 * ---------------------------------------------------------------------- */
static void dns_drain_flow_socket(struct netdata_dns_runtime *rt)
{
    if (!rt->per_query || rt->flow_fd < 0)
        return;

    char    buf[DNS_PACKET_BUF];
    ssize_t n;
    uint64_t now_us = dns_now_us();

    while ((n = recv(rt->flow_fd, buf, sizeof(buf), MSG_DONTWAIT)) > 0)
        dns_parse_raw_packet(rt, buf, n, now_us, NULL, NULL, NULL);
}

/* -------------------------------------------------------------------------
 * Open the dedicated per-query capture socket (ring buffer mode only).
 *
 * Uses a 5-instruction classic BPF (cBPF) filter that accepts IPv4 and
 * IPv6 frames and drops everything else.  Port filtering is done in user
 * space inside dns_parse_raw_packet(), so the filter is intentionally
 * loose to keep the bytecode minimal and auditable.
 * ---------------------------------------------------------------------- */
static int dns_open_flow_socket(void)
{
    /* Classic BPF (cBPF) filter: accept IPv4 and IPv6 Ethernet frames only.
     * Raw opcode values avoid any dependency on <linux/filter.h>:
     *   0x28 = LD H ABS   (load 16-bit half-word at absolute offset)
     *   0x15 = JEQ K      (jump if equal to constant)
     *   0x06 = RET K      (return constant) */
    static struct netdata_sock_filter flow_filter_code[5] = {
        { 0x28u, 0, 0, 12u          },  /* LD  H  ABS 12         → load EtherType */
        { 0x15u, 2, 0, 0x0800u      },  /* JEQ 0x0800, jt=2      → IPv4 → accept  */
        { 0x15u, 1, 0, 0x86DDu      },  /* JEQ 0x86DD, jt=1      → IPv6 → accept  */
        { 0x06u, 0, 0, 0u           },  /* RET 0                  → drop           */
        { 0x06u, 0, 0, 0xffffffffu  },  /* RET 0xffffffff         → accept all     */
    };
    struct netdata_sock_fprog flow_filter;
    memset(&flow_filter, 0, sizeof(flow_filter));
    flow_filter.len    = 5;
    flow_filter.filter = flow_filter_code;

    int sock = socket(AF_PACKET, SOCK_RAW, htons(DNS_ETH_P_ALL));
    if (sock < 0) {
        fprintf(stderr, "ebpf-go: dns: flow socket() failed (errno %d)\n", errno);
        return -1;
    }

    if (setsockopt(sock, SOL_SOCKET, SO_ATTACH_FILTER,
                   &flow_filter, sizeof(flow_filter)) < 0) {
        fprintf(stderr, "ebpf-go: dns: SO_ATTACH_FILTER failed (errno %d)\n", errno);
        close(sock);
        return -1;
    }

    return sock;
}

/* -------------------------------------------------------------------------
 * BPF helpers
 * ---------------------------------------------------------------------- */
static int dns_init_ports_map(struct bpf_object *obj)
{
    struct bpf_map *map = bpf_object__find_map_by_name(obj, "dns_ports");
    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    uint32_t val = 1;
    for (size_t i = 0; i < DNS_PORT_COUNT; i++) {
        uint16_t port = dns_monitored_ports[i];
        if (bpf_map_update_elem(fd, &port, &val, BPF_ANY) < 0)
            fprintf(stderr,
                    "ebpf-go: dns: failed to register port %u (errno %d)\n",
                    (unsigned)port, errno);
    }

    return 0;
}

static int dns_attach_filter(struct netdata_dns_runtime *rt, const char *prog_name)
{
    struct bpf_program *prog = bpf_object__find_program_by_name(rt->obj, prog_name);
    if (!prog) {
        fprintf(stderr, "ebpf-go: dns: program %s not found\n", prog_name);
        return -1;
    }

    int prog_fd = bpf_program__fd(prog);
    if (prog_fd < 0)
        return -1;

    int sock = socket(AF_PACKET, SOCK_RAW, htons(DNS_ETH_P_ALL));
    if (sock < 0) {
        fprintf(stderr, "ebpf-go: dns: socket() failed (errno %d)\n", errno);
        return -1;
    }

    if (setsockopt(sock, SOL_SOCKET, SO_ATTACH_BPF, &prog_fd, sizeof(prog_fd)) < 0) {
        fprintf(stderr, "ebpf-go: dns: SO_ATTACH_BPF failed (errno %d)\n", errno);
        close(sock);
        return -1;
    }

    rt->sock_fd = sock;
    return 0;
}

/* -------------------------------------------------------------------------
 * Public API
 * ---------------------------------------------------------------------- */
struct netdata_dns_runtime *netdata_dns_runtime_open_mode(const char *path, int use_core, int per_query)
{
    (void)use_core;

    struct netdata_dns_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt)
        return NULL;

    rt->sock_fd = -1;
    rt->flow_fd = -1;
    rt->per_query = per_query ? 1 : 0;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        free(rt);
        return NULL;
    }

    rt->obj = obj;

    if (bpf_object__find_program_by_name(obj, "socket__dns_filter_buffer"))
        rt->kind = DNS_RUNTIME_RINGBUF;
    else if (bpf_object__find_program_by_name(obj, "socket__dns_filter"))
        rt->kind = DNS_RUNTIME_BASE;
    else {
        fprintf(stderr, "ebpf-go: dns: no recognized program in %s\n", path);
        bpf_object__close(obj);
        free(rt);
        return NULL;
    }

    return rt;
}

int netdata_dns_runtime_prepare(struct netdata_dns_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;
    return 0;
}

int netdata_dns_runtime_load(struct netdata_dns_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;
    return bpf_object__load(rt->obj);
}

int netdata_dns_runtime_attach(struct netdata_dns_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

    if (dns_init_ports_map(rt->obj) < 0) {
        fprintf(stderr, "ebpf-go: dns: dns_ports map unavailable\n");
        return -1;
    }

    const char *prog_name = (rt->kind == DNS_RUNTIME_BASE)
        ? "socket__dns_filter"
        : "socket__dns_filter_buffer";

    if (dns_attach_filter(rt, prog_name) < 0)
        return -1;

    if (rt->kind == DNS_RUNTIME_RINGBUF) {
#if DNS_HAS_RINGBUF
        struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "dns_events");
        if (!map) {
            fprintf(stderr, "ebpf-go: dns: dns_events map not found\n");
            close(rt->sock_fd);
            rt->sock_fd = -1;
            return -1;
        }

        int fd = bpf_map__fd(map);
        if (fd < 0) {
            close(rt->sock_fd);
            rt->sock_fd = -1;
            return -1;
        }

        rt->rb = ring_buffer__new(fd, dns_rb_callback, rt, NULL);
        if (!rt->rb) {
            fprintf(stderr, "ebpf-go: dns: ring_buffer__new failed\n");
            close(rt->sock_fd);
            rt->sock_fd = -1;
            return -1;
        }

        /* Per-query DNS payload tracking requires a dedicated AF_PACKET
         * capture socket. The ring buffer path still emits aggregate counters
         * regardless of this flag. When the operator (or this Go side) does
         * not need per-query tracking, skip the flow socket entirely so the
         * kernel does not spend CPU on a capture socket no consumer reads. */
        if (rt->per_query) {
            rt->flow_fd = dns_open_flow_socket();
            if (rt->flow_fd < 0)
                fprintf(stderr,
                        "ebpf-go: dns: flow socket unavailable; per-query tracking disabled\n");
        }
#else
        fprintf(stderr,
                "ebpf-go: dns: ring buffer variant requires libbpf >= 0.2\n");
        close(rt->sock_fd);
        rt->sock_fd = -1;
        return -1;
#endif
    }

    return 0;
}

int netdata_dns_runtime_snapshot(
    struct netdata_dns_runtime *rt,
    struct netdata_dns_snapshot *out)
{
    if (!rt || !out)
        return -1;

    if (rt->kind == DNS_RUNTIME_RINGBUF) {
#if DNS_HAS_RINGBUF
        if (rt->rb)
            ring_buffer__poll(rt->rb, 0);
#endif
        dns_drain_flow_socket(rt);
    } else {
        if (rt->sock_fd >= 0)
            dns_drain_socket(rt);
    }

    *out = rt->acc;
    memset(&rt->acc, 0, sizeof(rt->acc));

    return 0;
}

/* Return per-query flow records that fall within the 20-second live window.
 * Records are written into out[0..max_records-1].
 * Returns the number of records written, or -1 on error. When the runtime
 * was attached with per_query disabled, returns 0 (no flow socket was ever
 * opened). */
int netdata_dns_runtime_flow_snapshot(
    struct netdata_dns_runtime *rt,
    struct netdata_dns_flow_record *out,
    int max_records)
{
    if (!rt || !out || max_records <= 0)
        return -1;

    if (!rt->per_query)
        return 0;

    uint64_t now_us = dns_now_us();
    int      count  = 0;

    /* head is the next write slot (monotonically increasing).
     * Active records span [head - min(head, CAP), head). */
    uint32_t head  = rt->flows.head;
    uint32_t total = (head < (uint32_t)DNS_FLOW_RING_CAP)
                     ? head : (uint32_t)DNS_FLOW_RING_CAP;
    uint32_t start = (head >= total) ? (head - total) : 0;

    for (uint32_t i = start; i < head && count < max_records; i++) {
        const struct netdata_dns_flow_record *r =
            &rt->flows.records[i % DNS_FLOW_RING_CAP];
        if (r->timestamp_us == 0)
            continue;
        if (now_us - r->timestamp_us > DNS_FLOW_TTL_US)
            continue;
        out[count++] = *r;
    }

    return count;
}

void netdata_dns_runtime_close(struct netdata_dns_runtime *rt)
{
    if (!rt)
        return;

#if DNS_HAS_RINGBUF
    if (rt->rb) {
        ring_buffer__free(rt->rb);
        rt->rb = NULL;
    }
#endif

    if (rt->flow_fd >= 0) {
        close(rt->flow_fd);
        rt->flow_fd = -1;
    }

    if (rt->sock_fd >= 0) {
        close(rt->sock_fd);
        rt->sock_fd = -1;
    }

    if (rt->obj) {
        bpf_object__close(rt->obj);
        rt->obj = NULL;
    }

    free(rt);
}
