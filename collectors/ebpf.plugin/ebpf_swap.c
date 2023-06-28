// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_swap.h"

static char *swap_dimension_name[NETDATA_SWAP_END] = { "read", "write" };
static netdata_syscall_stat_t swap_aggregated_data[NETDATA_SWAP_END];
static netdata_publish_syscall_t swap_publish_aggregated[NETDATA_SWAP_END];

static netdata_idx_t swap_hash_values[NETDATA_SWAP_END];
static netdata_idx_t *swap_values = NULL;

netdata_publish_swap_t *swap_vector = NULL;

struct config swap_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

static ebpf_local_maps_t swap_maps[] = {{.name = "tbl_pid_swap", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                         .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                        },
                                        {.name = "swap_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_CONTROLLER,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                         .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                        },
                                        {.name = "tbl_swap", .internal_input = NETDATA_SWAP_END,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                         .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                        },
                                        {.name = NULL, .internal_input = 0, .user_input = 0,
#ifdef LIBBPF_MAJOR_VERSION
                                         .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                        }};

netdata_ebpf_targets_t swap_targets[] = { {.name = "swap_readpage", .mode = EBPF_LOAD_TRAMPOLINE},
                                           {.name = "swap_writepage", .mode = EBPF_LOAD_TRAMPOLINE},
                                           {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects
 */
static void ebpf_swap_disable_probe(struct swap_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_swap_readpage_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_swap_writepage_probe, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_swap_disable_trampoline(struct swap_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_swap_readpage_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_swap_writepage_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_release_task_fentry, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_swap_set_trampoline_target(struct swap_bpf *obj)
{
    bpf_program__set_attach_target(obj->progs.netdata_swap_readpage_fentry, 0,
                                   swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name);

    bpf_program__set_attach_target(obj->progs.netdata_swap_writepage_fentry, 0,
                                   swap_targets[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name);

    bpf_program__set_attach_target(obj->progs.netdata_release_task_fentry, 0,
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
static int ebpf_swap_attach_kprobe(struct swap_bpf *obj)
{
    obj->links.netdata_swap_readpage_probe = bpf_program__attach_kprobe(obj->progs.netdata_swap_readpage_probe,
                                                                        false,
                                                                        swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name);
    int ret = libbpf_get_error(obj->links.netdata_swap_readpage_probe);
    if (ret)
        return -1;

    obj->links.netdata_swap_writepage_probe = bpf_program__attach_kprobe(obj->progs.netdata_swap_writepage_probe,
                                                                         false,
                                                                         swap_targets[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name);
    ret = libbpf_get_error(obj->links.netdata_swap_writepage_probe);
    if (ret)
        return -1;

    obj->links.netdata_release_task_probe = bpf_program__attach_kprobe(obj->progs.netdata_release_task_probe,
                                                                         false,
                                                                         EBPF_COMMON_FNCT_CLEAN_UP);
    ret = libbpf_get_error(obj->links.netdata_swap_writepage_probe);
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
static void ebpf_swap_set_hash_tables(struct swap_bpf *obj)
{
    swap_maps[NETDATA_PID_SWAP_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_pid_swap);
    swap_maps[NETDATA_SWAP_CONTROLLER].map_fd = bpf_map__fd(obj->maps.swap_ctrl);
    swap_maps[NETDATA_SWAP_GLOBAL_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_swap);
}

/**
 * Adjust Map
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_swap_adjust_map(struct swap_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(obj->maps.tbl_pid_swap, &swap_maps[NETDATA_PID_SWAP_TABLE],
                         em, bpf_map__name(obj->maps.tbl_pid_swap));

    ebpf_update_map_type(obj->maps.tbl_pid_swap, &swap_maps[NETDATA_PID_SWAP_TABLE]);
    ebpf_update_map_type(obj->maps.tbl_swap, &swap_maps[NETDATA_SWAP_GLOBAL_TABLE]);
    ebpf_update_map_type(obj->maps.swap_ctrl, &swap_maps[NETDATA_SWAP_CONTROLLER]);
}

/**
 *  Disable Release Task
 *
 *  Disable release task when apps is not enabled.
 *
 *  @param obj is the main structure for bpf objects.
 */
static void ebpf_swap_disable_release_task(struct swap_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_release_task_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_release_task_probe, false);
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
static inline int ebpf_swap_load_and_attach(struct swap_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_KEY_SWAP_READPAGE_CALL].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_swap_disable_probe(obj);

        ebpf_swap_set_trampoline_target(obj);
    } else {
        ebpf_swap_disable_trampoline(obj);
    }

    ebpf_swap_adjust_map(obj, em);

    if (!em->apps_charts && !em->cgroup_charts)
        ebpf_swap_disable_release_task(obj);

    int ret = swap_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ret = (test == EBPF_LOAD_TRAMPOLINE) ? swap_bpf__attach(obj) : ebpf_swap_attach_kprobe(obj);
    if (!ret) {
        ebpf_swap_set_hash_tables(obj);

        ebpf_update_controller(swap_maps[NETDATA_SWAP_CONTROLLER].map_fd, em);
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
 * Swap exit
 *
 * Cancel thread and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_swap_exit(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

#ifdef LIBBPF_MAJOR_VERSION
    if (bpf_obj)
        swap_bpf__destroy(bpf_obj);
#endif
    if (em->objects)
        ebpf_unload_legacy_code(em->objects, em->probe_links);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  COLLECTOR THREAD
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
static void swap_apps_accumulator(netdata_publish_swap_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_publish_swap_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_publish_swap_t *w = &out[i];
        total->write += w->write;
        total->read += w->read;
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
static void swap_fill_pid(uint32_t current_pid, netdata_publish_swap_t *publish)
{
    netdata_publish_swap_t *curr = swap_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_publish_swap_t));
        swap_pid[current_pid] = curr;
    }

    memcpy(curr, publish, sizeof(netdata_publish_swap_t));
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_swap_cgroup(int maps_per_core)
{
    ebpf_cgroup_target_t *ect ;
    netdata_publish_swap_t *cv = swap_vector;
    int fd = swap_maps[NETDATA_PID_SWAP_TABLE].map_fd;
    size_t length = sizeof(netdata_publish_swap_t);
    if (maps_per_core)
        length *= ebpf_nprocs;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_publish_swap_t *out = &pids->swap;
            if (likely(swap_pid) && swap_pid[pid]) {
                netdata_publish_swap_t *in = swap_pid[pid];

                memcpy(out, in, sizeof(netdata_publish_swap_t));
            } else {
                memset(cv, 0, length);
                if (!bpf_map_lookup_elem(fd, &pid, cv)) {
                    swap_apps_accumulator(cv, maps_per_core);

                    memcpy(out, cv, sizeof(netdata_publish_swap_t));

                    // We are cleaning to avoid passing data read from one process to other.
                    memset(cv, 0, length);
                }
            }
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
static void read_swap_apps_table(int maps_per_core)
{
    netdata_publish_swap_t *cv = swap_vector;
    uint32_t key;
    struct ebpf_pid_stat *pids = ebpf_root_of_pids;
    int fd = swap_maps[NETDATA_PID_SWAP_TABLE].map_fd;
    size_t length = sizeof(netdata_publish_swap_t);
    if (maps_per_core)
        length *= ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, cv)) {
            pids = pids->next;
            continue;
        }

        swap_apps_accumulator(cv, maps_per_core);

        swap_fill_pid(key, cv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);

        pids = pids->next;
    }
}

/**
* Send global
*
* Send global charts to Netdata
*/
static void swap_send_global()
{
    write_io_chart(NETDATA_MEM_SWAP_CHART, NETDATA_EBPF_SYSTEM_GROUP,
                   swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL].dimension,
                   (long long) swap_hash_values[NETDATA_KEY_SWAP_WRITEPAGE_CALL],
                   swap_publish_aggregated[NETDATA_KEY_SWAP_READPAGE_CALL].dimension,
                   (long long) swap_hash_values[NETDATA_KEY_SWAP_READPAGE_CALL]);
}

/**
 * Read global counter
 *
 * Read the table with number of calls to all functions
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_swap_read_global_table(int maps_per_core)
{
    netdata_idx_t *stored = swap_values;
    netdata_idx_t *val = swap_hash_values;
    int fd = swap_maps[NETDATA_SWAP_GLOBAL_TABLE].map_fd;

    uint32_t i, end = NETDATA_SWAP_END;
    for (i = NETDATA_KEY_SWAP_READPAGE_CALL; i < end; i++) {
        if (!bpf_map_lookup_elem(fd, &i, stored)) {
            int j;
            int last = (maps_per_core) ? ebpf_nprocs : 1;
            netdata_idx_t total = 0;
            for (j = 0; j < last; j++)
                total += stored[j];

            val[i] = total;
        }
    }
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param swap
 * @param root
 */
static void ebpf_swap_sum_pids(netdata_publish_swap_t *swap, struct ebpf_pid_on_target *root)
{
    uint64_t local_read = 0;
    uint64_t local_write = 0;

    while (root) {
        int32_t pid = root->pid;
        netdata_publish_swap_t *w = swap_pid[pid];
        if (w) {
            local_write += w->write;
            local_read += w->read;
        }
        root = root->next;
    }

    // These conditions were added, because we are using incremental algorithm
    swap->write = (local_write >= swap->write) ? local_write : swap->write;
    swap->read = (local_read >= swap->read) ? local_read : swap->read;
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_swap_send_apps_data(struct ebpf_target *root)
{
    struct ebpf_target *w;
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_swap_sum_pids(&w->swap, w->root_pid);
        }
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_MEM_SWAP_READ_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->swap.read);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_MEM_SWAP_WRITE_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->swap.write);
        }
    }
    write_end_chart();
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param swap
 * @param root
 */
static void ebpf_swap_sum_cgroup_pids(netdata_publish_swap_t *swap, struct pid_on_target2 *pids)
{
    uint64_t local_read = 0;
    uint64_t local_write = 0;

    while (pids) {
        netdata_publish_swap_t *w = &pids->swap;
        local_write += w->write;
        local_read += w->read;

        pids = pids->next;
    }

    // These conditions were added, because we are using incremental algorithm
    swap->write = (local_write >= swap->write) ? local_write : swap->write;
    swap->read = (local_read >= swap->read) ? local_read : swap->read;
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_swap_charts()
{
    ebpf_cgroup_target_t *ect;
    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_MEM_SWAP_READ_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long) ect->publish_systemd_swap.read);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_MEM_SWAP_WRITE_CHART);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long) ect->publish_systemd_swap.write);
        }
    }
    write_end_chart();
}

