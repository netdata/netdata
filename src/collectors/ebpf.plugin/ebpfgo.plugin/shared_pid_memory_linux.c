// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_pid_memory.h"

#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <stdio.h>
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
    uint32_t publish_timeouts; /* consecutive publish-side sem_timedwait failures */
    int shm_fd;
    sem_t *sem;
    bool shm_name_created; /* set when this context created/recreated the SHM name; triggers unlink on any exit */
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

    /* The semaphore guards the large entries[] array during publish.  These two
     * header scalars are written atomically (no tearing) and the reader detects
     * staleness via last_publish_ut == 0, so the writes are safe whether or not
     * we hold the semaphore.  This is the shutdown path; publish will not run
     * again on this context. */
    __atomic_store_n(&ctx->header->flags, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->last_publish_ut, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

/* Unlink and recreate both SHM and semaphore objects so that consumers
 * detect the new inode on their next refresh and re-open a semaphore that
 * starts at 1.  Called when sem_timedwait times out in the reused-segment
 * path: a timeout cannot distinguish a slow live holder from a crashed owner,
 * so posting without ownership is unsafe; replacing the generation is the
 * only protocol-correct recovery. */
static bool pid_shm_replace_generation(struct shared_pid_memory *ctx, size_t length)
{
    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }
    if (ctx->mapping) {
        munmap(ctx->mapping, length);
        ctx->mapping = NULL;
        ctx->header  = NULL;
        ctx->entries = NULL;
    }
    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }
    (void)sem_unlink(NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
    (void)shm_unlink(NETDATA_EBPFGO_INTEGRATION_NAME);

    ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        return false;
    /* Mark created before any further steps; close() unlinks on any failure path. */
    ctx->shm_name_created = true;

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        return false;

    ctx->mapping = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        return false;
    }
    ctx->header  = (struct ebpfgo_shm_header *)ctx->mapping;
    ctx->entries = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(*ctx->header));

    /* New SHM is kernel-zero-filled; no explicit memset needed. */
    ctx->sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED) {
        /* mapping is set but sem is not — next publish would write without a
         * lock.  Null out mapping so the publish guard catches it. */
        munmap(ctx->mapping, length);
        ctx->mapping = NULL;
        ctx->header  = NULL;
        ctx->entries = NULL;
        return false;
    }
    return true;
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

    /* close() calls shm_unlink so /dev/shm is not littered, but this also
     * means a graceful restart creates a fresh SHM object (new inode).
     * Consumers detect the inode change on their next refresh and reconnect
     * within one collection cycle.  The "reused" branch below fires only
     * after a crash (unlink did not run), where the prior segment still exists.
     *
     * We use O_CREAT|O_EXCL so shm_name_created is set only by the actual
     * creator — a racing second publisher that opens the same name sees EEXIST
     * and never sets shm_name_created, so a later failure on its side cannot
     * unlink a live publisher's segment.  On EEXIST we open without claiming
     * creation.  A non-zero pre-ftruncate size (reused=true) means a prior
     * publisher left rows that must be cleared — the kernel only zero-fills
     * fresh segments.  Clearing preserves the optimisation that avoids a
     * 17.5 MB page-fault storm on the first publish after creation. */
    struct stat pre_stat = {0};
    bool reused;
    ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_EXCL | O_RDWR, 0660);
    if (ctx->shm_fd >= 0) {
        reused = false;
        ctx->shm_name_created = true;
    } else if (errno == EEXIST) {
        ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_RDWR, 0);
        if (ctx->shm_fd < 0)
            goto fail;
        if (fstat(ctx->shm_fd, &pre_stat) != 0)
            goto fail;
        reused = pre_stat.st_size > 0;
    } else {
        goto fail;
    }

    ctx->total = total;
    size_t length = shared_pid_memory_nbytes(ctx);

    /* If a crashed writer left a segment whose size differs from what this
     * run needs, unlink and re-create to get a new inode.  Consumers detect
     * the inode change on their next refresh and remap, preventing a SIGBUS
     * from a stale larger mapping accessing pages past the shrunk file end. */
    if (reused && pre_stat.st_size != (off_t)length) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        (void)shm_unlink(NETDATA_EBPFGO_INTEGRATION_NAME);
        ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
        if (ctx->shm_fd < 0)
            goto fail;
        reused = false;
        ctx->shm_name_created = true;
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->mapping = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        goto fail;
    }
    ctx->header  = (struct ebpfgo_shm_header *)ctx->mapping;
    ctx->entries = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(*ctx->header));

    /* Do NOT sem_unlink here.  Unlinking followed by O_CREAT creates a new
     * semaphore object.  Consumers only reopen the semaphore when the SHM
     * inode changes; because close() preserves the SHM inode, a restart
     * with sem_unlink leaves consumers holding a stale handle to the old
     * object while the writer uses the new one — mutual exclusion gone.
     * Opening the existing semaphore (O_CREAT opens it if it already exists,
     * ignoring the initial value) keeps writer and all consumers on the same
     * underlying object. */
    ctx->sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED)
        goto fail;

    if (reused) {
        if (ebpfgo_shm_sem_wait(ctx->sem)) {
            /* Acquired: zero the segment so surviving consumers see a clean
             * slate, then release.  Zeroing last_publish_ut makes any
             * in-flight reader reject the data via is_live. */
            memset(ctx->mapping, 0, length);
            sem_post(ctx->sem);
        } else {
            /* Timed out: a slow live holder and a crashed owner both produce
             * the same timeout; posting without ownership is unsafe.  Replace
             * both SHM and semaphore so consumers detect the new inode on
             * their next refresh and re-open a semaphore that starts at 1. */
            if (!pid_shm_replace_generation(ctx, length))
                goto fail;
        }
    }
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
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            /* A consumer killed while holding the semaphore wedges it permanently
             * (POSIX semaphores are not released on process death).  After 3
             * consecutive timeouts assume the semaphore is stuck and replace the
             * generation so consumers detect the new inode and reconnect. */
            if (++ctx->publish_timeouts >= 3) {
                fprintf(stderr,
                        "ebpf-go.plugin: pid shm: semaphore wedged (%u consecutive timeouts), replacing generation\n",
                        ctx->publish_timeouts);
                if (pid_shm_replace_generation(ctx, shared_pid_memory_nbytes(ctx)))
                    ctx->publish_timeouts = 0;
                else
                    fprintf(stderr, "ebpf-go.plugin: pid shm: replace_generation failed; pid publishing suspended\n");
            }
            return -1;
        }
        ctx->publish_timeouts = 0;
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

    /* Unlink only when this context created (or recreated) the SHM name.  A
     * takeover of a pre-existing crashed segment (shm_name_created=false) must
     * NOT unlink: the creator may have restarted and called replace_generation,
     * and unlinking then would remove the new-generation name while the creator
     * is still live.  The sem name is intentionally NOT unlinked: consumers and
     * the next writer must share the same underlying kernel object. */
    if (ctx->shm_name_created)
        (void)shm_unlink(NETDATA_EBPFGO_INTEGRATION_NAME);

    free(ctx);
}
