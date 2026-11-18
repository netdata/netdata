//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

#include "../nd_alloc_shim.h"
#include "nd_ebpf_runtime_common.h"

#if defined(LIBBPF_MAJOR_VERSION) && (LIBBPF_MAJOR_VERSION >= 1) && defined(__has_include) && __has_include(<linux/btf.h>)
/* The CO-RE bundle ships the skeleton wrapper but not the arena BPF-side state
 * type.  The generated skeleton asserts the same size as nd_ebpf_arena_state. */
#ifndef NETDATA_PROCESS_ARENA_STATE_T_DEFINED
struct netdata_process_arena_state_t {
    unsigned char __pad[sizeof(struct nd_ebpf_arena_state)];
};
#endif

#if defined(NETDATA_PROCESS_HAS_SKELETON) || __has_include("process.skel.h")
#include "process.skel.h"
#include "process_buffer.skel.h"
#include "process_arena.skel.h"
#define NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED 1
#endif
#endif /* libbpf >= 1 */

enum {
    PROCESS_KIND_LEGACY = 0,
    PROCESS_KIND_CORE = 1,
};

enum {
    PROCESS_FLAVOR_BASE = 0,
    PROCESS_FLAVOR_BUFFER = 1,
    PROCESS_FLAVOR_ARENA = 2,
};

/* enum process_counters in netdata/kernel-collector includes/netdata_process.h:
 * the keys of the tbl_total_stats PERCPU_ARRAY. */
enum {
    PROCESS_KEY_CALLS_DO_EXIT = 0,
    PROCESS_KEY_CALLS_RELEASE_TASK = 1,
    PROCESS_KEY_CALLS_PROCESS = 2,
    PROCESS_KEY_ERROR_PROCESS = 3,
    PROCESS_KEY_CALLS_THREAD = 4,
    PROCESS_KEY_ERROR_THREAD = 5,
};

/* enum netdata_process_event_action in netdata/kernel-collector
 * includes/netdata_process_buffer.h. */
enum {
    PROCESS_EVENT_EXIT = 0,
    PROCESS_EVENT_RELEASE = 1,
    PROCESS_EVENT_EXEC = 2,
    PROCESS_EVENT_FORK = 3,
    PROCESS_EVENT_THREAD = 4,
    PROCESS_EVENT_FORK_ERR = 5,
};

/* struct netdata_pid_stat_t in netdata/kernel-collector includes/netdata_process.h,
 * the value type of tbl_pid_stats. */
struct netdata_ebpf_process_pid_entry {
    uint64_t ct;
    uint32_t uid;
    uint32_t gid;
    char name[16];
    uint32_t tgid;
    uint32_t pid;
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;
    uint32_t task_err;
};

struct netdata_ebpf_process_snapshot {
    uint64_t exits;
    uint64_t task_close;
    uint64_t process;
    uint64_t process_err;
    uint64_t thread;
    uint64_t thread_err;
};

struct netdata_ebpf_process_pid_snapshot {
    uint32_t pid;
    uint32_t ppid;
    uint64_t ct;
    char comm[96];
    uint32_t exits;
    uint32_t task_close;
    uint32_t forks;
    uint32_t clones;
    uint32_t errors;
};

struct netdata_ebpf_process_pid_snapshot_list {
    struct netdata_ebpf_process_pid_snapshot *items;
    size_t count;
};

struct netdata_ebpf_process_runtime {
    int kind;
    int flavor;
    struct bpf_object *obj;
    struct bpf_link **links;
    size_t link_count;
    uint64_t *percpu_u64;
    int percpu_u64_cap;
    struct netdata_ebpf_process_pid_entry *percpu_entries;
    int percpu_entries_cap;
    struct netdata_ebpf_process_pid_snapshot *items_buf;
    size_t items_cap;
    struct nd_ebpf_key_table pid_keys;
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    union {
        struct process_bpf *base;
        struct process_buffer_bpf *buffer;
        struct process_arena_bpf *arena;
    } core;
    struct ring_buffer *rb;
    void *arena_state;
    struct nd_ebpf_acc_table acc;
#endif
};

