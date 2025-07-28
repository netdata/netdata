// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_dcstat.h"

static char *dcstat_counter_dimension_name[NETDATA_DCSTAT_IDX_END] = {"ratio", "reference", "slow", "miss"};
static netdata_syscall_stat_t dcstat_counter_aggregated_data[NETDATA_DCSTAT_IDX_END];
static netdata_publish_syscall_t dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_END];

netdata_dcstat_pid_t *dcstat_vector = NULL;

static netdata_idx_t dcstat_hash_values[NETDATA_DCSTAT_IDX_END];
static netdata_idx_t *dcstat_values = NULL;

struct config dcstat_config = APPCONFIG_INITIALIZER;

ebpf_local_maps_t dcstat_maps[] = {
    {.name = "dcstat_global",
     .internal_input = NETDATA_DIRECTORY_CACHE_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "dcstat_pid",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "dcstat_ctrl",
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

static ebpf_specify_name_t dc_optional_name[] = {
    {.program_name = "netdata_lookup_fast",
     .function_to_attach = "lookup_fast",
     .optional = NULL,
     .retprobe = CONFIG_BOOLEAN_NO},
    {.program_name = NULL}};

netdata_ebpf_targets_t dc_targets[] = {
    {.name = "lookup_fast", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "d_lookup", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

struct netdata_static_thread ebpf_read_dcstat = {
    .name = "EBPF_READ_DCSTAT",
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
static inline void ebpf_dc_disable_probes(struct dc_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_lookup_fast_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_d_lookup_kretprobe, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_dc_disable_trampoline(struct dc_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_lookup_fast_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_d_lookup_fexit, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_dc_set_trampoline_target(struct dc_bpf *obj)
{
    bpf_program__set_attach_target(
        obj->progs.netdata_lookup_fast_fentry, 0, dc_targets[NETDATA_DC_TARGET_LOOKUP_FAST].name);

    bpf_program__set_attach_target(obj->progs.netdata_d_lookup_fexit, 0, dc_targets[NETDATA_DC_TARGET_D_LOOKUP].name);
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
static int ebpf_dc_attach_probes(struct dc_bpf *obj)
{
    obj->links.netdata_d_lookup_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_d_lookup_kretprobe, true, dc_targets[NETDATA_DC_TARGET_D_LOOKUP].name);
    long ret = libbpf_get_error(obj->links.netdata_d_lookup_kretprobe);
    if (ret)
        return -1;

    char *lookup_name = (dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional) ?
                            dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional :
                            dc_targets[NETDATA_DC_TARGET_LOOKUP_FAST].name;

    obj->links.netdata_lookup_fast_kprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_lookup_fast_kprobe, false, lookup_name);
    ret = libbpf_get_error(obj->links.netdata_lookup_fast_kprobe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Adjust Map Size
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_dc_adjust_map(struct dc_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(
        obj->maps.dcstat_pid, &dcstat_maps[NETDATA_DCSTAT_PID_STATS], em, bpf_map__name(obj->maps.dcstat_pid));

    ebpf_update_map_type(obj->maps.dcstat_global, &dcstat_maps[NETDATA_DCSTAT_GLOBAL_STATS]);
    ebpf_update_map_type(obj->maps.dcstat_pid, &dcstat_maps[NETDATA_DCSTAT_PID_STATS]);
    ebpf_update_map_type(obj->maps.dcstat_ctrl, &dcstat_maps[NETDATA_DCSTAT_CTRL]);
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_dc_set_hash_tables(struct dc_bpf *obj)
{
    dcstat_maps[NETDATA_DCSTAT_GLOBAL_STATS].map_fd = bpf_map__fd(obj->maps.dcstat_global);
    dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd = bpf_map__fd(obj->maps.dcstat_pid);
    dcstat_maps[NETDATA_DCSTAT_CTRL].map_fd = bpf_map__fd(obj->maps.dcstat_ctrl);
}

/**
 * Update Load
 *
 * For directory cache, some distributions change the function name, and we do not have condition to use
 * TRAMPOLINE like other functions.
 *
 * @param em  structure with configuration
 *
 * @return When then symbols were not modified, it returns TRAMPOLINE, else it returns RETPROBE.
 */
netdata_ebpf_program_loaded_t ebpf_dc_update_load(ebpf_module_t *em)
{
    if (!strcmp(
            dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional,
            dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].function_to_attach))
        return EBPF_LOAD_TRAMPOLINE;

    if (em->targets[NETDATA_DC_TARGET_LOOKUP_FAST].mode != EBPF_LOAD_RETPROBE)
        netdata_log_info(
            "When your kernel was compiled the symbol %s was modified, instead to use `trampoline`, the plugin will use `probes`.",
            dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].function_to_attach);

    return EBPF_LOAD_RETPROBE;
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
static inline int ebpf_dc_load_and_attach(struct dc_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_program_loaded_t test = ebpf_dc_update_load(em);
    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_dc_disable_probes(obj);

        ebpf_dc_set_trampoline_target(obj);
    } else {
        ebpf_dc_disable_trampoline(obj);
    }

    ebpf_dc_adjust_map(obj, em);

    int ret = dc_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ret = (test == EBPF_LOAD_TRAMPOLINE) ? dc_bpf__attach(obj) : ebpf_dc_attach_probes(obj);
    if (!ret) {
        ebpf_dc_set_hash_tables(obj);

        ebpf_update_controller(dcstat_maps[NETDATA_DCSTAT_CTRL].map_fd, em);
    }

    return ret;
}
#endif

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Update publish
 *
 * Update publish values before to write dimension.
 *
 * @param out           structure that will receive data.
 * @param cache_access  number of access to directory cache.
 * @param not_found     number of files not found on the file system
 */
void dcstat_update_publish(netdata_publish_dcstat_t *out, uint64_t cache_access, uint64_t not_found)
{
    NETDATA_DOUBLE successful_access = (NETDATA_DOUBLE)(((long long)cache_access) - ((long long)not_found));
    NETDATA_DOUBLE ratio = (cache_access) ? successful_access / (NETDATA_DOUBLE)cache_access : 0;

    out->ratio = (long long)(ratio * 100);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

static void ebpf_obsolete_specific_dc_charts(char *type, int update_every);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_dc_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_DC_HIT_CHART,
        "",
        "Percentage of files inside directory cache",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_DC_HIT_RATIO_CONTEXT,
        21200,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_DC_REFERENCE_CHART,
        "",
        "Count file access",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_DC_REFERENCE_CONTEXT,
        21201,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_DC_REQUEST_NOT_CACHE_CHART,
        "",
        "Files not present inside directory cache",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_DC_NOT_CACHE_CONTEXT,
        21202,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_DC_REQUEST_NOT_FOUND_CHART,
        "",
        "Files not found",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_DC_NOT_FOUND_CONTEXT,
        21203,
        em->update_every);
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_dc_cgroup_charts(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_dc_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_dc_charts(ect->name, em->update_every);
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
void ebpf_obsolete_dc_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_DCSTAT_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_dc_hit",
            "Percentage of files inside directory cache.",
            EBPF_COMMON_UNITS_PERCENTAGE,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "app.ebpf_dc_hit",
            20265,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_dc_reference",
            "Count file access.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_reference",
            20266,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_not_cache",
            "Files not present inside directory cache.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_not_cache",
            20267,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_not_found",
            "Files not found.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_not_found",
            20268,
            update_every);

        w->charts_created &= ~(1 << EBPF_MODULE_DCSTAT_IDX);
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
static void ebpf_obsolete_dc_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_DC_HIT_CHART,
        "",
        "Percentage of files inside directory cache",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_FS_DC_HIT_RATIO_CONTEXT,
        21200,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_DC_REFERENCE_CHART,
        "",
        "Variables used to calculate hit ratio.",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_FS_DC_REFERENCE_CONTEXT,
        21201,
        em->update_every);
}

/**
 * DCstat exit
 *
 * Cancel child and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_dcstat_exit(void *pptr)
{
    pids_fd[NETDATA_EBPF_PIDS_DCSTAT_IDX] = -1;
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    pthread_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_DCSTAT_IDX);
    pthread_mutex_unlock(&lock);

    if (ebpf_read_dcstat.thread)
        nd_thread_signal_cancel(ebpf_read_dcstat.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_dc_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_dc_apps_charts(em);
        }

        ebpf_obsolete_dc_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (dc_bpf_obj) {
        dc_bpf__destroy(dc_bpf_obj);
        dc_bpf_obj = NULL;
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
 *  APPS
 *
 *****************************************************************/

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_dcstat_apps_accumulator(netdata_dcstat_pid_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_dcstat_pid_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_dcstat_pid_t *w = &out[i];
        total->cache_access += w->cache_access;
        total->file_system += w->file_system;
        total->not_found += w->not_found;

        if (w->ct > ct)
            ct = w->ct;

        if (!total->name[0] && w->name[0])
            strncpyz(total->name, w->name, sizeof(total->name) - 1);
    }
    total->ct = ct;
}

/**
 * Read Directory Cache APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_dc_apps_table(int maps_per_core)
{
    netdata_dcstat_pid_t *cv = dcstat_vector;
    int fd = dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_dcstat_pid_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, cv)) {
            goto end_dc_loop;
        }

        ebpf_dcstat_apps_accumulator(cv, maps_per_core);

        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(key, NETDATA_EBPF_PIDS_DCSTAT_IDX);
        if (!local_pid)
            continue;
        netdata_publish_dcstat_t *publish = &local_pid->directory_cache;
        if (!publish->ct || publish->ct != cv->ct) {
            publish->ct = cv->ct;
            publish->curr.not_found = cv[0].not_found;
            publish->curr.file_system = cv[0].file_system;
            publish->curr.cache_access = cv[0].cache_access;
        } else {
            if (kill((pid_t)key, 0)) { // No PID found
                if (netdata_ebpf_reset_shm_pointer_unsafe(fd, key, NETDATA_EBPF_PIDS_DCSTAT_IDX))
                    memset(publish, 0, sizeof(*publish));
            }
        }

    end_dc_loop:
        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);
        key = next_key;
    }
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_dcstat_sum_pids(netdata_publish_dcstat_t *publish, struct ebpf_pid_on_target *root)
{
    memset(&publish->curr, 0, sizeof(netdata_publish_dcstat_pid_t));
    for (; root; root = root->next) {
        uint32_t pid = root->pid;
        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_DCSTAT_IDX);
        if (!local_pid)
            continue;
        netdata_publish_dcstat_t *w = &local_pid->directory_cache;

        publish->curr.cache_access += w->curr.cache_access;
        publish->curr.file_system += w->curr.file_system;
        publish->curr.not_found += w->curr.not_found;
    }
}

/**
 * Resume apps data
 */
void ebpf_dc_resume_apps_data()
{
    struct ebpf_target *w;

    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_DCSTAT_IDX))))
            continue;

        ebpf_dcstat_sum_pids(&w->dcstat, w->root_pid);

        uint64_t cache = w->dcstat.curr.cache_access;
        uint64_t not_found = w->dcstat.curr.not_found;

        dcstat_update_publish(&w->dcstat, cache, not_found);
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Update cgroup
 *
 * Update cgroup data based in collected PID.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_dc_cgroup()
{
    ebpf_cgroup_target_t *ect;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t pid = pids->pid;
            netdata_dcstat_pid_t *out = &pids->dc;
            netdata_ebpf_pid_stats_t *local_pid =
                netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_DCSTAT_IDX);
            if (!local_pid)
                continue;
            netdata_publish_dcstat_t *in = &local_pid->directory_cache;

            memcpy(out, &in->curr, sizeof(netdata_publish_dcstat_pid_t));
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
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
void ebpf_read_dcstat_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    int cgroups = em->cgroup_charts;
    if (!collect_pid)
        return;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    uint32_t running_time = 0;
    pids_fd[NETDATA_EBPF_PIDS_DCSTAT_IDX] = dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        sem_wait(shm_mutex_ebpf_integration);
        ebpf_read_dc_apps_table(maps_per_core);
        ebpf_dc_resume_apps_data();
        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_dc_cgroup();

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
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr)
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
            "_ebpf_dc_hit",
            "Percentage of files inside directory cache.",
            EBPF_COMMON_UNITS_PERCENTAGE,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "app.ebpf_dc_hit",
            20265,
            update_every,
            NETDATA_EBPF_MODULE_NAME_DCSTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION ratio '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_dc_reference",
            "Count file access.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_reference",
            20266,
            update_every,
            NETDATA_EBPF_MODULE_NAME_DCSTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION files '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_not_cache",
            "Files not present inside directory cache.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_not_cache",
            20267,
            update_every,
            NETDATA_EBPF_MODULE_NAME_DCSTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION files '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_not_found",
            "Files not found.",
            EBPF_COMMON_UNITS_FILES,
            NETDATA_DIRECTORY_CACHE_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_dc_not_found",
            20268,
            update_every,
            NETDATA_EBPF_MODULE_NAME_DCSTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION files '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        w->charts_created |= 1 << EBPF_MODULE_DCSTAT_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
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
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_dc_read_global_tables(netdata_idx_t *stats, int maps_per_core)
{
    ebpf_read_global_table_stats(
        dcstat_hash_values,
        dcstat_values,
        dcstat_maps[NETDATA_DCSTAT_GLOBAL_STATS].map_fd,
        maps_per_core,
        NETDATA_KEY_DC_REFERENCE,
        NETDATA_DIRECTORY_CACHE_END);

    ebpf_read_global_table_stats(
        stats,
        dcstat_values,
        dcstat_maps[NETDATA_DCSTAT_CTRL].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_dcache_send_apps_data(struct ebpf_target *root)
{
    struct ebpf_target *w;
    collected_number value;

    pthread_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_DCSTAT_IDX))))
            continue;

        value = (collected_number)w->dcstat.ratio;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_dc_hit");
        write_chart_dimension("ratio", value);
        ebpf_write_end_chart();

        if (w->dcstat.curr.cache_access < w->dcstat.prev.cache_access) {
            w->dcstat.prev.cache_access = 0;
        }
        w->dcstat.cache_access = (long long)w->dcstat.curr.cache_access - (long long)w->dcstat.prev.cache_access;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_dc_reference");
        value = (collected_number)w->dcstat.cache_access;
        write_chart_dimension("files", value);
        ebpf_write_end_chart();
        w->dcstat.prev.cache_access = w->dcstat.curr.cache_access;

        if (w->dcstat.curr.file_system < w->dcstat.prev.file_system) {
            w->dcstat.prev.file_system = 0;
        }
        value = (collected_number)(!w->dcstat.cache_access) ?
                    0 :
                    (long long)w->dcstat.curr.file_system - (long long)w->dcstat.prev.file_system;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_not_cache");
        write_chart_dimension("files", value);
        ebpf_write_end_chart();
        w->dcstat.prev.file_system = w->dcstat.curr.file_system;

        if (w->dcstat.curr.not_found < w->dcstat.prev.not_found) {
            w->dcstat.prev.not_found = 0;
        }
        value = (collected_number)(!w->dcstat.cache_access) ?
                    0 :
                    (long long)w->dcstat.curr.not_found - (long long)w->dcstat.prev.not_found;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_not_found");
        write_chart_dimension("files", value);
        ebpf_write_end_chart();
        w->dcstat.prev.not_found = w->dcstat.curr.not_found;
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Send global
 *
 * Send global charts to Netdata
 */
