//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <errno.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

/*
 * libbpf 0.0.9 (CentOS 7) compatibility shims — identical to those in
 * cachestat_libbpf.c so both files compile cleanly on old and new libbpf.
 */
#ifndef LIBBPF_MAJOR_VERSION
static inline int bpf_program__set_autoload(struct bpf_program *prog, bool autoload)
{
    (void)prog;
    (void)autoload;
    return 0;
}

static inline enum bpf_map_type bpf_map__type(const struct bpf_map *map)
{
    return bpf_map__def(map)->type;
}

static inline int bpf_map__set_type(struct bpf_map *map, enum bpf_map_type type)
{
    ((struct bpf_map_def *)bpf_map__def(map))->type = type;
    return 0;
}
#endif /* !LIBBPF_MAJOR_VERSION */

/* Passive connection value stored in tbl_lports.
 * Key: {protocol:u16, port:u16}  Value: this struct. */
typedef struct {
    uint32_t tgid;
    uint32_t pid;
    uint64_t counter;
} netdata_passive_conn_t;

/* IPPROTO_TCP / IPPROTO_UDP as known by the socket BPF programs. */
#define SOCKET_IPPROTO_TCP 6
#define SOCKET_IPPROTO_UDP 17

/* NETDATA_SOCKET_COUNTER = 18 (enum ebpf_socket_idx in ebpf_socket.h). */
#define SOCKET_GLOBAL_MAP_ENTRIES 18

enum netdata_ebpf_socket_runtime_kind {
    NETDATA_SOCKET_RUNTIME_LEGACY = 0,
    NETDATA_SOCKET_RUNTIME_CORE   = 1,
};

struct netdata_ebpf_socket_runtime {
    int kind;
    struct bpf_object *obj;
    struct bpf_link **links;
    int nlinks;

    /* Per-CPU buffer for tbl_global_sock lookups (uint64 per CPU per key).
     * Size = percpu_u64_cap × sizeof(uint64_t); allocated in prepare(). */
    uint64_t *percpu_u64;
    int       percpu_u64_cap;

    /* Per-CPU buffer for tbl_lports lookups.  Always sized for
     * libbpf_num_possible_cpus() entries so both HASH and PERCPU_HASH
     * variants of the map can be read without overflow. */
    netdata_passive_conn_t *percpu_passive;
    int                     percpu_passive_cap;
};

/* Snapshot output: raw counters from tbl_global_sock and tbl_lports. */
struct netdata_ebpf_socket_snapshot {
    /* tbl_global_sock keys 0-17 (enum ebpf_socket_idx order) */
    uint64_t calls_tcp_sendmsg;        /* key  0 */
    uint64_t error_tcp_sendmsg;        /* key  1 */
    uint64_t bytes_tcp_sendmsg;        /* key  2 */
    uint64_t calls_tcp_cleanup_rbuf;   /* key  3 */
    uint64_t error_tcp_cleanup_rbuf;   /* key  4 */
    uint64_t bytes_tcp_cleanup_rbuf;   /* key  5 */
    uint64_t calls_tcp_close;          /* key  6 */
    uint64_t calls_udp_recvmsg;        /* key  7 */
    uint64_t error_udp_recvmsg;        /* key  8 */
    uint64_t bytes_udp_recvmsg;        /* key  9 */
    uint64_t calls_udp_sendmsg;        /* key 10 */
    uint64_t error_udp_sendmsg;        /* key 11 */
    uint64_t bytes_udp_sendmsg;        /* key 12 */
    uint64_t tcp_retransmit;           /* key 13 */
    uint64_t calls_tcp_connect_ipv4;   /* key 14 */
    uint64_t error_tcp_connect_ipv4;   /* key 15 */
    uint64_t calls_tcp_connect_ipv6;   /* key 16 */
    uint64_t error_tcp_connect_ipv6;   /* key 17 */
    /* tbl_lports aggregated by protocol */
    uint64_t inbound_conn_tcp;
    uint64_t inbound_conn_udp;
};

