// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_process.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *process_dimension_names[NETDATA_MAX_MONITOR_VECTOR] = { "open",    "close", "delete",  "read",  "write",
                                                                     "process", "task",  "process", "thread" };
static char *process_id_names[NETDATA_MAX_MONITOR_VECTOR] = { "do_sys_open",  "__close_fd", "vfs_unlink",
                                                              "vfs_read",     "vfs_write",  "do_exit",
                                                              "release_task", "_do_fork",   "sys_clone" };
static char *status[] = { "process", "zombie" };

static netdata_idx_t *process_hash_values = NULL;
static netdata_syscall_stat_t *process_aggregated_data = NULL;
static netdata_publish_syscall_t *process_publish_aggregated = NULL;

static ebpf_data_t process_data;

static ebpf_process_stat_t **local_process_stats = NULL;
static ebpf_process_publish_apps_t **current_apps_data = NULL;
static ebpf_process_publish_apps_t **prev_apps_data = NULL;

int process_enabled = 0;

static int *map_fd = NULL;

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
static void ebpf_update_global_publish(
    netdata_publish_syscall_t *publish, netdata_publish_vfs_common_t *pvc, netdata_syscall_stat_t *input)
{
    netdata_publish_syscall_t *move = publish;
    while (move) {
        if (input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if (move->pcall) {
                move->ncall = (input->call > move->pcall) ? input->call - move->pcall : move->pcall - input->call;
                move->nbyte = (input->bytes > move->pbyte) ? input->bytes - move->pbyte : move->pbyte - input->bytes;
                move->nerr = (input->ecall > move->nerr) ? input->ecall - move->perr : move->perr - input->ecall;
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
static void
ebpf_process_update_apps_publish(ebpf_process_publish_apps_t *curr, ebpf_process_publish_apps_t *prev, int first)
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
    curr->publish_process      = curr->call_do_fork - prev->call_do_fork;
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
static void write_status_chart(char *family, netdata_publish_vfs_common_t *pvc)
{
    write_begin_chart(family, NETDATA_PROCESS_STATUS_NAME);

    write_chart_dimension(status[0], (long long)pvc->running);
    write_chart_dimension(status[1], (long long)pvc->zombie);

    write_end_chart();
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t pvc;
    ebpf_update_global_publish(process_publish_aggregated, &pvc, process_aggregated_data);

    write_count_chart(
        NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_EBPF_FAMILY, process_publish_aggregated, 2);

    write_count_chart(
        NETDATA_VFS_FILE_CLEAN_COUNT, NETDATA_EBPF_FAMILY, &process_publish_aggregated[NETDATA_DEL_START], 1);

    write_count_chart(
        NETDATA_VFS_FILE_IO_COUNT, NETDATA_EBPF_FAMILY, &process_publish_aggregated[NETDATA_IN_START_BYTE], 2);

    write_count_chart(
        NETDATA_EXIT_SYSCALL, NETDATA_EBPF_FAMILY, &process_publish_aggregated[NETDATA_EXIT_START], 2);
    write_count_chart(
        NETDATA_PROCESS_SYSCALL, NETDATA_EBPF_FAMILY, &process_publish_aggregated[NETDATA_PROCESS_START], 2);

    write_status_chart(NETDATA_EBPF_FAMILY, &pvc);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_EBPF_FAMILY, process_publish_aggregated, 2);
        write_err_chart(
            NETDATA_VFS_FILE_ERR_COUNT, NETDATA_EBPF_FAMILY, &process_publish_aggregated[2], NETDATA_VFS_ERRORS);
        write_err_chart(
            NETDATA_PROCESS_ERROR_NAME, NETDATA_EBPF_FAMILY, &process_publish_aggregated[NETDATA_PROCESS_START], 2);
    }

    write_io_chart(NETDATA_VFS_IO_FILE_BYTES, NETDATA_EBPF_FAMILY, process_id_names[3], process_id_names[4], &pvc);
}

/**
 * Sum values for pid
 *
 * @param root the structure with all available PIDs
 *
 * @param offset the address that we are reading
 *
 * @return it returns the sum of all PIDs
 */
long long ebpf_process_sum_values_for_pids(struct pid_on_target *root, size_t offset)
{
    long long ret = 0;
    while (root) {
        int32_t pid = root->pid;
        ebpf_process_publish_apps_t *w = current_apps_data[pid];
        if (w) {
            ret += get_value_from_structure((char *)w, offset);
        }

        root = root->next;
    }

    return ret;
}

/**
 * Remove process pid
 *
 * Remove from PID task table when task_release was called.
 */
void ebpf_process_remove_pids()
{
    struct pid_stat *pids = root_of_pids;
    int pid_fd = map_fd[0];
    while (pids) {
        uint32_t pid = pids->pid;
        ebpf_process_stat_t *w = local_process_stats[pid];
        if (w) {
            if (w->removeme) {
                freez(w);
                local_process_stats[pid] = NULL;
                bpf_map_delete_elem(pid_fd, &pid);
            }
        }

        pids = pids->next;
    }
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
 */
void ebpf_process_send_apps_data(ebpf_module_t *em, struct target *root)
{
    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_open));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                value = ebpf_process_sum_values_for_pids(
                    w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_open_error));
                write_chart_dimension(w->name, value);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSED);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value =
                ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_closed));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                value = ebpf_process_sum_values_for_pids(
                    w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_close_error));
                write_chart_dimension(w->name, value);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_FILE_DELETED);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value =
                ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_deleted));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(
                w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_write_call));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                value = ebpf_process_sum_values_for_pids(
                    w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_write_error));
                write_chart_dimension(w->name, value);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value =
                ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_read_call));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                value = ebpf_process_sum_values_for_pids(
                    w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_read_error));
                write_chart_dimension(w->name, value);
            }
        }
        write_end_chart();
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(
                w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_write_bytes));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_VFS_READ_BYTES);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(
                w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_read_bytes));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_PROCESS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value =
                ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_process));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_THREAD);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value =
                ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_thread));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_CLOSE);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, publish_task));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    ebpf_process_remove_pids();
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
        if (!bpf_map_lookup_elem(map_fd[1], &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
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
    size_t i;
    for (i = 0; i < all_pids_count; i++) {
        uint32_t current_pid = pid_index[i];
        ebpf_process_stat_t *ps = local_process_stats[current_pid];
        if (!ps)
            continue;

        ebpf_process_publish_apps_t *cad = current_apps_data[current_pid];
        ebpf_process_publish_apps_t *pad = prev_apps_data[current_pid];
        int lstatus;
        if (!cad) {
            ebpf_process_publish_apps_t *ptr = callocz(2, sizeof(ebpf_process_publish_apps_t));
            cad = &ptr[0];
            current_apps_data[current_pid] = cad;
            pad = &ptr[1];
            prev_apps_data[current_pid] = pad;
            lstatus = 1;
        } else {
            memcpy(pad, cad, sizeof(ebpf_process_publish_apps_t));
            lstatus = 0;
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

        cad->bytes_written = (uint64_t)ps->write_bytes + (uint64_t)ps->write_bytes;
        cad->bytes_read = (uint64_t)ps->read_bytes + (uint64_t)ps->readv_bytes;

        ebpf_process_update_apps_publish(cad, pad, lstatus);
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
static void ebpf_create_io_chart(char *family, char *name, char *axis, char *web, int order)
{
    printf("CHART %s.%s '' 'Bytes written and read' '%s' '%s' '' line %d %d\n",
           family,
           name,
           axis,
           web,
           order,
           update_every);

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
static void ebpf_process_status_chart(char *family, char *name, char *axis, char *web, int order)
{
    printf("CHART %s.%s '' 'Process not closed' '%s' '%s' '' line %d %d ''\n",
           family,
           name,
           axis,
           web,
           order,
           update_every);

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
    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_FILE_OPEN_CLOSE_COUNT,
                      "Open and close calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_FILE_GROUP,
                      21000,
                      ebpf_create_global_dimension,
                      process_publish_aggregated,
                      2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_FILE_OPEN_ERR_COUNT,
                          "Open fails",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_FILE_GROUP,
                          21001,
                          ebpf_create_global_dimension,
                          process_publish_aggregated,
                          2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_VFS_FILE_CLEAN_COUNT,
                      "Remove files",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      21002,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_DEL_START],
                      1);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_VFS_FILE_IO_COUNT,
                      "Calls to IO",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_VFS_GROUP,
                      21003,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_IN_START_BYTE],
                      2);

    ebpf_create_io_chart(NETDATA_EBPF_FAMILY,
                         NETDATA_VFS_IO_FILE_BYTES,
                         EBPF_COMMON_DIMENSION_BYTESS,
                         NETDATA_VFS_GROUP,
                         21004);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_VFS_FILE_ERR_COUNT,
                          "Fails to write or read",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_VFS_GROUP,
                          21005,
                          ebpf_create_global_dimension,
                          &process_publish_aggregated[2],
                          NETDATA_VFS_ERRORS);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_PROCESS_SYSCALL,
                      "Start process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      21006,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_PROCESS_START],
                      2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_EXIT_SYSCALL,
                      "Exit process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      21007,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_EXIT_START],
                      2);

    ebpf_process_status_chart(NETDATA_EBPF_FAMILY,
                              NETDATA_PROCESS_STATUS_NAME,
                              EBPF_COMMON_DIMENSION_DIFFERENCE,
                              NETDATA_PROCESS_GROUP,
                              21008);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_PROCESS_ERROR_NAME,
                          "Fails to create process",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_PROCESS_GROUP,
                          21009,
                          ebpf_create_global_dimension,
                          &process_publish_aggregated[NETDATA_PROCESS_START],
                          2);
    }
}

