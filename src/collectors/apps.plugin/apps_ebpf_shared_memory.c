// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

static netdata_ebpfgo_shared_pid_memory_t apps_ebpf_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

/* Sticky flags: set once the SHM refresh succeeds and the publisher has stamped
 * the module's EBPFGO_SHM_FLAG_* bit; never reset.  They guard chart creation so
 * that a module's charts never appear when the Go plugin is disabled, not yet
 * started, or running with a different subset of modules enabled. */
static bool apps_ebpf_cachestat_available = false;
static bool apps_ebpf_dcstat_available = false;
static bool apps_ebpf_fd_available = false;
static uint32_t apps_ebpf_shm_flags = 0;

/* Whether fd is publishing with `ebpf load mode = return`, i.e. whether its
 * error charts may be created.  Sticky like the availability flags above: once
 * the charts exist they must keep receiving values, and pluginsd logs an error
 * for every SET to a chart that was never declared. */
static bool apps_ebpf_fd_errors_available = false;

/* Per-cycle flag: set each cycle to the refresh() return value.  Prevents
 * stale data emission when the producer dies after its first successful
 * publish — the sticky flag above keeps charts alive, but SET calls must
 * be gated on a current-cycle refresh succeeding. */
static bool apps_ebpf_last_refresh_ok = false;

static inline int64_t apps_ebpf_diff_counters(uint64_t current, uint64_t previous)
{
    if (current < previous)
        return 0;

    return (int64_t)(current - previous);
}

static inline void apps_ebpf_add_fd_totals(struct target *w, const struct ebpf_publish_fd_stat *fd)
{
    if (!w)
        return;

    w->fd_totals.open_call += fd->open_call;
    w->fd_totals.close_call += fd->close_call;
    w->fd_totals.open_err += fd->open_err;
    w->fd_totals.close_err += fd->close_err;
}

void apps_ebpf_accumulate_cachestat(void)
{
    // dirty/hit/miss are monotonic accumulators that only grow; do not zero them.
    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        w->cachestat_totals_prev = w->cachestat_totals;
        memset(&w->cachestat_totals, 0, sizeof(w->cachestat_totals));
        w->cachestat.ct = 0;
        w->cachestat.ratio = 0;
    }

    for (struct pid_stat *p = root_of_pids(); p; p = p->next) {
        if (unlikely(!p->has_ebpf || !p->updated))
            continue;

        struct target *w = p->target;
        if (!w)
            continue;

        const struct ebpf_cachestat *current = &p->ebpf.cachestat.current;

        w->cachestat_totals.account_page_dirtied += current->account_page_dirtied;
        w->cachestat_totals.add_to_page_cache_lru += current->add_to_page_cache_lru;
        w->cachestat_totals.mark_buffer_dirty += current->mark_buffer_dirty;
        w->cachestat_totals.mark_page_accessed += current->mark_page_accessed;

        if (p->ebpf.cachestat.ct > w->cachestat.ct)
            w->cachestat.ct = p->ebpf.cachestat.ct;

        // Reset the ct gate on regression (Go plugin restart, map reset, SHM
        // inode swap, PID reuse) so deltas are not permanently suppressed.
        if (p->ebpf.cachestat.ct < p->ebpf_cachestat_ct)
            p->ebpf_cachestat_ct = 0;

        // Only add this PID's Go-computed delta when the Go plugin has published
        // fresh data (ct advanced).  This keeps dirty/hit/miss strictly
        // monotonic even when PIDs exit or are reclassified between cycles.
        if (p->ebpf.cachestat.ct > p->ebpf_cachestat_ct) {
            w->cachestat.dirty += p->ebpf.cachestat.dirty;
            w->cachestat.hit   += p->ebpf.cachestat.hit;
            w->cachestat.miss  += p->ebpf.cachestat.miss;
            p->ebpf_cachestat_ct = p->ebpf.cachestat.ct;
        }
    }

    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        // ratio uses per-interval deltas for current-interval accuracy
        int64_t mpa = apps_ebpf_diff_counters(w->cachestat_totals.mark_page_accessed, w->cachestat_totals_prev.mark_page_accessed);
        int64_t mbd = apps_ebpf_diff_counters(w->cachestat_totals.mark_buffer_dirty, w->cachestat_totals_prev.mark_buffer_dirty);
        int64_t apcl = apps_ebpf_diff_counters(w->cachestat_totals.add_to_page_cache_lru, w->cachestat_totals_prev.add_to_page_cache_lru);
        int64_t apd = apps_ebpf_diff_counters(w->cachestat_totals.account_page_dirtied, w->cachestat_totals_prev.account_page_dirtied);

        int64_t total = mpa - mbd;
        if (total < 0)
            total = 0;
        int64_t misses = apcl - apd;
        if (misses < 0)
            misses = 0;
        int64_t hits = total - misses;
        if (hits < 0)
            hits = 0;
        w->cachestat.ratio = (total > 0) ? (hits * 100) / total : 100;
    }
}

