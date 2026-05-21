//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <errno.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

/*
 * libbpf 0.0.9 (CentOS 7) does not define LIBBPF_MAJOR_VERSION and lacks
 * several APIs added in later releases.  Provide inline shims so the rest
 * of this file compiles unchanged on both old and new libbpf.
 */
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

static inline __u32 bpf_map__max_entries(const struct bpf_map *map)
{
    return bpf_map__def(map)->max_entries;
}
#endif /* !LIBBPF_MAJOR_VERSION */

#if defined(LIBBPF_MAJOR_VERSION) && (LIBBPF_MAJOR_VERSION >= 1) && defined(__has_include) && __has_include(<linux/btf.h>)
/*
 * The repo-local CO-RE bundle currently ships the skeleton wrapper but not the
 * arena BPF-side state type header. Keep the shim size aligned with the
 * generated skeleton so the Go collector can still build in this workspace.
 */
#ifndef NETDATA_CACHSTAT_ARENA_STATE_T_DEFINED
struct netdata_cachestat_arena_state_t {
    unsigned char __pad[49160];
};
#endif

#include "cachestat.skel.h"
#include "cachestat_buffer.skel.h"
#include "cachestat_arena.skel.h"
#define NETDATA_LIBBPF_CORE_SUPPORTED 1
#endif

enum netdata_ebpf_cachestat_runtime_kind {
    NETDATA_CACHESTAT_RUNTIME_LEGACY = 0,
    NETDATA_CACHESTAT_RUNTIME_CORE = 1,
};

enum netdata_ebpf_cachestat_runtime_flavor {
    NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE = 0,
    NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER = 1,
    NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA = 2,
};

struct netdata_ebpf_cachestat_runtime {
    int kind;
    int flavor;
    struct bpf_object *obj;
    struct bpf_link **links;
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    union {
        struct cachestat_bpf *base;
        struct cachestat_buffer_bpf *buffer;
        struct cachestat_arena_bpf *arena;
    } core;
    /* Event consumer and per-TGID accumulator (buffer and arena flavors).
     * These flavors emit ring buffer / arena events rather than writing
     * directly to cstat_pid, so per-app counters are accumulated here. */
    struct ring_buffer *rb;        /* non-NULL for buffer flavor */
    void              *arena_state; /* mmap'd arena pointer (arena flavor) */
    uint32_t           arena_tail;  /* last consumed head for arena flavor */
    struct netdata_ebpf_cachestat_pid_entry *acc;
    size_t acc_cap;
    size_t acc_count;
#endif
};

struct netdata_ebpf_cachestat_snapshot {
    uint64_t mark_page_accessed;
    uint64_t mark_buffer_dirty;
    uint64_t add_to_page_cache_lru;
    uint64_t account_page_dirtied;
};

struct netdata_ebpf_cachestat_pid_snapshot {
    uint32_t pid;
    uint32_t ppid;
    uint64_t ct;
    char comm[96];
    uint32_t add_to_page_cache_lru;
    uint32_t mark_page_accessed;
    uint32_t account_page_dirtied;
    uint32_t mark_buffer_dirty;
};

struct netdata_ebpf_cachestat_pid_snapshot_list {
    struct netdata_ebpf_cachestat_pid_snapshot *items;
    size_t count;
};

int netdata_cachestat_runtime_supports_core(void);

enum netdata_controller {
    NETDATA_CONTROLLER_APPS_ENABLED = 0,
    NETDATA_CONTROLLER_APPS_LEVEL = 1,
    NETDATA_CONTROLLER_PID_TABLE_ADD = 2,
    NETDATA_CONTROLLER_PID_TABLE_DEL = 3,
    NETDATA_CONTROLLER_TEMP_TABLE_ADD = 4,
    NETDATA_CONTROLLER_TEMP_TABLE_DEL = 5,
    NETDATA_CONTROLLER_END = 6,
};

