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

    /* Charts are created for every live cgroup while dcstat is publishing —
     * deliberately NOT conditioned on this cgroup having directory-cache data.
     *
     * This matches the C module this replaced, which created the charts for
     * every cgroup it had refreshed that cycle and never tested the values:
     *     if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_DC_CHART) && ect->updated)
     *         ebpf_create_specific_dc_charts(ect->name, update_every);
     *
     * Two narrower guards were tried here and both hid the charts:
     *   - a values-based guard, which for dcstat can only pass while lookups are
     *     actually happening (its idle ratio is 0, so every published value is 0
     *     on a quiet interval);
     *   - a presence-based guard (cg->dcstat.ct != 0), which hides any cgroup
     *     whose PIDs carry no dcstat row — path lookups are far sparser than
     *     page-cache activity, so cgroups routinely have none for long stretches.
     *
     * NOTE: cachestat deliberately differs — cgroup_ebpfgo_cachestat.c keeps a
     * values-based guard, and a cgroup with no page-cache rows never reaches its
     * calculate() step, so its charts stay hidden.  Do not "unify" the two:
     * dcstat follows the C module it replaced. */

    const bool is_service = is_cgroup_systemd_service(cg);
    const char *ratio_context = is_service ? "systemd.service.dc_ratio" : "cgroup.dc_ratio";
    const char *reference_context = is_service ? "systemd.service.dc_reference" : "cgroup.dc_reference";
    const char *not_cache_context = is_service ? "systemd.service.dc_not_cache" : "cgroup.dc_not_cache";
    const char *not_found_context = is_service ? "systemd.service.dc_not_found" : "cgroup.dc_not_found";
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5700;

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_dcstat_ratio,
        "dc_hit_ratio",
        "Percentage of directory lookups resolved by the cache",
        "directory_cache",
        ratio_context,
        "ratio",
        "%",
        prio,
        1,
        (collected_number)cg->dcstat.ratio);

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_dcstat_reference,
        "dc_reference",
        "Count file access",
        "directory_cache",
        reference_context,
        "reference",
        "files",
        prio + 1,
        1,
        (collected_number)cg->dcstat.reference);

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_dcstat_not_cache,
        "dc_not_cache",
        "Files not present inside directory cache",
        "directory_cache",
        not_cache_context,
        "slow",
        "files",
        prio + 2,
        1,
        (collected_number)cg->dcstat.slow);

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_dcstat_not_found,
        "dc_not_found",
        "Files not found",
        "directory_cache",
        not_found_context,
        "miss",
        "files",
        prio + 3,
        1,
        (collected_number)cg->dcstat.not_found);
}

#endif
