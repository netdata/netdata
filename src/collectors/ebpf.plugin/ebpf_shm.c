// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_shm.h"

static char *shm_dimension_name[NETDATA_SHM_END] = {"get", "at", "dt", "ctl"};
static netdata_syscall_stat_t shm_aggregated_data[NETDATA_SHM_END];
static netdata_publish_syscall_t shm_publish_aggregated[NETDATA_SHM_END];

netdata_ebpf_shm_t *shm_vector = NULL;

static netdata_idx_t shm_hash_values[NETDATA_SHM_END];
static netdata_idx_t *shm_values = NULL;

struct config shm_config = APPCONFIG_INITIALIZER;

static ebpf_local_maps_t shm_maps[] = {
    {.name = "tbl_pid_shm",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "shm_ctrl",
     .internal_input = NETDATA_CONTROLLER_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tbl_shm",
     .internal_input = NETDATA_SHM_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL, .internal_input = 0, .user_input = 0}};

netdata_ebpf_targets_t shm_targets[] = {
    {.name = "shmget", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "shmat", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "shmdt", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "shmctl", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef NETDATA_DEV_MODE
int shm_disable_priority;
#endif

struct netdata_static_thread ebpf_read_shm = {
    .name = "EBPF_READ_SHM",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL};

#ifdef LIBBPF_MAJOR_VERSION
/*****************************************************************
 *
 *  BTF FUNCTIONS
 *
 *****************************************************************/

/*
 * Disable tracepoint
 *
 * Disable all tracepoints to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_shm_disable_tracepoint(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmget, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmat, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmdt, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmctl, false);
}

/*
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_disable_probe(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_shmget_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmat_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmdt_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmctl_probe, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_disable_trampoline(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_shmget_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmat_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmdt_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmctl_fentry, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_set_trampoline_target(struct shm_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];
    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMGET_CALL].name, running_on_kernel);

    bpf_program__set_attach_target(obj->progs.netdata_shmget_fentry, 0, syscall);

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMAT_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmat_fentry, 0, syscall);

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMDT_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmdt_fentry, 0, syscall);

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMCTL_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmctl_fentry, 0, syscall);
}

/**
 * SHM Attach Probe
 *
 * Attach probes to target
 *
 * @param obj is the main structure for bpf objects.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_shm_attach_probe(struct shm_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];
    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMGET_CALL].name, running_on_kernel);

    obj->links.netdata_shmget_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmget_probe, false, syscall);
    long ret = libbpf_get_error(obj->links.netdata_shmget_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMAT_CALL].name, running_on_kernel);
    obj->links.netdata_shmat_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmat_probe, false, syscall);
    ret = libbpf_get_error(obj->links.netdata_shmat_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMDT_CALL].name, running_on_kernel);
    obj->links.netdata_shmdt_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmdt_probe, false, syscall);
    ret = libbpf_get_error(obj->links.netdata_shmdt_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, shm_targets[NETDATA_KEY_SHMCTL_CALL].name, running_on_kernel);
    obj->links.netdata_shmctl_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmctl_probe, false, syscall);
    ret = libbpf_get_error(obj->links.netdata_shmctl_probe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 */
static void ebpf_shm_set_hash_tables(struct shm_bpf *obj)
{
    shm_maps[NETDATA_PID_SHM_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_pid_shm);
    shm_maps[NETDATA_SHM_CONTROLLER].map_fd = bpf_map__fd(obj->maps.shm_ctrl);
    shm_maps[NETDATA_SHM_GLOBAL_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_shm);
}

/**
 * Adjust Map Size
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_shm_adjust_map(struct shm_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(
        obj->maps.tbl_pid_shm, &shm_maps[NETDATA_PID_SHM_TABLE], em, bpf_map__name(obj->maps.tbl_pid_shm));

    ebpf_update_map_type(obj->maps.tbl_shm, &shm_maps[NETDATA_SHM_GLOBAL_TABLE]);
    ebpf_update_map_type(obj->maps.tbl_pid_shm, &shm_maps[NETDATA_PID_SHM_TABLE]);
    ebpf_update_map_type(obj->maps.shm_ctrl, &shm_maps[NETDATA_SHM_CONTROLLER]);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 *
 * @return it returns 0 on success and -1 otherwise
 */
static inline int ebpf_shm_load_and_attach(struct shm_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *shmt = em->targets;
    netdata_ebpf_program_loaded_t test = shmt[NETDATA_KEY_SHMGET_CALL].mode;

    // We are testing only one, because all will have the same behavior
    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_shm_disable_tracepoint(obj);
        ebpf_disable_probe(obj);

        ebpf_set_trampoline_target(obj);
    } else if (test == EBPF_LOAD_PROBE || test == EBPF_LOAD_RETPROBE) {
        ebpf_shm_disable_tracepoint(obj);
        ebpf_disable_trampoline(obj);
    } else {
        ebpf_disable_probe(obj);
        ebpf_disable_trampoline(obj);
    }

    ebpf_shm_adjust_map(obj, em);

    int ret = shm_bpf__load(obj);
    if (!ret) {
        if (test != EBPF_LOAD_PROBE && test != EBPF_LOAD_RETPROBE)
            shm_bpf__attach(obj);
        else
            ret = ebpf_shm_attach_probe(obj);

        if (!ret)
            ebpf_shm_set_hash_tables(obj);
    }

    return ret;
}
#endif
/*****************************************************************
 *  FUNCTIONS TO CLOSE THE THREAD
 *****************************************************************/

static void ebpf_obsolete_specific_shm_charts(char *type, int update_every);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_shm_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_SHMGET_CHART,
        "",
        "Calls to syscall shmget(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_SHM_GET_CONTEXT,
        20191,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SHMAT_CHART,
        "",
        "Calls to syscall shmat(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_SHM_AT_CONTEXT,
        20192,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SHMDT_CHART,
        "",
        "Calls to syscall shmdt(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_SHM_DT_CONTEXT,
        20193,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SHMCTL_CHART,
        "",
        "Calls to syscall shmctl(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_SHM_CTL_CONTEXT,
        20194,
        em->update_every);
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_shm_cgroup_charts(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_shm_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_shm_charts(ect->name, em->update_every);
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
void ebpf_obsolete_shm_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SHM_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmget_call",
            "Calls to syscall shmget(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmget_call",
            20191,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmat_call",
            "Calls to syscall shmat(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmat_call",
            20192,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmdt_call",
            "Calls to syscall shmdt(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmdt_call",
            20193,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmctl_call",
            "Calls to syscall shmctl(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmctl_call",
            20194,
            update_every);

        w->charts_created &= ~(1 << EBPF_MODULE_SHM_IDX);
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_shm_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_SHM_GLOBAL_CHART,
        "",
        "Calls to shared memory system calls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_IPC_SHM_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "system.shared_memory_calls",
        NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_CALLS,
        em->update_every);
}