static struct bpf_object *process_object(struct netdata_ebpf_process_runtime *rt)
{
    if (!rt)
        return NULL;

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    if (rt->kind == PROCESS_KIND_CORE) {
        switch (rt->flavor) {
        case PROCESS_FLAVOR_BUFFER:
            return rt->core.buffer ? rt->core.buffer->obj : NULL;
        case PROCESS_FLAVOR_ARENA:
            return rt->core.arena ? rt->core.arena->obj : NULL;
        default:
            return rt->core.base ? rt->core.base->obj : NULL;
        }
    }
#endif

    return rt->obj;
}

static int process_flavor_from_path(const char *path)
{
    if (path && strstr(path, "_arena."))
        return PROCESS_FLAVOR_ARENA;
    if (path && strstr(path, "_buffer."))
        return PROCESS_FLAVOR_BUFFER;
    return PROCESS_FLAVOR_BASE;
}

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
static void process_destroy_core(struct netdata_ebpf_process_runtime *rt)
{
    switch (rt->flavor) {
    case PROCESS_FLAVOR_BUFFER:
        if (rt->core.buffer)
            process_buffer_bpf__destroy(rt->core.buffer);
        break;
    case PROCESS_FLAVOR_ARENA:
        if (rt->core.arena)
            process_arena_bpf__destroy(rt->core.arena);
        break;
    default:
        if (rt->core.base)
            process_bpf__destroy(rt->core.base);
        break;
    }
}
#endif

int netdata_process_runtime_supports_core(void)
{
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    return 1;
#else
    return 0;
#endif
}

struct netdata_ebpf_process_runtime *
netdata_process_runtime_open_mode(const char *path, int use_core, const char *custom_btf_path)
{
    struct netdata_ebpf_process_runtime *rt = callocz(1, sizeof(*rt));
    rt->flavor = process_flavor_from_path(path);

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    nd_ebpf_acc_init(
        &rt->acc, sizeof(struct netdata_ebpf_process_pid_entry), offsetof(struct netdata_ebpf_process_pid_entry, tgid));

    if (use_core) {
        rt->kind = PROCESS_KIND_CORE;
        struct bpf_object_open_opts opts = {
            .sz = sizeof(opts),
            .btf_custom_path = custom_btf_path && *custom_btf_path ? custom_btf_path : NULL,
        };
        switch (rt->flavor) {
        case PROCESS_FLAVOR_BUFFER:
            rt->core.buffer = process_buffer_bpf__open_opts(&opts);
            rt->obj = rt->core.buffer ? rt->core.buffer->obj : NULL;
            break;
        case PROCESS_FLAVOR_ARENA:
            rt->core.arena = process_arena_bpf__open_opts(&opts);
            rt->obj = rt->core.arena ? rt->core.arena->obj : NULL;
            break;
        default:
            rt->core.base = process_bpf__open_opts(&opts);
            rt->obj = rt->core.base ? rt->core.base->obj : NULL;
            break;
        }

        if (!rt->obj || libbpf_get_error(rt->obj)) {
            process_destroy_core(rt);
            nd_ebpf_acc_free(&rt->acc);
            freez(rt);
            return NULL;
        }
        return rt;
    }
#else
    (void)use_core;
    (void)custom_btf_path;
#endif

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
        nd_ebpf_acc_free(&rt->acc);
#endif
        freez(rt);
        return NULL;
    }
    rt->obj = obj;
    return rt;
}

/* Programs of the CO-RE base object that duplicate an accounting path we keep
 * (trampolines and the version-specific fork/clone return probes) or only exist
 * in older asset bundles.  Upstream: netdata/ebpf-co-re src/process.bpf.c. */
static const char *const process_base_disabled[] = {
    "netdata_release_task_fentry",
    "netdata_clone_fexit",
    "netdata_clone3_fexit",
    "netdata_do_fork_probe",
    "netdata_kernel_clone_probe",
    "netdata_sched_process_fork_btf",
};

