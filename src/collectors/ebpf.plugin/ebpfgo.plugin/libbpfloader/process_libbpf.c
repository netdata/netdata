//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

#include "../nd_alloc_shim.h"
#include "nd_ebpf_runtime_common.h"

#if defined(__has_include) && __has_include("process.skel.h")
#ifndef NETDATA_PROCESS_ARENA_STATE_T_DEFINED
struct netdata_process_arena_state_t { unsigned char __pad[49160]; };
#define NETDATA_PROCESS_ARENA_STATE_T_DEFINED 1
#endif
#include "process.skel.h"
#include "process_buffer.skel.h"
#include "process_arena.skel.h"
#define PROCESS_HAS_CORE 1
#endif

enum { PROCESS_BASE, PROCESS_BUFFER, PROCESS_ARENA };

struct process_entry {
	uint64_t ct;
	uint32_t tgid, uid, gid;
	char name[16];
	uint32_t exit_call, release_call, create_process, create_thread, task_err;
};

struct netdata_ebpf_process_runtime {
	int core, flavor;
	struct bpf_object *obj;
	struct bpf_link **links;
	size_t link_count;
	uint64_t *percpu;
	int percpu_count;
	struct process_entry *values;
	int values_cap;
	struct netdata_ebpf_process_pid_snapshot *items;
	size_t items_cap;
#ifdef PROCESS_HAS_CORE
	union { struct process_bpf *base; struct process_buffer_bpf *buffer; struct process_arena_bpf *arena; } skel;
	struct ring_buffer *rb;
	void *arena_state;
	uint32_t arena_tail;
	struct nd_ebpf_acc_table acc;
#endif
};

struct netdata_ebpf_process_snapshot { uint64_t exits, task_close, forks, clones, errors; };
struct netdata_ebpf_process_pid_snapshot { uint32_t pid, ppid; uint64_t ct; char comm[96]; uint32_t exits, task_close, forks, clones, errors; };
struct netdata_ebpf_process_pid_snapshot_list { struct netdata_ebpf_process_pid_snapshot *items; size_t count; };

static struct bpf_object *process_object(struct netdata_ebpf_process_runtime *rt)
{
	return rt ? rt->obj : NULL;
}

static int process_flavor(const char *path)
{
	if (path && strstr(path, "_arena.")) return PROCESS_ARENA;
	if (path && strstr(path, "_buffer.")) return PROCESS_BUFFER;
	return PROCESS_BASE;
}

int netdata_process_runtime_supports_core(void)
{
#ifdef PROCESS_HAS_CORE
	return 1;
#else
	return 0;
#endif
}

struct netdata_ebpf_process_runtime *netdata_process_runtime_open_mode(const char *path, int use_core)
{
	struct netdata_ebpf_process_runtime *rt = callocz(1, sizeof(*rt));
	if (!rt) return NULL;
	rt->core = use_core;
	rt->flavor = process_flavor(path);
#ifdef PROCESS_HAS_CORE
	if (use_core) {
		struct bpf_object_open_opts opts = {
			.sz = sizeof(opts),
		};
		switch (rt->flavor) {
		case PROCESS_BUFFER: rt->skel.buffer = process_buffer_bpf__open_opts(&opts); rt->obj = rt->skel.buffer ? rt->skel.buffer->obj : NULL; break;
		case PROCESS_ARENA: rt->skel.arena = process_arena_bpf__open_opts(&opts); rt->obj = rt->skel.arena ? rt->skel.arena->obj : NULL; break;
		default: rt->skel.base = process_bpf__open_opts(&opts); rt->obj = rt->skel.base ? rt->skel.base->obj : NULL; break;
		}
	} else
#else
	(void)use_core;
#endif
	{
		rt->obj = bpf_object__open_file(path, NULL);
	}
	if (!rt->obj || libbpf_get_error(rt->obj)) { freez(rt); return NULL; }
	return rt;
}

