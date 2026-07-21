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

static inline int bpf_map__set_max_entries(struct bpf_map *map, __u32 max_entries)
{
    return bpf_map__resize(map, max_entries);
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

/* 18 indexed per-CPU counters in the socket BPF program's tbl_global_sock map. */
#define SOCKET_GLOBAL_MAP_ENTRIES 18

/* -------------------------------------------------------------------------
 * tbl_nd_socket per-PID aggregation types
 * ---------------------------------------------------------------------- */

/* BPF map key for tbl_nd_socket.  Must match the key struct in the socket BPF
 * program: saddr[16] + daddr[16] + dport[2] + _pad[2] + pid[4] = 40 bytes.
 * The two-byte padding brings pid to a 4-byte boundary per C struct alignment. */
typedef struct {
    uint8_t  saddr[16];
    uint8_t  daddr[16];
    uint16_t dport;
    uint16_t _pad;
    uint32_t pid;
} netdata_socket_bpf_key_t;

/* BPF map value for tbl_nd_socket.  Must match netdata_socket_t in
 * ebpf-ipc.h.  Total size = 112 bytes (tcp substructure = 48 bytes
 * including 4-byte trailing pad to reach 8-byte alignment, udp = 24 bytes).
 * Note: ebpf-ipc.h omits the explicit tcp._pad field; the BPF program layout
 * adds it so the substructure size is a multiple of 8. */
#define ND_SOCK_COMM_LEN 16
typedef struct {
    char     name[ND_SOCK_COMM_LEN];
    uint64_t first_timestamp;
    uint64_t current_timestamp;
    uint16_t protocol;
    uint16_t family;
    uint32_t external_origin;
    struct {
        uint32_t call_tcp_sent;
        uint32_t call_tcp_received;
        uint64_t tcp_bytes_sent;
        uint64_t tcp_bytes_received;
        uint32_t close;
        uint32_t retransmit;
        uint32_t ipv4_connect;
        uint32_t ipv6_connect;
        uint32_t state;
        uint32_t _pad; /* trailing pad: sizeof(tcp substructure) must be multiple of 8 */
    } tcp;
    struct {
        uint32_t call_udp_sent;
        uint32_t call_udp_received;
        uint64_t udp_bytes_sent;
        uint64_t udp_bytes_received;
    } udp;
} netdata_socket_bpf_value_t;

/* Per-PID aggregated socket metrics — output type for per-PID snapshot.
 * Field order mirrors ebpf_socket_publish_apps_t in ebpf-ipc.h so the Go
 * layer can copy directly into ebpfSocketPublishApps. */
struct netdata_socket_per_pid_entry {
    uint32_t pid;
    uint64_t bytes_sent;       /* tcp_bytes_sent: TCP only; UDP traffic is in udp_bytes_sent */
    uint64_t bytes_received;   /* tcp_bytes_received */
    uint64_t call_tcp_sent;
    uint64_t call_tcp_received;
    uint64_t retransmit;
    uint64_t call_udp_sent;
    uint64_t call_udp_received;
    uint64_t call_close;
    uint64_t call_tcp_v4_connection;
    uint64_t call_tcp_v6_connection;
};

/* Per-PID hash table for accumulation during a snapshot cycle.
 * Open-addressing with linear probing; PID 0 = empty slot. */
#define SOCKET_PID_HT_BITS 14u
#define SOCKET_PID_HT_SIZE (1u << SOCKET_PID_HT_BITS) /* 16384 */
#define SOCKET_PID_HT_MASK (SOCKET_PID_HT_SIZE - 1u)

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

    /* Per-CPU buffer for tbl_nd_socket value reads (sizeof value × ncpus). */
    netdata_socket_bpf_value_t *percpu_nd_socket;
    int                         percpu_nd_socket_cap;

    /* Flat hash-table for per-PID accumulation during a snapshot cycle.
     * Allocated once in prepare(); reused every cycle (zeroed at start). */
    struct netdata_socket_per_pid_entry *pid_ht;

    /* Compact sorted output array from last per-PID snapshot (reused). */
    struct netdata_socket_per_pid_entry *per_pid_entries;
    int                                  per_pid_count;
    int                                  per_pid_cap;
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

int netdata_socket_runtime_prepare(struct netdata_ebpf_socket_runtime *rt, int maps_per_core,
                                   uint32_t nd_socket_size, uint32_t nv_udp_size)
{
    if (!rt || !rt->obj)
        return -1;

    socket_prepare_autoload(rt->obj, rt->kind == NETDATA_SOCKET_RUNTIME_CORE);
    socket_update_map_types(rt->obj, maps_per_core);

    /* Resize user-configurable hash maps before bpf_object__load().
     * A zero value means "keep the compiled-in default". */
    if (nd_socket_size > 0) {
        struct bpf_map *m = bpf_object__find_map_by_name(rt->obj, "tbl_nd_socket");
        if (m)
            bpf_map__set_max_entries(m, nd_socket_size);
    }
    if (nv_udp_size > 0) {
        struct bpf_map *m = bpf_object__find_map_by_name(rt->obj, "tbl_nv_udp");
        if (m)
            bpf_map__set_max_entries(m, nv_udp_size);
    }

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

    /* tbl_nd_socket may also be PERCPU_HASH — allocate per-CPU value buffer. */
    rt->percpu_nd_socket = calloc((size_t)lports_ncpu, sizeof(*rt->percpu_nd_socket));
    if (!rt->percpu_nd_socket)
        return -1;
    rt->percpu_nd_socket_cap = lports_ncpu;

    /* PID hash-table for per-PID accumulation: SOCKET_PID_HT_SIZE entries. */
    rt->pid_ht = calloc(SOCKET_PID_HT_SIZE, sizeof(*rt->pid_ht));
    if (!rt->pid_ht)
        return -1;

    /* Initial output array capacity = 256 unique PIDs (grows on demand). */
    rt->per_pid_cap = 256;
    rt->per_pid_entries = calloc((size_t)rt->per_pid_cap, sizeof(*rt->per_pid_entries));
    if (!rt->per_pid_entries)
        return -1;

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

    /* Map key order matches the 18 indexed counters in the socket BPF program's tbl_global_sock. */
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
    bool first_liter = true;

    while (bpf_map_get_next_key(lfd, first_liter ? NULL : &lkey, &lnext) == 0) {
        first_liter = false;
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

    free(rt->percpu_nd_socket);
    rt->percpu_nd_socket = NULL;

    free(rt->pid_ht);
    rt->pid_ht = NULL;

    free(rt->per_pid_entries);
    rt->per_pid_entries = NULL;
    rt->per_pid_count = 0;
    rt->per_pid_cap   = 0;

    if (rt->obj)
        bpf_object__close(rt->obj);

    free(rt);
}

/* -------------------------------------------------------------------------
 * Per-PID socket snapshot (tbl_nd_socket)
 * ---------------------------------------------------------------------- */

static int pid_ht_compare(const void *a, const void *b)
{
    uint32_t pa = ((const struct netdata_socket_per_pid_entry *)a)->pid;
    uint32_t pb = ((const struct netdata_socket_per_pid_entry *)b)->pid;
    return (pa > pb) - (pa < pb);
}

/* Accumulate one BPF value into the per-PID hash table. */
static void pid_ht_accumulate(struct netdata_socket_per_pid_entry *ht,
                               uint32_t pid,
                               const netdata_socket_bpf_value_t *v)
{
    /* Knuth multiplicative hash for 14-bit output. */
    uint32_t slot = (pid * 2654435761u) & SOCKET_PID_HT_MASK;
    for (uint32_t i = 0; i < SOCKET_PID_HT_SIZE; i++) {
        uint32_t s = (slot + i) & SOCKET_PID_HT_MASK;
        if (ht[s].pid == 0) {
            ht[s].pid = pid;
        }
        if (ht[s].pid == pid) {
            ht[s].bytes_sent            += v->tcp.tcp_bytes_sent;
            ht[s].bytes_received        += v->tcp.tcp_bytes_received;
            ht[s].call_tcp_sent         += v->tcp.call_tcp_sent;
            ht[s].call_tcp_received     += v->tcp.call_tcp_received;
            ht[s].retransmit            += v->tcp.retransmit;
            ht[s].call_udp_sent         += v->udp.call_udp_sent;
            ht[s].call_udp_received     += v->udp.call_udp_received;
            ht[s].call_close            += v->tcp.close;
            ht[s].call_tcp_v4_connection += v->tcp.ipv4_connect;
            ht[s].call_tcp_v6_connection += v->tcp.ipv6_connect;
            return;
        }
    }
    /* Hash table full — silent drop; should not occur for < 16384 unique PIDs. */
}

/* Read tbl_nd_socket, aggregate per PID, and store a sorted result array.
 * Returns pointer to the sorted array (owned by rt), count via *out_count.
 * Returns NULL on error. */
const struct netdata_socket_per_pid_entry *
netdata_socket_per_pid_snapshot(struct netdata_ebpf_socket_runtime *rt, int *out_count)
{
    if (!rt || !rt->obj || !rt->pid_ht || !out_count)
        return NULL;

    struct bpf_map *ndmap = bpf_object__find_map_by_name(rt->obj, "tbl_nd_socket");
    if (!ndmap) {
        *out_count = 0;
        return rt->per_pid_entries;
    }

    int ndfd = bpf_map__fd(ndmap);
    if (ndfd < 0 || !rt->percpu_nd_socket) {
        *out_count = 0;
        return rt->per_pid_entries;
    }

    /* Determine per-CPU count for this map. */
    enum bpf_map_type ndtype = bpf_map__type(ndmap);
    int ndcount = (ndtype == BPF_MAP_TYPE_PERCPU_HASH) ? rt->percpu_nd_socket_cap : 1;

    /* Zero the hash table for this cycle. */
    memset(rt->pid_ht, 0, SOCKET_PID_HT_SIZE * sizeof(*rt->pid_ht));

    /* Iterate all connection entries and accumulate per PID. */
    netdata_socket_bpf_key_t key = {}, next = {};
    netdata_socket_bpf_value_t *vbuf = rt->percpu_nd_socket;
    bool first_nditer = true;

    while (bpf_map_get_next_key(ndfd, first_nditer ? NULL : &key, &next) == 0) {
        first_nditer = false;
        if (next.pid != 0 && bpf_map_lookup_elem(ndfd, &next, vbuf) == 0) {
            if (ndcount == 1) {
                pid_ht_accumulate(rt->pid_ht, next.pid, vbuf);
            } else {
                /* PERCPU_HASH: sum per-CPU values into a temporary entry. */
                netdata_socket_bpf_value_t agg = {0};
                for (int c = 0; c < ndcount; c++) {
                    agg.tcp.tcp_bytes_sent      += vbuf[c].tcp.tcp_bytes_sent;
                    agg.tcp.tcp_bytes_received  += vbuf[c].tcp.tcp_bytes_received;
                    agg.tcp.call_tcp_sent       += vbuf[c].tcp.call_tcp_sent;
                    agg.tcp.call_tcp_received   += vbuf[c].tcp.call_tcp_received;
                    agg.tcp.retransmit          += vbuf[c].tcp.retransmit;
                    agg.udp.call_udp_sent       += vbuf[c].udp.call_udp_sent;
                    agg.udp.call_udp_received   += vbuf[c].udp.call_udp_received;
                    agg.tcp.close               += vbuf[c].tcp.close;
                    agg.tcp.ipv4_connect        += vbuf[c].tcp.ipv4_connect;
                    agg.tcp.ipv6_connect        += vbuf[c].tcp.ipv6_connect;
                }
                pid_ht_accumulate(rt->pid_ht, next.pid, &agg);
            }
        }
        key = next;
    }

    /* Compact non-empty hash-table entries into the sorted output array. */
    int count = 0;
    for (uint32_t i = 0; i < SOCKET_PID_HT_SIZE; i++) {
        if (rt->pid_ht[i].pid == 0)
            continue;

        /* Grow output array if needed. */
        if (count >= rt->per_pid_cap) {
            int newcap = rt->per_pid_cap * 2;
            struct netdata_socket_per_pid_entry *tmp =
                realloc(rt->per_pid_entries, (size_t)newcap * sizeof(*tmp));
            if (!tmp) {
                /* Partial result is better than nothing. */
                break;
            }
            rt->per_pid_entries = tmp;
            rt->per_pid_cap     = newcap;
        }

        rt->per_pid_entries[count++] = rt->pid_ht[i];
    }

    if (count > 1)
        qsort(rt->per_pid_entries, (size_t)count, sizeof(*rt->per_pid_entries), pid_ht_compare);

    rt->per_pid_count = count;
    *out_count = count;
    return rt->per_pid_entries;
}