/**
 * SHM Exit
 *
 * Cancel child thread.
 *
 * @param ptr thread data.
 */
static void ebpf_shm_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    pthread_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_SHM_IDX);
    pthread_mutex_unlock(&lock);

    if (ebpf_read_shm.thread)
        nd_thread_signal_cancel(ebpf_read_shm.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_shm_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_shm_apps_charts(em);
        }

        ebpf_obsolete_shm_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (shm_bpf_obj) {
        shm_bpf__destroy(shm_bpf_obj);
        shm_bpf_obj = NULL;
    }
#endif

    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *  COLLECTOR THREAD
 *****************************************************************/

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 * @param maps_per_core do I need to read all cores?
 */
static void shm_apps_accumulator(netdata_ebpf_shm_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_ebpf_shm_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_ebpf_shm_t *w = &out[i];
        total->get += w->get;
        total->at += w->at;
        total->dt += w->dt;
        total->ctl += w->ctl;

        if (w->ct > ct)
            ct = w->ct;

        if (!total->name[0] && w->name[0])
            strncpyz(total->name, w->name, sizeof(total->name) - 1);
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_shm_cgroup()
{
    netdata_ebpf_shm_t *cv = shm_vector;
    size_t length = sizeof(netdata_publish_shm_t);

    ebpf_cgroup_target_t *ect;

    memset(cv, 0, length);

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t pid = pids->pid;
            netdata_publish_shm_t *out = &pids->shm;
            netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_SHM_IDX);
            if (!local_pid)
                continue;

            netdata_publish_shm_t *in = &local_pid->shm;

            memcpy(out, in, sizeof(netdata_publish_shm_t));
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_shm_apps_table(int maps_per_core)
{
    netdata_ebpf_shm_t *cv = shm_vector;
    int fd = shm_maps[NETDATA_PID_SHM_TABLE].map_fd;
    size_t length = sizeof(netdata_ebpf_shm_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, cv)) {
            goto end_shm_loop;
        }

        shm_apps_accumulator(cv, maps_per_core);

        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(key, NETDATA_EBPF_PIDS_SHM_IDX);
        if (!local_pid)
            continue;
        netdata_publish_shm_t *publish = &local_pid->shm;

        if (!publish->ct || publish->ct != cv->ct) {
            memcpy(publish, &cv[0], sizeof(netdata_publish_shm_t));
        } else {
            if (kill((pid_t)key, 0)) { // No PID found
                if (netdata_ebpf_reset_shm_pointer_unsafe(fd, key, NETDATA_EBPF_PIDS_SHM_IDX))
                    memset(publish, 0, sizeof(*publish));
            }
        }

    end_shm_loop:
        // now that we've consumed the value, zero it out in the map.
        memset(cv, 0, length);
        bpf_map_update_elem(fd, &key, cv, BPF_EXIST);

        key = next_key;
    }
}