int netdata_process_runtime_prepare(struct netdata_ebpf_process_runtime *rt, unsigned int size, int maps_per_core)
{
	if (!rt || !rt->obj) return -1;
	const char *maps[] = {"tbl_total_stats", "process_ctrl", "tbl_pid_stats"};
	if (nd_ebpf_update_map_types(rt->obj, maps, 3, maps_per_core, "process") != 0) return -1;
	struct bpf_map *pid = bpf_object__find_map_by_name(rt->obj, "tbl_pid_stats");
	if (pid && size) bpf_map__set_max_entries(pid, size);
	if (nd_ebpf_alloc_percpu_buffers(&rt->percpu, &rt->percpu_count, (void **)&rt->values, &rt->values_cap, sizeof(*rt->values)) != 0) return -1;
#ifdef PROCESS_HAS_CORE
	if (rt->core && rt->flavor != PROCESS_BASE) {
		nd_ebpf_acc_init(&rt->acc, sizeof(struct process_entry), offsetof(struct process_entry, tgid));
		nd_ebpf_acc_set_max_entries(&rt->acc, size);
	}
#endif
	return 0;
}

#ifdef PROCESS_HAS_CORE
static int process_event(void *ctx, void *data, size_t size)
{
	if (size < sizeof(struct nd_ebpf_pid_event)) return 0;
	struct netdata_ebpf_process_runtime *rt = ctx;
	const struct nd_ebpf_pid_event *ev = data;
	struct process_entry *entry = nd_ebpf_acc_find_or_add(&rt->acc, ev->tgid ? ev->tgid : ev->pid);
	if (!entry) return 0;
	if (ev->ct > entry->ct) entry->ct = ev->ct;
	if (!entry->name[0]) nd_ebpf_copy_comm(entry->name, sizeof(entry->name), ev->name, sizeof(ev->name));
	switch (ev->action) {
	case 0: entry->exit_call++; break;
	case 1: entry->release_call++; break;
	case 2: entry->create_process++; break;
	case 3: entry->create_thread++; break;
	default: break;
	}
	if (ev->error) entry->task_err++;
	return 0;
}

static void process_event_arena(void *ctx, const struct nd_ebpf_pid_event *ev)
{
	process_event(ctx, (void *)ev, sizeof(*ev));
}

static int process_setup_events(struct netdata_ebpf_process_runtime *rt)
{
	if (rt->flavor == PROCESS_BUFFER) {
		rt->rb = nd_ebpf_ring_buffer_open(rt->obj, "process_events", process_event, rt, "process");
		return rt->rb ? 0 : -1;
	}
	if (rt->flavor == PROCESS_ARENA && rt->skel.arena && rt->skel.arena->bss)
		rt->arena_state = &rt->skel.arena->bss->process_arena_state;
	return rt->arena_state ? 0 : -1;
}
#endif

int netdata_process_runtime_load(struct netdata_ebpf_process_runtime *rt)
{
	if (!rt || !rt->obj) return -1;
#ifdef PROCESS_HAS_CORE
	if (rt->core) {
		switch (rt->flavor) {
		case PROCESS_BUFFER: return process_buffer_bpf__load(rt->skel.buffer) ? -1 : 0;
		case PROCESS_ARENA: return process_arena_bpf__load(rt->skel.arena) ? -1 : 0;
		default: return process_bpf__load(rt->skel.base) ? -1 : 0;
		}
	}
#endif
	return bpf_object__load(rt->obj);
}

