// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpfgo_shared_memory.h"
#include "ebpfgo_shm_liveness.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>

/* Bytes occupied by entries[] only (no header). */
static inline size_t ebpfgo_shm_entries_nbytes(size_t total)
{
    return total * sizeof(struct ebpf_pid_stat);
}

/* Returns the number of entries the SHM segment can hold, or 0 if the size is
 * invalid (too small to hold the header and at least one entry). */
static inline size_t ebpfgo_shm_stat_entry_count(const struct stat *st)
{
    size_t hdr = sizeof(struct ebpfgo_shm_header);
    if (st->st_size <= 0 || (size_t)st->st_size <= hdr + sizeof(struct ebpf_pid_stat) - 1)
        return 0;
    return ((size_t)st->st_size - hdr) / sizeof(struct ebpf_pid_stat);
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

    if (ctx->mapping) {
        nd_munmap(ctx->mapping, ctx->mapped_size);
        ctx->mapping = NULL;
        ctx->shm = NULL;
        ctx->mapped_size = 0;
    }

    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }

    freez(ctx->snapshot);
    ctx->snapshot = NULL;
    ctx->snapshot_total = 0;
    ctx->snapshot_cap = 0;

    ctx->shm_total = 0;
    ctx->shm_flags = 0;
    ctx->update_every_s = 0;
    ctx->last_publish_ut = 0;
    ctx->shm_dev = 0;
    ctx->shm_ino = 0;
}

static bool netdata_ebpfgo_shared_pid_snapshot_is_live(uint64_t last_publish_ut, usec_t now_ut, uint32_t update_every_s)
{
    return last_publish_ut != 0 &&
           now_ut >= last_publish_ut &&
           (now_ut - last_publish_ut) <= ebpfgo_shm_stale_timeout_ut(update_every_s);
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
    if (fstat(ctx->shm_fd, &st) != 0)
        goto fail;

    ctx->shm_total = ebpfgo_shm_stat_entry_count(&st);
    if (!ctx->shm_total)
        goto fail;

    ctx->shm_dev = st.st_dev;
    ctx->shm_ino = st.st_ino;
    ctx->mapped_size = (size_t)st.st_size;
    ctx->mapping = nd_mmap(NULL, ctx->mapped_size, PROT_READ, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        ctx->mapped_size = 0;
        goto fail;
    }
    /* entries[] follow the header; the header holds the per-module flags. */
    ctx->shm = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(struct ebpfgo_shm_header));

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

    /* Allocate at full segment capacity; shm_total never changes for a given
     * mapping, so this realloc fires at most once per segment lifetime. */
    if (ctx->snapshot_cap < ctx->shm_total) {
        ctx->snapshot = reallocz(ctx->snapshot, ebpfgo_shm_entries_nbytes(ctx->shm_total));
        ctx->snapshot_cap = ctx->shm_total;
    }

    /* Capture per-module validity flags, publish interval, and the commit
     * timestamp from the header.  Field read order within the semaphore hold
     * is unconstrained — no concurrent writer can run.  last_publish_ut is
     * the producer's commit stamp (written last, after entries and live_count);
     * reading it here gives the liveness timestamp for the post-snapshot check.
     * Producer write order: update_every_s, flags, entries[], live_count,
     * last_publish_ut.  If that order ever changes, update this comment. */
    const struct ebpfgo_shm_header *hdr = (const struct ebpfgo_shm_header *)ctx->mapping;
    ctx->shm_flags      = __atomic_load_n(&hdr->flags,            __ATOMIC_ACQUIRE);
    ctx->update_every_s = __atomic_load_n(&hdr->update_every_s,   __ATOMIC_ACQUIRE);
    ctx->last_publish_ut = __atomic_load_n(&hdr->last_publish_ut, __ATOMIC_ACQUIRE);

    /* live_count is read separately because it bounds the memcpy below. */
    uint32_t live = __atomic_load_n(&hdr->live_count, __ATOMIC_ACQUIRE);
    if (live > (uint32_t)ctx->shm_total)
        live = (uint32_t)ctx->shm_total;

    /* Copy only the live entries.  The Go publisher sorts entries ascending by
     * pid before writing to SHM, so the previous qsort was redundant.  This
     * shrinks the per-cycle semaphore hold from a full ~17.5 MiB
     * memcpy + qsort(32768) to a few-KiB memcpy(live_count). */
    if (live)
        memcpy(ctx->snapshot, ctx->shm, live * sizeof(*ctx->snapshot));
    ctx->snapshot_total = live;

    return true;
}

bool netdata_ebpfgo_shared_pid_memory_refresh(
    netdata_ebpfgo_shared_pid_memory_t *ctx,
    const char *shm_name,
    const char *sem_name)
{
    if (!ctx || !shm_name)
        return false;

    if (ctx->shm) {
        int fd = shm_open(shm_name, O_RDONLY, 0);
        if (fd < 0) {
            netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
            return false;
        }

        struct stat st;
        if (fstat(fd, &st) != 0) {
            close(fd);
            netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
            return false;
        }

        bool changed =
            (st.st_dev != ctx->shm_dev || st.st_ino != ctx->shm_ino ||
             ebpfgo_shm_stat_entry_count(&st) != ctx->shm_total);
        close(fd);

        if (changed)
            netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
    }

    if (!ctx->shm && !netdata_ebpfgo_shared_pid_memory_open(ctx, shm_name, sem_name))
        return false;

    if (ctx->sem == SEM_FAILED) {
        if (!netdata_ebpfgo_shared_pid_memory_open_sem(ctx, sem_name))
            return false;
    }

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            if (!netdata_ebpfgo_shared_pid_snapshot_is_live(ctx->last_publish_ut, ebpfgo_shm_now_monotonic_usec(), ctx->update_every_s)) {
                netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
                return false;
            }
            /* Return true if we have a prior snapshot, even with zero live entries:
             * snapshot_total may be 0 when the publisher is alive but tracks no PIDs. */
            return ctx->snapshot != NULL;
        }
        locked = true;
    }

    bool ok = netdata_ebpfgo_shared_pid_memory_copy_snapshot(ctx);

    if (locked)
        sem_post(ctx->sem);

    if (!ok)
        return false;

    if (!netdata_ebpfgo_shared_pid_snapshot_is_live(ctx->last_publish_ut, ebpfgo_shm_now_monotonic_usec(), ctx->update_every_s)) {
        netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
        return false;
    }

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

uint32_t netdata_ebpfgo_shared_pid_memory_flags(const netdata_ebpfgo_shared_pid_memory_t *ctx)
{
    return ctx ? ctx->shm_flags : 0;
}

void netdata_ebpfgo_shared_pid_memory_close(netdata_ebpfgo_shared_pid_memory_t *ctx)
{
    netdata_ebpfgo_shared_pid_memory_close_internal(ctx);
}

#endif
