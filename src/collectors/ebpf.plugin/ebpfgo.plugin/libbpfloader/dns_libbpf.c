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

#include "../nd_alloc_shim.h"

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
#ifndef SO_RCVBUF
#  define SO_RCVBUF 8
#endif
#ifndef SOL_PACKET
#  define SOL_PACKET 263
#endif
#ifndef PACKET_STATISTICS
#  define PACKET_STATISTICS 6
#endif
/* Mirror of struct tpacket_stats from <linux/if_packet.h> — stable Linux ABI.
 * Prefixed to avoid collision if a kernel header is in scope. */
struct netdata_tpacket_stats {
    uint32_t tp_packets;
    uint32_t tp_drops;
};

/* Log at most once per minute when the flow socket drops frames. */
#define DNS_FLOW_DROP_LOG_INTERVAL_USEC (60ULL * 1000000ULL)

/* Ethernet protocol IDs (network byte order values) */
#define DNS_ETH_P_ALL   0x0003u
#define DNS_ETH_P_IP    0x0800u
#define DNS_ETH_P_IPV6  0x86DDu

/* IP protocol numbers */
#define DNS_IPPROTO_UDP 17u
#define DNS_IPPROTO_TCP  6u

/* Direction constants extracted from BPF bytecode (dns_buffer.skel.h):
 *   insn [191] MOV r1 = 1  (NETDATA_DNS_QUERY)
 *   insn [193] MOV r1 = 0  (NETDATA_DNS_RESPONSE)
 * so QUERY = 1, RESPONSE = 0. */
#define DNS_DIRECTION_QUERY    1u
#define DNS_DIRECTION_RESPONSE 0u

/* DNS ports to monitor for aggregate counting.  5353 (mDNS) is included for
 * aggregate stats only; per-query pending tracking is skipped for it because
 * mDNS queries are sent to a multicast group while responses come from the
 * responder's unicast IP — server_ip can never match on lookup. */
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
#define DNS_PENDING_TIMEOUT_US  5000000ULL  /* 5 s: unmatched query → timeout             */
#define DNS_FLOW_RING_CAP       1000        /* ring capacity — matches SHM       */
#define DNS_FLOW_TTL_US_DEFAULT (20ULL * 1000000ULL) /* 20 s default live window */
#define DNS_DOMAIN_MAX          256
#define DNS_PACKET_BUF          65536
#define DNS_MAX_DRAIN_PACKETS   4096        /* per snapshot, per AF_PACKET socket */

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
struct netdata_dns_runtime {
    struct bpf_object *obj;
    int sock_fd;   /* AF_PACKET socket with eBPF program; passes packets only in base flavor */
    int flow_fd;   /* AF_PACKET + classic cBPF socket; opened when the eBPF program drops packets */
    int per_query; /* when false, skip the flow socket and per-query tracking */
    uint64_t flow_drop_last_log_usec; /* last time flow socket drops were logged */
    uint64_t sock_drop_last_log_usec; /* last time sock_fd drop warning was emitted */
    uint64_t flow_ttl_us; /* live window for FlowSnapshot; settable via netdata_dns_runtime_set_flow_ttl */
    struct netdata_dns_pending    pending[DNS_PENDING_CAP];
    struct netdata_dns_flow_ring  flows;
};

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

/* Receive one frame from fd and set *ts_us to CLOCK_MONOTONIC time.
 * All TTL and pending-timeout checks use dns_now_us() (CLOCK_MONOTONIC), so
 * per-packet timestamps must also use CLOCK_MONOTONIC or elapsed-time
 * comparisons underflow. Per-packet MONOTONIC kernel timestamps would require
 * SO_CLOCKID (Linux 5.0+) with SOF_TIMESTAMPING_SOFTWARE; left as a future
 * improvement. */