static const char *cachestat_account_program_name(const char *account_function, int flavor)
{
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER || flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA) {
        if (!strcmp(account_function, "__folio_mark_dirty"))
            return "netdata_folio_mark_dirty_buffer";
        if (!strcmp(account_function, "__set_page_dirty"))
            return "netdata_set_page_dirty_buffer";
        return "netdata_account_page_dirtied_buffer";
    }

    if (!strcmp(account_function, "__folio_mark_dirty"))
        return "netdata_folio_mark_dirty_kprobe";
    if (!strcmp(account_function, "__set_page_dirty"))
        return "netdata_set_page_dirty_kprobe";
#else
    (void)account_function;
    (void)flavor;
#endif
    return "netdata_account_page_dirtied_kprobe";
}

static const char *cachestat_data_program_name(const char *event_name, int flavor)
{
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER || flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA) {
        if (!strcmp(event_name, "add_to_page_cache_lru"))
            return "netdata_add_to_page_cache_lru_buffer";
        if (!strcmp(event_name, "mark_page_accessed"))
            return "netdata_mark_page_accessed_buffer";
        if (!strcmp(event_name, "mark_buffer_dirty"))
            return "netdata_mark_buffer_dirty_buffer";
    } else {
        if (!strcmp(event_name, "add_to_page_cache_lru"))
            return "netdata_add_to_page_cache_lru_kprobe";
        if (!strcmp(event_name, "mark_page_accessed"))
            return "netdata_mark_page_accessed_kprobe";
        if (!strcmp(event_name, "mark_buffer_dirty"))
            return "netdata_mark_buffer_dirty_kprobe";
    }
#else
    (void)event_name;
    (void)flavor;
#endif

    return NULL;
}

static void cachestat_disable_program_if_present(struct bpf_object *obj, const char *name)
{
    struct bpf_program *prog = bpf_object__find_program_by_name(obj, name);
    if (prog)
        bpf_program__set_autoload(prog, false);
}

static void cachestat_disable_fentry_programs(struct bpf_object *obj)
{
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        const char *name = bpf_program__name(prog);
        if (strstr(name, "_fentry"))
            bpf_program__set_autoload(prog, false);
    }
}

static void cachestat_prepare_autoload(struct bpf_object *obj, const char *account_function, int flavor)
{
    cachestat_disable_fentry_programs(obj);
    cachestat_disable_program_if_present(obj, "netdata_folio_mark_dirty_kprobe");
    cachestat_disable_program_if_present(obj, "netdata_set_page_dirty_kprobe");
    cachestat_disable_program_if_present(obj, "netdata_account_page_dirtied_kprobe");
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    cachestat_disable_program_if_present(obj, "netdata_folio_mark_dirty_buffer");
    cachestat_disable_program_if_present(obj, "netdata_set_page_dirty_buffer");
    cachestat_disable_program_if_present(obj, "netdata_account_page_dirtied_buffer");
#endif

    if (!account_function)
        account_function = "account_page_dirtied";

    const char *program_names[] = {
        cachestat_data_program_name("add_to_page_cache_lru", flavor),
        cachestat_data_program_name("mark_page_accessed", flavor),
        cachestat_account_program_name(account_function, flavor),
        cachestat_data_program_name("mark_buffer_dirty", flavor),
    };

    for (size_t i = 0; i < sizeof(program_names) / sizeof(program_names[0]); i++) {
        if (!program_names[i])
            continue;

        struct bpf_program *prog = bpf_object__find_program_by_name(obj, program_names[i]);
        if (prog)
            bpf_program__set_autoload(prog, true);
    }
}

static void cachestat_update_map_types(struct bpf_object *obj, int maps_per_core)
{
    struct bpf_map *map;
    bpf_object__for_each_map(map, obj)
    {
        const char *name = bpf_map__name(map);
        if (!strcmp(name, "cstat_global") || !strcmp(name, "cstat_pid") || !strcmp(name, "cstat_ctrl")) {
            enum bpf_map_type type = bpf_map__type(map);
            if (maps_per_core) {
                if (type == BPF_MAP_TYPE_HASH)
                    bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_HASH);
                else if (type == BPF_MAP_TYPE_ARRAY)
                    bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_ARRAY);
            } else {
                if (type == BPF_MAP_TYPE_PERCPU_HASH)
                    bpf_map__set_type(map, BPF_MAP_TYPE_HASH);
                else if (type == BPF_MAP_TYPE_PERCPU_ARRAY)
                    bpf_map__set_type(map, BPF_MAP_TYPE_ARRAY);
            }
        }
    }
}

