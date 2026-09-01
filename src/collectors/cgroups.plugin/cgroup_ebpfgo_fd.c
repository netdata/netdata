// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_fd.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

// Snapshot readiness is set externally to share the single SHM refresh
// performed by cgroup_ebpfgo_cachestat_refresh() each tick.
static bool cgroup_ebpfgo_fd_snapshot_ready = false;

// Whether fd is publishing with `ebpf load mode = return`, i.e. whether the
// error charts may be created.  Sticky: once a chart exists it must keep
// receiving values, so a producer that later switches to `entry` mode does not
// retract charts that are already on the dashboard.
static bool cgroup_ebpfgo_fd_errors_ready = false;

static size_t cgroup_ebpfgo_fd_token_index(const struct cgroup *cg, pid_t pid, bool *found)
{
    size_t left = 0;
    size_t right = cg->ebpf_fd_pid_tokens_count;
    while (left < right) {
        size_t mid = left + (right - left) / 2;
        if (cg->ebpf_fd_pid_tokens[mid].pid < pid)
            left = mid + 1;
        else
            right = mid;
    }
    *found = left < cg->ebpf_fd_pid_tokens_count && cg->ebpf_fd_pid_tokens[left].pid == pid;
    return left;
}

static uint64_t *cgroup_ebpfgo_fd_consumed_token(struct cgroup *cg, pid_t pid)
{
    bool found;
    size_t index = cgroup_ebpfgo_fd_token_index(cg, pid, &found);
    if (!found) {
        if (cg->ebpf_fd_pid_tokens_count == cg->ebpf_fd_pid_tokens_capacity) {
            size_t capacity = cg->ebpf_fd_pid_tokens_capacity ? cg->ebpf_fd_pid_tokens_capacity * 2 : 16;
            cg->ebpf_fd_pid_tokens = reallocz(cg->ebpf_fd_pid_tokens,
                                               capacity * sizeof(*cg->ebpf_fd_pid_tokens));
            cg->ebpf_fd_pid_tokens_capacity = capacity;
        }
        memmove(&cg->ebpf_fd_pid_tokens[index + 1], &cg->ebpf_fd_pid_tokens[index],
                (cg->ebpf_fd_pid_tokens_count - index) * sizeof(*cg->ebpf_fd_pid_tokens));
        cg->ebpf_fd_pid_tokens[index] = (cgroup_ebpfgo_fd_pid_token_t){.pid = pid};
        cg->ebpf_fd_pid_tokens_count++;
    }
    return &cg->ebpf_fd_pid_tokens[index].ct;
}

static void cgroup_ebpfgo_fd_prune_tokens(struct cgroup *cg)
{
    size_t token = 0, pid = 0;
    while (token < cg->ebpf_fd_pid_tokens_count && pid < cg->ebpf_pids_count) {
        if (cg->ebpf_fd_pid_tokens[token].pid < cg->ebpf_pids[pid]) {
            memmove(&cg->ebpf_fd_pid_tokens[token], &cg->ebpf_fd_pid_tokens[token + 1],
                    (cg->ebpf_fd_pid_tokens_count - token - 1) * sizeof(*cg->ebpf_fd_pid_tokens));
            cg->ebpf_fd_pid_tokens_count--;
        } else if (cg->ebpf_fd_pid_tokens[token].pid > cg->ebpf_pids[pid])
            pid++;
        else {
            token++;
            pid++;
        }
    }
    cg->ebpf_fd_pid_tokens_count = token;
}

void cgroup_ebpfgo_fd_set_snapshot_ready(bool ready, bool errors)
{
    cgroup_ebpfgo_fd_snapshot_ready = ready;
    if (ready && errors)
        cgroup_ebpfgo_fd_errors_ready = true;
}

/* Sums this interval's file-descriptor activity for one cgroup.
 *
 * The SHM row already carries per-interval deltas — struct ebpf_publish_fd_stat
 * has a single counter set with no curr/prev pair — so unlike the cachestat and
 * dcstat consumers there is nothing to diff here.  cgroups.plugin ticks faster
 * than the Go publisher, so a PID only contributes when its ct is newer than the
 * token this cgroup consumed for that PID; without that gate the same delta would
 * be counted once per tick until the next publish. */
