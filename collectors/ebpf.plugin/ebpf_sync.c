// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_sync.h"

static char *sync_counter_dimension_name[NETDATA_SYNC_IDX_END] = { "sync", "syncfs",  "msync", "fsync", "fdatasync",
                                                                   "sync_file_range" };
static netdata_syscall_stat_t sync_counter_aggregated_data[NETDATA_SYNC_IDX_END];
static netdata_publish_syscall_t sync_counter_publish_aggregated[NETDATA_SYNC_IDX_END];

static int read_thread_closed = 1;

static netdata_idx_t sync_hash_values[NETDATA_SYNC_IDX_END];

struct netdata_static_thread sync_threads = {"SYNC KERNEL", NULL, NULL, 1,
                                              NULL, NULL,  NULL};

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

ebpf_sync_syscalls_t local_syscalls[] = {
    {.syscall = "sync", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = "syncfs", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = "msync", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = "fsync", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = "fdatasync", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = "sync_file_range", .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL},
    {.syscall = NULL, .enabled = CONFIG_BOOLEAN_NO, .objects = NULL, .probe_links = NULL}
};

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

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
    for (i = 0; local_syscalls[i].syscall; i++) {
        ebpf_sync_syscalls_t *w = &local_syscalls[i];
        if (!w->probe_links && w->enabled) {
            em->thread_name = w->syscall;
            w->probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &w->objects);
            if (!w->probe_links) {
                em->thread_name = saved_name;
                return -1;
            }
        }
    }
    em->thread_name = saved_name;

    memset(sync_counter_aggregated_data, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_syscall_stat_t));
    memset(sync_counter_publish_aggregated, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_publish_syscall_t));
    memset(sync_hash_values, 0 , NETDATA_SYNC_IDX_END * sizeof(netdata_idx_t));

    return 0;
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
static void read_global_table()
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
 * Sync read hash
 *
 * This is the thread callback.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_sync_read_hash(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = NETDATA_EBPF_SYNC_SLEEP_MS * em->update_every;

    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }
    read_thread_closed = 1;

    return NULL;
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
    sync_threads.thread = mallocz(sizeof(netdata_thread_t));
    sync_threads.start_routine = ebpf_sync_read_hash;

    netdata_thread_create(sync_threads.thread, sync_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_sync_read_hash, em);

    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;
            pthread_mutex_lock(&lock);

            sync_send_data();

            pthread_mutex_unlock(&lock);
        }
        pthread_mutex_unlock(&collect_data_mutex);
    }
}


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
        if (w->probe_links) {
            struct bpf_program *prog;
            size_t j = 0 ;
            bpf_object__for_each_program(prog, w->objects) {
                bpf_link__destroy(w->probe_links[j]);
                j++;
            }
            bpf_object__close(w->objects);
        }
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_sync_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 2*USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    ebpf_sync_cleanup_objects();
    freez(sync_threads.thread);
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
    netdata_thread_cleanup_push(ebpf_sync_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = sync_maps;

    ebpf_sync_parse_syscalls();

    if (!em->enabled)
        goto endsync;

    if (ebpf_sync_initialize_syscall(em)) {
        pthread_mutex_unlock(&lock);
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
    pthread_mutex_unlock(&lock);

    sync_collector(em);

endsync:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
