// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_swap.h"

static char *swap_dimension_name[NETDATA_SWAP_END] = {"read", "write"};
static netdata_syscall_stat_t swap_aggregated_data[NETDATA_SWAP_END];
static netdata_publish_syscall_t swap_publish_aggregated[NETDATA_SWAP_END];

static netdata_idx_t swap_hash_values[NETDATA_SWAP_END];
static netdata_idx_t *swap_values = NULL;

netdata_ebpf_swap_t *swap_vector = NULL;

struct config swap_config = APPCONFIG_INITIALIZER;

static ebpf_local_maps_t swap_maps[] = {
    {.name = "tbl_pid_swap",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "swap_ctrl",
     .internal_input = NETDATA_CONTROLLER_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tbl_swap",
     .internal_input = NETDATA_SWAP_END,
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
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

netdata_ebpf_targets_t swap_targets[] = {
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "swap_writepage", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

static char *swap_read[] = {"swap_readpage", "swap_read_folio", NULL};

struct netdata_static_thread ebpf_read_swap = {
    .name = "EBPF_READ_SWAP",
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
static void ebpf_swap_disable_probe(struct swap_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_swap_readpage_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_swap_read_folio_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_swap_writepage_probe, false);
}

/**
 * Disable specific probe
 *
 * Disable specific probes according to available functions
 *
 * @param obj is the main structure for bpf objects
 */
static inline void ebpf_swap_disable_specific_probe(struct swap_bpf *obj)
{
    if (!strcmp(swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name, swap_read[NETDATA_KEY_SWAP_READPAGE_CALL])) {
        bpf_program__set_autoload(obj->progs.netdata_swap_read_folio_probe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_swap_readpage_probe, false);
    }
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
    bpf_program__set_autoload(obj->progs.netdata_swap_read_folio_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_swap_writepage_fentry, false);
}

/**
 * Disable specific trampoline
 *
 * Disable specific trampolines according to available functions
 *
 * @param obj is the main structure for bpf objects
 */
static inline void ebpf_swap_disable_specific_trampoline(struct swap_bpf *obj)
{
    if (!strcmp(swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name, swap_read[NETDATA_KEY_SWAP_READPAGE_CALL])) {
        bpf_program__set_autoload(obj->progs.netdata_swap_read_folio_fentry, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_swap_readpage_fentry, false);
    }
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
    bpf_program__set_attach_target(
        obj->progs.netdata_swap_readpage_fentry, 0, swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name);

    bpf_program__set_attach_target(
        obj->progs.netdata_swap_writepage_fentry, 0, swap_targets[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name);
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
    int ret;
    if (!strcmp(swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name, swap_read[NETDATA_KEY_SWAP_READPAGE_CALL])) {
        obj->links.netdata_swap_readpage_probe = bpf_program__attach_kprobe(
            obj->progs.netdata_swap_readpage_probe, false, swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name);
        ret = libbpf_get_error(obj->links.netdata_swap_readpage_probe);
    } else {
        obj->links.netdata_swap_read_folio_probe = bpf_program__attach_kprobe(
            obj->progs.netdata_swap_read_folio_probe, false, swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name);
        ret = libbpf_get_error(obj->links.netdata_swap_read_folio_probe);
    }
    if (ret)
        return -1;

    obj->links.netdata_swap_writepage_probe = bpf_program__attach_kprobe(
        obj->progs.netdata_swap_writepage_probe, false, swap_targets[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name);
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
    ebpf_update_map_size(
        obj->maps.tbl_pid_swap, &swap_maps[NETDATA_PID_SWAP_TABLE], em, bpf_map__name(obj->maps.tbl_pid_swap));

    ebpf_update_map_type(obj->maps.tbl_pid_swap, &swap_maps[NETDATA_PID_SWAP_TABLE]);
    ebpf_update_map_type(obj->maps.tbl_swap, &swap_maps[NETDATA_SWAP_GLOBAL_TABLE]);
    ebpf_update_map_type(obj->maps.swap_ctrl, &swap_maps[NETDATA_SWAP_CONTROLLER]);
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
        ebpf_swap_disable_specific_trampoline(obj);

        ebpf_swap_set_trampoline_target(obj);
    } else {
        ebpf_swap_disable_trampoline(obj);
        ebpf_swap_disable_specific_probe(obj);
    }

    ebpf_swap_adjust_map(obj, em);

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

static void ebpf_obsolete_specific_swap_charts(char *type, int update_every);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_swap_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_MEM_SWAP_READ_CHART,
        "",
        "Calls to function swap_readpage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_SWAP_READ_CONTEXT,
        20191,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_MEM_SWAP_WRITE_CHART,
        "",
        "Calls to function swap_writepage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SWAP_WRITE_CONTEXT,
        20192,
        em->update_every);
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_swap_cgroup_charts(ebpf_module_t *em)
{
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_swap_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_swap_charts(ect->name, em->update_every);
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
void ebpf_obsolete_swap_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SWAP_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_swap_readpage",
            "Calls to function swap_readpage.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_swap_readpage",
            20070,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_swap_writepage",
            "Calls to function swap_writepage.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_swap_writepage",
            20071,
            update_every);
        w->charts_created &= ~(1 << EBPF_MODULE_SWAP_IDX);
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
static void ebpf_obsolete_swap_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_MEM_SWAP_CHART,
        "",
        "Calls to access swap memory",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "mem.swapcalls",
        NETDATA_CHART_PRIO_MEM_SWAP_CALLS,
        em->update_every);
}

/**
 * Swap exit
 *
 * Cancel thread and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_swap_exit(void *ptr)
{
    pids_fd[NETDATA_EBPF_PIDS_SWAP_IDX] = -1;
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    pthread_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_SWAP_IDX);
    pthread_mutex_unlock(&lock);

    if (ebpf_read_swap.thread)
        nd_thread_signal_cancel(ebpf_read_swap.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_swap_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_swap_apps_charts(em);
        }

        ebpf_obsolete_swap_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (bpf_obj) {
        swap_bpf__destroy(bpf_obj);
        bpf_obj = NULL;
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
static void swap_apps_accumulator(netdata_ebpf_swap_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_ebpf_swap_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_ebpf_swap_t *w = &out[i];
        total->write += w->write;
        total->read += w->read;

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
 */
static void ebpf_update_swap_cgroup()
{
    ebpf_cgroup_target_t *ect;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t pid = pids->pid;
            netdata_publish_swap_t *out = &pids->swap;
            netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_SWAP_IDX);
            if (!local_pid)
                continue;
            netdata_publish_swap_t *in = &local_pid->swap;

            memcpy(out, in, sizeof(netdata_publish_swap_t));
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
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

    for (; root; root = root->next) {
        uint32_t pid = root->pid;
        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_SWAP_IDX);
        if (!local_pid)
            continue;
        netdata_publish_swap_t *w = &local_pid->swap;

        local_write += w->write;
        local_read += w->read;
    }

    // These conditions were added, because we are using incremental algorithm
    swap->write = (local_write >= swap->write) ? local_write : swap->write;
    swap->read = (local_read >= swap->read) ? local_read : swap->read;
}

