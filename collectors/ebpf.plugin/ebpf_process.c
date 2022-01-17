// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_process.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *process_dimension_names[NETDATA_KEY_PUBLISH_PROCESS_END] = { "process", "task",  "process", "thread" };
static char *process_id_names[NETDATA_KEY_PUBLISH_PROCESS_END] = { "do_exit", "release_task", "_do_fork", "sys_clone" };
static char *status[] = { "process", "zombie" };

static ebpf_local_maps_t process_maps[] = {{.name = "tbl_pid_stats", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                            .user_input = 0,
                                            .type = NETDATA_EBPF_MAP_RESIZABLE  | NETDATA_EBPF_MAP_PID,
                                            .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                           {.name = "tbl_total_stats", .internal_input = NETDATA_KEY_END_VECTOR,
                                            .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                            .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                           {.name = "process_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                            .user_input = 0,
                                            .type = NETDATA_EBPF_MAP_CONTROLLER,
                                            .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                           {.name = NULL, .internal_input = 0, .user_input = 0,
                                            .type = NETDATA_EBPF_MAP_CONTROLLER,
                                            .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

char *tracepoint_sched_type = { "sched" } ;
char *tracepoint_sched_process_exit = { "sched_process_exit" };
char *tracepoint_sched_process_exec = { "sched_process_exec" };
char *tracepoint_sched_process_fork = { "sched_process_fork" };
static int was_sched_process_exit_enabled = 0;
static int was_sched_process_exec_enabled = 0;
static int was_sched_process_fork_enabled = 0;

static netdata_idx_t *process_hash_values = NULL;
static netdata_syscall_stat_t process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_END];
static netdata_publish_syscall_t process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_END];

ebpf_process_stat_t **global_process_stats = NULL;
ebpf_process_publish_apps_t **current_apps_data = NULL;

int process_enabled = 0;

static struct bpf_object *objects = NULL;
static struct bpf_link **probe_links = NULL;

struct config process_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

static struct netdata_static_thread cgroup_thread = {"EBPF CGROUP", NULL, NULL,
                                                    1, NULL, NULL,  NULL};

static char *threads_stat[NETDATA_EBPF_THREAD_STAT_END] = {"total", "running"};
static char *load_event_stat[NETDATA_EBPF_LOAD_STAT_END] = {"legacy", "co-re"};

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
static void ebpf_update_global_publish(netdata_publish_syscall_t *publish, netdata_publish_vfs_common_t *pvc,
                                       netdata_syscall_stat_t *input)
{
    netdata_publish_syscall_t *move = publish;
    int selector = NETDATA_KEY_PUBLISH_PROCESS_EXIT;
    while (move) {
        move->ncall = (input->call > move->pcall) ? input->call - move->pcall : move->pcall - input->call;
        move->nbyte = (input->bytes > move->pbyte) ? input->bytes - move->pbyte : move->pbyte - input->bytes;
        move->nerr = (input->ecall > move->nerr) ? input->ecall - move->perr : move->perr - input->ecall;

        move->pcall = input->call;
        move->pbyte = input->bytes;
        move->perr = input->ecall;

        input = input->next;
        move = move->next;
        selector++;
    }

    pvc->running = (long)publish[NETDATA_KEY_PUBLISH_PROCESS_FORK].ncall -
                   (long)publish[NETDATA_KEY_PUBLISH_PROCESS_CLONE].ncall;
    publish[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].ncall = -publish[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].ncall;
    pvc->zombie = (long)publish[NETDATA_KEY_PUBLISH_PROCESS_EXIT].ncall +
                  (long)publish[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].ncall;
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
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t pvc;
    ebpf_update_global_publish(process_publish_aggregated, &pvc, process_aggregated_data);

    write_count_chart(NETDATA_EXIT_SYSCALL, NETDATA_EBPF_SYSTEM_GROUP,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT], 2);
    write_count_chart(NETDATA_PROCESS_SYSCALL, NETDATA_EBPF_SYSTEM_GROUP,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK], 2);

    write_status_chart(NETDATA_EBPF_SYSTEM_GROUP, &pvc);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_PROCESS_ERROR_NAME, NETDATA_EBPF_SYSTEM_GROUP,
                        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK], 2);
    }
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
    int pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;
    while (pids) {
        uint32_t pid = pids->pid;
        ebpf_process_stat_t *w = global_process_stats[pid];
        if (w) {
            if (w->removeme) {
                freez(w);
                global_process_stats[pid] = NULL;
                bpf_map_delete_elem(pid_fd, &pid);
            }
        }

        pids = pids->next;
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
 */
void ebpf_process_send_apps_data(struct target *root, ebpf_module_t *em)
{
    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_PROCESS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, create_process));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_THREAD);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, create_thread));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_EXIT);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t,
                                                                           call_do_exit));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_CLOSE);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t,
                                                                           call_release_task));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_ERROR);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed && w->processes)) {
                value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t,
                                                                               task_err));
                write_chart_dimension(w->name, value);
            }
        }
        write_end_chart();
    }

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
    netdata_idx_t res[NETDATA_KEY_END_VECTOR];

    netdata_idx_t *val = process_hash_values;
    int fd = process_maps[NETDATA_PROCESS_GLOBAL_TABLE].map_fd;
    for (idx = 0; idx < NETDATA_KEY_END_VECTOR; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
        } else {
            res[idx] = 0;
        }
    }

    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_EXIT].call = res[NETDATA_KEY_CALLS_DO_EXIT];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].call = res[NETDATA_KEY_CALLS_RELEASE_TASK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_FORK].call = res[NETDATA_KEY_CALLS_DO_FORK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_CLONE].call = res[NETDATA_KEY_CALLS_SYS_CLONE];

    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_FORK].ecall = res[NETDATA_KEY_ERROR_DO_FORK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_CLONE].ecall = res[NETDATA_KEY_ERROR_SYS_CLONE];
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void ebpf_process_update_apps_data()
{
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        uint32_t current_pid = pids->pid;
        ebpf_process_stat_t *ps = global_process_stats[current_pid];
        if (!ps) {
            pids = pids->next;
            continue;
        }

        ebpf_process_publish_apps_t *cad = current_apps_data[current_pid];
        if (!cad) {
            cad = callocz(1, sizeof(ebpf_process_publish_apps_t));
            current_apps_data[current_pid] = cad;
        }

        //Read data
        cad->call_do_exit = ps->exit_call;
        cad->call_release_task = ps->release_call;
        cad->create_process = ps->create_process;
        cad->create_thread = ps->create_thread;

        cad->task_err = ps->task_err;

        pids = pids->next;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 */
static void ebpf_update_process_cgroup()
{
    ebpf_cgroup_target_t *ect ;
    int pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            ebpf_process_stat_t *out = &pids->ps;
            if (global_process_stats[pid]) {
                ebpf_process_stat_t *in = global_process_stats[pid];

                memcpy(out, in, sizeof(ebpf_process_stat_t));
            } else {
                if (bpf_map_lookup_elem(pid_fd, &pid, out)) {
                    memset(out, 0, sizeof(ebpf_process_stat_t));
                }
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create process status chart
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashboard
 * @param order  the order number of the specified chart
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_process_status_chart(char *family, char *name, char *axis,
                                      char *web, char *algorithm, int order, int update_every)
{
    printf("CHART %s.%s '' 'Process not closed' '%s' '%s' '' line %d %d '' 'ebpf.plugin' 'process'\n",
           family,
           name,
           axis,
           web,
           order,
           update_every);

    printf("DIMENSION %s '' %s 1 1\n", status[0], algorithm);
    printf("DIMENSION %s '' %s 1 1\n", status[1], algorithm);
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
    ebpf_create_chart(NETDATA_EBPF_SYSTEM_GROUP,
                      NETDATA_PROCESS_SYSCALL,
                      "Start process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21002,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(NETDATA_EBPF_SYSTEM_GROUP,
                      NETDATA_EXIT_SYSCALL,
                      "Exit process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21003,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_process_status_chart(NETDATA_EBPF_SYSTEM_GROUP,
                              NETDATA_PROCESS_STATUS_NAME,
                              EBPF_COMMON_DIMENSION_DIFFERENCE,
                              NETDATA_PROCESS_GROUP,
                              ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                              21004, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_SYSTEM_GROUP,
                          NETDATA_PROCESS_ERROR_NAME,
                          "Fails to create process",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_PROCESS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          21005,
                          ebpf_create_global_dimension,
                          &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
                          2, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }
}

/**
 * Create chart for Statistic Thread
 *
 * Write to standard output current values for threads.
 *
 * @param em a pointer to the structure with the default values.
 */
static inline void ebpf_create_statistic_thread_chart(ebpf_module_t *em)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_THREADS,
                         "Threads info.",
                         "threads",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         140000,
                         em->update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(threads_stat[NETDATA_EBPF_THREAD_STAT_TOTAL],
                                threads_stat[NETDATA_EBPF_THREAD_STAT_TOTAL],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(threads_stat[NETDATA_EBPF_THREAD_STAT_RUNNING],
                                threads_stat[NETDATA_EBPF_THREAD_STAT_RUNNING],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create chart for Load Thread
 *
 * Write to standard output current values for load mode.
 *
 * @param em a pointer to the structure with the default values.
 */
static inline void ebpf_create_statistic_load_chart(ebpf_module_t *em)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_LOAD_METHOD,
                         "Load info.",
                         "methods",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         140001,
                         em->update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
                                load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE],
                                load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create Statistics Charts
 *
 * Create charts that will show statistics related to eBPF plugin.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_statistic_charts(ebpf_module_t *em)
{
    ebpf_create_statistic_thread_chart(em);

    ebpf_create_statistic_load_chart(em);
}

/**
 * Create process apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param ptr  a pointer for the targets.
 */
void ebpf_process_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_PROCESS,
                               "Process started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20065,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_THREAD,
                               "Threads started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20066,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_EXIT,
                               "Tasks starts exit process.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20067,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_CLOSE,
                               "Tasks closed",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20068,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_ERROR,
                                   "Errors to create process or threads.",
                                   EBPF_COMMON_DIMENSION_CALL,
                                   NETDATA_PROCESS_GROUP,
                                   NETDATA_EBPF_CHART_TYPE_STACKED,
                                   20069,
                                   ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                   root,
                                   em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param root a pointer for the targets.
 */
static void ebpf_create_apps_charts(struct target *root)
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

    int counter;
    for (counter = 0; ebpf_modules[counter].thread_name; counter++) {
        ebpf_module_t *current = &ebpf_modules[counter];
        if (current->enabled && current->apps_charts && current->apps_routine)
            current->apps_routine(current, root);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/

/**
 * Cgroup update shm
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data from shared memory.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_cgroup_update_shm(void *ptr)
{
    UNUSED(ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);

    usec_t step = 30 * USEC_PER_SEC;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        if (close_ebpf_plugin)
            break;

        if (!shm_ebpf_cgroup.header)
            ebpf_map_cgroup_shared_memory();

        ebpf_parse_cgroup_shm_data();
    }

    return NULL;
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param ps  structure used to store data
 * @param pids input data
 */
static void ebpf_process_sum_cgroup_pids(ebpf_process_stat_t *ps, struct pid_on_target2 *pids)
{
    ebpf_process_stat_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        ebpf_process_stat_t *ps = &pids->ps;

        accumulator.exit_call += ps->exit_call;
        accumulator.release_call += ps->release_call;
        accumulator.create_process += ps->create_process;
        accumulator.create_thread += ps->create_thread;

        accumulator.task_err += ps->task_err;

        pids = pids->next;
    }

    ps->exit_call = (accumulator.exit_call >= ps->exit_call) ? accumulator.exit_call : ps->exit_call;
    ps->release_call = (accumulator.release_call >= ps->release_call) ? accumulator.release_call : ps->release_call;
    ps->create_process = (accumulator.create_process >= ps->create_process) ? accumulator.create_process : ps->create_process;
    ps->create_thread = (accumulator.create_thread >= ps->create_thread) ? accumulator.create_thread : ps->create_thread;

    ps->task_err = (accumulator.task_err >= ps->task_err) ? accumulator.task_err : ps->task_err;
}

/*
 * Send Specific Process data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 * @param em   the structure with thread information
 */
static void ebpf_send_specific_process_data(char *type, ebpf_process_stat_t *values, ebpf_module_t *em)
{
    write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_PROCESS);
    write_chart_dimension(process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK].name,
                          (long long) values->create_process);
    write_end_chart();

    write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_THREAD);
    write_chart_dimension(process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_CLONE].name,
                          (long long) values->create_thread);
    write_end_chart();

    write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_EXIT);
    write_chart_dimension(process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT].name,
                          (long long) values->release_call);
    write_end_chart();

    write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_CLOSE);
    write_chart_dimension(process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].name,
                          (long long) values->release_call);
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_ERROR);
        write_chart_dimension(process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT].name,
                              (long long) values->task_err);
        write_end_chart();
    }
}

