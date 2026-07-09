// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_SHARED_MEMORY_H
#define NETDATA_EBPFGO_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include <semaphore.h>
#include <sys/stat.h>

#include "../ebpf.plugin/ebpfgo.plugin/apps_ebpf_shared_pid_row.h"

typedef struct netdata_ebpfgo_shared_pid_memory {
    void *mapping;                  /* raw mmap base (for munmap and header read) */
    struct ebpf_pid_stat *shm;      /* = (char*)mapping + sizeof(ebpfgo_shm_header) */
    struct ebpf_pid_stat *snapshot;
    uint32_t shm_flags;             /* EBPFGO_SHM_FLAG_* bits from last refresh */
    uint32_t update_every_s;        /* publish interval from SHM header; 0 = unknown */
    uint64_t last_publish_ut;       /* monotonic usec of the payload in snapshot */
    size_t shm_total;
    size_t snapshot_total;
    int shm_fd;
    sem_t *sem;
    dev_t shm_dev;
    ino_t shm_ino;
} netdata_ebpfgo_shared_pid_memory_t;

bool netdata_ebpfgo_shared_pid_memory_refresh(
    netdata_ebpfgo_shared_pid_memory_t *ctx,
    const char *shm_name,
    const char *sem_name);

const struct ebpf_pid_stat *netdata_ebpfgo_shared_pid_memory_lookup(
    const netdata_ebpfgo_shared_pid_memory_t *ctx,
    pid_t pid);

uint32_t netdata_ebpfgo_shared_pid_memory_flags(const netdata_ebpfgo_shared_pid_memory_t *ctx);

void netdata_ebpfgo_shared_pid_memory_close(netdata_ebpfgo_shared_pid_memory_t *ctx);

#endif

#endif
