// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_cachestat.h"

static char *cachestat_counter_dimension_name[NETDATA_CACHESTAT_END] = {"ratio", "dirty", "hit", "miss"};
static netdata_syscall_stat_t cachestat_counter_aggregated_data[NETDATA_CACHESTAT_END];
static netdata_publish_syscall_t cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_END];

netdata_cachestat_pid_t *cachestat_vector = NULL;

static netdata_idx_t cachestat_hash_values[NETDATA_CACHESTAT_END];
static netdata_idx_t *cachestat_values = NULL;

ebpf_local_maps_t cachestat_maps[] = {
    {.name = "cstat_global",
     .internal_input = NETDATA_CACHESTAT_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "cstat_pid",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "cstat_ctrl",
     .internal_input = NETDATA_CONTROLLER_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {
        .name = NULL,
        .internal_input = 0,
        .user_input = 0,
        .type = NETDATA_EBPF_MAP_CONTROLLER,
        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
#endif
    }};

struct config cachestat_config = APPCONFIG_INITIALIZER;

netdata_ebpf_targets_t cachestat_targets[] = {
    {.name = "add_to_page_cache_lru", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "mark_page_accessed", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "mark_buffer_dirty", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

static char *account_page[NETDATA_CACHESTAT_ACCOUNT_DIRTY_END] = {
    "account_page_dirtied",
    "__set_page_dirty",
    "__folio_mark_dirty"};

struct netdata_static_thread ebpf_read_cachestat = {
    .name = "EBPF_READ_CACHESTAT",
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
static void ebpf_cachestat_disable_probe(struct cachestat_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_add_to_page_cache_lru_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_mark_page_accessed_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_mark_buffer_dirty_kprobe, false);
}

/*
 * Disable specific probe
 *
 * Disable probes according the kernel version
 *
 * @param obj is the main structure for bpf objects
 */
static void ebpf_cachestat_disable_specific_probe(struct cachestat_bpf *obj)
{
    if (!strcmp(
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
            account_page[NETDATA_CACHESTAT_FOLIO_DIRTY])) {
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_kprobe, false);
    } else if (!strcmp(
                   cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
                   account_page[NETDATA_CACHESTAT_SET_PAGE_DIRTY])) {
        bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_kprobe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_kprobe, false);
    }
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_cachestat_disable_trampoline(struct cachestat_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_add_to_page_cache_lru_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_mark_page_accessed_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_mark_buffer_dirty_fentry, false);
}

/*
 * Disable specific trampoline
 *
 * Disable trampoline according to kernel version.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_cachestat_disable_specific_trampoline(struct cachestat_bpf *obj)
{
    if (!strcmp(
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
            account_page[NETDATA_CACHESTAT_FOLIO_DIRTY])) {
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_fentry, false);
    } else if (!strcmp(
                   cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
                   account_page[NETDATA_CACHESTAT_SET_PAGE_DIRTY])) {
        bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_fentry, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_folio_mark_dirty_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_fentry, false);
    }
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void netdata_set_trampoline_target(struct cachestat_bpf *obj)
{
    bpf_program__set_attach_target(
        obj->progs.netdata_add_to_page_cache_lru_fentry,
        0,
        cachestat_targets[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].name);

    bpf_program__set_attach_target(
        obj->progs.netdata_mark_page_accessed_fentry, 0, cachestat_targets[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED].name);

    if (!strcmp(
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
            account_page[NETDATA_CACHESTAT_FOLIO_DIRTY])) {
        bpf_program__set_attach_target(
            obj->progs.netdata_folio_mark_dirty_fentry,
            0,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    } else if (!strcmp(
                   cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
                   account_page[NETDATA_CACHESTAT_SET_PAGE_DIRTY])) {
        bpf_program__set_attach_target(
            obj->progs.netdata_set_page_dirty_fentry,
            0,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    } else {
        bpf_program__set_attach_target(
            obj->progs.netdata_account_page_dirtied_fentry,
            0,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    }

    bpf_program__set_attach_target(
        obj->progs.netdata_mark_buffer_dirty_fentry, 0, cachestat_targets[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY].name);
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
static int ebpf_cachestat_attach_probe(struct cachestat_bpf *obj)
{
    obj->links.netdata_add_to_page_cache_lru_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_add_to_page_cache_lru_kprobe,
        false,
        cachestat_targets[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].name);
    long ret = libbpf_get_error(obj->links.netdata_add_to_page_cache_lru_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_mark_page_accessed_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_mark_page_accessed_kprobe,
        false,
        cachestat_targets[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED].name);
    ret = libbpf_get_error(obj->links.netdata_mark_page_accessed_kprobe);
    if (ret)
        return -1;

    if (!strcmp(
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
            account_page[NETDATA_CACHESTAT_FOLIO_DIRTY])) {
        obj->links.netdata_folio_mark_dirty_kprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_folio_mark_dirty_kprobe,
            false,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_folio_mark_dirty_kprobe);
    } else if (!strcmp(
                   cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name,
                   account_page[NETDATA_CACHESTAT_SET_PAGE_DIRTY])) {
        obj->links.netdata_set_page_dirty_kprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_set_page_dirty_kprobe,
            false,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_set_page_dirty_kprobe);
    } else {
        obj->links.netdata_account_page_dirtied_kprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_account_page_dirtied_kprobe,
            false,
            cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_account_page_dirtied_kprobe);
    }

    if (ret)
        return -1;

    obj->links.netdata_mark_buffer_dirty_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_mark_buffer_dirty_kprobe,
        false,
        cachestat_targets[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY].name);
    ret = libbpf_get_error(obj->links.netdata_mark_buffer_dirty_kprobe);
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
static void ebpf_cachestat_adjust_map(struct cachestat_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(
        obj->maps.cstat_pid, &cachestat_maps[NETDATA_CACHESTAT_PID_STATS], em, bpf_map__name(obj->maps.cstat_pid));

    ebpf_update_map_type(obj->maps.cstat_global, &cachestat_maps[NETDATA_CACHESTAT_GLOBAL_STATS]);
    ebpf_update_map_type(obj->maps.cstat_pid, &cachestat_maps[NETDATA_CACHESTAT_PID_STATS]);
    ebpf_update_map_type(obj->maps.cstat_ctrl, &cachestat_maps[NETDATA_CACHESTAT_CTRL]);
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_cachestat_set_hash_tables(struct cachestat_bpf *obj)
{
    cachestat_maps[NETDATA_CACHESTAT_GLOBAL_STATS].map_fd = bpf_map__fd(obj->maps.cstat_global);
    cachestat_maps[NETDATA_CACHESTAT_PID_STATS].map_fd = bpf_map__fd(obj->maps.cstat_pid);
    cachestat_maps[NETDATA_CACHESTAT_CTRL].map_fd = bpf_map__fd(obj->maps.cstat_ctrl);
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
static inline int ebpf_cachestat_load_and_attach(struct cachestat_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_cachestat_disable_probe(obj);
        ebpf_cachestat_disable_specific_trampoline(obj);

        netdata_set_trampoline_target(obj);
    } else {
        ebpf_cachestat_disable_trampoline(obj);
        ebpf_cachestat_disable_specific_probe(obj);
    }

    ebpf_cachestat_adjust_map(obj, em);

    int ret = cachestat_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ret = (test == EBPF_LOAD_TRAMPOLINE) ? cachestat_bpf__attach(obj) : ebpf_cachestat_attach_probe(obj);
    if (!ret) {
        ebpf_cachestat_set_hash_tables(obj);

        ebpf_update_controller(cachestat_maps[NETDATA_CACHESTAT_CTRL].map_fd, em);
    }

    return ret;
}
#endif
/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

static void ebpf_obsolete_specific_cachestat_charts(char *type, int update_every);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_cachestat_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        "",
        "Hit ratio",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_CACHESTAT_HIT_RATIO_CONTEXT,
        21100,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_CACHESTAT_DIRTY_CHART,
        "",
        "Number of dirty pages",
        EBPF_CACHESTAT_UNITS_PAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        21101,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_CACHESTAT_HIT_CHART,
        "",
        "Number of accessed files",
        EBPF_CACHESTAT_UNITS_HITS,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_CACHESTAT_HIT_FILE_CONTEXT,
        21102,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_CACHESTAT_MISSES_CHART,
        "",
        "Files out of page cache",
        EBPF_CACHESTAT_UNITS_MISSES,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_SYSTEMD_CACHESTAT_MISS_FILES_CONTEXT,
        21103,
        em->update_every);
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_cachestat_cgroup_charts(ebpf_module_t *em)
{
    netdata_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_cachestat_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_cachestat_charts(ect->name, em->update_every);
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_cachestat_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        "",
        "Hit ratio",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_MEM_CACHESTAT_HIT_RATIO_CONTEXT,
        21100,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_DIRTY_CHART,
        "",
        "Number of dirty pages",
        EBPF_CACHESTAT_UNITS_PAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_MEM_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        21101,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_HIT_CHART,
        "",
        "Number of accessed files",
        EBPF_CACHESTAT_UNITS_HITS,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_MEM_CACHESTAT_HIT_FILES_CONTEXT,
        21102,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_MISSES_CHART,
        "",
        "Files out of page cache",
        EBPF_CACHESTAT_UNITS_MISSES,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_MEM_CACHESTAT_MISS_FILES_CONTEXT,
        21103,
        em->update_every);
}

/**
 * Obsolette apps charts
 *
 * Obsolete apps charts.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_obsolete_cachestat_apps_charts(struct ebpf_module *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_CACHESTAT_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_hit_ratio",
            "Hit ratio",
            EBPF_COMMON_UNITS_PERCENTAGE,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "app.ebpf_cachestat_hit_ratio",
            20260,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_dirty_pages",
            "Number of dirty pages",
            EBPF_CACHESTAT_UNITS_PAGE,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_cachestat_dirty_pages",
            20261,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_access",
            "Number of accessed files",
            EBPF_CACHESTAT_UNITS_HITS,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_cachestat_access",
            20262,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_misses",
            "Files out of page cache",
            EBPF_CACHESTAT_UNITS_MISSES,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_cachestat_misses",
            20263,
            update_every);
        w->charts_created &= ~(1 << EBPF_MODULE_CACHESTAT_IDX);
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Cachestat exit.
 *
 * Cancel child and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_cachestat_exit(void *pptr)
{
    pids_fd[NETDATA_EBPF_PIDS_CACHESTAT_IDX] = -1;
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    netdata_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_CACHESTAT_IDX);
    netdata_mutex_unlock(&lock);

    if (ebpf_read_cachestat.thread)
        nd_thread_signal_cancel(ebpf_read_cachestat.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_cachestat_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_cachestat_apps_charts(em);
        }

        ebpf_obsolete_cachestat_global(em);

        fflush(stdout);
        netdata_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (cachestat_bpf_obj) {
        cachestat_bpf__destroy(cachestat_bpf_obj);
        cachestat_bpf_obj = NULL;
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
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Update publish
 *
 * Update publish values before to write dimension.
 *
 * @param out  structure that will receive data.
 * @param mpa  calls for mark_page_accessed during the last second.
 * @param mbd  calls for mark_buffer_dirty during the last second.
 * @param apcl calls for add_to_page_cache_lru during the last second.
 * @param apd  calls for account_page_dirtied during the last second.
 */
void cachestat_update_publish(netdata_publish_cachestat_t *out, uint64_t mpa, uint64_t mbd, uint64_t apcl, uint64_t apd)
{
    // Adapted algorithm from https://github.com/iovisor/bcc/blob/master/tools/cachestat.py#L126-L138
    NETDATA_DOUBLE total = (NETDATA_DOUBLE)(((long long)mpa) - ((long long)mbd));
    if (total < 0)
        total = 0;

    NETDATA_DOUBLE misses = (NETDATA_DOUBLE)(((long long)apcl) - ((long long)apd));
    if (misses < 0)
        misses = 0;

    // If hits are < 0, then its possible misses are overestimate due to possibly page cache read ahead adding
    // more pages than needed. In this case just assume misses as total and reset hits.
    NETDATA_DOUBLE hits = total - misses;
    if (hits < 0) {
        misses = total;
        hits = 0;
    }

    NETDATA_DOUBLE ratio = (total > 0) ? hits / total : 1;

    out->ratio = (long long)(ratio * 100);
    out->hit = (long long)hits;
    out->miss = (long long)misses;
}

/**
 * Save previous values
 *
 * Save values used this time.
 *
 * @param publish
 */
static void save_previous_values(netdata_publish_cachestat_t *publish)
{
    publish->prev.mark_page_accessed = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED];
    publish->prev.account_page_dirtied = cachestat_hash_values[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED];
    publish->prev.add_to_page_cache_lru = cachestat_hash_values[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU];
    publish->prev.mark_buffer_dirty = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY];
}

