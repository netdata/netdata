// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_PROCESS_H
#define NETDATA_EBPF_PROCESS_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_PROCESS "process"

// Groups used on Dashboard
#define NETDATA_PROCESS_GROUP "processes"
#define NETDATA_PROCESS_CGROUP_GROUP "processes (eBPF)"

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

#define NETDATA_SYSTEMD_PROCESS_CREATE_CONTEXT "services.process_create"
#define NETDATA_SYSTEMD_THREAD_CREATE_CONTEXT "services.thread_create"
#define NETDATA_SYSTEMD_PROCESS_CLOSE_CONTEXT "services.task_close"
#define NETDATA_SYSTEMD_PROCESS_EXIT_CONTEXT "services.task_exit"
#define NETDATA_SYSTEMD_PROCESS_ERROR_CONTEXT "services.task_error"

// Statistical information
enum netdata_ebpf_thread_stats{
    NETDATA_EBPF_THREAD_STAT_TOTAL,
    NETDATA_EBPF_THREAD_STAT_RUNNING,

    NETDATA_EBPF_THREAD_STAT_END
};

enum netdata_ebpf_load_mode_stats{
    NETDATA_EBPF_LOAD_STAT_LEGACY,
    NETDATA_EBPF_LOAD_STAT_CORE,

    NETDATA_EBPF_LOAD_STAT_END
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

typedef struct ebpf_process_publish_apps {
    // Number of calls during the last read
    uint64_t call_do_exit;
    uint64_t call_release_task;
    uint64_t create_process;
    uint64_t create_thread;

    // Number of errors during the last read
    uint64_t task_err;
} ebpf_process_publish_apps_t;

enum ebpf_process_tables {
    NETDATA_PROCESS_PID_TABLE,
    NETDATA_PROCESS_GLOBAL_TABLE,
    NETDATA_PROCESS_CTRL_TABLE
};

extern struct config process_config;

#endif /* NETDATA_EBPF_PROCESS_H */
