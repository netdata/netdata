// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_PROCESS_H
#define NETDATA_EBPF_PROCESS_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_PROCESS "process"
#define NETDATA_EBPF_MODULE_PROCESS_DESC                                                                               \
    "Monitor information about process life. This thread is integrated with apps and cgroup."

// Groups used on Dashboard
#define NETDATA_PROCESS_GROUP "processes"

// Global chart name
#define NETDATA_EXIT_SYSCALL "exit"
#define NETDATA_PROCESS_SYSCALL "process_thread"
#define NETDATA_PROCESS_ERROR_NAME "task_error"
#define NETDATA_PROCESS_STATUS_NAME "process_status"

// Charts created on Apps submenu
#define NETDATA_SYSCALL_APPS_TASK_PROCESS "process_create"
#define NETDATA_SYSCALL_APPS_TASK_THREAD "thread_create"
#define NETDATA_SYSCALL_APPS_TASK_EXIT "task_exit"
#define NETDATA_SYSCALL_APPS_TASK_CLOSE "task_close"
#define NETDATA_SYSCALL_APPS_TASK_ERROR "task_error"

// Process configuration name
#define NETDATA_PROCESS_CONFIG_FILE "process.conf"

// Contexts
#define NETDATA_CGROUP_PROCESS_CREATE_CONTEXT "cgroup.process_create"
#define NETDATA_CGROUP_THREAD_CREATE_CONTEXT "cgroup.thread_create"
#define NETDATA_CGROUP_PROCESS_CLOSE_CONTEXT "cgroup.task_close"
#define NETDATA_CGROUP_PROCESS_EXIT_CONTEXT "cgroup.task_exit"
#define NETDATA_CGROUP_PROCESS_ERROR_CONTEXT "cgroup.task_error"

#define NETDATA_SYSTEMD_PROCESS_CREATE_CONTEXT "systemd.service.process_create"
#define NETDATA_SYSTEMD_THREAD_CREATE_CONTEXT "systemd.service.thread_create"
#define NETDATA_SYSTEMD_PROCESS_CLOSE_CONTEXT "systemd.service.task_close"
#define NETDATA_SYSTEMD_PROCESS_EXIT_CONTEXT "systemd.service.task_exit"
#define NETDATA_SYSTEMD_PROCESS_ERROR_CONTEXT "systemd.service.task_error"

#define NETDATA_EBPF_CGROUP_UPDATE 30

enum netdata_ebpf_stats_order {
    NETDATA_EBPF_ORDER_STAT_THREADS = 140000,
    NETDATA_EBPF_ORDER_PIDS,
    NETDATA_EBPF_ORDER_PIDS_IPC,
    NETDATA_EBPF_ORDER_STAT_LIFE_TIME,
    NETDATA_EBPF_ORDER_STAT_LOAD_METHOD,
    NETDATA_EBPF_ORDER_STAT_KERNEL_MEMORY,
    NETDATA_EBPF_ORDER_STAT_HASH_TABLES,
    NETDATA_EBPF_ORDER_STAT_HASH_CORE,
    NETDATA_EBPF_ORDER_STAT_HASH_GLOBAL_TABLE_TOTAL,
    NETDATA_EBPF_ORDER_STAT_HASH_PID_TABLE_ADDED,
    NETDATA_EBPF_ORDER_STAT_HASH_PID_TABLE_REMOVED,
    NETATA_EBPF_ORDER_STAT_ARAL_BEGIN,
    NETDATA_EBPF_ORDER_FUNCTION_PER_THREAD,
};

enum netdata_ebpf_load_mode_stats {
    NETDATA_EBPF_LOAD_STAT_LEGACY,
    NETDATA_EBPF_LOAD_STAT_CORE,

    NETDATA_EBPF_LOAD_STAT_END
};

enum netdata_ebpf_thread_per_core {
    NETDATA_EBPF_THREAD_PER_CORE,
    NETDATA_EBPF_THREAD_UNIQUE,

    NETDATA_EBPF_PER_CORE_END
};

// Index from kernel
typedef enum ebpf_process_index {
    NETDATA_KEY_CALLS_DO_EXIT,

    NETDATA_KEY_CALLS_RELEASE_TASK,

    NETDATA_KEY_CALLS_DO_FORK,
    NETDATA_KEY_ERROR_DO_FORK,

    NETDATA_KEY_CALLS_SYS_CLONE,
    NETDATA_KEY_ERROR_SYS_CLONE,

    NETDATA_KEY_END_VECTOR
} ebpf_process_index_t;

// This enum acts as an index for publish vector.
// Do not change the enum order because we use
// different algorithms to make charts with incremental
// values (the three initial positions) and absolute values
// (the remaining charts).
typedef enum netdata_publish_process {
    NETDATA_KEY_PUBLISH_PROCESS_EXIT,
    NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK,
    NETDATA_KEY_PUBLISH_PROCESS_FORK,
    NETDATA_KEY_PUBLISH_PROCESS_CLONE,

    NETDATA_KEY_PUBLISH_PROCESS_END
} netdata_publish_process_t;

enum ebpf_process_tables { NETDATA_PROCESS_PID_TABLE, NETDATA_PROCESS_GLOBAL_TABLE, NETDATA_PROCESS_CTRL_TABLE };

extern struct config process_config;

#endif /* NETDATA_EBPF_PROCESS_H */