int netdata_process_runtime_attach(struct netdata_ebpf_process_runtime *rt)
{
	if (!rt || !rt->obj) return -1;
#ifdef PROCESS_HAS_CORE
	if (rt->core) {
		switch (rt->flavor) {
		case PROCESS_BUFFER: return process_buffer_bpf__attach(rt->skel.buffer) ? -1 : 0;
		case PROCESS_ARENA: return process_arena_bpf__attach(rt->skel.arena) ? -1 : 0;
		default: return process_bpf__attach(rt->skel.base) ? -1 : 0;
		}
	}
#endif
	const char *names[] = {"netdata_tracepoint_sched_process_exit", "netdata_tracepoint_sched_process_exec", "netdata_tracepoint_sched_process_fork", "netdata_release_task", "netdata_sys_clone"};
	rt->links = callocz(5, sizeof(*rt->links));
	if (!rt->links) return -1;
	for (size_t i = 0; i < 5; i++) {
		struct bpf_program *prog = bpf_object__find_program_by_name(rt->obj, names[i]);
		if (!prog) continue;
		if (i < 3) {
			const char *tp[] = {"sched_process_exit", "sched_process_exec", "sched_process_fork"};
			rt->links[rt->link_count++] = bpf_program__attach_tracepoint(prog, "sched", tp[i]);
		} else {
			const char *target = i == 3 ? "release_task" : "kernel_clone";
			rt->links[rt->link_count++] = bpf_program__attach_kprobe(prog, false, target);
		}
	}
	for (size_t i = 0; i < rt->link_count; i++) if (!rt->links[i] || libbpf_get_error(rt->links[i])) return -1;
	return rt->link_count ? 0 : -1;
}

int netdata_process_runtime_update_controller(struct netdata_ebpf_process_runtime *rt, int enabled, int level)
{
	return nd_ebpf_update_controller(process_object(rt), "process_ctrl", enabled, level);
}

static void process_sum(uint32_t key, struct netdata_ebpf_process_runtime *rt, struct netdata_ebpf_process_snapshot *out)
{
	struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "tbl_total_stats");
	if (!map || bpf_map_lookup_elem(bpf_map__fd(map), &key, rt->values) != 0) return;
	int n = bpf_map__type(map) == BPF_MAP_TYPE_PERCPU_ARRAY ? rt->percpu_count : 1;
	struct process_entry *v = rt->values;
	for (int i = 0; i < n; i++, v++) { out->exits += v->exit_call; out->task_close += v->release_call; out->forks += v->create_process; out->clones += v->create_thread; out->errors += v->task_err; }
}

#ifdef PROCESS_HAS_CORE
static void process_acc_sum(struct netdata_ebpf_process_runtime *rt, struct netdata_ebpf_process_snapshot *out)
{
	for (size_t i = 0; i < rt->acc.count; i++) {
		const struct process_entry *v = nd_ebpf_acc_item(&rt->acc, i);
		out->exits += v->exit_call;
		out->task_close += v->release_call;
		out->forks += v->create_process;
		out->clones += v->create_thread;
		out->errors += v->task_err;
	}
}
#endif

int netdata_process_runtime_snapshot(struct netdata_ebpf_process_runtime *rt, int maps_per_core, struct netdata_ebpf_process_snapshot *out)
{
	(void)maps_per_core;
	if (!rt || !out || !rt->values) return -1;
	memset(out, 0, sizeof(*out));
	uint32_t key = 0;
#ifdef PROCESS_HAS_CORE
	if (rt->core && rt->flavor != PROCESS_BASE) {
		if (!rt->rb && !rt->arena_state) process_setup_events(rt);
		if (rt->rb) ring_buffer__poll(rt->rb, 0);
		if (rt->arena_state) rt->arena_tail = nd_ebpf_arena_drain(rt->arena_state, rt->arena_tail, process_event_arena, rt);
		process_acc_sum(rt, out);
	} else
#endif
	{
		process_sum(key, rt, out);
	}
	return 0;
}

