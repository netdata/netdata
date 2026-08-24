// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_EBPF_RUNTIME_COMMON_H
#define NETDATA_ND_EBPF_RUNTIME_COMMON_H 1

/* Machinery shared by the CGo eBPF runtimes in this package.
 *
 * Every module opens a BPF object, flips map types, reads a per-CPU global map,
 * and (for the buffer/arena flavors) accumulates per-TGID events in userspace.
 * That code is identical apart from map names and the per-module counter fields,
 * so it lives here once instead of once per module.
 *
 * Header-only on purpose: CGo compiles each *.c in the package as its own
 * translation unit, and static inline definitions keep the linker out of it. */

#include <errno.h>
#include <stdbool.h>
#include <stddef.h> /* offsetof, used by ND_EBPF_ASSERT_PID_FIRST */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

#include "../nd_alloc_shim.h"

/* ------------------------------------------------------------------------
 * libbpf compatibility
 *
 * libbpf 0.0.9 (CentOS 7) does not define LIBBPF_MAJOR_VERSION and lacks
 * several APIs added in later releases.
 * ------------------------------------------------------------------------ */
#ifndef LIBBPF_MAJOR_VERSION
static inline int bpf_program__set_autoload(struct bpf_program *prog, bool autoload)
{
    /* No autoload API in old libbpf; all programs load unconditionally.
     * Legacy .bpf.o files for old kernels do not contain fentry/CO-RE
     * programs, so missing this call is harmless. */
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
    /* bpf_map__def() is const-qualified but the map is mutable before load */
    ((struct bpf_map_def *)bpf_map__def(map))->type = type;
    return 0;
}

static inline int bpf_map__set_max_entries(struct bpf_map *map, __u32 max_entries)
{
    return bpf_map__resize(map, max_entries);
}
#endif /* !LIBBPF_MAJOR_VERSION */

/* ------------------------------------------------------------------------
 * Controller map keys (mirrors enum netdata_controller in the BPF sources)
 * ------------------------------------------------------------------------ */
enum nd_ebpf_controller {
    ND_EBPF_CONTROLLER_APPS_ENABLED = 0,
    ND_EBPF_CONTROLLER_APPS_LEVEL = 1,
    ND_EBPF_CONTROLLER_PID_TABLE_ADD = 2,
    ND_EBPF_CONTROLLER_PID_TABLE_DEL = 3,
    ND_EBPF_CONTROLLER_TEMP_TABLE_ADD = 4,
    ND_EBPF_CONTROLLER_TEMP_TABLE_DEL = 5,
    ND_EBPF_CONTROLLER_END = 6,
};

/* NETDATA_APPS_LEVEL_ALL (2): the BPF key is the TID and values[].tgid holds the
 * process TGID.  cgroup.procs lists TGIDs, so shared memory must be keyed by
 * TGID rather than by the parent's TGID that REAL_PARENT (0) would produce. */
#define ND_EBPF_APPS_LEVEL_ALL 2

/* ------------------------------------------------------------------------
 * Small shared helpers
 * ------------------------------------------------------------------------ */
static inline uint64_t nd_ebpf_sum_percpu_u64(const uint64_t *values, int count)
{
    uint64_t total = 0;
    for (int i = 0; i < count; i++)
        total += values[i];
    return total;
}

/* qsort comparator for any per-PID snapshot struct whose first member is the
 * uint32_t pid.  Each runtime static-asserts that offset with
 * ND_EBPF_ASSERT_PID_FIRST() so this stays safe. */
static inline int nd_ebpf_pid_first_u32_cmp(const void *a, const void *b)
{
    uint32_t pa, pb;
    memcpy(&pa, a, sizeof(pa));
    memcpy(&pb, b, sizeof(pb));
    if (pa < pb) return -1;
    if (pa > pb) return 1;
    return 0;
}