/**
 * Calculate statistics
 *
 * @param publish the structure where we will store the data.
 */
static void calculate_stats(netdata_publish_cachestat_t *publish)
{
    if (!publish->prev.mark_page_accessed) {
        save_previous_values(publish);
        return;
    }

    uint64_t mpa = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED] - publish->prev.mark_page_accessed;
    uint64_t mbd = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY] - publish->prev.mark_buffer_dirty;
    uint64_t apcl =
        cachestat_hash_values[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU] - publish->prev.add_to_page_cache_lru;
    uint64_t apd = cachestat_hash_values[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED] - publish->prev.account_page_dirtied;

    save_previous_values(publish);

    // We are changing the original algorithm to have a smooth ratio.
    cachestat_update_publish(publish, mpa, mbd, apcl, apd);
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
static void cachestat_apps_accumulator(netdata_cachestat_pid_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_cachestat_pid_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_cachestat_pid_t *w = &out[i];
        total->account_page_dirtied += w->account_page_dirtied;
        total->add_to_page_cache_lru += w->add_to_page_cache_lru;
        total->mark_buffer_dirty += w->mark_buffer_dirty;
        total->mark_page_accessed += w->mark_page_accessed;
        if (w->ct > ct)
            ct = w->ct;

        if (!total->name[0] && w->name[0])
            strncpyz(total->name, w->name, sizeof(total->name) - 1);
    }
    total->ct = ct;
}