int netdata_process_runtime_snapshot_apps(struct netdata_ebpf_process_runtime *rt, int maps_per_core, struct netdata_ebpf_process_pid_snapshot_list *out)
{
	(void)maps_per_core;
	if (!rt || !out) return -1;
	out->items = NULL; out->count = 0;
	if (!rt->values) return -1;
	struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "tbl_pid_stats");
	if (map) {
		int fd = bpf_map__fd(map), key = -1, next;
		int n = bpf_map__type(map) == BPF_MAP_TYPE_PERCPU_HASH ? rt->values_cap : 1;
		while (bpf_map_get_next_key(fd, key < 0 ? NULL : &key, &next) == 0) {
			if (bpf_map_lookup_elem(fd, &next, rt->values) == 0) {
				if (out->count == rt->items_cap) { size_t cap = rt->items_cap ? rt->items_cap * 2 : 64; rt->items = reallocz(rt->items, cap * sizeof(*rt->items)); rt->items_cap = cap; }
				struct netdata_ebpf_process_pid_snapshot *dst = &rt->items[out->count++]; memset(dst, 0, sizeof(*dst));
				for (int i = 0; i < n; i++) { struct process_entry *src = &rt->values[i]; if (src->ct > dst->ct) dst->ct = src->ct; if (!dst->comm[0]) nd_ebpf_copy_comm(dst->comm, sizeof(dst->comm), src->name, sizeof(src->name)); dst->pid = src->tgid ? src->tgid : (uint32_t)next; dst->exits += src->exit_call; dst->task_close += src->release_call; dst->forks += src->create_process; dst->clones += src->create_thread; dst->errors += src->task_err; }
			}
			key = next;
		}
		if (out->count) { qsort(rt->items, out->count, sizeof(*out->items), nd_ebpf_pid_first_u32_cmp); out->items = rt->items; }
	}
	#ifdef PROCESS_HAS_CORE
	if (rt->core && rt->flavor != PROCESS_BASE) {
		if (!rt->rb && !rt->arena_state) process_setup_events(rt);
		if (rt->rb) ring_buffer__poll(rt->rb, 0);
		if (rt->arena_state) rt->arena_tail = nd_ebpf_arena_drain(rt->arena_state, rt->arena_tail, process_event_arena, rt);
		if (rt->acc.count) {
			if (rt->acc.count > rt->items_cap) { rt->items = reallocz(rt->items, rt->acc.count * sizeof(*rt->items)); rt->items_cap = rt->acc.count; }
			for (size_t i = 0; i < rt->acc.count; i++) {
				struct process_entry *src = nd_ebpf_acc_item(&rt->acc, i);
				struct netdata_ebpf_process_pid_snapshot *dst = &rt->items[i];
				memset(dst, 0, sizeof(*dst)); dst->pid = src->tgid; dst->ct = src->ct;
				nd_ebpf_copy_comm(dst->comm, sizeof(dst->comm), src->name, sizeof(src->name));
				dst->exits = src->exit_call; dst->task_close = src->release_call; dst->forks = src->create_process; dst->clones = src->create_thread; dst->errors = src->task_err;
			}
			qsort(rt->items, rt->acc.count, sizeof(*rt->items), nd_ebpf_pid_first_u32_cmp);
			out->items = rt->items; out->count = rt->acc.count;
		}
	}
	#endif
	return 0;
}

void netdata_ebpf_process_pid_snapshot_list_free(struct netdata_ebpf_process_pid_snapshot_list *out)
{
	(void)out;
}

int netdata_process_runtime_delete_pids(struct netdata_ebpf_process_runtime *rt, unsigned int *pids, size_t count)
{
	if (!rt || !pids) return -1;
#ifdef PROCESS_HAS_CORE
	if (rt->core && rt->flavor != PROCESS_BASE) {
		for (size_t i = 0; i < count; i++) nd_ebpf_acc_evict_tgid(&rt->acc, pids[i]);
		return 0;
	}
#endif
	struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "tbl_pid_stats");
	if (!map) return 0;
	int fd = bpf_map__fd(map);
	for (size_t i = 0; i < count; i++) bpf_map_delete_elem(fd, &pids[i]);
	return 0;
}

void netdata_process_runtime_close(struct netdata_ebpf_process_runtime *rt)
{
	if (!rt) return;
#ifdef PROCESS_HAS_CORE
	if (rt->rb) ring_buffer__free(rt->rb);
	nd_ebpf_acc_free(&rt->acc);
	if (rt->core) {
		switch (rt->flavor) { case PROCESS_BUFFER: process_buffer_bpf__destroy(rt->skel.buffer); break; case PROCESS_ARENA: process_arena_bpf__destroy(rt->skel.arena); break; default: process_bpf__destroy(rt->skel.base); break; }
		freez(rt->items); freez(rt); return;
	}
#endif
	if (rt->links) { nd_ebpf_destroy_links(&rt->links, rt->link_count); }
	if (rt->obj) bpf_object__close(rt->obj);
	freez(rt->items);
	freez(rt);
}