#define ND_EBPF_ASSERT_PID_FIRST(type) \
    _Static_assert(offsetof(type, pid) == 0, #type " must start with its uint32_t pid for nd_ebpf_pid_first_u32_cmp")

static inline void nd_ebpf_destroy_links(struct bpf_link ***links, size_t count)
{
    if (!links || !*links)
        return;

    for (size_t i = 0; i < count; i++) {
        /* Attach helpers return error-encoded pointers, not NULL, on failure, and
         * callers pass the whole array here after spotting one.  libbpf only grew
         * the IS_ERR_OR_NULL guard inside bpf_link__destroy() in 1.0, while
         * NetdataLibBPF.cmake still selects the 0.x fork on kernels below 4.14 and
         * under FORCE_LEGACY_LIBBPF — so filter error pointers here rather than
         * relying on the library version. */
        if ((*links)[i] && !libbpf_get_error((*links)[i]))
            bpf_link__destroy((*links)[i]);
    }

    freez(*links);
    *links = NULL;
}

/* ------------------------------------------------------------------------
 * Per-PID map-walk helpers
 *
 * cachestat and dcstat walk their per-PID BPF map identically; only the counter
 * fields differ.  The field-agnostic steps live here so the per-module loop keeps
 * typed field access, which is where readability actually matters.
 * ------------------------------------------------------------------------ */

/* Resolves the TGID for one map entry.
 *
 * With ND_EBPF_APPS_LEVEL_ALL the BPF key is the thread ID and the process TGID
 * lives in the per-CPU values.  Shared memory must be keyed by TGID so
 * cgroup.procs lookups succeed.  Falls back to the map key only when every
 * per-CPU copy still reads 0, which is the race at entry creation — counters are
 * zero then, so the entry is harmless. */
static inline uint32_t nd_ebpf_snapshot_tgid(
    const void *values,
    int per_cpu_count,
    size_t value_size,
    size_t tgid_offset,
    uint32_t fallback_key)
{
    for (int i = 0; i < per_cpu_count; i++) {
        uint32_t tgid;
        memcpy(&tgid, (const char *)values + (size_t)i * value_size + tgid_offset, sizeof(tgid));
        if (tgid != 0)
            return tgid;
    }
    return fallback_key;
}

/* Ensures the persistent output buffer can hold one more item, doubling on
 * demand.  Capacity persists across cycles, so the steady state allocates
 * nothing. */
static inline void nd_ebpf_snapshot_reserve(void **items, size_t *cap, size_t item_size, size_t needed)
{
    if (needed < *cap)
        return;

    size_t new_cap = *cap ? *cap * 2 : 64;
    *items = reallocz(*items, new_cap * item_size);
    *cap = new_cap;
}

/* Folds one already-accumulated item into another with the same pid. */
typedef void (*nd_ebpf_snapshot_merge_fn)(void *dst, const void *src);

/* Collapses per-thread rows into one row per process.
 *
 * Threads of the same process produce separate map entries carrying the same
 * TGID, so the buffer is sorted by pid and consecutive duplicates are merged in
 * place.  Requires pid to be the first member of the item type — callers assert
 * that with ND_EBPF_ASSERT_PID_FIRST.  Returns the surviving item count. */
static inline size_t nd_ebpf_snapshot_merge_same_pid(
    void *items,
    size_t count,
    size_t item_size,
    nd_ebpf_snapshot_merge_fn merge)
{
    if (count < 2)
        return count;

    qsort(items, count, item_size, nd_ebpf_pid_first_u32_cmp);

    size_t merged = 0;
    for (size_t i = 0; i < count; i++) {
        char *cur = (char *)items + i * item_size;
        char *last = (char *)items + (merged ? merged - 1 : 0) * item_size;

        uint32_t cur_pid, last_pid;
        memcpy(&cur_pid, cur, sizeof(cur_pid));
        memcpy(&last_pid, last, sizeof(last_pid));

        if (merged == 0 || last_pid != cur_pid) {
            char *dst = (char *)items + merged * item_size;
            if (dst != cur)
                memcpy(dst, cur, item_size);
            merged++;
        } else {
            merge(last, cur);
        }
    }
    return merged;
}

/* Copies a TASK_COMM_LEN name into a wider NUL-terminated destination. */
static inline void nd_ebpf_copy_comm(char *dst, size_t dst_size, const char *src, size_t src_size)
{
    size_t len = strnlen(src, src_size);
    if (len > dst_size - 1)
        len = dst_size - 1;
    memset(dst, 0, dst_size);
    memcpy(dst, src, len);
}

/* Allocates the persistent per-CPU work buffers a runtime reuses on every
 * snapshot.  Always sized for libbpf_num_possible_cpus() so the post-load map
 * type re-query can read either an ARRAY (count 1) or a PERCPU_ARRAY (count
 * ncpu) without overflowing. */
static inline int nd_ebpf_alloc_percpu_buffers(
    uint64_t **percpu_u64,
    int *percpu_u64_cap,
    void **percpu_entries,
    int *percpu_entries_cap,
    size_t entry_size)
{
    int ncpu = libbpf_num_possible_cpus();
    if (ncpu < 1)
        ncpu = 1;

    *percpu_u64 = callocz((size_t)ncpu, sizeof(**percpu_u64));
    *percpu_u64_cap = ncpu;

    *percpu_entries = callocz((size_t)ncpu, entry_size);
    *percpu_entries_cap = ncpu;

    return 0;
}

/* ------------------------------------------------------------------------
 * Per-TGID event accumulator
 *
 * The buffer and arena flavors emit events instead of maintaining a per-PID BPF
 * map, so the counters are accumulated in userspace.  The table is generic over
 * the item type: callers supply the item size and the offset of the uint32_t
 * tgid inside it, keeping their own counter fields.
 * ------------------------------------------------------------------------ */
struct nd_ebpf_acc_table {
    void *items;         /* dense array of count items, each item_size bytes */
    size_t item_size;
    size_t tgid_offset;  /* offsetof(item_type, tgid) */
    size_t count;
    size_t cap;
    size_t max_entries; /* 0 = no limit; set from the configured PID table size */
    uint64_t dropped;   /* new TGIDs rejected after reaching max_entries */
    /* slot stores (index + 1), 0 = empty.  Rebuilt from scratch on every
     * structural change, so no tombstones are needed. */
    uint32_t *htable;
    size_t htable_sz;    /* power of two */
};

static inline void nd_ebpf_acc_init(struct nd_ebpf_acc_table *t, size_t item_size, size_t tgid_offset)
{
    memset(t, 0, sizeof(*t));
    t->item_size = item_size;
    t->tgid_offset = tgid_offset;
}

static inline void nd_ebpf_acc_set_max_entries(struct nd_ebpf_acc_table *t, size_t max_entries)
{
    t->max_entries = max_entries;
}

static inline void *nd_ebpf_acc_item(const struct nd_ebpf_acc_table *t, size_t index)
{
    return (char *)t->items + index * t->item_size;
}

static inline uint32_t nd_ebpf_acc_tgid(const struct nd_ebpf_acc_table *t, size_t index)
{
    uint32_t tgid;
    memcpy(&tgid, (char *)nd_ebpf_acc_item(t, index) + t->tgid_offset, sizeof(tgid));
    return tgid;
}

/* Knuth multiplicative hash — good dispersion for sequential PID values. */
static inline size_t nd_ebpf_acc_slot(uint32_t tgid, size_t sz)
{
    return (size_t)(((uint64_t)tgid * UINT64_C(2654435761)) >> 32) & (sz - 1);
}

/* Rebuilds the hash table from the current items.  Called after every
 * structural change (growth or eviction).  O(count) — acceptable because
 * evictions are rare and growth is amortised. */
static inline void nd_ebpf_acc_rebuild(struct nd_ebpf_acc_table *t)
{
    /* capacity = next power-of-2 >= 4 x count (load factor <= 0.25) */
    size_t need = t->count < 16 ? 64 : t->count * 4;
    size_t cap = 64;
    while (cap < need)
        cap <<= 1;

    if (cap != t->htable_sz) {
        t->htable = reallocz(t->htable, cap * sizeof(*t->htable));
        t->htable_sz = cap;
    }
    memset(t->htable, 0, cap * sizeof(*t->htable));

    for (size_t i = 0; i < t->count; i++) {
        size_t h = nd_ebpf_acc_slot(nd_ebpf_acc_tgid(t, i), cap);
        while (t->htable[h])
            h = (h + 1) & (cap - 1);
        t->htable[h] = (uint32_t)(i + 1);
    }
}

/* O(1) amortised: hash lookup, appending a zeroed item on a new TGID.
 *
 * LIFETIME: the returned pointer is valid only until the next call on this
 * table.  Growth reallocs t->items, so a caller that stashes the pointer and
 * writes through it after another find_or_add/evict writes into freed memory.
 * Use it within the current event callback and re-look-up otherwise. */
static inline void *nd_ebpf_acc_find_or_add(struct nd_ebpf_acc_table *t, uint32_t tgid)
{
    /* nd_ebpf_acc_init() must run before the first event: with item_size 0 the
     * growth path would allocate nothing and the tgid write would corrupt the
     * heap.  Refuse instead — callers treat NULL as "drop this event". */
    if (!t->item_size) {
        fprintf(stderr, "ebpf-go: accumulator used before nd_ebpf_acc_init()\n");
        return NULL;
    }

    /* Initialise the index before looking for an existing TGID. */
    if (!t->htable)
        nd_ebpf_acc_rebuild(t);

    size_t cap = t->htable_sz;
    size_t h = nd_ebpf_acc_slot(tgid, cap);

    while (t->htable[h]) {
        size_t idx = t->htable[h] - 1;
        if (nd_ebpf_acc_tgid(t, idx) == tgid)
            return nd_ebpf_acc_item(t, idx);
        h = (h + 1) & (cap - 1);
    }

    if (t->max_entries && t->count >= t->max_entries) {
        t->dropped++;
        if (t->dropped == 1 || !(t->dropped & 1023))
            fprintf(stderr, "ebpf-go: per-TGID accumulator is full (%zu entries); dropped %llu new TGIDs\n",
                    t->max_entries, (unsigned long long)t->dropped);
        return NULL;
    }

    /* Grow only after confirming this is a new accepted TGID.  In particular,
     * rejected TGIDs at the configured limit must not make the hash table grow. */
    if (t->count + 1 > t->htable_sz / 2) {
        nd_ebpf_acc_rebuild(t);
        cap = t->htable_sz;
        h = nd_ebpf_acc_slot(tgid, cap);
        while (t->htable[h])
            h = (h + 1) & (cap - 1);
    }

    if (t->count >= t->cap) {
        size_t new_cap = t->cap ? t->cap * 2 : 64;
        t->items = reallocz(t->items, new_cap * t->item_size);
        t->cap = new_cap;
        /* items may have moved — rebuild and re-probe. */
        nd_ebpf_acc_rebuild(t);
        cap = t->htable_sz;
        h = nd_ebpf_acc_slot(tgid, cap);
        while (t->htable[h])
            h = (h + 1) & (cap - 1);
    }

    void *entry = nd_ebpf_acc_item(t, t->count);
    memset(entry, 0, t->item_size);
    memcpy((char *)entry + t->tgid_offset, &tgid, sizeof(tgid));
    t->htable[h] = (uint32_t)(t->count + 1);
    t->count++;
    return entry;
}

/* Removes a TGID so dead processes do not inflate the accumulator.  Uses
 * swap-with-last for O(1) removal, then rebuilds the hash table. */
static inline void nd_ebpf_acc_evict_tgid(struct nd_ebpf_acc_table *t, uint32_t tgid)
{
    if (!t->htable || t->count == 0)
        return;

    size_t cap = t->htable_sz;
    size_t h = nd_ebpf_acc_slot(tgid, cap);

    while (t->htable[h]) {
        size_t idx = t->htable[h] - 1;
        if (nd_ebpf_acc_tgid(t, idx) == tgid) {
            size_t last = t->count - 1;
            if (idx != last)
                memcpy(nd_ebpf_acc_item(t, idx), nd_ebpf_acc_item(t, last), t->item_size);
            t->count--;
            /* Full rebuild: clears the deleted slot and fixes the moved item. */
            nd_ebpf_acc_rebuild(t);
            return;
        }
        h = (h + 1) & (cap - 1);
    }
}

static inline void nd_ebpf_acc_free(struct nd_ebpf_acc_table *t)
{
    freez(t->items);
    freez(t->htable);
    t->items = NULL;
    t->htable = NULL;
    t->count = 0;
    t->cap = 0;
    t->htable_sz = 0;
}

/* ------------------------------------------------------------------------
 * Ring buffer / arena event transport
 * ------------------------------------------------------------------------ */

/* Event layout emitted by the buffer and arena BPF programs.  It is shared:
 * netdata_cachestat_event_t and netdata_dc_event_t are byte-identical (48
 * bytes), verified against the arena skeletons' _Static_assert on the state size
 * and against the field offsets in the compiled objects.  action is interpreted
 * per module. */
struct nd_ebpf_pid_event {
    uint64_t ct;
    uint32_t pid;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char     name[16]; /* TASK_COMM_LEN */
    uint8_t  action;
    uint8_t  pad[3];
};

_Static_assert(sizeof(struct nd_ebpf_pid_event) == 48, "nd_ebpf_pid_event must match the BPF-side event layout");

/* NETDATA_ARENA_EVENT_SLOTS in ebpf-co-re/includes/netdata_arena_common.h */
#define ND_EBPF_ARENA_EVENT_SLOTS 1024

/* Layout of the NETDATA_ARENA_QUEUE_DECL state: a head counter followed by the
 * slot ring.  The 4 bytes of padding after head come from the events[] 8-byte
 * alignment. */
struct nd_ebpf_arena_state {
    uint32_t head;
    uint32_t _pad;
    struct nd_ebpf_pid_event events[ND_EBPF_ARENA_EVENT_SLOTS];
};

_Static_assert(sizeof(struct nd_ebpf_arena_state) == 49160, "nd_ebpf_arena_state does not match the BPF-side layout");

typedef void (*nd_ebpf_event_fn)(void *ctx, const struct nd_ebpf_pid_event *ev);

/* Consumes every slot published since tail and returns the new tail.
 *
 * NOT re-entrant: fn must not drain the same arena_state.  head is sampled once,
 * so a nested drain would consume events this call still has to replay and the
 * outer loop would process them twice. */
static inline uint32_t nd_ebpf_arena_drain(const void *arena_state, uint32_t tail, nd_ebpf_event_fn fn, void *ctx)
{
    if (!arena_state)
        return tail;

    const struct nd_ebpf_arena_state *state = (const struct nd_ebpf_arena_state *)arena_state;
    uint32_t head = __atomic_load_n(&state->head, __ATOMIC_ACQUIRE);

    while (tail != head) {
        fn(ctx, &state->events[tail % ND_EBPF_ARENA_EVENT_SLOTS]);
        tail++;
    }

    return head;
}

/* Opens the named ring buffer map.  module is used only for error reporting. */
static inline struct ring_buffer *nd_ebpf_ring_buffer_open(
    struct bpf_object *obj,
    const char *map_name,
    ring_buffer_sample_fn sample_fn,
    void *ctx,
    const char *module)
{
    if (!obj)
        return NULL;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, map_name);
    if (!map) {
        fprintf(stderr, "ebpf-go: %s ring buffer map '%s' not found\n", module, map_name);
        return NULL;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0) {
        fprintf(stderr, "ebpf-go: %s ring buffer map fd invalid (%d)\n", module, fd);
        return NULL;
    }

    struct ring_buffer *rb = ring_buffer__new(fd, sample_fn, ctx, NULL);
    if (!rb)
        fprintf(stderr, "ebpf-go: ring_buffer__new failed for %s (errno %d)\n", module, errno);

    return rb;
}

