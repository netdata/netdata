// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_fd.h"

static char *fd_dimension_names[NETDATA_FD_SYSCALL_END] = { "open", "close" };
static char *fd_id_names[NETDATA_FD_SYSCALL_END] = { "do_sys_open",  "__close_fd" };

static char *close_targets[NETDATA_EBPF_MAX_FD_TARGETS] = {"close_fd", "__close_fd"};
static char *open_targets[NETDATA_EBPF_MAX_FD_TARGETS] = {"do_sys_openat2", "do_sys_open"};

static netdata_syscall_stat_t fd_aggregated_data[NETDATA_FD_SYSCALL_END];
static netdata_publish_syscall_t fd_publish_aggregated[NETDATA_FD_SYSCALL_END];

static ebpf_local_maps_t fd_maps[] = {{.name = "tbl_fd_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                       .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_RESIZABLE  | NETDATA_EBPF_MAP_PID,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                      },
                                      {.name = "tbl_fd_global", .internal_input = NETDATA_KEY_END_VECTOR,
                                       .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                      },
                                      {.name = "fd_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                       .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_CONTROLLER,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                      },
                                      {.name = NULL, .internal_input = 0, .user_input = 0,
                                       .type = NETDATA_EBPF_MAP_CONTROLLER,
                                       .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                       .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                       }};


struct config fd_config = { .first_section = NULL, .last_section = NULL, .mutex = NETDATA_MUTEX_INITIALIZER,
                           .index = {.avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                     .rwlock = AVL_LOCK_INITIALIZER } };

static netdata_idx_t fd_hash_values[NETDATA_FD_COUNTER];
static netdata_idx_t *fd_values = NULL;

netdata_fd_stat_t *fd_vector = NULL;

netdata_ebpf_targets_t fd_targets[] = { {.name = "open", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "close", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef NETDATA_DEV_MODE
int fd_disable_priority;
#endif

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
    obj->links.netdata_sys_open_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_sys_open_kprobe, false,
                                                                    fd_targets[NETDATA_FD_SYSCALL_OPEN].name);
    int ret = libbpf_get_error(obj->links.netdata_sys_open_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_sys_open_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_sys_open_kretprobe, true,
                                                                       fd_targets[NETDATA_FD_SYSCALL_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_sys_open_kretprobe);
    if (ret)
        return -1;

    if (!strcmp(fd_targets[NETDATA_FD_SYSCALL_CLOSE].name, close_targets[NETDATA_FD_CLOSE_FD])) {
        obj->links.netdata_close_fd_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_close_fd_kretprobe, true,
                                                                           fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata_close_fd_kretprobe);
        if (ret)
            return -1;

        obj->links.netdata_close_fd_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_close_fd_kprobe, false,
                                                                        fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata_close_fd_kprobe);
        if (ret)
            return -1;
    } else {
        obj->links.netdata___close_fd_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata___close_fd_kretprobe,
                                                                             true,
                                                                             fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
        ret = libbpf_get_error(obj->links.netdata___close_fd_kretprobe);
        if (ret)
            return -1;

        obj->links.netdata___close_fd_kprobe = bpf_program__attach_kprobe(obj->progs.netdata___close_fd_kprobe,
                                                                          false,
                                                                          fd_targets[NETDATA_FD_SYSCALL_CLOSE].name);
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
    ebpf_update_map_size(obj->maps.tbl_fd_pid, &fd_maps[NETDATA_FD_PID_STATS],
                         em, bpf_map__name(obj->maps.tbl_fd_pid));

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
static void ebpf_obsolete_fd_services(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_SYSCALL_APPS_FILE_OPEN,
                              "",
                              "Number of open files",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_FILE_CGROUP_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_CGROUP_FD_OPEN_CONTEXT,
                              20270,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                                  NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR,
                                  "",
                                  "Fails to open files",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_FILE_CGROUP_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT,
                                  20271,
                                  em->update_every);
    }

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_SYSCALL_APPS_FILE_CLOSED,
                              "",
                              "Files closed",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_FILE_CGROUP_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_CGROUP_FD_CLOSE_CONTEXT,
                              20272,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                                  NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR,
                                  "",
                                  "Fails to close files",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_FILE_CGROUP_GROUP,
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
static inline void ebpf_obsolete_fd_cgroup_charts(ebpf_module_t *em) {
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_obsolete_fd_services(em);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

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
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1<<EBPF_MODULE_FD_IDX))))
            continue;

        ebpf_write_chart_obsolete(NETDATA_APP_FAMILY,
                                  w->clean_name,
                                  "_ebpf_file_open",
                                  "Number of open files",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_FILE_FDS,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  "app.ebpf_file_open",
                                  20220,
                                  update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(NETDATA_APP_FAMILY,
                                      w->clean_name,
                                      "_ebpf_file_open_error",
                                      "Fails to open files.",
                                      EBPF_COMMON_DIMENSION_CALL,
                                      NETDATA_APPS_FILE_FDS,
                                      NETDATA_EBPF_CHART_TYPE_STACKED,
                                      "app.ebpf_file_open_error",
                                      20221,
                                      update_every);
        }

        ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                                  w->clean_name,
                                  "_ebpf_file_closed",
                                  "Files closed.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_FILE_FDS,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  "app.ebpf_file_closed",
                                  20222,
                                  update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                                      w->clean_name,
                                      "_ebpf_file_close_error",
                                      "Fails to close files.",
                                      EBPF_COMMON_DIMENSION_CALL,
                                      NETDATA_APPS_FILE_FDS,
                                      NETDATA_EBPF_CHART_TYPE_STACKED,
                                      "app.ebpf_fd_close_error",
                                      20223,
                                      update_every);
        }
        w->charts_created &= ~(1<<EBPF_MODULE_FD_IDX);
    }
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
    ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY,
                              NETDATA_FILE_OPEN_CLOSE_COUNT,
                              "",
                              "Open and close calls",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_FILE_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              NETDATA_CHART_PRIO_EBPF_FD_CHARTS,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY,
                                  NETDATA_FILE_OPEN_ERR_COUNT,
                                  "",
                                  "Open fails",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_FILE_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  NULL,
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
static void ebpf_fd_exit(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

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

#ifdef NETDATA_DEV_MODE
        if (ebpf_aral_fd_pid)
            ebpf_statistic_obsolete_aral_chart(em, fd_disable_priority);
#endif


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

    write_count_chart(NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_FILESYSTEM_FAMILY, fd_publish_aggregated,
                      NETDATA_FD_SYSCALL_END);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_FILESYSTEM_FAMILY,
                        fd_publish_aggregated, NETDATA_FD_SYSCALL_END);
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
    ebpf_read_global_table_stats(fd_hash_values,
                                 fd_values,
                                 fd_maps[NETDATA_FD_GLOBAL_STATS].map_fd,
                                 maps_per_core,
                                 NETDATA_KEY_CALLS_DO_SYS_OPEN,
                                 NETDATA_FD_COUNTER);

    ebpf_read_global_table_stats(stats,
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
    for (i = 1; i < end; i++) {
        netdata_fd_stat_t *w = &out[i];
        total->open_call += w->open_call;
        total->close_call += w->close_call;
        total->open_err += w->open_err;
        total->close_err += w->close_err;
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
static void fd_fill_pid(uint32_t current_pid, netdata_fd_stat_t *publish)
{
    netdata_fd_stat_t *curr = fd_pid[current_pid];
    if (!curr) {
        curr = ebpf_fd_stat_get();
        fd_pid[current_pid] = curr;
    }

    memcpy(curr, &publish[0], sizeof(netdata_fd_stat_t));
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void read_fd_apps_table(int maps_per_core)
{
    netdata_fd_stat_t *fv = fd_vector;
    uint32_t key;
    struct ebpf_pid_stat *pids = ebpf_root_of_pids;
    int fd = fd_maps[NETDATA_FD_PID_STATS].map_fd;
    size_t length = sizeof(netdata_fd_stat_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, fv)) {
            pids = pids->next;
            continue;
        }

        fd_apps_accumulator(fv, maps_per_core);

        fd_fill_pid(key, fv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(fv, 0, length);

        pids = pids->next;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data collected per PID.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_fd_cgroup(int maps_per_core)
{
    ebpf_cgroup_target_t *ect ;
    netdata_fd_stat_t *fv = fd_vector;
    int fd = fd_maps[NETDATA_FD_PID_STATS].map_fd;
    size_t length = sizeof(netdata_fd_stat_t) * ebpf_nprocs;

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_fd_stat_t *out = &pids->fd;
            if (likely(fd_pid) && fd_pid[pid]) {
                netdata_fd_stat_t *in = fd_pid[pid];

                memcpy(out, in, sizeof(netdata_fd_stat_t));
            } else {
                memset(fv, 0, length);
                if (!bpf_map_lookup_elem(fd, &pid, fv)) {
                    fd_apps_accumulator(fv, maps_per_core);

                    memcpy(out, fv, sizeof(netdata_fd_stat_t));
                }
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
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
    uint32_t open_call = 0;
    uint32_t close_call = 0;
    uint32_t open_err = 0;
    uint32_t close_err = 0;

    while (root) {
        int32_t pid = root->pid;
        netdata_fd_stat_t *w = fd_pid[pid];
        if (w) {
            open_call += w->open_call;
            close_call += w->close_call;
            open_err += w->open_err;
            close_err += w->close_err;
        }

        root = root->next;
    }

    // These conditions were added, because we are using incremental algorithm
    fd->open_call = (open_call >= fd->open_call) ? open_call : fd->open_call;
    fd->close_call = (close_call >= fd->close_call) ? close_call : fd->close_call;
    fd->open_err = (open_err >= fd->open_err) ? open_err : fd->open_err;
    fd->close_err = (close_err >= fd->close_err) ? close_err : fd->close_err;
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
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1<<EBPF_MODULE_FD_IDX))))
            continue;

        ebpf_fd_sum_pids(&w->fd, w->root_pid);

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
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param fd  structure used to store data
 * @param pids input data
 */
static void ebpf_fd_sum_cgroup_pids(netdata_fd_stat_t *fd, struct pid_on_target2 *pids)
{
    netdata_fd_stat_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        netdata_fd_stat_t *w = &pids->fd;

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
    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_FILE_OPEN, "Number of open files",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                      NETDATA_CGROUP_FD_OPEN_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5400,
                      ebpf_create_global_dimension,
                      &fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_FD);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(type, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "Fails to open files",
                          EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                          NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5401,
                          ebpf_create_global_dimension,
                          &fd_publish_aggregated[NETDATA_FD_SYSCALL_OPEN],
                          1, em->update_every,
                          NETDATA_EBPF_MODULE_NAME_FD);
    }

    ebpf_create_chart(type, NETDATA_SYSCALL_APPS_FILE_CLOSED, "Files closed",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                      NETDATA_CGROUP_FD_CLOSE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5402,
                      ebpf_create_global_dimension,
                      &fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_FD);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(type, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "Fails to close files",
                          EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                          NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5403,
                          ebpf_create_global_dimension,
                          &fd_publish_aggregated[NETDATA_FD_SYSCALL_CLOSE],
                          1, em->update_every,
                          NETDATA_EBPF_MODULE_NAME_FD);
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
    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_FILE_OPEN, "", "Number of open files",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_FD_OPEN_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5400, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "", "Fails to open files",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT,
                                  NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5401, em->update_every);
    }

    ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_FILE_CLOSED, "", "Files closed",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_FD_CLOSE_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5402, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(type, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "", "Fails to close files",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT,
                                  NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5403, em->update_every);
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
static void ebpf_send_specific_fd_data(char *type, netdata_fd_stat_t *values, ebpf_module_t *em)
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
    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_FILE_OPEN, "Number of open files",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20061,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_FD_OPEN_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_FD, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "Fails to open files",
                                      EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                                      NETDATA_EBPF_CHART_TYPE_STACKED, 20062,
                                      ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_FD_OPEN_ERR_CONTEXT,
                                      NETDATA_EBPF_MODULE_NAME_FD, em->update_every);
    }

    ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_FILE_CLOSED, "Files closed",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20063,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_FD_CLOSE_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_FD, em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_charts_on_systemd(NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "Fails to close files",
                                      EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_FILE_CGROUP_GROUP,
                                      NETDATA_EBPF_CHART_TYPE_STACKED, 20064,
                                      ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_FD_CLOSE_ERR_CONTEXT,
                                      NETDATA_EBPF_MODULE_NAME_FD, em->update_every);
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
    ebpf_write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN, "");
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_fd.open_call);
        }
    }
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR, "");
        for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
            if (unlikely(ect->systemd) && unlikely(ect->updated)) {
                write_chart_dimension(ect->name, ect->publish_systemd_fd.open_err);
            }
        }
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSED, "");
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, ect->publish_systemd_fd.close_call);
        }
    }
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR, "");
        for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
            if (unlikely(ect->systemd) && unlikely(ect->updated)) {
                write_chart_dimension(ect->name, ect->publish_systemd_fd.close_err);
            }
        }
        ebpf_write_end_chart();
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the main collector structure
*/
static void ebpf_fd_send_cgroup_data(ebpf_module_t *em)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_fd_sum_cgroup_pids(&ect->publish_systemd_fd, ect->pids);
    }

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;
    if (has_systemd) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_fd_charts(em);
        }

        ebpf_send_systemd_fd_charts(em);
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_FD_CHART) && ect->updated) {
            ebpf_create_specific_fd_charts(ect->name, em);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_FD_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_FD_CHART ) {
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
    heartbeat_t hb;
    heartbeat_init(&hb);
    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    while (!ebpf_plugin_exit && running_time < lifetime) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);

        if (ebpf_plugin_exit || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_fd_read_global_tables(stats, maps_per_core);
        pthread_mutex_lock(&collect_data_mutex);
        if (apps)
            read_fd_apps_table(maps_per_core);

        if (cgroups)
            ebpf_update_fd_cgroup(maps_per_core);

        pthread_mutex_lock(&lock);

#ifdef NETDATA_DEV_MODE
        if (ebpf_aral_fd_pid)
            ebpf_send_data_aral_chart(ebpf_aral_fd_pid, em);
#endif

        ebpf_fd_send_data(em);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_fd_send_apps_data(em, apps_groups_root_target);

        if (cgroups)
            ebpf_fd_send_cgroup_data(em);

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

        ebpf_write_chart_cmd(NETDATA_APP_FAMILY,
                             w->clean_name,
                             "_ebpf_file_open",
                             "Number of open files",
                             EBPF_COMMON_DIMENSION_CALL,
                             NETDATA_APPS_FILE_FDS,
                             NETDATA_EBPF_CHART_TYPE_STACKED,
                             "app.ebpf_file_open",
                             20220,
                             update_every,
                             NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("app_group", w->name, 1);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(NETDATA_APP_FAMILY,
                                 w->clean_name,
                                 "_ebpf_file_open_error",
                                 "Fails to open files.",
                                 EBPF_COMMON_DIMENSION_CALL,
                                 NETDATA_APPS_FILE_FDS,
                                 NETDATA_EBPF_CHART_TYPE_STACKED,
                                 "app.ebpf_file_open_error",
                                 20221,
                                 update_every,
                                 NETDATA_EBPF_MODULE_NAME_FD);
            ebpf_create_chart_labels("app_group", w->name, 1);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(NETDATA_APP_FAMILY,
                             w->clean_name,
                             "_ebpf_file_closed",
                             "Files closed.",
                             EBPF_COMMON_DIMENSION_CALL,
                             NETDATA_APPS_FILE_FDS,
                             NETDATA_EBPF_CHART_TYPE_STACKED,
                             "app.ebpf_file_closed",
                             20222,
                             update_every,
                             NETDATA_EBPF_MODULE_NAME_FD);
        ebpf_create_chart_labels("app_group", w->name, 1);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(NETDATA_APP_FAMILY,
                                 w->clean_name,
                                 "_ebpf_file_close_error",
                                 "Fails to close files.",
                                 EBPF_COMMON_DIMENSION_CALL,
                                 NETDATA_APPS_FILE_FDS,
                                 NETDATA_EBPF_CHART_TYPE_STACKED,
                                 "app.ebpf_file_close_error",
                                 20223,
                                 update_every,
                                 NETDATA_EBPF_MODULE_NAME_FD);
            ebpf_create_chart_labels("app_group", w->name, 1);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        w->charts_created |= 1<<EBPF_MODULE_FD_IDX;
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
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                      NETDATA_FILE_OPEN_CLOSE_COUNT,
                      "Open and close calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_FILE_GROUP,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_EBPF_FD_CHARTS,
                      ebpf_create_global_dimension,
                      fd_publish_aggregated,
                      NETDATA_FD_SYSCALL_END,
                      em->update_every, NETDATA_EBPF_MODULE_NAME_FD);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY,
                          NETDATA_FILE_OPEN_ERR_COUNT,
                          "Open fails",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_FILE_GROUP,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          NETDATA_CHART_PRIO_EBPF_FD_CHARTS + 1,
                          ebpf_create_global_dimension,
                          fd_publish_aggregated,
                          NETDATA_FD_SYSCALL_END,
                          em->update_every, NETDATA_EBPF_MODULE_NAME_FD);
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
 *
 * @param apps is apps enabled?
 */
static void ebpf_fd_allocate_global_vectors(int apps)
{
    if (apps) {
        ebpf_fd_aral_init();
        fd_pid = callocz((size_t)pid_max, sizeof(netdata_fd_stat_t *));
        fd_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_fd_stat_t));
    }

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
    netdata_thread_cleanup_push(ebpf_fd_exit, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = fd_maps;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_fd_load_bpf(em))  {
        goto endfd;
    }

    ebpf_fd_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_FD_SYSCALL_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(fd_aggregated_data, fd_publish_aggregated, fd_dimension_names, fd_id_names,
                       algorithms, NETDATA_FD_SYSCALL_END);

    pthread_mutex_lock(&lock);
    ebpf_create_fd_global_charts(em);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
#ifdef NETDATA_DEV_MODE
    if (ebpf_aral_fd_pid)
        fd_disable_priority = ebpf_statistic_create_aral_chart(NETDATA_EBPF_FD_ARAL_NAME, em);
#endif

    pthread_mutex_unlock(&lock);

    fd_collector(em);

endfd:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
