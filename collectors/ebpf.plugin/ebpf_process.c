// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_process.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *dimension_names[NETDATA_MAX_MONITOR_VECTOR] = { "open", "close", "delete", "read", "write",
                                                             "process", "task", "process", "thread" };
static char *id_names[NETDATA_MAX_MONITOR_VECTOR] = { "do_sys_open", "__close_fd", "vfs_unlink", "vfs_read", "vfs_write",
                                                      "do_exit", "release_task", "_do_fork", "sys_clone" };
static char *status[] = { "process", "zombie" };

static netdata_idx_t *hash_values = NULL;
static netdata_syscall_stat_t *aggregated_data = NULL;
static netdata_publish_syscall_t *publish_aggregated = NULL;

ebpf_functions_t functions;

/**
 * Pointers used when collector is dynamically linked
 */

//Libbpf (It is necessary to have at least kernel 4.10)
int (*bpf_map_lookup_elem)(int, const void *, void *);

int *map_fd = NULL;
/**
 * End of the pointers
 */

/*****************************************************************
 *
 *  PROCESS DATA AND SEND TO NETDATA
 *
 *****************************************************************/

/**
 * Update publish structure before to send data to Netdata.
 *
 * @param publish  the first output structure with independent dimensions
 * @param pvc      the second output structure with correlated dimensions
 * @param input    the structure with the input data.
 */
static void ebpf_update_publish(netdata_publish_syscall_t *publish,
                                   netdata_publish_vfs_common_t *pvc,
                                   netdata_syscall_stat_t *input) {

    netdata_publish_syscall_t *move = publish;
    while(move) {
        if(input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if(move->pcall) {
                move->ncall = (input->call > move->pcall)?input->call - move->pcall: move->pcall - input->call;
                move->nbyte = (input->bytes > move->pbyte)?input->bytes - move->pbyte: move->pbyte - input->bytes;
                move->nerr = (input->ecall > move->nerr)?input->ecall - move->perr: move->perr - input->ecall;
            } else {
                move->ncall = 0;
                move->nbyte = 0;
                move->nerr = 0;
            }

            move->pcall = input->call;
            move->pbyte = input->bytes;
            move->perr = input->ecall;
        } else {
            move->ncall = 0;
            move->nbyte = 0;
            move->nerr = 0;
        }

        input = input->next;
        move = move->next;
    }

    pvc->write = -((long)publish[2].nbyte);
    pvc->read = (long)publish[3].nbyte;

    pvc->running = (long)publish[7].ncall - (long)publish[8].ncall;
    publish[6].ncall = -publish[6].ncall; // release
    pvc->zombie = (long)publish[5].ncall + (long)publish[6].ncall;
}

/**
 * Write begin command on standard output
 *
 * @param family the chart family name
 * @param name   the chart name
 */
static inline void write_begin_chart(char *family, char *name)
{
    int ret = printf( "BEGIN %s.%s\n"
        , family
        , name);

    (void)ret;
}

/**
 * Write set command on standard output
 *
 * @param dim    the dimension name
 * @param value  the value for the dimension
 */