static void cachestat_update_map_sizes(struct bpf_object *obj, unsigned int pid_table_size)
{
    struct bpf_map *map = bpf_object__find_map_by_name(obj, "cstat_pid");
    if (map && pid_table_size > 0)
        bpf_map__set_max_entries(map, pid_table_size);
}

static struct bpf_link *cachestat_attach_program_by_name(struct bpf_object *obj, const char *program_name, const char *target)
{
    struct bpf_program *prog = bpf_object__find_program_by_name(obj, program_name);
    if (!prog)
        return NULL;

    return bpf_program__attach_kprobe(prog, false, target);
}

static uint64_t cachestat_sum_percpu_values(const uint64_t *values, int count)
{
    uint64_t total = 0;
    for (int i = 0; i < count; i++)
        total += values[i];
    return total;
}

static int cachestat_pid_snapshot_cmp(const void *a, const void *b)
{
    const struct netdata_ebpf_cachestat_pid_snapshot *pa = a;
    const struct netdata_ebpf_cachestat_pid_snapshot *pb = b;
    if (pa->pid < pb->pid) return -1;
    if (pa->pid > pb->pid) return 1;
    return 0;
}

struct netdata_ebpf_cachestat_pid_entry {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[16];
    uint32_t add_to_page_cache_lru;
    uint32_t mark_page_accessed;
    uint32_t account_page_dirtied;
    uint32_t mark_buffer_dirty;
};

static void cachestat_destroy_links(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt || !rt->links)
        return;

    for (size_t i = 0; i < 4; i++) {
        if (rt->links[i])
            bpf_link__destroy(rt->links[i]);
    }

    free(rt->links);
    rt->links = NULL;
}

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
static int cachestat_runtime_flavor_from_path(const char *path)
{
    if (!path)
        return NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE;

    if (strstr(path, "_arena."))
        return NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA;

    if (strstr(path, "_buffer."))
        return NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER;

    return NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE;
}
#endif

static struct bpf_object *cachestat_runtime_object(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt)
        return NULL;

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE) {
        switch (rt->flavor) {
        case NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER:
            return rt->core.buffer ? rt->core.buffer->obj : NULL;
        case NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA:
            return rt->core.arena ? rt->core.arena->obj : NULL;
        default:
            return rt->core.base ? rt->core.base->obj : NULL;
        }
    }
#endif

    return rt->obj;
}

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
static int cachestat_runtime_load_core(struct netdata_ebpf_cachestat_runtime *rt)
{
    switch (rt->flavor) {
    case NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER:
        return cachestat_buffer_bpf__load(rt->core.buffer);
    case NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA:
        return cachestat_arena_bpf__load(rt->core.arena);
    default:
        return cachestat_bpf__load(rt->core.base);
    }
}

static void cachestat_runtime_destroy_core(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt)
        return;

    switch (rt->flavor) {
    case NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER:
        if (rt->core.buffer)
            cachestat_buffer_bpf__destroy(rt->core.buffer);
        break;
    case NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA:
        if (rt->core.arena)
            cachestat_arena_bpf__destroy(rt->core.arena);
        break;
    default:
        if (rt->core.base)
            cachestat_bpf__destroy(rt->core.base);
        break;
    }
}
#endif

int netdata_cachestat_runtime_supports_core(void)
{
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    return 1;
#else
    return 0;
#endif
}