/* ------------------------------------------------------------------------
 * Map operations
 * ------------------------------------------------------------------------ */

/* Writes apps_enabled / apps_level into a module's controller map, handling both
 * the per-CPU and the single-value map shapes. */
static inline int nd_ebpf_update_controller(
    struct bpf_object *obj,
    const char *ctrl_map_name,
    int apps_enabled,
    int apps_level)
{
    if (!obj)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, ctrl_map_name);
    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    /* controller maps store __u64 values; use uint64_t to match exactly */
    const uint64_t values[ND_EBPF_CONTROLLER_END] = {
        apps_enabled ? 1ULL : 0ULL,
        (uint64_t)apps_level,
        0,
        0,
        0,
        0,
    };
    const enum bpf_map_type type = bpf_map__type(map);
    const bool is_percpu = (type == BPF_MAP_TYPE_PERCPU_ARRAY || type == BPF_MAP_TYPE_PERCPU_HASH);

    for (uint32_t key = ND_EBPF_CONTROLLER_APPS_ENABLED; key < ND_EBPF_CONTROLLER_PID_TABLE_ADD; key++) {
        if (!is_percpu) {
            if (bpf_map_update_elem(fd, &key, &values[key], BPF_ANY))
                return -1;
            continue;
        }

        const int cpus = libbpf_num_possible_cpus();
        if (cpus <= 0)
            return -1;

        uint64_t *percpu = callocz((size_t)cpus, sizeof(*percpu));
        if (!percpu)
            return -1;

        for (int cpu = 0; cpu < cpus; cpu++)
            percpu[cpu] = values[key];

        const int rc = bpf_map_update_elem(fd, &key, percpu, BPF_ANY);
        freez(percpu);

        if (rc)
            return -1;
    }

    return 0;
}