static void dcstat_send_global(netdata_publish_dcstat_t *publish)
{
    dcstat_update_publish(
        publish, dcstat_hash_values[NETDATA_KEY_DC_REFERENCE], dcstat_hash_values[NETDATA_KEY_DC_MISS]);

    netdata_publish_syscall_t *ptr = dcstat_counter_publish_aggregated;
    netdata_idx_t value = dcstat_hash_values[NETDATA_KEY_DC_REFERENCE];
    if (value != ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall) {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = value - ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall;
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_SLOW];
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall = value - ptr[NETDATA_DCSTAT_IDX_SLOW].pcall;
        ptr[NETDATA_DCSTAT_IDX_SLOW].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_MISS];
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = value - ptr[NETDATA_DCSTAT_IDX_MISS].pcall;
        ptr[NETDATA_DCSTAT_IDX_MISS].pcall = value;
    } else {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = 0;
    }

    ebpf_one_dimension_write_charts(
        NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART, ptr[NETDATA_DCSTAT_IDX_RATIO].dimension, publish->ratio);

    write_count_chart(
        NETDATA_DC_REFERENCE_CHART,
        NETDATA_FILESYSTEM_FAMILY,
        &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE],
        3);
}

/**
 * Create specific directory cache charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_dc_charts(char *type, int update_every)
{
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_DC_HIT_CHART,
        "Percentage of files inside directory cache",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_CGROUP_DC_HIT_RATIO_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5700,
        ebpf_create_global_dimension,
        dcstat_counter_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_DC_REFERENCE_CHART,
        "Count file access",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_CGROUP_DC_REFERENCE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5701,
        ebpf_create_global_dimension,
        &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_DC_REQUEST_NOT_CACHE_CHART,
        "Files not present inside directory cache",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_CGROUP_DC_NOT_CACHE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5702,
        ebpf_create_global_dimension,
        &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_SLOW],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_DC_REQUEST_NOT_FOUND_CHART,
        "Files not found",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_CGROUP_DC_NOT_FOUND_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5703,
        ebpf_create_global_dimension,
        &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_MISS],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
}

/**
 * Obsolete specific directory cache charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_dc_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_DC_HIT_CHART,
        "",
        "Percentage of files inside directory cache",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_DC_HIT_RATIO_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5700,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_DC_REFERENCE_CHART,
        "",
        "Count file access",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_DC_REFERENCE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5701,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_DC_REQUEST_NOT_CACHE_CHART,
        "",
        "Files not present inside directory cache",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_DC_NOT_CACHE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5702,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_DC_REQUEST_NOT_FOUND_CHART,
        "",
        "Files not found",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_DC_NOT_FOUND_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5703,
        update_every);
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_dc_sum_cgroup_pids(netdata_publish_dcstat_t *publish, struct pid_on_target2 *root)
{
    memset(&publish->curr, 0, sizeof(netdata_dcstat_pid_t));
    while (root) {
        netdata_dcstat_pid_t *src = &root->dc;

        publish->curr.cache_access += src->cache_access;
        publish->curr.file_system += src->file_system;
        publish->curr.not_found += src->not_found;

        root = root->next;
    }
}

/**
 * Calc chart values
 *
 * Do necessary math to plot charts.
 */
