// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

static bool cgroup_ebpfgo_cachestat_snapshot_ready = false;

static bool cgroup_ebpfgo_find_procs_path(char *path_buf, size_t path_buf_size, const char *cg_id)
{
    struct stat buf;

    if (cgroup_use_unified_cgroups) {
        snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_unified_base, cg_id);
        return stat(path_buf, &buf) == 0;
    }

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_cpuset_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_blkio_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_memory_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    path_buf[0] = '\0';
    return false;
}

static procfile *cgroup_ebpfgo_open_procfile_fd(const char *path)
{
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0)
        return NULL;

    char fd_path[64];
    snprintfz(fd_path, sizeof(fd_path), "/proc/self/fd/%d", fd);

    procfile *ff = procfile_open_no_log(fd_path, " \t:", PROCFILE_FLAG_DEFAULT);
    close(fd);

    return ff;
}

bool cgroup_ebpfgo_cachestat_refresh(void)
{
    cgroup_ebpfgo_cachestat_snapshot_ready = cgroup_ebpfgo_shared_memory_refresh();
    return cgroup_ebpfgo_cachestat_snapshot_ready;
}

static inline void cgroup_ebpfgo_cachestat_initialize(struct cgroup *cg)
{
    cg->cachestat.prev = cg->cachestat.current;
}

static inline void cgroup_ebpfgo_cachestat_calculate(struct cgroup *cg)
{
    int64_t mpa = (int64_t)cg->cachestat.current.mark_page_accessed - (int64_t)cg->cachestat.prev.mark_page_accessed;
    if (mpa < 0)
        mpa = 0;

    int64_t mbd = (int64_t)cg->cachestat.current.mark_buffer_dirty - (int64_t)cg->cachestat.prev.mark_buffer_dirty;
    if (mbd < 0)
        mbd = 0;

    int64_t apcl =
        (int64_t)cg->cachestat.current.add_to_page_cache_lru - (int64_t)cg->cachestat.prev.add_to_page_cache_lru;
    if (apcl < 0)
        apcl = 0;

    int64_t apd =
        (int64_t)cg->cachestat.current.account_page_dirtied - (int64_t)cg->cachestat.prev.account_page_dirtied;
    if (apd < 0)
        apd = 0;

    cg->cachestat.dirty = (long long)mbd;

    NETDATA_DOUBLE total = (NETDATA_DOUBLE)mpa - (NETDATA_DOUBLE)mbd;
    if (total < 0)
        total = 0;

    NETDATA_DOUBLE misses = (NETDATA_DOUBLE)apcl - (NETDATA_DOUBLE)apd;
    if (misses < 0)
        misses = 0;

    NETDATA_DOUBLE hits = total - misses;
    if (hits < 0) {
        misses = total;
        hits = 0;
    }

    NETDATA_DOUBLE ratio = (total > 0) ? hits / total : 1;

    cg->cachestat.ratio = (long long)(ratio * 100);
    cg->cachestat.hit = (long long)hits;
    cg->cachestat.miss = (long long)misses;
}

