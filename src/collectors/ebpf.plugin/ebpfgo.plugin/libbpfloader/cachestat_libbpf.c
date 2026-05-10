//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

struct netdata_ebpf_cachestat_runtime {
    struct bpf_object *obj;
    struct bpf_link **links;
};

struct netdata_ebpf_cachestat_snapshot {
    uint64_t mark_page_accessed;
    uint64_t mark_buffer_dirty;
    uint64_t add_to_page_cache_lru;
    uint64_t account_page_dirtied;
};

static const char *cachestat_account_program_name(const char *account_function)
{
    if (!strcmp(account_function, "__folio_mark_dirty"))
        return "netdata_folio_mark_dirty_kprobe";
    if (!strcmp(account_function, "__set_page_dirty"))
        return "netdata_set_page_dirty_kprobe";
    return "netdata_account_page_dirtied_kprobe";
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

static void cachestat_prepare_autoload(struct bpf_object *obj, const char *account_function)
{
    cachestat_disable_fentry_programs(obj);
    cachestat_disable_program_if_present(obj, "netdata_folio_mark_dirty_kprobe");
    cachestat_disable_program_if_present(obj, "netdata_set_page_dirty_kprobe");
    cachestat_disable_program_if_present(obj, "netdata_account_page_dirtied_kprobe");

    if (!account_function)
        account_function = "account_page_dirtied";

    struct bpf_program *prog = bpf_object__find_program_by_name(obj, cachestat_account_program_name(account_function));
    if (prog)
        bpf_program__set_autoload(prog, true);
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

struct netdata_ebpf_cachestat_runtime *netdata_cachestat_runtime_open(const char *path)
{
    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        return NULL;
    }

    struct netdata_ebpf_cachestat_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt) {
        bpf_object__close(obj);
        return NULL;
    }

    rt->obj = obj;
    return rt;
}

int netdata_cachestat_runtime_prepare(struct netdata_ebpf_cachestat_runtime *rt, unsigned int pid_table_size, int maps_per_core)
{
    if (!rt || !rt->obj)
        return -1;

    cachestat_prepare_autoload(rt->obj, NULL);
    cachestat_update_map_types(rt->obj, maps_per_core);
    cachestat_update_map_sizes(rt->obj, pid_table_size);
    return 0;
}

int netdata_cachestat_runtime_load(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

    return bpf_object__load(rt->obj);
}

int netdata_cachestat_runtime_attach(struct netdata_ebpf_cachestat_runtime *rt, const char *account_function)
{
    if (!rt || !rt->obj)
        return -1;

    if (rt->links)
        cachestat_destroy_links(rt);

    if (!account_function)
        account_function = "account_page_dirtied";

    rt->links = calloc(4, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    rt->links[0] = cachestat_attach_program_by_name(rt->obj, "netdata_add_to_page_cache_lru_kprobe", "add_to_page_cache_lru");
    rt->links[1] = cachestat_attach_program_by_name(rt->obj, "netdata_mark_page_accessed_kprobe", "mark_page_accessed");
    rt->links[2] = cachestat_attach_program_by_name(rt->obj, cachestat_account_program_name(account_function), account_function);
    rt->links[3] = cachestat_attach_program_by_name(rt->obj, "netdata_mark_buffer_dirty_kprobe", "mark_buffer_dirty");

    for (size_t i = 0; i < 4; i++) {
        if (!rt->links[i] || libbpf_get_error(rt->links[i])) {
            cachestat_destroy_links(rt);
            return -1;
        }
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

    struct bpf_map *map = bpf_object__find_map_by_name(rt->obj, "cstat_global");
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

void netdata_cachestat_runtime_close(struct netdata_ebpf_cachestat_runtime *rt)
{
    if (!rt)
        return;

    cachestat_destroy_links(rt);
    if (rt->obj)
        bpf_object__close(rt->obj);
    free(rt);
}