bool apps_ebpf_shared_memory_refresh(void)
{
    bool ok = netdata_ebpfgo_shared_pid_memory_refresh(
        &apps_ebpf_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
    /* Publish the per-cycle result before the sticky availability flags: a reader
     * that sampled between the two writes would otherwise see available = true
     * with last_refresh_ok still false and skip a cycle.  Today every caller is
     * on the same thread, so this is ordering hygiene rather than a live race. */
    apps_ebpf_last_refresh_ok = ok;

    if (ok) {
        uint32_t flags = netdata_ebpfgo_shared_pid_memory_flags(&apps_ebpf_shared_memory_ctx);
        apps_ebpf_shm_flags = flags;
        if (flags & EBPFGO_SHM_FLAG_CACHESTAT)
            apps_ebpf_cachestat_available = true;
        if (flags & EBPFGO_SHM_FLAG_DCSTAT)
            apps_ebpf_dcstat_available = true;
        if (flags & EBPFGO_SHM_FLAG_FD) {
            apps_ebpf_fd_available = true;
            if (flags & EBPFGO_SHM_FLAG_FD_ERRORS)
                apps_ebpf_fd_errors_available = true;
        }
    }
    return ok;
}

bool apps_ebpf_cachestat_data_ready(void)
{
    return apps_ebpf_cachestat_available && apps_ebpf_last_refresh_ok;
}

bool apps_ebpf_cachestat_is_available(void)
{
    return apps_ebpf_cachestat_available;
}

/* Sums the per-PID directory-cache deltas the Go plugin published into each
 * target. All values are interval totals, matching the absolute chart contract
 * used by the C dcstat collector. */
void apps_ebpf_accumulate_dcstat(void)
{
    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        w->dcstat_totals.reference = 0;
        w->dcstat_totals.slow = 0;
        w->dcstat_totals.not_found = 0;
        w->dcstat_totals.ratio = 0;
    }

    for (struct pid_stat *p = root_of_pids(); p; p = p->next) {
        if (unlikely(!p->has_ebpf || !p->updated))
            continue;

        struct target *w = p->target;
        if (!w)
            continue;

        // Reset the ct gate on regression (Go plugin restart, map reset, SHM
        // inode swap, PID reuse) so deltas are not permanently suppressed.
        if (p->ebpf.dc.ct < p->ebpf_dcstat_ct)
            p->ebpf_dcstat_ct = 0;

        // Only consume this PID's counters when the Go plugin published fresh
        // data (ct advanced).  Per-PID deltas keep the target totals monotonic
        // even when PIDs exit or are reclassified between cycles.
        if (p->ebpf.dc.ct <= p->ebpf_dcstat_ct)
            continue;

        int64_t reference = apps_ebpf_diff_counters(p->ebpf.dc.curr.cache_access, p->ebpf.dc.prev.cache_access);
        int64_t slow = apps_ebpf_diff_counters(p->ebpf.dc.curr.file_system, p->ebpf.dc.prev.file_system);
        int64_t not_found = apps_ebpf_diff_counters(p->ebpf.dc.curr.not_found, p->ebpf.dc.prev.not_found);

        w->dcstat_totals.reference += (uint64_t)reference;
        w->dcstat_totals.slow += (uint64_t)slow;
        w->dcstat_totals.not_found += (uint64_t)not_found;

        p->ebpf_dcstat_ct = p->ebpf.dc.ct;
    }

    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        uint64_t reference = w->dcstat_totals.reference;
        if (!reference)
            continue; // no lookups this interval: ratio stays 0 (C collector convention)

        uint64_t not_found = w->dcstat_totals.not_found;
        uint64_t successful = (reference > not_found) ? reference - not_found : 0;
        w->dcstat_totals.ratio = (int64_t)((successful * 100) / reference);
    }
}

