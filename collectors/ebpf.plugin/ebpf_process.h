// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_PROCESS_H
#define NETDATA_EBPF_PROCESS_H 1

// Groups used on Dashboard
#define NETDATA_FILE_GROUP "File"
#define NETDATA_VFS_GROUP "VFS"
#define NETDATA_PROCESS_GROUP "Process"

// Internal constants
#define NETDATA_GLOBAL_VECTOR 24
#define NETDATA_MAX_MONITOR_VECTOR 9
#define NETDATA_VFS_ERRORS 3

// Map index
#define NETDATA_DEL_START 2
#define NETDATA_IN_START_BYTE 3
#define NETDATA_EXIT_START 5
#define NETDATA_PROCESS_START 7

// Global chart name
#define NETDATA_FILE_OPEN_CLOSE_COUNT "file_descriptor"
#define NETDATA_FILE_OPEN_ERR_COUNT "file_error"
#define NETDATA_VFS_FILE_CLEAN_COUNT "deleted_objects"
#define NETDATA_VFS_FILE_IO_COUNT "io"
#define NETDATA_VFS_FILE_ERR_COUNT "io_error"

#define NETDATA_EXIT_SYSCALL "exit"
#define NETDATA_PROCESS_SYSCALL "process_thread"
#define NETDATA_PROCESS_ERROR_NAME "task_error"
#define NETDATA_PROCESS_STATUS_NAME "process_status"

#define NETDATA_VFS_IO_FILE_BYTES "io_bytes"
#define NETDATA_VFS_DIM_IN_FILE_BYTES "write"
#define NETDATA_VFS_DIM_OUT_FILE_BYTES "read"

// Charts created on Apps submenu
#define NETDATA_SYSCALL_APPS_FILE_OPEN "file_open"
#define NETDATA_SYSCALL_APPS_FILE_CLOSED "file_closed"
#define NETDATA_SYSCALL_APPS_FILE_DELETED "file_deleted"
#define NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS "vfs_write_call"
#define NETDATA_SYSCALL_APPS_VFS_READ_CALLS "vfs_read_call"
#define NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES "vfs_write_bytes"
#define NETDATA_SYSCALL_APPS_VFS_READ_BYTES "vfs_read_bytes"
#define NETDATA_SYSCALL_APPS_TASK_PROCESS "process_create"
#define NETDATA_SYSCALL_APPS_TASK_THREAD "thread_create"
#define NETDATA_SYSCALL_APPS_TASK_CLOSE "task_close"

// Charts created on Apps submenu, if and only if, the return mode is active

#define NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR "file_open_error"
#define NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR "file_close_error"
#define NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR "vfs_write_error"
#define NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR "vfs_read_error"

// Index from kernel
typedef enum ebpf_process_index {
    NETDATA_KEY_CALLS_DO_SYS_OPEN,
    NETDATA_KEY_ERROR_DO_SYS_OPEN,

    NETDATA_KEY_CALLS_VFS_WRITE,
    NETDATA_KEY_ERROR_VFS_WRITE,
    NETDATA_KEY_BYTES_VFS_WRITE,

    NETDATA_KEY_CALLS_VFS_READ,
    NETDATA_KEY_ERROR_VFS_READ,
    NETDATA_KEY_BYTES_VFS_READ,

    NETDATA_KEY_CALLS_VFS_UNLINK,
    NETDATA_KEY_ERROR_VFS_UNLINK,

    NETDATA_KEY_CALLS_DO_EXIT,

    NETDATA_KEY_CALLS_RELEASE_TASK,

    NETDATA_KEY_CALLS_DO_FORK,
    NETDATA_KEY_ERROR_DO_FORK,

    NETDATA_KEY_CALLS_CLOSE_FD,
    NETDATA_KEY_ERROR_CLOSE_FD,

    NETDATA_KEY_CALLS_SYS_CLONE,
    NETDATA_KEY_ERROR_SYS_CLONE,

    NETDATA_KEY_CALLS_VFS_WRITEV,
    NETDATA_KEY_ERROR_VFS_WRITEV,
    NETDATA_KEY_BYTES_VFS_WRITEV,

    NETDATA_KEY_CALLS_VFS_READV,
    NETDATA_KEY_ERROR_VFS_READV,
    NETDATA_KEY_BYTES_VFS_READV

} ebpf_process_index_t;

typedef struct ebpf_process_publish_apps {
    // Number of calls during the last read
    uint64_t call_sys_open;
    uint64_t call_close_fd;
    uint64_t call_vfs_unlink;
    uint64_t call_read;
    uint64_t call_write;
    uint64_t call_do_exit;
    uint64_t call_release_task;
    uint64_t call_do_fork;
    uint64_t call_sys_clone;

    // Number of errors during the last read
    uint64_t ecall_sys_open;
    uint64_t ecall_close_fd;
    uint64_t ecall_vfs_unlink;
    uint64_t ecall_read;
    uint64_t ecall_write;
    uint64_t ecall_do_fork;
    uint64_t ecall_sys_clone;

    // Number of bytes during the last read
    uint64_t bytes_written;
    uint64_t bytes_read;

    // Dimensions sent to chart
    uint64_t publish_open;
    uint64_t publish_closed;
    uint64_t publish_deleted;
    uint64_t publish_write_call;
    uint64_t publish_write_bytes;
    uint64_t publish_read_call;
    uint64_t publish_read_bytes;
    uint64_t publish_process;
    uint64_t publish_thread;
    uint64_t publish_task;
    uint64_t publish_open_error;
    uint64_t publish_close_error;
    uint64_t publish_write_error;
    uint64_t publish_read_error;
} ebpf_process_publish_apps_t;

#endif /* NETDATA_EBPF_PROCESS_H */