struct netdata_ebpf_cachestat_runtime *netdata_cachestat_runtime_open_mode(const char *path, int use_core)
{
    struct netdata_ebpf_cachestat_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt) {
        return NULL;
    }

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (use_core) {
        rt->kind = NETDATA_CACHESTAT_RUNTIME_CORE;
        rt->flavor = cachestat_runtime_flavor_from_path(path);
        switch (rt->flavor) {
        case NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER:
            rt->core.buffer = cachestat_buffer_bpf__open();
            rt->obj = rt->core.buffer ? rt->core.buffer->obj : NULL;
            break;
        case NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA:
            rt->core.arena = cachestat_arena_bpf__open();
            rt->obj = rt->core.arena ? rt->core.arena->obj : NULL;
            break;
        default:
            rt->core.base = cachestat_bpf__open();
            rt->obj = rt->core.base ? rt->core.base->obj : NULL;
            break;
        }

        if (!rt->obj || libbpf_get_error(rt->obj)) {
            cachestat_runtime_destroy_core(rt);
            free(rt);
            return NULL;
        }
    } else
#else
    (void)use_core;
#endif
    {
        struct bpf_object *obj = bpf_object__open_file(path, NULL);
        if (!obj || libbpf_get_error(obj)) {
            if (obj && libbpf_get_error(obj))
                bpf_object__close(obj);
            free(rt);
            return NULL;
        }

        rt->obj = obj;
    }

    return rt;
}

int netdata_cachestat_runtime_prepare(
    struct netdata_ebpf_cachestat_runtime *rt,
    unsigned int pid_table_size,
    int maps_per_core,
    const char *account_function)
{
    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!rt || !obj)
        return -1;

    cachestat_prepare_autoload(obj, account_function, rt->flavor);
    cachestat_update_map_types(obj, maps_per_core);
    cachestat_update_map_sizes(obj, pid_table_size);
    return 0;
}

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED

/*
 * Ring buffer event layout — must match struct netdata_cachestat_event_t
 * in ebpf-co-re/kernel-collector/includes/netdata_cache_buffer.h.
 */
struct cachestat_rb_event {
    uint64_t ct;
    uint32_t pid;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char     name[16]; /* TASK_COMM_LEN */
    uint8_t  action;
    uint8_t  pad[3];
};

enum {
    CACHESTAT_RB_EVENT_PAGE_CACHE_LRU = 0,
    CACHESTAT_RB_EVENT_PAGE_ACCESSED  = 1,
    CACHESTAT_RB_EVENT_PAGE_DIRTIED   = 2,
    CACHESTAT_RB_EVENT_BUFFER_DIRTY   = 3,
};

static struct netdata_ebpf_cachestat_pid_entry *cachestat_acc_find_or_add(
    struct netdata_ebpf_cachestat_runtime *rt, uint32_t tgid)
{
    for (size_t i = 0; i < rt->acc_count; i++) {
        if (rt->acc[i].tgid == tgid)
            return &rt->acc[i];
    }

    if (rt->acc_count >= rt->acc_cap) {
        size_t new_cap = rt->acc_cap ? rt->acc_cap * 2 : 64;
        struct netdata_ebpf_cachestat_pid_entry *p = realloc(rt->acc, new_cap * sizeof(*p));
        if (!p)
            return NULL;
        rt->acc = p;
        rt->acc_cap = new_cap;
    }

    struct netdata_ebpf_cachestat_pid_entry *entry = &rt->acc[rt->acc_count++];
    memset(entry, 0, sizeof(*entry));
    entry->tgid = tgid;
    return entry;
}