/**
* Send global charts to netdata agent.
*/
static void shm_send_global()
{
    ebpf_write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, NETDATA_SHM_GLOBAL_CHART, "");
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL].dimension, (long long)shm_hash_values[NETDATA_KEY_SHMGET_CALL]);
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL].dimension, (long long)shm_hash_values[NETDATA_KEY_SHMAT_CALL]);
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL].dimension, (long long)shm_hash_values[NETDATA_KEY_SHMDT_CALL]);
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL].dimension, (long long)shm_hash_values[NETDATA_KEY_SHMCTL_CALL]);
    ebpf_write_end_chart();
}

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_shm_read_global_table(netdata_idx_t *stats, int maps_per_core)
{
    ebpf_read_global_table_stats(
        shm_hash_values,
        shm_values,
        shm_maps[NETDATA_SHM_GLOBAL_TABLE].map_fd,
        maps_per_core,
        NETDATA_KEY_SHMGET_CALL,
        NETDATA_SHM_END);

    ebpf_read_global_table_stats(
        stats,
        shm_values,
        shm_maps[NETDATA_SHM_CONTROLLER].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);
}

/**
 * Sum values for all targets.
 */
static void ebpf_shm_sum_pids(netdata_publish_shm_t *shm, struct ebpf_pid_on_target *root)
{
    memset(shm, 0, sizeof(netdata_publish_shm_t));
    for (; root; root = root->next) {
        uint32_t pid = root->pid;
        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_SHM_IDX);
        if (!local_pid)
            continue;

        netdata_publish_shm_t *w = &local_pid->shm;

        shm->get += w->get;
        shm->at += w->at;
        shm->dt += w->dt;
        shm->ctl += w->ctl;
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_shm_send_apps_data(struct ebpf_target *root)
{
    struct ebpf_target *w;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SHM_IDX))))
            continue;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_shmget_call");
        write_chart_dimension("calls", (long long)w->shm.get);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_shmat_call");
        write_chart_dimension("calls", (long long)w->shm.at);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_shmdt_call");
        write_chart_dimension("calls", (long long)w->shm.dt);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_shmctl_call");
        write_chart_dimension("calls", (long long)w->shm.ctl);
        ebpf_write_end_chart();
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Sum values for all targets.
 */
static void ebpf_shm_sum_cgroup_pids(netdata_publish_shm_t *shm, struct pid_on_target2 *root)
{
    netdata_publish_shm_t shmv;
    memset(&shmv, 0, sizeof(shmv));
    while (root) {
        netdata_publish_shm_t *w = &root->shm;
        shmv.get += w->get;
        shmv.at += w->at;
        shmv.dt += w->dt;
        shmv.ctl += w->ctl;

        root = root->next;
    }

    memcpy(shm, &shmv, sizeof(shmv));
}