static inline void write_chart_dimension(char *dim, long long value)
{
    int ret = printf("SET %s = %lld\n", dim, value);
    (void)ret;
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param name    the chart name
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 * @param end     the number of values that will be written on standard output
 */
static void write_global_count_chart(char *name, char *family, netdata_publish_syscall_t *move, int end) {
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    printf("END\n");
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param name    the chart name
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 * @param end     the number of values that will be written on standard output
 */
static void write_global_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end) {
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
    }

    printf("END\n");
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 */
static void write_io_chart(char *family, netdata_publish_vfs_common_t *pvc) {
    write_begin_chart(family, NETDATA_VFS_IO_FILE_BYTES);

    write_chart_dimension(id_names[3], (long long) pvc->write);
    write_chart_dimension(id_names[4], (long long) pvc->read);

    printf("END\n");
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 */
static void write_status_chart(char *family, netdata_publish_vfs_common_t *pvc) {
    write_begin_chart(family, NETDATA_PROCESS_STATUS_NAME);

    write_chart_dimension(status[0], (long long) pvc->running);
    write_chart_dimension(status[1], (long long) pvc->zombie);

    printf("END\n");
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em) {
    netdata_publish_vfs_common_t pvc;
    ebpf_update_publish(publish_aggregated, &pvc, aggregated_data);

    write_global_count_chart(NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_EBPF_FAMILY, publish_aggregated, 2);
    write_global_count_chart(NETDATA_VFS_FILE_CLEAN_COUNT,
                             NETDATA_EBPF_FAMILY,
                             &publish_aggregated[NETDATA_DEL_START],
                             1);
    write_global_count_chart(NETDATA_VFS_FILE_IO_COUNT,
                             NETDATA_EBPF_FAMILY,
                             &publish_aggregated[NETDATA_IN_START_BYTE],
                             2);
    write_global_count_chart(NETDATA_EXIT_SYSCALL,
                             NETDATA_EBPF_FAMILY,
                             &publish_aggregated[NETDATA_EXIT_START],
                             2);
    write_global_count_chart(NETDATA_PROCESS_SYSCALL,
                             NETDATA_EBPF_FAMILY,
                             &publish_aggregated[NETDATA_PROCESS_START],
                             2);

    write_status_chart(NETDATA_EBPF_FAMILY, &pvc);
    if(em->mode < MODE_ENTRY) {
        write_global_err_chart(NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_EBPF_FAMILY, publish_aggregated, 2);
        write_global_err_chart(NETDATA_VFS_FILE_ERR_COUNT,
                               NETDATA_EBPF_FAMILY,
                               &publish_aggregated[2],
                               NETDATA_VFS_ERRORS);
        write_global_err_chart(NETDATA_PROCESS_ERROR_NAME,
                               NETDATA_EBPF_FAMILY,
                               &publish_aggregated[NETDATA_PROCESS_START],
                               2);

        write_io_chart(NETDATA_EBPF_FAMILY, &pvc);
    }
}

/*****************************************************************
 *
 *  READ INFORMATION FROM KERNEL RING
 *
 *****************************************************************/

/**
 * Read the hash table and store data to allocated vectors.
 */
static void read_hash_global_tables()
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_GLOBAL_VECTOR];

    netdata_idx_t *val = hash_values;
    for (idx = 0; idx < NETDATA_GLOBAL_VECTOR; idx++) {
        if(!bpf_map_lookup_elem(map_fd[1], &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = (running_on_kernel < NETDATA_KERNEL_V4_15)?1:ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
        } else {
            res[idx] = 0;
        }
    }

    aggregated_data[0].call = res[NETDATA_KEY_CALLS_DO_SYS_OPEN];
    aggregated_data[1].call = res[NETDATA_KEY_CALLS_CLOSE_FD];
    aggregated_data[2].call = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    aggregated_data[3].call = res[NETDATA_KEY_CALLS_VFS_READ] + res[NETDATA_KEY_CALLS_VFS_READV];
    aggregated_data[4].call = res[NETDATA_KEY_CALLS_VFS_WRITE] + res[NETDATA_KEY_CALLS_VFS_WRITEV];
    aggregated_data[5].call = res[NETDATA_KEY_CALLS_DO_EXIT];
    aggregated_data[6].call = res[NETDATA_KEY_CALLS_RELEASE_TASK];
    aggregated_data[7].call = res[NETDATA_KEY_CALLS_DO_FORK];
    aggregated_data[8].call = res[NETDATA_KEY_CALLS_SYS_CLONE];

    aggregated_data[0].ecall = res[NETDATA_KEY_ERROR_DO_SYS_OPEN];
    aggregated_data[1].ecall = res[NETDATA_KEY_ERROR_CLOSE_FD];
    aggregated_data[2].ecall = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    aggregated_data[3].ecall = res[NETDATA_KEY_ERROR_VFS_READ] + res[NETDATA_KEY_ERROR_VFS_READV];
    aggregated_data[4].ecall = res[NETDATA_KEY_ERROR_VFS_WRITE] + res[NETDATA_KEY_ERROR_VFS_WRITEV];
    aggregated_data[7].ecall = res[NETDATA_KEY_ERROR_DO_FORK];
    aggregated_data[8].ecall = res[NETDATA_KEY_ERROR_SYS_CLONE];

    aggregated_data[2].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    aggregated_data[3].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];
}

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/


