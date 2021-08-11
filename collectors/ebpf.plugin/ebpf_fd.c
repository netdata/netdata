// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_fd.h"

static char *fd_dimension_names[NETDATA_FD_SYSCALL_END] = { "open", "close" };
static char *fd_id_names[NETDATA_FD_SYSCALL_END] = { "do_sys_open",  "__close_fd" };

static netdata_syscall_stat_t fd_aggregated_data[NETDATA_FD_SYSCALL_END];
static netdata_publish_syscall_t fd_publish_aggregated[NETDATA_FD_SYSCALL_END];

static ebpf_local_maps_t fd_maps[] = {{.name = "tbl_fd_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                       .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_RESIZABLE  | NETDATA_EBPF_MAP_PID,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                      {.name = "tbl_fd_global", .internal_input = NETDATA_KEY_END_VECTOR,
                                       .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                      {.name = "fd_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                       .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_CONTROLLER,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                      {.name = NULL, .internal_input = 0, .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_CONTROLLER,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};


struct config fd_config = { .first_section = NULL, .last_section = NULL, .mutex = NETDATA_MUTEX_INITIALIZER,
                           .index = {.avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                     .rwlock = AVL_LOCK_INITIALIZER } };

static ebpf_data_t fd_data;
static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

struct netdata_static_thread fd_thread = {"FD KERNEL", NULL, NULL, 1, NULL,
                                          NULL,  NULL};
static int read_thread_closed = 1;
static netdata_idx_t fd_hash_values[NETDATA_FD_COUNTER];
static netdata_idx_t *fd_values = NULL;

netdata_fd_stat_t *fd_vector = NULL;
netdata_fd_stat_t **fd_pid;

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
void clean_fd_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(fd_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_fd_cleanup(void *ptr)
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

    ebpf_cleanup_publish_syscall(fd_publish_aggregated);
    freez(fd_data.map_fd);
    freez(fd_thread.thread);
    freez(fd_values);
    freez(fd_vector);

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
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_fd_send_data(ebpf_module_t *em)
{
    fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].ncall = fd_hash_values[NETDATA_KEY_CALLS_DO_SYS_OPEN];
    fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].nerr = fd_hash_values[NETDATA_KEY_ERROR_DO_SYS_OPEN];

    fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].ncall = fd_hash_values[NETDATA_KEY_CALLS_CLOSE_FD];
    fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].nerr = fd_hash_values[NETDATA_KEY_ERROR_CLOSE_FD];

    write_count_chart(NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_FILESYSTEM_FAMILY, fd_publish_aggregated,
                      NETDATA_FD_SYSCALL_END);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_FILESYSTEM_FAMILY,
                        fd_publish_aggregated, NETDATA_FD_SYSCALL_END);
    }
}

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    uint32_t idx;
    netdata_idx_t *val = fd_hash_values;
    netdata_idx_t *stored = fd_values;
    int fd = fd_maps[NETDATA_FD_GLOBAL_STATS].map_fd;

    for (idx = NETDATA_KEY_CALLS_DO_SYS_OPEN; idx < NETDATA_FD_COUNTER; idx++) {
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
 * File descriptor read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_fd_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    usec_t step = NETDATA_FD_SLEEP_MS * em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }

    read_thread_closed = 1;
    return NULL;
}

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 */
static void fd_apps_accumulator(netdata_fd_stat_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    netdata_fd_stat_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_fd_stat_t *w = &out[i];
        total->open_call += w->open_call;
        total->close_call += w->close_call;
        total->open_err += w->open_err;
        total->close_err += w->close_err;
    }
}

/**
 * Fill PID
 *
 * Fill PID structures
 *
 * @param current_pid pid that we are collecting data
 * @param out         values read from hash tables;
 */
static void fd_fill_pid(uint32_t current_pid, netdata_fd_stat_t *publish)
{
    netdata_fd_stat_t *curr = fd_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_fd_stat_t));
        fd_pid[current_pid] = curr;
    }

    memcpy(curr, &publish[0], sizeof(netdata_fd_stat_t));
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 */
static void read_apps_table()
{
    netdata_fd_stat_t *fv = fd_vector;
    uint32_t key;
    struct pid_stat *pids = root_of_pids;
    int fd = fd_maps[NETDATA_FD_PID_STATS].map_fd;
    size_t length = sizeof(netdata_fd_stat_t) * ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, fv)) {
            pids = pids->next;
            continue;
        }

        fd_apps_accumulator(fv);

        fd_fill_pid(key, fv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(fv, 0, length);

        pids = pids->next;
    }
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param fd   the output
 * @param root list of pids
 */