/**
 * Save Pid values
 *
 * Save the current values inside the structure
 *
 * @param out     vector used to plot charts
 * @param in vector with values read from hash tables.
 */
static inline void cachestat_save_pid_values(netdata_publish_cachestat_t *out, netdata_cachestat_pid_t *in)
{
    out->ct = in->ct;
    if (out->current.mark_page_accessed) {
        memcpy(&out->prev, &out->current, sizeof(netdata_cachestat_t));
    }

    out->current.account_page_dirtied = in[0].account_page_dirtied;
    out->current.add_to_page_cache_lru = in[0].add_to_page_cache_lru;
    out->current.mark_buffer_dirty = in[0].mark_buffer_dirty;
    out->current.mark_page_accessed = in[0].mark_page_accessed;
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_read_cachestat_apps_table(int maps_per_core)
{
    netdata_cachestat_pid_t *cv = cachestat_vector;
    int fd = cachestat_maps[NETDATA_CACHESTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_cachestat_pid_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, cv)) {
            goto end_cachestat_loop;
        }

        cachestat_apps_accumulator(cv, maps_per_core);

        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(key, NETDATA_EBPF_PIDS_CACHESTAT_IDX);
        if (!local_pid)
            continue;
        netdata_publish_cachestat_t *publish = &local_pid->cachestat;

        if (!publish->ct || publish->ct != cv->ct) {
            cachestat_save_pid_values(publish, cv);
        } else {
            if (kill((pid_t)key, 0)) { // No PID found
                if (netdata_ebpf_reset_shm_pointer_unsafe(fd, key, NETDATA_EBPF_PIDS_CACHESTAT_IDX))
                    memset(publish, 0, sizeof(*publish));
            }
        }

    end_cachestat_loop:
        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);
        key = next_key;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 *
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_update_cachestat_cgroup()
{
    ebpf_cgroup_target_t *ect;
    netdata_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t pid = pids->pid;
            netdata_publish_cachestat_t *out = &pids->cachestat;

            netdata_ebpf_pid_stats_t *local_pid =
                netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_CACHESTAT_IDX);
            if (!local_pid)
                continue;

            netdata_publish_cachestat_t *in = &local_pid->cachestat;
            memcpy(&out->current, &in->current, sizeof(netdata_cachestat_t));
        }
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_cachestat_sum_pids(netdata_publish_cachestat_t *publish, struct ebpf_pid_on_target *root)
{
    memcpy(&publish->prev, &publish->current, sizeof(publish->current));
    memset(&publish->current, 0, sizeof(publish->current));

    netdata_cachestat_t *dst = &publish->current;
    for (; root; root = root->next) {
        uint32_t pid = root->pid;
        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_CACHESTAT_IDX);
        if (!local_pid)
            continue;
        netdata_publish_cachestat_t *w = &local_pid->cachestat;

        netdata_cachestat_t *src = &w->current;
        dst->account_page_dirtied += src->account_page_dirtied;
        dst->add_to_page_cache_lru += src->add_to_page_cache_lru;
        dst->mark_buffer_dirty += src->mark_buffer_dirty;
        dst->mark_page_accessed += src->mark_page_accessed;
    }
}

