//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <errno.h>
#include <signal.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

#include "../nd_alloc_shim.h"

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
#define SOCKET_PID_LIVENESS_CACHE 256

/* Run the dead-PID eviction pass every this many collection cycles to amortise
 * the per-PID kill() overhead on hosts with many active network processes. */
#define SOCKET_ACC_EVICT_INTERVAL 4

/* Minimum power-of-two capacity for the per-connection last-seen snapshot table.
 * 2× this value is the actual allocation, keeping load factor at or below 50%. */
#define SOCKET_CONN_SNAP_MIN (1u << 14u)

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

/* Ring-buffer event emitted by the buffer-flavor socket BPF program.
 * One event per kprobe invocation: idx identifies the connection, data holds
 * exactly one counter incremented (or set for byte bandwidth fields). */
typedef struct {
    netdata_socket_bpf_key_t   idx;  /* connection key */
    netdata_socket_bpf_value_t data; /* per-event delta counters */
} netdata_socket_rb_event_t;

/* Per-PID aggregated socket metrics — output type for per-PID snapshot.
 * Field order mirrors struct ebpf_socket_publish_apps in
 * apps_ebpf_shared_pid_row.h so the Go layer can copy directly into
 * ebpfSocketPublishApps. */
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

/* Per-connection last-seen snapshot for base-flavor delta accumulation.
 * Open-addressing, linear probe; in_use == 0 marks an empty slot.
 * Deletions use the shift-back algorithm — no tombstones — so the invariant
 * "empty == end of probe chain" holds throughout the table's lifetime.
 * Table is always at ≤50% load so UINT32_MAX from conn_snap_upsert never fires. */
typedef struct {
    netdata_socket_bpf_key_t key;
    uint64_t bytes_sent;
    uint64_t bytes_received;
    uint32_t call_tcp_sent;
    uint32_t call_tcp_received;
    uint32_t retransmit;
    uint32_t call_udp_sent;
    uint32_t call_udp_received;
    uint32_t call_close;
    uint32_t call_tcp_v4_connection;
    uint32_t call_tcp_v6_connection;
    uint8_t  in_use;
    uint8_t  _pad[3];
} socket_conn_snapshot_t;

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

    /* Compact sorted output array from last per-PID snapshot (reused). */
    struct netdata_socket_per_pid_entry *per_pid_entries;
    int                                  per_pid_count;
    int                                  per_pid_cap;

    /* Deferred-delete buffer for tbl_nd_socket stale entries (reused every cycle).
     * Sized to max(nd_socket_size, SOCKET_CONN_SNAP_MIN) so every stale entry
     * can be collected in a single pass without silent overflow. */
    netdata_socket_bpf_key_t *nd_del_keys;
    uint32_t                   nd_del_cap;

    /* Per-connection last-seen snapshot (base flavor only).
     * Each cycle: delta = current − last_seen is folded into rt->acc so the
     * accumulator is monotonically increasing, matching the buffer-flavor contract. */
    socket_conn_snapshot_t *conn_snap;
    uint32_t                conn_snap_sz;  /* power-of-two capacity */

    /* Dense per-PID accumulator — shared by buffer flavor and base flavor.
     * Buffer flavor: fed by ring-buffer events.
     * Base flavor: fed by per-cycle conn_snap deltas.
     * Never reset between calls; Go computes per-interval deltas by subtraction. */
    struct netdata_socket_per_pid_entry *acc;
    uint32_t acc_cap;
    uint32_t acc_count;
    uint32_t acc_evict_counter; /* cycles since last dead-PID eviction pass */
    uint32_t *acc_htable;       /* PID → (acc_index + 1); 0 = empty slot */
    uint32_t acc_htable_sz;     /* power-of-two capacity */

#if defined(LIBBPF_MAJOR_VERSION)
    /* Ring-buffer handle — non-NULL only when the loaded object is buffer flavor.
     * NULL on base/arena flavor objects or builds without modern libbpf. */
    struct ring_buffer *rb;
