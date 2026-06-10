// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

static netdata_ebpfgo_shared_pid_memory_t apps_ebpf_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

/* Set to true on the first successful SHM refresh; never reset.
 * Gates chart creation and data sending so no cachestat charts appear
 * when the Go plugin is disabled or not yet started. */
static bool apps_ebpf_cachestat_available = false;

static inline int64_t apps_ebpf_diff_counters(uint64_t current, uint64_t previous)
{
    if (current < previous)
        return 0;

    return (int64_t)(current - previous);
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
    if (ok)
        apps_ebpf_cachestat_available = true;
    return ok;
}

bool apps_ebpf_cachestat_is_available(void)
{
    return apps_ebpf_cachestat_available;
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
        return false;
    }

    memcpy(&p->ebpf, item, sizeof(p->ebpf));
    p->has_ebpf = true;
    return true;
}

#endif