/**
 * Resume apps data
 */
void ebpf_cachestat_resume_apps_data()
{
    struct ebpf_target *w;

    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_CACHESTAT_IDX))))
            continue;

        ebpf_cachestat_sum_pids(&w->cachestat, w->root_pid);
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Cachestat thread
 *
 * Thread used to generate cachestat charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_read_cachestat_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    if (!collect_pid)
        return;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int cgroups = em->cgroup_charts;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    uint32_t running_time = 0;
    pids_fd[NETDATA_EBPF_PIDS_CACHESTAT_IDX] = cachestat_maps[NETDATA_CACHESTAT_PID_STATS].map_fd;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        sem_wait(shm_mutex_ebpf_integration);
        ebpf_read_cachestat_apps_table(maps_per_core);
        ebpf_cachestat_resume_apps_data();
        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_cachestat_cgroup();
        sem_post(shm_mutex_ebpf_integration);

        counter = 0;

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_cachestat_create_apps_charts(struct ebpf_module *em, void *ptr)
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
            "_ebpf_cachestat_hit_ratio",
            "Hit ratio",
            EBPF_COMMON_UNITS_PERCENTAGE,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "app.ebpf_cachestat_hit_ratio",
            20260,
            update_every,
            NETDATA_EBPF_MODULE_NAME_CACHESTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION ratio '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_dirty_pages",
            "Number of dirty pages",
            EBPF_CACHESTAT_UNITS_PAGE,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "app.ebpf_cachestat_dirty_pages",
            20261,
            update_every,
            NETDATA_EBPF_MODULE_NAME_CACHESTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION pages '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_access",
            "Number of accessed files",
            EBPF_CACHESTAT_UNITS_HITS,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_cachestat_access",
            20262,
            update_every,
            NETDATA_EBPF_MODULE_NAME_CACHESTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION hits '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_cachestat_misses",
            "Files out of page cache",
            EBPF_CACHESTAT_UNITS_MISSES,
            NETDATA_CACHESTAT_SUBMENU,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_cachestat_misses",
            20263,
            update_every,
            NETDATA_EBPF_MODULE_NAME_CACHESTAT);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION misses '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
        w->charts_created |= 1 << EBPF_MODULE_CACHESTAT_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/*****************************************************************
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_cachestat_read_global_tables(netdata_idx_t *stats, int maps_per_core)
{
    ebpf_read_global_table_stats(
        cachestat_hash_values,
        cachestat_values,
        cachestat_maps[NETDATA_CACHESTAT_GLOBAL_STATS].map_fd,
        maps_per_core,
        NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU,
        NETDATA_CACHESTAT_END);

    ebpf_read_global_table_stats(
        stats,
        cachestat_values,
        cachestat_maps[NETDATA_CACHESTAT_CTRL].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);
}

/**
 * Send global
 *
 * Send global charts to Netdata
 */
