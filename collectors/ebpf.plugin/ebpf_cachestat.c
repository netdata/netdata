// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_cachestat.h"

netdata_publish_cachestat_t **cachestat_pid;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static char *cachestat_counter_dimension_name[NETDATA_CACHESTAT_END] = { "ratio", "dirty", "hit",
                                                                         "miss" };
static netdata_syscall_stat_t cachestat_counter_aggregated_data[NETDATA_CACHESTAT_END];
static netdata_publish_syscall_t cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_END];

netdata_cachestat_pid_t *cachestat_vector = NULL;

static netdata_idx_t cachestat_hash_values[NETDATA_CACHESTAT_END];
static netdata_idx_t *cachestat_values = NULL;

static int read_thread_closed = 1;

struct netdata_static_thread cachestat_threads = {"CACHESTAT KERNEL",
                                                  NULL, NULL, 1, NULL,
                                                  NULL,  NULL};

static ebpf_local_maps_t cachestat_maps[] = {{.name = "cstat_global", .internal_input = NETDATA_CACHESTAT_END,
                                              .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                              .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                             {.name = "cstat_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                              .user_input = 0,
                                              .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                              .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                             {.name = "cstat_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                              .user_input = 0,
                                              .type = NETDATA_EBPF_MAP_CONTROLLER,
                                              .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                             {.name = NULL, .internal_input = 0, .user_input = 0,
                                              .type = NETDATA_EBPF_MAP_CONTROLLER,
                                              .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

struct config cachestat_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

netdata_ebpf_targets_t cachestat_targets[] = { {.name = "add_to_page_cache_lru", .mode = EBPF_LOAD_TRAMPOLINE},
                                               {.name = "mark_page_accessed", .mode = EBPF_LOAD_TRAMPOLINE},
                                               {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE},
                                               {.name = "mark_buffer_dirty", .mode = EBPF_LOAD_TRAMPOLINE},
                                               {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
#include "includes/cachestat.skel.h" // BTF code

static struct cachestat_bpf *bpf_obj = NULL;

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
    if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_16) {
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_kprobe, false);
    } else if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_15) {
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
    if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_16) {
        bpf_program__set_autoload(obj->progs.netdata_account_page_dirtied_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_set_page_dirty_fentry, false);
    } else if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_15) {
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
    bpf_program__set_attach_target(obj->progs.netdata_add_to_page_cache_lru_fentry, 0,
                                   cachestat_targets[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].name);

    bpf_program__set_attach_target(obj->progs.netdata_mark_page_accessed_fentry, 0,
                                   cachestat_targets[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED].name);

    if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_16) {
        bpf_program__set_attach_target(obj->progs.netdata_folio_mark_dirty_fentry, 0,
                                       cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    } else if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_15) {
        bpf_program__set_attach_target(obj->progs.netdata_set_page_dirty_fentry, 0,
                                       cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    } else {
        bpf_program__set_attach_target(obj->progs.netdata_account_page_dirtied_fentry, 0,
                                       cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
    }

    bpf_program__set_attach_target(obj->progs.netdata_mark_buffer_dirty_fentry, 0,
                                   cachestat_targets[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY].name);
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
    obj->links.netdata_add_to_page_cache_lru_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_add_to_page_cache_lru_kprobe,
                                                                                 false,
                                                                                 cachestat_targets[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU].name);
    int ret = libbpf_get_error(obj->links.netdata_add_to_page_cache_lru_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_mark_page_accessed_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_mark_page_accessed_kprobe,
                                                                              false,
                                                                              cachestat_targets[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED].name);
    ret = libbpf_get_error(obj->links.netdata_mark_page_accessed_kprobe);
    if (ret)
        return -1;

    if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_16) {
        obj->links.netdata_folio_mark_dirty_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_folio_mark_dirty_kprobe,
                                                                                false,
                                                                                cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_folio_mark_dirty_kprobe);
    } else if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_15) {
        obj->links.netdata_set_page_dirty_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_set_page_dirty_kprobe,
                                                                              false,
                                                                              cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_set_page_dirty_kprobe);
    } else {
        obj->links.netdata_account_page_dirtied_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_account_page_dirtied_kprobe,
                                                                                    false,
                                                                                    cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name);
        ret = libbpf_get_error(obj->links.netdata_account_page_dirtied_kprobe);
    }

    if (ret)
        return -1;

    obj->links.netdata_mark_buffer_dirty_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_mark_buffer_dirty_kprobe,
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
static void ebpf_cachestat_adjust_map_size(struct cachestat_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(obj->maps.cstat_pid, &cachestat_maps[NETDATA_CACHESTAT_PID_STATS],
                         em, bpf_map__name(obj->maps.cstat_pid));
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
 * @return it returns 0 on succes and -1 otherwise
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

    int ret = cachestat_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ebpf_cachestat_adjust_map_size(obj, em);

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

/**
 * Clean PID structures
 *
 * Clean the allocated structures.
 */
void clean_cachestat_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(cachestat_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_cachestat_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 2*USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    ebpf_cleanup_publish_syscall(cachestat_counter_publish_aggregated);

    freez(cachestat_vector);
    freez(cachestat_values);

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
    }
#ifdef LIBBPF_MAJOR_VERSION
    else if (bpf_obj)
        cachestat_bpf__destroy(bpf_obj);
#endif
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
void cachestat_update_publish(netdata_publish_cachestat_t *out, uint64_t mpa, uint64_t mbd,
                              uint64_t apcl, uint64_t apd)
{
    // Adapted algorithm from https://github.com/iovisor/bcc/blob/master/tools/cachestat.py#L126-L138
    calculated_number total = (calculated_number) (((long long)mpa) - ((long long)mbd));
    if (total < 0)
        total = 0;

    calculated_number misses = (calculated_number) ( ((long long) apcl) - ((long long) apd) );
    if (misses < 0)
        misses = 0;

    // If hits are < 0, then its possible misses are overestimate due to possibly page cache read ahead adding
    // more pages than needed. In this case just assume misses as total and reset hits.
    calculated_number hits = total - misses;
    if (hits < 0 ) {
        misses = total;
        hits = 0;
    }

    calculated_number ratio = (total > 0) ? hits/total : 1;

    out->ratio = (long long )(ratio*100);
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
static void save_previous_values(netdata_publish_cachestat_t *publish) {
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
static void calculate_stats(netdata_publish_cachestat_t *publish) {
    if (!publish->prev.mark_page_accessed) {
        save_previous_values(publish);
        return;
    }

    uint64_t mpa = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED] - publish->prev.mark_page_accessed;
    uint64_t mbd = cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY] - publish->prev.mark_buffer_dirty;
    uint64_t apcl = cachestat_hash_values[NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU] - publish->prev.add_to_page_cache_lru;
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
 */
static void cachestat_apps_accumulator(netdata_cachestat_pid_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    netdata_cachestat_pid_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_cachestat_pid_t *w = &out[i];
        total->account_page_dirtied += w->account_page_dirtied;
        total->add_to_page_cache_lru += w->add_to_page_cache_lru;
        total->mark_buffer_dirty += w->mark_buffer_dirty;
        total->mark_page_accessed += w->mark_page_accessed;
    }
}

/**
 * Save Pid values
 *
 * Save the current values inside the structure
 *
 * @param out     vector used to plot charts
 * @param publish vector with values read from hash tables.
 */
static inline void cachestat_save_pid_values(netdata_publish_cachestat_t *out, netdata_cachestat_pid_t *publish)
{
    if (!out->current.mark_page_accessed) {
        memcpy(&out->current, &publish[0], sizeof(netdata_cachestat_pid_t));
        return;
    }

    memcpy(&out->prev, &out->current, sizeof(netdata_cachestat_pid_t));
    memcpy(&out->current, &publish[0], sizeof(netdata_cachestat_pid_t));
}

/**
 * Fill PID
 *
 * Fill PID structures
 *
 * @param current_pid pid that we are collecting data
 * @param out         values read from hash tables;
 */
static void cachestat_fill_pid(uint32_t current_pid, netdata_cachestat_pid_t *publish)
{
    netdata_publish_cachestat_t *curr = cachestat_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_publish_cachestat_t));
        cachestat_pid[current_pid] = curr;

        cachestat_save_pid_values(curr, publish);
        return;
    }

    cachestat_save_pid_values(curr, publish);
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 */
static void read_apps_table()
{
    netdata_cachestat_pid_t *cv = cachestat_vector;
    uint32_t key;
    struct pid_stat *pids = root_of_pids;
    int fd = cachestat_maps[NETDATA_CACHESTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_cachestat_pid_t)*ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, cv)) {
            pids = pids->next;
            continue;
        }

        cachestat_apps_accumulator(cv);

        cachestat_fill_pid(key, cv);

        // We are cleaning to avoid passing data read from one process to other.
        memset(cv, 0, length);

        pids = pids->next;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 */
