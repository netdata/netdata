// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_fd.h"

static char *fd_dimension_names[NETDATA_FD_SYSCALL_END] = {"open", "close"};
static char *fd_id_names[NETDATA_FD_SYSCALL_END] = {"do_sys_open", "__close_fd"};

#ifdef LIBBPF_MAJOR_VERSION
static char *close_targets[NETDATA_EBPF_MAX_FD_TARGETS] = {"close_fd", "__close_fd"};
static char *open_targets[NETDATA_EBPF_MAX_FD_TARGETS] = {"do_sys_openat2", "do_sys_open"};
#endif

static netdata_syscall_stat_t fd_aggregated_data[NETDATA_FD_SYSCALL_END];
static netdata_publish_syscall_t fd_publish_aggregated[NETDATA_FD_SYSCALL_END];

static ebpf_local_maps_t fd_maps[] = {
    {.name = "tbl_fd_pid",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tbl_fd_global",
     .internal_input = NETDATA_KEY_END_VECTOR,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "fd_ctrl",
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

struct config fd_config = APPCONFIG_INITIALIZER;

static netdata_idx_t fd_hash_values[NETDATA_FD_COUNTER];
static netdata_idx_t *fd_values = NULL;

netdata_fd_stat_t *fd_vector = NULL;

netdata_ebpf_targets_t fd_targets[] = {
    {.name = "open", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "close", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

struct netdata_static_thread ebpf_read_fd = {
    .name = "EBPF_READ_FD",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL};

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects
*/
static inline void ebpf_fd_disable_probes(struct fd_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_sys_open_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_sys_open_kretprobe, false);
    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        bpf_program__set_autoload(obj->progs.netdata___close_fd_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata___close_fd_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_close_fd_kprobe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata___close_fd_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_close_fd_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_close_fd_kprobe, false);
    }
}

/*
 * Disable specific probe
 *
 * Disable probes according the kernel version
 *
 * @param obj is the main structure for bpf objects
 */
static inline void ebpf_disable_specific_probes(struct fd_bpf *obj)
{
    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        bpf_program__set_autoload(obj->progs.netdata___close_fd_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata___close_fd_kprobe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_close_fd_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_close_fd_kprobe, false);
    }
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_disable_trampoline(struct fd_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_sys_open_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_sys_open_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_close_fd_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_close_fd_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata___close_fd_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata___close_fd_fexit, false);
}

/*
 * Disable specific trampoline
 *
 * Disable trampoline according to kernel version.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_disable_specific_trampoline(struct fd_bpf *obj)
{
    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        bpf_program__set_autoload(obj->progs.netdata___close_fd_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata___close_fd_fexit, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_close_fd_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_close_fd_fexit, false);
    }
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_set_trampoline_target(struct fd_bpf *obj)
{
    bpf_program__set_attach_target(obj->progs.netdata_sys_open_fentry, 0, fd_targets[NETDATA_FD_SYSCALL_OPEN].name);
    bpf_program__set_attach_target(obj->progs.netdata_sys_open_fexit, 0, fd_targets[NETDATA_FD_SYSCALL_OPEN].name);

    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        bpf_program__set_attach_target(
            obj->progs.netdata_close_fd_fentry, 0, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        bpf_program__set_attach_target(obj->progs.netdata_close_fd_fexit, 0, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
    } else {
        bpf_program__set_attach_target(
            obj->progs.netdata___close_fd_fentry, 0, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        bpf_program__set_attach_target(
            obj->progs.netdata___close_fd_fexit, 0, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
    }
}

/**
 * Mount Attach Probe
 *
 * Attach probes to target
 *
 * @param obj is the main structure for bpf objects.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_fd_attach_probe(struct fd_bpf *obj)
{
    obj->links.netdata_sys_open_kprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_sys_open_kprobe, false, fd_targets[NETDATA_FD_SYSCALL_OPEN].name);
    long ret = libbpf_get_error(obj->links.netdata_sys_open_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_sys_open_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_sys_open_kretprobe, true, fd_targets[NETDATA_FD_SYSCALL_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_sys_open_kretprobe);
    if (ret)
        return -1;

    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        obj->links.netdata_close_fd_kretprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_close_fd_kretprobe, true, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata_close_fd_kretprobe);
        if (ret)
            return -1;

        obj->links.netdata_close_fd_kprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_close_fd_kprobe, false, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata_close_fd_kprobe);
        if (ret)
            return -1;
    } else {
        obj->links.netdata___close_fd_kretprobe = bpf_program__attach_kprobe(
            obj->progs.netdata___close_fd_kretprobe, true, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata___close_fd_kretprobe);
        if (ret)
            return -1;

        obj->links.netdata___close_fd_kprobe = bpf_program__attach_kprobe(
            obj->progs.netdata___close_fd_kprobe, false, fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata___close_fd_kprobe);
        if (ret)
            return -1;
    }

    return 0;
}

/**
 * FD Fill Address
 *
 * Fill address value used to load probes/trampoline.
 */
static inline void ebpf_fd_fill_address(ebpf_addresses_t *address, char **targets)
{
    int i;
    for (i = 0; i < NETDATA_EBPF_MAX_FD_TARGETS; i++) {
        address->function = targets[i];
        ebpf_load_addresses(address, -1);
        if (address->addr)
            break;
    }
}

/**
 * Set target values
 *
 * Set pointers used to load data.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_fd_set_target_values()
{
    ebpf_addresses_t address = {.function = NULL, .hash = 0, .addr = 0};
    ebpf_fd_fill_address(&address, close_targets);

    if (!address.addr)
        return -1;

    fd_targets[NETDATA_FD_SYSCALL_CLOSE].name = address.function;

    address.addr = 0;
    ebpf_fd_fill_address(&address, open_targets);

    if (!address.addr)
        return -1;

    fd_targets[NETDATA_FD_SYSCALL_OPEN].name = address.function;

    return 0;
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_fd_set_hash_tables(struct fd_bpf *obj)
{
    fd_maps[NETDATA_FD_GLOBAL_STATS].map_fd = bpf_map__fd(obj->maps.tbl_fd_global);
    fd_maps[NETDATA_FD_PID_STATS].map_fd = bpf_map__fd(obj->maps.tbl_fd_pid);
    fd_maps[NETDATA_FD_CONTROLLER].map_fd = bpf_map__fd(obj->maps.fd_ctrl);
}

/**
 * Adjust Map Size
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_fd_adjust_map(struct fd_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(obj->maps.tbl_fd_pid, &fd_maps[NETDATA_FD_PID_STATS], em, bpf_map__name(obj->maps.tbl_fd_pid));

    ebpf_update_map_type(obj->maps.tbl_fd_global, &fd_maps[NETDATA_FD_GLOBAL_STATS]);
    ebpf_update_map_type(obj->maps.tbl_fd_pid, &fd_maps[NETDATA_FD_PID_STATS]);
    ebpf_update_map_type(obj->maps.fd_ctrl, &fd_maps[NETDATA_FD_CONTROLLER]);
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
static inline int ebpf_fd_load_and_attach(struct fd_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_FD_SYSCALL_OPEN].mode;

    if (ebpf_fd_set_target_values()) {
        netdata_log_error("%s file descriptor.", NETDATA_EBPF_DEFAULT_FNT_NOT_FOUND);
        return -1;
    }

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_fd_disable_probes(obj);
        ebpf_disable_specific_trampoline(obj);

        ebpf_set_trampoline_target(obj);
    } else {
        ebpf_disable_trampoline(obj);
        ebpf_disable_specific_probes(obj);
    }

    ebpf_fd_adjust_map(obj, em);

    int ret = fd_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ret = (test == EBPF_LOAD_TRAMPOLINE) ? fd_bpf__attach(obj) : ebpf_fd_attach_probe(obj);
    if (!ret) {
        ebpf_fd_set_hash_tables(obj);

        ebpf_update_controller(fd_maps[NETDATA_FD_CONTROLLER].map_fd, em);
    }

    return ret;
}
#endif

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

static void ebpf_obsolete_specific_fd_charts(char *type, ebpf_module_t *em);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_fd_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_FILE_OPEN,
        "",
        "Number of open files",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CGROUP_FD_OPEN_CONTEXT,
        20270,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
            "",
            "Fails to open files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT,
            20271,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_FILE_CLOSED,
        "",
        "Files closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CGROUP_FD_CLOSE_CONTEXT,
        20272,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
            "",
            "Fails to close files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT,
            20273,
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
static inline void ebpf_obsolete_fd_cgroup_charts(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_fd_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_fd_charts(ect->name, em);
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
void ebpf_obsolete_fd_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_FD_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_file_open",
            "Number of open files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_FDS,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_file_open",
            20220,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_file_open_error",
                "Fails to open files.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_APPS_FILE_FDS,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_file_open_error",
                20221,
                update_every);
        }

        ebpf_write_chart_obsolete(
            NETDATA_APPS_FAMILY,
            w->clean_name,
            "_ebpf_file_closed",
            "Files closed.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_FDS,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_file_closed",
            20222,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APPS_FAMILY,
                w->clean_name,
                "_ebpf_file_close_error",
                "Fails to close files.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_APPS_FILE_FDS,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_fd_close_error",
                20223,
                update_every);
        }
        w->charts_created &= ~(1 << EBPF_MODULE_FD_IDX);
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
static void ebpf_obsolete_fd_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_FILE_OPEN_CLOSE_COUNT,
        "",
        "Open and close calls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_FILE_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_FS_FILEDESCRIPTOR_CONTEXT,
        NETDATA_CHART_PRIO_EBPF_FD_CHARTS,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_FILE_OPEN_ERR_COUNT,
            "",
            "Open fails",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_FILE_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_FS_FILEDESCRIPTOR_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_EBPF_FD_CHARTS + 1,
            em->update_every);
    }
}

/**
 * FD Exit
 *
 * Cancel child thread and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_fd_exit(void *pptr)
{
    pids_fd[NETDATA_EBPF_PIDS_FD_IDX] = -1;
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    pthread_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_FD_IDX);
    pthread_mutex_unlock(&lock);

    if (ebpf_read_fd.thread)
        nd_thread_signal_cancel(ebpf_read_fd.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_fd_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_fd_apps_charts(em);
        }

        ebpf_obsolete_fd_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (fd_bpf_obj) {
        fd_bpf__destroy(fd_bpf_obj);
        fd_bpf_obj = NULL;
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
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_fd_send_data(ebpf_module_t *em)
{
    fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].ncall = fd_hash_values[NETDATA_KEY_CALLS_DO_SYS_OPEN];
    fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].nerr = fd_hash_values[NETDATA_KEY_ERROR_DO_SYS_OPEN];

    fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].ncall = fd_hash_values[NETDATA_KEY_CALLS_CLOSE_FD];
    fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].nerr = fd_hash_values[NETDATA_KEY_ERROR_CLOSE_FD];

    write_count_chart(
        NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_FILESYSTEM_FAMILY, fd_publish_aggregated, NETDATA_FD_SYSCALL_END);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_FILESYSTEM_FAMILY, fd_publish_aggregated, NETDATA_FD_SYSCALL_END);
    }
}

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_fd_read_global_tables(netdata_idx_t *stats, int maps_per_core)
{
    ebpf_read_global_table_stats(
        fd_hash_values,
        fd_values,
        fd_maps[NETDATA_FD_GLOBAL_STATS].map_fd,
        maps_per_core,
        NETDATA_KEY_CALLS_DO_SYS_OPEN,
        NETDATA_FD_COUNTER);

    ebpf_read_global_table_stats(
        stats,
        fd_values,
        fd_maps[NETDATA_FD_CONTROLLER].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);
}

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 * @param maps_per_core do I need to read all cores?
 */
static void fd_apps_accumulator(netdata_fd_stat_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_fd_stat_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_fd_stat_t *w = &out[i];
        total->open_call += w->open_call;
        total->close_call += w->close_call;
        total->open_err += w->open_err;
        total->close_err += w->close_err;

        if (w->ct > ct)
            ct = w->ct;

        if (!total->name[0] && w->name[0])
            strncpyz(total->name, w->name, sizeof(total->name) - 1);
    }
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_fd_apps_table(int maps_per_core)
{
    netdata_fd_stat_t *fv = fd_vector;
    int fd = fd_maps[NETDATA_FD_PID_STATS].map_fd;
    size_t length = sizeof(netdata_fd_stat_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, fv)) {
            goto end_fd_loop;
        }

        fd_apps_accumulator(fv, maps_per_core);

        ebpf_pid_data_t *pid_stat = ebpf_get_pid_data(key, fv->tgid, fv->name, NETDATA_EBPF_PIDS_FD_IDX);
        netdata_publish_fd_stat_t *publish_fd = pid_stat->fd;
        if (!publish_fd)
            pid_stat->fd = publish_fd = ebpf_fd_allocate_publish();

        if (!publish_fd->ct || publish_fd->ct != fv->ct) {
            publish_fd->ct = fv->ct;
            publish_fd->open_call = fv->open_call;
            publish_fd->close_call = fv->close_call;
            publish_fd->open_err = fv->open_err;
            publish_fd->close_err = fv->close_err;

            pid_stat->not_updated = 0;
        } else {
            if (kill(key, 0)) { // No PID found
                ebpf_reset_specific_pid_data(pid_stat);
            } else { // There is PID, but there is not data anymore
                ebpf_release_pid_data(pid_stat, fd, key, NETDATA_EBPF_PIDS_FD_IDX);
                ebpf_fd_release_publish(publish_fd);
                pid_stat->fd = NULL;
            }
        }

    end_fd_loop:
        // We are cleaning to avoid passing data read from one process to other.
        memset(fv, 0, length);
        key = next_key;
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
static void ebpf_fd_sum_pids(netdata_fd_stat_t *fd, struct ebpf_pid_on_target *root)
{
    memset(fd, 0, sizeof(netdata_fd_stat_t));

    for (; root; root = root->next) {
        int32_t pid = root->pid;
        ebpf_pid_data_t *pid_stat = ebpf_get_pid_data(pid, 0, NULL, NETDATA_EBPF_PIDS_FD_IDX);
        netdata_publish_fd_stat_t *w = pid_stat->fd;
        if (!w)
            continue;

        fd->open_call += w->open_call;
        fd->close_call += w->close_call;
        fd->open_err += w->open_err;
        fd->close_err += w->close_err;
    }
}

/**
 * Resume apps data
 */
void ebpf_fd_resume_apps_data()
{
    struct ebpf_target *w;

    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_FD_IDX))))
            continue;

        ebpf_fd_sum_pids(&w->fd, w->root_pid);
    }
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
void *ebpf_read_fd_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    if (!collect_pid)
        return NULL;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    uint32_t running_time = 0;
    pids_fd[NETDATA_EBPF_PIDS_FD_IDX] = fd_maps[NETDATA_FD_PID_STATS].map_fd;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        pthread_mutex_lock(&collect_data_mutex);
        ebpf_read_fd_apps_table(maps_per_core);
        ebpf_fd_resume_apps_data();
        pthread_mutex_unlock(&collect_data_mutex);

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
 * Update cgroup
 *
 * Update cgroup data collected per PID.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_fd_cgroup()
{
    ebpf_cgroup_target_t *ect;

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_publish_fd_stat_t *out = &pids->fd;
            ebpf_pid_data_t *local_pid = ebpf_get_pid_data(pid, 0, NULL, NETDATA_EBPF_PIDS_FD_IDX);
            netdata_publish_fd_stat_t *in = local_pid->fd;
            if (!in)
                continue;
            memcpy(out, in, sizeof(netdata_publish_fd_stat_t));
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
*/
void ebpf_fd_send_apps_data(ebpf_module_t *em, struct ebpf_target *root)
{
    struct ebpf_target *w;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_FD_IDX))))
            continue;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_file_open");
        write_chart_dimension("calls", w->fd.open_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_file_open_error");
            write_chart_dimension("calls", w->fd.open_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_file_closed");
        write_chart_dimension("calls", w->fd.close_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_file_close_error");
            write_chart_dimension("calls", w->fd.close_err);
            ebpf_write_end_chart();
        }
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param fd  structure used to store data
 * @param pids input data
 */
static void ebpf_fd_sum_cgroup_pids(netdata_publish_fd_stat_t *fd, struct pid_on_target2 *pids)
{
    netdata_fd_stat_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        netdata_publish_fd_stat_t *w = &pids->fd;

        accumulator.open_err += w->open_err;
        accumulator.open_call += w->open_call;
        accumulator.close_call += w->close_call;
        accumulator.close_err += w->close_err;

        pids = pids->next;
    }

    fd->open_call = (accumulator.open_call >= fd->open_call) ? accumulator.open_call : fd->open_call;
    fd->open_err = (accumulator.open_err >= fd->open_err) ? accumulator.open_err : fd->open_err;
    fd->close_call = (accumulator.close_call >= fd->close_call) ? accumulator.close_call : fd->close_call;
    fd->close_err = (accumulator.close_err >= fd->close_err) ? accumulator.close_err : fd->close_err;
}

/**
 * Create specific file descriptor charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param em   the main thread structure.
 */
static void ebpf_create_specific_fd_charts(char *type, ebpf_module_t *em)
{
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_FILE_OPEN,
        "Number of open files",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_CGROUP_FD_OPEN_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5400,
        ebpf_create_global_dimension,
        &fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_FD);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
            "Fails to open files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5401,
            ebpf_create_global_dimension,
            &fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_FILE_CLOSED,
        "Files closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_CGROUP_FD_CLOSE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5402,
        ebpf_create_global_dimension,
        &fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_FD);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
            "Fails to close files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5403,
            ebpf_create_global_dimension,
            &fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }
}