static void cachestat_send_global(netdata_publish_cachestat_t *publish)
{
    calculate_stats(publish);

    netdata_publish_syscall_t *ptr = cachestat_counter_publish_aggregated;
    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        ptr[NETDATA_CACHESTAT_IDX_RATIO].dimension,
        publish->ratio);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_DIRTY_CHART,
        ptr[NETDATA_CACHESTAT_IDX_DIRTY].dimension,
        (long long)cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY]);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_CHART, ptr[NETDATA_CACHESTAT_IDX_HIT].dimension, publish->hit);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_MISSES_CHART,
        ptr[NETDATA_CACHESTAT_IDX_MISS].dimension,
        publish->miss);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_cache_send_apps_data(struct ebpf_target *root)
{
    struct ebpf_target *w;
    collected_number value;

    netdata_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_CACHESTAT_IDX))))
            continue;

        netdata_cachestat_t *current = &w->cachestat.current;
        netdata_cachestat_t *prev = &w->cachestat.prev;

        uint64_t mpa = current->mark_page_accessed - prev->mark_page_accessed;
        uint64_t mbd = current->mark_buffer_dirty - prev->mark_buffer_dirty;
        w->cachestat.dirty = (long long)mbd;
        uint64_t apcl = current->add_to_page_cache_lru - prev->add_to_page_cache_lru;
        uint64_t apd = current->account_page_dirtied - prev->account_page_dirtied;

        cachestat_update_publish(&w->cachestat, mpa, mbd, apcl, apd);

        value = (collected_number)w->cachestat.ratio;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_cachestat_hit_ratio");
        write_chart_dimension("ratio", value);
        ebpf_write_end_chart();

        value = (collected_number)w->cachestat.dirty;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_cachestat_dirty_pages");
        write_chart_dimension("pages", value);
        ebpf_write_end_chart();

        value = (collected_number)w->cachestat.hit;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_cachestat_access");
        write_chart_dimension("hits", value);
        ebpf_write_end_chart();

        value = (collected_number)w->cachestat.miss;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_cachestat_misses");
        write_chart_dimension("misses", value);
        ebpf_write_end_chart();
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_cachestat_sum_cgroup_pids(netdata_publish_cachestat_t *publish, struct pid_on_target2 *root)
{
    memcpy(&publish->prev, &publish->current, sizeof(publish->current));
    memset(&publish->current, 0, sizeof(publish->current));

    netdata_cachestat_t *dst = &publish->current;
    for (; root; root = root->next) {
        netdata_cachestat_t *src = &root->cachestat.current;

        dst->account_page_dirtied += src->account_page_dirtied;
        dst->add_to_page_cache_lru += src->add_to_page_cache_lru;
        dst->mark_buffer_dirty += src->mark_buffer_dirty;
        dst->mark_page_accessed += src->mark_page_accessed;
    }
}

