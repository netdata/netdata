// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_VFS_H
#define NETDATA_EBPF_VFS_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_VFS "vfs"
#define NETDATA_EBPF_VFS_MODULE_DESC                                                                                   \
    "Monitor VFS (Virtual File System) functions. This thread is integrated with apps and cgroup."

#define NETDATA_DIRECTORY_VFS_CONFIG_FILE "vfs.conf"

// Global chart name
#define NETDATA_VFS_FILE_CLEAN_COUNT "vfs_deleted_objects"
#define NETDATA_VFS_FILE_IO_COUNT "vfs_io"
#define NETDATA_VFS_FILE_ERR_COUNT "vfs_io_error"
#define NETDATA_VFS_IO_FILE_BYTES "vfs_io_bytes"
#define NETDATA_VFS_FSYNC "vfs_fsync"
#define NETDATA_VFS_FSYNC_ERR "vfs_fsync_error"
#define NETDATA_VFS_OPEN "vfs_open"
#define NETDATA_VFS_OPEN_ERR "vfs_open_error"
#define NETDATA_VFS_CREATE "vfs_create"
#define NETDATA_VFS_CREATE_ERR "vfs_create_error"

// Charts created on Apps submenu
#define NETDATA_SYSCALL_APPS_FILE_DELETED "file_deleted"
#define NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS "vfs_write_call"
#define NETDATA_SYSCALL_APPS_VFS_READ_CALLS "vfs_read_call"
#define NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES "vfs_write_bytes"
#define NETDATA_SYSCALL_APPS_VFS_READ_BYTES "vfs_read_bytes"
#define NETDATA_SYSCALL_APPS_VFS_FSYNC "vfs_fsync"
#define NETDATA_SYSCALL_APPS_VFS_OPEN "vfs_open"
#define NETDATA_SYSCALL_APPS_VFS_CREATE "vfs_create"

#define NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR "vfs_write_error"
#define NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR "vfs_read_error"
#define NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR "vfs_fsync_error"
#define NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR "vfs_open_error"
#define NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR "vfs_create_error"

// Group used on Dashboard
#define NETDATA_VFS_GROUP "vfs"

// Contexts
#define NETDATA_CGROUP_VFS_UNLINK_CONTEXT "cgroup.vfs_unlink"
#define NETDATA_CGROUP_VFS_WRITE_CONTEXT "cgroup.vfs_write"
#define NETDATA_CGROUP_VFS_WRITE_ERROR_CONTEXT "cgroup.vfs_write_error"
#define NETDATA_CGROUP_VFS_READ_CONTEXT "cgroup.vfs_read"
#define NETDATA_CGROUP_VFS_READ_ERROR_CONTEXT "cgroup.vfs_read_error"
#define NETDATA_CGROUP_VFS_WRITE_BYTES_CONTEXT "cgroup.vfs_write_bytes"
#define NETDATA_CGROUP_VFS_READ_BYTES_CONTEXT "cgroup.vfs_read_bytes"
#define NETDATA_CGROUP_VFS_CREATE_CONTEXT "cgroup.vfs_create"
#define NETDATA_CGROUP_VFS_CREATE_ERROR_CONTEXT "cgroup.vfs_create_error"
#define NETDATA_CGROUP_VFS_OPEN_CONTEXT "cgroup.vfs_open"
#define NETDATA_CGROUP_VFS_OPEN_ERROR_CONTEXT "cgroup.vfs_open_error"
#define NETDATA_CGROUP_VFS_FSYNC_CONTEXT "cgroup.vfs_fsync"
#define NETDATA_CGROUP_VFS_FSYNC_ERROR_CONTEXT "cgroup.vfs_fsync_error"

