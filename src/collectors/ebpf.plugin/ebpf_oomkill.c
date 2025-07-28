// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_oomkill.h"

struct config oomkill_config = APPCONFIG_INITIALIZER;

#define OOMKILL_MAP_KILLCNT 0
static ebpf_local_maps_t oomkill_maps[] = {
    {.name = "tbl_oomkill",
     .internal_input = NETDATA_OOMKILL_MAX_ENTRIES,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    /* end */
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

static ebpf_tracepoint_t oomkill_tracepoints[] = {
    {.enabled = false, .class = "oom", .event = "mark_victim"},
    /* end */
    {.enabled = false, .class = NULL, .event = NULL}};

static netdata_publish_syscall_t oomkill_publish_aggregated =
    {.name = "kills", .dimension = "kills", .algorithm = "absolute", .next = NULL};

static void ebpf_obsolete_specific_oomkill_charts(char *type, int update_every);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_oomkill_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_OOMKILL_CHART,
        "",
        "Systemd service OOM kills.",
        EBPF_OOMKILL_UNIT_KILLS,
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CGROUP_OOMKILLS_CONTEXT,
        20191,
        em->update_every);
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_oomkill_cgroup_charts(ebpf_module_t *em)
{
    netdata_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_oomkill_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_oomkill_charts(ect->name, em->update_every);
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
static void ebpf_obsolete_oomkill_apps(ebpf_module_t *em)
{
    struct ebpf_target *w;
    int update_every = em->update_every;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_OOMKILL_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            NETDATA_OOMKILL_CHART,
            "Processes OOM kills.",
            EBPF_OOMKILL_UNIT_KILLS,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "ebpf.app_oomkill",
            20072,
            update_every);

        w->charts_created &= ~(1 << EBPF_MODULE_OOMKILL_IDX);
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void oomkill_cleanup(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    netdata_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_OOMKILL_IDX);
    netdata_mutex_unlock(&lock);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);

        if (em->cgroup_charts) {
            ebpf_obsolete_oomkill_cgroup_charts(em);
        }

        ebpf_obsolete_oomkill_apps(em);

        fflush(stdout);
        netdata_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

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

static void oomkill_write_data(int32_t *keys, uint32_t total)
{
    // for each app, see if it was OOM killed. record as 1 if so otherwise 0.
    struct ebpf_target *w;
    uint32_t used_pid = 0;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w != NULL; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_OOMKILL_IDX))))
            continue;

        bool was_oomkilled = false;
        if (total) {
            struct ebpf_pid_on_target *pids = w->root_pid;
            while (pids) {
                uint32_t j;
                for (j = 0; j < total; j++) {
                    if (pids->pid == keys[j]) {
                        used_pid++;
                        was_oomkilled = true;
                        // set to 0 so we consider it "done".
                        keys[j] = 0;
                        goto write_dim;
                    }
                }
                pids = pids->next;
            }
        }
    write_dim:
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, NETDATA_OOMKILL_CHART);
        write_chart_dimension(oomkill_publish_aggregated.dimension, was_oomkilled);
        ebpf_write_end_chart();
    }

    if (total != used_pid) {
        // for any remaining keys for which we couldn't find a group, this could be
        // for various reasons, but the primary one is that the PID has not yet
        // been picked up by the process thread when parsing the proc filesystem.
        // since it's been OOM killed, it will never be parsed in the future, so
        // we have no choice but to dump it into `other`.
        uint32_t rem_count = total - used_pid;
        ebpf_write_begin_chart(NETDATA_APP_FAMILY, "other", NETDATA_OOMKILL_CHART);
        write_chart_dimension(oomkill_publish_aggregated.dimension, rem_count);
        ebpf_write_end_chart();
    }

    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Create specific OOMkill charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_oomkill_charts(char *type, int update_every)
{
    ebpf_create_chart(
        type,
        NETDATA_OOMKILL_CHART,
        "Cgroup OOM kills.",
        EBPF_OOMKILL_UNIT_KILLS,
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_CGROUP_OOMKILLS_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5600,
        ebpf_create_global_dimension,
        &oomkill_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_OOMKILL);
}