static void cgroup_ebpfgo_cachestat_sum_pids(struct cgroup *cg)
{
    char path_buf[FILENAME_MAX + 1];
    procfile *ff = NULL;
    uint64_t mpa = 0;
    uint64_t mbd = 0;
    uint64_t apcl = 0;
    uint64_t apd = 0;
    uint64_t ct = 0;

    cg->cachestat.prev = cg->cachestat.current;
    memset(&cg->cachestat.current, 0, sizeof(cg->cachestat.current));
    cg->cachestat.ratio = 0;
    cg->cachestat.dirty = 0;
    cg->cachestat.hit = 0;
    cg->cachestat.miss = 0;
    cg->cachestat.ct = 0;

    if (!cgroup_ebpfgo_find_procs_path(path_buf, sizeof(path_buf), cg->id))
        goto calculate;

    ff = cgroup_ebpfgo_open_procfile_fd(path_buf);
    if (!ff)
        goto calculate;

    procfile *read = procfile_readall(ff);
    if (!read) {
        procfile_close(ff);
        ff = NULL;
        goto calculate;
    }
    ff = read;

    for (size_t l = 0; l < procfile_lines(ff); l++) {
        pid_t pid = (pid_t)str2l(procfile_lineword(ff, l, 0));
        if (pid <= 0)
            continue;

        const struct ebpf_pid_stat *item = cgroup_ebpfgo_shared_memory_lookup(pid);
        if (!item)
            continue;

        const struct ebpf_cachestat *current = &item->cachestat.current;
        mpa += current->mark_page_accessed;
        mbd += current->mark_buffer_dirty;
        apcl += current->add_to_page_cache_lru;
        apd += current->account_page_dirtied;

        if (item->cachestat.ct > ct)
            ct = item->cachestat.ct;
    }

calculate:
    if (ff)
        procfile_close(ff);

    cg->cachestat.current.mark_page_accessed = (mpa > UINT32_MAX) ? UINT32_MAX : (uint32_t)mpa;
    cg->cachestat.current.mark_buffer_dirty = (mbd > UINT32_MAX) ? UINT32_MAX : (uint32_t)mbd;
    cg->cachestat.current.add_to_page_cache_lru = (apcl > UINT32_MAX) ? UINT32_MAX : (uint32_t)apcl;
    cg->cachestat.current.account_page_dirtied = (apd > UINT32_MAX) ? UINT32_MAX : (uint32_t)apd;
    cg->cachestat.ct = ct;

    if (!cg->cachestat.prev.mark_page_accessed && !cg->cachestat.prev.add_to_page_cache_lru &&
        !cg->cachestat.prev.mark_buffer_dirty && !cg->cachestat.prev.account_page_dirtied) {
        cgroup_ebpfgo_cachestat_initialize(cg);
        return;
    }

    cgroup_ebpfgo_cachestat_calculate(cg);
}

static void cgroup_ebpfgo_cachestat_update_single_chart(
    struct cgroup *cg,
    RRDSET **chart_ptr,
    const char *chart_id,
    const char *title,
    const char *context,
    const char *dimension,
    const char *units,
    int priority,
    collected_number value)
{
    RRDSET *chart = *chart_ptr;

    if (unlikely(!chart)) {
        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = *chart_ptr = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            chart_id,
            NULL,
            "page_cache",
            context,
            title,
            units,
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            priority,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, dimension, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, dimension, value);
    rrdset_done(chart);
}

void cgroup_ebpfgo_cachestat_update_locked(void)
{
    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        if (unlikely(!cg->enabled || cg->pending_renames))
            continue;

        cgroup_ebpfgo_cachestat_sum_pids(cg);
    }
}

void cgroup_ebpfgo_cachestat_update_charts(struct cgroup *cg)
{
    if (unlikely(!cg || !cg->enabled || cg->pending_renames))
        return;

    if (unlikely(!cgroup_ebpfgo_cachestat_snapshot_ready))
        return;

    const bool is_service = is_cgroup_systemd_service(cg);
    const char *ratio_context = is_service ? "services.cachestat_ratio" : "cgroup.cachestat_ratio";
    const char *dirty_context = is_service ? "services.cachestat_dirties" : "cgroup.cachestat_dirties";
    const char *hit_context = is_service ? "services.cachestat_hits" : "cgroup.cachestat_hits";
    const char *miss_context = is_service ? "services.cachestat_misses" : "cgroup.cachestat_misses";
    const int prio = (is_service ? NETDATA_CHART_PRIO_CGROUPS_SYSTEMD : NETDATA_CHART_PRIO_CGROUPS_CONTAINERS) + 5200;

    cgroup_ebpfgo_cachestat_update_single_chart(
        cg,
        &cg->st_cachestat_ratio,
        "cachestat_ratio",
        "Hit ratio",
        ratio_context,
        "ratio",
        "%",
        prio,
        (collected_number)cg->cachestat.ratio);

    cgroup_ebpfgo_cachestat_update_single_chart(
        cg,
        &cg->st_cachestat_dirties,
        "cachestat_dirties",
        "Number of dirty pages",
        dirty_context,
        "dirty",
        "page/s",
        prio + 1,
        (collected_number)cg->cachestat.dirty);

    cgroup_ebpfgo_cachestat_update_single_chart(
        cg,
        &cg->st_cachestat_hits,
        "cachestat_hits",
        "Number of accessed files",
        hit_context,
        "hit",
        "hits/s",
        prio + 2,
        (collected_number)cg->cachestat.hit);

    cgroup_ebpfgo_cachestat_update_single_chart(
        cg,
        &cg->st_cachestat_misses,
        "cachestat_misses",
        "Files out of page cache",
        miss_context,
        "miss",
        "misses/s",
        prio + 3,
        (collected_number)cg->cachestat.miss);
}

#endif
