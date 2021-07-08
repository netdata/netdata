// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_dcstat.h"

static char *dcstat_counter_dimension_name[NETDATA_DCSTAT_IDX_END] = { "ratio", "reference", "slow", "miss" };
static netdata_syscall_stat_t dcstat_counter_aggregated_data[NETDATA_DCSTAT_IDX_END];
static netdata_publish_syscall_t dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_END];

static ebpf_data_t dcstat_data;

netdata_dcstat_pid_t *dcstat_vector = NULL;
netdata_publish_dcstat_t **dcstat_pid = NULL;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static netdata_idx_t dcstat_hash_values[NETDATA_DCSTAT_IDX_END];
static netdata_idx_t *dcstat_values = NULL;

static int read_thread_closed = 1;

struct config dcstat_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

struct netdata_static_thread dcstat_threads = {"DCSTAT KERNEL",
                                               NULL, NULL, 1, NULL,
                                               NULL,  NULL};

static ebpf_local_maps_t dcstat_maps[] = {{.name = "dcstat_global", .internal_input = NETDATA_DIRECTORY_CACHE_END,
                                           .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                          {.name = "dcstat_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                           .user_input = 0,
                                           .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                          {.name = "dcstat_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                           .user_input = 0,
                                           .type = NETDATA_EBPF_MAP_CONTROLLER,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                          {.name = NULL, .internal_input = 0, .user_input = 0,
                                           .type = NETDATA_EBPF_MAP_CONTROLLER,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

static ebpf_specify_name_t dc_optional_name[] = { {.program_name = "netdata_lookup_fast",
                                                   .function_to_attach = "lookup_fast",
                                                   .optional = NULL,
                                                   .retprobe = CONFIG_BOOLEAN_NO},
                                                  {.program_name = NULL}};

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Update publish
 *
 * Update publish values before to write dimension.
 *
 * @param out           strcuture that will receive data.
 * @param cache_access  number of access to directory cache.
 * @param not_found     number of files not found on the file system
 */
void dcstat_update_publish(netdata_publish_dcstat_t *out, uint64_t cache_access, uint64_t not_found)
{
    calculated_number successful_access = (calculated_number) (((long long)cache_access) - ((long long)not_found));
    calculated_number ratio = (cache_access) ? successful_access/(calculated_number)cache_access : 0;

    out->ratio = (long long )(ratio*100);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean PID structures
 *
 * Clean the allocated structures.
 */
void clean_dcstat_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(dcstat_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 *  Clean names
 *
 *  Clean the optional names allocated during startup.
 */
void ebpf_dcstat_clean_names()
{
    size_t i = 0;
    while (dc_optional_name[i].program_name) {
        freez(dc_optional_name[i].optional);
        i++;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_dcstat_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 2 * USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    freez(dcstat_vector);
    freez(dcstat_values);

    ebpf_cleanup_publish_syscall(dcstat_counter_publish_aggregated);

    ebpf_dcstat_clean_names();

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
    }
}

/*****************************************************************
 *
 *  APPS
 *
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_DC_HIT_CHART,
                               "Percentage of files listed inside directory cache",
                               EBPF_COMMON_DIMENSION_PERCENTAGE,
                               NETDATA_APPS_DCSTAT_GROUP,
                               NETDATA_EBPF_CHART_TYPE_LINE,
                               20100,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root);

    ebpf_create_charts_on_apps(NETDATA_DC_REFERENCE_CHART,
                               "Count file access.",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_APPS_DCSTAT_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20101,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root);

    ebpf_create_charts_on_apps(NETDATA_DC_REQUEST_NOT_CACHE_CHART,
                               "Access to files that were not present inside directory cache.",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_APPS_DCSTAT_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20102,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root);

    ebpf_create_charts_on_apps(NETDATA_DC_REQUEST_NOT_FOUND_CHART,
                               "Number of requests for files that were not found on filesystem.",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_APPS_DCSTAT_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20103,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root);
}

/*****************************************************************
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 */
static void dcstat_apps_accumulator(netdata_dcstat_pid_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    netdata_dcstat_pid_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_dcstat_pid_t *w = &out[i];
        total->cache_access += w->cache_access;
        total->file_system += w->file_system;
        total->not_found += w->not_found;
    }
}

/**
 * Save PID values
 *
 * Save the current values inside the structure
 *
 * @param out     vector used to plot charts
 * @param publish vector with values read from hash tables.
 */
static inline void dcstat_save_pid_values(netdata_publish_dcstat_t *out, netdata_dcstat_pid_t *publish)
{
    memcpy(&out->curr, &publish[0], sizeof(netdata_dcstat_pid_t));
}

/**
 * Fill PID
 *
 * Fill PID structures
 *
 * @param current_pid pid that we are collecting data
 * @param out         values read from hash tables;
 */
static void dcstat_fill_pid(uint32_t current_pid, netdata_dcstat_pid_t *publish)
{
    netdata_publish_dcstat_t *curr = dcstat_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_publish_dcstat_t));
        dcstat_pid[current_pid] = curr;
    }

    dcstat_save_pid_values(curr, publish);
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 */
static void read_apps_table()
{
    netdata_dcstat_pid_t *cv = dcstat_vector;
    uint32_t key;
    struct pid_stat *pids = root_of_pids;
    int fd = dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_dcstat_pid_t)*ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, cv)) {
            pids = pids->next;
            continue;
        }

        dcstat_apps_accumulator(cv);

        dcstat_fill_pid(key, cv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);

        pids = pids->next;
    }
}