/**
 * Create specific process charts
 *
 * Create charts for cgroup/application
 *
 * @param type the chart type.
 * @param em   the structure with thread information
 */
static void ebpf_create_specific_process_charts(char *type, ebpf_module_t *em)
{
    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_TASK_PROCESS, "Process started",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_CGROUP_GROUP,
                      NETDATA_CGROUP_PROCESS_CREATE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5000,
                      ebpf_create_global_dimension, &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_TASK_THREAD, "Threads started",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_CGROUP_GROUP,
                      NETDATA_CGROUP_THREAD_CREATE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5001,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_CLONE],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_TASK_EXIT, "Tasks starts exit process.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_CGROUP_GROUP,
                      NETDATA_CGROUP_PROCESS_EXIT_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5002,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_TASK_CLOSE, "Tasks closed",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_CGROUP_GROUP,
                      NETDATA_CGROUP_PROCESS_CLOSE_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5003,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(type, NETDATA_SYSCALL_APPS_TASK_ERROR, "Errors to create process or threads.",
                          EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_CGROUP_GROUP,
                          NETDATA_CGROUP_PROCESS_ERROR_CONTEXT,
                          NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5004,
                          ebpf_create_global_dimension,
                          &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
                          1, em->update_every, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }
}

/**
 * Obsolete specific process charts
 *
 * Obsolete charts for cgroup/application
 *
 * @param type the chart type.
 * @param em   the structure with thread information
 */
