// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_rate.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

// Snapshot readiness is set externally to share the single SHM refresh
// performed by cgroup_ebpfgo_cachestat_refresh() each tick.
static bool cgroup_ebpfgo_process_snapshot_ready = false;

// Current producer mode controls creation; an existing chart keeps receiving
// values after a mode change.
static bool cgroup_ebpfgo_process_errors_current = false;

static cgroup_ebpfgo_rate_table_t cgroup_ebpfgo_process_rates;

void cgroup_ebpfgo_process_set_snapshot_ready(bool ready, bool errors)
{
    cgroup_ebpfgo_process_snapshot_ready = ready;
    cgroup_ebpfgo_process_errors_current = ready && errors;
}

/* Sums this interval's task-lifecycle rates for one cgroup.  The SHM row carries
 * per-interval deltas (ebpf_process_stat), consumed as in the fd consumer. */
static void cgroup_ebpfgo_process_sum_pids(struct cgroup *cg, uint32_t chart_every)
{
    memset(&cg->process, 0, sizeof(cg->process));

    if (!cg->ebpf_pids_count)
        return;

    long long totals[5] = {0};

    for (size_t i = 0; i < cg->ebpf_pids_count; i++) {
        pid_t pid = cg->ebpf_pids[i];

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        const struct ebpf_process_stat *ps = &item->process;
        const uint32_t deltas[] = {
            ps->create_process, ps->create_thread, ps->exit_call, ps->release_call, ps->task_err};
        cgroup_ebpfgo_rate_add_pid(
            &cgroup_ebpfgo_process_rates, pid, cg, ps->ct, ps->process_update_every_s, chart_every,
            deltas, _countof(deltas), totals);
    }

    cg->process.process_create = totals[0];
    cg->process.thread_create = totals[1];
    cg->process.task_exit = totals[2];
    cg->process.task_close = totals[3];
    cg->process.task_error = totals[4];
}

void cgroup_ebpfgo_process_update_locked(void)
{
    uint32_t chart_every = cgroup_update_every > 0 ? (uint32_t)cgroup_update_every : 1;

    cgroup_ebpfgo_rate_begin_cycle(&cgroup_ebpfgo_process_rates);
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames))
            continue;

        cgroup_ebpfgo_process_sum_pids(cg, chart_every);
    }
    cgroup_ebpfgo_rate_end_cycle(&cgroup_ebpfgo_process_rates, chart_every);
}

struct cgroup_ebpfgo_process_chart {
    size_t st_offset;
    size_t value_offset;
    const char *id;
    const char *title;
    // The C process module's dimension of the cgroup chart: id and shown name.
    // Its systemd service charts used a single `calls` dimension instead.
    const char *cgroup_dimension;
    const char *cgroup_dimension_name;
    int prio_offset;
    bool error_chart;
};

/* Chart ids, contexts, family, titles, dimensions and priorities match the C
 * process module (ebpf_create_specific_process_charts in the removed
 * ebpf_process.c), so existing chart instances keep resolving. */
static const struct cgroup_ebpfgo_process_chart cgroup_ebpfgo_process_charts[] = {
    {offsetof(struct cgroup, st_process_create), offsetof(cgroup_ebpfgo_publish_process_t, process_create),
     "process_create", "Process started", "_do_fork", "process", 0, false},
    {offsetof(struct cgroup, st_thread_create), offsetof(cgroup_ebpfgo_publish_process_t, thread_create),
     "thread_create", "Threads started", "sys_clone", "thread", 1, false},
    {offsetof(struct cgroup, st_task_exit), offsetof(cgroup_ebpfgo_publish_process_t, task_exit),
     "task_exit", "Tasks starts exit process.", "do_exit", "process", 2, false},
    {offsetof(struct cgroup, st_task_close), offsetof(cgroup_ebpfgo_publish_process_t, task_close),
     "task_close", "Tasks closed", "release_task", "task", 3, false},
    {offsetof(struct cgroup, st_task_error), offsetof(cgroup_ebpfgo_publish_process_t, task_error),
     "task_error", "Errors to create process or threads.", "do_exit", "process", 4, true},
};

void cgroup_ebpfgo_process_update_charts(struct cgroup *cg)
{
    if (!cg)
        return;

    if (!cg->enabled || cg->pending_renames)
        return;

    if (unlikely(!cgroup_ebpfgo_process_snapshot_ready))
        return;

    /* Created for every live cgroup while process is publishing, like the fd and
     * dcstat consumers: the values are per-interval rates, 0 while idle. */
    const bool is_service = is_cgroup_systemd_service(cg);
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5000;

    for (size_t i = 0; i < _countof(cgroup_ebpfgo_process_charts); i++) {
        const struct cgroup_ebpfgo_process_chart *c = &cgroup_ebpfgo_process_charts[i];
        RRDSET **st = (RRDSET **)((char *)cg + c->st_offset);

        if (c->error_chart && !cgroup_ebpfgo_process_errors_current && !*st)
            continue;

        char context[64];
        snprintfz(context, sizeof(context), "%s.%s", is_service ? "systemd.service" : "cgroup", c->id);

        long long value = *(const long long *)((const char *)&cg->process + c->value_offset);

        cgroup_ebpfgo_update_single_chart_named(
            cg,
            st,
            c->id,
            c->title,
            "processes",
            context,
            is_service ? "calls" : c->cgroup_dimension,
            is_service ? NULL : c->cgroup_dimension_name,
            "calls/s",
            prio + c->prio_offset,
            CGROUP_EBPFGO_RATE_SCALE,
            (collected_number)value);
    }
}

#endif
