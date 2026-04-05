// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_cgroup.h"
#include "libbpf_api/ebpf_library.h"

ebpf_cgroup_target_t *ebpf_cgroup_pids = NULL;
int send_cgroup_chart = 0;

#ifdef OS_LINUX
static nipc_cgroups_cache_t ebpf_cgroup_cache;
static bool ebpf_cgroup_cache_initialized = false;
#endif

// --------------------------------------------------------------------------------------------------------------------
// Close and Cleanup

/**
 * Clean Specific cgroup pid
 *
 * Clean all PIDs associated with cgroup.
 *
 * @param pt structure pid on target that will have your PRs removed
 */
static inline void ebpf_clean_specific_cgroup_pids(struct pid_on_target2 *pt)
{
    while (pt) {
        struct pid_on_target2 *next_pid = pt->next;

        freez(pt);
        pt = next_pid;
    }
}

/**
 * Remove Cgroup Update Target Update List
 *
 * Remove from cgroup target and update the link list
 */
static void ebpf_remove_cgroup_target_update_list()
{
    ebpf_cgroup_target_t *next, *ect = ebpf_cgroup_pids;
    ebpf_cgroup_target_t *prev = ebpf_cgroup_pids;
    while (ect) {
        next = ect->next;
        if (!ect->updated) {
            if (ect == ebpf_cgroup_pids) {
                ebpf_cgroup_pids = next;
                prev = next;
            } else {
                prev->next = next;
            }

            ebpf_clean_specific_cgroup_pids(ect->pids);
            freez(ect);
        } else {
            prev = ect;
        }

        ect = next;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Fill variables

/**
 * Set Target Data
 *
 * Set local variable values from a netipc cache item.
 *
 * @param out  local output variable.
 * @param item netipc cache item.
 */
static inline void ebpf_cgroup_set_target_data(ebpf_cgroup_target_t *out, const nipc_cgroups_cache_item_t *item)
{
    out->hash = item->hash;
    snprintfz(out->name, 255, "%s", item->name);
    out->systemd = item->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;
    out->updated = 1;
}

/**
 * Find or create
 *
 * Find the structure inside the link list or allocate and link when it is not present.
 *
 * @param item netipc cache item.
 *
 * @return It returns a pointer for the structure associated with the input.
 */
static ebpf_cgroup_target_t *ebpf_cgroup_find_or_create(const nipc_cgroups_cache_item_t *item)
{
    for (ebpf_cgroup_target_t *ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->hash == item->hash && !strcmp(ect->name, item->name)) {
            ect->updated = 1;
            return ect;
        }
    }

    ebpf_cgroup_target_t *new_ect = callocz(1, sizeof(*new_ect));
    ebpf_cgroup_set_target_data(new_ect, item);
    new_ect->next = ebpf_cgroup_pids;
    ebpf_cgroup_pids = new_ect;

    return new_ect;
}

/**
 * Update pid link list
 *
 * Update PIDs list associated with specific cgroup.
 *
 * @param ect  cgroup structure where pids will be stored
 * @param path file with PIDs associated to cgroup.
 */
static void ebpf_update_pid_link_list(ebpf_cgroup_target_t *ect, const char *path)
{
    procfile *ff = procfile_open_no_log(path, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return;

    ff = procfile_readall(ff);
    if (!ff)
        return;

    for (size_t l = 0; l < procfile_lines(ff); l++) {
        int pid = (int)str2l(procfile_lineword(ff, l, 0));
        if (!pid)
            continue;

        int found = 0;
        for (struct pid_on_target2 *pt = ect->pids; pt; pt = pt->next) {
            if (pt->pid == pid) {
                pt->updated = 1;
                found = 1;
                break;
            }
        }

        if (!found) {
            struct pid_on_target2 *w = callocz(1, sizeof(*w));
            w->pid = pid;
            w->updated = 1;
            w->next = ect->pids;
            ect->pids = w;
        }
    }

    struct pid_on_target2 **pt = &ect->pids;
    while (*pt) {
        if (!(*pt)->updated) {
            struct pid_on_target2 *tmp = *pt;
            *pt = tmp->next;
            freez(tmp);
        } else {
            (*pt)->updated = 0;
            pt = &(*pt)->next;
        }
    }

    procfile_close(ff);
}

/**
 * Set remove var
 *
 * Set variable remove. If this variable is not reset, the structure will be removed from link list.
 */
void ebpf_reset_updated_var()
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ect->updated = 0;
    }
}

/**
 * Initialize netipc cgroup cache
 *
 * Connect to the cgroups-snapshot service via netipc.
 */