static void ebpf_update_cachestat_cgroup()
{
    netdata_cachestat_pid_t *cv = cachestat_vector;
    int fd = cachestat_maps[NETDATA_CACHESTAT_PID_STATS].map_fd;
    size_t length = sizeof(netdata_cachestat_pid_t) * ebpf_nprocs;

    ebpf_cgroup_target_t *ect;
    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_cachestat_pid_t *out = &pids->cachestat;
            if (likely(cachestat_pid) && cachestat_pid[pid]) {
                netdata_publish_cachestat_t *in = cachestat_pid[pid];

                memcpy(out, &in->current, sizeof(netdata_cachestat_pid_t));
            } else {
                memset(cv, 0, length);
                if (bpf_map_lookup_elem(fd, &pid, cv)) {
                    continue;
                }

                cachestat_apps_accumulator(cv);

                memcpy(out, cv, sizeof(netdata_cachestat_pid_t));
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
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
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_CACHESTAT_HIT_RATIO_CHART,
                               "Hit ratio",
                               EBPF_COMMON_DIMENSION_PERCENTAGE,
                               NETDATA_CACHESTAT_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_LINE,
                               20090,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_charts_on_apps(NETDATA_CACHESTAT_DIRTY_CHART,
                               "Number of dirty pages",
                               EBPF_CACHESTAT_DIMENSION_PAGE,
                               NETDATA_CACHESTAT_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20091,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_charts_on_apps(NETDATA_CACHESTAT_HIT_CHART,
                               "Number of accessed files",
                               EBPF_CACHESTAT_DIMENSION_HITS,
                               NETDATA_CACHESTAT_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20092,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_charts_on_apps(NETDATA_CACHESTAT_MISSES_CHART,
                               "Files out of page cache",
                               EBPF_CACHESTAT_DIMENSION_MISSES,
                               NETDATA_CACHESTAT_SUBMENU,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20093,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);
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
 */
static void read_global_table()
{
    uint32_t idx;
    netdata_idx_t *val = cachestat_hash_values;
    netdata_idx_t *stored = cachestat_values;
    int fd = cachestat_maps[NETDATA_CACHESTAT_GLOBAL_STATS].map_fd;

    for (idx = NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU; idx < NETDATA_CACHESTAT_END; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, stored)) {
            int i;
            int end = ebpf_nprocs;
            netdata_idx_t total = 0;
            for (i = 0; i < end; i++)
                total += stored[i];

            val[idx] = total;
        }
    }
}

/**
 * Socket read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data on very busy socket.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_cachestat_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_LATENCY_CACHESTAT_SLEEP_MS * em->update_every;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }
    read_thread_closed = 1;

    return NULL;
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
        NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_RATIO_CHART, ptr[NETDATA_CACHESTAT_IDX_RATIO].dimension,
        publish->ratio);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_DIRTY_CHART, ptr[NETDATA_CACHESTAT_IDX_DIRTY].dimension,
        cachestat_hash_values[NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY]);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_CHART, ptr[NETDATA_CACHESTAT_IDX_HIT].dimension, publish->hit);

    ebpf_one_dimension_write_charts(
        NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_MISSES_CHART, ptr[NETDATA_CACHESTAT_IDX_MISS].dimension,
        publish->miss);
}

/**
 * Cachestat sum PIDs
 *
 * Sum values for all PIDs associated to a group
 *
 * @param publish  output structure.
 * @param root     structure with listed IPs
 */
void ebpf_cachestat_sum_pids(netdata_publish_cachestat_t *publish, struct pid_on_target *root)
{
    memcpy(&publish->prev, &publish->current,sizeof(publish->current));
    memset(&publish->current, 0, sizeof(publish->current));

    netdata_cachestat_pid_t *dst = &publish->current;
    while (root) {
        int32_t pid = root->pid;
        netdata_publish_cachestat_t *w = cachestat_pid[pid];
        if (w) {
            netdata_cachestat_pid_t *src = &w->current;
            dst->account_page_dirtied += src->account_page_dirtied;
            dst->add_to_page_cache_lru += src->add_to_page_cache_lru;
            dst->mark_buffer_dirty += src->mark_buffer_dirty;
            dst->mark_page_accessed += src->mark_page_accessed;
        }

        root = root->next;
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_cache_send_apps_data(struct target *root)
{
    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_CACHESTAT_HIT_RATIO_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_cachestat_sum_pids(&w->cachestat, w->root_pid);
            netdata_cachestat_pid_t *current = &w->cachestat.current;
            netdata_cachestat_pid_t *prev = &w->cachestat.prev;

            uint64_t mpa = current->mark_page_accessed - prev->mark_page_accessed;
            uint64_t mbd = current->mark_buffer_dirty - prev->mark_buffer_dirty;
            w->cachestat.dirty = mbd;
            uint64_t apcl = current->add_to_page_cache_lru - prev->add_to_page_cache_lru;
            uint64_t apd = current->account_page_dirtied - prev->account_page_dirtied;

            cachestat_update_publish(&w->cachestat, mpa, mbd, apcl, apd);
            value = (collected_number) w->cachestat.ratio;
            // Here we are using different approach to have a chart more smooth
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_CACHESTAT_DIRTY_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = (collected_number) w->cachestat.dirty;
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_CACHESTAT_HIT_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = (collected_number) w->cachestat.hit;
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_CACHESTAT_MISSES_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = (collected_number) w->cachestat.miss;
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();
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
    memcpy(&publish->prev, &publish->current,sizeof(publish->current));
    memset(&publish->current, 0, sizeof(publish->current));

    netdata_cachestat_pid_t *dst = &publish->current;
    while (root) {
        netdata_cachestat_pid_t *src = &root->cachestat;

        dst->account_page_dirtied += src->account_page_dirtied;
        dst->add_to_page_cache_lru += src->add_to_page_cache_lru;
        dst->mark_buffer_dirty += src->mark_buffer_dirty;
        dst->mark_page_accessed += src->mark_page_accessed;

        root = root->next;
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
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_cachestat_sum_cgroup_pids(&ect->publish_cachestat, ect->pids);

        netdata_cachestat_pid_t *current = &ect->publish_cachestat.current;
        netdata_cachestat_pid_t *prev = &ect->publish_cachestat.prev;

        uint64_t mpa = current->mark_page_accessed - prev->mark_page_accessed;
        uint64_t mbd = current->mark_buffer_dirty - prev->mark_buffer_dirty;
        ect->publish_cachestat.dirty = mbd;
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
    ebpf_create_charts_on_systemd(NETDATA_CACHESTAT_HIT_RATIO_CHART,
                                  "Hit ratio",
                                  EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_CACHESTAT_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE, 21100,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_CACHESTAT_HIT_RATIO_CONTEXT, NETDATA_EBPF_MODULE_NAME_CACHESTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_CACHESTAT_DIRTY_CHART,
                                  "Number of dirty pages",
                                  EBPF_CACHESTAT_DIMENSION_PAGE, NETDATA_CACHESTAT_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE, 21101,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_CACHESTAT_MODIFIED_CACHE_CONTEXT, NETDATA_EBPF_MODULE_NAME_CACHESTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_CACHESTAT_HIT_CHART, "Number of accessed files",
                                  EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE, 21102,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_CACHESTAT_HIT_FILE_CONTEXT, NETDATA_EBPF_MODULE_NAME_CACHESTAT,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_CACHESTAT_MISSES_CHART, "Files out of page cache",
                                  EBPF_CACHESTAT_DIMENSION_MISSES, NETDATA_CACHESTAT_SUBMENU,
                                  NETDATA_EBPF_CHART_TYPE_LINE, 21103,
                                  ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                                  NETDATA_SYSTEMD_CACHESTAT_MISS_FILES_CONTEXT, NETDATA_EBPF_MODULE_NAME_CACHESTAT,
                                  update_every);
}

/**
 * Send Cache Stat charts
 *
 * Send collected data to Netdata.
 *
 * @return It returns the status for chart creation, if it is necessary to remove a specific dimension, zero is returned
 *         otherwise function returns 1 to avoid chart recreation
 */
static int ebpf_send_systemd_cachestat_charts()
{
    int ret = 1;
    ebpf_cgroup_target_t *ect;

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_CACHESTAT_HIT_RATIO_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_cachestat.ratio);
        } else
            ret = 0;
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_CACHESTAT_DIRTY_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_cachestat.dirty);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_CACHESTAT_HIT_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_cachestat.hit);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_CACHESTAT_MISSES_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_cachestat.miss);
        }
    }
    write_end_chart();

    return ret;
}

