// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_process.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *process_dimension_names[NETDATA_KEY_PUBLISH_PROCESS_END] = {"process", "task", "process", "thread"};
static char *process_id_names[NETDATA_KEY_PUBLISH_PROCESS_END] = {"do_exit", "release_task", "_do_fork", "sys_clone"};
static char *status[] = {"process", "zombie"};

static ebpf_local_maps_t process_maps[] = {
    {.name = "tbl_pid_stats",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tbl_total_stats",
     .internal_input = NETDATA_KEY_END_VECTOR,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "process_ctrl",
     .internal_input = NETDATA_CONTROLLER_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

char *tracepoint_sched_type = {"sched"};
char *tracepoint_sched_process_exit = {"sched_process_exit"};
char *tracepoint_sched_process_exec = {"sched_process_exec"};
char *tracepoint_sched_process_fork = {"sched_process_fork"};
static int was_sched_process_exit_enabled = 0;
static int was_sched_process_exec_enabled = 0;
static int was_sched_process_fork_enabled = 0;

static netdata_idx_t *process_hash_values = NULL;
ebpf_process_stat_t *process_stat_vector = NULL;
static netdata_syscall_stat_t process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_END];
static netdata_publish_syscall_t process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_END];

struct config process_config = APPCONFIG_INITIALIZER;

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
    netdata_publish_syscall_t *publish,
    netdata_publish_vfs_common_t *pvc,
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

    pvc->running =
        (long)publish[NETDATA_KEY_PUBLISH_PROCESS_FORK].ncall - (long)publish[NETDATA_KEY_PUBLISH_PROCESS_CLONE].ncall;
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
    ebpf_write_begin_chart(family, NETDATA_PROCESS_STATUS_NAME, "");

    write_chart_dimension(status[0], (long long)pvc->running);
    write_chart_dimension(status[1], (long long)pvc->zombie);

    ebpf_write_end_chart();
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

    write_count_chart(
        NETDATA_EXIT_SYSCALL,
        NETDATA_EBPF_SYSTEM_GROUP,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
        2);
    write_count_chart(
        NETDATA_PROCESS_SYSCALL,
        NETDATA_EBPF_SYSTEM_GROUP,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
        2);

    write_status_chart(NETDATA_EBPF_SYSTEM_GROUP, &pvc);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_PROCESS_ERROR_NAME,
            NETDATA_EBPF_SYSTEM_GROUP,
            &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
            2);
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
 */
void ebpf_process_send_apps_data(struct ebpf_target *root, ebpf_module_t *em)
{
    struct ebpf_target *w;

    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_PROCESS_IDX))))
            continue;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_process_start");
        write_chart_dimension("calls", w->process.create_process);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_thread_start");
        write_chart_dimension("calls", w->process.create_thread);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_task_exit");
        write_chart_dimension("calls", w->process.exit_call);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_task_released");
        write_chart_dimension("calls", w->process.release_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_task_error");
            write_chart_dimension("calls", w->process.task_err);
            ebpf_write_end_chart();
        }
    }
}

/*****************************************************************
 *
 *  READ INFORMATION FROM KERNEL RING
 *
 *****************************************************************/

