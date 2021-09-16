// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_vfs.h"

static char *vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_END] = { "delete",  "read",  "write",
                                                                  "fsync", "open", "create" };
static char *vfs_id_names[NETDATA_KEY_PUBLISH_VFS_END] = { "vfs_unlink", "vfs_read", "vfs_write",
                                                           "vfs_fsync", "vfs_open", "vfs_create"};

static netdata_idx_t *vfs_hash_values = NULL;
static netdata_syscall_stat_t vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_END];
static netdata_publish_syscall_t vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_END];
netdata_publish_vfs_t **vfs_pid = NULL;
netdata_publish_vfs_t *vfs_vector = NULL;

static ebpf_local_maps_t vfs_maps[] = {{.name = "tbl_vfs_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                        .user_input = 0, .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                       {.name = "tbl_vfs_stats", .internal_input = NETDATA_VFS_COUNTER,
                                        .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                       {.name = "vfs_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                        .user_input = 0,
                                        .type = NETDATA_EBPF_MAP_CONTROLLER,
                                        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                       {.name = NULL, .internal_input = 0, .user_input = 0}};

struct config vfs_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
    .rwlock = AVL_LOCK_INITIALIZER } };

static struct bpf_object *objects = NULL;
static struct bpf_link **probe_links = NULL;

struct netdata_static_thread vfs_threads = {"VFS KERNEL",
                                            NULL, NULL, 1, NULL,
                                            NULL,  NULL};