/**
 * Send Directory Cache charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_specific_cachestat_data(char *type, netdata_publish_cachestat_t *npc)
{
    write_begin_chart(type, NETDATA_CACHESTAT_HIT_RATIO_CHART);
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_RATIO].name, (long long)npc->ratio);
    write_end_chart();

    write_begin_chart(type, NETDATA_CACHESTAT_DIRTY_CHART);
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY].name, (long long)npc->dirty);
    write_end_chart();

    write_begin_chart(type, NETDATA_CACHESTAT_HIT_CHART);
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT].name, (long long)npc->hit);
    write_end_chart();

    write_begin_chart(type, NETDATA_CACHESTAT_MISSES_CHART);
    write_chart_dimension(cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS].name, (long long)npc->miss);
    write_end_chart();
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
    ebpf_create_chart(type, NETDATA_CACHESTAT_HIT_RATIO_CHART,
                      "Hit ratio",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_CACHESTAT_CGROUP_SUBMENU,
                      NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5200,
                      ebpf_create_global_dimension,
                      cachestat_counter_publish_aggregated, 1, update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(type, NETDATA_CACHESTAT_DIRTY_CHART,
                      "Number of dirty pages",
                      EBPF_CACHESTAT_DIMENSION_PAGE, NETDATA_CACHESTAT_CGROUP_SUBMENU,
                      NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5201,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(type, NETDATA_CACHESTAT_HIT_CHART,
                      "Number of accessed files",
                      EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_CGROUP_SUBMENU,
                      NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5202,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(type, NETDATA_CACHESTAT_MISSES_CHART,
                      "Files out of page cache",
                      EBPF_CACHESTAT_DIMENSION_MISSES, NETDATA_CACHESTAT_CGROUP_SUBMENU,
                      NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5203,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);
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
    ebpf_write_chart_obsolete(type, NETDATA_CACHESTAT_HIT_RATIO_CHART,
                      "Hit ratio",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_CACHESTAT_SUBMENU,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5200, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_CACHESTAT_DIRTY_CHART,
                      "Number of dirty pages",
                      EBPF_CACHESTAT_DIMENSION_PAGE, NETDATA_CACHESTAT_SUBMENU,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5201, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_CACHESTAT_HIT_CHART,
                      "Number of accessed files",
                      EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_SUBMENU,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5202, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_CACHESTAT_MISSES_CHART,
                      "Files out of page cache",
                      EBPF_CACHESTAT_DIMENSION_MISSES, NETDATA_CACHESTAT_SUBMENU,
                      NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5203, update_every);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_cachestat_send_cgroup_data(int update_every)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    ebpf_cachestat_calc_chart_values();

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;
    if (has_systemd) {
        static int systemd_charts = 0;
        if (!systemd_charts) {
            ebpf_create_systemd_cachestat_charts(update_every);
            systemd_charts = 1;
        }

        systemd_charts = ebpf_send_systemd_cachestat_charts();
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
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

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void cachestat_collector(ebpf_module_t *em)
{
    cachestat_threads.thread = mallocz(sizeof(netdata_thread_t));
    cachestat_threads.start_routine = ebpf_cachestat_read_hash;

    netdata_thread_create(cachestat_threads.thread, cachestat_threads.name, NETDATA_THREAD_OPTION_JOINABLE,
                          ebpf_cachestat_read_hash, em);

    netdata_publish_cachestat_t publish;
    memset(&publish, 0, sizeof(publish));
    int apps = em->apps_charts;
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;
            if (apps)
                read_apps_table();

            if (cgroups)
                ebpf_update_cachestat_cgroup();

            pthread_mutex_lock(&lock);

            cachestat_send_global(&publish);

            if (apps)
                ebpf_cache_send_apps_data(apps_groups_root_target);

            if (cgroups)
                ebpf_cachestat_send_cgroup_data(update_every);

            pthread_mutex_unlock(&lock);
        }

        pthread_mutex_unlock(&collect_data_mutex);
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
    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_RATIO_CHART,
                      "Hit ratio",
                      EBPF_COMMON_DIMENSION_PERCENTAGE, NETDATA_CACHESTAT_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21100,
                      ebpf_create_global_dimension,
                      cachestat_counter_publish_aggregated, 1, em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_DIRTY_CHART,
                      "Number of dirty pages",
                      EBPF_CACHESTAT_DIMENSION_PAGE, NETDATA_CACHESTAT_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21101,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_DIRTY], 1,
                      em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_HIT_CHART,
                      "Number of accessed files",
                      EBPF_CACHESTAT_DIMENSION_HITS, NETDATA_CACHESTAT_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21102,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_HIT], 1,
                      em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

    ebpf_create_chart(NETDATA_EBPF_MEMORY_GROUP, NETDATA_CACHESTAT_MISSES_CHART,
                      "Files out of page cache",
                      EBPF_CACHESTAT_DIMENSION_MISSES, NETDATA_CACHESTAT_SUBMENU,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      21103,
                      ebpf_create_global_dimension,
                      &cachestat_counter_publish_aggregated[NETDATA_CACHESTAT_IDX_MISS], 1,
                      em->update_every, NETDATA_EBPF_MODULE_NAME_CACHESTAT);

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
static void ebpf_cachestat_allocate_global_vectors(int apps)
{
    if (apps)
        cachestat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_cachestat_t *));

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
 */
