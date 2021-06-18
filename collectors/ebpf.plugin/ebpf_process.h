// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_PROCESS_H
#define NETDATA_EBPF_PROCESS_H 1

// Groups used on Dashboard
#define NETDATA_FILE_GROUP "File"
#define NETDATA_PROCESS_GROUP "Process"

// Global chart name
#define NETDATA_FILE_OPEN_CLOSE_COUNT "file_descriptor"
#define NETDATA_FILE_OPEN_ERR_COUNT "file_error"

#define NETDATA_EXIT_SYSCALL "exit"
#define NETDATA_PROCESS_SYSCALL "process_thread"
#define NETDATA_PROCESS_ERROR_NAME "task_error"
#define NETDATA_PROCESS_STATUS_NAME "process_status"

// Charts created on Apps submenu
#define NETDATA_SYSCALL_APPS_FILE_OPEN "file_open"
#define NETDATA_SYSCALL_APPS_FILE_CLOSED "file_closed"
#define NETDATA_SYSCALL_APPS_TASK_PROCESS "process_create"
#define NETDATA_SYSCALL_APPS_TASK_THREAD "thread_create"
#define NETDATA_SYSCALL_APPS_TASK_CLOSE "task_close"

// Charts created on Apps submenu, if and only if, the return mode is active

#define NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR "file_open_error"
#define NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR "file_close_error"

// Process configuration name
#define NETDATA_PROCESS_CONFIG_FILE "process.conf"

// Index from kernel
typedef enum ebpf_process_index {
    NETDATA_KEY_CALLS_DO_SYS_OPEN,
    NETDATA_KEY_ERROR_DO_SYS_OPEN,

    NETDATA_KEY_CALLS_DO_EXIT,

    NETDATA_KEY_CALLS_RELEASE_TASK,

    NETDATA_KEY_CALLS_DO_FORK,
    NETDATA_KEY_ERROR_DO_FORK,

    NETDATA_KEY_CALLS_CLOSE_FD,
    NETDATA_KEY_ERROR_CLOSE_FD,

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
    NETDATA_KEY_PUBLISH_PROCESS_OPEN,
    NETDATA_KEY_PUBLISH_PROCESS_CLOSE,
    NETDATA_KEY_PUBLISH_PROCESS_EXIT,
    NETDATA_KEY_PUBLISH_PROCESS_RELEASE_TASK,
    NETDATA_KEY_PUBLISH_PROCESS_FORK,
    NETDATA_KEY_PUBLISH_PROCESS_CLONE,

    NETDATA_KEY_PUBLISH_PROCESS_END
} netdata_publish_process_t;

typedef struct ebpf_process_publish_apps {
    // Number of calls during the last read
    uint64_t call_sys_open;
    uint64_t call_close_fd;
    uint64_t call_do_exit;
    uint64_t call_release_task;
    uint64_t call_do_fork;
    uint64_t call_sys_clone;

    // Number of errors during the last read
    uint64_t ecall_sys_open;
    uint64_t ecall_close_fd;
    uint64_t ecall_do_fork;
    uint64_t ecall_sys_clone;
} ebpf_process_publish_apps_t;

enum ebpf_process_tables {
    NETDATA_PROCESS_PID_TABLE,
    NETDATA_PROCESS_GLOBAL_TABLE,
    NETDATA_PROCESS_CTRL_TABLE
};

extern struct config process_config;

#endif /* NETDATA_EBPF_PROCESS_H */