static int read_thread_closed = 1;

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
void clean_vfs_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(vfs_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
* Clean up the main thread.
*
* @param ptr thread data.
**/
static void ebpf_vfs_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 50 * USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    freez(vfs_hash_values);
    freez(vfs_vector);

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
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
*/
static void ebpf_vfs_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t pvc;

    pvc.write = (long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes;
    pvc.read = (long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes;

    write_count_chart(NETDATA_VFS_FILE_CLEAN_COUNT, NETDATA_FILESYSTEM_FAMILY,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK], 1);

    write_count_chart(NETDATA_VFS_FILE_IO_COUNT, NETDATA_FILESYSTEM_FAMILY,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ], 2);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_VFS_FILE_ERR_COUNT, NETDATA_FILESYSTEM_FAMILY,
                        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ], 2);
    }

    write_io_chart(NETDATA_VFS_IO_FILE_BYTES, NETDATA_FILESYSTEM_FAMILY, vfs_id_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
                   (long long)pvc.write, vfs_id_names[NETDATA_KEY_PUBLISH_VFS_READ], (long long)pvc.read);

    write_count_chart(NETDATA_VFS_FSYNC, NETDATA_FILESYSTEM_FAMILY,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_VFS_FSYNC_ERR, NETDATA_FILESYSTEM_FAMILY,
                        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC], 1);
    }

    write_count_chart(NETDATA_VFS_OPEN, NETDATA_FILESYSTEM_FAMILY,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_VFS_OPEN_ERR, NETDATA_FILESYSTEM_FAMILY,
                        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN], 1);
    }

    write_count_chart(NETDATA_VFS_CREATE, NETDATA_FILESYSTEM_FAMILY,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_VFS_CREATE_ERR,
            NETDATA_FILESYSTEM_FAMILY,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
            1);
    }
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void read_global_table()
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_VFS_COUNTER];

    netdata_idx_t *val = vfs_hash_values;
    int fd = vfs_maps[NETDATA_VFS_ALL].map_fd;
    for (idx = 0; idx < NETDATA_VFS_COUNTER; idx++) {
        uint64_t total = 0;
        if (!bpf_map_lookup_elem(fd, &idx, val)) {
            int i;
            int end = ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];
        }
        res[idx] = total;
    }

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].ncall = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].ncall = res[NETDATA_KEY_CALLS_VFS_READ] +
                                                             res[NETDATA_KEY_CALLS_VFS_READV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].ncall = res[NETDATA_KEY_CALLS_VFS_WRITE] +
                                                              res[NETDATA_KEY_CALLS_VFS_WRITEV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].ncall = res[NETDATA_KEY_CALLS_VFS_FSYNC];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].ncall = res[NETDATA_KEY_CALLS_VFS_OPEN];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].ncall = res[NETDATA_KEY_CALLS_VFS_CREATE];

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].nerr = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].nerr = res[NETDATA_KEY_ERROR_VFS_READ] +
                                                                res[NETDATA_KEY_ERROR_VFS_READV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].nerr = res[NETDATA_KEY_ERROR_VFS_WRITE] +
                                                                 res[NETDATA_KEY_ERROR_VFS_WRITEV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].nerr = res[NETDATA_KEY_ERROR_VFS_FSYNC];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].nerr = res[NETDATA_KEY_ERROR_VFS_OPEN];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].nerr = res[NETDATA_KEY_ERROR_VFS_CREATE];

    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] +
                                                               (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] +
                                                              (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param swap output structure
 * @param root link list with structure to be used
 */
static void ebpf_vfs_sum_pids(netdata_publish_vfs_t *vfs, struct pid_on_target *root)
{
    netdata_publish_vfs_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (root) {
        int32_t pid = root->pid;
        netdata_publish_vfs_t *w = vfs_pid[pid];
        if (w) {
            accumulator.write_call += w->write_call;
            accumulator.writev_call += w->writev_call;
            accumulator.read_call += w->read_call;
            accumulator.readv_call += w->readv_call;
            accumulator.unlink_call += w->unlink_call;
            accumulator.fsync_call += w->fsync_call;
            accumulator.open_call += w->open_call;
            accumulator.create_call += w->create_call;

            accumulator.write_bytes += w->write_bytes;
            accumulator.writev_bytes += w->writev_bytes;
            accumulator.read_bytes += w->read_bytes;
            accumulator.readv_bytes += w->readv_bytes;

            accumulator.write_err += w->write_err;
            accumulator.writev_err += w->writev_err;
            accumulator.read_err += w->read_err;
            accumulator.readv_err += w->readv_err;
            accumulator.unlink_err += w->unlink_err;
            accumulator.fsync_err += w->fsync_err;
            accumulator.open_err += w->open_err;
            accumulator.create_err += w->create_err;
        }
        root = root->next;
    }

    // These conditions were added, because we are using incremental algorithm
    vfs->write_call = (accumulator.write_call >= vfs->write_call) ? accumulator.write_call : vfs->write_call;
    vfs->writev_call = (accumulator.writev_call >= vfs->writev_call) ? accumulator.writev_call : vfs->writev_call;
    vfs->read_call = (accumulator.read_call >= vfs->read_call) ? accumulator.read_call : vfs->read_call;
    vfs->readv_call = (accumulator.readv_call >= vfs->readv_call) ? accumulator.readv_call : vfs->readv_call;
    vfs->unlink_call = (accumulator.unlink_call >= vfs->unlink_call) ? accumulator.unlink_call : vfs->unlink_call;
    vfs->fsync_call = (accumulator.fsync_call >= vfs->fsync_call) ? accumulator.fsync_call : vfs->fsync_call;
    vfs->open_call = (accumulator.open_call >= vfs->open_call) ? accumulator.open_call : vfs->open_call;
    vfs->create_call = (accumulator.create_call >= vfs->create_call) ? accumulator.create_call : vfs->create_call;

    vfs->write_bytes = (accumulator.write_bytes >= vfs->write_bytes) ? accumulator.write_bytes : vfs->write_bytes;
    vfs->writev_bytes = (accumulator.writev_bytes >= vfs->writev_bytes) ? accumulator.writev_bytes : vfs->writev_bytes;
    vfs->read_bytes = (accumulator.read_bytes >= vfs->read_bytes) ? accumulator.read_bytes : vfs->read_bytes;
    vfs->readv_bytes = (accumulator.readv_bytes >= vfs->readv_bytes) ? accumulator.readv_bytes : vfs->readv_bytes;

    vfs->write_err = (accumulator.write_err >= vfs->write_err) ? accumulator.write_err : vfs->write_err;
    vfs->writev_err = (accumulator.writev_err >= vfs->writev_err) ? accumulator.writev_err : vfs->writev_err;
    vfs->read_err = (accumulator.read_err >= vfs->read_err) ? accumulator.read_err : vfs->read_err;
    vfs->readv_err = (accumulator.readv_err >= vfs->readv_err) ? accumulator.readv_err : vfs->readv_err;
    vfs->unlink_err = (accumulator.unlink_err >= vfs->unlink_err) ? accumulator.unlink_err : vfs->unlink_err;
    vfs->fsync_err = (accumulator.fsync_err >= vfs->fsync_err) ? accumulator.fsync_err : vfs->fsync_err;
    vfs->open_err = (accumulator.open_err >= vfs->open_err) ? accumulator.open_err : vfs->open_err;
    vfs->create_err = (accumulator.create_err >= vfs->create_err) ? accumulator.create_err : vfs->create_err;
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
 */
void ebpf_vfs_send_apps_data(ebpf_module_t *em, struct target *root)
{
    struct target *w;
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_vfs_sum_pids(&w->vfs, w->root_pid);
        }
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_DELETED);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.unlink_call);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.write_call + w->vfs.writev_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->vfs.write_err + w->vfs.writev_err);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.read_call + w->vfs.readv_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->vfs.read_err + w->vfs.readv_err);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.write_bytes + w->vfs.writev_bytes);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_BYTES);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.read_bytes + w->vfs.readv_bytes);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_FSYNC);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.fsync_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->vfs.fsync_err);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_OPEN);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.open_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->vfs.open_err);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_CREATE);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, w->vfs.create_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                write_chart_dimension(w->name, w->vfs.create_err);
            }
        }
        write_end_chart();
    }
}

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 */
static void vfs_apps_accumulator(netdata_publish_vfs_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    netdata_publish_vfs_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_publish_vfs_t *w = &out[i];

        total->write_call += w->write_call;
        total->writev_call += w->writev_call;
        total->read_call += w->read_call;
        total->readv_call += w->readv_call;
        total->unlink_call += w->unlink_call;

        total->write_bytes += w->write_bytes;
        total->writev_bytes += w->writev_bytes;
        total->read_bytes += w->read_bytes;
        total->readv_bytes += w->readv_bytes;

        total->write_err += w->write_err;
        total->writev_err += w->writev_err;
        total->read_err += w->read_err;
        total->readv_err += w->readv_err;
        total->unlink_err += w->unlink_err;
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
static void vfs_fill_pid(uint32_t current_pid, netdata_publish_vfs_t *publish)
{
    netdata_publish_vfs_t *curr = vfs_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_publish_vfs_t));
        vfs_pid[current_pid] = curr;
    }

    memcpy(curr, &publish[0], sizeof(netdata_publish_vfs_t));
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void ebpf_vfs_read_apps()
{
    struct pid_stat *pids = root_of_pids;
    netdata_publish_vfs_t *vv = vfs_vector;
    int fd = vfs_maps[NETDATA_VFS_PID].map_fd;
    size_t length = sizeof(netdata_publish_vfs_t) * ebpf_nprocs;
    while (pids) {
        uint32_t key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, vv)) {
            pids = pids->next;
            continue;
        }

        vfs_apps_accumulator(vv);

        vfs_fill_pid(key, vv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(vv, 0, length);

        pids = pids->next;
    }
}

