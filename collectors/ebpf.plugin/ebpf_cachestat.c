// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_cachestat.h"

static ebpf_data_t cachestat_data;
netdata_publish_cachestat_t **cachestat_pid;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static char *cachestat_counter_dimension_name[NETDATA_CACHESTAT_END] = { "ratio", "dirty", "hit",
                                                                         "miss" };
static netdata_syscall_stat_t *cachestat_counter_aggregated_data = NULL;
static netdata_publish_syscall_t *cachestat_counter_publish_aggregated = NULL;

netdata_cachestat_pid_t *cachestat_vector = NULL;

static netdata_idx_t *cachestat_hash_values = NULL;

static int read_thread_closed = 1;

struct netdata_static_thread cachestat_threads = {"CACHESTAT KERNEL",
                                                  NULL, NULL, 1, NULL,
                                                  NULL,  NULL};

static int *map_fd = NULL;

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
static void clean_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(cachestat_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_cachestat_cleanup(void *ptr)
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

    clean_pid_structures();
    freez(cachestat_pid);

    freez(cachestat_counter_aggregated_data);
    ebpf_cleanup_publish_syscall(cachestat_counter_publish_aggregated);
    freez(cachestat_counter_publish_aggregated);

    freez(cachestat_vector);
    freez(cachestat_hash_values);

    struct bpf_program *prog;
    size_t i = 0 ;
    bpf_object__for_each_program(prog, objects) {
        bpf_link__destroy(probe_links[i]);
        i++;
    }
    bpf_object__close(objects);
}

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Write charts
 *
 * Write the current information to publish the charts.
 *
 * @param family chart family
 * @param chart  chart id
 * @param dim    dimension name
 * @param v1     value.
 */
static inline void cachestat_write_charts(char *family, char *chart, char *dim, long long v1)
{
    write_begin_chart(family, chart);

    write_chart_dimension(dim, v1);

    write_end_chart();
}

/**
 * Update publish
 *
 * Update publish values before to write dimension.
 *
 * @param out  strcuture that will receive data.
 * @param mpa  calls for mark_page_accessed during the last second.
 * @param mbd  calls for mark_buffer_dirty during the last second.
 * @param apcl calls for add_to_page_cache_lru during the last second.
 * @param apd  calls for account_page_dirtied during the last second.
 */
void cachestat_update_publish(netdata_publish_cachestat_t *out, uint64_t mpa, uint64_t mbd,
                              uint64_t apcl, uint64_t apd)
{
    // Adapted algorithm from https://github.com/iovisor/bcc/blob/master/tools/cachestat.py#L126-L138
    calculated_number total = (calculated_number) (((long long)mpa) - ((long long)mbd));
    if (total < 0)
        total = 0;

    calculated_number misses = (calculated_number) (((long long)apcl) -((long long)apd));
    if (misses < 0)
        misses = 0;

    // If hits are < 0, then its possible misses are overestimate due to possibly page cache read ahead adding
    // more pages than needed. In this case just assume misses as total and reset hits.
    calculated_number hits = total - misses;
    if (hits < 0 ) {
        misses = total;
        hits = 0;
    }

    calculated_number ratio = (total > 0)?hits/total:0;

    out->ratio = (long long )(ratio*100);
    out->hit = (long long)hits;
    out->miss = (long long)misses;
}

/**
 * Save previous values
 *
 * Save values used this time.
 *
 * @param publish
 */
static void save_previous_values(netdata_publish_cachestat_t *publish) {
    publish->prev.mark_page_accessed = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED];
    publish->prev.account_page_dirtied = cachestat_hash_values[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED];
    publish->prev.add_to_page_cache_lru = cachestat_hash_values[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU];
    publish->prev.mark_buffer_dirty = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY];
}

/**
 * Calculate statistics
 *
 * @param publish the structure where we will store the data.
 */
static void calculate_stats(netdata_publish_cachestat_t *publish) {
    if (!publish->prev.mark_page_accessed) {
        save_previous_values(publish);
        return;
    }

    uint64_t mpa = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED] - publish->prev.mark_page_accessed;
    uint64_t mbd = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY] - publish->prev.mark_buffer_dirty;
    uint64_t apcl = cachestat_hash_values[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU] - publish->prev.add_to_page_cache_lru;
    uint64_t apd = cachestat_hash_values[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED] - publish->prev.account_page_dirtied;

    save_previous_values(publish);

    // We are changing the original algorithm to have a smooth ratio.
    cachestat_update_publish(publish, mpa, mbd, apcl, apd);
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
void ebpf_cachestat_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
}