/**
 * Resume apps data
 */
void ebpf_swap_resume_apps_data()
{
    struct ebpf_target *w;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SWAP_IDX))))
            continue;

        ebpf_swap_sum_pids(&w->swap, w->root_pid);
    }
    pthread_mutex_unlock(&collect_data_mutex);
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_swap_apps_table(int maps_per_core)
{
    netdata_ebpf_swap_t *cv = swap_vector;
    int fd = swap_maps[NETDATA_PID_SWAP_TABLE].map_fd;
    size_t length = sizeof(netdata_ebpf_swap_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, cv)) {
            goto end_swap_loop;
        }

        swap_apps_accumulator(cv, maps_per_core);

        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(key, NETDATA_EBPF_PIDS_SWAP_IDX);
        if (!local_pid)
            continue;
        netdata_publish_swap_t *publish = &local_pid->swap;

        if (!publish->ct || publish->ct != cv->ct) {
            memcpy(publish, cv, sizeof(netdata_publish_swap_t));
        } else {
            if (kill((pid_t)key, 0)) { // No PID found
                if (netdata_ebpf_reset_shm_pointer_unsafe(fd, key, NETDATA_EBPF_PIDS_SWAP_IDX))
                    memset(publish, 0, sizeof(*publish));
            }
        }

        // We are cleaning to avoid passing data read from one process to other.
    end_swap_loop:
        memset(cv, 0, length);
        key = next_key;
    }
}