/**
 * Create specific shared memory charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_shm_charts(char *type, int update_every)
{
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_SHMGET_CHART,
        "Calls to syscall shmget(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_CGROUP_SHM_GET_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5800,
        ebpf_create_global_dimension,
        &shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SHM);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SHMAT_CHART,
        "Calls to syscall shmat(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_CGROUP_SHM_AT_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5801,
        ebpf_create_global_dimension,
        &shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SHM);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SHMDT_CHART,
        "Calls to syscall shmdt(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_CGROUP_SHM_DT_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5802,
        ebpf_create_global_dimension,
        &shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SHM);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SHMCTL_CHART,
        "Calls to syscall shmctl(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_CGROUP_SHM_CTL_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5803,
        ebpf_create_global_dimension,
        &shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SHM);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
}

/**
 * Obsolete specific shared memory charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_shm_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_SHMGET_CHART,
        "",
        "Calls to syscall shmget(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SHM_GET_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5800,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SHMAT_CHART,
        "",
        "Calls to syscall shmat(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SHM_AT_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5801,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SHMDT_CHART,
        "",
        "Calls to syscall shmdt(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SHM_DT_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5802,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SHMCTL_CHART,
        "",
        "Calls to syscall shmctl(2).",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_IPC_SHM_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SHM_CTL_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5803,
        update_every);
}

/**
 *  Create Systemd Swap Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_shm_charts(int update_every)
{
    static ebpf_systemd_args_t data_shmget = {
        .title = "Calls to syscall shmget(2).",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_IPC_SHM_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20191,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SHM_GET_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SHM,
        .update_every = 0,
        .suffix = NETDATA_SHMGET_CHART,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_shmat = {
        .title = "Calls to syscall shmat(2).",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_IPC_SHM_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20192,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SHM_AT_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SHM,
        .update_every = 0,
        .suffix = NETDATA_SHMAT_CHART,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_shmdt = {
        .title = "Calls to syscall shmdt(2).",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_IPC_SHM_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20193,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SHM_DT_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SHM,
        .update_every = 0,
        .suffix = NETDATA_SHMDT_CHART,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_shmctl = {
        .title = "Calls to syscall shmctl(2).",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_IPC_SHM_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20194,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SHM_CTL_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SHM,
        .update_every = 0,
        .suffix = NETDATA_SHMCTL_CHART,
        .dimension = "calls"};

    if (!data_shmget.update_every)
        data_shmat.update_every = data_shmctl.update_every = data_shmdt.update_every = data_shmget.update_every =
            update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_SHM_CHART))
            continue;

        data_shmat.id = data_shmctl.id = data_shmdt.id = data_shmget.id = w->name;
        ebpf_create_charts_on_systemd(&data_shmat);

        ebpf_create_charts_on_systemd(&data_shmctl);

        ebpf_create_charts_on_systemd(&data_shmdt);

        ebpf_create_charts_on_systemd(&data_shmget);

        w->flags |= NETDATA_EBPF_SERVICES_HAS_SHM_CHART;
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_shm_charts()
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_SHM_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SHMGET_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_shm.get);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SHMAT_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_shm.at);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SHMDT_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_shm.dt);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SHMCTL_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_shm.ctl);
        ebpf_write_end_chart();
    }
}

/*
 * Send Specific Shared memory data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_shm_data(char *type, netdata_publish_shm_t *values)
{
    ebpf_write_begin_chart(type, NETDATA_SHMGET_CHART, "");
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL].name, (long long)values->get);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SHMAT_CHART, "");
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL].name, (long long)values->at);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SHMDT_CHART, "");
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL].name, (long long)values->dt);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SHMCTL_CHART, "");
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL].name, (long long)values->ctl);
    ebpf_write_end_chart();
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_shm_send_cgroup_data(int update_every)
{
    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_shm_sum_cgroup_pids(&ect->publish_shm, ect->pids);
    }

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_shm_charts(update_every);
        }

        ebpf_send_systemd_shm_charts();
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_SHM_CHART) && ect->updated) {
            ebpf_create_specific_shm_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_SHM_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_SHM_CHART) {
            if (ect->updated) {
                ebpf_send_specific_shm_data(ect->name, &ect->publish_shm);
            } else {
                ebpf_obsolete_specific_shm_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_SWAP_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Resume apps data
 */