static ssize_t dns_recv_ts(int fd, char *buf, size_t buf_sz, uint64_t *ts_us)
{
    ssize_t n = recv(fd, buf, buf_sz, MSG_DONTWAIT);
    if (n >= 0)
        *ts_us = dns_now_us();
    return n;
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
    int  ptrs      = 0; /* compression-pointer-follow count; guards against pointer loops */
    int  first_end = -1;
    bool jumped    = false;

    while (current < msg_len && ptrs < 32) {
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
            ptrs++;
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
            if (!((ch >= 'a' && ch <= 'z') ||
                  (ch >= '0' && ch <= '9') ||
                  ch == '-' || ch == '_'))
                ch = '_';
            out[out_len++] = (char)ch;
        }

        current += (int)label;
        /* ptrs is NOT incremented here: regular labels advance current and are
         * bounded by msg_len; only pointer follows need the loop-guard counter. */
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

/* Emit a timed-out flow record for a pending entry and clear its in_use flag.
 * Used by both the normal expiry path and the forced-eviction path so timeout
 * records are never silently dropped. */
static void dns_pending_emit_timeout(struct netdata_dns_runtime *rt,
                                     struct netdata_dns_pending *p,
                                     uint64_t now_us)
{
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

/* Find a free slot; if table is full, evict the oldest entry and emit a
 * timeout record for it so the overflow is visible in the flow ring. */
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

    /* Table full — emit a timeout record before reusing the slot. */
    dns_pending_emit_timeout(rt, oldest, dns_now_us());
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
        dns_pending_emit_timeout(rt, p, now_us);
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
        /* Clamp n to the IP total-length field so Ethernet padding bytes are
         * excluded from all subsequent length checks and dns_parse_payload. */
        uint16_t ipv4_total = dns_read_u16be(buf, off + 2);
        if (off + (int)ipv4_total > (int)n)
            return false;
        n             = (ssize_t)(off + ipv4_total);
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
        /* Clamp n to the IPv6 payload-length field (bytes after the 40-byte
         * fixed header) to exclude Ethernet padding. */
        uint16_t ipv6_payload = dns_read_u16be(buf, off + 4);
        if (off + 40 + (int)ipv6_payload > (int)n)
            return false;
        n             = (ssize_t)(off + 40 + ipv6_payload);
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

    /* Skip per-query tracking for mDNS (port 5353): queries are sent to a
     * multicast group but responses arrive from the responder's unicast IP,
     * so server_ip can never match in dns_pending_find.  mDNS traffic is
     * therefore absent from dns-queries output. */
    uint16_t server_port = is_resp_flag ? sport : dport;
    if (server_port != 5353)
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
    char     buf[DNS_PACKET_BUF];
    ssize_t  n;
    uint64_t now_us;
    int      drained = 0;

    while (drained < DNS_MAX_DRAIN_PACKETS &&
           (n = dns_recv_ts(rt->sock_fd, buf, sizeof(buf), &now_us)) > 0) {
        dns_parse_raw_packet(rt, buf, n, now_us, NULL, NULL, NULL);
        drained++;
    }
}

/* -------------------------------------------------------------------------
 * Ring buffer mode: drain dedicated flow-capture socket for per-query tracking.
 * ---------------------------------------------------------------------- */
static void dns_drain_flow_socket(struct netdata_dns_runtime *rt)
{
    if (rt->flow_fd < 0)
        return;

    char     buf[DNS_PACKET_BUF];
    ssize_t  n;
    uint64_t now_us;
    int      drained = 0;

    while (drained < DNS_MAX_DRAIN_PACKETS &&
           (n = dns_recv_ts(rt->flow_fd, buf, sizeof(buf), &now_us)) > 0) {
        dns_parse_raw_packet(rt, buf, n, now_us, NULL, NULL, NULL);
        drained++;
    }

    /* Read kernel-side drop counter.  PACKET_STATISTICS resets tp_drops on each
     * getsockopt call so this always reports drops since the previous drain. */
    struct netdata_tpacket_stats stats;
    socklen_t stats_len = sizeof(stats);
    if (getsockopt(rt->flow_fd, SOL_PACKET, PACKET_STATISTICS, &stats, &stats_len) == 0 &&
        stats.tp_drops > 0) {
        uint64_t now = dns_now_us();
        if (rt->flow_drop_last_log_usec == 0 ||
            now - rt->flow_drop_last_log_usec >= DNS_FLOW_DROP_LOG_INTERVAL_USEC) {
            fprintf(stderr,
                    "ebpf-go: dns: flow socket dropped %u frame(s) this interval;"
                    " consider raising net.core.rmem_max\n",
                    (unsigned)stats.tp_drops);
            rt->flow_drop_last_log_usec = now;
        }
    }
}

/* -------------------------------------------------------------------------
 * Open the dedicated per-query capture socket (ring buffer mode only).
 *
 * A 27-instruction classic BPF (cBPF) filter accepts only IPv4/IPv6 frames
 * carrying TCP or UDP with src or dst port 53 (DNS) or 5353 (mDNS).
 * Everything else is dropped in the kernel before recv() is called.
 *
 * Limitation: assumes no IPv4 options (IHL=5) and no IPv6 extension headers.
 * For IPv4-with-options the filter reads transport ports at the wrong offset
 * (fixed at 34 instead of 14+IHL) and typically drops the packet even though
 * dns_parse_raw_packet handles variable IHL correctly.  DNS traffic never uses
 * IPv4 options in practice, so this gap is theoretical only.
 * The aggregate count socket (sock_fd, no filter) is unaffected; only per-query
 * flow_fd tracking misses these packets.
 * IPv6 with extension headers: both the filter and dns_parse_raw_packet read
 * the same fixed next-header offset (20), so behaviour is consistent.
 * VLAN-tagged frames (802.1Q/802.1AD) are not supported and are dropped by
 * both the filter (EtherType != 0x0800/0x86DD) and dns_parse_raw_packet
 * (same EtherType check, falls through to "return false").
 *
 * Opcode reference (raw values, no <linux/filter.h> dependency):
 *   0x28 = BPF_LD  | BPF_H | BPF_ABS  (load 16-bit half-word at abs offset)
 *   0x30 = BPF_LD  | BPF_B | BPF_ABS  (load 8-bit byte at abs offset)
 *   0x15 = BPF_JMP | BPF_JEQ | BPF_K  (jump if A == K; jt/jf skip counts)
 *   0x06 = BPF_RET | BPF_K             (return K)
 *
 * NOTE: this socket uses classic cBPF (SO_ATTACH_FILTER), not an eBPF program.
 * Attaching socket__dns_filter_buffer via SO_ATTACH_BPF would drop all packets
 * (that program returns 0 on every frame) so recv() would receive nothing.
 * ---------------------------------------------------------------------- */
static int dns_open_flow_socket(void)
{
    /* 27-instruction cBPF: IPv4/IPv6, TCP/UDP, (src|dst) port 53 or 5353.
     * IPv4 transport starts at offset 34 (eth=14, no IP options → IP=20).
     * IPv6 transport starts at offset 54 (eth=14, no ext headers → IPv6=40). */
    static struct netdata_sock_filter flow_filter_code[27] = {
        /* EtherType dispatch ------------------------------------------------ */
        /* [0]  */ { 0x28u,  0,  0, 12u       },  /* LD H ABS 12  → EtherType    */
        /* [1]  */ { 0x15u,  2,  0, 0x0800u   },  /* JEQ IPv4     → [4]          */
        /* [2]  */ { 0x15u, 12,  0, 0x86DDu   },  /* JEQ IPv6     → [15]         */
        /* [3]  */ { 0x06u,  0,  0, 0u        },  /* RET 0        → drop         */
        /* IPv4 protocol at eth(14)+IP(9)=23 -------------------------------- */
        /* [4]  */ { 0x30u,  0,  0, 23u       },  /* LD B ABS 23  → IP proto     */
        /* [5]  */ { 0x15u,  2,  0, 17u       },  /* JEQ UDP      → [8]          */
        /* [6]  */ { 0x15u,  1,  0, 6u        },  /* JEQ TCP      → [8]          */
        /* [7]  */ { 0x06u,  0,  0, 0u        },  /* RET 0        → drop         */
        /* IPv4 ports: src=34, dst=36 --------------------------------------- */
        /* [8]  */ { 0x28u,  0,  0, 34u       },  /* LD H ABS 34  → src port     */
        /* [9]  */ { 0x15u, 16,  0, 53u       },  /* JEQ 53       → [26] accept  */
        /* [10] */ { 0x15u, 15,  0, 5353u     },  /* JEQ 5353     → [26] accept  */
        /* [11] */ { 0x28u,  0,  0, 36u       },  /* LD H ABS 36  → dst port     */
        /* [12] */ { 0x15u, 13,  0, 53u       },  /* JEQ 53       → [26] accept  */
        /* [13] */ { 0x15u, 12,  0, 5353u     },  /* JEQ 5353     → [26] accept  */
        /* [14] */ { 0x06u,  0,  0, 0u        },  /* RET 0        → drop         */
        /* IPv6 next header at eth(14)+IPv6(6)=20 --------------------------- */
        /* [15] */ { 0x30u,  0,  0, 20u       },  /* LD B ABS 20  → next header  */
        /* [16] */ { 0x15u,  2,  0, 17u       },  /* JEQ UDP      → [19]         */
        /* [17] */ { 0x15u,  1,  0, 6u        },  /* JEQ TCP      → [19]         */
        /* [18] */ { 0x06u,  0,  0, 0u        },  /* RET 0        → drop         */
        /* IPv6 ports: src=54, dst=56 --------------------------------------- */
        /* [19] */ { 0x28u,  0,  0, 54u       },  /* LD H ABS 54  → src port     */
        /* [20] */ { 0x15u,  5,  0, 53u       },  /* JEQ 53       → [26] accept  */
        /* [21] */ { 0x15u,  4,  0, 5353u     },  /* JEQ 5353     → [26] accept  */
        /* [22] */ { 0x28u,  0,  0, 56u       },  /* LD H ABS 56  → dst port     */
        /* [23] */ { 0x15u,  2,  0, 53u       },  /* JEQ 53       → [26] accept  */
        /* [24] */ { 0x15u,  1,  0, 5353u     },  /* JEQ 5353     → [26] accept  */
        /* [25] */ { 0x06u,  0,  0, 0u        },  /* RET 0        → drop         */
        /* [26] */ { 0x06u,  0,  0, 0xffffffffu }, /* RET -1       → accept       */
    };
    struct netdata_sock_fprog flow_filter;
    memset(&flow_filter, 0, sizeof(flow_filter));
    flow_filter.len    = 27;
    flow_filter.filter = flow_filter_code;

    int sock = socket(AF_PACKET, SOCK_RAW, htons(DNS_ETH_P_ALL));
    if (sock < 0) {
        fprintf(stderr, "ebpf-go: dns: flow socket() failed (errno %d)\n", errno);
        return -1;
    }

    /* Large receive buffer so the kernel can queue ~10 s of peak DNS traffic
     * between drains.  The cBPF filter limits this socket to port-53/5353 only,
     * so 4 MB covers roughly 27,000 small DNS frames — enough for ~2,700 qps.
     * The kernel silently caps the value at net.core.rmem_max; log and continue
     * on failure rather than aborting — a smaller default buffer still works. */
    int rcvbuf = 4 * 1024 * 1024;
    if (setsockopt(sock, SOL_SOCKET, SO_RCVBUF, &rcvbuf, sizeof(rcvbuf)) < 0)
        fprintf(stderr,
                "ebpf-go: dns: SO_RCVBUF(%d) failed (errno %d); using default\n",
                rcvbuf, errno);

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

    struct netdata_dns_runtime *rt = callocz(1, sizeof(*rt));
    if (!rt)
        return NULL;

    rt->sock_fd = -1;
    rt->flow_fd = -1;
    rt->per_query = per_query ? 1 : 0;

    rt->flow_ttl_us = DNS_FLOW_TTL_US_DEFAULT;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        freez(rt);
        return NULL;
    }

    rt->obj = obj;

    if (!bpf_object__find_program_by_name(obj, "socket__dns_filter") &&
        !bpf_object__find_program_by_name(obj, "socket__dns_filter_buffer")) {
        fprintf(stderr, "ebpf-go: dns: no recognized program in %s\n", path);
        bpf_object__close(obj);
        freez(rt);
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

    /* Prefer the base program (passes packets to recv()); fall back to the
     * ring-buffer program if the base is absent (older object files). */
    const char *prog_name = bpf_object__find_program_by_name(rt->obj, "socket__dns_filter")
        ? "socket__dns_filter"
        : "socket__dns_filter_buffer";

    if (dns_attach_filter(rt, prog_name) < 0)
        return -1;

    /* socket__dns_filter_buffer returns 0 on every packet so recv() on sock_fd
     * delivers nothing.  Open a separate cBPF socket for per-query tracking. */
    bool prog_drops = strcmp(prog_name, "socket__dns_filter_buffer") == 0;
    if (prog_drops && rt->per_query) {
        rt->flow_fd = dns_open_flow_socket();
        if (rt->flow_fd < 0)
            fprintf(stderr,
                    "ebpf-go: dns: flow socket unavailable; per-query tracking disabled\n");
    }

    return 0;
}

/* Return per-query flow records that fall within the 20-second live window.
 * Drains the AF_PACKET capture socket(s) before reading so the current cycle
 * is included (both variants use recv() on AF_PACKET, not a BPF ring buffer).
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

    if (rt->flow_fd >= 0) {
        dns_drain_flow_socket(rt);
        /* In buffer mode the eBPF program returns 0 on every frame so
         * sock_fd's recv buffer is never filled by normal operation.
         * tp_drops > 0 means the filter is not executing (detached, JIT
         * failure, etc.) and DNS queries are being silently discarded. */
        struct netdata_tpacket_stats stats;
        socklen_t stats_len = sizeof(stats);
        if (getsockopt(rt->sock_fd, SOL_PACKET, PACKET_STATISTICS, &stats, &stats_len) == 0 &&
            stats.tp_drops > 0) {
            uint64_t now = dns_now_us();
            if (rt->sock_drop_last_log_usec == 0 ||
                now - rt->sock_drop_last_log_usec >= DNS_FLOW_DROP_LOG_INTERVAL_USEC) {
                fprintf(stderr,
                        "ebpf-go: dns: sock_fd (buffer mode) dropped %u frame(s);"
                        " eBPF filter may not be executing\n",
                        (unsigned)stats.tp_drops);
                rt->sock_drop_last_log_usec = now;
            }
        }
    } else if (rt->sock_fd >= 0)
        dns_drain_socket(rt);

    if (!rt->per_query)
        return 0;

    uint64_t now_us = dns_now_us();
    /* Expire pending entries once, after all buffered packets have been
     * drained and matched.  Calling this per-packet (inside dns_parse_raw_packet)
     * caused false timeouts: a response buffered for < 5 s was unreachable
     * because its pending slot was expired before dns_process_packet ran. */
    dns_expire_pending(rt, now_us);
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
        if (now_us - r->timestamp_us > rt->flow_ttl_us)
            continue;
        out[count++] = *r;
    }

    return count;
}

