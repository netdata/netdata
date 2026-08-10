// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_DNS_SHARED_MEMORY_H
#define NETDATA_EBPFGO_DNS_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include <semaphore.h>
#include <sys/stat.h>

#include "../ebpf.plugin/ebpfgo.plugin/apps_ebpf_shared_dns_row.h"

typedef struct netdata_ebpfgo_dns_shared_memory {
    struct ebpfgo_dns_shared *shm;  /* mmap pointer (read-only, kept alive) */
    struct ebpfgo_dns_shared data;  /* local copy, updated under semaphore  */
    bool has_data;
    int shm_fd;
    sem_t *sem;
    dev_t shm_dev;
    ino_t shm_ino;
} netdata_ebpfgo_dns_shared_memory_t;

bool netdata_ebpfgo_dns_shared_memory_refresh(
    netdata_ebpfgo_dns_shared_memory_t *ctx,
    const char *shm_name,
    const char *sem_name);

void netdata_ebpfgo_dns_shared_memory_close(netdata_ebpfgo_dns_shared_memory_t *ctx);

#endif /* OS_LINUX */

#endif /* NETDATA_EBPFGO_DNS_SHARED_MEMORY_H */