/**
 * VFS read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_vfs_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_LATENCY_VFS_SLEEP_MS * em->update_time;
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
 *
 * @param step the number of microseconds used with heart beat
 * @param em   the structure with thread information
 */
static void vfs_collector(ebpf_module_t *em)
{
    vfs_threads.thread = mallocz(sizeof(netdata_thread_t));
    vfs_threads.start_routine = ebpf_vfs_read_hash;

    netdata_thread_create(vfs_threads.thread, vfs_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_vfs_read_hash, em);

    int apps = em->apps_charts;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (apps)
            ebpf_vfs_read_apps();

        pthread_mutex_lock(&lock);

        ebpf_vfs_send_data(em);
        fflush(stdout);

        if (apps)
            ebpf_vfs_send_apps_data(em, apps_groups_root_target);

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create IO chart
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashboard
 * @param order  the order number of the specified chart
 * @param algorithm the algorithm used to make the charts.
 */
static void ebpf_create_io_chart(char *family, char *name, char *axis, char *web, int order, int algorithm)
{
    printf("CHART %s.%s '' 'Bytes written and read' '%s' '%s' '' line %d %d '' 'ebpf.plugin' 'filesystem'\n",
           family,
           name,
           axis,
           web,
           order,
           update_every);

    printf("DIMENSION %s %s %s 1 1\n",
           vfs_id_names[NETDATA_KEY_PUBLISH_VFS_READ],
           vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_READ],
           ebpf_algorithms[algorithm]);
    printf("DIMENSION %s %s %s -1 1\n",
           vfs_id_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
           vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
           ebpf_algorithms[algorithm]);
}

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_global_charts(ebpf_module_t *em)
{
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_VFS_FILE_CLEAN_COUNT,
                      "Remove files",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_FILESYSTEM_VFS_CLEAN,
                      ebpf_create_global_dimension,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK],
                      1, NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_VFS_FILE_IO_COUNT,
                      "Calls to IO",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_COUNT,
                      ebpf_create_global_dimension,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
                      2, NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_io_chart(NETDATA_FILESYSTEM_FAMILY,
                         NETDATA_VFS_IO_FILE_BYTES, EBPF_COMMON_DIMENSION_BYTES,
                         NETDATA_VFS_GROUP,
                         NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_BYTES,
                         NETDATA_EBPF_INCREMENTAL_IDX);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_VFS_FILE_ERR_COUNT,
                          "Fails to write or read",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_VFS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EBYTES,
                          ebpf_create_global_dimension,
                          &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
                          2, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_VFS_FSYNC,
                      "Calls to vfs_fsync",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_FSYNC,
                      ebpf_create_global_dimension,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
                      1, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_VFS_FSYNC_ERR,
                          "Fails to synchronize",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_VFS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EFSYNC,
                          ebpf_create_global_dimension,
                          &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
                          1, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_VFS_OPEN,
                      "Calls to vfs_open",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_OPEN,
                      ebpf_create_global_dimension,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
                      1, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_VFS_OPEN_ERR,
                          "Fails to open a file",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_VFS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EOPEN,
                          ebpf_create_global_dimension,
                          &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
                          1, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_VFS_CREATE,
                      "Calls to vfs_create",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_CREATE,
                      ebpf_create_global_dimension,
                      &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
                      1, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_VFS_CREATE_ERR,
                          "Fails to create a file.",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_VFS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_ECREATE,
                          ebpf_create_global_dimension,
                          &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
                          1, NETDATA_EBPF_MODULE_NAME_VFS);
    }
}