/* -------------------------------------------------------------------------
 * Autoload helpers
 * ---------------------------------------------------------------------- */

static void socket_disable_programs_with_suffix(struct bpf_object *obj, const char *suffix)
{
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        if (strstr(bpf_program__name(prog), suffix))
            bpf_program__set_autoload(prog, false);
    }
}

/* In legacy mode disable fentry/fexit; in CO-RE mode disable kprobe/kretprobe. */
static void socket_prepare_autoload(struct bpf_object *obj, int use_core)
{
    if (use_core) {
        socket_disable_programs_with_suffix(obj, "_kprobe");
        socket_disable_programs_with_suffix(obj, "_kretprobe");
    } else {
        socket_disable_programs_with_suffix(obj, "_fentry");
        socket_disable_programs_with_suffix(obj, "_fexit");
    }
}

/* -------------------------------------------------------------------------
 * Map type configuration
 * ---------------------------------------------------------------------- */

static void socket_update_map_types(struct bpf_object *obj, int maps_per_core)
{
    /* Only tbl_global_sock benefits from per-CPU map type toggling.
     * The open-socket and UDP session hashes are keyed by 5-tuple and are
     * not per-CPU (each socket entry is unique across the system). */
    struct bpf_map *map = bpf_object__find_map_by_name(obj, "tbl_global_sock");
    if (!map)
        return;

    enum bpf_map_type type = bpf_map__type(map);
    if (maps_per_core) {
        if (type == BPF_MAP_TYPE_ARRAY)
            bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_ARRAY);
    } else {
        if (type == BPF_MAP_TYPE_PERCPU_ARRAY)
            bpf_map__set_type(map, BPF_MAP_TYPE_ARRAY);
    }
}

/* -------------------------------------------------------------------------
 * Link management
 * ---------------------------------------------------------------------- */

static void socket_destroy_links(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->links)
        return;

    for (int i = 0; i < rt->nlinks; i++) {
        if (rt->links[i])
            bpf_link__destroy(rt->links[i]);
    }

    free(rt->links);
    rt->links  = NULL;
    rt->nlinks = 0;
}

/* -------------------------------------------------------------------------
 * Attach — kprobe / kretprobe
 * All three socket binary flavors (base, buffer, arena) use kprobe-style
 * program sections exclusively.  Program names confirmed via readelf -s on
 * pnetdata_ebpf_socket{,_buffer,_arena}.*.o — identical across all flavors.
 * ---------------------------------------------------------------------- */

struct socket_kprobe_target {
    const char *prog_name;   /* C function name in the BPF object ELF */
    const char *kernel_func; /* kernel function to probe */
    bool retprobe;
    bool optional; /* tcp_v6_connect may be absent on some kernels */
};

static const struct socket_kprobe_target socket_kprobe_targets[] = {
    {"netdata_inet_csk_accept",    "inet_csk_accept",    true,  false},
    {"netdata_tcp_sendmsg",        "tcp_sendmsg",        false, false},
    {"netdata_tcp_retransmit_skb", "tcp_retransmit_skb", false, false},
    {"netdata_tcp_set_state",      "tcp_set_state",      false, false},
    {"netdata_tcp_cleanup_rbuf",   "tcp_cleanup_rbuf",   false, false},
    {"netdata_tcp_close",          "tcp_close",          false, false},
    {"netdata_tcp_v4_connect",     "tcp_v4_connect",     false, false},
    {"netdata_tcp_v6_connect",     "tcp_v6_connect",     false, true},
    {"trace_udp_recvmsg",          "udp_recvmsg",        false, false},
    {"trace_udp_ret_recvmsg",      "udp_recvmsg",        true,  false},
    {"trace_udp_sendmsg",          "udp_sendmsg",        false, false},
};

#define SOCKET_KPROBE_TARGET_COUNT \
    (sizeof(socket_kprobe_targets) / sizeof(socket_kprobe_targets[0]))

