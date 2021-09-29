// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_CGROUP_H
#define NETDATA_EBPF_CGROUP_H 1

#define NETDATA_EBPF_CGROUP_MAX_TRIES 3
#define NETDATA_EBPF_CGROUP_NEXT_TRY_SEC 30

#include "ebpf.h"
#include "ebpf_apps.h"

#define NETDATA_SERVICE_FAMILY "services"

struct pid_on_target2 {
    int32_t pid;
    int updated;

    netdata_publish_swap_t swap;

    struct pid_on_target2 *next;
};

enum ebpf_cgroup_flags {
    NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART = 1,
    NETDATA_EBPF_CGROUP_HAS_SWAP_CHART = 1<<2,
    NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART = 1<<3,
    NETDATA_EBPF_CGROUP_HAS_FD_CHART = 1<<4,
    NETDATA_EBPF_CGROUP_HAS_VFS_CHART = 1<<5,
    NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART = 1<<6,
    NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART = 1<<7,
    NETDATA_EBPF_CGROUP_HAS_DC_CHART = 1<<8,
    NETDATA_EBPF_CGROUP_HAS_SHM_CHART = 1<<9
};

typedef struct ebpf_cgroup_target {
    char name[256]; // title
    uint32_t hash;
    uint32_t flags;
    uint32_t systemd;
    uint32_t updated;

    netdata_publish_swap_t publish_systemd_swap;

    struct pid_on_target2 *pids;
    struct ebpf_cgroup_target *next;
} ebpf_cgroup_target_t;

extern void ebpf_map_cgroup_shared_memory();
extern void ebpf_parse_cgroup_shm_data();
extern void ebpf_close_cgroup_shm();
extern void ebpf_clean_cgroup_pids();
extern void ebpf_create_charts_on_systemd(char *id, char *title, char *units, char *family, char *charttype, int order,
                                          char *algorithm, char *module);

#endif /* NETDATA_EBPF_CGROUP_H */
