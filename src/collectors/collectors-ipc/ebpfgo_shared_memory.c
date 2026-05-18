// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

#include <errno.h>
#include <fcntl.h>
#include <semaphore.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>

static int netdata_ebpfgo_shared_pid_memory_compare_pid(const void *a, const void *b)
{
    const struct ebpf_pid_stat *pa = a;
    const struct ebpf_pid_stat *pb = b;

    if (pa->pid < pb->pid)
        return -1;
    if (pa->pid > pb->pid)
        return 1;

    return 0;
}

static bool netdata_ebpfgo_shared_pid_memory_sem_wait(sem_t *sem)
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

        return false;
    }
}

static bool netdata_ebpfgo_shared_pid_memory_open_sem(
    netdata_ebpfgo_shared_pid_memory_t *ctx,
    const char *sem_name)
{
    if (!ctx || !sem_name)
        return false;

    sem_t *sem = sem_open(sem_name, 0);
    if (sem == SEM_FAILED)
        return false;

    ctx->sem = sem;
    return true;
}

static void netdata_ebpfgo_shared_pid_memory_close_internal(netdata_ebpfgo_shared_pid_memory_t *ctx)
{
    if (!ctx)
        return;

    if (ctx->sem && ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }

    if (ctx->shm) {
        nd_munmap(ctx->shm, ctx->shm_total * sizeof(struct ebpf_pid_stat));
        ctx->shm = NULL;
    }

    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }

    freez(ctx->snapshot);
    ctx->snapshot = NULL;
    ctx->snapshot_total = 0;

    ctx->shm_total = 0;
    ctx->shm_dev = 0;
    ctx->shm_ino = 0;
}

static bool netdata_ebpfgo_shared_pid_memory_open(
    netdata_ebpfgo_shared_pid_memory_t *ctx,
    const char *shm_name,
    const char *sem_name)
{
    if (!ctx || !shm_name)
        return false;

    if (ctx->shm)
        return true;

    ctx->shm_fd = shm_open(shm_name, O_RDONLY, 0);
    if (ctx->shm_fd < 0)
        return false;

    struct stat st;
    if (fstat(ctx->shm_fd, &st) != 0 || st.st_size <= 0 || (size_t)st.st_size < sizeof(struct ebpf_pid_stat))
        goto fail;

    ctx->shm_dev = st.st_dev;
    ctx->shm_ino = st.st_ino;
    ctx->shm_total = (size_t)st.st_size / sizeof(struct ebpf_pid_stat);
    ctx->shm = nd_mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->shm == MAP_FAILED) {
        ctx->shm = NULL;
        goto fail;
    }

    if (sem_name)
        netdata_ebpfgo_shared_pid_memory_open_sem(ctx, sem_name);

    return true;

fail:
    netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
    return false;
}

static bool netdata_ebpfgo_shared_pid_memory_copy_snapshot(netdata_ebpfgo_shared_pid_memory_t *ctx)
{
    if (!ctx || !ctx->shm || !ctx->shm_total)
        return false;

    if (ctx->snapshot_total != ctx->shm_total) {
        ctx->snapshot = reallocz(ctx->snapshot, ctx->shm_total * sizeof(*ctx->snapshot));
        ctx->snapshot_total = ctx->shm_total;
    }

    memcpy(ctx->snapshot, ctx->shm, ctx->shm_total * sizeof(*ctx->snapshot));
    qsort(ctx->snapshot, ctx->shm_total, sizeof(*ctx->snapshot), netdata_ebpfgo_shared_pid_memory_compare_pid);
    return true;
}

bool netdata_ebpfgo_shared_pid_memory_refresh(
    netdata_ebpfgo_shared_pid_memory_t *ctx,
    const char *shm_name,
    const char *sem_name)
{
    if (!ctx || !shm_name)
        return false;

    if (!ctx->shm) {
        if (!netdata_ebpfgo_shared_pid_memory_open(ctx, shm_name, sem_name))
            return false;
    } else {
        int fd = shm_open(shm_name, O_RDONLY, 0);
        if (fd < 0) {
            netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
            return false;
        }

        struct stat st;
        if (fstat(fd, &st) != 0) {
            close(fd);
            return false;
        }

        bool changed =
            (st.st_dev != ctx->shm_dev || st.st_ino != ctx->shm_ino ||
             st.st_size <= 0 || (size_t)st.st_size < sizeof(struct ebpf_pid_stat) ||
             (size_t)st.st_size / sizeof(struct ebpf_pid_stat) != ctx->shm_total);
        close(fd);

        if (changed) {
            netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
            if (!netdata_ebpfgo_shared_pid_memory_open(ctx, shm_name, sem_name))
                return false;
        }
    }

    if (ctx->sem == SEM_FAILED) {
        if (!netdata_ebpfgo_shared_pid_memory_open_sem(ctx, sem_name))
            return false;
    }

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!netdata_ebpfgo_shared_pid_memory_sem_wait(ctx->sem)) {
            return ctx->snapshot && ctx->snapshot_total;
        }
        locked = true;
    }

    bool ok = netdata_ebpfgo_shared_pid_memory_copy_snapshot(ctx);

    if (locked)
        sem_post(ctx->sem);

    return ok;
}

const struct ebpf_pid_stat *netdata_ebpfgo_shared_pid_memory_lookup(
    const netdata_ebpfgo_shared_pid_memory_t *ctx,
    pid_t pid)
{
    if (!ctx || !ctx->snapshot || !ctx->snapshot_total)
        return NULL;

    size_t left = 0;
    size_t right = ctx->snapshot_total;

    while (left < right) {
        size_t mid = left + (right - left) / 2;
        const struct ebpf_pid_stat *item = &ctx->snapshot[mid];
        if (item->pid == (uint32_t)pid)
            return item;
        if (item->pid < (uint32_t)pid)
            left = mid + 1;
        else
            right = mid;
    }

    return NULL;
}

void netdata_ebpfgo_shared_pid_memory_close(netdata_ebpfgo_shared_pid_memory_t *ctx)
{
    netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
}

#endif