/**
 * Read global table
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    uint32_t idx;
    netdata_idx_t *val = dcstat_hash_values;
    netdata_idx_t *stored = dcstat_values;
    int fd = dcstat_maps[NETDATA_DCSTAT_GLOBAL_STATS].map_fd;

    for (idx = NETDATA_KEY_DC_REFERENCE; idx < NETDATA_DIRECTORY_CACHE_END; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, stored)) {
            int i;
            int end = ebpf_nprocs;
            netdata_idx_t total = 0;
            for (i = 0; i < end; i++)
                total += stored[i];

            val[idx] = total;
        }
    }
}

/**
 * DCstat read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_dcstat_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_LATENCY_DCSTAT_SLEEP_MS * em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }
    read_thread_closed = 1;

    return NULL;
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_dcstat_sum_pids(netdata_publish_dcstat_t *publish, struct pid_on_target *root)
{
    memset(&publish->curr, 0, sizeof(netdata_dcstat_pid_t));
    netdata_dcstat_pid_t *dst = &publish->curr;
    while (root) {
        int32_t pid = root->pid;
        netdata_publish_dcstat_t *w = dcstat_pid[pid];
        if (w) {
            netdata_dcstat_pid_t *src = &w->curr;
            dst->cache_access += src->cache_access;
            dst->file_system += src->file_system;
            dst->not_found += src->not_found;
        }

        root = root->next;
    }
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param root the target list.
*/
void ebpf_dcache_send_apps_data(struct target *root)
{
    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_HIT_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_dcstat_sum_pids(&w->dcstat, w->root_pid);

            uint64_t cache = w->dcstat.curr.cache_access;
            uint64_t not_found = w->dcstat.curr.not_found;

            dcstat_update_publish(&w->dcstat, cache, not_found);
            value = (collected_number) w->dcstat.ratio;
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REFERENCE_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.cache_access < w->dcstat.prev.cache_access) {
                w->dcstat.prev.cache_access = 0;
            }

            w->dcstat.cache_access = (long long)w->dcstat.curr.cache_access - (long long)w->dcstat.prev.cache_access;
            value = (collected_number) w->dcstat.cache_access;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.cache_access = w->dcstat.curr.cache_access;
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REQUEST_NOT_CACHE_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.file_system < w->dcstat.prev.file_system) {
                w->dcstat.prev.file_system = 0;
            }

            value = (collected_number) (!w->dcstat.cache_access) ? 0 :
                    (long long )w->dcstat.curr.file_system - (long long)w->dcstat.prev.file_system;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.file_system = w->dcstat.curr.file_system;
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REQUEST_NOT_FOUND_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.not_found < w->dcstat.prev.not_found) {
                w->dcstat.prev.not_found = 0;
            }
            value = (collected_number) (!w->dcstat.cache_access) ? 0 :
                    (long long)w->dcstat.curr.not_found - (long long)w->dcstat.prev.not_found;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.not_found = w->dcstat.curr.not_found;
        }
    }
    write_end_chart();
}

