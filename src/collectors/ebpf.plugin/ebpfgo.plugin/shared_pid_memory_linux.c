// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_pid_memory.h"

#include <fcntl.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

struct shared_pid_memory {
    void *mapping;                    /* raw mmap base (for munmap and header access) */
    struct ebpfgo_shm_header *header; /* = mapping; holds per-module validity flags */
    struct ebpf_pid_stat *entries;    /* = (char*)mapping + sizeof(*header) */
    size_t total;
    size_t prev_count; /* entries written in the previous publish cycle */
    uint32_t update_every_s;
    int shm_fd;
    sem_t *sem;
};

static inline size_t shared_pid_memory_nbytes(const struct shared_pid_memory *ctx)
{
    return sizeof(struct ebpfgo_shm_header) + ctx->total * sizeof(struct ebpf_pid_stat);
}

static uint64_t shared_pid_memory_now_monotonic_usec(void)
{
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0)
        return 0;

    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)ts.tv_nsec / 1000ULL;
}

static void shared_pid_memory_invalidate(struct shared_pid_memory *ctx)
{
    if (!ctx || !ctx->header)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (ebpfgo_shm_sem_wait(ctx->sem))
            locked = true;
    }

    __atomic_store_n(&ctx->header->flags, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->last_publish_ut, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

struct shared_pid_memory *shared_pid_memory_open(size_t total, uint32_t update_every_s)
{
    if (!total)
        return NULL;

    struct shared_pid_memory *ctx = calloc(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;
    ctx->update_every_s = update_every_s;

    /* A normal Close() only closes this process' handles; it does not unlink
     * the SHM name.  Keeping the name present avoids a consumer refresh window
     * where shm_open by name fails during graceful eBPFGo restarts.  The
     * "reused" branch below fires when a prior segment still exists, either
     * after a graceful close or after a crash.
     *
     * We probe the pre-truncate size with fstat() to detect the reused
     * case: a non-zero size means the prior publisher may have left rows in
     * the segment, and the new run must clear them — the kernel only
     * zero-fills when shm_open creates a new segment.  If fstat() cannot
     * prove which state we opened, fail the open rather than optimistically
     * treating unknown state as brand new.  Clearing on the reuse path
     * preserves the original optimisation that avoids a 17.5 MB page-fault
     * storm on the first-publish-after-create path. */
    ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        goto fail;

    struct stat pre_stat;
    if (fstat(ctx->shm_fd, &pre_stat) != 0)
        goto fail;
    bool reused = pre_stat.st_size > 0;

    ctx->total = total;
    size_t length = shared_pid_memory_nbytes(ctx);
    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->mapping = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        goto fail;
    }
    ctx->header  = (struct ebpfgo_shm_header *)ctx->mapping;
    ctx->entries = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(*ctx->header));

    if (reused) {
        /* Reused SHM: the previous run may have written rows beyond
         * this run's high-water mark, and those rows would survive
         * forever (publish's tail-zeroing only operates on prev_count
         * which starts at 0).  Zero the segment so the consumer's
         * binary search does not return stale PIDs from the prior
         * run.  This is a one-time cost on restart, not per cycle. */
        memset(ctx->mapping, 0, length);
    }

    ctx->sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED)
        goto fail;

    /* POSIX guarantees a newly-created shm_open segment is zero-filled,
     * so when !reused we trust the kernel and skip the explicit memset
     * (this preserves the per-startup 17.5 MB page-fault saving). */
    return ctx;

fail:
    shared_pid_memory_close(ctx);
    return NULL;
}

int shared_pid_memory_publish(struct shared_pid_memory *ctx, const struct ebpf_pid_stat *entries, size_t count, uint32_t flags)
{
    if (!ctx || !ctx->mapping)
        return -1;

    if (count > ctx->total)
        count = ctx->total;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem))
            return -1;
        locked = true;
    }

    /* Set per-module validity flags and publish interval for this cycle.
     * update_every_s lets the reader compute a correctly-sized stale window
     * instead of using a hardcoded constant that breaks when update_every > 10. */
    __atomic_store_n(&ctx->header->update_every_s, ctx->update_every_s, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->flags, flags, __ATOMIC_RELEASE);

    if (entries && count)
        memcpy(ctx->entries, entries, count * sizeof(struct ebpf_pid_stat));

    /* Zero only the slots vacated since the previous cycle.  POSIX
     * guarantees the shm_open segment is zero-filled at create time, so
     * on the first call prev_count==0 and this is a no-op. */
    if (ctx->prev_count > count)
        memset(ctx->entries + count, 0,
               (ctx->prev_count - count) * sizeof(struct ebpf_pid_stat));

    ctx->prev_count = count;
    /* live_count is stored after entries are written so a reader that sees the
     * updated last_publish_ut is guaranteed to also see the correct live_count. */
    __atomic_store_n(&ctx->header->live_count, (uint32_t)count, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->last_publish_ut, shared_pid_memory_now_monotonic_usec(), __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);

    return 0;
}

void shared_pid_memory_close(struct shared_pid_memory *ctx)
{
    if (!ctx)
        return;

    shared_pid_memory_invalidate(ctx);

    if (ctx->sem != SEM_FAILED)
        sem_close(ctx->sem);

    if (ctx->mapping)
        munmap(ctx->mapping, shared_pid_memory_nbytes(ctx));

    if (ctx->shm_fd >= 0)
        close(ctx->shm_fd);

    free(ctx);
}