static int cachestat_rb_callback(void *ctx, void *data, size_t data_sz)
{
    if (data_sz < sizeof(struct cachestat_rb_event))
        return 0;

    struct netdata_ebpf_cachestat_runtime *rt = ctx;
    const struct cachestat_rb_event *ev = data;

    uint32_t tgid = ev->tgid ? ev->tgid : ev->pid;
    struct netdata_ebpf_cachestat_pid_entry *entry = cachestat_acc_find_or_add(rt, tgid);
    if (!entry)
        return 0;

    if (ev->ct > entry->ct)
        entry->ct = ev->ct;
    if (!entry->name[0] && ev->name[0]) {
        size_t nlen = strnlen(ev->name, sizeof(ev->name));
        memcpy(entry->name, ev->name, nlen < sizeof(entry->name) - 1 ? nlen : sizeof(entry->name) - 1);
    }

    switch (ev->action) {
    case CACHESTAT_RB_EVENT_PAGE_CACHE_LRU: entry->add_to_page_cache_lru++; break;
    case CACHESTAT_RB_EVENT_PAGE_ACCESSED:  entry->mark_page_accessed++;    break;
    case CACHESTAT_RB_EVENT_PAGE_DIRTIED:   entry->account_page_dirtied++;  break;
    case CACHESTAT_RB_EVENT_BUFFER_DIRTY:   entry->mark_buffer_dirty++;     break;
    default:                                                                  break;
    }

    return 0;
}

static void cachestat_setup_ring_buffer(struct netdata_ebpf_cachestat_runtime *rt)
{
    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!obj)
        return;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "cachestat_events");
    if (!map) {
        fprintf(stderr, "ebpf-go: cachestat ring buffer map 'cachestat_events' not found\n");
        return;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0) {
        fprintf(stderr, "ebpf-go: cachestat ring buffer map fd invalid (%d)\n", fd);
        return;
    }

    rt->rb = ring_buffer__new(fd, cachestat_rb_callback, rt, NULL);
    if (!rt->rb)
        fprintf(stderr, "ebpf-go: ring_buffer__new failed for cachestat (errno %d)\n", errno);
}

static void cachestat_destroy_ring_buffer(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (rt->rb) {
        ring_buffer__free(rt->rb);
        rt->rb = NULL;
    }
    free(rt->acc);
    rt->acc     = NULL;
    rt->acc_cap   = 0;
    rt->acc_count = 0;
}

/*
 * Arena flavor: the BPF programs write events into a shared-memory circular
 * slot buffer (BPF arena, mmap-able) instead of a ring buffer.
 *
 * Layout of struct netdata_cachestat_arena_state_t (must match the macro
 * expansion in netdata_arena_common.h / netdata_cachestat_arena.h):
 *   __u32 head          (4 bytes)
 *   [4 bytes padding to align events[] to 8 bytes]
 *   cachestat_rb_event events[1024]  (1024 × 48 = 49152 bytes)
 *   total = 49160 bytes  (verified by skeleton _Static_assert)
 */
#define CACHESTAT_ARENA_EVENT_SLOTS 1024

struct cachestat_arena_state {
    uint32_t head;
    uint32_t _pad;
    struct cachestat_rb_event events[CACHESTAT_ARENA_EVENT_SLOTS];
};

_Static_assert(
    sizeof(struct cachestat_arena_state) == 49160,
    "cachestat_arena_state size does not match BPF-side layout");

/* Capture the mmap'd arena state pointer once at load time.
 *
 * bpf_map__initial_value() on a BPF_MAP_TYPE_ARENA map returns the arena
 * region start (mmap offset 0).  The actual data section is placed at
 * (arena_sz - roundup(arena_data_sz, page_sz)) bytes into the region — for a
 * 1 MB arena with a 49 160-byte data section that is offset 995 328.  Using
 * the arena map pointer directly therefore reads uninitialized memory.
 *
 * The arena skeleton wires a companion BSS map ("cachesta.bss") whose mmaped
 * pointer libbpf resolves to the correct offset after load.  Use that. */
static void cachestat_setup_arena(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt->core.arena || !rt->core.arena->bss)
        return;

    rt->arena_state = (void *)&rt->core.arena->bss->cachestat_arena_state;
}

static int cachestat_snapshot_from_acc(
    struct netdata_ebpf_cachestat_runtime *rt,
    struct netdata_ebpf_cachestat_pid_snapshot_list *out)
{
    out->items = NULL;
    out->count = 0;

    if (!rt->acc || rt->acc_count == 0)
        return 0;

