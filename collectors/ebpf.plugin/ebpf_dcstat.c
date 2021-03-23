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

static int *map_fd = NULL;
static netdata_idx_t dcstat_hash_values[NETDATA_DCSTAT_IDX_END];

static int read_thread_closed = 1;

struct config dcstat_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

struct netdata_static_thread dcstat_threads = {"DCSTAT KERNEL",
                                               NULL, NULL, 1, NULL,
                                               NULL,  NULL};

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
static inline void dcstat_write_charts(char *family, char *chart, char *dim, long long v1)
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
 * @param out   strcuture that will receive data.
 * @param hit   calls to search a specific file
 * @param miss  calls that did not find files
 */
void dcstat_update_publish(netdata_publish_dcstat_t *out, uint64_t hit, uint64_t miss)
{
    calculated_number found = (calculated_number) (((long long)hit) - ((long long)miss));
    calculated_number ratio = (hit != 0) ? found/(calculated_number)hit : 0;

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
static void clean_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(dcstat_pid[pids->pid]);

        pids = pids->next;
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

    clean_pid_structures();
    freez(dcstat_pid);
    freez(dcstat_vector);

    ebpf_cleanup_publish_syscall(dcstat_counter_publish_aggregated);

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
    UNUSED(ptr);
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
    netdata_idx_t *val = dcstat_hash_values;
    netdata_idx_t stored;
    int fd = map_fd[NETDATA_DCSTAT_GLOBAL_STATS];

    for (idx = NETDATA_KEY_DC_REFERENCE; idx < NETDATA_DIRECTORY_CACHE_END; idx++) {
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
 * Send global
 *
 * Send global charts to Netdata
 */
static void dcstat_send_global(netdata_publish_dcstat_t *publish)
{
    dcstat_update_publish(publish, dcstat_hash_values[NETDATA_DCSTAT_IDX_REFERENCE],
                          dcstat_hash_values[NETDATA_DCSTAT_IDX_MISS]);

    dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE].ncall = dcstat_hash_values[NETDATA_DCSTAT_IDX_REFERENCE];
    dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_SLOW].ncall = dcstat_hash_values[NETDATA_DCSTAT_IDX_SLOW];
    dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_MISS].ncall = dcstat_hash_values[NETDATA_DCSTAT_IDX_MISS];

    netdata_publish_syscall_t *ptr = dcstat_counter_publish_aggregated;
    dcstat_write_charts(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                        ptr[NETDATA_DCSTAT_IDX_RATIO].dimension, publish->ratio);

    write_count_chart(NETDATA_DC_REQUEST_CHART, NETDATA_FILESYSTEM_FAMILY,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 3);
}

/**
* Main loop for this collector.
*/
static void dcstat_collector(ebpf_module_t *em)
{
    dcstat_threads.thread = mallocz(sizeof(netdata_thread_t));
    dcstat_threads.start_routine = ebpf_dcstat_read_hash;

    netdata_publish_dcstat_t publish;
    memset(&publish, 0, sizeof(publish));
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        pthread_mutex_lock(&lock);

        dcstat_send_global(&publish);

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
static void ebpf_create_memory_charts()
{
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                      "Percentage of files listed inside directory cache",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21200,
                      ebpf_create_global_dimension,
                      dcstat_counter_publish_aggregated, 1);

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_REQUEST_CHART,
                      "Variables used to calculate hit ratio.",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
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

    memset(dcstat_counter_aggregated_data, 0, length*sizeof(netdata_syscall_stat_t));
    memset(dcstat_counter_publish_aggregated, 0, length*sizeof(netdata_publish_syscall_t));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_dcstat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_dcstat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&dcstat_data);

    ebpf_update_module(em, &dcstat_config, NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE);

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
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(dcstat_counter_aggregated_data, dcstat_counter_publish_aggregated,
                       dcstat_counter_dimension_name, dcstat_counter_dimension_name,
                       algorithms, NETDATA_DCSTAT_IDX_END);

    ebpf_create_memory_charts();
    pthread_mutex_unlock(&lock);

    dcstat_collector(em);

enddcstat:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