/**
 * Calc chart values
 *
 * Do necessary math to plot charts.
 */
void ebpf_cachestat_calc_chart_values()
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_cachestat_sum_cgroup_pids(&ect->publish_cachestat, ect->pids);

        netdata_cachestat_t *current = &ect->publish_cachestat.current;
        netdata_cachestat_t *prev = &ect->publish_cachestat.prev;

        uint64_t mpa = current->mark_page_accessed - prev->mark_page_accessed;
        uint64_t mbd = current->mark_buffer_dirty - prev->mark_buffer_dirty;
        ect->publish_cachestat.dirty = (long long)mbd;
        uint64_t apcl = current->add_to_page_cache_lru - prev->add_to_page_cache_lru;
        uint64_t apd = current->account_page_dirtied - prev->account_page_dirtied;

        cachestat_update_publish(&ect->publish_cachestat, mpa, mbd, apcl, apd);
    }
}

/**
 *  Create Systemd cachestat Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_cachestat_charts(int update_every)
{
    static ebpf_systemd_args_t data_hit_ratio = {
        .title = "Hit ratio",
        .units = EBPF_COMMON_UNITS_PERCENTAGE,
        .family = NETDATA_CACHESTAT_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21100,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_CACHESTAT_HIT_RATIO_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_CACHESTAT,
        .update_every = 0,
        .suffix = NETDATA_CACHESTAT_HIT_RATIO_CHART,
        .dimension = "percentage"};

    static ebpf_systemd_args_t data_dirty = {
        .title = "Number of dirty pages",
        .units = EBPF_CACHESTAT_UNITS_PAGE,
        .family = NETDATA_CACHESTAT_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21101,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_CACHESTAT,
        .update_every = 0,
        .suffix = NETDATA_CACHESTAT_DIRTY_CHART,
        .dimension = "pages"};

    static ebpf_systemd_args_t data_hit = {
        .title = "Number of accessed files",
        .units = EBPF_CACHESTAT_UNITS_HITS,
        .family = NETDATA_CACHESTAT_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21102,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_CACHESTAT_HIT_FILE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_CACHESTAT,
        .update_every = 0,
        .suffix = NETDATA_CACHESTAT_HIT_CHART,
        .dimension = "hits"};

    static ebpf_systemd_args_t data_miss = {
        .title = "Files out of page cache",
        .units = EBPF_CACHESTAT_UNITS_MISSES,
        .family = NETDATA_CACHESTAT_SUBMENU,
        .charttype = NETDATA_EBPF_CHART_TYPE_LINE,
        .order = 21103,
        .algorithm = EBPF_CHART_ALGORITHM_ABSOLUTE,
        .context = NETDATA_SYSTEMD_CACHESTAT_MISS_FILES_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_CACHESTAT,
        .update_every = 0,
        .suffix = NETDATA_CACHESTAT_MISSES_CHART,
        .dimension = "misses"};

    if (!data_miss.update_every)
        data_hit_ratio.update_every = data_dirty.update_every = data_hit.update_every = data_miss.update_every =
            update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_CACHESTAT_CHART))
            continue;

        data_hit_ratio.id = data_dirty.id = data_hit.id = data_miss.id = w->name;
        ebpf_create_charts_on_systemd(&data_hit_ratio);

        ebpf_create_charts_on_systemd(&data_dirty);

        ebpf_create_charts_on_systemd(&data_hit);

        ebpf_create_charts_on_systemd(&data_miss);

        w->flags |= NETDATA_EBPF_SERVICES_HAS_CACHESTAT_CHART;
    }
}

/**
 * Send Cache Stat charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_cachestat_charts()
{
    ebpf_cgroup_target_t *ect;

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_CACHESTAT_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_CACHESTAT_HIT_RATIO_CHART, "");
        write_chart_dimension("percentage", (long long)ect->publish_cachestat.ratio);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_CACHESTAT_DIRTY_CHART, "");
        write_chart_dimension("pages", (long long)ect->publish_cachestat.dirty);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_CACHESTAT_HIT_CHART, "");
        write_chart_dimension("hits", (long long)ect->publish_cachestat.hit);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_CACHESTAT_MISSES_CHART, "");
        write_chart_dimension("misses", (long long)ect->publish_cachestat.miss);
        ebpf_write_end_chart();
    }
}

/**
 * Send Directory Cache charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_specific_cachestat_data(char *type, netdata_publish_cachestat_t *npc)
{
    ebpf_write_begin_chart(type, NETDATA_CACHESTAT_HIT_RATIO_CHART, "");
    write_chart_dimension(
        cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_RATIO].name, (long long)npc->ratio);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_CACHESTAT_DIRTY_CHART, "");
    write_chart_dimension(
        cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY].name, (long long)npc->dirty);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_CACHESTAT_HIT_CHART, "");
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT].name, (long long)npc->hit);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_CACHESTAT_MISSES_CHART, "");
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS].name, (long long)npc->miss);
    ebpf_write_end_chart();
}

/**
 * Create specific cache Stat charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_cachestat_charts(char *type, int update_every)
{
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        "Hit ratio",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5200,
        ebpf_create_global_dimension,
        cachestat_counter_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_CACHESTAT_DIRTY_CHART,
        "Number of dirty pages",
        EBPF_CACHESTAT_UNITS_PAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5201,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_CACHESTAT_HIT_CHART,
        "Number of accessed files",
        EBPF_CACHESTAT_UNITS_HITS,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5202,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_CACHESTAT_MISSES_CHART,
        "Files out of page cache",
        EBPF_CACHESTAT_UNITS_MISSES,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5203,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS],
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
}

/**
 * Obsolete specific cache stat charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_cachestat_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        "",
        "Hit ratio",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5200,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_CACHESTAT_DIRTY_CHART,
        "",
        "Number of dirty pages",
        EBPF_CACHESTAT_UNITS_PAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5201,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_CACHESTAT_HIT_CHART,
        "",
        "Number of accessed files",
        EBPF_CACHESTAT_UNITS_HITS,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5202,
        update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_CACHESTAT_MISSES_CHART,
        "",
        "Files out of page cache",
        EBPF_CACHESTAT_UNITS_MISSES,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5203,
        update_every);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_cachestat_send_cgroup_data(int update_every)
{
    netdata_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    ebpf_cachestat_calc_chart_values();

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_cachestat_charts(update_every);
        }

        ebpf_send_systemd_cachestat_charts();
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART) && ect->updated) {
            ebpf_create_specific_cachestat_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART) {
            if (ect->updated) {
                ebpf_send_specific_cachestat_data(ect->name, &ect->publish_cachestat);
            } else {
                ebpf_obsolete_specific_cachestat_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART;
            }
        }
    }

    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void cachestat_collector(ebpf_module_t *em)
{
    netdata_publish_cachestat_t publish;
    memset(&publish, 0, sizeof(publish));
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    int maps_per_core = em->maps_per_core;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int counter = update_every - 1;
    //This will be cancelled by its parent
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_cachestat_read_global_tables(stats, maps_per_core);

        netdata_mutex_lock(&lock);

        cachestat_send_global(&publish);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_cache_send_apps_data(apps_groups_root_target);

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_cachestat_send_cgroup_data(update_every);

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
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_create_memory_charts(ebpf_module_t *em)
{
    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_HIT_RATIO_CHART,
        "Hit ratio",
        EBPF_COMMON_UNITS_PERCENTAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_MEM_CACHESTAT_HIT_RATIO_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21100,
        ebpf_create_global_dimension,
        cachestat_counter_publish_aggregated,
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_DIRTY_CHART,
        "Number of dirty pages",
        EBPF_CACHESTAT_UNITS_PAGE,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_MEM_CACHESTAT_MODIFIED_CACHE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21101,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_HIT_CHART,
        "Number of accessed files",
        EBPF_CACHESTAT_UNITS_HITS,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_MEM_CACHESTAT_HIT_FILES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21102,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CACHESTAT_MISSES_CHART,
        "Files out of page cache",
        EBPF_CACHESTAT_UNITS_MISSES,
        NETDATA_CACHESTAT_SUBMENU,
        NETDATA_MEM_CACHESTAT_MISS_FILES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        21103,
        ebpf_create_global_dimension,
        &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    fflush(stdout);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_cachestat_allocate_global_vectors()
{
    cachestat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_cachestat_pid_t));
    cachestat_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(cachestat_hash_values, 0, NETDATA_CACHESTAT_END * sizeof(netdata_idx_t));
    memset(cachestat_counter_aggregated_data, 0, NETDATA_CACHESTAT_END * sizeof(netdata_syscall_stat_t));
    memset(cachestat_counter_publish_aggregated, 0, NETDATA_CACHESTAT_END * sizeof(netdata_publish_syscall_t));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Update Internal value
 *
 * Update values used during runtime.
 *
 * @return It returns 0 when one of the functions is present and -1 otherwise.
 */