static void ebpf_obsolete_specific_process_charts(char *type, ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_TASK_PROCESS, "Process started",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                              NETDATA_CGROUP_PROCESS_CREATE_CONTEXT, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5000,
                              em->update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_TASK_THREAD, "Threads started",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                              NETDATA_CGROUP_THREAD_CREATE_CONTEXT, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5001,
                              em->update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_TASK_EXIT,"Tasks starts exit process.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                              NETDATA_CGROUP_PROCESS_EXIT_CONTEXT, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5002,
                              em->update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_TASK_CLOSE,"Tasks closed",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                              NETDATA_CGROUP_PROCESS_CLOSE_CONTEXT, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5003,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_TASK_ERROR,"Errors to create process or threads.",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_PROCESS_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                                  NETDATA_CGROUP_PROCESS_ERROR_CONTEXT, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5004,
                                  em->update_every);
    }
}

/**
 *  Create Systemd process Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param em   the structure with thread information
 **/
static void ebpf_create_systemd_process_charts(ebpf_module_t *em)
{
    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_TASK_PROCESS, "Process started",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_PROCESS_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20065,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_PROCESS_CREATE_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_PROCESS, em->update_every);

    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_TASK_THREAD, "Threads started",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_PROCESS_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20066,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_THREAD_CREATE_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_PROCESS, em->update_every);

    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_TASK_CLOSE, "Tasks starts exit process.",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_PROCESS_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20067,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_PROCESS_EXIT_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_PROCESS, em->update_every);

    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_TASK_EXIT, "Tasks closed",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_PROCESS_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20068,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_PROCESS_CLOSE_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_PROCESS, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_TASK_ERROR, "Errors to create process or threads.",
                                      EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_PROCESS_GROUP,
                                      NETDATA_EBPF_CHART_TYPE_STACKED, 20069,
                                      ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_PROCESS_ERROR_CONTEXT,
                                      NETDATA_EBPF_MODULE_NAME_PROCESS, em->update_every);
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 *
 *  @param em   the structure with thread information
 *
 * @return It returns the status for chart creation, if it is necessary to remove a specific dimension, zero is returned
 *         otherwise function returns 1 to avoid chart recreation
 */