static void ebpf_fd_sum_pids(netdata_fd_stat_t *fd, struct pid_on_target *root)
{
    uint32_t open_call = 0;
    uint32_t close_call = 0;
    uint32_t open_err = 0;
    uint32_t close_err = 0;

    while (root) {
        int32_t pid = root->pid;
        netdata_fd_stat_t *w = fd_pid[pid];
        if (w) {
            open_call += w->open_call;
            close_call += w->close_call;
            open_err += w->open_err;
            close_err += w->close_err;
        }

        root = root->next;
    }

    // These conditions were added, because we are using incremental algorithm
    fd->open_call = (open_call >= fd->open_call) ? open_call : fd->open_call;
    fd->close_call = (close_call >= fd->close_call) ? close_call : fd->close_call;
    fd->open_err = (open_err >= fd->open_err) ? open_err : fd->open_err;
    fd->close_err = (close_err >= fd->close_err) ? close_err : fd->close_err;
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
*/
void ebpf_fd_send_apps_data(ebpf_module_t *em, struct target *root)
{
    struct target *w;
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_fd_sum_pids(&w->fd, w->root_pid);
        }
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->fd.open_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->fd.open_err);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSED);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->fd.close_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->fd.close_err);
            }
        }
        write_end_chart();
    }
}

/**
* Main loop for this collector.
*/
static void fd_collector(ebpf_module_t *em)
{
    fd_thread.thread = mallocz(sizeof(netdata_thread_t));
    fd_thread.start_routine = ebpf_fd_read_hash;

    netdata_thread_create(fd_thread.thread, fd_thread.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_fd_read_hash, em);

    int apps = em->apps_charts;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (apps)
            read_apps_table();

        pthread_mutex_lock(&lock);

        ebpf_fd_send_data(em);

        if (apps)
            ebpf_fd_send_apps_data(em, apps_groups_root_target);

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *
 *  CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_fd_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN,
                               "Number of open files",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_FILE_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20061,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_PROCESS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
                                   "Fails to open files",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_FILE_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20062,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSED,
                               "Files closed",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_FILE_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20063,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_PROCESS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
                                   "Fails to close files",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_FILE_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20064,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }
}

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_fd_global_charts(ebpf_module_t *em)
{
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_FILE_OPEN_CLOSE_COUNT,
                      "Open and close calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_FILE_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_EBPF_FD_CHARTS,
                      ebpf_create_global_dimension,
                      fd_publish_aggregated,
                      NETDATA_FD_SYSCALL_END,
                      NETDATA_EBPF_MODULE_NAME_FD);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_FILE_OPEN_ERR_COUNT,
                          "Open fails",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_FILE_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_EBPF_FD_CHARTS + 1,
                          ebpf_create_global_dimension,
                          fd_publish_aggregated,
                          NETDATA_FD_SYSCALL_END,
                          NETDATA_EBPF_MODULE_NAME_FD);
    }
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_fd_allocate_global_vectors()
{
    fd_pid = callocz((size_t)pid_max, sizeof(netdata_fd_stat_t *));
    fd_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_fd_stat_t));

    fd_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));
}

/**
 * Directory Cache thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always returns NULL
 */
void *ebpf_fd_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_fd_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = fd_maps;
    fill_ebpf_data(&fd_data);

    if (!em->enabled)
        goto endfd;

    if (ebpf_update_kernel(&fd_data))
        goto endfd;

    ebpf_fd_allocate_global_vectors();

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, fd_data.map_fd);
    if (!probe_links) {
        goto endfd;
    }

    int algorithms[NETDATA_FD_SYSCALL_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(fd_aggregated_data, fd_publish_aggregated, fd_dimension_names, fd_id_names,
                       algorithms, NETDATA_FD_SYSCALL_END);

    pthread_mutex_lock(&lock);
    ebpf_create_fd_global_charts(em);
    pthread_mutex_unlock(&lock);

    fd_collector(em);

endfd:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
