// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_dcstat.h"

static char *dcstat_counter_dimension_name[NETDATA_DCSTAT_IDX_END] = { "ratio", "reference", "slow", "miss" };
static netdata_syscall_stat_t dcstat_counter_aggregated_data[NETDATA_DCSTAT_IDX_END];
static netdata_publish_syscall_t dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_END];

netdata_dcstat_pid_t *dcstat_vector = NULL;

static netdata_idx_t dcstat_hash_values[NETDATA_DCSTAT_IDX_END];
static netdata_idx_t *dcstat_values = NULL;

struct config dcstat_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

ebpf_local_maps_t dcstat_maps[] = {{.name = "dcstat_global", .internal_input = NETDATA_DIRECTORY_CACHE_END,
                                    .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                    .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                    .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                   },
                                   {.name = "dcstat_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                    .user_input = 0,
                                    .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                    .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                    .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                   },
                                   {.name = "dcstat_ctrl", .internal_input = NETDATA_CONTROLLER_END,
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

static ebpf_specify_name_t dc_optional_name[] = { {.program_name = "netdata_lookup_fast",
                                                   .function_to_attach = "lookup_fast",
                                                   .optional = NULL,
                                                   .retprobe = CONFIG_BOOLEAN_NO},
                                                  {.program_name = NULL}};

netdata_ebpf_targets_t dc_targets[] = { {.name = "lookup_fast", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = "d_lookup", .mode = EBPF_LOAD_TRAMPOLINE},
                                        {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

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
    bpf_program__set_autoload(obj->progs.netdata_dcstat_release_task_kprobe, false);
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
    bpf_program__set_autoload(obj->progs.netdata_dcstat_release_task_fentry, false);
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
    bpf_program__set_attach_target(obj->progs.netdata_lookup_fast_fentry, 0,
                                   dc_targets[NETDATA_DC_TARGET_LOOKUP_FAST].name);

    bpf_program__set_attach_target(obj->progs.netdata_d_lookup_fexit, 0,
                                   dc_targets[NETDATA_DC_TARGET_D_LOOKUP].name);

    bpf_program__set_attach_target(obj->progs.netdata_dcstat_release_task_fentry, 0,
                                   EBPF_COMMON_FNCT_CLEAN_UP);
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
    obj->links.netdata_d_lookup_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_d_lookup_kretprobe,
                                                                       true,
                                                                       dc_targets[NETDATA_DC_TARGET_D_LOOKUP].name);
    int ret = libbpf_get_error(obj->links.netdata_d_lookup_kretprobe);
    if (ret)
        return -1;

    char *lookup_name = (dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional) ?
        dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional :
        dc_targets[NETDATA_DC_TARGET_LOOKUP_FAST].name ;

    obj->links.netdata_lookup_fast_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_lookup_fast_kprobe,
                                                                       false,
                                                                       lookup_name);
    ret = libbpf_get_error(obj->links.netdata_lookup_fast_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_dcstat_release_task_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_dcstat_release_task_kprobe,
                                                                       false,
                                                                       EBPF_COMMON_FNCT_CLEAN_UP);
    ret = libbpf_get_error(obj->links.netdata_dcstat_release_task_kprobe);
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
    ebpf_update_map_size(obj->maps.dcstat_pid, &dcstat_maps[NETDATA_DCSTAT_PID_STATS],
                         em, bpf_map__name(obj->maps.dcstat_pid));

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
    if (!strcmp(dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].optional,
                dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].function_to_attach))
        return EBPF_LOAD_TRAMPOLINE;

    if (em->targets[NETDATA_DC_TARGET_LOOKUP_FAST].mode != EBPF_LOAD_RETPROBE)
        info("When your kernel was compiled the symbol %s was modified, instead to use `trampoline`, the plugin will use `probes`.",
             dc_optional_name[NETDATA_DC_TARGET_LOOKUP_FAST].function_to_attach);

    return EBPF_LOAD_RETPROBE;
}

/**
 *  Disable Release Task
 *
 *  Disable release task when apps is not enabled.
 *
 *  @param obj is the main structure for bpf objects.
 */