/**
 * Create process apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param root a pointer for the targets.
 */
static void ebpf_process_create_apps_charts(ebpf_module_t *em, struct target *root)
{
    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN,
                               "Number of open files",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20061,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
                                   "Fails to open files",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20062,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSED,
                               "Files closed",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20063,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
                                   "Fails to close files",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20064,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_FILE_DELETED,
                               "Files deleted",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20065,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
                               "Write to disk",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20066,
                               apps_groups_root_target);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
                                   "Fails to write",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20067,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
                               "Read from disk",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20068,
                               root);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
                                   "Fails to read",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_APPS_SYSCALL_GROUP,
                                   20069,
                                   root);
    }

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
                               "Bytes written on disk",
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20070,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
                               "Bytes read from disk",
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20071,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_PROCESS,
                               "Process started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20072,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_THREAD,
                               "Threads started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20073,
                               root);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_CLOSE,
                               "Tasks closed",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_SYSCALL_GROUP,
                               20074,
                               root);
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param root a pointer for the targets.
 */
static void ebpf_create_apps_charts(ebpf_module_t *em, struct target *root)
{
    struct target *w;
    int newly_added = 0;

    for (w = root; w; w = w->next) {
        if (w->target)
            continue;

        if (unlikely(w->processes && (debug_enabled || w->debug_enabled))) {
            struct pid_on_target *pid_on_target;

            fprintf(
                stderr, "ebpf.plugin: target '%s' has aggregated %u process%s:", w->name, w->processes,
                (w->processes == 1) ? "" : "es");

            for (pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
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

    if (!newly_added)
        return;

    if (ebpf_modules[EBPF_MODULE_PROCESS_IDX].apps_charts)
        ebpf_process_create_apps_charts(em, root);

    if (ebpf_modules[EBPF_MODULE_SOCKET_IDX].apps_charts)
        ebpf_socket_create_apps_charts(NULL, root);
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
    int publish_global = em->global_charts;
    int apps_enabled = em->apps_charts;
    int pid_fd = map_fd[0];
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();

        pthread_mutex_lock(&collect_data_mutex);
        cleanup_exited_pids(local_process_stats);
        collect_data_for_all_processes(local_process_stats, pid_index, pid_fd);

        ebpf_create_apps_charts(em, apps_groups_root_target);

        pthread_cond_broadcast(&collect_data_cond_var);
        pthread_mutex_unlock(&collect_data_mutex);

        int publish_apps = 0;
        if (apps_enabled && all_pids_count > 0) {
            publish_apps = 1;
            ebpf_process_update_apps_data();
        }

        pthread_mutex_lock(&lock);
        if (publish_global) {
            ebpf_process_send_data(em);
        }

        if (publish_apps) {
            ebpf_process_send_apps_data(em, apps_groups_root_target);
        }
        pthread_mutex_unlock(&lock);

        fflush(stdout);
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

    freez(process_aggregated_data);
    freez(process_publish_aggregated);
    freez(process_hash_values);

    freez(local_process_stats);

    freez(process_data.map_fd);
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
static void ebpf_process_allocate_global_vectors(size_t length)
{
    process_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    process_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
    process_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    local_process_stats = callocz((size_t)pid_max, sizeof(ebpf_process_stat_t *));
    current_apps_data = callocz((size_t)pid_max, sizeof(ebpf_process_publish_apps_t *));
    prev_apps_data = callocz((size_t)pid_max, sizeof(ebpf_process_publish_apps_t *));
}

void change_process_event()
{
    int i;
    if (running_on_kernel < NETDATA_KERNEL_V5_3)
        process_probes[EBPF_SYS_CLONE_IDX].name = NULL;

    for (i = 0; process_probes[i].name; i++) {
        process_probes[i].type = 'p';
    }
}

static void change_syscalls()
{
    static char *lfork = { "do_fork" };
    process_id_names[7] = lfork;
    process_probes[8].name = lfork;
}

/**
 * Set local variables
 *
 */
static void set_local_pointers()
{
    map_fd = process_data.map_fd;

    if (process_data.isrh >= NETDATA_MINIMUM_RH_VERSION && process_data.isrh < NETDATA_RH_8)
        change_syscalls();
}

/*****************************************************************
 *
 *  EBPF PROCESS THREAD
 *
 *****************************************************************/

/**
 *
 */
static void wait_for_all_threads_die()
{
    ebpf_modules[EBPF_MODULE_PROCESS_IDX].enabled = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    int max = 10;
    int i;
    for (i = 0; i < max; i++) {
        heartbeat_next(&hb, 200000);

        size_t j, counter = 0, compare = 0;
        for (j = 0; ebpf_modules[j].thread_name; j++) {
            if (!ebpf_modules[j].enabled)
                counter++;

            compare++;
        }

        if (counter == compare)
            break;
    }
}

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
    process_enabled = em->enabled;
    fill_ebpf_data(&process_data);

    pthread_mutex_lock(&lock);
    ebpf_process_allocate_global_vectors(NETDATA_MAX_MONITOR_VECTOR);

    if (ebpf_update_kernel(&process_data)) {
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    set_local_pointers();
    if (ebpf_load_program(
            ebpf_plugin_dir, em->thread_id, em->mode, kernel_string, em->thread_name, process_data.map_fd)) {
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    ebpf_global_labels(
        process_aggregated_data, process_publish_aggregated, process_dimension_names, process_id_names,
        NETDATA_MAX_MONITOR_VECTOR);

    if (process_enabled) {
        ebpf_create_global_charts(em);
    }

    pthread_mutex_unlock(&lock);

    process_collector((usec_t)(em->update_time * USEC_PER_SEC), em);

endprocess:
    wait_for_all_threads_die();
    netdata_thread_cleanup_pop(1);
    return NULL;
}