static void ebpf_cachestat_set_internal_value()
{
    static char *account_page[] = { "account_page_dirtied", "__set_page_dirty", "__folio_mark_dirty"  };
    if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_16)
        cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name = account_page[NETDATA_CACHESTAT_FOLIO_DIRTY];
    else if (running_on_kernel >= NETDATA_EBPF_KERNEL_5_15)
        cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name = account_page[NETDATA_CACHESTAT_SET_PAGE_DIRTY];
    else
        cachestat_targets[NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED].name = account_page[NETDATA_CACHESTAT_ACCOUNT_PAGE_DIRTY];
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
    int ret = 0;
    if (em->load == EBPF_LOAD_LEGACY) {
        probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &objects);
        if (!probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        bpf_obj = cachestat_bpf__open();
        if (!bpf_obj)
            ret = -1;
        else
            ret = ebpf_cachestat_load_and_attach(bpf_obj, em);
    }
#endif

    if (ret)
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);

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
void *ebpf_cachestat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_cachestat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = cachestat_maps;

    ebpf_update_pid_table(&cachestat_maps[NETDATA_CACHESTAT_PID_STATS], em);

    if (!em->enabled)
        goto endcachestat;

    ebpf_cachestat_set_internal_value();

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_cachestat_load_bpf(em)) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endcachestat;
    }

    ebpf_cachestat_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_CACHESTAT_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(cachestat_counter_aggregated_data, cachestat_counter_publish_aggregated,
                       cachestat_counter_dimension_name, cachestat_counter_dimension_name,
                       algorithms, NETDATA_CACHESTAT_END);

    pthread_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_create_memory_charts(em);
    pthread_mutex_unlock(&lock);

    cachestat_collector(em);

endcachestat:
    if (!em->enabled)
        ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