static int ebpf_cachestat_set_internal_value()
{
    ebpf_addresses_t address = {.function = NULL, .hash = 0, .addr = 0};
    int i;
    for (i = 0; i < NETDATA_CACHESTAT_ACCOUNT_DIRTY_END; i++) {
        address.function = account_page[i];
        ebpf_load_addresses(&address, -1);
        if (address.addr)
            break;
    }

    if (!address.addr) {
        netdata_log_error("%s cachestat.", NETDATA_EBPF_DEFAULT_FNT_NOT_FOUND);
        return -1;
    }

    cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name = address.function;

    return 0;
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_cachestat_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(cachestat_maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        cachestat_bpf_obj = cachestat_bpf__open();
        if (!cachestat_bpf_obj)
            ret = -1;
        else
            ret = ebpf_cachestat_load_and_attach(cachestat_bpf_obj, em);
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Cachestat thread
 *
 * Thread used to make cachestat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_cachestat_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_cachestat_exit) cleanup_ptr = em;

    em->maps = cachestat_maps;

    ebpf_update_pid_table(&cachestat_maps[NETDATA_CACHESTAT_PID_STATS], em);

    if (ebpf_cachestat_set_internal_value()) {
        goto endcachestat;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_cachestat_load_bpf(em)) {
        goto endcachestat;
    }

    ebpf_cachestat_allocate_global_vectors();

    int algorithms[NETDATA_CACHESTAT_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX};

    ebpf_global_labels(
        cachestat_counter_aggregated_data,
        cachestat_counter_publish_aggregated,
        cachestat_counter_dimension_name,
        cachestat_counter_dimension_name,
        algorithms,
        NETDATA_CACHESTAT_END);

    netdata_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    ebpf_create_memory_charts(em);

    netdata_mutex_unlock(&lock);

    ebpf_read_cachestat.thread =
        nd_thread_create(ebpf_read_cachestat.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_cachestat_thread, em);

    cachestat_collector(em);

endcachestat:
    ebpf_update_disabled_plugin_stats(em);
}
