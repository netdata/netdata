// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

static bool cgroup_ebpfgo_cachestat_snapshot_ready = false;

static procfile *cgroup_ebpfgo_open_procfile_fd(const char *path);

static procfile *cgroup_ebpfgo_open_nonempty_procs_file(char *path_buf, size_t path_buf_size, const char *cg_id)
{
    struct stat buf;
    const char *bases[] = {
        cgroup_unified_base,
        cgroup_cpuset_base,
        cgroup_blkio_base,
        cgroup_memory_base,
        cgroup_cpuacct_base,
    };
    char best_path[FILENAME_MAX + 1] = "";
    procfile *best = NULL;
    size_t best_lines = 0;

    if (cgroup_use_unified_cgroups) {
        snprintfz(path_buf, path_buf_size - 1, "%s%s", cgroup_unified_base, cg_id);
        if (stat(path_buf, &buf) == 0) {
            procfile *ff = cgroup_ebpfgo_open_procfile_fd(path_buf);
            if (ff) {
                procfile *read = procfile_readall(ff);
                if (read) {
                    size_t lines = procfile_lines(read);
                    if (lines > 0) {
                        if (path_buf && path_buf_size)
                            snprintfz(best_path, sizeof(best_path) - 1, "%s", path_buf);
                        best = read;
                    } else {
                        procfile_close(read);
                    }
                } else {
                    procfile_close(ff);
                }
            }
        }

        if (best) {
            if (path_buf && path_buf_size)
                snprintfz(path_buf, path_buf_size - 1, "%s", best_path);
            return best;
        }

        path_buf[0] = '\0';
        return NULL;
    }

    for (size_t i = 0; i < sizeof(bases) / sizeof(bases[0]); i++) {
        const char *base = bases[i];

        if (!base)
            continue;

        snprintfz(path_buf, path_buf_size - 1, "%s%s", base, cg_id);
        if (stat(path_buf, &buf) != 0)
            continue;

        procfile *ff = cgroup_ebpfgo_open_procfile_fd(path_buf);
        if (!ff)
            continue;

        procfile *read = procfile_readall(ff);
        if (!read) {
            procfile_close(ff);
            continue;
        }

        size_t lines = procfile_lines(read);
        if (lines == 0) {
            procfile_close(read);
            continue;
        }

        if (!best || lines > best_lines) {
            if (best)
                procfile_close(best);
            best = read;
            best_lines = lines;
            snprintfz(best_path, sizeof(best_path) - 1, "%s", path_buf);
        } else {
            procfile_close(read);
        }
    }

    if (best) {
        snprintfz(path_buf, path_buf_size - 1, "%s", best_path);
        return best;
    }

    path_buf[0] = '\0';
    return NULL;
}

static procfile *cgroup_ebpfgo_open_procfile_fd(const char *path)
{
    int dirfd = open(path, O_RDONLY | O_CLOEXEC | O_DIRECTORY);
    if (dirfd < 0)
        return NULL;

    int fd = openat(dirfd, "cgroup.procs", O_RDONLY | O_CLOEXEC);
    close(dirfd);
    if (fd < 0)
        return NULL;

    char fd_path[64];
    snprintfz(fd_path, sizeof(fd_path), "/proc/self/fd/%d", fd);

    procfile *ff = procfile_open_no_log(fd_path, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff) {
        close(fd);
        return NULL;
    }

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
    uint64_t mpa = 0;
    if (cg->cachestat.current.mark_page_accessed >= cg->cachestat.prev.mark_page_accessed)
        mpa = cg->cachestat.current.mark_page_accessed - cg->cachestat.prev.mark_page_accessed;

    uint64_t mbd = 0;
    if (cg->cachestat.current.mark_buffer_dirty >= cg->cachestat.prev.mark_buffer_dirty)
        mbd = cg->cachestat.current.mark_buffer_dirty - cg->cachestat.prev.mark_buffer_dirty;

    uint64_t apcl = 0;
    if (cg->cachestat.current.add_to_page_cache_lru >= cg->cachestat.prev.add_to_page_cache_lru)
        apcl = cg->cachestat.current.add_to_page_cache_lru - cg->cachestat.prev.add_to_page_cache_lru;

    uint64_t apd = 0;
    if (cg->cachestat.current.account_page_dirtied >= cg->cachestat.prev.account_page_dirtied)
        apd = cg->cachestat.current.account_page_dirtied - cg->cachestat.prev.account_page_dirtied;

    cg->cachestat.dirty = (long long)mbd;

    uint64_t total = (mpa > mbd) ? (mpa - mbd) : 0;

    uint64_t misses = (apcl > apd) ? (apcl - apd) : 0;

    uint64_t hits = (total > misses) ? (total - misses) : 0;
    if (hits == 0 && misses > total)
        misses = total;

    NETDATA_DOUBLE ratio = (total > 0) ? ((NETDATA_DOUBLE)hits / (NETDATA_DOUBLE)total) : 1;

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

    ff = cgroup_ebpfgo_open_nonempty_procs_file(path_buf, sizeof(path_buf), cg->id);
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

    cg->cachestat.current.mark_page_accessed = mpa;
    cg->cachestat.current.mark_buffer_dirty = mbd;
    cg->cachestat.current.add_to_page_cache_lru = apcl;
    cg->cachestat.current.account_page_dirtied = apd;
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
        rrddim_add(chart, dimension, NULL, 1, scale, RRD_ALGORITHM_ABSOLUTE);
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
    if (unlikely(!cg))
        return;

    if (unlikely(!cg->enabled || cg->pending_renames))
        return;

    if (unlikely(!cgroup_ebpfgo_cachestat_snapshot_ready))
        return;

    const bool is_service = is_cgroup_systemd_service(cg);
    const char *ratio_context = is_service ? "systemd.service.cachestat_ratio" : "cgroup.cachestat_ratio";
    const char *dirty_context = is_service ? "systemd.service.cachestat_dirties" : "cgroup.cachestat_dirties";
    const char *hit_context = is_service ? "systemd.service.cachestat_hits" : "cgroup.cachestat_hits";
    const char *miss_context = is_service ? "systemd.service.cachestat_misses" : "cgroup.cachestat_misses";
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
        1,
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
        cgroup_update_every,
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
        cgroup_update_every,
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
        cgroup_update_every,
        (collected_number)cg->cachestat.miss);
}

#endif