/**
 * Main loop for this collector.
 *
 * @param step the number of microseconds used with heart beat
 * @param em   the structure with thread information
 */
static void process_collector(usec_t step, ebpf_module_t *em)
{
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();
        ebpf_process_send_data(em);

        fflush(stdout);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Write chart cmd on standard output
 *
 * @param type  the chart type
 * @param id    the chart id
 * @param axis  the axis label
 * @param web   the group name used to attach the chart on dashaboard
 * @param order the chart order
 */
static inline void ebpf_write_chart_cmd(char *type
    , char *id
    , char *axis
    , char *web
    , int order)
{
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
        , type
        , id
        , axis
        , web
        , order);
}

/**
 * Write the dimension command on standard output
 *
 * @param n the dimension name
 * @param d the dimension information
 */
static inline void ebpf_write_global_dimension(char *n, char *d)
{
    printf("DIMENSION %s %s absolute 1 1\n", n, d);
}

/**
 * Call ebpf_write_global_dimension to create the dimensions for a specific chart
 *
 * @param ptr a pointer to a structure of the type netdata_publish_syscall_t
 * @param end the number of dimensions for the structure ptr
 */
static void ebpf_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        ebpf_write_global_dimension(move->name, move->dimension);

        move = move->next;
        i++;
    }
}

/**
 *  Call write_chart_cmd to create the charts
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashaboard
 * @param order  the order number of the specified chart
 * @param ncd    a pointer to a function called to create dimensions
 * @param move   a pointer for a structure that has the dimensions
 * @param end    number of dimensions for the chart created
 */
static inline void ebpf_create_chart(char *family
    , char *name
    , char *axis
    , char *web
    , int order
    , void (*ncd)(void *, int)
    , void *move
    , int end)
{
    ebpf_write_chart_cmd(family, name, axis, web, order);

    ncd(move, end);
}

/**
 * Create IO chart
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashaboard
 * @param order  the order number of the specified chart
 */
static void ebpf_create_io_chart(char *family, char *name, char *axis, char *web, int order) {
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
        , family
        , name
        , axis
        , web
        , order);

    printf("DIMENSION %s %s absolute 1 1\n", id_names[3], NETDATA_VFS_DIM_OUT_FILE_BYTES);
    printf("DIMENSION %s %s absolute 1 1\n", id_names[4], NETDATA_VFS_DIM_IN_FILE_BYTES);
}

/**
 * Create process status chart
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashaboard
 * @param order  the order number of the specified chart
 */