/*****************************************************************
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    uint32_t idx;
    netdata_idx_t *val = cachestat_hash_values;
    netdata_idx_t stored;
    int fd = map_fd[NETDATA_CACHESTAT_GLOBAL_STATS];

    for (idx = NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU; idx < NETDATA_CACHESTAT_END; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, &stored)) {
            val[idx] = stored;
        }
    }
}

/**
 * Socket read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data on very busy socket.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_cachestat_read_hash(void *ptr)
{
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = NETDATA_LATENCY_CACHESTAT_SLEEP_MS;

    read_thread_closed = 0;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }
    read_thread_closed = 1;

    return NULL;
}

/**
 * Send global
 *
 * Send global charts to Netdata
 */
static void cachestat_send_global(netdata_publish_cachestat_t *publish)
{
    calculate_stats(publish);

    netdata_publish_syscall_t *ptr = cachestat_counter_publish_aggregated;
    // The algorithm sets this value to zero sometimes, we are not written them to have a sooth chart
    if (publish->ratio) {
        cachestat_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_RATIO_CHART,
                               ptr[NETDATA_CACHESTAT_IDX_RATIO].dimension, publish->ratio);
    }

    cachestat_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_DIRTY_CHART,
                           ptr[NETDATA_CACHESTAT_IDX_DIRTY].dimension,
                           cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY]);

    cachestat_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_CHART,
                           ptr[NETDATA_CACHESTAT_IDX_HIT].dimension, publish->hit);

    cachestat_write_charts(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_MISSES_CHART,
                           ptr[NETDATA_CACHESTAT_IDX_MISS].dimension, publish->miss);
}


/**
* Main loop for this collector.
*/
static void cachestat_collector(ebpf_module_t *em)
{
    cachestat_threads.thread = mallocz(sizeof(netdata_thread_t));
    cachestat_threads.start_routine = ebpf_cachestat_read_hash;

    map_fd = cachestat_data.map_fd;

    netdata_thread_create(cachestat_threads.thread, cachestat_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_cachestat_read_hash, em);

    netdata_publish_cachestat_t publish;
    memset(&publish, 0, sizeof(publish));
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        pthread_mutex_lock(&lock);

        cachestat_send_global(&publish);

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
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 */
static void ebpf_create_global_charts()
{
    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_RATIO_CHART,
                      "Total cache added without dirties per total added because of red misses.",
                      EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_SUBMENU,
                      "mem.pagecache",
                      21100,
                      ebpf_create_global_dimension,
                      cachestat_counter_publish_aggregated, 1);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_DIRTY_CHART,
                      "Number of dirty pages added to the page cache.",
                      EBPF_CACHESTAT_DIMENSION_PAGE, NETDATA_CACHESTAT_SUBMENU,
                      "mem.pagecache",
                      21101,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY], 1);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_CHART,
                      "Hits are function calls that Netdata counts.",
                      EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_SUBMENU,
                      "mem.pagecache",
                      21102,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT], 1);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_MISSES_CHART,
                      "Misses are function calls that Netdata counts.",
                      EBPF_CACHESTAT_DIMENSION_MISSES, NETDATA_CACHESTAT_SUBMENU,
                      "mem.pagecache",
                      21103,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS], 1);

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
static void ebpf_cachestat_allocate_global_vectors(size_t length)
{
    cachestat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_cachestat_t *));
    cachestat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_cachestat_pid_t));

    cachestat_hash_values = callocz(length, sizeof(netdata_idx_t));

    cachestat_counter_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    cachestat_counter_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make cachestat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_cachestat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_cachestat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&cachestat_data);

    if (!em->enabled)
        goto endcachestat;

    pthread_mutex_lock(&lock);
    ebpf_cachestat_allocate_global_vectors(NETDATA_CACHESTAT_END);
    if (ebpf_update_kernel(&cachestat_data)) {
        pthread_mutex_unlock(&lock);
        goto endcachestat;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, cachestat_data.map_fd);
    if (!probe_links) {
        pthread_mutex_unlock(&lock);
        goto endcachestat;
    }

    int algorithms[NETDATA_CACHESTAT_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(cachestat_counter_aggregated_data, cachestat_counter_publish_aggregated,
                       cachestat_counter_dimension_name, cachestat_counter_dimension_name,
                       algorithms, NETDATA_CACHESTAT_END);

    ebpf_create_global_charts();

    pthread_mutex_unlock(&lock);

    cachestat_collector(em);

endcachestat:
    netdata_thread_cleanup_pop(1);
    return NULL;
}