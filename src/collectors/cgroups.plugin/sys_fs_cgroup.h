// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYS_FS_CGROUP_H
#define NETDATA_SYS_FS_CGROUP_H 1

#include "database/rrd.h"

#define PLUGIN_CGROUPS_NAME "cgroups.plugin"
#define PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME "systemd"
#define PLUGIN_CGROUPS_MODULE_CGROUPS_NAME "/sys/fs/cgroup"

#define CGROUP_OPTIONS_DISABLED_DUPLICATE   (1 << 0)
#define CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE (1 << 1)
#define CGROUP_OPTIONS_IS_UNIFIED           (1 << 2)
#define CGROUP_OPTIONS_DISABLED_EXCLUDED    (1 << 3)

typedef struct netdata_ebpf_cgroup_shm_header {
    int cgroup_root_count;
    int cgroup_max;
    int systemd_enabled;
    int __pad;
    size_t body_length;
} netdata_ebpf_cgroup_shm_header_t;

#define CGROUP_EBPF_NAME_SHARED_LENGTH 256

typedef struct netdata_ebpf_cgroup_shm_body {
    // Considering what is exposed in this link https://en.wikipedia.org/wiki/Comparison_of_file_systems#Limits
    // this length is enough to store what we want.
    char name[CGROUP_EBPF_NAME_SHARED_LENGTH];
    uint32_t hash;
    uint32_t options;
    int enabled;
    char path[FILENAME_MAX + 1];
} netdata_ebpf_cgroup_shm_body_t;

typedef struct netdata_ebpf_cgroup_shm {
    netdata_ebpf_cgroup_shm_header_t *header;
    netdata_ebpf_cgroup_shm_body_t *body;
} netdata_ebpf_cgroup_shm_t;

#define NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME "netdata_shm_cgroup_ebpf"
#define NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME "/netdata_sem_cgroup_ebpf"

#include "../proc.plugin/plugin_proc.h"

#endif //NETDATA_SYS_FS_CGROUP_H
