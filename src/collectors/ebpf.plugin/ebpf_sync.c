// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_sync.h"

static char *sync_counter_dimension_name[NETDATA_SYNC_IDX_END] =
    {"sync", "syncfs", "msync", "fsync", "fdatasync", "sync_file_range"};
static netdata_syscall_stat_t sync_counter_aggregated_data[NETDATA_SYNC_IDX_END];
static netdata_publish_syscall_t sync_counter_publish_aggregated[NETDATA_SYNC_IDX_END];

static netdata_idx_t sync_hash_values[NETDATA_SYNC_IDX_END];

ebpf_local_maps_t sync_maps[] = {
    {.name = "tbl_sync",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

ebpf_local_maps_t syncfs_maps[] = {
    {.name = "tbl_syncfs",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

ebpf_local_maps_t msync_maps[] = {
    {.name = "tbl_msync",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

ebpf_local_maps_t fsync_maps[] = {
    {.name = "tbl_fsync",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

ebpf_local_maps_t fdatasync_maps[] = {
    {.name = "tbl_fdatasync",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

ebpf_local_maps_t sync_file_range_maps[] = {
    {.name = "tbl_syncfr",
     .internal_input = NETDATA_SYNC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

struct config sync_config = APPCONFIG_INITIALIZER;

netdata_ebpf_targets_t sync_targets[] = {
    {.name = NETDATA_SYSCALLS_SYNC, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NETDATA_SYSCALLS_SYNCFS, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NETDATA_SYSCALLS_MSYNC, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NETDATA_SYSCALLS_FSYNC, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NETDATA_SYSCALLS_FDATASYNC, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NETDATA_SYSCALLS_SYNC_FILE_RANGE, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
/*****************************************************************
 *
 *  BTF FUNCTIONS
 *
 *****************************************************************/

/**
 * Disable probe
 *
 * Disable kprobe to use another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_sync_disable_probe(struct sync_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_sync_kprobe, false);
}

/**
 * Disable trampoline
 *
 * Disable trampoline to use another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_sync_disable_trampoline(struct sync_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_sync_fentry, false);
}

/**
 * Disable tracepoint
 *
 * Disable tracepoints according information given.
 *
 * @param obj  object loaded
 * @param idx  Which syscall will not be disabled
 */
void ebpf_sync_disable_tracepoints(struct sync_bpf *obj, sync_syscalls_index_t idx)
{
    if (idx != NETDATA_SYNC_SYNC_IDX)
        bpf_program__set_autoload(obj->progs.netdata_sync_entry, false);

    if (idx != NETDATA_SYNC_SYNCFS_IDX)
        bpf_program__set_autoload(obj->progs.netdata_syncfs_entry, false);

    if (idx != NETDATA_SYNC_MSYNC_IDX)
        bpf_program__set_autoload(obj->progs.netdata_msync_entry, false);

    if (idx != NETDATA_SYNC_FSYNC_IDX)
        bpf_program__set_autoload(obj->progs.netdata_fsync_entry, false);

    if (idx != NETDATA_SYNC_FDATASYNC_IDX)
        bpf_program__set_autoload(obj->progs.netdata_fdatasync_entry, false);

    if (idx != NETDATA_SYNC_SYNC_FILE_RANGE_IDX)
        bpf_program__set_autoload(obj->progs.netdata_sync_file_range_entry, false);
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param map    the map loaded.
 * @param obj    the main structure for bpf objects.
 */
static void ebpf_sync_set_hash_tables(ebpf_local_maps_t *map, struct sync_bpf *obj)
{
    map->map_fd = bpf_map__fd(obj->maps.tbl_sync);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj    is the main structure for bpf objects.
 * @param em     the structure with configuration
 * @param target the syscall that we are attaching a tracer.
 * @param idx    the index for the main structure
 *
 * @return it returns 0 on success and -1 otherwise
 */
static inline int
ebpf_sync_load_and_attach(struct sync_bpf *obj, ebpf_module_t *em, char *target, sync_syscalls_index_t idx)
{
    netdata_ebpf_targets_t *synct = em->targets;
    netdata_ebpf_program_loaded_t test = synct[NETDATA_SYNC_SYNC_IDX].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_sync_disable_probe(obj);
        ebpf_sync_disable_tracepoints(obj, NETDATA_SYNC_IDX_END);

        bpf_program__set_attach_target(obj->progs.netdata_sync_fentry, 0, target);
    } else if (test == EBPF_LOAD_PROBE || test == EBPF_LOAD_RETPROBE) {
        ebpf_sync_disable_tracepoints(obj, NETDATA_SYNC_IDX_END);
        ebpf_sync_disable_trampoline(obj);
    } else {
        ebpf_sync_disable_probe(obj);
        ebpf_sync_disable_trampoline(obj);

        ebpf_sync_disable_tracepoints(obj, idx);
    }

    ebpf_update_map_type(obj->maps.tbl_sync, &em->maps[NETDATA_SYNC_GLOBAL_TABLE]);

    int ret = sync_bpf__load(obj);
    if (!ret) {
        if (test != EBPF_LOAD_PROBE && test != EBPF_LOAD_RETPROBE) {
            ret = sync_bpf__attach(obj);
        } else {
            obj->links.netdata_sync_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_sync_kprobe, false, target);
            ret = (int)libbpf_get_error(obj->links.netdata_sync_kprobe);
        }

        if (!ret)
            ebpf_sync_set_hash_tables(&em->maps[NETDATA_SYNC_GLOBAL_TABLE], obj);
    }

    return ret;
}
#endif

/*****************************************************************
 *
 *  CLEANUP THREAD
 *
 *****************************************************************/

/**
 * Cleanup Objects
 *
 * Cleanup loaded objects when thread was initialized.
 */
void ebpf_sync_cleanup_objects()
{
    int i;
    for (i = 0; local_syscalls[i].syscall; i++) {
        ebpf_sync_syscalls_t *w = &local_syscalls[i];
#ifdef LIBBPF_MAJOR_VERSION
        if (w->sync_obj) {
            sync_bpf__destroy(w->sync_obj);
            w->sync_obj = NULL;
        }
#endif
        if (w->probe_links) {
            ebpf_unload_legacy_code(w->objects, w->probe_links);
            w->objects = NULL;
            w->probe_links = NULL;
        }
    }
}

/*
    static void ebpf_create_sync_chart(char *id,
                                       char *title,
                                       int order,
                                       int idx,
                                       int end,
                                       int update_every)
    {
        ebpf_write_chart_cmd(NETDATA_EBPF_MEMORY_GROUP, id, title, EBPF_COMMON_UNITS_CALL,
                             NETDATA_EBPF_SYNC_SUBMENU, NETDATA_EBPF_CHART_TYPE_LINE, NULL, order,
                             update_every,
                             NETDATA_EBPF_MODULE_NAME_SYNC);
 */

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_sync_global(ebpf_module_t *em)
{
    if (local_syscalls[NETDATA_SYNC_FSYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].enabled)
        ebpf_write_chart_obsolete(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_FILE_SYNC_CHART,
            "",
            "Monitor calls to fsync(2) and fdatasync(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_SYNC_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "mem.file_sync",
            21300,
            em->update_every);

    if (local_syscalls[NETDATA_SYNC_MSYNC_IDX].enabled)
        ebpf_write_chart_obsolete(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_MSYNC_CHART,
            "",
            "Monitor calls to msync(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_SYNC_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "mem.memory_map",
            21301,
            em->update_every);

    if (local_syscalls[NETDATA_SYNC_SYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_SYNCFS_IDX].enabled)
        ebpf_write_chart_obsolete(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_SYNC_CHART,
            "",
            "Monitor calls to sync(2) and syncfs(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_SYNC_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "mem.sync",
            21302,
            em->update_every);

    if (local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].enabled)
        ebpf_write_chart_obsolete(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_FILE_SEGMENT_CHART,
            "",
            "Monitor calls to sync_file_range(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_SYNC_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "mem.file_segment",
            21303,
            em->update_every);
}

/**
 * Exit
 *
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_sync_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        ebpf_obsolete_sync_global(em);
        pthread_mutex_unlock(&lock);
    }

    ebpf_sync_cleanup_objects();

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Load Legacy
 *
 * Load legacy code.
 *
 * @param w   is the sync output structure with pointers to objects loaded.
 * @param em  is structure with configuration
 *
 * @return 0 on success and -1 otherwise.
 */
static int ebpf_sync_load_legacy(ebpf_sync_syscalls_t *w, ebpf_module_t *em)
{
    em->info.thread_name = w->syscall;
    if (!w->probe_links) {
        w->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &w->objects);
        if (!w->probe_links) {
            return -1;
        }
    }

    return 0;
}

/*
 * Initialize Syscalls
 *
 * Load the eBPF programs to monitor syscalls
 *
 * @return 0 on success and -1 otherwise.
 */
static int ebpf_sync_initialize_syscall(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(sync_maps, em->maps_per_core, running_on_kernel);
    ebpf_define_map_type(syncfs_maps, em->maps_per_core, running_on_kernel);
    ebpf_define_map_type(msync_maps, em->maps_per_core, running_on_kernel);
    ebpf_define_map_type(fsync_maps, em->maps_per_core, running_on_kernel);
    ebpf_define_map_type(fdatasync_maps, em->maps_per_core, running_on_kernel);
    ebpf_define_map_type(sync_file_range_maps, em->maps_per_core, running_on_kernel);
#endif

    int i;
    const char *saved_name = em->info.thread_name;
    int errors = 0;
    for (i = 0; local_syscalls[i].syscall; i++) {
        ebpf_sync_syscalls_t *w = &local_syscalls[i];
        w->sync_maps = local_syscalls[i].sync_maps;
        em->maps = local_syscalls[i].sync_maps;
        if (w->enabled) {
            if (em->load & EBPF_LOAD_LEGACY) {
                if (ebpf_sync_load_legacy(w, em))
                    errors++;

                em->info.thread_name = saved_name;
            }
#ifdef LIBBPF_MAJOR_VERSION
            else {
                char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH];
                ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, w->syscall, running_on_kernel);
                if (ebpf_is_function_inside_btf(default_btf, syscall)) {
                    w->sync_obj = sync_bpf__open();
                    if (!w->sync_obj) {
                        w->enabled = false;
                        errors++;
                    } else {
                        if (ebpf_sync_load_and_attach(w->sync_obj, em, syscall, i)) {
                            w->enabled = false;
                            errors++;
                        }
                    }
                } else {
                    netdata_log_info("Cannot find syscall %s we are not going to monitor it.", syscall);
                    w->enabled = false;
                }

                em->info.thread_name = saved_name;
            }
#endif
        }
    }
    em->info.thread_name = saved_name;

    memset(sync_counter_aggregated_data, 0, NETDATA_SYNC_IDX_END * sizeof(netdata_syscall_stat_t));
    memset(sync_counter_publish_aggregated, 0, NETDATA_SYNC_IDX_END * sizeof(netdata_publish_syscall_t));
    memset(sync_hash_values, 0, NETDATA_SYNC_IDX_END * sizeof(netdata_idx_t));

    return (errors) ? -1 : 0;
}

/*****************************************************************
 *
 *  DATA THREAD
 *
 *****************************************************************/

/**
 * Read global table
 *
 * Read the table with number of calls for all functions
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_sync_read_global_table(int maps_per_core)
{
    netdata_idx_t stored[NETDATA_MAX_PROCESSOR];
    uint32_t idx = NETDATA_SYNC_CALL;
    int i;
    for (i = 0; local_syscalls[i].syscall; i++) {
        ebpf_sync_syscalls_t *w = &local_syscalls[i];
        if (w->enabled) {
            int fd = w->sync_maps[NETDATA_SYNC_GLOBAL_TABLE].map_fd;
            if (!bpf_map_lookup_elem(fd, &idx, &stored)) {
                int j, end = (maps_per_core) ? ebpf_nprocs : 1;
                netdata_idx_t total = 0;
                for (j = 0; j < end; j++)
                    total += stored[j];

                sync_hash_values[i] = total;
            }
        }
    }
}

/**
 * Create Sync charts
 *
 * Create charts and dimensions according user input.
 *
 * @param id        chart id
 * @param idx       the first index with data.
 * @param end       the last index with data.
 */
static void ebpf_send_sync_chart(char *id, int idx, int end)
{
    ebpf_write_begin_chart(NETDATA_EBPF_MEMORY_GROUP, id, "");

    netdata_publish_syscall_t *move = &sync_counter_publish_aggregated[idx];

    while (move && idx <= end) {
        if (local_syscalls[idx].enabled)
            write_chart_dimension(move->name, (long long)sync_hash_values[idx]);

        move = move->next;
        idx++;
    }

    ebpf_write_end_chart();
}

/**
 * Send data
 *
 * Send global charts to Netdata
 */
static void sync_send_data()
{
    if (local_syscalls[NETDATA_SYNC_FSYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].enabled) {
        ebpf_send_sync_chart(NETDATA_EBPF_FILE_SYNC_CHART, NETDATA_SYNC_FSYNC_IDX, NETDATA_SYNC_FDATASYNC_IDX);
    }

    if (local_syscalls[NETDATA_SYNC_MSYNC_IDX].enabled)
        ebpf_one_dimension_write_charts(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_MSYNC_CHART,
            sync_counter_publish_aggregated[NETDATA_SYNC_MSYNC_IDX].dimension,
            sync_hash_values[NETDATA_SYNC_MSYNC_IDX]);

    if (local_syscalls[NETDATA_SYNC_SYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_SYNCFS_IDX].enabled) {
        ebpf_send_sync_chart(NETDATA_EBPF_SYNC_CHART, NETDATA_SYNC_SYNC_IDX, NETDATA_SYNC_SYNCFS_IDX);
    }

    if (local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].enabled)
        ebpf_one_dimension_write_charts(
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_FILE_SEGMENT_CHART,
            sync_counter_publish_aggregated[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].dimension,
            sync_hash_values[NETDATA_SYNC_SYNC_FILE_RANGE_IDX]);
}

/**
* Main loop for this collector.
*/
static void sync_collector(ebpf_module_t *em)
{
    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        ebpf_sync_read_global_table(maps_per_core);
        pthread_mutex_lock(&lock);

        sync_send_data();

        pthread_mutex_unlock(&lock);

        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Create Sync charts
 *
 * Create charts and dimensions according user input.
 *
 * @param id        chart id
 * @param title     chart title
 * @param order     order number of the specified chart
 * @param idx       the first index with data.
 * @param end       the last index with data.
 * @param update_every value to overwrite the update frequency set by the server.
 * @param context   the chart context
 */
static void ebpf_create_sync_chart(char *id, char *title, int order, int idx, int end, int update_every, char *context)
{
    ebpf_write_chart_cmd(
        NETDATA_EBPF_MEMORY_GROUP,
        id,
        "",
        title,
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_EBPF_SYNC_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        context,
        order,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SYNC);

    netdata_publish_syscall_t *move = &sync_counter_publish_aggregated[idx];

    while (move && idx <= end) {
        if (local_syscalls[idx].enabled)
            ebpf_write_global_dimension(move->name, move->dimension, move->algorithm);

        move = move->next;
        idx++;
    }
}

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_sync_charts(int update_every)
{
    if (local_syscalls[NETDATA_SYNC_FSYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].enabled)
        ebpf_create_sync_chart(
            NETDATA_EBPF_FILE_SYNC_CHART,
            "Monitor calls to fsync(2) and fdatasync(2).",
            21300,
            NETDATA_SYNC_FSYNC_IDX,
            NETDATA_SYNC_FDATASYNC_IDX,
            update_every,
            "mem.file_sync");

    if (local_syscalls[NETDATA_SYNC_MSYNC_IDX].enabled)
        ebpf_create_sync_chart(
            NETDATA_EBPF_MSYNC_CHART,
            "Monitor calls to msync(2).",
            21301,
            NETDATA_SYNC_MSYNC_IDX,
            NETDATA_SYNC_MSYNC_IDX,
            update_every,
            "mem.memory_map");

    if (local_syscalls[NETDATA_SYNC_SYNC_IDX].enabled && local_syscalls[NETDATA_SYNC_SYNCFS_IDX].enabled)
        ebpf_create_sync_chart(
            NETDATA_EBPF_SYNC_CHART,
            "Monitor calls to sync(2) and syncfs(2).",
            21302,
            NETDATA_SYNC_SYNC_IDX,
            NETDATA_SYNC_SYNCFS_IDX,
            update_every,
            "mem.sync");

    if (local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].enabled)
        ebpf_create_sync_chart(
            NETDATA_EBPF_FILE_SEGMENT_CHART,
            "Monitor calls to sync_file_range(2).",
            21303,
            NETDATA_SYNC_SYNC_FILE_RANGE_IDX,
            NETDATA_SYNC_SYNC_FILE_RANGE_IDX,
            update_every,
            "mem.file_segment");

    fflush(stdout);
}

/**
 * Parse Syscalls
 *
 * Parse syscall options available inside ebpf.d/sync.conf
 */
static void ebpf_sync_parse_syscalls()
{
    for (int i = 0; local_syscalls[i].syscall; i++) {
        local_syscalls[i].enabled = inicfg_get_boolean(&sync_config, NETDATA_SYNC_CONFIG_NAME,
                                                          local_syscalls[i].syscall, CONFIG_BOOLEAN_YES);
    }
}

/**
 * Set sync maps
 *
 * When thread is initialized the variable sync_maps is set as null,
 * this function fills the variable before to use.
 */
static void ebpf_set_sync_maps()
{
    local_syscalls[NETDATA_SYNC_SYNC_IDX].sync_maps = sync_maps;
    local_syscalls[NETDATA_SYNC_SYNCFS_IDX].sync_maps = syncfs_maps;
    local_syscalls[NETDATA_SYNC_MSYNC_IDX].sync_maps = msync_maps;
    local_syscalls[NETDATA_SYNC_FSYNC_IDX].sync_maps = fsync_maps;
    local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].sync_maps = fdatasync_maps;
    local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].sync_maps = sync_file_range_maps;
}

/**
 * Sync thread
 *
 * Thread used to make sync thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_sync_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_sync_exit) cleanup_ptr = em;

    ebpf_set_sync_maps();
    ebpf_sync_parse_syscalls();

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_sync_initialize_syscall(em)) {
        goto endsync;
    }

    int algorithms[NETDATA_SYNC_IDX_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX};
    ebpf_global_labels(
        sync_counter_aggregated_data,
        sync_counter_publish_aggregated,
        sync_counter_dimension_name,
        sync_counter_dimension_name,
        algorithms,
        NETDATA_SYNC_IDX_END);

    pthread_mutex_lock(&lock);
    ebpf_create_sync_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    sync_collector(em);

endsync:
    ebpf_update_disabled_plugin_stats(em);
}
