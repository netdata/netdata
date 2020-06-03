// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_process.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *process_dimension_names[NETDATA_MAX_MONITOR_VECTOR] = { "open", "close", "delete", "read", "write",
                                                             "process", "task", "process", "thread" };
static char *process_id_names[NETDATA_MAX_MONITOR_VECTOR] = { "do_sys_open", "__close_fd", "vfs_unlink", "vfs_read", "vfs_write",
                                                      "do_exit", "release_task", "_do_fork", "sys_clone" };
static char *status[] = { "process", "zombie" };

static netdata_idx_t *process_hash_values = NULL;
static netdata_syscall_stat_t *process_aggregated_data = NULL;
static netdata_publish_syscall_t *process_publish_aggregated = NULL;

static ebpf_functions_t process_functions;

static ebpf_process_stat_t **local_process_stats = NULL;
static ebpf_process_publish_apps_t **current_apps_data = NULL;
static ebpf_process_publish_apps_t **prev_apps_data = NULL;

#ifndef STATIC
/**
 * Pointers used when collector is dynamically linked
 */

//Libbpf (It is necessary to have at least kernel 4.10)
static int (*bpf_map_lookup_elem)(int, const void *, void *);

static int *map_fd = NULL;
/**
 * End of the pointers
 */
 #endif

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
static void ebpf_update_global_publish(netdata_publish_syscall_t *publish,
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
 * Update apps dimension to publish.
 *
 * @param curr Last values read from memory.
 * @param prev Previous values read from memory.
 * @param first was it allocated now?
 */
static void ebpf_process_update_apps_publish(ebpf_process_publish_apps_t *curr,
                                             ebpf_process_publish_apps_t *prev,int first)
{
    if (first)
        return;

    curr->publish_open         = curr->call_sys_open - prev->call_sys_open;
    curr->publish_closed       = curr->call_close_fd - prev->call_close_fd;
    curr->publish_deleted      = curr->call_vfs_unlink - prev->call_vfs_unlink;
    curr->publish_write_call   = curr->call_write - prev->call_write;
    curr->publish_write_bytes  = curr->bytes_written - prev->bytes_written;
    curr->publish_read_call    = curr->call_read - prev->call_read;
    curr->publish_read_bytes   = curr->bytes_read - prev->bytes_read;
    curr->publish_process       = curr->call_do_fork - prev->call_do_fork;
    curr->publish_thread       = curr->call_sys_clone - prev->call_sys_clone;
    curr->publish_task         = curr->call_release_task - prev->call_release_task;
    curr->publish_open_error   = curr->ecall_sys_open - prev->ecall_sys_open;
    curr->publish_close_error  = curr->ecall_close_fd - prev->ecall_close_fd;
    curr->publish_write_error  = curr->ecall_write - prev->ecall_write;
    curr->publish_read_error   = curr->ecall_read - prev->ecall_read;
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
    ebpf_update_global_publish(process_publish_aggregated, &pvc, process_aggregated_data);

    write_count_chart(NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_EBPF_FAMILY, process_publish_aggregated, 2);
    write_count_chart(NETDATA_VFS_FILE_CLEAN_COUNT,
                             NETDATA_EBPF_FAMILY,
                             &process_publish_aggregated[NETDATA_DEL_START],
                             1);
    write_count_chart(NETDATA_VFS_FILE_IO_COUNT,
                             NETDATA_EBPF_FAMILY,
                             &process_publish_aggregated[NETDATA_IN_START_BYTE],
                             2);
    write_count_chart(NETDATA_EXIT_SYSCALL,
                             NETDATA_EBPF_FAMILY,
                             &process_publish_aggregated[NETDATA_EXIT_START],
                             2);
    write_count_chart(NETDATA_PROCESS_SYSCALL,
                             NETDATA_EBPF_FAMILY,
                             &process_publish_aggregated[NETDATA_PROCESS_START],
                             2);

    write_status_chart(NETDATA_EBPF_FAMILY, &pvc);
    if(em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_EBPF_FAMILY, process_publish_aggregated, 2);
        write_err_chart(NETDATA_VFS_FILE_ERR_COUNT,
                               NETDATA_EBPF_FAMILY,
                               &process_publish_aggregated[2],
                               NETDATA_VFS_ERRORS);
        write_err_chart(NETDATA_PROCESS_ERROR_NAME,
                               NETDATA_EBPF_FAMILY,
                               &process_publish_aggregated[NETDATA_PROCESS_START],
                               2);

        write_io_chart(NETDATA_VFS_IO_FILE_BYTES, NETDATA_EBPF_FAMILY, process_id_names[3],
                       process_id_names[4], &pvc);
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

    netdata_idx_t *val = process_hash_values;
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

    process_aggregated_data[0].call = res[NETDATA_KEY_CALLS_DO_SYS_OPEN];
    process_aggregated_data[1].call = res[NETDATA_KEY_CALLS_CLOSE_FD];
    process_aggregated_data[2].call = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    process_aggregated_data[3].call = res[NETDATA_KEY_CALLS_VFS_READ] + res[NETDATA_KEY_CALLS_VFS_READV];
    process_aggregated_data[4].call = res[NETDATA_KEY_CALLS_VFS_WRITE] + res[NETDATA_KEY_CALLS_VFS_WRITEV];
    process_aggregated_data[5].call = res[NETDATA_KEY_CALLS_DO_EXIT];
    process_aggregated_data[6].call = res[NETDATA_KEY_CALLS_RELEASE_TASK];
    process_aggregated_data[7].call = res[NETDATA_KEY_CALLS_DO_FORK];
    process_aggregated_data[8].call = res[NETDATA_KEY_CALLS_SYS_CLONE];

    process_aggregated_data[0].ecall = res[NETDATA_KEY_ERROR_DO_SYS_OPEN];
    process_aggregated_data[1].ecall = res[NETDATA_KEY_ERROR_CLOSE_FD];
    process_aggregated_data[2].ecall = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    process_aggregated_data[3].ecall = res[NETDATA_KEY_ERROR_VFS_READ] + res[NETDATA_KEY_ERROR_VFS_READV];
    process_aggregated_data[4].ecall = res[NETDATA_KEY_ERROR_VFS_WRITE] + res[NETDATA_KEY_ERROR_VFS_WRITEV];
    process_aggregated_data[7].ecall = res[NETDATA_KEY_ERROR_DO_FORK];
    process_aggregated_data[8].ecall = res[NETDATA_KEY_ERROR_SYS_CLONE];

    process_aggregated_data[2].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] +
                                       (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    process_aggregated_data[3].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] +
                                       (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void ebpf_process_update_apps_data()
{
    int i;
    for ( i = 0 ; i < pids_running ; i++) {
        pid_t current_pid = pid_index[i];
        ebpf_process_stat_t *ps = local_process_stats[current_pid];
        if (!ps)
            continue;

        ebpf_process_publish_apps_t *cad = current_apps_data[current_pid];
        ebpf_process_publish_apps_t *pad = prev_apps_data[current_pid];
        int status;
        if (!cad) {
            cad = callocz(2, sizeof(netdata_syscall_stat_t));
            current_apps_data[current_pid] = &cad[0];
            prev_apps_data[current_pid] = &cad[1];
            status = 1;
        } else {
            memcpy(pad, cad, sizeof(netdata_syscall_stat_t));
            status = 0;
        }

        //Read data
        cad->call_sys_open = ps->open_call;
        cad->call_close_fd = ps->close_call;
        cad->call_vfs_unlink = ps->unlink_call;
        cad->call_read = ps->read_call + ps->readv_call;
        cad->call_write = ps->write_call + ps->writev_call;
        cad->call_do_exit = ps->exit_call;
        cad->call_release_task = ps->release_call;
        cad->call_do_fork = ps->fork_call;
        cad->call_sys_clone = ps->clone_call;

        cad->ecall_sys_open = ps->open_err;
        cad->ecall_close_fd = ps->close_err;
        cad->ecall_vfs_unlink = ps->unlink_err;
        cad->ecall_read = ps->read_err + ps->readv_err;
        cad->ecall_write = ps->write_err + ps->writev_err;
        cad->ecall_do_fork = ps->fork_err;
        cad->ecall_sys_clone = ps->clone_err;

        cad->bytes_written = (uint64_t)ps->write_bytes +
                                           (uint64_t)ps->write_bytes;
        cad->bytes_read = (uint64_t)ps->read_bytes +
                                           (uint64_t)ps->readv_bytes;

        ebpf_process_update_apps_publish(cad, pad, status);
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

    printf("DIMENSION %s %s absolute 1 1\n", process_id_names[3], NETDATA_VFS_DIM_OUT_FILE_BYTES);
    printf("DIMENSION %s %s absolute 1 1\n", process_id_names[4], NETDATA_VFS_DIM_IN_FILE_BYTES);
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
static void ebpf_create_global_charts(ebpf_module_t *em)
{
    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_FILE_OPEN_CLOSE_COUNT
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_FILE_GROUP
        , 21000
        , ebpf_create_global_dimension
        , process_publish_aggregated
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_FILE_OPEN_ERR_COUNT
            , EBPF_COMMON_DIMENSION_CALL
            , NETDATA_FILE_GROUP
            , 21001
            , ebpf_create_global_dimension
            , process_publish_aggregated
            , 2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_VFS_FILE_CLEAN_COUNT
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_VFS_GROUP
        , 21002
        , ebpf_create_global_dimension
        , &process_publish_aggregated[NETDATA_DEL_START]
        , 1);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_VFS_FILE_IO_COUNT
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_VFS_GROUP
        , 21003
        , ebpf_create_global_dimension
        , &process_publish_aggregated[NETDATA_IN_START_BYTE]
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_io_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_IO_FILE_BYTES
            , EBPF_COMMON_DIMENSION_BYTESS
            , NETDATA_VFS_GROUP
            , 21004);

        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_ERR_COUNT
            , EBPF_COMMON_DIMENSION_CALL
            , NETDATA_VFS_GROUP
            , 21005
            , ebpf_create_global_dimension
            , &process_publish_aggregated[2]
            , NETDATA_VFS_ERRORS);

    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_PROCESS_SYSCALL
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_PROCESS_GROUP
        , 21006
        , ebpf_create_global_dimension
        , &process_publish_aggregated[NETDATA_PROCESS_START]
        , 2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_EXIT_SYSCALL
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_PROCESS_GROUP
        , 21007
        , ebpf_create_global_dimension
        , &process_publish_aggregated[NETDATA_EXIT_START]
        , 2);

    ebpf_process_status_chart(NETDATA_EBPF_FAMILY
        , NETDATA_PROCESS_STATUS_NAME
        , EBPF_COMMON_DIMENSION_DIFFERENCE
        , NETDATA_PROCESS_GROUP
        , 21008);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_PROCESS_ERROR_NAME
            , EBPF_COMMON_DIMENSION_CALL
            , NETDATA_PROCESS_GROUP
            , 21009
            , ebpf_create_global_dimension
            , &process_publish_aggregated[NETDATA_PROCESS_START]
            , 2);
    }

}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_process_create_apps_charts(ebpf_module_t *em, struct target *root)
{
    struct target *w;
    int newly_added = 0;

    for(w = root ; w ; w = w->next) {
        if (w->target) continue;

        if(unlikely(w->processes && (debug_enabled || w->debug_enabled))) {
            struct pid_on_target *pid_on_target;

            fprintf(stderr, "ebpf.plugin: target '%s' has aggregated %u process%s:", w->name, w->processes, (w->processes == 1)?"":"es");

            for(pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
                fprintf(stderr, " %d", pid_on_target->pid);
            }

            fputc('\n', stderr);
        }

        if (!w->exposed && w->processes) {
            newly_added++;
            w->exposed = 1;
            if (debug_enabled || w->debug_enabled)
                debug_log_int("%s just added - regenerating charts.", w->name);
        }
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20061,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20062,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSED,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20063,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20064,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_DELETED,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20065,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20066,
                               apps_groups_root_target);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20067,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20068,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20069,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20070,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20071,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20072,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_PROCESS,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20073,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_THREAD,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20074,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_CLOSE,
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20075,
                               root);

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
    int enabled = em->enabled;
    int apps_enabled = em->apps_charts;
    while(!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();
        pids_running = collect_data_for_all_processes(local_process_stats,
                                                      pid_index,
                                                      process_functions.bpf_map_get_next_key,
                                                      process_functions.bpf_map_lookup_elem,
                                                      map_fd[0]);

        if (enabled) {
            int publish_apps = 0;
            if (pids_running > 0 && apps_enabled){
                publish_apps = 1;
                ebpf_process_update_apps_data();
            }

            pthread_mutex_lock(&lock);
            ebpf_process_send_data(em);
            if (publish_apps) {
                ebpf_process_create_apps_charts(em, apps_groups_root_target);
            }
            pthread_mutex_unlock(&lock);
        }

        fflush(stdout);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean the allocated process stat structure
 */
static void clean_process_stat()
{
    int i;
    for (i = 0 ; i < pids_running ; i++) {
        ebpf_process_stat_t *w = local_process_stats[pid_index[i]];
        freez(w);
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_process_cleanup(void *ptr)
{
    (void)ptr;

    freez(process_aggregated_data);
    freez(process_publish_aggregated);
    freez(process_hash_values);

    clean_process_stat();
    freez(local_process_stats);

    if (process_functions.libnetdata) {
        dlclose(process_functions.libnetdata);
    }

    freez(process_functions.map_fd);
    freez(current_apps_data);
    freez(prev_apps_data);
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
static void ebpf_process_allocate_global_vectors(size_t length) {
    process_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    process_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
    process_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    local_process_stats = callocz((size_t)pid_max, sizeof(ebpf_process_stat_t *));
    current_apps_data = callocz((size_t)pid_max, sizeof(ebpf_process_publish_apps_t *));
    prev_apps_data = callocz(length, sizeof(ebpf_process_publish_apps_t *));
}

static void change_collector_event() {
    int i;
    if (running_on_kernel < NETDATA_KERNEL_V5_3)
        process_probes[EBPF_SYS_CLONE_IDX].name = NULL;

    for (i = 0; process_probes[i].name ; i++ ) {
        process_probes[i].type = 'p';
    }
}

static void change_syscalls() {
    static char *lfork = { "do_fork" };
    process_id_names[7] = lfork;
    process_probes[8].name = lfork;
}

/**
 * Set local variables
 */
static void set_local_pointers(ebpf_module_t *em) {
#ifndef STATIC
    bpf_map_lookup_elem = process_functions.bpf_map_lookup_elem;

#endif

    map_fd = process_functions.map_fd;

    if (em->mode == MODE_ENTRY) {
        change_collector_event();
    }

    if (process_functions.isrh >= NETDATA_MINIMUM_RH_VERSION && process_functions.isrh < NETDATA_RH_8)
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
    fill_ebpf_functions(&process_functions);

    pthread_mutex_lock(&lock);
    ebpf_process_allocate_global_vectors(NETDATA_MAX_MONITOR_VECTOR);

    if (ebpf_load_libraries(&process_functions, "libnetdata_ebpf.so", ebpf_plugin_dir)) {
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    set_local_pointers(em);
    if (ebpf_load_program(ebpf_plugin_dir, em->thread_id, em->mode, kernel_string,
                      em->thread_name, process_functions.map_fd, process_functions.load_bpf_file) ) {
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    ebpf_global_labels(process_aggregated_data, process_publish_aggregated, process_dimension_names,
                       process_id_names, NETDATA_MAX_MONITOR_VECTOR);

    if (em->enabled) {
        ebpf_create_global_charts(em);
        if (em->apps_charts) {
            ebpf_process_create_apps_charts(em, apps_groups_root_target);
        }
    }

    pthread_mutex_unlock(&lock);

    process_collector((usec_t)(em->update_time*USEC_PER_SEC), em);

endprocess:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
