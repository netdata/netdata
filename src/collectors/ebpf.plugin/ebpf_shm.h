// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SHM_H
#define NETDATA_EBPF_SHM_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_SHM "shm"
#define NETDATA_EBPF_SHM_MODULE_DESC                                                                                   \
    "Show calls to syscalls shmget(2), shmat(2), shmdt(2) and shmctl(2). This thread is integrated with apps and cgroup."

// charts
#define NETDATA_SHM_GLOBAL_CHART "shared_memory_calls"
#define NETDATA_SHMGET_CHART "shmget_call"
#define NETDATA_SHMAT_CHART "shmat_call"
#define NETDATA_SHMDT_CHART "shmdt_call"
#define NETDATA_SHMCTL_CHART "shmctl_call"

// configuration file
#define NETDATA_DIRECTORY_SHM_CONFIG_FILE "shm.conf"

// Contexts
#define NETDATA_CGROUP_SHM_GET_CONTEXT "cgroup.shmget"
#define NETDATA_CGROUP_SHM_AT_CONTEXT "cgroup.shmat"
#define NETDATA_CGROUP_SHM_DT_CONTEXT "cgroup.shmdt"
#define NETDATA_CGROUP_SHM_CTL_CONTEXT "cgroup.shmctl"

#define NETDATA_SYSTEMD_SHM_GET_CONTEXT "systemd.service.shmget"
#define NETDATA_SYSTEMD_SHM_AT_CONTEXT "systemd.service.shmat"
#define NETDATA_SYSTEMD_SHM_DT_CONTEXT "systemd.service.shmdt"
#define NETDATA_SYSTEMD_SHM_CTL_CONTEXT "systemd.service.shmctl"

typedef struct __attribute__((packed)) netdata_publish_shm {
    uint64_t ct;

    uint32_t get;
    uint32_t at;
    uint32_t dt;
    uint32_t ctl;
} netdata_publish_shm_t;

enum shm_tables { NETDATA_PID_SHM_TABLE, NETDATA_SHM_CONTROLLER, NETDATA_SHM_GLOBAL_TABLE };

enum shm_counters {
    NETDATA_KEY_SHMGET_CALL,
    NETDATA_KEY_SHMAT_CALL,
    NETDATA_KEY_SHMDT_CALL,
    NETDATA_KEY_SHMCTL_CALL,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_SHM_END
};

void *ebpf_shm_thread(void *ptr);
void ebpf_shm_create_apps_charts(struct ebpf_module *em, void *ptr);
void ebpf_shm_release(netdata_publish_shm_t *stat);
extern netdata_ebpf_targets_t shm_targets[];

extern struct config shm_config;

#endif