/**
 * Read the hash table and store data to allocated vectors.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_process_hash_global_tables(netdata_idx_t *stats, int maps_per_core)
{
    netdata_idx_t res[NETDATA_KEY_END_VECTOR];
    ebpf_read_global_table_stats(
        res,
        process_hash_values,
        process_maps[NETDATA_PROCESS_GLOBAL_TABLE].map_fd,
        maps_per_core,
        0,
        NETDATA_KEY_END_VECTOR);

    ebpf_read_global_table_stats(
        stats,
        process_hash_values,
        process_maps[NETDATA_PROCESS_CTRL_TABLE].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);

    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_EXIT].call = res[NETDATA_KEY_CALLS_DO_EXIT];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].call = res[NETDATA_KEY_CALLS_RELEASE_TASK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_FORK].call = res[NETDATA_KEY_CALLS_DO_FORK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_CLONE].call = res[NETDATA_KEY_CALLS_SYS_CLONE];

    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_FORK].ecall = res[NETDATA_KEY_ERROR_DO_FORK];
    process_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_CLONE].ecall = res[NETDATA_KEY_ERROR_SYS_CLONE];
}

/**
 * Update cgroup
 *
 * Update cgroup data based in PID running.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_process_cgroup()
{
    ebpf_cgroup_target_t *ect;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            ebpf_publish_process_t *out = &pids->ps;
            ebpf_pid_data_t *local_pid = ebpf_get_pid_data(pid, 0, NULL, NETDATA_EBPF_PIDS_PROCESS_IDX);
            ebpf_publish_process_t *in = local_pid->process;
            if (!in)
                continue;

            memcpy(out, in, sizeof(ebpf_publish_process_t));
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
static void
ebpf_process_status_chart(char *family, char *name, char *axis, char *web, char *algorithm, int order, int update_every)
{
    printf(
        "CHART %s.%s '' 'Process not closed' '%s' '%s' 'system.process_status' line %d %d '' 'ebpf.plugin' 'process'\n",
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
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_PROCESS_SYSCALL,
        "Start process",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        "system.process_thread",
        NETDATA_EBPF_CHART_TYPE_LINE,
        21002,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
        2,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_EXIT_SYSCALL,
        "Exit process",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        "system.exit",
        NETDATA_EBPF_CHART_TYPE_LINE,
        21003,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
        2,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_process_status_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_PROCESS_STATUS_NAME,
        EBPF_COMMON_UNITS_CALLS,
        NETDATA_PROCESS_GROUP,
        ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
        21004,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            NETDATA_EBPF_SYSTEM_GROUP,
            NETDATA_PROCESS_ERROR_NAME,
            "Fails to create process",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            "system.task_error",
            NETDATA_EBPF_CHART_TYPE_LINE,
            21005,
            ebpf_create_global_dimension,
            &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
            2,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
    }

    fflush(stdout);
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
    struct ebpf_target *root = ptr;
    struct ebpf_target *w;
    int update_every = em->update_every;
    for (w = root; w; w = w->next) {
        if (unlikely(!w->exposed))
            continue;

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_process_start",
            "Process started.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_process_start",
            20161,
            update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_thread_start",
            "Threads started.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_thread_start",
            20162,
            update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_task_exit",
            "Tasks starts exit process.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_task_exit",
            20163,
            update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_task_released",
            "Tasks released.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_task_released",
            20164,
            update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_task_error",
                "Errors to create process or threads.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_PROCESS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_task_error",
                20165,
                update_every,
                NETDATA_EBPF_MODULE_NAME_PROCESS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }
        w->charts_created |= 1 << EBPF_MODULE_PROCESS_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

static void ebpf_obsolete_specific_process_charts(char *type, ebpf_module_t *em);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_process_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_TASK_PROCESS,
        "",
        "Process started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_PROCESS_CREATE_CONTEXT,
        20065,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_TASK_THREAD,
        "",
        "Threads started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_THREAD_CREATE_CONTEXT,
        20066,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_TASK_CLOSE,
        "",
        "Tasks starts exit process.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_PROCESS_EXIT_CONTEXT,
        20067,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_TASK_EXIT,
        "",
        "Tasks closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_PROCESS_CLOSE_CONTEXT,
        20068,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_TASK_ERROR,
            "",
            "Errors to create process or threads.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_PROCESS_ERROR_CONTEXT,
            20069,
            em->update_every);
    }
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_process_cgroup_charts(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_process_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_process_charts(ect->name, em);
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Obsolette apps charts
 *
 * Obsolete apps charts.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_obsolete_process_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_PROCESS_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_process_start",
            "Process started.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_process_start",
            20161,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_thread_start",
            "Threads started.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_thread_start",
            20162,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_task_exit",
            "Tasks starts exit process.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_task_exit",
            20163,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_task_released",
            "Tasks released.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_task_released",
            20164,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_task_error",
                "Errors to create process or threads.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_PROCESS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_task_error",
                20165,
                update_every);
        }

        w->charts_created &= ~(1 << EBPF_MODULE_PROCESS_IDX);
    }
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_process_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_PROCESS_SYSCALL,
        "",
        "Start process",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "system.process_thread",
        21002,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_EXIT_SYSCALL,
        "",
        "Exit process",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "system.exit",
        21003,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_PROCESS_STATUS_NAME,
        "",
        "Process not closed",
        EBPF_COMMON_UNITS_CALLS,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "system.process_status",
        21004,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            NETDATA_EBPF_SYSTEM_GROUP,
            NETDATA_PROCESS_ERROR_NAME,
            "",
            "Fails to create process",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "system.task_error",
            21005,
            em->update_every);
    }
}

/**
 * Process disable tracepoints
 *
 * Disable tracepoints when the plugin was responsible to enable it.
 */
static void ebpf_process_disable_tracepoints()
{
    char *default_message = {"Cannot disable the tracepoint"};
    if (!was_sched_process_exit_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exit))
            netdata_log_error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_exit);
    }

    if (!was_sched_process_exec_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_exec))
            netdata_log_error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_exec);
    }

    if (!was_sched_process_fork_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_sched_type, tracepoint_sched_process_fork))
            netdata_log_error("%s %s/%s.", default_message, tracepoint_sched_type, tracepoint_sched_process_fork);
    }
}

/**
 * Process Exit
 *
 * Cancel child thread.
 *
 * @param ptr thread data.
 */
