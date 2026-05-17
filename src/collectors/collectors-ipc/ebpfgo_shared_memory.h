// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_SHARED_MEMORY_H
#define NETDATA_EBPFGO_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include <semaphore.h>
#include <sys/stat.h>

#include "../ebpf.plugin/ebpfgo.plugin/apps_ebpf_shared_pid_row.h"

typedef struct netdata_ebpfgo_shared_pid_memory {
    struct ebpf_pid_stat *shm;
    struct ebpf_pid_stat *snapshot;
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

void netdata_ebpfgo_shared_pid_memory_close(netdata_ebpfgo_shared_pid_memory_t *ctx);

#endif

#endif