static void cgroup_ebpfgo_fd_sum_pids(struct cgroup *cg)
{
    cg->fd.open_call = 0;
    cg->fd.close_call = 0;
    cg->fd.open_err = 0;
    cg->fd.close_err = 0;

    /* Keep the consumed ct when the cgroup momentarily has no PIDs (an empty or
     * unreadable cgroup.procs).  Resetting it would make the next tick treat
     * every row as unconsumed and replay the last interval, spiking the charts. */
    if (!cg->ebpf_pids_count)
        return;

    long long open_call = 0;
    long long close_call = 0;
    long long open_err = 0;
    long long close_err = 0;

    for (size_t i = 0; i < cg->ebpf_pids_count; i++) {
        pid_t pid = cg->ebpf_pids[i];

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        const struct ebpf_publish_fd_stat *fd = &item->fd;

        uint64_t *consumed_ct = cgroup_ebpfgo_fd_consumed_token(cg, pid);
        if (fd->ct <= *consumed_ct)
            continue;

        uint32_t update_every_s = fd->fd_update_every_s;
        if (!update_every_s && cgroup_update_every > 0)
            update_every_s = (uint32_t)cgroup_update_every;

        open_call = cgroup_ebpfgo_fd_add_rate(
            open_call, cgroup_ebpfgo_fd_normalize_rate(fd->open_call, update_every_s));
        close_call = cgroup_ebpfgo_fd_add_rate(
            close_call, cgroup_ebpfgo_fd_normalize_rate(fd->close_call, update_every_s));
        open_err = cgroup_ebpfgo_fd_add_rate(
            open_err, cgroup_ebpfgo_fd_normalize_rate(fd->open_err, update_every_s));
        close_err = cgroup_ebpfgo_fd_add_rate(
            close_err, cgroup_ebpfgo_fd_normalize_rate(fd->close_err, update_every_s));
        *consumed_ct = fd->ct;
    }

    cgroup_ebpfgo_fd_prune_tokens(cg);

    cg->fd.open_call = open_call;
    cg->fd.close_call = close_call;
    cg->fd.open_err = open_err;
    cg->fd.close_err = close_err;
}

void cgroup_ebpfgo_fd_update_locked(void)
{
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames))
            continue;

        cgroup_ebpfgo_fd_sum_pids(cg);
    }
}

void cgroup_ebpfgo_fd_update_charts(struct cgroup *cg)
{
    if (!cg)
        return;

    if (!cg->enabled || cg->pending_renames)
        return;

    if (unlikely(!cgroup_ebpfgo_fd_snapshot_ready))
        return;

    /* Charts are created for every live cgroup while fd is publishing —
     * deliberately NOT conditioned on this cgroup having file-descriptor data,
     * matching the C module this replaced, which created them for every cgroup it
     * had refreshed that cycle and never tested the values:
     *     if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_FD_CHART) && ect->updated)
     *         ebpf_create_specific_fd_charts(ect->name, em);
     *
     * A values-based guard (the one cgroup_ebpfgo_cachestat.c keeps) would hide
     * these charts on any quiet interval, because every published value is a
     * per-interval delta and therefore 0 while the cgroup is idle.  Do not
     * "unify" the two: fd follows the C module it replaced, as dcstat does. */

    const bool is_service = is_cgroup_systemd_service(cg);
    const char *open_context = is_service ? "systemd.service.fd_open" : "cgroup.fd_open";
    const char *open_err_context = is_service ? "systemd.service.fd_open_error" : "cgroup.fd_open_error";
    const char *close_context = is_service ? "systemd.service.fd_close" : "cgroup.fd_close";
    const char *close_err_context = is_service ? "systemd.service.fd_close_error" : "cgroup.fd_close_error";
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5400;

    const collected_number divisor = CGROUP_EBPFGO_FD_RATE_SCALE;

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_fd_open,
        "fd_open",
        "Number of open files",
        "file_access",
        open_context,
        "calls",
        "calls/s",
        prio,
        divisor,
        (collected_number)cg->fd.open_call);

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_fd_close,
        "fd_close",
        "Files closed",
        "file_access",
        close_context,
        "calls",
        "calls/s",
        prio + 2,
        divisor,
        (collected_number)cg->fd.close_call);

    if (!cgroup_ebpfgo_fd_errors_ready)
        return;

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_fd_open_error,
        "fd_open_error",
        "Fails to open files",
        "file_access",
        open_err_context,
        "calls",
        "calls/s",
        prio + 1,
        divisor,
        (collected_number)cg->fd.open_err);

    cgroup_ebpfgo_update_single_chart(
        cg,
        &cg->st_fd_close_error,
        "fd_close_error",
        "Fails to close files",
        "file_access",
        close_err_context,
        "calls",
        "calls/s",
        prio + 3,
        divisor,
        (collected_number)cg->fd.close_err);
}

#endif