/* Fork accounting programs, preferred first.  Asset releases rename or omit
 * them, so the first one present wins and the others are disabled. */
static const char *const process_fork_programs[] = {
    "netdata_wake_up_new_task_probe",
    "netdata_wake_up_new_task",
    "netdata_tracepoint_sched_process_fork",
    "netdata_tracepoint_sched_process_fork_v2",
};

/* Syscall-exit tracepoints that may not exist on every kernel/arch (clone3 needs
 * 5.3, arm64 has no fork/vfork).  Their attach failure is not fatal. */
static const char *const process_optional_programs[] = {
    "netdata_clone3_exit",
    "netdata_fork_exit",
    "netdata_vfork_exit",
};

static bool process_name_in(const char *name, const char *const *list, size_t count)
{
    for (size_t i = 0; i < count; i++) {
        if (!strcmp(name, list[i]))
            return true;
    }
    return false;
}

/* Only the CO-RE base object bundles alternative implementations of the same
 * accounting; legacy and buffer/arena objects are built per kernel and every
 * program they carry is attached, as the legacy C collector did. */
static void process_prepare_autoload(struct netdata_ebpf_process_runtime *rt, struct bpf_object *obj)
{
    if (rt->kind != PROCESS_KIND_CORE || rt->flavor != PROCESS_FLAVOR_BASE)
        return;

    const char *fork_program = NULL;
    for (size_t i = 0; i < sizeof(process_fork_programs) / sizeof(process_fork_programs[0]); i++) {
        if (bpf_object__find_program_by_name(obj, process_fork_programs[i])) {
            fork_program = process_fork_programs[i];
            break;
        }
    }

    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        const char *name = bpf_program__name(prog);
        bool disabled =
            process_name_in(name, process_base_disabled, sizeof(process_base_disabled) / sizeof(process_base_disabled[0])) ||
            (process_name_in(name, process_fork_programs, sizeof(process_fork_programs) / sizeof(process_fork_programs[0])) &&
             (!fork_program || strcmp(name, fork_program) != 0));
        bpf_program__set_autoload(prog, !disabled);
    }
}

int netdata_process_runtime_prepare(struct netdata_ebpf_process_runtime *rt, unsigned int pid_table_size, int maps_per_core)
{
    struct bpf_object *obj = process_object(rt);
    if (!rt || !obj)
        return -1;

    process_prepare_autoload(rt, obj);

    static const char *const maps[] = {"tbl_total_stats", "process_ctrl", "tbl_pid_stats"};
    if (nd_ebpf_update_map_types(obj, maps, sizeof(maps) / sizeof(maps[0]), maps_per_core, "process") != 0)
        return -1;

    struct bpf_map *pid = bpf_object__find_map_by_name(obj, "tbl_pid_stats");
    if (pid && pid_table_size)
        bpf_map__set_max_entries(pid, pid_table_size);

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    nd_ebpf_acc_set_max_entries(&rt->acc, pid_table_size);
#endif

    return nd_ebpf_alloc_percpu_buffers(
        &rt->percpu_u64, &rt->percpu_u64_cap,
        (void **)&rt->percpu_entries, &rt->percpu_entries_cap, sizeof(*rt->percpu_entries));
}

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
static int process_event_callback(void *ctx, void *data, size_t size)
{
    if (size < sizeof(struct nd_ebpf_pid_event))
        return 0;

    struct netdata_ebpf_process_runtime *rt = ctx;
    const struct nd_ebpf_pid_event *ev = data;

    struct netdata_ebpf_process_pid_entry *entry = nd_ebpf_acc_find_or_add(&rt->acc, ev->tgid ? ev->tgid : ev->pid);
    if (!entry)
        return 0;

    if (ev->ct > entry->ct)
        entry->ct = ev->ct;
    if (!entry->name[0] && ev->name[0])
        nd_ebpf_copy_comm(entry->name, sizeof(entry->name), ev->name, sizeof(ev->name));

    /* Mirrors the per-PID map updates of the base object: a new thread counts as
     * both a created task and a created thread. */
    switch (ev->action) {
    case PROCESS_EVENT_EXIT:
        entry->exit_call++;
        break;
    case PROCESS_EVENT_RELEASE:
        entry->release_call++;
        break;
    case PROCESS_EVENT_EXEC:
    case PROCESS_EVENT_FORK:
        entry->create_process++;
        break;
    case PROCESS_EVENT_THREAD:
        entry->create_process++;
        entry->create_thread++;
        break;
    case PROCESS_EVENT_FORK_ERR:
        entry->task_err++;
        break;
    default:
        break;
    }

    return 0;
}