void ebpf_shm_resume_apps_data()
{
    struct ebpf_target *w;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SHM_IDX))))
            continue;

        ebpf_shm_sum_pids(&w->shm, w->root_pid);
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * DCstat thread
 *
 * Thread used to generate dcstat charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_read_shm_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    if (!collect_pid)
        return NULL;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    int cgroups = em->cgroup_charts;
    uint32_t running_time = 0;
    pids_fd[NETDATA_EBPF_PIDS_SHM_IDX] = shm_maps[NETDATA_PID_SHM_TABLE].map_fd;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        sem_wait(shm_mutex_ebpf_integration);
        ebpf_read_shm_apps_table(maps_per_core);
        ebpf_shm_resume_apps_data();
        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_shm_cgroup();

        sem_post(shm_mutex_ebpf_integration);

        counter = 0;

        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }

    return NULL;
}

/**
* Main loop for this collector.
*/
static void shm_collector(ebpf_module_t *em)
{
    int cgroups = em->cgroup_charts;
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

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_shm_read_global_table(stats, maps_per_core);
        pthread_mutex_lock(&lock);

        shm_send_global();

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_shm_send_apps_data(apps_groups_root_target);
        }

        if (cgroups && shm_ebpf_cgroup.header) {
            ebpf_shm_send_cgroup_data(update_every);
        }

        pthread_mutex_unlock(&lock);

        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *  INITIALIZE THREAD
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_shm_create_apps_charts(struct ebpf_module *em, void *ptr)
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
            "_ebpf_shmget_call",
            "Calls to syscall shmget(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmget_call",
            20191,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SHM);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmat_call",
            "Calls to syscall shmat(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmat_call",
            20192,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SHM);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmdt_call",
            "Calls to syscall shmdt(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmdt_call",
            20193,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SHM);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_shmctl_call",
            "Calls to syscall shmctl(2).",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_IPC_SHM_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_shmctl_call",
            20194,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SHM);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        w->charts_created |= 1 << EBPF_MODULE_SHM_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param apps is apps enabled?
 */
static void ebpf_shm_allocate_global_vectors(int apps)
{
    UNUSED(apps);
    shm_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_publish_shm_t));
    shm_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(shm_hash_values, 0, sizeof(shm_hash_values));
}

/*****************************************************************
 *  MAIN THREAD
 *****************************************************************/

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_shm_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_SHM_GLOBAL_CHART,
        "Calls to shared memory system calls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_IPC_SHM_SUBMENU,
        "system.shared_memory_calls",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_CALLS,
        ebpf_create_global_dimension,
        shm_publish_aggregated,
        NETDATA_SHM_END,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SHM);

    fflush(stdout);
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_shm_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;

    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_KEY_SHMGET_CALL].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        shm_bpf_obj = shm_bpf__open();
        if (!shm_bpf_obj)
            ret = -1;
        else
            ret = ebpf_shm_load_and_attach(shm_bpf_obj, em);
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Shared memory thread.
 *
 * @param ptr a pointer to `struct ebpf_module`
 * @return It always return NULL
 */
void *ebpf_shm_thread(void *ptr)
{
    pids_fd[NETDATA_EBPF_PIDS_SHM_IDX] = -1;
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_shm_exit) cleanup_ptr = em;

    em->maps = shm_maps;

    ebpf_update_pid_table(&shm_maps[NETDATA_PID_SHM_TABLE], em);

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_shm_load_bpf(em)) {
        goto endshm;
    }

    ebpf_shm_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_SHM_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX};
    ebpf_global_labels(
        shm_aggregated_data,
        shm_publish_aggregated,
        shm_dimension_name,
        shm_dimension_name,
        algorithms,
        NETDATA_SHM_END);

    pthread_mutex_lock(&lock);
    ebpf_create_shm_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    pthread_mutex_unlock(&lock);

    ebpf_read_shm.thread =
        nd_thread_create(ebpf_read_shm.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_shm_thread, em);

    shm_collector(em);

endshm:
    ebpf_update_disabled_plugin_stats(em);

    return NULL;
}