#define NETDATA_SYSTEMD_VFS_UNLINK_CONTEXT "systemd.service.vfs_unlink"
#define NETDATA_SYSTEMD_VFS_WRITE_CONTEXT "systemd.service.vfs_write"
#define NETDATA_SYSTEMD_VFS_WRITE_ERROR_CONTEXT "systemd.service.vfs_write_error"
#define NETDATA_SYSTEMD_VFS_READ_CONTEXT "systemd.service.vfs_read"
#define NETDATA_SYSTEMD_VFS_READ_ERROR_CONTEXT "systemd.service.vfs_read_error"
#define NETDATA_SYSTEMD_VFS_WRITE_BYTES_CONTEXT "systemd.service.vfs_write_bytes"
#define NETDATA_SYSTEMD_VFS_READ_BYTES_CONTEXT "systemd.service.vfs_read_bytes"
#define NETDATA_SYSTEMD_VFS_CREATE_CONTEXT "systemd.service.vfs_create"
#define NETDATA_SYSTEMD_VFS_CREATE_ERROR_CONTEXT "systemd.service.vfs_create_error"
#define NETDATA_SYSTEMD_VFS_OPEN_CONTEXT "systemd.service.vfs_open"
#define NETDATA_SYSTEMD_VFS_OPEN_ERROR_CONTEXT "systemd.service.vfs_open_error"
#define NETDATA_SYSTEMD_VFS_FSYNC_CONTEXT "systemd.service.vfs_fsync"
#define NETDATA_SYSTEMD_VFS_FSYNC_ERROR_CONTEXT "systemd.service.vfs_fsync_error"

// ARAL name
#define NETDATA_EBPF_VFS_ARAL_NAME "ebpf_vfs"

// dimension
#define EBPF_COMMON_UNITS_BYTES "bytes/s"

typedef struct __attribute__((packed)) netdata_publish_vfs {
    uint64_t ct;

    //Counter
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t fsync_call;
    uint32_t open_call;
    uint32_t create_call;

    //Accumulator
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;

    //Counter
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fsync_err;
    uint32_t open_err;
    uint32_t create_err;

} netdata_publish_vfs_t;

enum netdata_publish_vfs_list {
    NETDATA_KEY_PUBLISH_VFS_UNLINK,
    NETDATA_KEY_PUBLISH_VFS_READ,
    NETDATA_KEY_PUBLISH_VFS_WRITE,
    NETDATA_KEY_PUBLISH_VFS_FSYNC,
    NETDATA_KEY_PUBLISH_VFS_OPEN,
    NETDATA_KEY_PUBLISH_VFS_CREATE,

    NETDATA_KEY_PUBLISH_VFS_END
};

enum vfs_counters {
    NETDATA_KEY_CALLS_VFS_WRITE,
    NETDATA_KEY_ERROR_VFS_WRITE,
    NETDATA_KEY_BYTES_VFS_WRITE,

    NETDATA_KEY_CALLS_VFS_WRITEV,
    NETDATA_KEY_ERROR_VFS_WRITEV,
    NETDATA_KEY_BYTES_VFS_WRITEV,

    NETDATA_KEY_CALLS_VFS_READ,
    NETDATA_KEY_ERROR_VFS_READ,
    NETDATA_KEY_BYTES_VFS_READ,

    NETDATA_KEY_CALLS_VFS_READV,
    NETDATA_KEY_ERROR_VFS_READV,
    NETDATA_KEY_BYTES_VFS_READV,

    NETDATA_KEY_CALLS_VFS_UNLINK,
    NETDATA_KEY_ERROR_VFS_UNLINK,

    NETDATA_KEY_CALLS_VFS_FSYNC,
    NETDATA_KEY_ERROR_VFS_FSYNC,

    NETDATA_KEY_CALLS_VFS_OPEN,
    NETDATA_KEY_ERROR_VFS_OPEN,

    NETDATA_KEY_CALLS_VFS_CREATE,
    NETDATA_KEY_ERROR_VFS_CREATE,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_VFS_COUNTER
};

enum netdata_vfs_tables { NETDATA_VFS_PID, NETDATA_VFS_ALL, NETDATA_VFS_CTRL };

enum netdata_vfs_calls_name {
    NETDATA_EBPF_VFS_WRITE,
    NETDATA_EBPF_VFS_WRITEV,
    NETDATA_EBPF_VFS_READ,
    NETDATA_EBPF_VFS_READV,
    NETDATA_EBPF_VFS_UNLINK,
    NETDATA_EBPF_VFS_FSYNC,
    NETDATA_EBPF_VFS_OPEN,
    NETDATA_EBPF_VFS_CREATE,

    NETDATA_VFS_END_LIST
};

void *ebpf_vfs_thread(void *ptr);
void ebpf_vfs_create_apps_charts(struct ebpf_module *em, void *ptr);
void ebpf_vfs_release(netdata_publish_vfs_t *stat);
extern netdata_ebpf_targets_t vfs_targets[];

extern struct config vfs_config;

#endif /* NETDATA_EBPF_VFS_H */