/**
 * Create specific swap charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_swap_charts(char *type, int update_every)
{
    ebpf_create_chart(type, NETDATA_MEM_SWAP_READ_CHART,
                      "Calls to function <code>swap_readpage</code>.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                      NETDATA_CGROUP_SWAP_READ_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5100,
                      ebpf_create_global_dimension,
                      swap_publish_aggregated, 1, update_every, NETDATA_EBPF_MODULE_NAME_SWAP);

    ebpf_create_chart(type, NETDATA_MEM_SWAP_WRITE_CHART,
                      "Calls to function <code>swap_writepage</code>.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                      NETDATA_CGROUP_SWAP_WRITE_CONTEXT, NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5101,
                      ebpf_create_global_dimension,
                      &swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SWAP);
}

/**
 * Create specific swap charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_swap_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(type, NETDATA_MEM_SWAP_READ_CHART,"Calls to function <code>swap_readpage</code>.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SWAP_READ_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5100, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_MEM_SWAP_WRITE_CHART, "Calls to function <code>swap_writepage</code>.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SWAP_WRITE_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5101, update_every);
}

/*
 * Send Specific Swap data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_swap_data(char *type, netdata_publish_swap_t *values)
{
    write_begin_chart(type, NETDATA_MEM_SWAP_READ_CHART);
    write_chart_dimension(swap_publish_aggregated[NETDATA_KEY_SWAP_READPAGE_CALL].name, (long long) values->read);
    write_end_chart();

    write_begin_chart(type, NETDATA_MEM_SWAP_WRITE_CHART);
    write_chart_dimension(swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name, (long long) values->write);
    write_end_chart();
}

/**
 *  Create Systemd Swap Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_swap_charts(int update_every)
{
    ebpf_create_charts_on_systemd(NETDATA_MEM_SWAP_READ_CHART,
                                  "Calls to <code>swap_readpage</code>.",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20191,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_SWAP_READ_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_SWAP, update_every);

    ebpf_create_charts_on_systemd(NETDATA_MEM_SWAP_WRITE_CHART,
                                  "Calls to function <code>swap_writepage</code>.",
                                  EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_STACKED, 20192,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX], NETDATA_SYSTEMD_SWAP_WRITE_CONTEXT,
                                  NETDATA_EBPF_MODULE_NAME_SWAP, update_every);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_swap_send_cgroup_data(int update_every)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_swap_sum_cgroup_pids(&ect->publish_systemd_swap, ect->pids);
    }

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;

    if (has_systemd) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_swap_charts(update_every);
            fflush(stdout);
        }
        ebpf_send_systemd_swap_charts();
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_SWAP_CHART) && ect->updated) {
            ebpf_create_specific_swap_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_SWAP_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_SWAP_CHART) {
            if (ect->updated) {
                ebpf_send_specific_swap_data(ect->name, &ect->publish_systemd_swap);
            } else {
                ebpf_obsolete_specific_swap_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_SWAP_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void swap_collector(ebpf_module_t *em)
{
    int cgroup = em->cgroup_charts;
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
        ebpf_swap_read_global_table(maps_per_core);
        pthread_mutex_lock(&collect_data_mutex);
        if (apps)
            read_swap_apps_table(maps_per_core);

        if (cgroup)
            ebpf_update_swap_cgroup(maps_per_core);

        pthread_mutex_lock(&lock);

        swap_send_global();

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_swap_send_apps_data(apps_groups_root_target);

        if (cgroup)
            ebpf_swap_send_cgroup_data(update_every);

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
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_swap_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct ebpf_target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_MEM_SWAP_READ_CHART,
                               "Calls to function <code>swap_readpage</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_SWAP_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20191,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SWAP);

    ebpf_create_charts_on_apps(NETDATA_MEM_SWAP_WRITE_CHART,
                               "Calls to function <code>swap_writepage</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_SWAP_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20192,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SWAP);
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
static void ebpf_swap_allocate_global_vectors(int apps)
{
    if (apps)
        swap_pid = callocz((size_t)pid_max, sizeof(netdata_publish_swap_t *));

    swap_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_publish_swap_t));

    swap_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(swap_hash_values, 0, sizeof(swap_hash_values));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_swap_charts(int update_every)
{
    ebpf_create_chart(NETDATA_EBPF_SYSTEM_GROUP, NETDATA_MEM_SWAP_CHART,
                      "Calls to access swap memory",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_SYSTEM_SWAP_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_SYSTEM_SWAP_CALLS,
                      ebpf_create_global_dimension,
                      swap_publish_aggregated, NETDATA_SWAP_END,
                      update_every, NETDATA_EBPF_MODULE_NAME_SWAP);
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_swap_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_KEY_SWAP_READPAGE_CALL].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        bpf_obj = swap_bpf__open();
        if (!bpf_obj)
            ret = -1;
        else
            ret = ebpf_swap_load_and_attach(bpf_obj, em);
    }
#endif

    if (ret)
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);

    return ret;
}

/**
 * SWAP thread
 *
 * Thread used to make swap thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_swap_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_swap_exit, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = swap_maps;

    ebpf_update_pid_table(&swap_maps[NETDATA_PID_SWAP_TABLE], em);

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
   if (ebpf_swap_load_bpf(em)) {
        goto endswap;
    }

    ebpf_swap_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_SWAP_END] = { NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX };
    ebpf_global_labels(swap_aggregated_data, swap_publish_aggregated, swap_dimension_name, swap_dimension_name,
                       algorithms, NETDATA_SWAP_END);

    pthread_mutex_lock(&lock);
    ebpf_create_swap_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps);
    pthread_mutex_unlock(&lock);

    swap_collector(em);

endswap:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