    struct netdata_ebpf_cachestat_pid_snapshot *items = calloc(rt->acc_count, sizeof(*items));
    if (!items)
        return -1;

    for (size_t i = 0; i < rt->acc_count; i++) {
        const struct netdata_ebpf_cachestat_pid_entry *src = &rt->acc[i];
        struct netdata_ebpf_cachestat_pid_snapshot *dst = &items[i];
        dst->pid = src->tgid;
        dst->ct  = src->ct;
        size_t nlen = strnlen(src->name, sizeof(src->name));
        memcpy(dst->comm, src->name, nlen < sizeof(dst->comm) - 1 ? nlen : sizeof(dst->comm) - 1);
        dst->add_to_page_cache_lru = src->add_to_page_cache_lru;
        dst->mark_page_accessed    = src->mark_page_accessed;
        dst->account_page_dirtied  = src->account_page_dirtied;
        dst->mark_buffer_dirty     = src->mark_buffer_dirty;
    }

    qsort(items, rt->acc_count, sizeof(*items), cachestat_pid_snapshot_cmp);

    out->items = items;
    out->count = rt->acc_count;
    return 0;
}

static void cachestat_drain_arena(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt->arena_state)
        return;

    struct cachestat_arena_state *state =
        (struct cachestat_arena_state *)rt->arena_state;

    uint32_t head = __atomic_load_n(&state->head, __ATOMIC_ACQUIRE);
    uint32_t tail = rt->arena_tail;

    while (tail != head) {
        const struct cachestat_rb_event *ev =
            &state->events[tail % CACHESTAT_ARENA_EVENT_SLOTS];
        cachestat_rb_callback(rt, (void *)ev, sizeof(*ev));
        tail++;
    }

    rt->arena_tail = head;
}

#endif /* NETDATA_LIBBPF_CORE_SUPPORTED */

int netdata_cachestat_runtime_load(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE) {
        int rc = cachestat_runtime_load_core(rt);
        if (rc != 0)
            return rc;
        if (rt->flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_BUFFER)
            cachestat_setup_ring_buffer(rt);
        else if (rt->flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA)
            cachestat_setup_arena(rt);
        return 0;
    }
#endif

    return bpf_object__load(rt->obj);
}

int netdata_cachestat_runtime_attach(struct netdata_ebpf_cachestat_runtime *rt, const char *account_function)
{
    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!rt || !obj)
        return -1;

    if (rt->links)
        cachestat_destroy_links(rt);

    if (!account_function)
        account_function = "account_page_dirtied";

    rt->links = calloc(4, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    const char *account_prog = cachestat_account_program_name(account_function, rt->flavor);
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    const char *add_prog = (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE && rt->flavor != NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE) ?
                               "netdata_add_to_page_cache_lru_buffer" :
                               "netdata_add_to_page_cache_lru_kprobe";
    const char *access_prog = (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE && rt->flavor != NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE) ?
                                  "netdata_mark_page_accessed_buffer" :
                                  "netdata_mark_page_accessed_kprobe";
    const char *dirty_prog = (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE && rt->flavor != NETDATA_CACHESTAT_RUNTIME_FLAVOR_BASE) ?
                                 "netdata_mark_buffer_dirty_buffer" :
                                 "netdata_mark_buffer_dirty_kprobe";
#else
    const char *add_prog = "netdata_add_to_page_cache_lru_kprobe";
    const char *access_prog = "netdata_mark_page_accessed_kprobe";
    const char *dirty_prog = "netdata_mark_buffer_dirty_kprobe";
#endif

    rt->links[0] = cachestat_attach_program_by_name(obj, add_prog, "add_to_page_cache_lru");
    rt->links[1] = cachestat_attach_program_by_name(obj, access_prog, "mark_page_accessed");
    rt->links[2] = cachestat_attach_program_by_name(obj, account_prog, account_function);
    rt->links[3] = cachestat_attach_program_by_name(obj, dirty_prog, "mark_buffer_dirty");

    for (size_t i = 0; i < 4; i++) {
        if (!rt->links[i] || libbpf_get_error(rt->links[i])) {
            cachestat_destroy_links(rt);
            return -1;
        }
    }

    return 0;
}