static int ebpf_send_systemd_process_charts(ebpf_module_t *em)
{
    int ret = 1;
    ebpf_cgroup_target_t *ect;
    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_TASK_PROCESS);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_ps.create_process);
        } else
            ret = 0;
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_TASK_THREAD);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_ps.create_thread);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_TASK_EXIT);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_ps.exit_call);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_TASK_CLOSE);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_ps.release_call);
        }
    }
    write_end_chart();

    if (em->mode < MODE_ENTRY) {
        write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_TASK_ERROR);
        for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
            if (unlikely(ect->systemd) && unlikely(ect->updated)) {
                write_chart_dimension(ect->name, ect->publish_systemd_ps.task_err);
            }
        }
        write_end_chart();
    }

    return ret;
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em   the structure with thread information
*/
static void ebpf_process_send_cgroup_data(ebpf_module_t *em)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_process_sum_cgroup_pids(&ect->publish_systemd_ps, ect->pids);
    }

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;

    if (has_systemd) {
        static int systemd_chart = 0;
        if (!systemd_chart) {
            ebpf_create_systemd_process_charts(em);
            systemd_chart = 1;
        }

        systemd_chart = ebpf_send_systemd_process_charts(em);
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART) && ect->updated) {
            ebpf_create_specific_process_charts(ect->name, em);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART) {
            if (ect->updated) {
                ebpf_send_specific_process_data(ect->name, &ect->publish_systemd_ps, em);
            } else {
                ebpf_obsolete_specific_process_charts(ect->name, em);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Update Cgroup algorithm
 *
 * Change algorithm from absolute to incremental
 */
void ebpf_process_update_cgroup_algorithm()
{
    int i;
    for (i = 0; i < NETDATA_KEY_PUBLISH_PROCESS_END; i++)  {
        netdata_publish_syscall_t *ptr = &process_publish_aggregated[i];
        freez(ptr->algorithm);
        ptr->algorithm = strdupz(ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
    }
}

/**
 * Send Statistic Data
 *
 * Send statistic information to netdata.
 */
void ebpf_send_statistic_data()
{
    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_THREADS);
    write_chart_dimension(threads_stat[NETDATA_EBPF_THREAD_STAT_TOTAL], (long long)plugin_statistics.threads);
    write_chart_dimension(threads_stat[NETDATA_EBPF_THREAD_STAT_RUNNING], (long long)plugin_statistics.running);
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_LOAD_METHOD);
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY], (long long)plugin_statistics.legacy);
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE], (long long)plugin_statistics.core);
    write_end_chart();
}

