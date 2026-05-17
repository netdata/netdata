// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include <errno.h>
#include <semaphore.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

static struct ebpf_pid_stat *apps_ebpf_shm = NULL;
static struct ebpf_pid_stat *apps_ebpf_snapshot = NULL;
static size_t apps_ebpf_shm_total = 0;
static size_t apps_ebpf_snapshot_total = 0;
static int apps_ebpf_shm_fd = -1;
static sem_t *apps_ebpf_sem = SEM_FAILED;
static dev_t apps_ebpf_shm_dev = 0;
static ino_t apps_ebpf_shm_ino = 0;

static int apps_ebpf_compare_pid(const void *a, const void *b)
{
    const struct ebpf_pid_stat *pa = a;
    const struct ebpf_pid_stat *pb = b;

    if (pa->pid < pb->pid)
        return -1;
    if (pa->pid > pb->pid)
        return 1;

    return 0;
}

static bool apps_ebpf_sem_wait(sem_t *sem)
{
    if (!sem || sem == SEM_FAILED) {
        errno = EINVAL;
        return false;
    }

    while (1) {
        struct timespec ts;
        if (clock_gettime(CLOCK_REALTIME, &ts) == -1)
            return false;

        ts.tv_nsec += 200 * 1000 * 1000;
        if (ts.tv_nsec >= 1000000000L) {
            ts.tv_sec += ts.tv_nsec / 1000000000L;
            ts.tv_nsec %= 1000000000L;
        }

        if (sem_timedwait(sem, &ts) == 0)
            return true;

        if (errno == EINTR)
            continue;

        if (errno == ETIMEDOUT)
            return false;

        return false;
    }
}

static void apps_ebpf_close_shared_memory(void)
{
    if (apps_ebpf_sem != SEM_FAILED) {
        sem_close(apps_ebpf_sem);
        apps_ebpf_sem = SEM_FAILED;
    }

    if (apps_ebpf_shm) {
        nd_munmap(apps_ebpf_shm, apps_ebpf_shm_total * sizeof(struct ebpf_pid_stat));
        apps_ebpf_shm = NULL;
    }

    if (apps_ebpf_shm_fd >= 0) {
        close(apps_ebpf_shm_fd);
        apps_ebpf_shm_fd = -1;
    }

    apps_ebpf_shm_total = 0;
    apps_ebpf_shm_dev = 0;
    apps_ebpf_shm_ino = 0;
}

static bool apps_ebpf_open_shared_memory(void)
{
    if (apps_ebpf_shm)
        return true;

    apps_ebpf_shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_RDONLY, 0);
    if (apps_ebpf_shm_fd < 0)
        return false;

    struct stat st;
    if (fstat(apps_ebpf_shm_fd, &st) != 0 || st.st_size <= 0 || (size_t)st.st_size < sizeof(struct ebpf_pid_stat))
        goto fail;

    apps_ebpf_shm_dev = st.st_dev;
    apps_ebpf_shm_ino = st.st_ino;
    apps_ebpf_shm_total = (size_t)st.st_size / sizeof(struct ebpf_pid_stat);
    apps_ebpf_shm = nd_mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, apps_ebpf_shm_fd, 0);
    if (apps_ebpf_shm == MAP_FAILED) {
        apps_ebpf_shm = NULL;
        goto fail;
    }

    apps_ebpf_sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, 0);
    if (apps_ebpf_sem == SEM_FAILED) {
        apps_ebpf_sem = SEM_FAILED;
    }

    return true;

fail:
    apps_ebpf_close_shared_memory();
    return false;
}

static bool apps_ebpf_refresh_shared_memory_mapping(void)
{
    if (!apps_ebpf_shm)
        return apps_ebpf_open_shared_memory();

    int fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_RDONLY, 0);
    if (fd < 0)
        return false;

    struct stat st;
    if (fstat(fd, &st) != 0) {
        close(fd);
        return false;
    }

    bool changed =
        (st.st_dev != apps_ebpf_shm_dev || st.st_ino != apps_ebpf_shm_ino ||
         st.st_size <= 0 || (size_t)st.st_size < sizeof(struct ebpf_pid_stat) ||
         (size_t)st.st_size / sizeof(struct ebpf_pid_stat) != apps_ebpf_shm_total);
    close(fd);

    if (!changed)
        return true;

    apps_ebpf_close_shared_memory();
    return apps_ebpf_open_shared_memory();
}

static bool apps_ebpf_copy_snapshot(void)
{
    if (!apps_ebpf_shm || !apps_ebpf_shm_total)
        return false;

    if (apps_ebpf_snapshot_total != apps_ebpf_shm_total) {
        apps_ebpf_snapshot = reallocz(apps_ebpf_snapshot, apps_ebpf_shm_total * sizeof(*apps_ebpf_snapshot));
        apps_ebpf_snapshot_total = apps_ebpf_shm_total;
    }

    memcpy(apps_ebpf_snapshot, apps_ebpf_shm, apps_ebpf_shm_total * sizeof(*apps_ebpf_snapshot));
    qsort(apps_ebpf_snapshot, apps_ebpf_shm_total, sizeof(*apps_ebpf_snapshot), apps_ebpf_compare_pid);
    return true;
}

static inline int64_t apps_ebpf_diff_counters(uint64_t current, uint64_t previous)
{
    if (current < previous)
        return 0;

    return (int64_t)(current - previous);
}

void apps_ebpf_accumulate_cachestat(void)
{
    for (struct pid_stat *p = root_of_pids(); p; p = p->next) {
        if (unlikely(!p->has_ebpf || !p->updated))
            continue;

        if (unlikely(!p->target))
            continue;
        break;
    }

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

        if (unlikely(!p->target))
            continue;

        struct target *w = p->target;
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
    if (!apps_ebpf_refresh_shared_memory_mapping())
        return false;

    bool locked = false;
    if (apps_ebpf_sem != SEM_FAILED) {
        if (!apps_ebpf_sem_wait(apps_ebpf_sem))
            return false;
        locked = true;
    }

    bool ok = apps_ebpf_copy_snapshot();

    if (locked)
        sem_post(apps_ebpf_sem);

    return ok;
}

static const struct ebpf_pid_stat *apps_ebpf_lookup_snapshot(pid_t pid)
{
    if (!apps_ebpf_snapshot || !apps_ebpf_snapshot_total)
        return NULL;

    size_t left = 0;
    size_t right = apps_ebpf_snapshot_total;

    while (left < right) {
        size_t mid = left + (right - left) / 2;
        const struct ebpf_pid_stat *item = &apps_ebpf_snapshot[mid];
        if (item->pid == (uint32_t)pid)
            return item;
        if (item->pid < (uint32_t)pid)
            left = mid + 1;
        else
            right = mid;
    }

    return NULL;
}

bool apps_ebpf_sync_pid_stat(struct pid_stat *p)
{
    if (!p || !apps_ebpf_snapshot)
        return false;

    const struct ebpf_pid_stat *item = apps_ebpf_lookup_snapshot(p->pid);
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
