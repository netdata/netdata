// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_sync.h"

static char *sync_counter_dimension_name[NETDATA_SYNC_IDX_END] = { "sync", "syncfs",  "msync", "fsync", "fdatasync",
                                                                   "sync_file_range" };
static netdata_syscall_stat_t sync_counter_aggregated_data[NETDATA_SYNC_IDX_END];
static netdata_publish_syscall_t sync_counter_publish_aggregated[NETDATA_SYNC_IDX_END];

static netdata_idx_t sync_hash_values[NETDATA_SYNC_IDX_END];

static ebpf_local_maps_t sync_maps[] = {{.name = "tbl_sync", .internal_input = NETDATA_SYNC_END,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_syncfs", .internal_input = NETDATA_SYNC_END,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_msync", .internal_input = NETDATA_SYNC_END,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_fsync", .internal_input = NETDATA_SYNC_END,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_fdatasync", .internal_input = NETDATA_SYNC_END,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                         {.name = "tbl_syncfr", .internal_input = NETDATA_SYNC_END,
                                          .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                          .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = NULL, .internal_input = 0, .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_CONTROLLER,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

struct config sync_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

netdata_ebpf_targets_t sync_targets[] = { {.name = NETDATA_SYSCALLS_SYNC, .mode = EBPF_LOAD_TRAMPOLINE},
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
 * @param obj is the main structure for bpf objects.
 * @param idx    the index for the main structure
 */
static void ebpf_sync_set_hash_tables(struct sync_bpf *obj, sync_syscalls_index_t idx)
{
    sync_maps[idx].map_fd = bpf_map__fd(obj->maps.tbl_sync);
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
static inline int ebpf_sync_load_and_attach(struct sync_bpf *obj, ebpf_module_t *em, char *target,
                                            sync_syscalls_index_t idx)
{
    netdata_ebpf_targets_t *synct = em->targets;
    netdata_ebpf_program_loaded_t test = synct[NETDATA_SYNC_SYNC_IDX].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_sync_disable_probe(obj);
        ebpf_sync_disable_tracepoints(obj, NETDATA_SYNC_IDX_END);

        bpf_program__set_attach_target(obj->progs.netdata_sync_fentry, 0,
                                       target);
    } else if (test == EBPF_LOAD_PROBE ||
    test == EBPF_LOAD_RETPROBE) {
        ebpf_sync_disable_tracepoints(obj, NETDATA_SYNC_IDX_END);
        ebpf_sync_disable_trampoline(obj);
    } else {
        ebpf_sync_disable_probe(obj);
        ebpf_sync_disable_trampoline(obj);

        ebpf_sync_disable_tracepoints(obj, idx);
    }

    int ret = sync_bpf__load(obj);
    if (!ret) {
        if (test != EBPF_LOAD_PROBE && test != EBPF_LOAD_RETPROBE) {
            ret = sync_bpf__attach(obj);
        } else {
            obj->links.netdata_sync_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_sync_kprobe,
                                                                        false, target);
            ret = (int)libbpf_get_error(obj->links.netdata_sync_kprobe);
        }

        if (!ret)
            ebpf_sync_set_hash_tables(obj, idx);
    }

    return ret;
}
#endif

/*****************************************************************
 *
 *  CLEANUP THREAD
 *
 *****************************************************************/

#ifdef LIBBPF_MAJOR_VERSION
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
        if (w->sync_obj)
            sync_bpf__destroy(w->sync_obj);
    }
}
#endif

/**
 * Sync Free
 *
 * Cleanup variables after child threads to stop
 *
 * @param ptr thread data.
 */
static void ebpf_sync_free(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_sync_cleanup_objects();
#endif

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/**
 * Exit
 *
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_sync_exit(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    ebpf_sync_free(em);
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
    em->thread_name = w->syscall;
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
    int i;
    const char *saved_name = em->thread_name;
    int errors = 0;
    for (i = 0; local_syscalls[i].syscall; i++) {
        ebpf_sync_syscalls_t *w = &local_syscalls[i];
        if (w->enabled) {
            if (em->load & EBPF_LOAD_LEGACY) {
                if (ebpf_sync_load_legacy(w, em))
                    errors++;

                em->thread_name = saved_name;
            }
#ifdef LIBBPF_MAJOR_VERSION
            else {
                char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH];
                ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, w->syscall, running_on_kernel);
                w->sync_obj = sync_bpf__open();
                if (!w->sync_obj) {
                    errors++;
                } else {
                    if (ebpf_is_function_inside_btf(default_btf, syscall)) {
                        if (ebpf_sync_load_and_attach(w->sync_obj, em, syscall, i)) {
                            errors++;
                        }
                    } else {
                        if (ebpf_sync_load_legacy(w, em))
                            errors++;
                    }
                    em->thread_name = saved_name;
                }
            }
#endif
        }
    }
    em->thread_name = saved_name;

    memset(sync_counter_aggregated_data, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_syscall_stat_t));
    memset(sync_counter_publish_aggregated, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_publish_syscall_t));
    memset(sync_hash_values, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_idx_t));

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
 */
static void ebpf_sync_read_global_table()
{
    netdata_idx_t stored;
    uint32_t idx = NETDATA_SYNC_CALL;
    int i;
    for (i = 0; local_syscalls[i].syscall; i++) {
        if (local_syscalls[i].enabled) {
            int fd = sync_maps[i].map_fd;
            if (!bpf_map_lookup_elem(fd, &idx, &stored)) {
                sync_hash_values[i] = stored;
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
static void ebpf_send_sync_chart(char *id,
                                   int idx,
                                   int end)
{
    write_begin_chart(NETDATA_EBPF_MEMORY_GROUP, id);

    netdata_publish_syscall_t *move = &sync_counter_publish_aggregated[idx];

    while (move && idx <= end) {
        if (local_syscalls[idx].enabled)
            write_chart_dimension(move->name, sync_hash_values[idx]);

        move = move->next;
        idx++;
    }

    write_end_chart();
}

/**
 * Send data
 *
 * Send global charts to Netdata
 */
static void sync_send_data()
{
    if (local_syscalls[NETDATA_SYNC_FSYNC_IDX].enabled || local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].enabled) {
        ebpf_send_sync_chart(NETDATA_EBPF_FILE_SYNC_CHART, NETDATA_SYNC_FSYNC_IDX, NETDATA_SYNC_FDATASYNC_IDX);
    }

    if (local_syscalls[NETDATA_SYNC_MSYNC_IDX].enabled)
        ebpf_one_dimension_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_EBPF_MSYNC_CHART,
                                        sync_counter_publish_aggregated[NETDATA_SYNC_MSYNC_IDX].dimension,
                                        sync_hash_values[NETDATA_SYNC_MSYNC_IDX]);

    if (local_syscalls[NETDATA_SYNC_SYNC_IDX].enabled || local_syscalls[NETDATA_SYNC_SYNCFS_IDX].enabled) {
        ebpf_send_sync_chart(NETDATA_EBPF_SYNC_CHART, NETDATA_SYNC_SYNC_IDX, NETDATA_SYNC_SYNCFS_IDX);
    }

    if (local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].enabled)
        ebpf_one_dimension_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_EBPF_FILE_SEGMENT_CHART,
                                        sync_counter_publish_aggregated[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].dimension,
                                        sync_hash_values[NETDATA_SYNC_SYNC_FILE_RANGE_IDX]);
}

/**
* Main loop for this collector.
*/
static void sync_collector(ebpf_module_t *em)
{
    heartbeat_t hb;
    heartbeat_init(&hb);
    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!ebpf_exit_plugin) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);
        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        counter = 0;
        ebpf_sync_read_global_table();
        pthread_mutex_lock(&lock);

        sync_send_data();

        pthread_mutex_unlock(&lock);
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
 */
static void ebpf_create_sync_chart(char *id,
                                   char *title,
                                   int order,
                                   int idx,
                                   int end,
                                   int update_every)
{
    ebpf_write_chart_cmd(NETDATA_EBPF_MEMORY_GROUP, id, title, EBPF_COMMON_DIMENSION_CALL,
                         NETDATA_EBPF_SYNC_SUBMENU, NETDATA_EBPF_CHART_TYPE_LINE, NULL, order,
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
    if (local_syscalls[NETDATA_SYNC_FSYNC_IDX].enabled || local_syscalls[NETDATA_SYNC_FDATASYNC_IDX].enabled)
        ebpf_create_sync_chart(NETDATA_EBPF_FILE_SYNC_CHART,
                               "Monitor calls for <code>fsync(2)</code> and <code>fdatasync(2)</code>.", 21300,
                               NETDATA_SYNC_FSYNC_IDX, NETDATA_SYNC_FDATASYNC_IDX, update_every);

    if (local_syscalls[NETDATA_SYNC_MSYNC_IDX].enabled)
        ebpf_create_sync_chart(NETDATA_EBPF_MSYNC_CHART,
                               "Monitor calls for <code>msync(2)</code>.", 21301,
                               NETDATA_SYNC_MSYNC_IDX, NETDATA_SYNC_MSYNC_IDX, update_every);

    if (local_syscalls[NETDATA_SYNC_SYNC_IDX].enabled || local_syscalls[NETDATA_SYNC_SYNCFS_IDX].enabled)
        ebpf_create_sync_chart(NETDATA_EBPF_SYNC_CHART,
                               "Monitor calls for <code>sync(2)</code> and <code>syncfs(2)</code>.", 21302,
                               NETDATA_SYNC_SYNC_IDX, NETDATA_SYNC_SYNCFS_IDX, update_every);

    if (local_syscalls[NETDATA_SYNC_SYNC_FILE_RANGE_IDX].enabled)
        ebpf_create_sync_chart(NETDATA_EBPF_FILE_SEGMENT_CHART,
                               "Monitor calls for <code>sync_file_range(2)</code>.", 21303,
                               NETDATA_SYNC_SYNC_FILE_RANGE_IDX, NETDATA_SYNC_SYNC_FILE_RANGE_IDX, update_every);
}

/**
 * Parse Syscalls
 *
 * Parse syscall options available inside ebpf.d/sync.conf
 */
static void ebpf_sync_parse_syscalls()
{
    int i;
    for (i = 0; local_syscalls[i].syscall; i++) {
        local_syscalls[i].enabled = appconfig_get_boolean(&sync_config, NETDATA_SYNC_CONFIG_NAME,
                                                          local_syscalls[i].syscall, CONFIG_BOOLEAN_YES);
    }
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
void *ebpf_sync_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_sync_exit, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = sync_maps;

    ebpf_sync_parse_syscalls();

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_sync_initialize_syscall(em)) {
        goto endsync;
    }

    int algorithms[NETDATA_SYNC_IDX_END] = { NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,
                                             NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,
                                             NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX };
    ebpf_global_labels(sync_counter_aggregated_data, sync_counter_publish_aggregated,
                       sync_counter_dimension_name, sync_counter_dimension_name,
                       algorithms, NETDATA_SYNC_IDX_END);

    pthread_mutex_lock(&lock);
    ebpf_create_sync_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    sync_collector(em);

endsync:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
