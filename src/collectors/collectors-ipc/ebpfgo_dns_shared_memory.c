// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpfgo_dns_shared_memory.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include "../ebpf.plugin/ebpfgo.plugin/apps_ebpf_shared_pid_row.h"
#include <sys/mman.h>
#include <unistd.h>

#define EBPFGO_DNS_SHM_STALE_TIMEOUT_UT (10ULL * USEC_PER_SEC)

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
    ctx->last_publish_ut = 0;
}

static bool netdata_ebpfgo_dns_payload_is_live(uint64_t last_publish_ut, usec_t now_ut)
{
    return last_publish_ut != 0 &&
           now_ut >= last_publish_ut &&
           (now_ut - last_publish_ut) <= EBPFGO_DNS_SHM_STALE_TIMEOUT_UT;
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
        /* SEM_FAILED is non-fatal: proceed without the semaphore and rely on
         * the kernel's mmap visibility for reader/writer ordering. */
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
        bool changed = (fstat(fd, &st) != 0 ||
                        st.st_dev != ctx->shm_dev ||
                        st.st_ino != ctx->shm_ino);
        close(fd);

        if (changed)
            netdata_ebpfgo_dns_shm_close_internal(ctx);
    }

    if (!ctx->shm && !netdata_ebpfgo_dns_shm_open(ctx, shm_name, sem_name))
        return false;

    if (ctx->sem == SEM_FAILED && sem_name) {
        ctx->sem = sem_open(sem_name, 0);
        /* SEM_FAILED is non-fatal: subsequent calls will treat the SHM as
         * lock-free and rely on mmap visibility for cross-process ordering. */
    }

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            if (!netdata_ebpfgo_dns_payload_is_live(ctx->last_publish_ut, ebpfgo_shm_now_monotonic_usec())) {
                netdata_ebpfgo_dns_shm_close_internal(ctx);
                ctx->has_data = false;
                return false;
            }
            return ctx->has_data;
        }
        locked = true;
    }

    memcpy(&ctx->data, ctx->shm, sizeof(struct ebpfgo_dns_shared));
    ctx->last_publish_ut = ctx->data.last_publish_ut;

    if (locked)
        sem_post(ctx->sem);

    if (!netdata_ebpfgo_dns_payload_is_live(ctx->last_publish_ut, ebpfgo_shm_now_monotonic_usec())) {
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