static void ebpf_dc_disable_release_task(struct dc_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_dcstat_release_task_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_dcstat_release_task_fentry, false);
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
    netdata_ebpf_program_loaded_t test =  ebpf_dc_update_load(em);
    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_dc_disable_probes(obj);

        ebpf_dc_set_trampoline_target(obj);
    } else {
        ebpf_dc_disable_trampoline(obj);
    }

    ebpf_dc_adjust_map(obj, em);

    if (!em->apps_charts && !em->cgroup_charts)
        ebpf_dc_disable_release_task(obj);

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
    NETDATA_DOUBLE successful_access = (NETDATA_DOUBLE) (((long long)cache_access) - ((long long)not_found));
    NETDATA_DOUBLE ratio = (cache_access) ? successful_access/(NETDATA_DOUBLE)cache_access : 0;

    out->ratio = (long long )(ratio*100);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * DCstat exit
 *
 * Cancel child and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_dcstat_exit(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

#ifdef LIBBPF_MAJOR_VERSION
    if (dc_bpf_obj)
        dc_bpf__destroy(dc_bpf_obj);
#endif

    if (em->objects)
        ebpf_unload_legacy_code(em->objects, em->probe_links);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  APPS
 *
 *****************************************************************/

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
    ebpf_create_charts_on_apps(NETDATA_DC_HIT_CHART,
                               "Percentage of files inside directory cache",
                               EBPF_COMMON_DIMENSION_PERCENTAGE,
                               NETDATA_DIRECTORY_CACHE_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_LINE,
                               20100,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_charts_on_apps(NETDATA_DC_REFERENCE_CHART,
                               "Count file access",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_DIRECTORY_CACHE_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20101,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_charts_on_apps(NETDATA_DC_REQUEST_NOT_CACHE_CHART,
                               "Files not present inside directory cache",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_DIRECTORY_CACHE_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20102,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_charts_on_apps(NETDATA_DC_REQUEST_NOT_FOUND_CHART,
                               "Files not found",
                               EBPF_COMMON_DIMENSION_FILES,
                               NETDATA_DIRECTORY_CACHE_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20103,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/*****************************************************************
 *
 *  MAIN LOOP
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
static void dcstat_apps_accumulator(netdata_dcstat_pid_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_dcstat_pid_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_dcstat_pid_t *w = &out[i];
        total->cache_access += w->cache_access;
        total->file_system += w->file_system;
        total->not_found += w->not_found;
    }
}

/**
 * Save PID values
 *
 * Save the current values inside the structure
 *
 * @param out     vector used to plot charts
 * @param publish vector with values read from hash tables.
 */
static inline void dcstat_save_pid_values(netdata_publish_dcstat_t *out, netdata_dcstat_pid_t *publish)
{
    memcpy(&out->curr, &publish[0], sizeof(netdata_dcstat_pid_t));
}

/**
 * Fill PID
 *
 * Fill PID structures
 *
 * @param current_pid pid that we are collecting data
 * @param out         values read from hash tables;
 */
static void dcstat_fill_pid(uint32_t current_pid, netdata_dcstat_pid_t *publish)
{
    netdata_publish_dcstat_t *curr = dcstat_pid[current_pid];
    if (!curr) {
        curr = ebpf_publish_dcstat_get();
        dcstat_pid[current_pid] = curr;
    }

    dcstat_save_pid_values(curr, publish);
}

/**
 * Read Directory Cache APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void read_dc_apps_table(int maps_per_core)
{
    netdata_dcstat_pid_t *cv = dcstat_vector;
    uint32_t key;
    struct ebpf_pid_stat *pids = ebpf_root_of_pids;
    int fd = dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_dcstat_pid_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, cv)) {
            pids = pids->next;
            continue;
        }

        dcstat_apps_accumulator(cv, maps_per_core);

        dcstat_fill_pid(key, cv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);

        pids = pids->next;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in collected PID.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_dc_cgroup(int maps_per_core)
{
    netdata_dcstat_pid_t *cv = dcstat_vector;
    int fd = dcstat_maps[NETDATA_DCSTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_dcstat_pid_t)*ebpf_nprocs;

    ebpf_cgroup_target_t *ect;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_dcstat_pid_t *out = &pids->dc;
            if (likely(dcstat_pid) && dcstat_pid[pid]) {
                netdata_publish_dcstat_t *in = dcstat_pid[pid];

                memcpy(out, &in->curr, sizeof(netdata_dcstat_pid_t));
            } else {
                memset(cv, 0, length);
                if (bpf_map_lookup_elem(fd, &pid, cv)) {
                    continue;
                }

                dcstat_apps_accumulator(cv, maps_per_core);

                memcpy(out, cv, sizeof(netdata_dcstat_pid_t));
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Read global table
 *
 * Read the table with number of calls for all functions
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_dc_read_global_table(int maps_per_core)
{
    uint32_t idx;
    netdata_idx_t *val = dcstat_hash_values;
    netdata_idx_t *stored = dcstat_values;
    int fd = dcstat_maps[NETDATA_DCSTAT_GLOBAL_STATS].map_fd;

    for (idx = NETDATA_KEY_DC_REFERENCE; idx < NETDATA_DIRECTORY_CACHE_END; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, stored)) {
            int i;
            int end = (maps_per_core) ? ebpf_nprocs: 1;
            netdata_idx_t total = 0;
            for (i = 0; i < end; i++)
                total += stored[i];

            val[idx] = total;
        }
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
    memset(&publish->curr, 0, sizeof(netdata_dcstat_pid_t));
    netdata_dcstat_pid_t *dst = &publish->curr;
    while (root) {
        int32_t pid = root->pid;
        netdata_publish_dcstat_t *w = dcstat_pid[pid];
        if (w) {
            netdata_dcstat_pid_t *src = &w->curr;
            dst->cache_access += src->cache_access;
            dst->file_system += src->file_system;
            dst->not_found += src->not_found;
        }

        root = root->next;
    }
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

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_HIT_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_dcstat_sum_pids(&w->dcstat, w->root_pid);

            uint64_t cache = w->dcstat.curr.cache_access;
            uint64_t not_found = w->dcstat.curr.not_found;

            dcstat_update_publish(&w->dcstat, cache, not_found);
            value = (collected_number) w->dcstat.ratio;
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REFERENCE_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.cache_access < w->dcstat.prev.cache_access) {
                w->dcstat.prev.cache_access = 0;
            }

            w->dcstat.cache_access = (long long)w->dcstat.curr.cache_access - (long long)w->dcstat.prev.cache_access;
            value = (collected_number) w->dcstat.cache_access;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.cache_access = w->dcstat.curr.cache_access;
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REQUEST_NOT_CACHE_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.file_system < w->dcstat.prev.file_system) {
                w->dcstat.prev.file_system = 0;
            }

            value = (collected_number) (!w->dcstat.cache_access) ? 0 :
                    (long long )w->dcstat.curr.file_system - (long long)w->dcstat.prev.file_system;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.file_system = w->dcstat.curr.file_system;
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_DC_REQUEST_NOT_FOUND_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            if (w->dcstat.curr.not_found < w->dcstat.prev.not_found) {
                w->dcstat.prev.not_found = 0;
            }
            value = (collected_number) (!w->dcstat.cache_access) ? 0 :
                    (long long)w->dcstat.curr.not_found - (long long)w->dcstat.prev.not_found;
            write_chart_dimension(w->name, value);
            w->dcstat.prev.not_found = w->dcstat.curr.not_found;
        }
    }
    write_end_chart();
}

/**
 * Send global
 *
 * Send global charts to Netdata
 */
static void dcstat_send_global(netdata_publish_dcstat_t *publish)
{
    dcstat_update_publish(publish, dcstat_hash_values[NETDATA_KEY_DC_REFERENCE],
                          dcstat_hash_values[NETDATA_KEY_DC_MISS]);

    netdata_publish_syscall_t *ptr = dcstat_counter_publish_aggregated;
    netdata_idx_t value = dcstat_hash_values[NETDATA_KEY_DC_REFERENCE];
    if (value != ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall) {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = value - ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall;
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_SLOW];
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall =  value - ptr[NETDATA_DCSTAT_IDX_SLOW].pcall;
        ptr[NETDATA_DCSTAT_IDX_SLOW].pcall = value;

        value = dcstat_hash_values[NETDATA_KEY_DC_MISS];
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = value - ptr[NETDATA_DCSTAT_IDX_MISS].pcall;
        ptr[NETDATA_DCSTAT_IDX_MISS].pcall = value;
    } else {
        ptr[NETDATA_DCSTAT_IDX_REFERENCE].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_SLOW].ncall = 0;
        ptr[NETDATA_DCSTAT_IDX_MISS].ncall = 0;
    }

    ebpf_one_dimension_write_charts(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                                    ptr[NETDATA_DCSTAT_IDX_RATIO].dimension, publish->ratio);

    write_count_chart(
        NETDATA_DC_REFERENCE_CHART, NETDATA_FILESYSTEM_FAMILY,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 3);
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
    ebpf_create_chart(type, NETDATA_DC_HIT_CHART, "Percentage of files inside directory cache",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NETDATA_CGROUP_DC_HIT_RATIO_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5700,
                      ebpf_create_global_dimension,
                      dcstat_counter_publish_aggregated, 1, update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_chart(type, NETDATA_DC_REFERENCE_CHART, "Count file access",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NETDATA_CGROUP_DC_REFERENCE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5701,
                      ebpf_create_global_dimension,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_chart(type, NETDATA_DC_REQUEST_NOT_CACHE_CHART,
                      "Files not present inside directory cache",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NETDATA_CGROUP_DC_NOT_CACHE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5702,
                      ebpf_create_global_dimension,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_SLOW], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_chart(type, NETDATA_DC_REQUEST_NOT_FOUND_CHART,
                      "Files not found",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NETDATA_CGROUP_DC_NOT_FOUND_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5703,
                      ebpf_create_global_dimension,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_MISS], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);
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
    ebpf_write_chart_obsolete(type, NETDATA_DC_HIT_CHART,
                              "Percentage of files inside directory cache",
                              EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_DIRECTORY_CACHE_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_DC_HIT_RATIO_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5700, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_DC_REFERENCE_CHART,
                              "Count file access",
                              EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_DC_REFERENCE_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5701, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_DC_REQUEST_NOT_CACHE_CHART,
                              "Files not present inside directory cache",
                              EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_DC_NOT_CACHE_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5702, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_DC_REQUEST_NOT_FOUND_CHART,
                              "Files not found",
                              EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_DC_NOT_FOUND_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5703, update_every);
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
    netdata_dcstat_pid_t *dst = &publish->curr;
    while (root) {
        netdata_dcstat_pid_t *src = &root->dc;

        dst->cache_access += src->cache_access;
        dst->file_system += src->file_system;
        dst->not_found += src->not_found;

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
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_dc_sum_cgroup_pids(&ect->publish_dc, ect->pids);
        uint64_t cache = ect->publish_dc.curr.cache_access;
        uint64_t not_found = ect->publish_dc.curr.not_found;

        dcstat_update_publish(&ect->publish_dc, cache, not_found);

        ect->publish_dc.cache_access = (long long)ect->publish_dc.curr.cache_access -
            (long long)ect->publish_dc.prev.cache_access;
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
    ebpf_create_charts_on_systemd(NETDATA_DC_HIT_CHART,
                                  "Percentage of files inside directory cache",
                                  EBPF_COMMON_DIMENSION_PERCENTAGE,
                                  NETDATA_DIRECTORY_CACHE_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  21200,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_DC_HIT_RATIO_CONTEXT, NETDATA_EBPF_MODULE_NAME_DCSTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_DC_REFERENCE_CHART,
                                  "Count file access",
                                  EBPF_COMMON_DIMENSION_FILES,
                                  NETDATA_DIRECTORY_CACHE_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  21201,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_DC_REFERENCE_CONTEXT, NETDATA_EBPF_MODULE_NAME_DCSTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_DC_REQUEST_NOT_CACHE_CHART,
                                  "Files not present inside directory cache",
                                  EBPF_COMMON_DIMENSION_FILES,
                                  NETDATA_DIRECTORY_CACHE_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  21202,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_DC_NOT_CACHE_CONTEXT, NETDATA_EBPF_MODULE_NAME_DCSTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_DC_REQUEST_NOT_FOUND_CHART,
                                  "Files not found",
                                  EBPF_COMMON_DIMENSION_FILES,
                                  NETDATA_DIRECTORY_CACHE_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  21202,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_DC_NOT_FOUND_CONTEXT, NETDATA_EBPF_MODULE_NAME_DCSTAT,
                                  update_every);
}

/**
 * Send Directory Cache charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_dc_charts()
{
    collected_number value;
    ebpf_cgroup_target_t *ect;
    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_DC_HIT_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long) ect->publish_dc.ratio);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_DC_REFERENCE_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long) ect->publish_dc.cache_access);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_DC_REQUEST_NOT_CACHE_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            value = (collected_number) (!ect->publish_dc.cache_access) ? 0 :
                (long long )ect->publish_dc.curr.file_system - (long long)ect->publish_dc.prev.file_system;
            ect->publish_dc.prev.file_system = ect->publish_dc.curr.file_system;

            write_chart_dimension(ect->name, (long long) value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_DC_REQUEST_NOT_FOUND_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            value = (collected_number) (!ect->publish_dc.cache_access) ? 0 :
                (long long)ect->publish_dc.curr.not_found - (long long)ect->publish_dc.prev.not_found;

            ect->publish_dc.prev.not_found = ect->publish_dc.curr.not_found;

            write_chart_dimension(ect->name, (long long) value);
        }
    }
    write_end_chart();
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
    write_begin_chart(type, NETDATA_DC_HIT_CHART);
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_RATIO].name,
                          (long long) pdc->ratio);
    write_end_chart();

    write_begin_chart(type, NETDATA_DC_REFERENCE_CHART);
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE].name,
                          (long long) pdc->cache_access);
    write_end_chart();

    value = (collected_number) (!pdc->cache_access) ? 0 :
        (long long )pdc->curr.file_system - (long long)pdc->prev.file_system;
    pdc->prev.file_system = pdc->curr.file_system;

    write_begin_chart(type, NETDATA_DC_REQUEST_NOT_CACHE_CHART);
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_SLOW].name, (long long) value);
    write_end_chart();

    value = (collected_number) (!pdc->cache_access) ? 0 :
        (long long)pdc->curr.not_found - (long long)pdc->prev.not_found;
    pdc->prev.not_found = pdc->curr.not_found;

    write_begin_chart(type, NETDATA_DC_REQUEST_NOT_FOUND_CHART);
    write_chart_dimension(dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_MISS].name, (long long) value);
    write_end_chart();
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_dc_send_cgroup_data(int update_every)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    ebpf_dc_calc_chart_values();

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;
    if (has_systemd) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_dc_charts(update_every);
        }

        ebpf_send_systemd_dc_charts();
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
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
    heartbeat_init(&hb);
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    while (!ebpf_exit_plugin) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);

        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_dc_read_global_table(maps_per_core);
        pthread_mutex_lock(&collect_data_mutex);
        if (apps)
            read_dc_apps_table(maps_per_core);

        if (cgroups)
            ebpf_update_dc_cgroup(maps_per_core);

        pthread_mutex_lock(&lock);

        dcstat_send_global(&publish);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_dcache_send_apps_data(apps_groups_root_target);

#ifdef NETDATA_DEV_MODE
        if (ebpf_aral_dcstat_pid)
            ebpf_send_data_aral_chart(ebpf_aral_dcstat_pid, em);
#endif

        if (cgroups)
            ebpf_dc_send_cgroup_data(update_every);

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
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
static void ebpf_create_filesystem_charts(int update_every)
{
    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_HIT_CHART,
                      "Percentage of files inside directory cache",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21200,
                      ebpf_create_global_dimension,
                      dcstat_counter_publish_aggregated, 1, update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, NETDATA_DC_REFERENCE_CHART,
                      "Variables used to calculate hit ratio.",
                      EBPF_COMMON_DIMENSION_FILES, NETDATA_DIRECTORY_CACHE_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21201,
                      ebpf_create_global_dimension,
                      &dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_REFERENCE], 3,
                      update_every, NETDATA_EBPF_MODULE_NAME_DCSTAT);

    fflush(stdout);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param apps is apps enabled?
 */
static void ebpf_dcstat_allocate_global_vectors(int apps)
{
    if (apps) {
        ebpf_dcstat_aral_init();
        dcstat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_dcstat_t *));
        dcstat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_dcstat_pid_t));
    }

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
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);

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
void *ebpf_dcstat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_dcstat_exit, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = dcstat_maps;

    ebpf_update_pid_table(&dcstat_maps[NETDATA_DCSTAT_PID_STATS], em);

    ebpf_update_names(dc_optional_name, em);

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_dcstat_load_bpf(em)) {
        goto enddcstat;
    }

    ebpf_dcstat_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_DCSTAT_IDX_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX,
        NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(dcstat_counter_aggregated_data, dcstat_counter_publish_aggregated,
                       dcstat_counter_dimension_name, dcstat_counter_dimension_name,
                       algorithms, NETDATA_DCSTAT_IDX_END);

    pthread_mutex_lock(&lock);
    ebpf_create_filesystem_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps);
#ifdef NETDATA_DEV_MODE
    if (ebpf_aral_dcstat_pid)
        ebpf_statistic_create_aral_chart(NETDATA_EBPF_DCSTAT_ARAL_NAME, em);
#endif

    pthread_mutex_unlock(&lock);

    dcstat_collector(em);

enddcstat:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