/**
 *  Create Systemd OOMkill Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_oomkill_charts(int update_every)
{
    static ebpf_systemd_args_t data_oom = {
        .title = "Systemd service OOM kills.",
        .units = EBPF_OOMKILL_UNIT_KILLS,
        .family = NETDATA_EBPF_MEMORY_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20191,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_OOMKILLS_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_OOMKILL,
        .update_every = 0,
        .suffix = NETDATA_OOMKILL_CHART,
        .dimension = "kills"};

    if (!data_oom.update_every)
        data_oom.update_every = update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_OOMKILL_CHART))
            continue;

        data_oom.id = w->name;
        ebpf_create_charts_on_systemd(&data_oom);

        w->flags |= NETDATA_EBPF_SERVICES_HAS_OOMKILL_CHART;
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_oomkill_charts()
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_OOMKILL_CHART))) {
            continue;
        }
        ebpf_write_begin_chart(ect->name, NETDATA_OOMKILL_CHART, "");
        write_chart_dimension(oomkill_publish_aggregated.dimension, (long long)ect->oomkill);
        ect->oomkill = 0;
        ebpf_write_end_chart();
    }
}

/*
 * Send Specific OOMkill data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param value  value for oomkill
 */
static void ebpf_send_specific_oomkill_data(char *type, int value)
{
    ebpf_write_begin_chart(type, NETDATA_OOMKILL_CHART, "");
    write_chart_dimension(oomkill_publish_aggregated.dimension, (long long)value);
    ebpf_write_end_chart();
}

/**
 * Create specific OOMkill charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_oomkill_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_OOMKILL_CHART,
        "",
        "Cgroup OOM kills.",
        EBPF_OOMKILL_UNIT_KILLS,
        NETDATA_EBPF_MEMORY_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_OOMKILLS_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5600,
        update_every);
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_oomkill_send_cgroup_data(int update_every)
{
    netdata_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_oomkill_charts(update_every);
        }
        ebpf_send_systemd_oomkill_charts();
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART) && ect->updated) {
            ebpf_create_specific_oomkill_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART) {
            if (ect->updated) {
                ebpf_send_specific_oomkill_data(ect->name, ect->oomkill);
            } else {
                ebpf_obsolete_specific_oomkill_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART;
            }
        }
    }

    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Read data
 *
 * Read OOMKILL events from table.
 *
 * @param keys vector where data will be stored
 *
 * @return It returns the number of read elements
 */
