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

/* -------------------------------------------------------------------------
 * Runtime types
 * ---------------------------------------------------------------------- */

enum netdata_dns_runtime_kind {
    DNS_RUNTIME_BASE    = 0, /* no ring buffer; count from raw socket reads */
    DNS_RUNTIME_RINGBUF = 1, /* ring buffer (buffer or arena ELF flavor)    */
};

/* netdata_dns_event_t mirrors the BPF event layout confirmed from BPF bytecode:
 *   insn [180] MOV r2 = 56  → bpf_ringbuf_reserve size = 56 bytes
 * Field offsets:
 *   ct:0(8) saddr[4]:8(16) daddr[4]:24(16) pkt_len:40(4)
 *   sport:44(2) dport:46(2) protocol:48(1) ip_version:49(1) direction:50(1) pad:51(1)
 * Trailing padding to align-8 boundary → sizeof == 56. */
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
    uint32_t _align_pad; /* explicit trailing pad; struct size == 56 bytes */
};

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

struct netdata_dns_runtime {
    int kind;
    struct bpf_object *obj;
    int sock_fd;
#if DNS_HAS_RINGBUF
    struct ring_buffer *rb;
#else
    void *rb; /* always NULL on old libbpf */
#endif
    struct netdata_dns_snapshot acc; /* accumulated since last Snapshot call */
};

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

    int is_query = (ev->direction == DNS_DIRECTION_QUERY);
    int is_udp   = (ev->protocol  == DNS_IPPROTO_UDP);
    int is_ipv4  = (ev->ip_version == 4);

    struct netdata_dns_snapshot *a = &rt->acc;
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

    return 0;
}
#endif /* DNS_HAS_RINGBUF */

/* -------------------------------------------------------------------------
 * Base variant: drain raw AF_PACKET socket with manual header parsing
 * ---------------------------------------------------------------------- */

static int dns_is_monitored_port(uint16_t port)
{
    for (size_t i = 0; i < DNS_PORT_COUNT; i++) {
        if (port == dns_monitored_ports[i])
            return 1;
    }
    return 0;
}

/* Read a big-endian u16 from a byte buffer at the given offset. */
static uint16_t dns_read_u16be(const char *buf, int off)
{
    return (uint16_t)(((uint8_t)buf[off] << 8) | (uint8_t)buf[off + 1]);
}

static void dns_drain_socket(struct netdata_dns_runtime *rt)
{
    char buf[2048];
    ssize_t n;

    while ((n = recv(rt->sock_fd, buf, sizeof(buf), MSG_DONTWAIT)) > 0) {
        if (n < 14)  /* minimum Ethernet frame */
            continue;

        /* Ethernet: dest(6) + src(6) + ethertype(2) */
        uint16_t ethertype = dns_read_u16be(buf, 12);
        int off = 14;

        /* Strip 802.1Q / 802.1ad VLAN tags */
        while (ethertype == DNS_ETH_P_8021Q || ethertype == DNS_ETH_P_8021AD) {
            if (off + 4 > n)
                goto next_pkt;
            ethertype = dns_read_u16be(buf, off + 2);
            off += 4;
        }

        uint8_t ip_version;
        uint8_t proto;
        int transport_off;

        if (ethertype == DNS_ETH_P_IP) {
            /* IPv4: byte 0 has version(4b) + IHL(4b); protocol at byte 9 */
            if (off + 20 > n) continue;
            int ihl = (buf[off] & 0x0f) * 4;
            if (ihl < 20 || off + ihl > n) continue;
            ip_version    = 4;
            proto         = (uint8_t)buf[off + 9];
            transport_off = off + ihl;
        } else if (ethertype == DNS_ETH_P_IPV6) {
            /* IPv6: nexthdr at byte 6; fixed 40-byte header */
            if (off + 40 > n) continue;
            ip_version    = 6;
            proto         = (uint8_t)buf[off + 6];
            transport_off = off + 40;
        } else {
            continue; /* non-IP; BPF filter should have blocked this */
        }

        uint16_t sport = 0, dport = 0;
        if (proto == DNS_IPPROTO_UDP) {
            /* UDP: src(2) dst(2) len(2) check(2) */
            if (transport_off + 8 > n) continue;
            sport = dns_read_u16be(buf, transport_off);
            dport = dns_read_u16be(buf, transport_off + 2);
        } else if (proto == DNS_IPPROTO_TCP) {
            /* TCP: src(2) dst(2) … */
            if (transport_off + 20 > n) continue;
            sport = dns_read_u16be(buf, transport_off);
            dport = dns_read_u16be(buf, transport_off + 2);
        } else {
            continue;
        }

        int is_query    = dns_is_monitored_port(dport);
        int is_response = !is_query && dns_is_monitored_port(sport);
        if (!is_query && !is_response)
            continue;

        int is_udp  = (proto == DNS_IPPROTO_UDP);
        int is_ipv4 = (ip_version == 4);

        struct netdata_dns_snapshot *a = &rt->acc;
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

    next_pkt:;
    }
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

    /* Create raw AF_PACKET socket to receive all frames on all interfaces. */
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

struct netdata_dns_runtime *netdata_dns_runtime_open_mode(const char *path, int use_core)
{
    (void)use_core; /* DNS socket filter has no kprobe/fentry variants */

    struct netdata_dns_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt)
        return NULL;

    rt->sock_fd = -1;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        free(rt);
        return NULL;
    }

    rt->obj = obj;

    /* Determine flavor from program name present in the ELF. */
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
    /* DNS programs have no per-CPU maps to reconfigure. */
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
#else
        /* Ring buffer requires libbpf >= 0.2; caller should try base flavor. */
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
            ring_buffer__poll(rt->rb, 0); /* drain all pending events, non-blocking */
#endif
    } else {
        if (rt->sock_fd >= 0)
            dns_drain_socket(rt);
    }

    *out = rt->acc;
    memset(&rt->acc, 0, sizeof(rt->acc));

    return 0;
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
