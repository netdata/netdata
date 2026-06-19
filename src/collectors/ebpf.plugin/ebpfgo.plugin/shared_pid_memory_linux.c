// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_pid_memory.h"

#include <fcntl.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

struct shared_pid_memory {
    void *mapping;                    /* raw mmap base (for munmap and header access) */
    struct ebpfgo_shm_header *header; /* = mapping; holds per-module validity flags */
    struct ebpf_pid_stat *entries;    /* = (char*)mapping + sizeof(*header) */
    size_t total;
    size_t prev_count; /* entries written in the previous publish cycle */
    int shm_fd;
    sem_t *sem;
};

static inline size_t shared_pid_memory_nbytes(const struct shared_pid_memory *ctx)
{
    return sizeof(struct ebpfgo_shm_header) + ctx->total * sizeof(struct ebpf_pid_stat);
}

static void shared_pid_memory_unlink_all(void)
{
    (void)shm_unlink(NETDATA_EBPFGO_INTEGRATION_NAME);
    (void)sem_unlink(NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
}

struct shared_pid_memory *shared_pid_memory_open(size_t total)
{
    if (!total)
        return NULL;

    struct shared_pid_memory *ctx = calloc(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;

    /* A normal Close() unlinks both objects (see shared_pid_memory_close
     * below), so on a clean restart shm_open with O_CREAT creates a fresh
     * segment and the kernel zero-fills it.  The "reused" branch below
     * therefore fires only on crash-restart: the prior publisher died
     * before its Close() could unlink, the SHM name persists in the
     * kernel, and the new shm_open reopens the same inode.  This is
     * preferable to the previous behaviour (unlink on every open) which
     * deleted the SHM the previous publisher was still writing to,
     * breaking mutual exclusion on netdata respawn.
     *
     * We probe the pre-truncate size with fstat() to detect the reused
     * case: a non-zero size means the crashed publisher left rows in the
     * segment, and the new run must clear them — the kernel only
     * zero-fills when shm_open creates a new segment.  Clearing on the
     * reuse path preserves the original optimisation that avoids a
     * 17.5 MB page-fault storm on the first-publish-after-create path. */
    ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        goto fail;

    struct stat pre_stat;
    bool reused = (fstat(ctx->shm_fd, &pre_stat) == 0) && (pre_stat.st_size > 0);

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

    /* Reset the per-module flags at the start of every cycle so a module
     * that stops publishing has its bit cleared on the next consumer read.
     * The caller's flags are then OR'd in.  Producers that publish
     * concurrently (cachestat and socket) each call shared_pid_memory_publish
     * in their own goroutine, but the OR is non-destructive per-bit and
     * the same reset/OR pattern is used by the Go store on the activeModules
     * side (see cachestat_shared_memory.go:UpdateApps / UpdateSocketApps). */
    ctx->header->flags = 0;
    ctx->header->flags |= flags;

    if (entries && count)
        memcpy(ctx->entries, entries, count * sizeof(struct ebpf_pid_stat));

    /* Zero only the slots vacated since the previous cycle.  POSIX
     * guarantees the shm_open segment is zero-filled at create time, so
     * on the first call prev_count==0 and this is a no-op. */
    if (ctx->prev_count > count)
        memset(ctx->entries + count, 0,
               (ctx->prev_count - count) * sizeof(struct ebpf_pid_stat));

    ctx->prev_count = count;

    if (locked)
        sem_post(ctx->sem);

    return 0;
}

void shared_pid_memory_close(struct shared_pid_memory *ctx)
{
    if (!ctx)
        return;

    if (ctx->sem != SEM_FAILED)
        sem_close(ctx->sem);

    if (ctx->mapping)
        munmap(ctx->mapping, shared_pid_memory_nbytes(ctx));

    if (ctx->shm_fd >= 0)
        close(ctx->shm_fd);

    shared_pid_memory_unlink_all();

    free(ctx);
}