int netdata_cachestat_runtime_update_controller(
    struct netdata_ebpf_cachestat_runtime *rt,
    int apps_enabled,
    int apps_level)
{
    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!rt || !obj)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "cstat_ctrl");
    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    /* cstat_ctrl BPF map stores __u64 values; use uint64_t to match exactly */
    const uint64_t values[NETDATA_CONTROLLER_END] = {
        apps_enabled ? 1ULL : 0ULL,
        (uint64_t)apps_level,
        0,
        0,
        0,
        0,
    };
    const enum bpf_map_type type = bpf_map__type(map);
    const bool is_percpu = (type == BPF_MAP_TYPE_PERCPU_ARRAY || type == BPF_MAP_TYPE_PERCPU_HASH);

    for (uint32_t key = NETDATA_CONTROLLER_APPS_ENABLED; key < NETDATA_CONTROLLER_PID_TABLE_ADD; key++) {
        if (!is_percpu) {
            if (bpf_map_update_elem(fd, &key, &values[key], BPF_ANY))
                return -1;
            continue;
        }

        const int cpus = libbpf_num_possible_cpus();
        if (cpus <= 0)
            return -1;

        uint64_t *percpu = calloc((size_t)cpus, sizeof(*percpu));
        if (!percpu)
            return -1;

        for (int cpu = 0; cpu < cpus; cpu++)
            percpu[cpu] = values[key];

        const int rc = bpf_map_update_elem(fd, &key, percpu, BPF_ANY);
        free(percpu);

        if (rc)
            return -1;
    }

    return 0;
}

int netdata_cachestat_runtime_snapshot(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_snapshot *out)
{
    if (!rt || !rt->obj || !out)
        return -1;

    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!obj)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "cstat_global");
    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    int count = maps_per_core ? libbpf_num_possible_cpus() : 1;
    if (count < 1)
        count = 1;

    uint64_t *values = calloc((size_t)count, sizeof(*values));
    if (!values)
        return -1;

    struct {
        uint64_t *dst;
        uint32_t key;
    } entries[] = {
        {&out->add_to_page_cache_lru, 0},
        {&out->mark_page_accessed, 1},
        {&out->account_page_dirtied, 2},
        {&out->mark_buffer_dirty, 3},
    };

    for (size_t i = 0; i < sizeof(entries) / sizeof(entries[0]); i++) {
        uint32_t key = entries[i].key;
        *entries[i].dst = 0;

        if (bpf_map_lookup_elem(fd, &key, values) == 0)
            *entries[i].dst = cachestat_sum_percpu_values(values, count);
    }

    free(values);
    return 0;
}

int netdata_cachestat_runtime_snapshot_apps(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_pid_snapshot_list *out)
{
    if (!rt || !out)
        return -1;

    struct bpf_object *obj = cachestat_runtime_object(rt);
    if (!obj)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "cstat_pid");

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (!map) {
        /* Buffer and arena flavors have no cstat_pid map; drain events into
         * the userspace accumulator and convert to snapshot format. */
        if (rt->rb) {
            ring_buffer__consume(rt->rb);
            return cachestat_snapshot_from_acc(rt, out);
        }
        if (rt->flavor == NETDATA_CACHESTAT_RUNTIME_FLAVOR_ARENA) {
            cachestat_drain_arena(rt);
            return cachestat_snapshot_from_acc(rt, out);
        }
    }
