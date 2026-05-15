// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_pid_memory.h"

#include <fcntl.h>
#include <semaphore.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

struct shared_pid_memory {
    struct ebpf_pid_stat *entries;
    size_t total;
    int shm_fd;
    sem_t *sem;
};

struct shared_pid_memory *shared_pid_memory_open(size_t total)
{
    if (!total)
        return NULL;

    struct shared_pid_memory *ctx = calloc(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;

    ctx->shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (ctx->shm_fd < 0)
        goto fail;

    ctx->total = total;
    size_t length = total * sizeof(struct ebpf_pid_stat);
    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->entries = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->entries == MAP_FAILED) {
        ctx->entries = NULL;
        goto fail;
    }

    ctx->sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, O_CREAT, 0660, 1);
    if (ctx->sem == SEM_FAILED)
        ctx->sem = SEM_FAILED;

    memset(ctx->entries, 0, length);
    return ctx;

fail:
    shared_pid_memory_close(ctx);
    return NULL;
}

int shared_pid_memory_publish(struct shared_pid_memory *ctx, const struct ebpf_pid_stat *entries, size_t count)
{
    if (!ctx || !ctx->entries)
        return -1;

    if (count > ctx->total)
        count = ctx->total;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (sem_trywait(ctx->sem) != 0)
            return 0;
        locked = true;
    }

    size_t length = ctx->total * sizeof(struct ebpf_pid_stat);
    memset(ctx->entries, 0, length);
    if (entries && count)
        memcpy(ctx->entries, entries, count * sizeof(struct ebpf_pid_stat));

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

    if (ctx->entries)
        munmap(ctx->entries, ctx->total * sizeof(struct ebpf_pid_stat));

    if (ctx->shm_fd >= 0)
        close(ctx->shm_fd);

    (void)shm_unlink(NETDATA_EBPFGO_INTEGRATION_NAME);
    (void)sem_unlink(NETDATA_EBPFGO_SHM_INTEGRATION_NAME);

    free(ctx);
}