/**
 * Create process apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param ptr  a pointer for the targets.
 **/
void ebpf_vfs_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct target *root = ptr;

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_DELETED,
                               "Files deleted",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20065,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
                               "Write to disk",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20066,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               apps_groups_root_target, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
                                   "Fails to write",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_VFS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20067,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
                               "Read from disk",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20068,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
                                   "Fails to read",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_VFS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20069,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
                               "Bytes written on disk", EBPF_COMMON_DIMENSION_BYTES,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20070,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
                               "Bytes read from disk", EBPF_COMMON_DIMENSION_BYTES,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20071,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_FSYNC,
                               "Calls for <code>vfs_fsync</code>", EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20072,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR,
                                   "Sync error",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_VFS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20073,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_OPEN,
                               "Calls for <code>vfs_open</code>", EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20074,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR,
                                   "Open error",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_VFS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20075,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_VFS);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_CREATE,
                               "Calls for <code>vfs_create</code>", EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_VFS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20076,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR,
                                   "Create error",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_VFS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20077,
                                   ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                   root, NETDATA_EBPF_MODULE_NAME_VFS);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO START THREAD
 *
 *****************************************************************/

/**
 * Allocate vectors used with this thread.
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 *  @param length is the length for the vectors used inside the collector.
 */
static void ebpf_vfs_allocate_global_vectors()
{
    memset(vfs_aggregated_data, 0, sizeof(vfs_aggregated_data));
    memset(vfs_publish_aggregated, 0, sizeof(vfs_publish_aggregated));

    vfs_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
    vfs_vector = callocz(ebpf_nprocs, sizeof(netdata_publish_vfs_t));
    vfs_pid = callocz((size_t)pid_max, sizeof(netdata_publish_vfs_t *));
}

/*****************************************************************
 *
 *  EBPF VFS THREAD
 *
 *****************************************************************/

/**
 * Process thread
 *
 * Thread used to generate process charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_vfs_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_vfs_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = vfs_maps;

    ebpf_update_pid_table(&vfs_maps[NETDATA_VFS_PID], em);

    ebpf_vfs_allocate_global_vectors();

    if (!em->enabled)
        goto endvfs;

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects);
    if (!probe_links) {
        goto endvfs;
    }

    int algorithms[NETDATA_KEY_PUBLISH_VFS_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(vfs_aggregated_data, vfs_publish_aggregated, vfs_dimension_names,
                       vfs_id_names, algorithms, NETDATA_KEY_PUBLISH_VFS_END);

    pthread_mutex_lock(&lock);
    ebpf_create_global_charts(em);
    pthread_mutex_unlock(&lock);

    vfs_collector(em);

endvfs:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