static void ebpf_process_exit(void *pptr)
{
    pids_fd[NETDATA_EBPF_PIDS_PROCESS_IDX] = -1;
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    pthread_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_PROCESS_IDX);
    pthread_mutex_unlock(&lock);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_process_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_process_apps_charts(em);
        }

        ebpf_obsolete_process_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    freez(process_hash_values);
    freez(process_stat_vector);

    ebpf_process_disable_tracepoints();

    pthread_mutex_lock(&ebpf_exit_cleanup);
    process_pid_fd = -1;
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param ps  structure used to store data
 * @param pids input data
 */
static void ebpf_process_sum_cgroup_pids(ebpf_publish_process_t *ps, struct pid_on_target2 *pids)
{
    ebpf_publish_process_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        ebpf_publish_process_t *pps = &pids->ps;

        accumulator.exit_call += pps->exit_call;
        accumulator.release_call += pps->release_call;
        accumulator.create_process += pps->create_process;
        accumulator.create_thread += pps->create_thread;

        accumulator.task_err += pps->task_err;

        pids = pids->next;
    }

    ps->exit_call = (accumulator.exit_call >= ps->exit_call) ? accumulator.exit_call : ps->exit_call;
    ps->release_call = (accumulator.release_call >= ps->release_call) ? accumulator.release_call : ps->release_call;
    ps->create_process =
        (accumulator.create_process >= ps->create_process) ? accumulator.create_process : ps->create_process;
    ps->create_thread =
        (accumulator.create_thread >= ps->create_thread) ? accumulator.create_thread : ps->create_thread;

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
static void ebpf_send_specific_process_data(char *type, ebpf_publish_process_t *values, ebpf_module_t *em)
{
    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_PROCESS, "");
    write_chart_dimension(
        process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK].name, (long long)values->create_process);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_THREAD, "");
    write_chart_dimension(
        process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_CLONE].name, (long long)values->create_thread);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_EXIT, "");
    write_chart_dimension(
        process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT].name, (long long)values->release_call);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_CLOSE, "");
    write_chart_dimension(
        process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK].name, (long long)values->release_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_TASK_ERROR, "");
        write_chart_dimension(
            process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT].name, (long long)values->task_err);
        ebpf_write_end_chart();
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
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_TASK_PROCESS,
        "Process started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_CGROUP_PROCESS_CREATE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5000,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_FORK],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_TASK_THREAD,
        "Threads started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_CGROUP_THREAD_CREATE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5001,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_CLONE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_TASK_EXIT,
        "Tasks starts exit process.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_CGROUP_PROCESS_EXIT_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5002,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_TASK_CLOSE,
        "Tasks closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_CGROUP_PROCESS_CLOSE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5003,
        ebpf_create_global_dimension,
        &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_TASK_ERROR,
            "Errors to create process or threads.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_CGROUP_PROCESS_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5004,
            ebpf_create_global_dimension,
            &process_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_EXIT],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_PROCESS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
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
    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_TASK_PROCESS,
        "",
        "Process started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_PROCESS_CREATE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5000,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_TASK_THREAD,
        "",
        "Threads started",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_THREAD_CREATE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5001,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_TASK_EXIT,
        "",
        "Tasks starts exit process.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_PROCESS_EXIT_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5002,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_TASK_CLOSE,
        "",
        "Tasks closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_PROCESS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_PROCESS_CLOSE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5003,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_TASK_ERROR,
            "",
            "Errors to create process or threads.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_PROCESS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_PROCESS_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5004,
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
    static ebpf_systemd_args_t data_process = {
        .title = "Process started",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_PROCESS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20065,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_PROCESS_CREATE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_PROCESS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_TASK_PROCESS,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_thread = {
        .title = "Threads started",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_PROCESS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20066,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_THREAD_CREATE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_PROCESS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_TASK_THREAD,
        .dimension = "calls"};

    static ebpf_systemd_args_t task_exit = {
        .title = "Tasks starts exit process.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_PROCESS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20067,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_PROCESS_EXIT_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_PROCESS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_TASK_CLOSE,
        .dimension = "calls"};

    static ebpf_systemd_args_t task_closed = {
        .title = "Tasks closed",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_PROCESS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20068,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_PROCESS_CLOSE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_PROCESS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_TASK_EXIT,
        .dimension = "calls"};

    static ebpf_systemd_args_t task_error = {
        .title = "Errors to create process or threads.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_PROCESS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20069,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_PROCESS_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_PROCESS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_TASK_ERROR,
        .dimension = "calls"};

    ebpf_cgroup_target_t *w;
    netdata_run_mode_t mode = em->mode;
    if (!task_exit.update_every)
        data_process.update_every = data_thread.update_every = task_exit.update_every = task_closed.update_every =
            task_error.update_every = em->update_every;

    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_PROCESS_CHART))
            continue;

        data_process.id = data_thread.id = task_exit.id = task_closed.id = task_error.id = w->name;
        ebpf_create_charts_on_systemd(&data_process);

        ebpf_create_charts_on_systemd(&data_thread);

        ebpf_create_charts_on_systemd(&task_exit);

        ebpf_create_charts_on_systemd(&task_closed);
        if (mode < MODE_ENTRY) {
            ebpf_create_charts_on_systemd(&task_error);
        }
        w->flags |= NETDATA_EBPF_SERVICES_HAS_PROCESS_CHART;
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 *
 *  @param em   the structure with thread information
 */