static int socket_attach_kprobes(struct netdata_ebpf_socket_runtime *rt)
{
    rt->links = calloc(SOCKET_KPROBE_TARGET_COUNT, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    for (size_t i = 0; i < SOCKET_KPROBE_TARGET_COUNT; i++) {
        const struct socket_kprobe_target *t = &socket_kprobe_targets[i];

        struct bpf_program *prog = bpf_object__find_program_by_name(rt->obj, t->prog_name);
        if (!prog) {
            if (!t->optional)
                fprintf(stderr, "ebpf-go: socket kprobe program %s not found\n", t->prog_name);
            continue;
        }

        struct bpf_link *link = bpf_program__attach_kprobe(prog, t->retprobe, t->kernel_func);
        if (!link || libbpf_get_error(link)) {
            if (!t->optional) {
                fprintf(stderr, "ebpf-go: attach %s -> %s failed (errno %d)\n",
                        t->prog_name, t->kernel_func, errno);
                socket_destroy_links(rt);
                return -1;
            }
            continue;
        }

        rt->links[rt->nlinks++] = link;
    }

    return 0;
}

/* -------------------------------------------------------------------------
 * Per-CPU sum helpers
 * ---------------------------------------------------------------------- */

static uint64_t socket_sum_percpu_u64(const uint64_t *values, int count)
{
    uint64_t sum = 0;
    for (int i = 0; i < count; i++)
        sum += values[i];
    return sum;
}

static uint64_t socket_sum_percpu_passive_counter(const netdata_passive_conn_t *values, int count)
{
    uint64_t sum = 0;
    for (int i = 0; i < count; i++)
        sum += values[i].counter;
    return sum;
}

/* -------------------------------------------------------------------------
 * Public API
 * ---------------------------------------------------------------------- */

struct netdata_ebpf_socket_runtime *netdata_socket_runtime_open_mode(const char *path, int use_core)
{
    struct netdata_ebpf_socket_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt)
        return NULL;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        free(rt);
        return NULL;
    }

    rt->obj  = obj;
    rt->kind = use_core ? NETDATA_SOCKET_RUNTIME_CORE : NETDATA_SOCKET_RUNTIME_LEGACY;
    return rt;
}

int netdata_socket_runtime_prepare(struct netdata_ebpf_socket_runtime *rt, int maps_per_core)
{
    if (!rt || !rt->obj)
        return -1;

    socket_prepare_autoload(rt->obj, rt->kind == NETDATA_SOCKET_RUNTIME_CORE);
    socket_update_map_types(rt->obj, maps_per_core);

    /* Allocate per-CPU buffer for tbl_global_sock snapshot reads.
     * Size depends on whether the map was switched to PERCPU_ARRAY. */
    int ncpu = maps_per_core ? libbpf_num_possible_cpus() : 1;
    if (ncpu <= 0)
        ncpu = 1;

    rt->percpu_u64 = calloc((size_t)ncpu, sizeof(*rt->percpu_u64));
    if (!rt->percpu_u64)
        return -1;
    rt->percpu_u64_cap = ncpu;

    /* tbl_lports may be HASH or PERCPU_HASH depending on the binary and libbpf
     * version.  Always allocate for the maximum possible CPU count so both
     * variants are handled without a buffer overflow on lookup. */
    int lports_ncpu = libbpf_num_possible_cpus();
    if (lports_ncpu <= 0)
        lports_ncpu = 1;
    rt->percpu_passive = calloc((size_t)lports_ncpu, sizeof(*rt->percpu_passive));
    if (!rt->percpu_passive)
        return -1;
    rt->percpu_passive_cap = lports_ncpu;

    return 0;
}

int netdata_socket_runtime_load(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;
    return bpf_object__load(rt->obj);
}

int netdata_socket_runtime_attach(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

    if (rt->links)
        socket_destroy_links(rt);

    /* All socket binary flavors use kprobe-style sections; attach identically
     * for both legacy and CO-RE modes. */
    return socket_attach_kprobes(rt);
}

int netdata_socket_runtime_snapshot(
    struct netdata_ebpf_socket_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_socket_snapshot *out)
{
    if (!rt || !rt->obj || !out)
        return -1;