void ebpf_dc_calc_chart_values()
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_dc_sum_cgroup_pids(&ect->publish_dc, ect->pids);
        uint64_t cache = ect->publish_dc.curr.cache_access;
        uint64_t not_found = ect->publish_dc.curr.not_found;

        dcstat_update_publish(&ect->publish_dc, cache, not_found);

        ect->publish_dc.cache_access =
            (long long)ect->publish_dc.curr.cache_access - (long long)ect->publish_dc.prev.cache_access;
        ect->publish_dc.prev.cache_access = ect->publish_dc.curr.cache_access;

        if (ect->publish_dc.curr.not_found < ect->publish_dc.prev.not_found) {
            ect->publish_dc.prev.not_found = 0;
        }
    }
}

/**
 *  Create Systemd directory cache Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_dc_charts(int update_every)
{
    static ebpf_systemd_args_t data_dc_hit_ratio = {
        .title = "Percentage of files inside directory cache",
        .units = EBPF_COMMON_UNITS_PERCENTAGE,
        .family = NETDATA_DIRECTORY_CACHE_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21200,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_DC_HIT_RATIO_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_DCSTAT,
        .update_every = 0,
        .suffix = NETDATA_DC_HIT_CHART,
        .dimension = "percentage"};

    static ebpf_systemd_args_t data_dc_references = {
        .title = "Count file access",
        .units = EBPF_COMMON_UNITS_FILES,
        .family = NETDATA_DIRECTORY_CACHE_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21201,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_DC_REFERENCE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_DCSTAT,
        .update_every = 0,
        .suffix = NETDATA_DC_REFERENCE_CHART,
        .dimension = "files"};

    static ebpf_systemd_args_t data_dc_not_cache = {
        .title = "Files not present inside directory cache",
        .units = EBPF_COMMON_UNITS_FILES,
        .family = NETDATA_DIRECTORY_CACHE_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21202,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_DC_NOT_CACHE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_DCSTAT,
        .update_every = 0,
        .suffix = NETDATA_DC_REQUEST_NOT_CACHE_CHART,
        .dimension = "files"};

    static ebpf_systemd_args_t data_dc_not_found = {
        .title = "Files not found",
        .units = EBPF_COMMON_UNITS_FILES,
        .family = NETDATA_DIRECTORY_CACHE_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21203,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_DC_NOT_CACHE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_DCSTAT,
        .update_every = 0,
        .suffix = NETDATA_DC_REQUEST_NOT_FOUND_CHART,
        .dimension = "files"};

    if (!data_dc_not_cache.update_every)
        data_dc_hit_ratio.update_every = data_dc_not_cache.update_every = data_dc_not_found.update_every =
            data_dc_references.update_every = update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_DC_CHART))
            continue;

        data_dc_hit_ratio.id = data_dc_not_cache.id = data_dc_not_found.id = data_dc_references.id = w->name;
        ebpf_create_charts_on_systemd(&data_dc_hit_ratio);

        ebpf_create_charts_on_systemd(&data_dc_not_found);

        ebpf_create_charts_on_systemd(&data_dc_not_cache);

        ebpf_create_charts_on_systemd(&data_dc_references);

        w->flags |= NETDATA_EBPF_SERVICES_HAS_DC_CHART;
    }
}

/**
 * Send Directory Cache charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_dc_charts()
{
    ebpf_cgroup_target_t *ect;
    collected_number value;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_DC_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_DC_HIT_CHART, "");
        write_chart_dimension("percentage", (long long)ect->publish_dc.ratio);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_DC_REFERENCE_CHART, "");
        write_chart_dimension("files", (long long)ect->publish_dc.cache_access);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_DC_REQUEST_NOT_CACHE_CHART, "");
        value = (collected_number)(!ect->publish_dc.cache_access) ?
                    0 :
                    (long long)ect->publish_dc.curr.file_system - (long long)ect->publish_dc.prev.file_system;
        ect->publish_dc.prev.file_system = ect->publish_dc.curr.file_system;
        write_chart_dimension("files", (long long)value);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_DC_REQUEST_NOT_FOUND_CHART, "");
        value = (collected_number)(!ect->publish_dc.cache_access) ?
                    0 :
                    (long long)ect->publish_dc.curr.not_found - (long long)ect->publish_dc.prev.not_found;

        ect->publish_dc.prev.not_found = ect->publish_dc.curr.not_found;

        write_chart_dimension("files", (long long)value);
        ebpf_write_end_chart();
    }
}

/**
 * Send Directory Cache charts
 *
 * Send collected data to Netdata.
 *
 */