/**
 * Send global
 *
 * Send global charts to Netdata
 */
static void dcstat_send_global(netdata_publish_dcstat_t *publish)
{
    dcstat_update_publish(publish, dcstat_hash_values[NETDATA_KEY_DC_REFERENCE],
                          dcstat_hash_values[NETDATA_KEY_DC_MISS]);

    netdata_publish_syscall_t *ptr = dcstat_counter_publish_aggregated;
    netdata_idx_t value = dcstat_hash_values[NETDATA_KEY_DC_REFERENCE];
    if (value != ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall) {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = value - ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall;
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_SLOW];
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall =  value - ptr[NETDATA_DCSTAT_IDX_SLOW].pcall;
        ptr[NETDATA_DCSTAT_IDX_SLOW].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_MISS];
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = value - ptr[NETDATA_DCSTAT_IDX_MISS].pcall;
        ptr[NETDATA_DCSTAT_IDX_MISS].pcall = value;
    } else {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = 0;
    }

    ebpf_one_dimension_write_charts(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                                    ptr[NETDATA_DCSTAT_IDX_RATIO].dimension, publish->ratio);

    write_count_chart(
        NETDATA_DC_REFERENCE_CHART, NETDATA_FILESYSTEM_FAMILY,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 3);
}

/**
* Main loop for this collector.
*/
static void dcstat_collector(ebpf_module_t *em)
{
    dcstat_threads.thread = mallocz(sizeof(netdata_thread_t));
    dcstat_threads.start_routine = ebpf_dcstat_read_hash;

    netdata_thread_create(dcstat_threads.thread, dcstat_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_dcstat_read_hash, em);

    netdata_publish_dcstat_t publish;
    memset(&publish, 0, sizeof(publish));
    int apps = em->apps_charts;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (apps)
            read_apps_table();

        pthread_mutex_lock(&lock);

        dcstat_send_global(&publish);

        if (apps)
            ebpf_dcache_send_apps_data(apps_groups_root_target);

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Create filesystem charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 */
static void ebpf_create_filesystem_charts()
{
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                      "Percentage of files listed inside directory cache",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_DIRECTORY_FILESYSTEM_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21200,
                      ebpf_create_global_dimension,
                      dcstat_counter_publish_aggregated, 1);

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_REFERENCE_CHART,
                      "Variables used to calculate hit ratio.",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_FILESYSTEM_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21201,
                      ebpf_create_global_dimension,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 3);

    fflush(stdout);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param length is the length for the vectors used inside the collector.
 */
static void ebpf_dcstat_allocate_global_vectors(size_t length)
{
    dcstat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_dcstat_t *));
    dcstat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_dcstat_pid_t));
    dcstat_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(dcstat_counter_aggregated_data, 0, length*sizeof(netdata_syscall_stat_t));
    memset(dcstat_counter_publish_aggregated, 0, length*sizeof(netdata_publish_syscall_t));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Directory Cache thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always returns NULL
 */
void *ebpf_dcstat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_dcstat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = dcstat_maps;
    fill_ebpf_data(&dcstat_data);

    ebpf_update_pid_table(&dcstat_maps[NETDATA_DCSTAT_PID_STATS], em);

    ebpf_update_names(dc_optional_name, em);

    if (!em->enabled)
        goto enddcstat;

    ebpf_dcstat_allocate_global_vectors(NETDATA_DCSTAT_IDX_END);

    pthread_mutex_lock(&lock);

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, dcstat_data.map_fd);
    if (!probe_links) {
        pthread_mutex_unlock(&lock);
        goto enddcstat;
    }

    int algorithms[NETDATA_DCSTAT_IDX_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX,
        NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(dcstat_counter_aggregated_data, dcstat_counter_publish_aggregated,
                       dcstat_counter_dimension_name, dcstat_counter_dimension_name,
                       algorithms, NETDATA_DCSTAT_IDX_END);

    ebpf_create_filesystem_charts();
    pthread_mutex_unlock(&lock);

    dcstat_collector(em);

enddcstat:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