static void ebpf_cgroup_cache_init(void)
{
#ifdef OS_LINUX
    if (ebpf_cgroup_cache_initialized)
        return;

    uint64_t auth = netipc_auth_token();

    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = auth,
    };

    nipc_cgroups_cache_init(&ebpf_cgroup_cache,
                             os_run_dir(true),
                             "cgroups-snapshot",
                             &config);

    ebpf_cgroup_cache_initialized = true;
#endif
}

/**
 * Close the netipc cgroup cache and release resources.
 */
void ebpf_cgroup_cache_cleanup(void)
{
#ifdef OS_LINUX
    if (ebpf_cgroup_cache_initialized) {
        nipc_cgroups_cache_close(&ebpf_cgroup_cache);
        ebpf_cgroup_cache_initialized = false;
    }
#endif
}

/**
 * Refresh cgroup data from netipc cache
 *
 * Replaces the legacy SHM parse function.
 */
static void ebpf_parse_cgroup_netipc_data(void)
{
#ifdef OS_LINUX
    static uint32_t previous_count = 0;

    if (!ebpf_cgroup_cache_initialized)
        return;

    static int refresh_fail_count = 0;
    if (!nipc_cgroups_cache_refresh(&ebpf_cgroup_cache)) {
        if (++refresh_fail_count % 10 == 1)
            collector_error("EBPF CGROUP: netipc refresh failed (%d consecutive failures)", refresh_fail_count);
        return;
    }
    refresh_fail_count = 0;

    uint32_t count = ebpf_cgroup_cache.item_count;
    if (count == 0)
        return;

    // update global flags for other ebpf modules
    ebpf_cgroup_systemd_enabled = (int)ebpf_cgroup_cache.systemd_enabled;
    ebpf_cgroup_integration_active = 1;

    netdata_mutex_lock(&mutex_cgroup_shm);
    ebpf_remove_cgroup_target_update_list();
    ebpf_reset_updated_var();

    for (uint32_t i = 0; i < count; i++) {
        const nipc_cgroups_cache_item_t *item = &ebpf_cgroup_cache.items[i];
        if (item->enabled) {
            ebpf_cgroup_target_t *ect = ebpf_cgroup_find_or_create(item);
            ebpf_update_pid_link_list(ect, item->path);
        }
    }

    send_cgroup_chart = previous_count != count;
    previous_count = count;
    netdata_mutex_unlock(&mutex_cgroup_shm);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// Create charts

/**
 * Create charts on systemd submenu
 *
 * @param id   the chart id
 * @param title  the value displayed on vertical axis.
 * @param units  the value displayed on vertical axis.
 * @param family Submenu that the chart will be attached on dashboard.
 * @param charttype chart type
 * @param order  the chart order
 * @param algorithm the algorithm used by dimension
 * @param context   add context for chart
 * @param module    chart module name, this is the eBPF thread.
 * @param update_every value to overwrite the update frequency set by the server.
 */
void ebpf_create_charts_on_systemd(ebpf_systemd_args_t *chart)
{
    ebpf_write_chart_cmd(
        chart->id,
        chart->suffix,
        "",
        chart->title,
        chart->units,
        chart->family,
        chart->charttype,
        chart->context,
        chart->order,
        chart->update_every,
        chart->module);
    char service_name[512];
    snprintfz(service_name, 511, "%s", (!strstr(chart->id, "systemd_")) ? chart->id : (chart->id + 8));
    ebpf_create_chart_labels("service_name", service_name, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
    // Let us keep original string that can be used in another place. Chart creation does not happen frequently.
    char *move = strdupz(chart->dimension);
    char *ptr = move;
    while (ptr) {
        char *next_dim = strchr(ptr, ',');
        if (next_dim) {
            *next_dim = '\0';
            next_dim++;
        }

        fprintf(stdout, "DIMENSION %s '' %s 1 1\n", ptr, chart->algorithm);
        ptr = next_dim;
    }
    freez(move);
}

// --------------------------------------------------------------------------------------------------------------------
// Cgroup main thread

/**
 * Cgroup integration
 *
 * Thread responsible to call functions responsible to sync data between plugins.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void ebpf_cgroup_integration(void *ptr __maybe_unused)
{
    int counter = NETDATA_EBPF_CGROUP_UPDATE - 1;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (!ebpf_plugin_stop()) {
        if (ebpf_plugin_stop())
            break;

        heartbeat_next(&hb);

        if (ebpf_plugin_stop())
            break;

        // refresh every NETDATA_EBPF_CGROUP_UPDATE seconds
        if (++counter >= NETDATA_EBPF_CGROUP_UPDATE) {
            counter = 0;
            if (!ebpf_cgroup_cache_initialized)
                ebpf_cgroup_cache_init();

            ebpf_parse_cgroup_netipc_data();
        }
    }
}