static void process_arena_event(void *ctx, const struct nd_ebpf_pid_event *ev)
{
    process_event_callback(ctx, (void *)ev, sizeof(*ev));
}

static int process_setup_events(struct netdata_ebpf_process_runtime *rt)
{
    if (rt->flavor == PROCESS_FLAVOR_BUFFER) {
        rt->rb = nd_ebpf_ring_buffer_open(process_object(rt), "process_events", process_event_callback, rt, "process");
        return rt->rb ? 0 : -1;
    }

    if (rt->flavor == PROCESS_FLAVOR_ARENA) {
        if (!rt->core.arena || !rt->core.arena->arena)
            return -1;
        rt->arena_state = (void *)&rt->core.arena->arena->process_arena_state;
        return nd_ebpf_arena_state_valid(
            rt->core.arena->maps.process_events, rt->arena_state, sizeof(struct nd_ebpf_arena_state));
    }

    return 0;
}

static void process_drain_events(struct netdata_ebpf_process_runtime *rt)
{
    if (rt->rb)
        ring_buffer__consume(rt->rb);
    else if (rt->arena_state)
        nd_ebpf_arena_drain(rt->arena_state, process_arena_event, rt);
}
#endif

int netdata_process_runtime_load(struct netdata_ebpf_process_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    if (rt->kind == PROCESS_KIND_CORE) {
        int rc;
        switch (rt->flavor) {
        case PROCESS_FLAVOR_BUFFER:
            rc = process_buffer_bpf__load(rt->core.buffer);
            break;
        case PROCESS_FLAVOR_ARENA:
            rc = process_arena_bpf__load(rt->core.arena);
            break;
        default:
            rc = process_bpf__load(rt->core.base);
            break;
        }
        if (rc)
            return -1;
        return process_setup_events(rt);
    }

    // Raw arena objects need the generated skeleton's mmap'd data section.
    if (rt->flavor == PROCESS_FLAVOR_ARENA)
        return -1;
#endif

    if (bpf_object__load(rt->obj))
        return -1;

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    return process_setup_events(rt);
#else
    return rt->flavor == PROCESS_FLAVOR_BASE ? 0 : -1;
#endif
}

/* Every loaded program attaches through its SEC() name, which carries the
 * probe type and the kernel target of the object it was built for. */
int netdata_process_runtime_attach(struct netdata_ebpf_process_runtime *rt)
{
    struct bpf_object *obj = process_object(rt);
    if (!rt || !obj)
        return -1;

    nd_ebpf_destroy_links(&rt->links, rt->link_count);
    rt->link_count = 0;

    size_t total = 0;
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj) total++;

    rt->links = callocz(total ? total : 1, sizeof(*rt->links));

    bpf_object__for_each_program(prog, obj)
    {
        if (!bpf_program__autoload(prog))
            continue;

        struct bpf_link *link = bpf_program__attach(prog);
        if (!link || libbpf_get_error(link)) {
            if (process_name_in(
                    bpf_program__name(prog), process_optional_programs,
                    sizeof(process_optional_programs) / sizeof(process_optional_programs[0])))
                continue;

            nd_ebpf_destroy_links(&rt->links, rt->link_count);
            rt->link_count = 0;
            return -1;
        }
        rt->links[rt->link_count++] = link;
    }

    return rt->link_count ? 0 : -1;
}