static void ebpf_send_specific_dc_data(char *type, netdata_publish_dcstat_t *pdc)
{
    collected_number value;
    ebpf_write_begin_chart(type, NETDATA_DC_HIT_CHART, "");
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_RATIO].name, (long long)pdc->ratio);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_DC_REFERENCE_CHART, "");
    write_chart_dimension(
        dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE].name, (long long)pdc->cache_access);
    ebpf_write_end_chart();

    value = (collected_number)(!pdc->cache_access) ?
                0 :
                (long long)pdc->curr.file_system - (long long)pdc->prev.file_system;
    pdc->prev.file_system = pdc->curr.file_system;

    ebpf_write_begin_chart(type, NETDATA_DC_REQUEST_NOT_CACHE_CHART, "");
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_SLOW].name, (long long)value);
    ebpf_write_end_chart();

    value =
        (collected_number)(!pdc->cache_access) ? 0 : (long long)pdc->curr.not_found - (long long)pdc->prev.not_found;
    pdc->prev.not_found = pdc->curr.not_found;

    ebpf_write_begin_chart(type, NETDATA_DC_REQUEST_NOT_FOUND_CHART, "");
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_MISS].name, (long long)value);
    ebpf_write_end_chart();
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_dc_send_cgroup_data(int update_every)
{
    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    ebpf_dc_calc_chart_values();

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_dc_charts(update_every);
        }

        ebpf_send_systemd_dc_charts();
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_DC_CHART) && ect->updated) {
            ebpf_create_specific_dc_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_DC_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_DC_CHART) {
            if (ect->updated) {
                ebpf_send_specific_dc_data(ect->name, &ect->publish_dc);
            } else {
                ebpf_obsolete_specific_dc_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_DC_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void dcstat_collector(ebpf_module_t *em)
{
    netdata_publish_dcstat_t publish;
    memset(&publish, 0, sizeof(publish));
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_dc_read_global_tables(stats, maps_per_core);

        pthread_mutex_lock(&lock);

        dcstat_send_global(&publish);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_dcache_send_apps_data(apps_groups_root_target);

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_dc_send_cgroup_data(update_every);

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
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Create filesystem charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_dc_global_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_DC_HIT_CHART,
        "Percentage of files inside directory cache",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_FS_DC_HIT_RATIO_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21200,
        ebpf_create_global_dimension,
        dcstat_counter_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_DC_REFERENCE_CHART,
        "Variables used to calculate hit ratio.",
        EBPF_COMMON_UNITS_FILES,
        NETDATA_DIRECTORY_CACHE_SUBMENU,
        NETDATA_FS_DC_REFERENCE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21201,
        ebpf_create_global_dimension,
        &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE],
        3,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DCSTAT);

    fflush(stdout);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_dcstat_allocate_global_vectors()
{
    dcstat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_dcstat_pid_t));
    dcstat_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(dcstat_counter_aggregated_data, 0, NETDATA_DCSTAT_IDX_END * sizeof(netdata_syscall_stat_t));
    memset(dcstat_counter_publish_aggregated, 0, NETDATA_DCSTAT_IDX_END * sizeof(netdata_publish_syscall_t));
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
static int ebpf_dcstat_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(dcstat_maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_DC_TARGET_LOOKUP_FAST].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        dc_bpf_obj = dc_bpf__open();
        if (!dc_bpf_obj)
            ret = -1;
        else
            ret = ebpf_dc_load_and_attach(dc_bpf_obj, em);
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
void ebpf_dcstat_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    CLEANUP_FUNCTION_REGISTER(ebpf_dcstat_exit) cleanup_ptr = em;

    em->maps = dcstat_maps;

    ebpf_update_pid_table(&dcstat_maps[NETDATA_DCSTAT_PID_STATS], em);

    ebpf_update_names(dc_optional_name, em);

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_dcstat_load_bpf(em)) {
        goto enddcstat;
    }

    ebpf_dcstat_allocate_global_vectors();

    int algorithms[NETDATA_DCSTAT_IDX_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX};

    ebpf_global_labels(
        dcstat_counter_aggregated_data,
        dcstat_counter_publish_aggregated,
        dcstat_counter_dimension_name,
        dcstat_counter_dimension_name,
        algorithms,
        NETDATA_DCSTAT_IDX_END);

    pthread_mutex_lock(&lock);
    ebpf_create_dc_global_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);

    pthread_mutex_unlock(&lock);

    ebpf_read_dcstat.thread =
        nd_thread_create(ebpf_read_dcstat.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_dcstat_thread, em);

    dcstat_collector(em);

enddcstat:
    ebpf_update_disabled_plugin_stats(em);
}
