// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SHM_H
#define NETDATA_EBPF_SHM_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_SHM "shm"

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

#define NETDATA_SYSTEMD_SHM_GET_CONTEXT "services.shmget"
#define NETDATA_SYSTEMD_SHM_AT_CONTEXT "services.shmat"
#define NETDATA_SYSTEMD_SHM_DT_CONTEXT "services.shmdt"
#define NETDATA_SYSTEMD_SHM_CTL_CONTEXT "services.shmctl"

// ARAL name
#define NETDATA_EBPF_SHM_ARAL_NAME "ebpf_shm"

typedef struct netdata_publish_shm {
    uint64_t get;
    uint64_t at;
    uint64_t dt;
    uint64_t ctl;
} netdata_publish_shm_t;

enum shm_tables {
    NETDATA_PID_SHM_TABLE,
    NETDATA_SHM_CONTROLLER,
    NETDATA_SHM_GLOBAL_TABLE
};

enum shm_counters {
    NETDATA_KEY_SHMGET_CALL,
    NETDATA_KEY_SHMAT_CALL,
    NETDATA_KEY_SHMDT_CALL,
    NETDATA_KEY_SHMCTL_CALL,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_SHM_END
};

extern netdata_publish_shm_t **shm_pid;

void *ebpf_shm_thread(void *ptr);
void ebpf_shm_create_apps_charts(struct ebpf_module *em, void *ptr);
void ebpf_shm_release(netdata_publish_shm_t *stat);
extern netdata_ebpf_targets_t shm_targets[];

extern struct config shm_config;

#endif