/**
 * Main loop for this collector.
 *
 * @param em   the structure with thread information
 */
static void process_collector(ebpf_module_t *em)
{
    cgroup_thread.thread = mallocz(sizeof(netdata_thread_t));
    cgroup_thread.start_routine = ebpf_cgroup_update_shm;

    netdata_thread_create(cgroup_thread.thread, cgroup_thread.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_cgroup_update_shm, em);

    heartbeat_t hb;
    heartbeat_init(&hb);
    int publish_global = em->global_charts;
    int apps_enabled = em->apps_charts;
    int cgroups = em->cgroup_charts;
    int thread_enabled = em->enabled;
    if (cgroups)
        ebpf_process_update_cgroup_algorithm();

    int pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;
    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, USEC_PER_SEC);
        (void)dt;

        pthread_mutex_lock(&collect_data_mutex);
        cleanup_exited_pids();
        collect_data_for_all_processes(pid_fd);

        ebpf_create_apps_charts(apps_groups_root_target);

        pthread_cond_broadcast(&collect_data_cond_var);
        pthread_mutex_unlock(&collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;

            read_hash_global_tables();

            int publish_apps = 0;
            if (all_pids_count > 0) {
                if (apps_enabled) {
                    publish_apps = 1;
                    ebpf_process_update_apps_data();
                }

                if (cgroups) {
                    ebpf_update_process_cgroup();
                }
            }

            pthread_mutex_lock(&lock);
            ebpf_send_statistic_data();

            if (thread_enabled) {
                if (publish_global) {
                    ebpf_process_send_data(em);
                }

                if (publish_apps) {
                    ebpf_process_send_apps_data(apps_groups_root_target, em);
                }

                if (cgroups) {
                    ebpf_process_send_cgroup_data(em);
                }
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

void clean_global_memory() {
    int pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        uint32_t pid = pids->pid;
        freez(global_process_stats[pid]);

        bpf_map_delete_elem(pid_fd, &pid);
        freez(current_apps_data[pid]);

        pids = pids->next;
    }
}

/**
 * Process disable tracepoints
 *
 * Disable tracepoints when the plugin was responsible to enable it.
 */
static void ebpf_process_disable_tracepoints()
{
    char *default_message = { "Cannot disable the tracepoint" };
    if (!was_sched_process_exit_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exit))
            error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_exit);
    }

    if (!was_sched_process_exec_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exec))
            error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_exec);
    }

    if (!was_sched_process_fork_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_fork))
            error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_fork);
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_process_cleanup(void *ptr)
{
    UNUSED(ptr);

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick =  1 * USEC_PER_SEC;
    while (!finalized_threads) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    ebpf_cleanup_publish_syscall(process_publish_aggregated);
    freez(process_hash_values);

    clean_global_memory();
    freez(global_process_stats);
    freez(current_apps_data);

    ebpf_process_disable_tracepoints();

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
    }

    freez(cgroup_thread.thread);
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
    memset(process_aggregated_data, 0, length * sizeof(netdata_syscall_stat_t));
    memset(process_publish_aggregated, 0, length * sizeof(netdata_publish_syscall_t));
    process_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    global_process_stats = callocz((size_t)pid_max, sizeof(ebpf_process_stat_t *));
    current_apps_data = callocz((size_t)pid_max, sizeof(ebpf_process_publish_apps_t *));
}