int netdata_process_runtime_update_controller(struct netdata_ebpf_process_runtime *rt, int apps_enabled, int apps_level)
{
    if (!rt)
        return -1;

    return nd_ebpf_update_controller(process_object(rt), "process_ctrl", apps_enabled, apps_level);
}

/* All flavors keep the global counters in tbl_total_stats, so the system charts
 * never depend on the bounded per-TGID userspace accumulator. */
int netdata_process_runtime_snapshot(
    struct netdata_ebpf_process_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_process_snapshot *out)
{
    (void)maps_per_core;

    struct bpf_object *obj = process_object(rt);
    if (!obj || !out || !rt->percpu_u64)
        return -1;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "tbl_total_stats");
    if (!map)
        return -1;

    int fd = bpf_map__fd(map);
    if (fd < 0)
        return -1;

    const struct nd_ebpf_snap_global_entry entries[] = {
        {&out->exits, PROCESS_KEY_CALLS_DO_EXIT},
        {&out->task_close, PROCESS_KEY_CALLS_RELEASE_TASK},
        {&out->process, PROCESS_KEY_CALLS_PROCESS},
        {&out->process_err, PROCESS_KEY_ERROR_PROCESS},
        {&out->thread, PROCESS_KEY_CALLS_THREAD},
        {&out->thread_err, PROCESS_KEY_ERROR_THREAD},
    };

    nd_ebpf_global_snap_aggregate(map, fd, rt->percpu_u64, rt->percpu_u64_cap, entries, sizeof(entries) / sizeof(entries[0]));
    return 0;
}

static void process_fill_snapshot_counters(
    struct netdata_ebpf_process_pid_snapshot *dst,
    const struct netdata_ebpf_process_pid_entry *src)
{
    if (src->ct > dst->ct)
        dst->ct = src->ct;
    dst->exits += src->exit_call;
    dst->task_close += src->release_call;
    dst->forks += src->create_process;
    dst->clones += src->create_thread;
    dst->errors += src->task_err;
    if (!dst->comm[0] && src->name[0])
        nd_ebpf_copy_comm(dst->comm, sizeof(dst->comm), src->name, sizeof(src->name));
}

static void process_apps_snap_fold(void *dst_v, const void *values_v, int i)
{
    const struct netdata_ebpf_process_pid_entry *values = values_v;
    process_fill_snapshot_counters(dst_v, &values[i]);
}

static void process_snapshot_merge_same_pid(void *dst_v, const void *src_v)
{
    struct netdata_ebpf_process_pid_snapshot *dst = dst_v;
    const struct netdata_ebpf_process_pid_snapshot *src = src_v;

    if (src->ct > dst->ct)
        dst->ct = src->ct;
    dst->exits += src->exits;
    dst->task_close += src->task_close;
    dst->forks += src->forks;
    dst->clones += src->clones;
    dst->errors += src->errors;
    if (!dst->comm[0] && src->comm[0])
        nd_ebpf_copy_comm(dst->comm, sizeof(dst->comm), src->comm, sizeof(src->comm));
}

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
static void process_snapshot_from_acc(
    struct netdata_ebpf_process_runtime *rt,
    struct netdata_ebpf_process_pid_snapshot_list *out)
{
    if (!rt->acc.items || rt->acc.count == 0)
        return;

    if (rt->acc.count > rt->items_cap) {
        rt->items_buf = reallocz(rt->items_buf, rt->acc.count * sizeof(*rt->items_buf));
        rt->items_cap = rt->acc.count;
    }

    for (size_t i = 0; i < rt->acc.count; i++) {
        const struct netdata_ebpf_process_pid_entry *src = nd_ebpf_acc_item(&rt->acc, i);
        struct netdata_ebpf_process_pid_snapshot *dst = &rt->items_buf[i];
        memset(dst, 0, sizeof(*dst));
        dst->pid = src->tgid;
        process_fill_snapshot_counters(dst, src);
    }