/*
 * Bulk-deletes PIDs from a per-PID map.  Prefers bpf_map_delete_batch
 * (kernel >= 5.6) and falls back to a tight loop of bpf_map_delete_elem.
 *
 * map_missing is set when the object has no such map, which is the normal case
 * for the buffer/arena flavors: the caller then evicts from its userspace
 * accumulator instead.
 */
static inline int nd_ebpf_map_delete_pids(
    struct bpf_object *obj,
    const char *map_name,
    uint32_t *pids,
    size_t count,
    bool *map_missing)
{
    *map_missing = false;

    if (!obj || !pids || count == 0)
        return 0;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, map_name);
    if (!map) {
        *map_missing = true;
        return 0;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    /* Fall back to per-key delete on ENOSYS (older kernels) or
     * EINVAL/EOPNOTSUPP (the kernel rejected the batch shape).  ENOENT also
     * falls through because htab_map_delete_batch stops at the first missing
     * key — the remaining PIDs are still deletable individually.
     *
     * bpf_map_delete_batch takes __u32 count, not size_t; on overflow we skip
     * the batch and go straight to the loop. */
    if (count <= UINT32_MAX) {
        uint32_t batch_count = (uint32_t)count;
        int rc = bpf_map_delete_batch(fd, pids, &batch_count, NULL);
        if (rc == 0)
            return 0;
        if (rc != -ENOSYS && rc != -EINVAL && rc != -EOPNOTSUPP && rc != -ENOENT)
            return rc;
    }

    for (size_t i = 0; i < count; i++) {
        int per = bpf_map_delete_elem(fd, &pids[i]);
        /* Treat "not found" as success: the PID may have been removed by the
         * BPF program between snapshot and delete. */
        if (per != 0 && per != -ENOENT)
            return per;
    }
    return 0;
}

