// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

// Snapshot readiness is set externally to share the single SHM refresh
// performed by cgroup_ebpfgo_cachestat_refresh() each tick.
static bool cgroup_ebpfgo_dcstat_snapshot_ready = false;

void cgroup_ebpfgo_dcstat_set_snapshot_ready(bool ready)
{
    cgroup_ebpfgo_dcstat_snapshot_ready = ready;
}

static inline uint64_t cgroup_ebpfgo_dcstat_delta(uint64_t current, uint64_t previous)
{
    return (current > previous) ? current - previous : 0;
}

/* Sums this interval's directory-cache activity for one cgroup.
 *
 * The SHM row carries the raw cumulative counters in curr/prev, so the interval
 * delta is computed per PID here.  cgroups.plugin ticks faster than the Go
 * publisher, so a PID only contributes when its ct is newer than the ct this
 * cgroup consumed last tick — without that gate every delta would be counted
 * once per tick until the next publish.  This mirrors the cachestat consumer;
 * it can under-count a PID whose ct trails the cgroup's newest ct, but it never
 * double-counts. */
static void cgroup_ebpfgo_dcstat_sum_pids(struct cgroup *cg)
{
    uint64_t prev_ct = cg->dcstat.ct;
    uint64_t ct = 0;

    cg->dcstat.ratio = 0;
    cg->dcstat.reference = 0;
    cg->dcstat.slow = 0;
    cg->dcstat.not_found = 0;

    /* Keep the consumed ct when the cgroup momentarily has no PIDs (an empty or
     * unreadable cgroup.procs).  Resetting it would make the next tick treat
     * every row as unconsumed and replay the last interval, spiking the charts. */
    if (!cg->ebpf_pids_count)
        return;

    uint64_t reference = 0;
    uint64_t slow = 0;
    uint64_t not_found = 0;

    for (size_t i = 0; i < cg->ebpf_pids_count; i++) {
        pid_t pid = cg->ebpf_pids[i];

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        const struct ebpf_publish_dcstat *dc = &item->dc;

        if (dc->ct > ct)
            ct = dc->ct;

        if (dc->ct <= prev_ct)
            continue;

        reference += cgroup_ebpfgo_dcstat_delta(dc->curr.cache_access, dc->prev.cache_access);
        slow += cgroup_ebpfgo_dcstat_delta(dc->curr.file_system, dc->prev.file_system);
        not_found += cgroup_ebpfgo_dcstat_delta(dc->curr.not_found, dc->prev.not_found);
    }

    /* The consumed marker is a watermark and must only move forward.  It is the
     * maximum ct of whichever PIDs the cgroup happens to hold this tick, so a
     * PID leaving the cgroup can lower that maximum; adopting it would move the
     * boundary back and replay rows already accounted for on the next tick.
     * ct only ever decreases across a reboot, which restarts the agent and
     * zeroes this field anyway. */
    if (ct > cg->dcstat.ct)
        cg->dcstat.ct = ct;
    cg->dcstat.reference = (long long)reference;
    cg->dcstat.slow = (long long)slow;
    cg->dcstat.not_found = (long long)not_found;

    // Idle convention of the C dcstat collector: no lookups this interval means
    // a ratio of 0 (cachestat deliberately reports 100 in its idle case).
    if (reference) {
        uint64_t successful = (reference > not_found) ? reference - not_found : 0;
        cg->dcstat.ratio = (long long)((successful * 100) / reference);
    }
}

static void cgroup_ebpfgo_dcstat_update_single_chart(
    struct cgroup *cg,
    RRDSET **chart_ptr,
    const char *chart_id,
    const char *title,
    const char *context,
    const char *dimension,
    const char *units,
    int priority,
    collected_number divisor,
    collected_number value)
{
    RRDSET *chart = *chart_ptr;
    collected_number scale = divisor ? divisor : 1;

    if (unlikely(!chart)) {
        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = *chart_ptr = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            chart_id,
            NULL,
            "directory_cache",
            context,
            title,
            units,
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            priority,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, dimension, NULL, 1, scale, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, dimension, value);
    rrdset_done(chart);
}

void cgroup_ebpfgo_dcstat_update_locked(void)
{
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames))
            continue;

        cgroup_ebpfgo_dcstat_sum_pids(cg);
    }
}

void cgroup_ebpfgo_dcstat_update_charts(struct cgroup *cg)
{
    if (!cg)
        return;

    if (!cg->enabled || cg->pending_renames)
        return;

    if (unlikely(!cgroup_ebpfgo_dcstat_snapshot_ready))
        return;

    // Don't create charts until the cgroup has actual directory-cache activity.
    // Once st_dcstat_ratio exists the guard is skipped — charts persist on idle.
    if (!cg->st_dcstat_ratio &&
        !cg->dcstat.ratio && !cg->dcstat.reference &&
        !cg->dcstat.slow && !cg->dcstat.not_found)
        return;

    const bool is_service = is_cgroup_systemd_service(cg);
    const char *ratio_context = is_service ? "systemd.service.dc_ratio" : "cgroup.dc_ratio";
    const char *reference_context = is_service ? "systemd.service.dc_reference" : "cgroup.dc_reference";
    const char *not_cache_context = is_service ? "systemd.service.dc_not_cache" : "cgroup.dc_not_cache";
    const char *not_found_context = is_service ? "systemd.service.dc_not_found" : "cgroup.dc_not_found";
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5700;

    cgroup_ebpfgo_dcstat_update_single_chart(
        cg,
        &cg->st_dcstat_ratio,
        "dc_hit_ratio",
        "Percentage of files inside directory cache",
        ratio_context,
        "ratio",
        "%",
        prio,
        1,
        (collected_number)cg->dcstat.ratio);

    cgroup_ebpfgo_dcstat_update_single_chart(
        cg,
        &cg->st_dcstat_reference,
        "dc_reference",
        "Count file access",
        reference_context,
        "reference",
        "files/s",
        prio + 1,
        cgroup_update_every,
        (collected_number)cg->dcstat.reference);

    cgroup_ebpfgo_dcstat_update_single_chart(
        cg,
        &cg->st_dcstat_not_cache,
        "dc_not_cache",
        "Files not present inside directory cache",
        not_cache_context,
        "slow",
        "files/s",
        prio + 2,
        cgroup_update_every,
        (collected_number)cg->dcstat.slow);

    cgroup_ebpfgo_dcstat_update_single_chart(
        cg,
        &cg->st_dcstat_not_found,
        "dc_not_found",
        "Files not found",
        not_found_context,
        "miss",
        "files/s",
        prio + 3,
        cgroup_update_every,
        (collected_number)cg->dcstat.not_found);
}

#endif
