// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_swap.h"

static char *swap_dimension_name[NETDATA_SWAP_END] = { "write", "read" };
static netdata_syscall_stat_t swap_aggregated_data[NETDATA_SWAP_END];
static netdata_publish_syscall_t swap_publish_aggregated[NETDATA_SWAP_END];

static int read_thread_closed = 1;
static int *map_fd = NULL;

static netdata_idx_t swap_hash_values[NETDATA_SWAP_END];

netdata_publish_swap_t **swap_pid = NULL;

static ebpf_data_t swap_data;
struct config swap_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

static ebpf_local_maps_t swap_maps[] = {{.name = "tbl_pid_swap", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                         .user_input = 0},
                                        {.name = NULL, .internal_input = 0, .user_input = 0}};

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

struct netdata_static_thread swap_threads = {"SWAP KERNEL", NULL, NULL, 1,
                                             NULL, NULL,  NULL};

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean swap strcuture
 */
void clean_swap_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(swap_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_swap_cleanup(void *ptr)
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

    ebpf_cleanup_publish_syscall(swap_publish_aggregated);

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
 *  COLLECTOR THREAD
 *
 *****************************************************************/

/**
* Send global
*
* Send global charts to Netdata
*/
static void swap_send_global()
{
    write_io_chart(NETDATA_MEM_SWAP_CHART, NETDATA_EBPF_SYSTEM_GROUP,
                   swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL].dimension,
                   (long long) swap_hash_values[NETDATA_KEY_SWAP_WRITEPAGE_CALL],
                   swap_publish_aggregated[NETDATA_KEY_SWAP_READPAGE_CALL].dimension,
                   (long long) swap_hash_values[NETDATA_KEY_SWAP_READPAGE_CALL]);
}

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    uint64_t stored;
    netdata_idx_t *val = swap_hash_values;
    int fd = map_fd[NETDATA_SWAP_GLOBAL_TABLE];

    uint32_t i, end = NETDATA_SWAP_END;
    for (i = NETDATA_KEY_SWAP_READPAGE_CALL; i < end; i++) {
        if (!bpf_map_lookup_elem(fd, &i, &stored)) {
            val[i] = stored;
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
void *ebpf_swap_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    usec_t step = NETDATA_SWAP_SLEEP_MS * em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }

    read_thread_closed = 1;
    return NULL;
}

/**
* Main loop for this collector.
*/
static void swap_collector(ebpf_module_t *em)
{
    swap_threads.thread = mallocz(sizeof(netdata_thread_t));
    swap_threads.start_routine = ebpf_swap_read_hash;

    map_fd = swap_data.map_fd;

    netdata_thread_create(swap_threads.thread, swap_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_swap_read_hash, em);

    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        pthread_mutex_lock(&lock);

        swap_send_global();

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
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_swap_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param length is the length for the vectors used inside the collector.
 */
static void ebpf_swap_allocate_global_vectors()
{
    dcstat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_dcstat_t *));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 */
static void ebpf_create_swap_charts()
{
    ebpf_create_chart(NETDATA_EBPF_SYSTEM_GROUP, NETDATA_MEM_SWAP_CHART,
                      "Calls for internal functions used to access swap.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_SWAP_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      202,
                      ebpf_create_global_dimension,
                      swap_publish_aggregated, NETDATA_SWAP_END);
}

/**
 * SWAP thread
 *
 * Thread used to make swap thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_swap_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_swap_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = swap_maps;
    fill_ebpf_data(&swap_data);

    ebpf_update_module(em, &swap_config, NETDATA_DIRECTORY_SWAP_CONFIG_FILE);
    ebpf_update_pid_table(&swap_maps[0], em);

    if (!em->enabled)
        goto endswap;

    if (ebpf_update_kernel(&swap_data)) {
        goto endswap;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, swap_data.map_fd);
    if (!probe_links) {
        goto endswap;
    }

    ebpf_swap_allocate_global_vectors();

    int algorithms[NETDATA_SWAP_END] = { NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX };
    ebpf_global_labels(swap_aggregated_data, swap_publish_aggregated, swap_dimension_name, swap_dimension_name,
                       algorithms, NETDATA_SWAP_END);

    pthread_mutex_lock(&lock);
    ebpf_create_swap_charts();
    pthread_mutex_unlock(&lock);

    swap_collector(em);

endswap:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