/* Deletes one PID from a per-PID map; see nd_ebpf_map_delete_pids for
 * map_missing. */
static inline int nd_ebpf_map_delete_pid(
    struct bpf_object *obj,
    const char *map_name,
    uint32_t pid,
    bool *map_missing)
{
    *map_missing = false;

    if (!obj)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, map_name);
    if (!map) {
        *map_missing = true;
        return 0;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    return bpf_map_delete_elem(fd, &pid);
}

/* Converts the requested per-core preference into the map types the object must
 * use before load.  Returns -1 when a needed conversion failed, which the caller
 * must treat as fatal: a buffer sized for a non-percpu map is too small for a
 * percpu kernel write. */
static inline int nd_ebpf_update_map_types(
    struct bpf_object *obj,
    const char *const *map_names,
    size_t map_count,
    int maps_per_core,
    const char *module)
{
    struct bpf_map *map;
    bpf_object__for_each_map(map, obj)
    {
        const char *name = bpf_map__name(map);
        bool wanted = false;
        for (size_t i = 0; i < map_count && !wanted; i++)
            wanted = !strcmp(name, map_names[i]);
        if (!wanted)
            continue;

        enum bpf_map_type type = bpf_map__type(map);
        int ret = 0;
        if (maps_per_core) {
            if (type == BPF_MAP_TYPE_HASH)
                ret = bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_HASH);
            else if (type == BPF_MAP_TYPE_ARRAY)
                ret = bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_ARRAY);
        } else {
            if (type == BPF_MAP_TYPE_PERCPU_HASH)
                ret = bpf_map__set_type(map, BPF_MAP_TYPE_HASH);
            else if (type == BPF_MAP_TYPE_PERCPU_ARRAY)
                ret = bpf_map__set_type(map, BPF_MAP_TYPE_ARRAY);
        }
        if (ret != 0) {
            fprintf(stderr,
                    "ebpf-go.plugin: %s: bpf_map__set_type failed for map '%s': %d; refusing to load\n",
                    module, name, ret);
            return -1;
        }
    }
    return 0;
}

#endif /* NETDATA_ND_EBPF_RUNTIME_COMMON_H */
