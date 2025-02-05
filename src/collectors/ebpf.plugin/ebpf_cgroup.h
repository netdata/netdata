// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_CGROUP_H
#define NETDATA_EBPF_CGROUP_H 1

#define NETDATA_EBPF_CGROUP_MAX_TRIES 3
#define NETDATA_EBPF_CGROUP_NEXT_TRY_SEC 30

#include "ebpf.h"
#include "ebpf_apps.h"

struct pid_on_target2 {
    int32_t pid;
    int updated;

    netdata_publish_swap_t swap;
    netdata_publish_fd_stat_t fd;
    netdata_publish_vfs_t vfs;
    ebpf_publish_process_t ps;
    netdata_dcstat_pid_t dc;
    netdata_publish_shm_t shm;
    netdata_socket_t socket;
    netdata_publish_cachestat_t cachestat;

    struct pid_on_target2 *next;
};

enum ebpf_cgroup_flags {
    NETDATA_EBPF_CGROUP_HAS_PROCESS_CHART = 1,
    NETDATA_EBPF_CGROUP_HAS_SWAP_CHART = 1 << 2,
    NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART = 1 << 3,
    NETDATA_EBPF_CGROUP_HAS_FD_CHART = 1 << 4,
    NETDATA_EBPF_CGROUP_HAS_VFS_CHART = 1 << 5,
    NETDATA_EBPF_CGROUP_HAS_OOMKILL_CHART = 1 << 6,
    NETDATA_EBPF_CGROUP_HAS_CACHESTAT_CHART = 1 << 7,
    NETDATA_EBPF_CGROUP_HAS_DC_CHART = 1 << 8,
    NETDATA_EBPF_CGROUP_HAS_SHM_CHART = 1 << 9,

    NETDATA_EBPF_SERVICES_HAS_PROCESS_CHART = 1 << 16,
    NETDATA_EBPF_SERVICES_HAS_SWAP_CHART = 1 << 17,
    NETDATA_EBPF_SERVICES_HAS_SOCKET_CHART = 1 << 18,
    NETDATA_EBPF_SERVICES_HAS_FD_CHART = 1 << 19,
    NETDATA_EBPF_SERVICES_HAS_VFS_CHART = 1 << 20,
    NETDATA_EBPF_SERVICES_HAS_OOMKILL_CHART = 1 << 21,
    NETDATA_EBPF_SERVICES_HAS_CACHESTAT_CHART = 1 << 22,
    NETDATA_EBPF_SERVICES_HAS_DC_CHART = 1 << 23,
    NETDATA_EBPF_SERVICES_HAS_SHM_CHART = 1 << 24
};

typedef struct ebpf_cgroup_target {
    char name[256]; // title
    uint32_t hash;
    uint32_t flags;
    uint32_t systemd;
    uint32_t updated;

    netdata_publish_swap_t publish_systemd_swap;
    netdata_publish_fd_stat_t publish_systemd_fd;
    netdata_publish_vfs_t publish_systemd_vfs;
    ebpf_publish_process_t publish_systemd_ps;
    netdata_publish_dcstat_t publish_dc;
    int oomkill;
    netdata_publish_shm_t publish_shm;
    ebpf_socket_publish_apps_t publish_socket;
    netdata_publish_cachestat_t publish_cachestat;

    struct pid_on_target2 *pids;
    struct ebpf_cgroup_target *next;
} ebpf_cgroup_target_t;

typedef struct ebpf_systemd_args {
    char *id;
    char *title;
    char *units;
    char *family;
    char *charttype;
    int order;
    char *algorithm;
    char *context;
    char *module;
    int update_every;
    char *suffix;
    char *dimension;
} ebpf_systemd_args_t;

void ebpf_map_cgroup_shared_memory();
void ebpf_parse_cgroup_shm_data();
void ebpf_create_charts_on_systemd(ebpf_systemd_args_t *chart);
void *ebpf_cgroup_integration(void *ptr);
void ebpf_unmap_cgroup_shared_memory();
extern int send_cgroup_chart;

#endif /* NETDATA_EBPF_CGROUP_H */