    qsort(rt->items_buf, rt->acc.count, sizeof(*rt->items_buf), nd_ebpf_pid_first_u32_cmp);
    out->items = rt->items_buf;
    out->count = rt->acc.count;
}
#endif

int netdata_process_runtime_snapshot_apps(
    struct netdata_ebpf_process_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_process_pid_snapshot_list *out)
{
    (void)maps_per_core;

    struct bpf_object *obj = process_object(rt);
    if (!obj || !out)
        return -1;

    out->items = NULL;
    out->count = 0;

    struct bpf_map *map = bpf_object__find_map_by_name(obj, "tbl_pid_stats");
    if (!map) {
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
        /* Buffer and arena flavors publish events instead of a per-PID map. */
        if (rt->rb || rt->arena_state) {
            process_drain_events(rt);
            process_snapshot_from_acc(rt, out);
            return 0;
        }
#endif
        return -1;
    }

    int fd = bpf_map__fd(map);
    if (fd < 0 || !rt->percpu_entries)
        return -1;

    size_t count = nd_ebpf_apps_snap_iterate(
        map, fd, rt->percpu_entries_cap, rt->percpu_entries, sizeof(*rt->percpu_entries),
        offsetof(struct netdata_ebpf_process_pid_entry, tgid),
        offsetof(struct netdata_ebpf_process_pid_entry, ct),
        (void **)&rt->items_buf, &rt->items_cap, sizeof(*rt->items_buf),
        process_apps_snap_fold, process_snapshot_merge_same_pid,
        &rt->pid_keys);

    if (count) {
        out->items = rt->items_buf;
        out->count = count;
    }
    return 0;
}

void netdata_ebpf_process_pid_snapshot_list_free(struct netdata_ebpf_process_pid_snapshot_list *out)
{
    /* items_buf is owned by the runtime and freed in netdata_process_runtime_close(). */
    (void)out;
}

int netdata_process_runtime_delete_pids(struct netdata_ebpf_process_runtime *rt, uint32_t *pids, size_t count)
{
    if (!rt || !pids || count == 0)
        return 0;

    bool map_missing = false;
    int rc = nd_ebpf_map_delete_tgids(
        process_object(rt), "tbl_pid_stats", &rt->pid_keys, pids, count, rt->percpu_entries,
        sizeof(*rt->percpu_entries), offsetof(struct netdata_ebpf_process_pid_entry, tgid),
        offsetof(struct netdata_ebpf_process_pid_entry, ct), rt->percpu_entries_cap, &map_missing);
    if (!map_missing)
        return rc;

#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    for (size_t i = 0; i < count; i++)
        nd_ebpf_acc_evict_tgid(&rt->acc, pids[i]);
    if (rt->acc.count > 0)
        nd_ebpf_acc_rebuild(&rt->acc);
#endif
    return 0;
}

void netdata_process_runtime_close(struct netdata_ebpf_process_runtime *rt)
{
    if (!rt)
        return;

    nd_ebpf_destroy_links(&rt->links, rt->link_count);
    freez(rt->percpu_u64);
    freez(rt->percpu_entries);
    freez(rt->items_buf);
    nd_ebpf_key_table_free(&rt->pid_keys);
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    if (rt->rb)
        ring_buffer__free(rt->rb);
    nd_ebpf_acc_free(&rt->acc);
    if (rt->kind == PROCESS_KIND_CORE)
        process_destroy_core(rt);
    else if (rt->obj)
        bpf_object__close(rt->obj);
#else
    if (rt->obj)
        bpf_object__close(rt->obj);
#endif
    freez(rt);
}

unsigned long long netdata_process_runtime_arena_drops(struct netdata_ebpf_process_runtime *rt)
{
#ifdef NETDATA_PROCESS_LIBBPF_CORE_SUPPORTED
    return rt && rt->arena_state ? nd_ebpf_arena_dropped(rt->arena_state) : 0;
#else
    (void)rt;
    return 0;
#endif
}