static void ebpf_send_systemd_process_charts(ebpf_module_t *em)
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_PROCESS_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_TASK_PROCESS, "");
        write_chart_dimension("calls", ect->publish_systemd_ps.create_process);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_TASK_THREAD, "");
        write_chart_dimension("calls", ect->publish_systemd_ps.create_thread);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_TASK_EXIT, "");
        write_chart_dimension("calls", ect->publish_systemd_ps.exit_call);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_TASK_CLOSE, "");
        write_chart_dimension("calls", ect->publish_systemd_ps.release_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_TASK_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_ps.task_err);
            ebpf_write_end_chart();
        }
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em   the structure with thread information
*/
static void ebpf_process_send_cgroup_data(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_process_sum_cgroup_pids(&ect->publish_systemd_ps, ect->pids);
    }

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_process_charts(em);
        }

        ebpf_send_systemd_process_charts(em);
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
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
    for (i = 0; i < NETDATA_KEY_PUBLISH_PROCESS_END; i++) {
        netdata_publish_syscall_t *ptr = &process_publish_aggregated[i];
        ptr->algorithm = ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX];
    }
}

/**
 * Main loop for this collector.
 *
 * @param em   the structure with thread information
 */
static void process_collector(ebpf_module_t *em)
{
    int publish_global = em->global_charts;
    int cgroups = em->cgroup_charts;
    pthread_mutex_lock(&ebpf_exit_cleanup);
    process_pid_fd = process_maps[NETDATA_PROCESS_PID_TABLE].map_fd;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
    if (cgroups)
        ebpf_process_update_cgroup_algorithm();

    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop())
            break;

        if (++counter == update_every) {
            counter = 0;

            ebpf_read_process_hash_global_tables(stats, maps_per_core);

            netdata_apps_integration_flags_t apps_enabled = em->apps_charts;
            pthread_mutex_lock(&collect_data_mutex);

            if (ebpf_all_pids_count > 0) {
                if (cgroups && shm_ebpf_cgroup.header) {
                    ebpf_update_process_cgroup();
                }
            }

            pthread_mutex_lock(&lock);

            if (publish_global) {
                ebpf_process_send_data(em);
            }

            if (apps_enabled & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
                ebpf_process_send_apps_data(apps_groups_root_target, em);
            }

            if (cgroups && shm_ebpf_cgroup.header) {
                ebpf_process_send_cgroup_data(em);
            }

            pthread_mutex_unlock(&lock);
            pthread_mutex_unlock(&collect_data_mutex);

            pthread_mutex_lock(&ebpf_exit_cleanup);
            if (running_time && !em->running_time)
                running_time = update_every;
            else
                running_time += update_every;

            em->running_time = running_time;
            pthread_mutex_unlock(&ebpf_exit_cleanup);
        }

        fflush(stdout);
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
    process_stat_vector = callocz(ebpf_nprocs, sizeof(ebpf_process_stat_t));
}

static void change_syscalls()
{
    static char *lfork = {"do_fork"};
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
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_process_exit) cleanup_ptr = em;

    em->maps = process_maps;

    pthread_mutex_lock(&ebpf_exit_cleanup);
    if (ebpf_process_enable_tracepoints()) {
        em->enabled = em->global_charts = em->apps_charts = em->cgroup_charts = NETDATA_THREAD_EBPF_STOPPING;
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    pthread_mutex_lock(&lock);
    ebpf_process_allocate_global_vectors(NETDATA_KEY_PUBLISH_PROCESS_END);

    ebpf_update_pid_table(&process_maps[0], em);

    set_local_pointers();
    em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
    if (!em->probe_links) {
        em->enabled = em->global_charts = em->apps_charts = em->cgroup_charts = NETDATA_THREAD_EBPF_STOPPING;
    }

    int algorithms[NETDATA_KEY_PUBLISH_PROCESS_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX};

    ebpf_global_labels(
        process_aggregated_data,
        process_publish_aggregated,
        process_dimension_names,
        process_id_names,
        algorithms,
        NETDATA_KEY_PUBLISH_PROCESS_END);

    ebpf_create_global_charts(em);

    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);

    pthread_mutex_unlock(&lock);

    process_collector(em);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    ebpf_update_disabled_plugin_stats(em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    return NULL;
}