#endif

    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    int count = maps_per_core ? libbpf_num_possible_cpus() : 1;
    if (count < 1)
        count = 1;

    size_t max_entries = bpf_map__max_entries(map);
    if (max_entries == 0)
        max_entries = 1;

    struct netdata_ebpf_cachestat_pid_entry *values = calloc((size_t)count, sizeof(*values));
    if (!values)
        return -1;

    struct netdata_ebpf_cachestat_pid_snapshot *items = calloc(max_entries, sizeof(*items));
    if (!items) {
        free(values);
        return -1;
    }

    size_t out_count = 0;
    uint32_t key = 0, next_key = 0;

    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &next_key, values)) {
            key = next_key;
            memset(values, 0, (size_t)count * sizeof(*values));
            continue;
        }

        if (out_count >= max_entries)
            goto next_key_iter;

        /*
         * With NETDATA_APPS_LEVEL_ALL the BPF key is the thread ID (TID).
         * The process TGID is stored in values[i].tgid by the BPF program.
         * We must index shared memory by TGID so that cgroup.procs lookups
         * (which use TGIDs) succeed.  Fall back to next_key only when every
         * per-CPU entry has tgid==0 (race at entry creation; counters are
         * also zero in that case so the entry is harmless).
         */
        uint32_t tgid = 0;
        for (int i = 0; i < count; i++) {
            if (values[i].tgid != 0) {
                tgid = values[i].tgid;
                break;
            }
        }
        if (tgid == 0)
            tgid = next_key;

        struct netdata_ebpf_cachestat_pid_snapshot *dst = &items[out_count];
        memset(dst, 0, sizeof(*dst));
        dst->pid = tgid;

        for (int i = 0; i < count; i++) {
            if (values[i].ct > dst->ct)
                dst->ct = values[i].ct;
            dst->add_to_page_cache_lru += values[i].add_to_page_cache_lru;
            dst->mark_page_accessed += values[i].mark_page_accessed;
            dst->account_page_dirtied += values[i].account_page_dirtied;
            dst->mark_buffer_dirty += values[i].mark_buffer_dirty;
            if (!dst->comm[0] && values[i].name[0]) {
                size_t comm_len = strnlen(values[i].name, sizeof(values[i].name));
                memcpy(dst->comm, values[i].name, comm_len);
                dst->comm[comm_len] = '\0';
            }
        }
        out_count++;

next_key_iter:
        key = next_key;
        memset(values, 0, (size_t)count * sizeof(*values));
    }

    free(values);

    /*
     * Multiple threads of the same process produce separate BPF entries with
     * the same TGID.  Sort by pid (TGID) and merge consecutive same-pid
     * entries so each shared-memory slot represents one process.
     */
    if (out_count > 1) {
        qsort(items, out_count, sizeof(*items), cachestat_pid_snapshot_cmp);

        size_t merged_count = 0;
        for (size_t i = 0; i < out_count; i++) {
            if (merged_count == 0 || items[merged_count - 1].pid != items[i].pid) {
                if (merged_count != i)
                    items[merged_count] = items[i];
                merged_count++;
            } else {
                struct netdata_ebpf_cachestat_pid_snapshot *m = &items[merged_count - 1];
                if (items[i].ct > m->ct)
                    m->ct = items[i].ct;
                m->add_to_page_cache_lru += items[i].add_to_page_cache_lru;
                m->mark_page_accessed += items[i].mark_page_accessed;
                m->account_page_dirtied += items[i].account_page_dirtied;
                m->mark_buffer_dirty += items[i].mark_buffer_dirty;
                if (!m->comm[0] && items[i].comm[0])
                    memcpy(m->comm, items[i].comm, sizeof(m->comm));
            }
        }
        out_count = merged_count;
    }

    out->items = items;
    out->count = out_count;
    return 0;
}

void netdata_cachestat_runtime_free_apps_snapshot(struct netdata_ebpf_cachestat_pid_snapshot_list *out)
{
    if (!out)
        return;

    free(out->items);
    out->items = NULL;
    out->count = 0;
}

void netdata_cachestat_runtime_close(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt)
        return;

    cachestat_destroy_links(rt);
#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
    if (rt->kind == NETDATA_CACHESTAT_RUNTIME_CORE) {
        cachestat_destroy_ring_buffer(rt);
        cachestat_runtime_destroy_core(rt);
    } else if (rt->obj)
        bpf_object__close(rt->obj);
#else
    if (rt->obj)
        bpf_object__close(rt->obj);
#endif
    free(rt);
}
