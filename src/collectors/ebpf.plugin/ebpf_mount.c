// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_mount.h"

static ebpf_local_maps_t mount_maps[] = {
    {.name = "tbl_mount",
     .internal_input = NETDATA_MOUNT_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
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

static char *mount_dimension_name[NETDATA_EBPF_MOUNT_SYSCALL] = {"mount", "umount"};
static netdata_syscall_stat_t mount_aggregated_data[NETDATA_EBPF_MOUNT_SYSCALL];
static netdata_publish_syscall_t mount_publish_aggregated[NETDATA_EBPF_MOUNT_SYSCALL];

struct config mount_config = APPCONFIG_INITIALIZER;

static netdata_idx_t mount_hash_values[NETDATA_MOUNT_END];

netdata_ebpf_targets_t mount_targets[] = {
    {.name = "mount", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "umount", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
/*****************************************************************
 *
 *  BTF FUNCTIONS
 *
 *****************************************************************/

/*
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_mount_disable_probe(struct mount_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_mount_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_umount_probe, false);

    bpf_program__set_autoload(obj->progs.netdata_mount_retprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_umount_retprobe, false);
}

/*
 * Disable tracepoint
 *
 * Disable all tracepoints to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_mount_disable_tracepoint(struct mount_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_mount_exit, false);
    bpf_program__set_autoload(obj->progs.netdata_umount_exit, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_mount_disable_trampoline(struct mount_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_mount_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_umount_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_mount_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_umount_fexit, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void netdata_set_trampoline_target(struct mount_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];
    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, mount_targets[NETDATA_MOUNT_SYSCALL].name, running_on_kernel);

    bpf_program__set_attach_target(obj->progs.netdata_mount_fentry, 0, syscall);

    bpf_program__set_attach_target(obj->progs.netdata_mount_fexit, 0, syscall);

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, mount_targets[NETDATA_UMOUNT_SYSCALL].name, running_on_kernel);

    bpf_program__set_attach_target(obj->progs.netdata_umount_fentry, 0, syscall);

    bpf_program__set_attach_target(obj->progs.netdata_umount_fexit, 0, syscall);
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
static int ebpf_mount_attach_probe(struct mount_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, mount_targets[NETDATA_MOUNT_SYSCALL].name, running_on_kernel);

    obj->links.netdata_mount_probe = bpf_program__attach_kprobe(obj->progs.netdata_mount_probe, false, syscall);
    int ret = (int)libbpf_get_error(obj->links.netdata_mount_probe);
    if (ret)
        return -1;

    obj->links.netdata_mount_retprobe = bpf_program__attach_kprobe(obj->progs.netdata_mount_retprobe, true, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_mount_retprobe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(
        syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH, mount_targets[NETDATA_UMOUNT_SYSCALL].name, running_on_kernel);

    obj->links.netdata_umount_probe = bpf_program__attach_kprobe(obj->progs.netdata_umount_probe, false, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_umount_probe);
    if (ret)
        return -1;

    obj->links.netdata_umount_retprobe = bpf_program__attach_kprobe(obj->progs.netdata_umount_retprobe, true, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_umount_retprobe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_mount_set_hash_tables(struct mount_bpf *obj)
{
    mount_maps[NETDATA_KEY_MOUNT_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_mount);
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
static inline int ebpf_mount_load_and_attach(struct mount_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_MOUNT_SYSCALL].mode;

    // We are testing only one, because all will have the same behavior
    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_mount_disable_probe(obj);
        ebpf_mount_disable_tracepoint(obj);

        netdata_set_trampoline_target(obj);
    } else if (test == EBPF_LOAD_PROBE || test == EBPF_LOAD_RETPROBE) {
        ebpf_mount_disable_tracepoint(obj);
        ebpf_mount_disable_trampoline(obj);
    } else {
        ebpf_mount_disable_probe(obj);
        ebpf_mount_disable_trampoline(obj);
    }

    ebpf_update_map_type(obj->maps.tbl_mount, &mount_maps[NETDATA_KEY_MOUNT_TABLE]);

    int ret = mount_bpf__load(obj);
    if (!ret) {
        if (test != EBPF_LOAD_PROBE && test != EBPF_LOAD_RETPROBE)
            ret = mount_bpf__attach(obj);
        else
            ret = ebpf_mount_attach_probe(obj);

        if (!ret)
            ebpf_mount_set_hash_tables(obj);
    }

    return ret;
}
#endif
/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_mount_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        NETDATA_EBPF_MOUNT_CALLS,
        "",
        "Calls to mount and umount syscalls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_EBPF_MOUNT_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "mount_points.call",
        NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        NETDATA_EBPF_MOUNT_ERRORS,
        "",
        "Errors to mount and umount file systems",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_EBPF_MOUNT_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "mount_points.error",
        NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS + 1,
        em->update_every);
}

/**
 * Mount Exit
 *
 * Cancel child thread.
 *
 * @param ptr thread data.
 */
static void ebpf_mount_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);

        ebpf_obsolete_mount_global(em);

        fflush(stdout);
        netdata_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (mount_bpf_obj) {
        mount_bpf__destroy(mount_bpf_obj);
        mount_bpf_obj = NULL;
    }
#endif
    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    netdata_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Read global table
 *
 * Read the table with number of calls for all functions
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_mount_read_global_table(int maps_per_core)
{
    static netdata_idx_t *mount_values = NULL;
    if (!mount_values)
        mount_values = callocz((size_t)ebpf_nprocs + 1, sizeof(netdata_idx_t));

    uint32_t idx;
    netdata_idx_t *val = mount_hash_values;
    netdata_idx_t *stored = mount_values;
    size_t length = sizeof(netdata_idx_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    int fd = mount_maps[NETDATA_KEY_MOUNT_TABLE].map_fd;

    for (idx = NETDATA_KEY_MOUNT_CALL; idx < NETDATA_MOUNT_END; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, stored)) {
            int i;
            int end = (maps_per_core) ? ebpf_nprocs : 1;
            netdata_idx_t total = 0;
            for (i = 0; i < end; i++)
                total += stored[i];

            val[idx] = total;
            memset(stored, 0, length);
        }
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
*/
static void ebpf_mount_send_data()
{
    int i, j;
    int end = NETDATA_EBPF_MOUNT_SYSCALL;
    for (i = NETDATA_KEY_MOUNT_CALL, j = NETDATA_KEY_MOUNT_ERROR; i < end; i++, j++) {
        mount_publish_aggregated[i].ncall = mount_hash_values[i];
        mount_publish_aggregated[i].nerr = mount_hash_values[j];
    }

    write_count_chart(
        NETDATA_EBPF_MOUNT_CALLS,
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        mount_publish_aggregated,
        NETDATA_EBPF_MOUNT_SYSCALL);

    write_err_chart(
        NETDATA_EBPF_MOUNT_ERRORS,
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        mount_publish_aggregated,
        NETDATA_EBPF_MOUNT_SYSCALL);
}

/**
* Main loop for this collector.
*/
static void mount_collector(ebpf_module_t *em)
{
    memset(mount_hash_values, 0, sizeof(mount_hash_values));

    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        ebpf_mount_read_global_table(maps_per_core);
        netdata_mutex_lock(&lock);

        ebpf_mount_send_data();

        netdata_mutex_unlock(&lock);

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Create mount charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_mount_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        NETDATA_EBPF_MOUNT_CALLS,
        "Calls to mount and umount syscalls",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_EBPF_MOUNT_FAMILY,
        "mount_points.call",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS,
        ebpf_create_global_dimension,
        mount_publish_aggregated,
        NETDATA_EBPF_MOUNT_SYSCALL,
        update_every,
        NETDATA_EBPF_MODULE_NAME_MOUNT);

    ebpf_create_chart(
        NETDATA_EBPF_MOUNT_GLOBAL_FAMILY,
        NETDATA_EBPF_MOUNT_ERRORS,
        "Errors to mount and umount file systems",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_EBPF_MOUNT_FAMILY,
        "mount_points.error",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_EBPF_MOUNT_CHARTS + 1,
        ebpf_create_global_dimension,
        mount_publish_aggregated,
        NETDATA_EBPF_MOUNT_SYSCALL,
        update_every,
        NETDATA_EBPF_MODULE_NAME_MOUNT);

    fflush(stdout);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_mount_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        mount_bpf_obj = mount_bpf__open();
        if (!mount_bpf_obj)
            ret = -1;
        else
            ret = ebpf_mount_load_and_attach(mount_bpf_obj, em);
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Mount thread
 *
 * Thread used to make mount thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always returns NULL
 */
void ebpf_mount_thread(void *ptr)
{
    ebpf_module_t *em = ptr;
    CLEANUP_FUNCTION_REGISTER(ebpf_mount_exit) cleanup_ptr = em;

    em->maps = mount_maps;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_mount_load_bpf(em)) {
        goto endmount;
    }

    int algorithms[NETDATA_EBPF_MOUNT_SYSCALL] = {NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX};

    ebpf_global_labels(
        mount_aggregated_data,
        mount_publish_aggregated,
        mount_dimension_name,
        mount_dimension_name,
        algorithms,
        NETDATA_EBPF_MOUNT_SYSCALL);

    netdata_mutex_lock(&lock);
    ebpf_create_mount_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    netdata_mutex_unlock(&lock);

    mount_collector(em);

endmount:
    ebpf_update_disabled_plugin_stats(em);
}
