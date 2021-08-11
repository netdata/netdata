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

static netdata_idx_t *process_hash_values = NULL;
static netdata_syscall_stat_t process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_END];
static netdata_publish_syscall_t process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_END];

static ebpf_data_t process_data;

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
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t pvc;
    ebpf_update_global_publish(process_publish_aggregated, &pvc, process_aggregated_data);

    write_count_chart(NETDATA_EXIT_SYSCALL, NETDATA_EBPF_FAMILY,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT], 2);
    write_count_chart(NETDATA_PROCESS_SYSCALL, NETDATA_EBPF_FAMILY,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK], 2);

    write_status_chart(NETDATA_EBPF_FAMILY, &pvc);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_PROCESS_ERROR_NAME, NETDATA_EBPF_FAMILY,
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
 * Send data to Netdata calling auxiliar functions.
 *
 * @param root the target list.
 */
void ebpf_process_send_apps_data(struct target *root)
{
    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_PROCESS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, call_do_fork));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SYSCALL_APPS_TASK_THREAD);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_process_sum_values_for_pids(w->root_pid, offsetof(ebpf_process_publish_apps_t, call_sys_clone));
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
        cad->call_do_fork = ps->fork_call;
        cad->call_sys_clone = ps->clone_call;

        cad->ecall_do_fork = ps->fork_err;
        cad->ecall_sys_clone = ps->clone_err;

        pids = pids->next;
    }
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
 */
static void ebpf_process_status_chart(char *family, char *name, char *axis,
                                      char *web, char *algorithm, int order)
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
    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_PROCESS_SYSCALL,
                      "Start process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21002,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
                      2, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_EXIT_SYSCALL,
                      "Exit process",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_PROCESS_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21003,
                      ebpf_create_global_dimension,
                      &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
                      2, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_process_status_chart(NETDATA_EBPF_FAMILY,
                              NETDATA_PROCESS_STATUS_NAME,
                              EBPF_COMMON_DIMENSION_DIFFERENCE,
                              NETDATA_PROCESS_GROUP,
                              ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                              21004);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_PROCESS_ERROR_NAME,
                          "Fails to create process",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_PROCESS_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          21005,
                          ebpf_create_global_dimension,
                          &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
                          2, NETDATA_EBPF_MODULE_NAME_PROCESS);
    }
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
    UNUSED(em);
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_PROCESS,
                               "Process started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20065,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_THREAD,
                               "Threads started",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20066,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_charts_on_apps(NETDATA_SYSCALL_APPS_TASK_CLOSE,
                               "Tasks closed",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_PROCESS_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20067,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_PROCESS);
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
    int pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();

        pthread_mutex_lock(&collect_data_mutex);
        cleanup_exited_pids();
        collect_data_for_all_processes(pid_fd);

        ebpf_create_apps_charts(apps_groups_root_target);

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
            ebpf_process_send_apps_data(apps_groups_root_target);
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
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_process_cleanup(void *ptr)
{
    UNUSED(ptr);

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 50*USEC_PER_MS;
    while (!finalized_threads) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    ebpf_cleanup_publish_syscall(process_publish_aggregated);
    freez(process_hash_values);

    clean_global_memory();
    freez(global_process_stats);
    freez(current_apps_data);

    freez(process_data.map_fd);

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
    em->maps = process_maps;
    process_enabled = em->enabled;
    fill_ebpf_data(&process_data);

    pthread_mutex_lock(&lock);
    ebpf_process_allocate_global_vectors(NETDATA_KEY_PUBLISH_PROCESS_END);

    if (ebpf_update_kernel(&process_data)) {
        pthread_mutex_unlock(&lock);
        goto endprocess;
    }

    ebpf_update_pid_table(&process_maps[0], em);

    set_local_pointers();
    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, process_data.map_fd);
    if (!probe_links) {
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

    pthread_mutex_unlock(&lock);

    process_collector((usec_t)(em->update_time * USEC_PER_SEC), em);

endprocess:
    wait_for_all_threads_die();
    netdata_thread_cleanup_pop(1);
    return NULL;
}
