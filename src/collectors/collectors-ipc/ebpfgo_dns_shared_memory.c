// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpfgo_dns_shared_memory.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include "ebpfgo_shm_liveness.h"
#include <sys/mman.h>
#include <unistd.h>

static void netdata_ebpfgo_dns_shm_close_internal(netdata_ebpfgo_dns_shared_memory_t *ctx)
{
    if (!ctx)
        return;

    if (ctx->sem && ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }

    if (ctx->shm) {
        nd_munmap(ctx->shm, sizeof(struct ebpfgo_dns_shared));
        ctx->shm = NULL;
    }

    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }

    ctx->shm_dev = 0;
    ctx->shm_ino = 0;

    /* Zero liveness fields so a stale check after the next successful open
     * does not pass against a timestamp from a prior session. */
    ctx->data.hdr.last_publish_ut = 0;
    ctx->data.hdr.update_every_s  = 0;
}

static bool netdata_ebpfgo_dns_payload_is_live(const netdata_ebpfgo_dns_shared_memory_t *ctx, usec_t now_ut)
{
    uint64_t ts = ctx->data.hdr.last_publish_ut;
    uint32_t ue = ctx->data.hdr.update_every_s;

    return ts != 0 && now_ut >= ts &&
           (now_ut - ts) <= ebpfgo_shm_stale_timeout_ut(ue);
}

static bool netdata_ebpfgo_dns_shm_open(
    netdata_ebpfgo_dns_shared_memory_t *ctx,
    const char *shm_name,
    const char *sem_name)
{
    if (!ctx || !shm_name)
        return false;

    ctx->shm_fd = shm_open(shm_name, O_RDONLY, 0);
    if (ctx->shm_fd < 0)
        return false;

    struct stat st;
    if (fstat(ctx->shm_fd, &st) != 0 ||
        (size_t)st.st_size != sizeof(struct ebpfgo_dns_shared))
        goto fail;

    ctx->shm = nd_mmap(NULL, sizeof(struct ebpfgo_dns_shared),
                       PROT_READ, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->shm == MAP_FAILED) {
        ctx->shm = NULL;
        goto fail;
    }

    ctx->shm_dev = st.st_dev;
    ctx->shm_ino = st.st_ino;

    if (sem_name) {
        ctx->sem = sem_open(sem_name, 0);
        if (ctx->sem == SEM_FAILED)
            goto fail;
    }

    return true;

fail:
    netdata_ebpfgo_dns_shm_close_internal(ctx);
    return false;
}

bool netdata_ebpfgo_dns_shared_memory_refresh(
    netdata_ebpfgo_dns_shared_memory_t *ctx,
    const char *shm_name,
    const char *sem_name)
{
    if (!ctx || !shm_name)
        return false;

    if (ctx->shm) {
        /* Detect writer restart via inode change. */
        int fd = shm_open(shm_name, O_RDONLY, 0);
        if (fd < 0) {
            netdata_ebpfgo_dns_shm_close_internal(ctx);
            return false;
        }

        struct stat st;
        if (fstat(fd, &st) != 0) {
            close(fd);
            netdata_ebpfgo_dns_shm_close_internal(ctx);
            return false;
        }

        bool changed = (st.st_dev != ctx->shm_dev || st.st_ino != ctx->shm_ino);
        close(fd);

        if (changed)
            netdata_ebpfgo_dns_shm_close_internal(ctx);
    }

    if (!ctx->shm && !netdata_ebpfgo_dns_shm_open(ctx, shm_name, sem_name))
        return false;

    if (ctx->sem == SEM_FAILED && sem_name) {
        ctx->sem = sem_open(sem_name, 0);
        if (ctx->sem == SEM_FAILED)
            return false;
    }

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            if (!netdata_ebpfgo_dns_payload_is_live(ctx, ebpfgo_shm_now_monotonic_usec())) {
                netdata_ebpfgo_dns_shm_close_internal(ctx);
                ctx->has_data = false;
                return false;
            }
            return ctx->has_data;
        }
        locked = true;
    }

    /* Partial copy: header + aggregate + only the live flow records.
     * Avoids memcpy-ing the full ~312 KB ring when flow_count is small. */
    uint32_t live = __atomic_load_n(&ctx->shm->hdr.live_count, __ATOMIC_ACQUIRE);
    if (live > NETDATA_EBPFGO_DNS_FLOW_RING_CAP)
        live = NETDATA_EBPFGO_DNS_FLOW_RING_CAP;

    memcpy(&ctx->data.hdr, &ctx->shm->hdr, sizeof(ctx->data.hdr));
    memcpy(&ctx->data.agg, &ctx->shm->agg, sizeof(ctx->data.agg));
    if (live)
        memcpy(ctx->data.ring, ctx->shm->ring, live * sizeof(ctx->data.ring[0]));
    ctx->data.hdr.live_count = live; /* re-apply cap in the local copy */

    if (locked)
        sem_post(ctx->sem);

    if (!netdata_ebpfgo_dns_payload_is_live(ctx, ebpfgo_shm_now_monotonic_usec())) {
        netdata_ebpfgo_dns_shm_close_internal(ctx);
        ctx->has_data = false;
        memset(&ctx->data, 0, sizeof(ctx->data));
        return false;
    }

    ctx->has_data = true;

    return true;
}

void netdata_ebpfgo_dns_shared_memory_close(netdata_ebpfgo_dns_shared_memory_t *ctx)
{
    netdata_ebpfgo_dns_shm_close_internal(ctx);
    ctx->has_data = false;
}

#endif /* OS_LINUX */