bool apps_ebpf_dcstat_data_ready(void)
{
    return apps_ebpf_dcstat_available && apps_ebpf_last_refresh_ok;
}

bool apps_ebpf_dcstat_is_available(void)
{
    return apps_ebpf_dcstat_available;
}

/* Sums the per-PID file-descriptor deltas the Go plugin published into each
 * target.
 *
 * The totals are MONOTONIC, not per-interval: the fd charts use the incremental
 * algorithm, so zeroing them each cycle would make every point a spike.  This
 * mirrors apps_ebpf_accumulate_cachestat()'s hit/miss handling rather than
 * apps_ebpf_accumulate_dcstat()'s absolute one. */
void apps_ebpf_accumulate_fd(void)
{
    for (struct pid_stat *p = root_of_pids(); p; p = p->next) {
        if (unlikely(!p->has_ebpf || !p->updated))
            continue;

        struct target *w = p->target;
        if (!w)
            continue;

        // Reset the ct gate on regression (Go plugin restart, map reset, SHM
        // inode swap, PID reuse) so deltas are not permanently suppressed.
        if (p->ebpf.fd.ct < p->ebpf_fd_ct)
            p->ebpf_fd_ct = 0;

        // Only consume this PID's counters when the Go plugin published fresh
        // data (ct advanced).  Per-PID deltas keep the target totals monotonic
        // even when PIDs exit or are reclassified between cycles.
        if (p->ebpf.fd.ct <= p->ebpf_fd_ct)
            continue;

        apps_ebpf_add_fd_totals(w, &p->ebpf.fd);
#if (PROCESSES_HAVE_UID == 1)
        if (enable_users_charts)
            apps_ebpf_add_fd_totals(p->uid_target, &p->ebpf.fd);
#endif
#if (PROCESSES_HAVE_GID == 1)
        if (enable_groups_charts)
            apps_ebpf_add_fd_totals(p->gid_target, &p->ebpf.fd);
#endif
#if (PROCESSES_HAVE_SID == 1)
        if (enable_users_charts)
            apps_ebpf_add_fd_totals(p->sid_target, &p->ebpf.fd);
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
        if (enable_services_charts)
            apps_ebpf_add_fd_totals(p->service_target, &p->ebpf.fd);
#endif

        p->ebpf_fd_ct = p->ebpf.fd.ct;
    }
}

bool apps_ebpf_fd_data_ready(void)
{
    return apps_ebpf_fd_available && apps_ebpf_last_refresh_ok &&
           (apps_ebpf_shm_flags & EBPFGO_SHM_FLAG_FD);
}

bool apps_ebpf_fd_is_available(void)
{
    return apps_ebpf_fd_available;
}

bool apps_ebpf_fd_errors_are_available(void)
{
    return apps_ebpf_fd_errors_available;
}

bool apps_ebpf_sync_pid_stat(struct pid_stat *p)
{
    if (!p)
        return false;

    const struct ebpf_pid_stat *item =
        netdata_ebpfgo_shared_pid_memory_lookup(&apps_ebpf_shared_memory_ctx, p->pid);
    if (!item) {
        p->has_ebpf = false;
        memset(&p->ebpf, 0, sizeof(p->ebpf));
        p->ebpf_cachestat_ct = 0;
        p->ebpf_dcstat_ct = 0;
        p->ebpf_fd_ct = 0;
        return false;
    }

    memcpy(&p->ebpf, item, sizeof(p->ebpf));
    p->has_ebpf = true;
    return true;
}

#endif