static void ebpf_process_status_chart(char *family, char *name, char *axis, char *web, int order) {
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
        , family
        , name
        , axis
        , web
        , order);

    printf("DIMENSION %s '' absolute 1 1\n", status[0]);
    printf("DIMENSION %s '' absolute 1 1\n", status[1]);
}

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_global_charts(ebpf_module_t *em) {
    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_FILE_OPEN_CLOSE_COUNT
        , "Calls"
        , NETDATA_FILE_GROUP
        , 970
        , ebpf_create_global_dimension
        , publish_aggregated
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_FILE_OPEN_ERR_COUNT
            , "Calls"
            , NETDATA_FILE_GROUP
            , 971
            , ebpf_create_global_dimension
            , publish_aggregated
            , 2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_VFS_FILE_CLEAN_COUNT
        , "Calls"
        , NETDATA_VFS_GROUP
        , 972
        , ebpf_create_global_dimension
        , &publish_aggregated[NETDATA_DEL_START]
        , 1);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_VFS_FILE_IO_COUNT
        , "Calls"
        , NETDATA_VFS_GROUP
        , 973
        , ebpf_create_global_dimension
        , &publish_aggregated[NETDATA_IN_START_BYTE]
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_io_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_IO_FILE_BYTES
            , "bytes/s"
            , NETDATA_VFS_GROUP
            , 974);

        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_ERR_COUNT
            , "Calls"
            , NETDATA_VFS_GROUP
            , 975
            , ebpf_create_global_dimension
            , &publish_aggregated[2]
            , NETDATA_VFS_ERRORS);

    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_PROCESS_SYSCALL
        , "Calls"
        , NETDATA_PROCESS_GROUP
        , 976
        , ebpf_create_global_dimension
        , &publish_aggregated[NETDATA_PROCESS_START]
        , 2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_EXIT_SYSCALL
        , "Calls"
        , NETDATA_PROCESS_GROUP
        , 977
        , ebpf_create_global_dimension
        , &publish_aggregated[NETDATA_EXIT_START]
        , 2);

    ebpf_process_status_chart(NETDATA_EBPF_FAMILY
        , NETDATA_PROCESS_STATUS_NAME
        , "Total"
        , NETDATA_PROCESS_GROUP
        , 978);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_PROCESS_ERROR_NAME
            , "Calls"
            , NETDATA_PROCESS_GROUP
            , 979
            , ebpf_create_global_dimension
            , &publish_aggregated[NETDATA_PROCESS_START]
            , 2);
    }

}

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
static void ebpf_process_cleanup(void *ptr)
{
    (void)ptr;
    freez(aggregated_data);
    freez(publish_aggregated);
    freez(hash_values);

    if (functions.libnetdata) {
        dlclose(functions.libnetdata);
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
 */
void ebpf_process_allocate_global_vectors() {
    aggregated_data = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_syscall_stat_t));
    publish_aggregated = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_publish_syscall_t));
    hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
}

static void change_collector_event() {
    int i;
    if (running_on_kernel < NETDATA_KERNEL_V5_3)
        process_probes[10].name = NULL;

    for (i = 0; process_probes[i].name ; i++ ) {
        process_probes[i].type = 'p';
    }
}

static void change_syscalls() {
    static char *lfork = { "do_fork" };
    id_names[7] = lfork;
    process_probes[8].name = lfork;
}

/**
 * Set local function pointers, this function will never be compiled with static libraries
 */
static void set_local_pointers(ebpf_module_t *em) {
    bpf_map_lookup_elem = functions.bpf_map_lookup_elem;

    map_fd = functions.map_fd;

    if (em->mode == MODE_ENTRY)
        change_collector_event();

    if (functions.isrh >= NETDATA_MINIMUM_RH_VERSION && functions.isrh < NETDATA_RH_8)
        change_syscalls();
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
void *ebpf_process_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_process_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    ebpf_process_allocate_global_vectors();

    pthread_mutex_lock(&lock);
    fill_ebpf_functions(&functions);
    if (ebpf_load_libraries(&functions, "libnetdata_ebpf.so", ebpf_plugin_dir)) {
        pthread_mutex_unlock(&lock);
        return NULL;
    }
    pthread_mutex_unlock(&lock);

    set_local_pointers(em);
    ebpf_load_program(ebpf_plugin_dir, em->thread_id, em->mode, kernel_string,
                      em->thread_name, functions.load_bpf_file);

    ebpf_global_labels(aggregated_data, publish_aggregated, dimension_names, id_names, NETDATA_MAX_MONITOR_VECTOR);

    ebpf_create_global_charts(em);
    process_collector((usec_t)(em->update_time*USEC_PER_SEC), em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
