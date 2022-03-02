// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_mount.h"

static ebpf_local_maps_t mount_maps[] = {{.name = "tbl_mount", .internal_input = NETDATA_MOUNT_END,
                                          .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                          .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                         {.name = NULL, .internal_input = 0, .user_input = 0,
                                          .type = NETDATA_EBPF_MAP_CONTROLLER,
                                          .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

static char *mount_dimension_name[NETDATA_EBPF_MOUNT_SYSCALL] = { "mount", "umount" };
static netdata_syscall_stat_t mount_aggregated_data[NETDATA_EBPF_MOUNT_SYSCALL];
static netdata_publish_syscall_t mount_publish_aggregated[NETDATA_EBPF_MOUNT_SYSCALL];

struct config mount_config = { .first_section = NULL, .last_section = NULL, .mutex = NETDATA_MUTEX_INITIALIZER,
                               .index = {.avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                         .rwlock = AVL_LOCK_INITIALIZER } };

static int read_thread_closed = 1;
static netdata_idx_t *mount_values = NULL;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static netdata_idx_t mount_hash_values[NETDATA_MOUNT_END];

struct netdata_static_thread mount_thread = {"MOUNT KERNEL",
                                              NULL, NULL, 1, NULL,
                                              NULL,  NULL};

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_mount_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    freez(mount_thread.thread);
    freez(mount_values);

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
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Read global table
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    uint32_t idx;
    netdata_idx_t *val = mount_hash_values;
    netdata_idx_t *stored = mount_values;
    int fd = mount_maps[NETDATA_KEY_MOUNT_TABLE].map_fd;

    for (idx = NETDATA_KEY_MOUNT_CALL; idx < NETDATA_MOUNT_END; idx++) {
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
 * Mount read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_mount_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_LATENCY_MOUNT_SLEEP_MS * em->update_every;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }
    read_thread_closed = 1;

    return NULL;
}

/**
 * Send data to Netdata calling auxiliary functions.
*/
static void ebpf_mount_send_data()
{
    int i, j;
    int end = NETDATA_EBPF_MOUNT_SYSCALL;
    for (i = NETDATA_KEY_MOUNT_CALL, j = NETDATA_KEY_MOUNT_ERROR; i < end; i++, j++) {
        mount_publish_aggregated[i].ncall = mount_hash_values[i];
        mount_publish_aggregated[i].nerr = mount_hash_values[j];
    }

    write_count_chart(NETDATA_EBPF_MOUNT_CALLS, NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
                      mount_publish_aggregated, NETDATA_EBPF_MOUNT_SYSCALL);

    write_err_chart(NETDATA_EBPF_MOUNT_ERRORS, NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
                    mount_publish_aggregated, NETDATA_EBPF_MOUNT_SYSCALL);
}

/**
* Main loop for this collector.
*/
static void mount_collector(ebpf_module_t *em)
{
    mount_thread.thread = mallocz(sizeof(netdata_thread_t));
    mount_thread.start_routine = ebpf_mount_read_hash;
    memset(mount_hash_values, 0, sizeof(mount_hash_values));

    mount_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    netdata_thread_create(mount_thread.thread, mount_thread.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_mount_read_hash, em);

    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;
            pthread_mutex_lock(&lock);

            ebpf_mount_send_data();

            pthread_mutex_unlock(&lock);
        }

        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Create mount charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_mount_charts(int update_every)
{
    ebpf_create_chart(NETDATA_EBPF_MOUNT_GLOBAL_FAMILY, NETDATA_EBPF_MOUNT_CALLS,
                      "Calls to mount and umount syscalls.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_EBPF_MOUNT_FAMILY,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS,
                      ebpf_create_global_dimension,
                      mount_publish_aggregated, NETDATA_EBPF_MOUNT_SYSCALL,
                      update_every, NETDATA_EBPF_MODULE_NAME_MOUNT);

    ebpf_create_chart(NETDATA_EBPF_MOUNT_GLOBAL_FAMILY, NETDATA_EBPF_MOUNT_ERRORS,
                      "Errors to mount and umount syscalls.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_EBPF_MOUNT_FAMILY,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS + 1,
                      ebpf_create_global_dimension,
                      mount_publish_aggregated, NETDATA_EBPF_MOUNT_SYSCALL,
                      update_every, NETDATA_EBPF_MODULE_NAME_MOUNT);

    fflush(stdout);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_mount_load_bpf(ebpf_module_t *em)
{
    int ret = 0;
    if (em->load == EBPF_LOAD_LEGACY) {
        probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &objects);
        if (!probe_links) {
            em->enabled = CONFIG_BOOLEAN_NO;
            ret = -1;
        }
    }

    if (ret)
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);

    return ret;
}

/**
 * Mount thread
 *
 * Thread used to make mount thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always returns NULL
 */
void *ebpf_mount_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_mount_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = mount_maps;

    if (!em->enabled)
        goto endmount;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_mount_load_bpf(em)) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endmount;
    }

    int algorithms[NETDATA_EBPF_MOUNT_SYSCALL] = { NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX };

    ebpf_global_labels(mount_aggregated_data, mount_publish_aggregated, mount_dimension_name, mount_dimension_name,
                       algorithms, NETDATA_EBPF_MOUNT_SYSCALL);

    pthread_mutex_lock(&lock);
    ebpf_create_mount_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    mount_collector(em);

endmount:
    if (!em->enabled)
        ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
