// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_dns_memory.h"

/* ebpfgo_shm_sem_wait and ebpfgo_shm_now_monotonic_usec are defined in
 * apps_ebpf_shared_pid_row.h, included transitively via apps_ebpf_shared_dns_row.h */
#include "apps_ebpf_shared_pid_row.h"

#include <fcntl.h>
#include <semaphore.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

struct shared_dns_memory {
    struct ebpfgo_dns_shared *data;
    uint32_t update_every_s;
    int shm_fd;
    sem_t *sem;
    bool shm_owned;        /* set on successful open; triggers unlink for the reused-takeover path */
    bool shm_name_created; /* set when this context created/recreated the SHM name; triggers unlink on any exit */
};

static void shared_dns_memory_invalidate(struct shared_dns_memory *ctx)
{
    if (!ctx || !ctx->data)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (ebpfgo_shm_sem_wait(ctx->sem))
            locked = true;
    }

    __atomic_store_n(&ctx->data->hdr.live_count,      0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.last_publish_ut, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

/* See pid_shm_replace_generation for the full rationale.  Same generation-
 * replacement approach applied to the DNS SHM and semaphore objects. */
static bool dns_shm_replace_generation(struct shared_dns_memory *ctx, size_t length)
{
    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }
    if (ctx->data) {
        munmap(ctx->data, length);
        ctx->data = NULL;
    }
    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }
    (void)sem_unlink(NETDATA_EBPFGO_DNS_SEM_NAME);
    (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);

    ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        return false;
    /* Mark created before any further steps; close() unlinks on any failure path. */
    ctx->shm_name_created = true;

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        return false;

    ctx->data = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->data == MAP_FAILED) {
        ctx->data = NULL;
        return false;
    }

    /* New SHM is kernel-zero-filled; no explicit memset needed. */
    ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT, 0660, 1);
    return ctx->sem != SEM_FAILED;
}

struct shared_dns_memory *shared_dns_memory_open(uint32_t update_every_s)
{
    struct shared_dns_memory *ctx = calloc(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;
    ctx->update_every_s = update_every_s;

    ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        goto fail;

    struct stat pre_stat;
    if (fstat(ctx->shm_fd, &pre_stat) != 0)
        goto fail;
    bool reused = pre_stat.st_size > 0;
    if (!reused)
        ctx->shm_name_created = true;
    size_t length = sizeof(struct ebpfgo_dns_shared);

    /* Same size-mismatch guard as shared_pid_memory_linux.c: unlink and
     * re-create when a crashed writer left a segment of a different size. */
    if (reused && pre_stat.st_size != (off_t)length) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);
        ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_RDWR, 0660);
        if (ctx->shm_fd < 0)
            goto fail;
        reused = false;
        ctx->shm_name_created = true;
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->data = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->data == MAP_FAILED) {
        ctx->data = NULL;
        goto fail;
    }

    /* Do NOT sem_unlink here — same reason as shared_pid_memory_linux.c:
     * unlinking creates a new semaphore object, leaving consumers with a
     * stale handle and no mutual exclusion.  O_CREAT opens the existing
     * semaphore when it already exists, keeping all parties on the same
     * underlying object. */
    ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED)
        goto fail;

    if (reused) {
        if (ebpfgo_shm_sem_wait(ctx->sem)) {
            memset(ctx->data, 0, length);
            sem_post(ctx->sem);
        } else {
            /* Timed out: see pid_shm_replace_generation for the rationale.
             * Replace both SHM and semaphore instead of posting without
             * ownership. */
            if (!dns_shm_replace_generation(ctx, length))
                goto fail;
        }
    }

    ctx->shm_owned = true;
    return ctx;

fail:
    shared_dns_memory_close(ctx);
    return NULL;
}

void shared_dns_memory_publish(
    struct shared_dns_memory *ctx,
    const struct ebpfgo_dns_aggregate *agg,
    const struct ebpfgo_dns_flow_record *flows,
    uint32_t flow_count)
{
    if (!ctx || !ctx->data)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem))
            return;
        locked = true;
    }

    if (agg)
        memcpy(&ctx->data->agg, agg, sizeof(ctx->data->agg));

    uint32_t n = flow_count < NETDATA_EBPFGO_DNS_FLOW_RING_CAP
                 ? flow_count : NETDATA_EBPFGO_DNS_FLOW_RING_CAP;
    if (flows && n > 0)
        memcpy(ctx->data->ring, flows, n * sizeof(struct ebpfgo_dns_flow_record));

    /* Write update_every_s and live_count before last_publish_ut so a reader
     * that sees a fresh last_publish_ut also sees consistent metadata. */
    __atomic_store_n(&ctx->data->hdr.update_every_s, ctx->update_every_s, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.live_count,      n,                  __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.last_publish_ut, ebpfgo_shm_now_monotonic_usec(), __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

void shared_dns_memory_close(struct shared_dns_memory *ctx)
{
    if (!ctx)
        return;

    shared_dns_memory_invalidate(ctx);

    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }

    if (ctx->data) {
        munmap(ctx->data, sizeof(struct ebpfgo_dns_shared));
        ctx->data = NULL;
    }

    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }

    /* Unlink the SHM name — see shared_pid_memory_linux.c for the full rationale.
     * Unlink when shm_name_created (we created the name — clean up on any exit) or
     * shm_owned (took over existing — clean up on graceful close).  Both false means
     * a failed open on a pre-existing segment — must not unlink a live publisher's name.
     * sem name is intentionally NOT unlinked here. */
    if (ctx->shm_name_created || ctx->shm_owned)
        (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);

    free(ctx);
}
