// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DISK_H
#define NETDATA_EBPF_DISK_H 1

#include "libnetdata/avl/avl.h"
#include "libnetdata/ebpf/ebpf.h"

extern struct config disk_config;

extern void *ebpf_disk_thread(void *ptr);

#endif /* NETDATA_EBPF_DISK_H */