/**
 * Obsolete specific file descriptor charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param em   the main thread structure.
 */
static void ebpf_obsolete_specific_fd_charts(char *type, ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_FILE_OPEN,
        "",
        "Number of open files",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_FD_OPEN_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5400,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
            "",
            "Fails to open files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5401,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_FILE_CLOSED,
        "",
        "Files closed",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_APPS_FILE_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_FD_CLOSE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5402,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
            "",
            "Fails to close files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5403,
            em->update_every);
    }
}

/*
 * Send specific file descriptor data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_fd_data(char *type, netdata_publish_fd_stat_t *values, ebpf_module_t *em)
{
    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_FILE_OPEN, "");
    write_chart_dimension(fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].name, (long long)values->open_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "");
        write_chart_dimension(fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN].name, (long long)values->open_err);
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_FILE_CLOSED, "");
    write_chart_dimension(fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].name, (long long)values->close_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "");
        write_chart_dimension(fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE].name, (long long)values->close_err);
        ebpf_write_end_chart();
    }
}

/**
 *  Create systemd file descriptor charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param em the main collector structure
 **/
static void ebpf_create_systemd_fd_charts(ebpf_module_t *em)
{
    static ebpf_systemd_args_t data_open = {
        .title = "Number of open files",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_FILE_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20270,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_FD_OPEN_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_FD,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_FILE_OPEN,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_open_error = {
        .title = "Fails to open files",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_FILE_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20271,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_FD_OPEN_ERR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_FD,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_close = {
        .title = "Files closed",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_FILE_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20272,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_FD_CLOSE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_FD,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_FILE_CLOSED,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_close_error = {
        .title = "Fails to close files",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_APPS_FILE_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20273,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_FD_OPEN_ERR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_FD,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
        .dimension = "calls"};

    if (!data_open.update_every)
        data_open.update_every = data_open_error.update_every = data_close.update_every =
            data_close_error.update_every = em->update_every;

    ebpf_cgroup_target_t *w;
    netdata_run_mode_t mode = em->mode;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_FD_CHART))
            continue;

        data_open.id = data_open_error.id = data_close.id = data_close_error.id = w->name;
        ebpf_create_charts_on_systemd(&data_open);
        ebpf_create_charts_on_systemd(&data_close);
        if (mode < MODE_ENTRY) {
            ebpf_create_charts_on_systemd(&data_open_error);
            ebpf_create_charts_on_systemd(&data_close_error);
        }

        w->flags |= NETDATA_EBPF_SERVICES_HAS_FD_CHART;
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 *
 *  @param em the main collector structure
 */
static void ebpf_send_systemd_fd_charts(ebpf_module_t *em)
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_FD_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_FILE_OPEN, "");
        write_chart_dimension("calls", ect->publish_systemd_fd.open_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_fd.open_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_FILE_CLOSED, "");
        write_chart_dimension("calls", ect->publish_systemd_fd.close_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_fd.close_err);
            ebpf_write_end_chart();
        }
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the main collector structure
*/
static void ebpf_fd_send_cgroup_data(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_fd_sum_cgroup_pids(&ect->publish_systemd_fd, ect->pids);
    }

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_fd_charts(em);
        }

        ebpf_send_systemd_fd_charts(em);
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_FD_CHART) && ect->updated) {
            ebpf_create_specific_fd_charts(ect->name, em);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_FD_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_FD_CHART) {
            if (ect->updated) {
                ebpf_send_specific_fd_data(ect->name, &ect->publish_systemd_fd, em);
            } else {
                ebpf_obsolete_specific_fd_charts(ect->name, em);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_FD_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void fd_collector(ebpf_module_t *em)
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
        ebpf_fd_read_global_tables(stats, maps_per_core);

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_fd_cgroup();

        pthread_mutex_lock(&lock);

        ebpf_fd_send_data(em);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_fd_send_apps_data(em, apps_groups_root_target);

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_fd_send_cgroup_data(em);

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
    struct ebpf_target *root = ptr;
    struct ebpf_target *w;
    int update_every = em->update_every;
    for (w = root; w; w = w->next) {
        if (unlikely(!w->exposed))
            continue;

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_file_open",
            "Number of open files",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_FDS,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_file_open",
            20220,
            update_every,
            NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_file_open_error",
                "Fails to open files.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_APPS_FILE_FDS,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_file_open_error",
                20221,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FD);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_file_closed",
            "Files closed.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_APPS_FILE_FDS,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_file_closed",
            20222,
            update_every,
            NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_file_close_error",
                "Fails to close files.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_APPS_FILE_FDS,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_file_close_error",
                20223,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FD);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        w->charts_created |= 1 << EBPF_MODULE_FD_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
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
    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_FILE_OPEN_CLOSE_COUNT,
        "Open and close calls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_FILE_GROUP,
        NETDATA_FS_FILEDESCRIPTOR_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_EBPF_FD_CHARTS,
        ebpf_create_global_dimension,
        fd_publish_aggregated,
        NETDATA_FD_SYSCALL_END,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_FD);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_FILE_OPEN_ERR_COUNT,
            "Open fails",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_FILE_GROUP,
            NETDATA_FS_FILEDESCRIPTOR_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_EBPF_FD_CHARTS + 1,
            ebpf_create_global_dimension,
            fd_publish_aggregated,
            NETDATA_FD_SYSCALL_END,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_FD);
    }

    fflush(stdout);
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
static inline void ebpf_fd_allocate_global_vectors()
{
    fd_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_fd_stat_t));
    fd_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_fd_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(fd_maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_FD_SYSCALL_OPEN].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        fd_bpf_obj = fd_bpf__open();
        if (!fd_bpf_obj)
            ret = -1;
        else
            ret = ebpf_fd_load_and_attach(fd_bpf_obj, em);
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
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
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_fd_exit) cleanup_ptr = em;

    em->maps = fd_maps;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_fd_load_bpf(em)) {
        goto endfd;
    }

    ebpf_fd_allocate_global_vectors();

    int algorithms[NETDATA_FD_SYSCALL_END] = {NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX};

    ebpf_global_labels(
        fd_aggregated_data, fd_publish_aggregated, fd_dimension_names, fd_id_names, algorithms, NETDATA_FD_SYSCALL_END);

    pthread_mutex_lock(&lock);
    ebpf_create_fd_global_charts(em);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);

    pthread_mutex_unlock(&lock);

    ebpf_read_fd.thread = nd_thread_create(ebpf_read_fd.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_fd_thread, em);

    fd_collector(em);

endfd:
    ebpf_update_disabled_plugin_stats(em);

    return NULL;
}