/**
 * SWAP thread
 *
 * Thread used to generate swap charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_read_swap_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    if (!collect_pid)
        return;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    uint32_t running_time = 0;
    int cgroups = em->cgroup_charts;
    pids_fd[NETDATA_EBPF_PIDS_SWAP_IDX] = swap_maps[NETDATA_PID_SWAP_TABLE].map_fd;

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        sem_wait(shm_mutex_ebpf_integration);
        ebpf_read_swap_apps_table(maps_per_core);
        ebpf_swap_resume_apps_data();
        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_swap_cgroup();

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
* Send global
*
* Send global charts to Netdata
*/
static void swap_send_global()
{
    write_io_chart(
        NETDATA_MEM_SWAP_CHART,
        NETDATA_EBPF_MEMORY_GROUP,
        swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL].dimension,
        (long long)swap_hash_values[NETDATA_KEY_SWAP_WRITEPAGE_CALL],
        swap_publish_aggregated[NETDATA_KEY_SWAP_READPAGE_CALL].dimension,
        (long long)swap_hash_values[NETDATA_KEY_SWAP_READPAGE_CALL]);
}

/**
 * Read global counter
 *
 * Read the table with number of calls to all functions
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_swap_read_global_table(netdata_idx_t *stats, int maps_per_core)
{
    ebpf_read_global_table_stats(
        swap_hash_values,
        swap_values,
        swap_maps[NETDATA_SWAP_GLOBAL_TABLE].map_fd,
        maps_per_core,
        NETDATA_KEY_SWAP_READPAGE_CALL,
        NETDATA_SWAP_END);

    ebpf_read_global_table_stats(
        stats,
        swap_values,
        swap_maps[NETDATA_SWAP_CONTROLLER].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_swap_send_apps_data(struct ebpf_target *root)
{
    struct ebpf_target *w;
    pthread_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_SWAP_IDX))))
            continue;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_swap_readpage");
        write_chart_dimension("calls", (long long)w->swap.read);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_swap_writepage");
        write_chart_dimension("calls", (long long)w->swap.write);
        ebpf_write_end_chart();
    }
    pthread_mutex_unlock(&collect_data_mutex);
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
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_SWAP_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_MEM_SWAP_READ_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_systemd_swap.read);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_MEM_SWAP_WRITE_CHART, "");
        write_chart_dimension("calls", (long long)ect->publish_systemd_swap.write);
        ebpf_write_end_chart();
    }
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
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_MEM_SWAP_READ_CHART,
        "Calls to function swap_readpage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_CGROUP_SWAP_READ_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5100,
        ebpf_create_global_dimension,
        swap_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SWAP);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_MEM_SWAP_WRITE_CHART,
        "Calls to function swap_writepage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_CGROUP_SWAP_WRITE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5101,
        ebpf_create_global_dimension,
        &swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SWAP);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
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
    ebpf_write_chart_obsolete(
        type,
        NETDATA_MEM_SWAP_READ_CHART,
        "",
        "Calls to function swap_readpage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SWAP_READ_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5100,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_MEM_SWAP_WRITE_CHART,
        "",
        "Calls to function swap_writepage.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_SWAP_WRITE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5101,
        update_every);
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
    ebpf_write_begin_chart(type, NETDATA_MEM_SWAP_READ_CHART, "");
    write_chart_dimension(swap_publish_aggregated[NETDATA_KEY_SWAP_READPAGE_CALL].name, (long long)values->read);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_MEM_SWAP_WRITE_CHART, "");
    write_chart_dimension(swap_publish_aggregated[NETDATA_KEY_SWAP_WRITEPAGE_CALL].name, (long long)values->write);
    ebpf_write_end_chart();
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
    static ebpf_systemd_args_t data_read = {
        .title = "Calls to function swap_readpage.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_SYSTEM_SWAP_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20191,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SWAP_READ_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SWAP,
        .update_every = 0,
        .suffix = NETDATA_MEM_SWAP_READ_CHART,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_write = {
        .title = "Calls to function swap_writepage.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_SYSTEM_SWAP_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20192,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_SWAP_WRITE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_SWAP,
        .update_every = 0,
        .suffix = NETDATA_MEM_SWAP_WRITE_CHART,
        .dimension = "calls"};

    if (!data_write.update_every)
        data_read.update_every = data_write.update_every = update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_SWAP_CHART))
            continue;

        data_read.id = data_write.id = w->name;
        ebpf_create_charts_on_systemd(&data_read);

        ebpf_create_charts_on_systemd(&data_write);

        w->flags |= NETDATA_EBPF_SERVICES_HAS_SWAP_CHART;
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_swap_send_cgroup_data(int update_every)
{
    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_swap_sum_cgroup_pids(&ect->publish_systemd_swap, ect->pids);
    }

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_swap_charts(update_every);
            fflush(stdout);
        }
        ebpf_send_systemd_swap_charts();
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
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
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_swap_read_global_table(stats, maps_per_core);

        pthread_mutex_lock(&lock);

        swap_send_global();

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_swap_send_apps_data(apps_groups_root_target);

        if (cgroup && shm_ebpf_cgroup.header)
            ebpf_swap_send_cgroup_data(update_every);

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
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_swap_create_apps_charts(struct ebpf_module *em, void *ptr)
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
            "_ebpf_call_swap_readpage",
            "Calls to function swap_readpage.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_swap_readpage",
            20070,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SWAP);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_swap_writepage",
            "Calls to function swap_writepage.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_swap_writepage",
            20071,
            update_every,
            NETDATA_EBPF_MODULE_NAME_SWAP);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        w->charts_created |= 1 << EBPF_MODULE_SWAP_IDX;
    }
    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_swap_allocate_global_vectors()
{
    swap_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_ebpf_swap_t));

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
    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_MEM_SWAP_CHART,
        "Calls to access swap memory",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_SYSTEM_SWAP_SUBMENU,
        "mem.swapcalls",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_MEM_SWAP_CALLS,
        ebpf_create_global_dimension,
        swap_publish_aggregated,
        NETDATA_SWAP_END,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SWAP);

    fflush(stdout);
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
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Update Internal value
 *
 * Update values used during runtime.
 *
 * @return It returns 0 when one of the functions is present and -1 otherwise.
 */