static void change_syscalls()
{
    static char *lfork = { "do_fork" };
    process_id_names[NETDATA_KEY_PUBLISH_PROCESS_FORK] = lfork;
}

/**
 * Set local variables
 *
 */
static void set_local_pointers()
{
    if (isrh >= NETDATA_MINIMUM_RH_VERSION && isrh < NETDATA_RH_8)
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
 * Enable tracepoints
 *
 * Enable necessary tracepoints for thread.
 *
 * @return  It returns 0 on success and -1 otherwise
 */
static int ebpf_process_enable_tracepoints()
{
    int test = ebpf_is_tracepoint_enabled(tracepoint_sched_type, tracepoint_sched_process_exit);
    if (test == -1)
        return -1;
    else if (!test) {
        if (ebpf_enable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exit))
            return -1;
    }
    was_sched_process_exit_enabled = test;

    test = ebpf_is_tracepoint_enabled(tracepoint_sched_type, tracepoint_sched_process_exec);
    if (test == -1)
        return -1;
    else if (!test) {
        if (ebpf_enable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exec))
            return -1;
    }
    was_sched_process_exec_enabled = test;

    test = ebpf_is_tracepoint_enabled(tracepoint_sched_type, tracepoint_sched_process_fork);
    if (test == -1)
        return -1;
    else if (!test) {
        if (ebpf_enable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_fork))
            return -1;
    }
    was_sched_process_fork_enabled = test;

    return 0;
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
    em->maps = process_maps;

    if (ebpf_process_enable_tracepoints()) {
        em->enabled = em->global_charts = em->apps_charts = em->cgroup_charts =  CONFIG_BOOLEAN_NO;
    }
    process_enabled = em->enabled;

    pthread_mutex_lock(&lock);
    ebpf_process_allocate_global_vectors(NETDATA_KEY_PUBLISH_PROCESS_END);

    ebpf_update_pid_table(&process_maps[0], em);

    set_local_pointers();
    probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &objects);
    if (!probe_links) {
        em->enabled = CONFIG_BOOLEAN_NO;
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    int algorithms[NETDATA_KEY_PUBLISH_PROCESS_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(
        process_aggregated_data, process_publish_aggregated, process_dimension_names, process_id_names,
        algorithms, NETDATA_KEY_PUBLISH_PROCESS_END);

    if (process_enabled) {
        ebpf_create_global_charts(em);
    }

    ebpf_update_stats(&plugin_statistics, em);
    ebpf_create_statistic_charts(em);

    pthread_mutex_unlock(&lock);

    process_collector(em);

endprocess:
    if (!em->enabled)
        ebpf_update_disabled_plugin_stats(em);

    wait_for_all_threads_die();
    netdata_thread_cleanup_pop(1);
    return NULL;
}
