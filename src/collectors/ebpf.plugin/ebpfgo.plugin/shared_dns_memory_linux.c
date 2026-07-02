// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_dns_memory.h"

/* ebpfgo_shm_sem_wait is defined in apps_ebpf_shared_pid_row.h */
#include "apps_ebpf_shared_pid_row.h"

#include <fcntl.h>
#include <semaphore.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

struct shared_dns_memory {
    struct ebpfgo_dns_shared *data;
    int shm_fd;
    sem_t *sem;
};

static uint64_t shared_dns_memory_now_monotonic_usec(void)
{
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0)
        return 0;

    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)ts.tv_nsec / 1000ULL;
}

static void shared_dns_memory_invalidate(struct shared_dns_memory *ctx)
{
    if (!ctx || !ctx->data)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (ebpfgo_shm_sem_wait(ctx->sem))
            locked = true;
    }

    __atomic_store_n(&ctx->data->ring_count, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->last_publish_ut, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

struct shared_dns_memory *shared_dns_memory_open(void)
{
    struct shared_dns_memory *ctx = calloc(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;

    ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        goto fail;

    struct stat pre_stat;
    if (fstat(ctx->shm_fd, &pre_stat) != 0)
        goto fail;
    bool reused = pre_stat.st_size > 0;

    if (ftruncate(ctx->shm_fd, (off_t)sizeof(struct ebpfgo_dns_shared)) != 0)
        goto fail;

    ctx->data = mmap(NULL, sizeof(struct ebpfgo_dns_shared),
                     PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->data == MAP_FAILED) {
        ctx->data = NULL;
        goto fail;
    }

    if (reused)
        memset(ctx->data, 0, sizeof(struct ebpfgo_dns_shared));

    ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED)
        goto fail;

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

    ctx->data->ring_count = n;
    __atomic_store_n(&ctx->data->last_publish_ut, shared_dns_memory_now_monotonic_usec(), __ATOMIC_RELEASE);

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

    free(ctx);
}