static uint32_t oomkill_read_data(int32_t *keys)
{
    // the first `i` entries of `keys` will contain the currently active PIDs
    // in the eBPF map.
    uint32_t i = 0;

    uint32_t curr_key = 0;
    uint32_t key = 0;
    int mapfd = oomkill_maps[OOMKILL_MAP_KILLCNT].map_fd;
    uint32_t limit = NETDATA_OOMKILL_MAX_ENTRIES - 1;
    while (bpf_map_get_next_key(mapfd, &curr_key, &key) == 0) {
        curr_key = key;

        keys[i] = (int32_t)key;
        i += 1;

        // delete this key now that we've recorded its existence. there's no
        // race here, as the same PID will only get OOM killed once.
        int test = bpf_map_delete_elem(mapfd, &key);
        if (unlikely(test < 0)) {
            // since there's only 1 thread doing these deletions, it should be
            // impossible to get this condition.
            netdata_log_error("key unexpectedly not available for deletion.");
        }
        if (i > limit)
            break;
    }

    return i;
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 *
 * @param keys  vector with pids that had oomkill event
 * @param total number of elements in keys vector.
 */
static void ebpf_update_oomkill_cgroup(int32_t *keys, uint32_t total)
{
    ebpf_cgroup_target_t *ect;
    netdata_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ect->oomkill = 0;
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t j;
            int32_t pid = pids->pid;
            for (j = 0; j < total; j++) {
                if (pid == keys[j]) {
                    ect->oomkill = 1;
                    break;
                }
            }
        }
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Update OOMkill period
 *
 * Update oomkill period according function arguments.
 *
 * @param running_time  current value of running_value.
 * @param em            the thread main structure.
 *
 * @return It returns new running_time value.
 */
static int ebpf_update_oomkill_period(int running_time, ebpf_module_t *em)
{
    netdata_mutex_lock(&ebpf_exit_cleanup);
    if (running_time && !em->running_time)
        running_time = em->update_every;
    else
        running_time += em->update_every;

    em->running_time = running_time;
    netdata_mutex_unlock(&ebpf_exit_cleanup);

    return running_time;
}

/**
* Main loop for this collector.
 *
 * @param em the thread main structure.
*/
static void oomkill_collector(ebpf_module_t *em)
{
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    int32_t keys[NETDATA_OOMKILL_MAX_ENTRIES];
    memset(keys, 0, sizeof(keys));

    // loop and read until ebpf plugin is closed.
    int counter = update_every - 1;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        (void)heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;

        uint32_t count = oomkill_read_data(keys);

        stats[NETDATA_CONTROLLER_PID_TABLE_ADD] += (uint64_t)count;
        stats[NETDATA_CONTROLLER_PID_TABLE_DEL] += (uint64_t)count;

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_update_oomkill_cgroup(keys, count);

        netdata_apps_integration_flags_t apps = em->apps_charts;
        netdata_mutex_lock(&lock);
        // write everything from the ebpf map.
        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_oomkill_send_cgroup_data(update_every);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            oomkill_write_data(keys, count);

        netdata_mutex_unlock(&lock);

        running_time = ebpf_update_oomkill_period(running_time, em);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_oomkill_create_apps_charts(struct ebpf_module *em, void *ptr)
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
            NETDATA_OOMKILL_CHART,
            "Processes OOM kills.",
            EBPF_OOMKILL_UNIT_KILLS,
            NETDATA_EBPF_MEMORY_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_oomkill",
            20072,
            update_every,
            NETDATA_EBPF_MODULE_NAME_OOMKILL);
        ebpf_create_chart_labels("app_group", w->clean_name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(
            stdout,
            "DIMENSION '%s' '' %s 1 1\n",
            oomkill_publish_aggregated.dimension,
            ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

        w->charts_created |= 1 << EBPF_MODULE_OOMKILL_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/**
 * OOM kill tracking thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void ebpf_oomkill_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(oomkill_cleanup) cleanup_ptr = em;

    em->maps = oomkill_maps;

#define NETDATA_DEFAULT_OOM_DISABLED_MSG "Disabling OOMKILL thread, because"
    if (unlikely(!em->apps_charts)) {
        // When we are not running integration with apps, we won't fill necessary variables for this thread to run, so
        // we need to disable it.
        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (em->enabled)
            netdata_log_info("%s apps integration is completely disabled.", NETDATA_DEFAULT_OOM_DISABLED_MSG);
        netdata_mutex_unlock(&ebpf_exit_cleanup);

        goto endoomkill;
    } else if (running_on_kernel < NETDATA_EBPF_KERNEL_4_14) {
        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (em->enabled)
            netdata_log_info("%s kernel does not have necessary tracepoints.", NETDATA_DEFAULT_OOM_DISABLED_MSG);
        netdata_mutex_unlock(&ebpf_exit_cleanup);

        goto endoomkill;
    }

    if (ebpf_enable_tracepoints(oomkill_tracepoints) == 0) {
        goto endoomkill;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif
    em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
    if (!em->probe_links) {
        goto endoomkill;
    }

    netdata_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    netdata_mutex_unlock(&lock);

    oomkill_collector(em);

endoomkill:
    ebpf_update_disabled_plugin_stats(em);
}