#endif
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
        if (type == BPF_MAP_TYPE_ARRAY) {
            if (bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_ARRAY) != 0)
                fprintf(stderr, "ebpf-go.plugin: socket: failed to set tbl_global_sock type to PERCPU_ARRAY\n");
        }
    } else {
        if (type == BPF_MAP_TYPE_PERCPU_ARRAY) {
            if (bpf_map__set_type(map, BPF_MAP_TYPE_ARRAY) != 0)
                fprintf(stderr, "ebpf-go.plugin: socket: failed to set tbl_global_sock type to ARRAY\n");
        }
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

    freez(rt->links);
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
    rt->links = callocz(SOCKET_KPROBE_TARGET_COUNT, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    for (size_t i = 0; i < SOCKET_KPROBE_TARGET_COUNT; i++) {
        const struct socket_kprobe_target *t = &socket_kprobe_targets[i];

        struct bpf_program *prog = bpf_object__find_program_by_name(rt->obj, t->prog_name);
        if (!prog) {
            if (!t->optional) {
                fprintf(stderr, "ebpf-go: socket kprobe program %s not found\n", t->prog_name);
                socket_destroy_links(rt);
                return -1;
            }
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

    /* All programs absent from the BPF object is a hard failure. */
    return rt->nlinks == 0 ? -1 : 0;
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

static uint32_t next_pow2_u32(uint32_t n)
{
    if (n == 0) return 1u;
    n--;
    n |= n >> 1; n |= n >> 2; n |= n >> 4; n |= n >> 8; n |= n >> 16;
    return n + 1u;
}

/* -------------------------------------------------------------------------
 * Per-PID accumulator helpers (shared by buffer and base flavors)
 * ---------------------------------------------------------------------- */

static uint32_t socket_acc_htable_slot(uint32_t pid, uint32_t cap)
{
    return (pid * 2654435761u) & (cap - 1u);
}

static void socket_acc_htable_rebuild(struct netdata_ebpf_socket_runtime *rt)
{
    memset(rt->acc_htable, 0, (size_t)rt->acc_htable_sz * sizeof(*rt->acc_htable));
    for (uint32_t i = 0; i < rt->acc_count; i++) {
        uint32_t h = socket_acc_htable_slot(rt->acc[i].pid, rt->acc_htable_sz);
        while (rt->acc_htable[h])
            h = (h + 1u) & (rt->acc_htable_sz - 1u);
        rt->acc_htable[h] = i + 1u;
    }
}

/* Find or create a per-PID accumulator entry.  Returns NULL only on OOM. */
static struct netdata_socket_per_pid_entry *
socket_acc_find_or_add(struct netdata_ebpf_socket_runtime *rt, uint32_t pid)
{
    if (!rt->acc_htable || rt->acc_htable_sz == 0)
        return NULL;

    uint32_t cap = rt->acc_htable_sz;
    uint32_t h   = socket_acc_htable_slot(pid, cap);

    while (rt->acc_htable[h]) {
        uint32_t idx = rt->acc_htable[h] - 1u;
        if (rt->acc[idx].pid == pid)
            return &rt->acc[idx];
        h = (h + 1u) & (cap - 1u);
    }

    /* Not found: grow arrays if needed, then insert. */
    if (rt->acc_count >= rt->acc_cap) {
        uint32_t new_cap = rt->acc_cap ? rt->acc_cap * 2u : 256u;
        rt->acc = reallocz(rt->acc, (size_t)new_cap * sizeof(*rt->acc));
        rt->acc_cap = new_cap;
    }

    /* Grow hash table when load factor would exceed 50%. */
    if (rt->acc_count + 1u > rt->acc_htable_sz / 2u) {
        uint32_t new_sz = rt->acc_htable_sz * 2u;
        rt->acc_htable = reallocz(rt->acc_htable, (size_t)new_sz * sizeof(*rt->acc_htable));
        rt->acc_htable_sz = new_sz;
        socket_acc_htable_rebuild(rt);
        cap = rt->acc_htable_sz;
        h   = socket_acc_htable_slot(pid, cap);
        while (rt->acc_htable[h])
            h = (h + 1u) & (cap - 1u);
    }

    struct netdata_socket_per_pid_entry *entry = &rt->acc[rt->acc_count];
    memset(entry, 0, sizeof(*entry));
    entry->pid = pid;
    rt->acc_htable[h] = rt->acc_count + 1u;
    rt->acc_count++;
    return entry;
}

/* -------------------------------------------------------------------------
 * Per-connection last-seen snapshot helpers (base flavor)
 * ---------------------------------------------------------------------- */

/* FNV-1a hash over the 40-byte connection key. */
static uint32_t conn_snap_hash(const netdata_socket_bpf_key_t *k, uint32_t sz)
{
    const uint8_t *b = (const uint8_t *)k;
    uint32_t h = 2166136261u;
    for (size_t i = 0; i < sizeof(*k); i++) {
        h ^= b[i];
        h *= 16777619u;
    }
    return h & (sz - 1u);
}

/* Find or allocate a slot for key.  Returns UINT32_MAX if table is full
 * (should never happen at ≤50% load).  New and recycled slots are fully
 * zeroed so callers always see in_use==0 and zero counter fields. */
static uint32_t conn_snap_upsert(socket_conn_snapshot_t *snap, uint32_t sz,
                                  const netdata_socket_bpf_key_t *key)
{
    if (!snap || sz == 0)
        return UINT32_MAX;
    uint32_t h = conn_snap_hash(key, sz);
    for (uint32_t i = 0; i < sz; i++) {
        uint32_t s = (h + i) & (sz - 1u);
        if (!snap[s].in_use) {
            /* Zero all fields so recycled slots never carry stale counters
             * into the next connection's first-interval delta. */
            memset(&snap[s], 0, sizeof(snap[s]));
            snap[s].key = *key;
            return s;
        }
        if (memcmp(&snap[s].key, key, sizeof(*key)) == 0)
            return s;
    }
    return UINT32_MAX;
}

/* Remove the entry at slot i using shift-back to keep all probe chains
 * contiguous.  Maintains the invariant: empty slot == end of chain. */
static void conn_snap_delete_at(socket_conn_snapshot_t *snap, uint32_t sz, uint32_t i)
{
    memset(&snap[i], 0, sizeof(snap[i]));   /* zero gap: key, counters, in_use */
    uint32_t j = (i + 1u) & (sz - 1u);
    while (snap[j].in_use) {
        uint32_t nat = conn_snap_hash(&snap[j].key, sz);
        uint32_t di  = (i - nat + sz) & (sz - 1u);
        uint32_t dj  = (j - nat + sz) & (sz - 1u);
        if (di <= dj) {
            snap[i] = snap[j];
            memset(&snap[j], 0, sizeof(snap[j])); /* zero vacated slot fully */
            i = j;
        }
        j = (j + 1u) & (sz - 1u);
    }
}

/* -------------------------------------------------------------------------
 * Ring-buffer accumulator (buffer flavor — socket_events)
 * ring_buffer__new is unavailable in old libbpf (0.0.9 / CentOS 7);
 * guard only the ring-buffer-specific code so old builds compile cleanly.
 * ---------------------------------------------------------------------- */

#if defined(LIBBPF_MAJOR_VERSION)

static int socket_rb_callback(void *ctx, void *data, size_t data_sz)
{
    if (data_sz < sizeof(netdata_socket_rb_event_t))
        return 0;

    struct netdata_ebpf_socket_runtime *rt = ctx;
    const netdata_socket_rb_event_t *ev    = data;

    uint32_t pid = ev->idx.pid;
    if (!pid)
        return 0;

    struct netdata_socket_per_pid_entry *entry = socket_acc_find_or_add(rt, pid);
    if (!entry)
        return 0;

    entry->bytes_sent             += ev->data.tcp.tcp_bytes_sent;
    entry->bytes_received         += ev->data.tcp.tcp_bytes_received;
    entry->call_tcp_sent          += ev->data.tcp.call_tcp_sent;
    entry->call_tcp_received      += ev->data.tcp.call_tcp_received;
    entry->retransmit             += ev->data.tcp.retransmit;
    entry->call_udp_sent          += ev->data.udp.call_udp_sent;
    entry->call_udp_received      += ev->data.udp.call_udp_received;
    entry->call_close             += ev->data.tcp.close;
    entry->call_tcp_v4_connection += ev->data.tcp.ipv4_connect;
    entry->call_tcp_v6_connection += ev->data.tcp.ipv6_connect;
    return 0;
}

static void socket_setup_ring_buffer(struct netdata_ebpf_socket_runtime *rt)
{
    struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "socket_events");
    if (!map) {
        fprintf(stderr, "ebpf-go.plugin: socket: ring buffer map 'socket_events' not found\n");
        return;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0) {
        fprintf(stderr, "ebpf-go.plugin: socket: ring buffer map fd invalid (%d)\n", fd);
        return;
    }

    rt->rb = ring_buffer__new(fd, socket_rb_callback, rt, NULL);
    if (!rt->rb)
        fprintf(stderr, "ebpf-go.plugin: socket: ring_buffer__new failed (errno %d)\n", errno);
}

static void socket_destroy_ring_buffer(struct netdata_ebpf_socket_runtime *rt)
{
    if (rt->rb) {
        ring_buffer__free(rt->rb);
        rt->rb = NULL;
    }
}

#endif /* LIBBPF_MAJOR_VERSION */

/* -------------------------------------------------------------------------
 * Public API
 * ---------------------------------------------------------------------- */

struct netdata_ebpf_socket_runtime *netdata_socket_runtime_open_mode(const char *path, int use_core)
{
    struct netdata_ebpf_socket_runtime *rt = callocz(1, sizeof(*rt));
    if (!rt)
        return NULL;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        freez(rt);
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
        if (m && bpf_map__set_max_entries(m, nd_socket_size) != 0)
            fprintf(stderr, "ebpf-go.plugin: socket: failed to resize tbl_nd_socket to %u\n", nd_socket_size);
    }
    if (nv_udp_size > 0) {
        struct bpf_map *m = bpf_object__find_map_by_name(rt->obj, "tbl_nv_udp");
        if (m && bpf_map__set_max_entries(m, nv_udp_size) != 0)
            fprintf(stderr, "ebpf-go.plugin: socket: failed to resize tbl_nv_udp to %u\n", nv_udp_size);
    }

    /* Allocate per-CPU buffer for tbl_global_sock snapshot reads.  Always size
     * for libbpf_num_possible_cpus() so the post-load type query in the
     * snapshot path can safely use either ARRAY (count=1) or PERCPU_ARRAY
     * (count=percpu_u64_cap) without a buffer overflow. */
    int ncpu = libbpf_num_possible_cpus();
    if (ncpu <= 0)
        ncpu = 1;

    rt->percpu_u64 = callocz((size_t)ncpu, sizeof(*rt->percpu_u64));
    if (!rt->percpu_u64)
        return -1;
    rt->percpu_u64_cap = ncpu;

    /* tbl_lports may be HASH or PERCPU_HASH depending on the binary and libbpf
     * version.  Always allocate for the maximum possible CPU count so both
     * variants are handled without a buffer overflow on lookup. */
    int lports_ncpu = libbpf_num_possible_cpus();
    if (lports_ncpu <= 0)
        lports_ncpu = 1;
    rt->percpu_passive = callocz((size_t)lports_ncpu, sizeof(*rt->percpu_passive));
    if (!rt->percpu_passive)
        return -1;
    rt->percpu_passive_cap = lports_ncpu;

    /* tbl_nd_socket may also be PERCPU_HASH — allocate per-CPU value buffer. */
    rt->percpu_nd_socket = callocz((size_t)lports_ncpu, sizeof(*rt->percpu_nd_socket));
    if (!rt->percpu_nd_socket)
        return -1;
    rt->percpu_nd_socket_cap = lports_ncpu;

    /* Per-connection last-seen snapshot and deferred-delete buffer: sized at
     * 2× max(nd_socket_size, SOCKET_CONN_SNAP_MIN) for ~50% load factor.
     * Guard against overflow before calling next_pow2_u32. */
    uint32_t snap_base = nd_socket_size > SOCKET_CONN_SNAP_MIN ? nd_socket_size : SOCKET_CONN_SNAP_MIN;
    if (snap_base > 0x40000000u)
        return -1;
    uint32_t snap_sz = next_pow2_u32(snap_base * 2u);
    rt->conn_snap_sz = snap_sz;
    rt->conn_snap = callocz(snap_sz, sizeof(*rt->conn_snap));
    if (!rt->conn_snap)
        return -1;

    /* Deferred-delete buffer: sized to the BPF map capacity so every stale
     * entry can be collected in a single pass with no silent overflow. */
    rt->nd_del_cap  = snap_sz / 2u;  /* = snap_base >= nd_socket_size */
    rt->nd_del_keys = callocz(rt->nd_del_cap, sizeof(*rt->nd_del_keys));
    if (!rt->nd_del_keys)
        return -1;

    /* Initial output array capacity = 256 unique PIDs (grows on demand). */
    rt->per_pid_cap = 256;
    rt->per_pid_entries = callocz((size_t)rt->per_pid_cap, sizeof(*rt->per_pid_entries));
    if (!rt->per_pid_entries)
        return -1;

    /* Per-PID accumulator: used by both buffer flavor (via ring-buffer events)
     * and base flavor (via conn_snap deltas).  Never reset between calls. */
    rt->acc_cap = 256u;
    rt->acc = callocz((size_t)rt->acc_cap, sizeof(*rt->acc));
    if (!rt->acc)
        return -1;
    rt->acc_htable_sz = 512u; /* 2× initial cap → ~50% max load factor */
    rt->acc_htable = callocz((size_t)rt->acc_htable_sz, sizeof(*rt->acc_htable));
    if (!rt->acc_htable)
        return -1;

    return 0;
}

int netdata_socket_runtime_load(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;
    int rc = bpf_object__load(rt->obj);
    if (rc != 0)
        return rc;
#if defined(LIBBPF_MAJOR_VERSION)
    /* Detect buffer flavor by the presence of the socket_events ring buffer map. */
    if (bpf_object__find_map_by_name(rt->obj, "socket_events"))
        socket_setup_ring_buffer(rt);
#endif
    return 0;
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

    /* Query the actual post-load map type so the read count is correct even
     * when bpf_map__set_type silently failed during prepare(). */
    enum bpf_map_type gtype = bpf_map__type(gmap);
    int count = (gtype == BPF_MAP_TYPE_PERCPU_ARRAY && rt->percpu_u64_cap > 0)
                ? rt->percpu_u64_cap : 1;
    (void)maps_per_core;
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

    freez(rt->percpu_u64);
    rt->percpu_u64 = NULL;

    freez(rt->percpu_passive);
    rt->percpu_passive = NULL;

    freez(rt->percpu_nd_socket);
    rt->percpu_nd_socket = NULL;

    freez(rt->conn_snap);
    rt->conn_snap    = NULL;
    rt->conn_snap_sz = 0;

    freez(rt->nd_del_keys);
    rt->nd_del_keys = NULL;
    rt->nd_del_cap  = 0;

    freez(rt->per_pid_entries);
    rt->per_pid_entries = NULL;
    rt->per_pid_count = 0;
    rt->per_pid_cap   = 0;

    freez(rt->acc);
    freez(rt->acc_htable);
    rt->acc           = NULL;
    rt->acc_htable    = NULL;
    rt->acc_cap       = 0;
    rt->acc_count     = 0;
    rt->acc_htable_sz = 0;

#if defined(LIBBPF_MAJOR_VERSION)
    socket_destroy_ring_buffer(rt);
#endif

    if (rt->obj)
        bpf_object__close(rt->obj);

    freez(rt);
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

/* Read tbl_nd_socket, accumulate per-connection deltas into rt->acc, and store
 * a sorted result array.
 * Returns pointer to the sorted array (owned by rt), count via *out_count.
 * Returns NULL on error. */
const struct netdata_socket_per_pid_entry *
netdata_socket_per_pid_snapshot(struct netdata_ebpf_socket_runtime *rt, int *out_count)
{
    if (!rt || !rt->obj || !out_count)
        return NULL;

#if defined(LIBBPF_MAJOR_VERSION)
    /* Buffer flavor: drain the ring-buffer accumulator and return its contents.
     * One ring_buffer__consume() drains all events queued since the last call.
     * The accumulator is NOT reset: it holds monotonically increasing per-PID
     * totals; Go's UpdateSocketApps computes per-cycle deltas by subtraction. */
    if (rt->rb) {
        ring_buffer__consume(rt->rb);

        /* Sort by PID to match the tbl_nd_socket base-flavor output contract. */
        if (rt->acc_count > 1u)
            qsort(rt->acc, (size_t)rt->acc_count, sizeof(*rt->acc), pid_ht_compare);

        /* Evict dead PIDs every SOCKET_ACC_EVICT_INTERVAL cycles to amortise the
         * per-PID kill() cost.  Dead entries accumulate for at most that many cycles
         * before eviction, producing zero-delta rows in the meantime.  EPERM means
         * alive (process exists but no signal permission); only ESRCH confirms gone. */
        rt->acc_evict_counter = (rt->acc_evict_counter + 1u) % SOCKET_ACC_EVICT_INTERVAL;
        if (rt->acc_evict_counter == 0u) {
            uint32_t live = 0;
            for (uint32_t i = 0; i < rt->acc_count; i++) {
                if (kill((pid_t)rt->acc[i].pid, 0) == 0 || errno == EPERM) {
                    if (live != i)
                        rt->acc[live] = rt->acc[i];
                    live++;
                }
            }
            rt->acc_count = live;
        }

        /* Grow the persistent output buffer if needed. */
        if ((int)rt->acc_count > rt->per_pid_cap) {
            rt->per_pid_entries = reallocz(rt->per_pid_entries,
                                           (size_t)rt->acc_count * sizeof(*rt->per_pid_entries));
            rt->per_pid_cap = (int)rt->acc_count;
        }
        if (rt->acc_count > 0u)
            memcpy(rt->per_pid_entries, rt->acc,
                   (size_t)rt->acc_count * sizeof(*rt->per_pid_entries));
        rt->per_pid_count = (int)rt->acc_count;
        *out_count = (int)rt->acc_count;

        /* Rebuild the htable index after qsort + eviction modified rt->acc.
         * The accumulator is NOT reset: monotonically increasing per-PID totals
         * let Go's UpdateSocketApps compute interval deltas by subtraction. */
        socket_acc_htable_rebuild(rt);

        return rt->per_pid_entries;
    }
#endif

    if (!rt->conn_snap)
        return NULL;

    struct bpf_map *ndmap = bpf_object__find_map_by_name(rt->obj, "tbl_nd_socket");
    if (!ndmap) {
        /* tbl_nd_socket is only present in the base object flavor; the
         * default "buffer" flavor uses a ring buffer and does not provide it.
         * Log once so operators know per-PID socket data is unavailable. */
        static bool warned = false;
        if (!warned) {
            warned = true;
            fprintf(stderr, "ebpf-go.plugin: socket: tbl_nd_socket not found; "
                    "per-PID socket metrics are unavailable with this object flavor\n");
        }
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

    /* Iterate all connection entries, compute per-connection deltas, and fold
     * them into rt->acc.  Deletions are deferred to after the loop — deleting
     * the predecessor key mid-iteration causes htab_get_next_key to restart
     * from the first bucket and double-count already-visited entries. */
    netdata_socket_bpf_key_t key = {}, next = {};
    netdata_socket_bpf_value_t *vbuf = rt->percpu_nd_socket;
    bool first_nditer = true;

    netdata_socket_bpf_key_t *del_keys = rt->nd_del_keys;
    uint32_t ndel = 0;
    struct {
        uint32_t pid;
        bool dead;
    } live_cache[SOCKET_PID_LIVENESS_CACHE] = {0};
    uint32_t live_cache_next = 0;

    while (bpf_map_get_next_key(ndfd, first_nditer ? NULL : &key, &next) == 0) {
        first_nditer = false;
        key = next;

        if (next.pid == 0 || bpf_map_lookup_elem(ndfd, &next, vbuf) != 0)
            continue;

        /* Defer deletion of unbound entries (both addresses all-zero).
         * Mirrors ebpf_socket.c:1860-1862. */
        static const uint8_t zero16[16] = {0};
        if (memcmp(next.saddr, zero16, 16) == 0 && memcmp(next.daddr, zero16, 16) == 0) {
            if (ndel < rt->nd_del_cap)
                del_keys[ndel++] = next;
            continue;
        }

        /* Liveness check: kill(ESRCH) confirms the process is gone.
         * EPERM means alive but unpermitted — keep those entries. */
        bool dead_pid = false;
        bool cached_pid = false;
        for (uint32_t i = 0; i < SOCKET_PID_LIVENESS_CACHE; i++) {
            if (live_cache[i].pid == next.pid) {
                dead_pid = live_cache[i].dead;
                cached_pid = true;
                break;
            }
        }
        if (!cached_pid) {
            dead_pid = kill((pid_t)next.pid, 0) != 0 && errno == ESRCH;
            live_cache[live_cache_next].pid  = next.pid;
            live_cache[live_cache_next].dead = dead_pid;
            live_cache_next = (live_cache_next + 1u) % SOCKET_PID_LIVENESS_CACHE;
        }

        /* Aggregate per-CPU values into a single entry. */
        netdata_socket_bpf_value_t agg;
        if (ndcount == 1) {
            agg = *vbuf;
        } else {
            memset(&agg, 0, sizeof(agg));
            for (int c = 0; c < ndcount; c++) {
                agg.tcp.tcp_bytes_sent     += vbuf[c].tcp.tcp_bytes_sent;
                agg.tcp.tcp_bytes_received += vbuf[c].tcp.tcp_bytes_received;
                agg.tcp.call_tcp_sent      += vbuf[c].tcp.call_tcp_sent;
                agg.tcp.call_tcp_received  += vbuf[c].tcp.call_tcp_received;
                agg.tcp.retransmit         += vbuf[c].tcp.retransmit;
                agg.udp.call_udp_sent      += vbuf[c].udp.call_udp_sent;
                agg.udp.call_udp_received  += vbuf[c].udp.call_udp_received;
                agg.tcp.close              += vbuf[c].tcp.close;
                agg.tcp.ipv4_connect       += vbuf[c].tcp.ipv4_connect;
                agg.tcp.ipv6_connect       += vbuf[c].tcp.ipv6_connect;
            }
        }

        /* A connection is discarded when it is closing (tcp.close > 0) or its
         * process is gone.  We fold the final delta first so totals in rt->acc
         * never decrease — the Go consumer computes per-interval deltas by
         * subtraction and clamps to zero, so any decrease becomes a lost event. */
        bool discard = dead_pid || (agg.tcp.close > 0);

        /* Find or allocate a last-seen slot.  conn_snap_upsert zeroes all counter
         * fields for new and recycled slots, so delta = current − 0 = current on
         * first use, matching the buffer-flavor first-ring-event semantic. */
        uint32_t cs_slot = conn_snap_upsert(rt->conn_snap, rt->conn_snap_sz, &next);
        socket_conn_snapshot_t *cs = (cs_slot != UINT32_MAX) ? &rt->conn_snap[cs_slot] : NULL;

        struct netdata_socket_per_pid_entry *acc_e = socket_acc_find_or_add(rt, next.pid);
        if (acc_e) {
            uint64_t pb_sent = cs ? cs->bytes_sent              : 0ULL;
            uint64_t pb_recv = cs ? cs->bytes_received          : 0ULL;
            uint32_t pt_sent = cs ? cs->call_tcp_sent           : 0u;
            uint32_t pt_recv = cs ? cs->call_tcp_received       : 0u;
            uint32_t p_retr  = cs ? cs->retransmit              : 0u;
            uint32_t pu_sent = cs ? cs->call_udp_sent           : 0u;
            uint32_t pu_recv = cs ? cs->call_udp_received       : 0u;
            uint32_t p_cls   = cs ? cs->call_close              : 0u;
            uint32_t p_v4    = cs ? cs->call_tcp_v4_connection  : 0u;
            uint32_t p_v6    = cs ? cs->call_tcp_v6_connection  : 0u;

#define D64(c, p) ((c) >= (p) ? (c) - (p) : 0ULL)
/* Clamp-on-decrease, matching D64, socketDelta, and socketIntervalDelta policy.
 * Any apparent decrease (stale prev, counter reset) produces 0, not a huge
 * wrap-around value that would explode user charts. */
#define D32(c, p) ((uint32_t)(c) >= (uint32_t)(p) \
                   ? (uint64_t)((uint32_t)(c) - (uint32_t)(p)) : 0ULL)
            acc_e->bytes_sent             += D64(agg.tcp.tcp_bytes_sent,    pb_sent);
            acc_e->bytes_received         += D64(agg.tcp.tcp_bytes_received, pb_recv);
            acc_e->call_tcp_sent          += D32(agg.tcp.call_tcp_sent,     pt_sent);
            acc_e->call_tcp_received      += D32(agg.tcp.call_tcp_received,  pt_recv);
            acc_e->retransmit             += D32(agg.tcp.retransmit,         p_retr);
            acc_e->call_udp_sent          += D32(agg.udp.call_udp_sent,     pu_sent);
            acc_e->call_udp_received      += D32(agg.udp.call_udp_received,  pu_recv);
            acc_e->call_close             += D32(agg.tcp.close,             p_cls);
            acc_e->call_tcp_v4_connection += D32(agg.tcp.ipv4_connect,      p_v4);
            acc_e->call_tcp_v6_connection += D32(agg.tcp.ipv6_connect,      p_v6);
#undef D64
#undef D32
        }

        if (discard) {
            if (cs)
                conn_snap_delete_at(rt->conn_snap, rt->conn_snap_sz, cs_slot);
            if (ndel < rt->nd_del_cap)
                del_keys[ndel++] = next;
        } else if (cs) {
            cs->bytes_sent             = agg.tcp.tcp_bytes_sent;
            cs->bytes_received         = agg.tcp.tcp_bytes_received;
            cs->call_tcp_sent          = agg.tcp.call_tcp_sent;
            cs->call_tcp_received      = agg.tcp.call_tcp_received;
            cs->retransmit             = agg.tcp.retransmit;
            cs->call_udp_sent          = agg.udp.call_udp_sent;
            cs->call_udp_received      = agg.udp.call_udp_received;
            cs->call_close             = agg.tcp.close;
            cs->call_tcp_v4_connection = agg.tcp.ipv4_connect;
            cs->call_tcp_v6_connection = agg.tcp.ipv6_connect;
            cs->in_use = 1;
        }
    }

    /* Apply deferred deletions now that traversal is complete. */
    for (uint32_t i = 0; i < ndel; i++)
        bpf_map_delete_elem(ndfd, &del_keys[i]);

    /* Evict dead PIDs from the accumulator every SOCKET_ACC_EVICT_INTERVAL cycles
     * to amortise the per-PID kill() cost.  Mirrors the buffer-flavor eviction. */
    rt->acc_evict_counter = (rt->acc_evict_counter + 1u) % SOCKET_ACC_EVICT_INTERVAL;
    if (rt->acc_evict_counter == 0u) {
        uint32_t live = 0;
        for (uint32_t i = 0; i < rt->acc_count; i++) {
            if (kill((pid_t)rt->acc[i].pid, 0) == 0 || errno == EPERM) {
                if (live != i)
                    rt->acc[live] = rt->acc[i];
                live++;
            }
        }
        rt->acc_count = live;
    }

    if (rt->acc_count > 1u)
        qsort(rt->acc, (size_t)rt->acc_count, sizeof(*rt->acc), pid_ht_compare);

    if ((int)rt->acc_count > rt->per_pid_cap) {
        rt->per_pid_entries = reallocz(rt->per_pid_entries,
                                       (size_t)rt->acc_count * sizeof(*rt->per_pid_entries));
        rt->per_pid_cap = (int)rt->acc_count;
    }
    if (rt->acc_count > 0u)
        memcpy(rt->per_pid_entries, rt->acc,
               (size_t)rt->acc_count * sizeof(*rt->per_pid_entries));
    rt->per_pid_count = (int)rt->acc_count;
    *out_count = (int)rt->acc_count;

    /* Rebuild htable after qsort + eviction reordered rt->acc. */
    socket_acc_htable_rebuild(rt);
    return rt->per_pid_entries;
}
