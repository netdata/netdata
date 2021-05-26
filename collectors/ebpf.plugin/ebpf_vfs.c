// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_vfs.h"

static char *vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_END] = { "delete",  "read",  "write",
                                                                  "fsync", "open", "create" };
static char *vfs_id_names[NETDATA_KEY_PUBLISH_VFS_END] = { "vfs_unlink", "vfs_read", "vfs_write",
                                                           "vfs_fsync", "vfs_open", "vfs_create"};

static netdata_idx_t *vfs_hash_values = NULL;
static netdata_syscall_stat_t vfs_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_END];
static netdata_publish_syscall_t vfs_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_END];
netdata_publish_vfs_t **vfs_pid = NULL;
netdata_publish_vfs_t *vfs_vector = NULL;

static ebpf_data_t vfs_data;

static ebpf_local_maps_t vfs_maps[] = {{.name = "tbl_vfs_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                        .user_input = 0},
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

static int *map_fd = NULL;

static int read_thread_closed = 1;

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

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

    freez(vfs_data.map_fd);
    freez(vfs_hash_values);
    freez(vfs_vector);

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

    pvc.write = -((long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes);
    pvc.read = (long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes;

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].ncall =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_UNLINK].call;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].ncall =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].call;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].ncall = vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].call;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].ncall =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_FSYNC].call;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].ncall = vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_OPEN].call;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].ncall =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_CREATE].call;

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].nerr =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_UNLINK].ecall;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].nerr =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].ecall;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].nerr = vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].ecall;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].nerr =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_FSYNC].ecall;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].nerr = vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_OPEN].ecall;
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].nerr =
        vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_CREATE].ecall;

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
    int fd = map_fd[NETDATA_VFS_ALL];
    for (idx = 0; idx < NETDATA_VFS_COUNTER; idx++) {
        uint64_t total = 0;
        if (!bpf_map_lookup_elem(fd, &idx, val)) {
            int i;
            int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];
        }
        res[idx] = total;
    }

    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_UNLINK].call = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].call = res[NETDATA_KEY_CALLS_VFS_READ] + res[NETDATA_KEY_CALLS_VFS_READV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].call = res[NETDATA_KEY_CALLS_VFS_WRITE] + res[NETDATA_KEY_CALLS_VFS_WRITEV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_FSYNC].call = res[NETDATA_KEY_CALLS_VFS_FSYNC];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_OPEN].call = res[NETDATA_KEY_CALLS_VFS_OPEN];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_CREATE].call = res[NETDATA_KEY_CALLS_VFS_CREATE];

    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_UNLINK].ecall = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].ecall = res[NETDATA_KEY_ERROR_VFS_READ] + res[NETDATA_KEY_ERROR_VFS_READV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].ecall = res[NETDATA_KEY_ERROR_VFS_WRITE] + res[NETDATA_KEY_ERROR_VFS_WRITEV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_FSYNC].ecall = res[NETDATA_KEY_ERROR_VFS_FSYNC];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_OPEN].ecall = res[NETDATA_KEY_ERROR_VFS_OPEN];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_CREATE].ecall = res[NETDATA_KEY_ERROR_VFS_CREATE];

    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] +
                                                               (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] +
                                                              (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];


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

    map_fd = vfs_data.map_fd;

    netdata_thread_create(vfs_threads.thread, vfs_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_vfs_read_hash, em);

    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        pthread_mutex_lock(&lock);

        ebpf_vfs_send_data(em);
        fflush(stdout);

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
    printf("CHART %s.%s '' 'Bytes written and read' '%s' '%s' '' line %d %d\n",
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
    printf("DIMENSION %s %s %s 1 1\n",
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
                      1);

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
                      2);

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
                          2);
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
                      1);

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
                          1);
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
                      1);

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
                          1);
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
                      1);

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
                          1);
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
 *  EBPF PROCESS THREAD
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
    fill_ebpf_data(&vfs_data);

    ebpf_update_pid_table(&vfs_maps[0], em);

    ebpf_vfs_allocate_global_vectors();

    if (!em->enabled)
        goto endvfs;

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, vfs_data.map_fd);
    if (!probe_links) {
        goto endvfs;
    }

    int algorithms[NETDATA_KEY_PUBLISH_PROCESS_END] = {
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