    /* ---- tbl_global_sock: 18-entry ARRAY or PERCPU_ARRAY of uint64 ---- */
    struct bpf_map *gmap = bpf_object__find_map_by_name(rt->obj, "tbl_global_sock");
    if (!gmap)
        return -1;

    int gfd = bpf_map__fd(gmap);
    if (gfd < 0)
        return -1;

    (void)maps_per_core; /* CPU count was fixed at prepare() time */
    int count = rt->percpu_u64_cap > 0 ? rt->percpu_u64_cap : 1;
    uint64_t *ubuf = rt->percpu_u64;
    if (!ubuf)
        return -1;

    /* Map key order matches enum ebpf_socket_idx from ebpf_socket.h. */
    uint64_t *dst[] = {
        &out->calls_tcp_sendmsg,       /* 0  */
        &out->error_tcp_sendmsg,       /* 1  */
        &out->bytes_tcp_sendmsg,       /* 2  */
        &out->calls_tcp_cleanup_rbuf,  /* 3  */
        &out->error_tcp_cleanup_rbuf,  /* 4  */
        &out->bytes_tcp_cleanup_rbuf,  /* 5  */
        &out->calls_tcp_close,         /* 6  */
        &out->calls_udp_recvmsg,       /* 7  */
        &out->error_udp_recvmsg,       /* 8  */
        &out->bytes_udp_recvmsg,       /* 9  */
        &out->calls_udp_sendmsg,       /* 10 */
        &out->error_udp_sendmsg,       /* 11 */
        &out->bytes_udp_sendmsg,       /* 12 */
        &out->tcp_retransmit,          /* 13 */
        &out->calls_tcp_connect_ipv4,  /* 14 */
        &out->error_tcp_connect_ipv4,  /* 15 */
        &out->calls_tcp_connect_ipv6,  /* 16 */
        &out->error_tcp_connect_ipv6,  /* 17 */
    };

    for (uint32_t key = 0; key < SOCKET_GLOBAL_MAP_ENTRIES; key++) {
        *dst[key] = 0;
        if (bpf_map_lookup_elem(gfd, &key, ubuf) == 0)
            *dst[key] = socket_sum_percpu_u64(ubuf, count);
    }

    /* ---- tbl_lports: HASH or PERCPU_HASH of passive connections ---- */
    out->inbound_conn_tcp = 0;
    out->inbound_conn_udp = 0;

    struct bpf_map *lmap = bpf_object__find_map_by_name(rt->obj, "tbl_lports");
    if (!lmap)
        return 0; /* tbl_global_sock read succeeded; lports absence is non-fatal */

    int lfd = bpf_map__fd(lmap);
    if (lfd < 0 || !rt->percpu_passive)
        return 0;

    /* Determine how many per-CPU values to sum from each lookup. */
    enum bpf_map_type ltype = bpf_map__type(lmap);
    int lcount = (ltype == BPF_MAP_TYPE_PERCPU_HASH) ? rt->percpu_passive_cap : 1;

    typedef struct { uint16_t protocol; uint16_t port; } lkey_t;
    lkey_t lkey = {}, lnext = {};
    netdata_passive_conn_t *pbuf = rt->percpu_passive;

    while (bpf_map_get_next_key(lfd, &lkey, &lnext) == 0) {
        if (bpf_map_lookup_elem(lfd, &lnext, pbuf) == 0) {
            uint64_t conn = socket_sum_percpu_passive_counter(pbuf, lcount);
            if (lnext.protocol == SOCKET_IPPROTO_TCP)
                out->inbound_conn_tcp += conn;
            else
                out->inbound_conn_udp += conn;
        }
        lkey = lnext;
    }

    return 0;
}

void netdata_socket_runtime_close(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt)
        return;

    socket_destroy_links(rt);

    free(rt->percpu_u64);
    rt->percpu_u64 = NULL;

    free(rt->percpu_passive);
    rt->percpu_passive = NULL;

    if (rt->obj)
        bpf_object__close(rt->obj);

    free(rt);
}
