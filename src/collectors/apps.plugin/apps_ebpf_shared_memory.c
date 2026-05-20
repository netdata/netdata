// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

static netdata_ebpfgo_shared_pid_memory_t apps_ebpf_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

static inline int64_t apps_ebpf_diff_counters(uint64_t current, uint64_t previous)
{
    if (current < previous)
        return 0;

    return (int64_t)(current - previous);
}

void apps_ebpf_accumulate_cachestat(void)
{
    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        w->cachestat_totals_prev = w->cachestat_totals;
        memset(&w->cachestat_totals, 0, sizeof(w->cachestat_totals));
        w->cachestat.ct = 0;
        w->cachestat.ratio = 0;
        w->cachestat.dirty = 0;
        w->cachestat.hit = 0;
        w->cachestat.miss = 0;
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
    }

    for (struct target *w = apps_groups_root_target; w; w = w->next) {
        int64_t mpa = apps_ebpf_diff_counters(w->cachestat_totals.mark_page_accessed, w->cachestat_totals_prev.mark_page_accessed);
        int64_t mbd = apps_ebpf_diff_counters(w->cachestat_totals.mark_buffer_dirty, w->cachestat_totals_prev.mark_buffer_dirty);
        int64_t apcl = apps_ebpf_diff_counters(w->cachestat_totals.add_to_page_cache_lru, w->cachestat_totals_prev.add_to_page_cache_lru);
        int64_t apd = apps_ebpf_diff_counters(w->cachestat_totals.account_page_dirtied, w->cachestat_totals_prev.account_page_dirtied);

        w->cachestat.dirty = mbd;

        int64_t total = mpa - mbd;
        if (total < 0)
            total = 0;

        int64_t misses = apcl - apd;
        if (misses < 0)
            misses = 0;

        int64_t hits = total - misses;
        if (hits < 0) {
            misses = total;
            hits = 0;
        }

        if (total > 0)
            w->cachestat.ratio = (hits * 100) / total;
        else
            w->cachestat.ratio = 100;

        w->cachestat.hit = hits;
        w->cachestat.miss = misses;
    }
}

bool apps_ebpf_shared_memory_refresh(void)
{
    return netdata_ebpfgo_shared_pid_memory_refresh(
        &apps_ebpf_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
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
        return false;
    }

    memcpy(&p->ebpf, item, sizeof(p->ebpf));
    p->has_ebpf = true;
    return true;
}

#endif