static int ebpf_swap_set_internal_value()
{
    ebpf_addresses_t address = {.function = NULL, .hash = 0, .addr = 0};
    int i;
    for (i = 0; swap_read[i]; i++) {
        address.function = swap_read[i];
        ebpf_load_addresses(&address, -1);
        if (address.addr)
            break;
    }

    if (!address.addr) {
        netdata_log_error("%s swap.", NETDATA_EBPF_DEFAULT_FNT_NOT_FOUND);
        return -1;
    }

    swap_targets[NETDATA_KEY_SWAP_READPAGE_CALL].name = address.function;

    return 0;
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
void ebpf_swap_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_swap_exit) cleanup_ptr = em;

    em->maps = swap_maps;

    ebpf_update_pid_table(&swap_maps[NETDATA_PID_SWAP_TABLE], em);

    if (ebpf_swap_set_internal_value()) {
        goto endswap;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_swap_load_bpf(em)) {
        goto endswap;
    }

    ebpf_swap_allocate_global_vectors();

    int algorithms[NETDATA_SWAP_END] = {NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX};
    ebpf_global_labels(
        swap_aggregated_data,
        swap_publish_aggregated,
        swap_dimension_name,
        swap_dimension_name,
        algorithms,
        NETDATA_SWAP_END);

    pthread_mutex_lock(&lock);
    ebpf_create_swap_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    pthread_mutex_unlock(&lock);

    ebpf_read_swap.thread =
        nd_thread_create(ebpf_read_swap.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_swap_thread, em);

    swap_collector(em);

endswap:
    ebpf_update_disabled_plugin_stats(em);
}