void netdata_dns_runtime_set_flow_ttl(struct netdata_dns_runtime *rt, uint64_t ttl_seconds)
{
    if (!rt || ttl_seconds == 0)
        return;
    /* Clamp before multiply to avoid uint64 overflow. */
    if (ttl_seconds > UINT64_MAX / 1000000ULL)
        ttl_seconds = UINT64_MAX / 1000000ULL;
    rt->flow_ttl_us = ttl_seconds * 1000000ULL;
}

void netdata_dns_runtime_close(struct netdata_dns_runtime *rt)
{
    if (!rt)
        return;

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

    freez(rt);
}

#ifdef NETDATA_EBPF_TEST
/* -------------------------------------------------------------------------
 * Test helpers — compiled only when NETDATA_EBPF_TEST is defined (i.e. test
 * builds using the netdata_ebpf_test Go build tag).  Thin non-static wrappers
 * so the Go test layer can exercise the packet parser without a live BPF
 * runtime.  Using void * for the runtime pointer avoids pulling the full
 * struct definition (which requires libbpf headers) into the CGo preamble.
 * ---------------------------------------------------------------------- */

void *netdata_dns_alloc_test_runtime(void)
{
    struct netdata_dns_runtime *rt = callocz(1, sizeof(*rt));
    if (rt) {
        rt->sock_fd = -1;
        rt->flow_fd = -1;
    }
    return rt;
}

void netdata_dns_free_test_runtime(void *p)
{
    freez(p);
}

/* Delegates to the static dns_parse_raw_packet with now_us=0 and NULL output
 * pointers — sufficient for boundary/malformed-packet tests. */
int netdata_dns_test_parse_raw_packet(void *p, const char *buf, int n)
{
    return (int)dns_parse_raw_packet(
        (struct netdata_dns_runtime *)p,
        buf, (ssize_t)n,
        0 /* now_us */,
        NULL, NULL, NULL);
}

/* Thin wrapper so the test layer can vary out_size to check overflow guards. */
int netdata_dns_test_read_name(const char *msg, int msg_len, int offset,
                               char *out, int out_size)
{
    return dns_read_name(msg, msg_len, offset, out, out_size);
}

#endif /* NETDATA_EBPF_TEST */
