// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FD_H
#define NETDATA_EBPF_FD_H 1

// Module name & File description
#define NETDATA_EBPF_MODULE_NAME_FD "filedescriptor"
#define NETDATA_EBPF_FD_MODULE_DESC                                                                                    \
    "Monitor when files are open and closed. This thread is integrated with apps and cgroup."

// Menu group
#define NETDATA_FILE_GROUP "file_access"

// Global chart name
#define NETDATA_FILE_OPEN_CLOSE_COUNT "file_descriptor"
#define NETDATA_FILE_OPEN_ERR_COUNT "file_error"

// Charts created on Apps submenu
#define NETDATA_SYSCALL_APPS_FILE_OPEN "file_open"
#define NETDATA_SYSCALL_APPS_FILE_CLOSED "file_closed"
#define NETDATA_SYSCALL_APPS_FILE_OPEN_ERROR "file_open_error"
#define NETDATA_SYSCALL_APPS_FILE_CLOSE_ERROR "file_close_error"

// Process configuration name
#define NETDATA_FD_CONFIG_FILE "fd.conf"

// Contexts
#define NETDATA_FS_FILEDESCRIPTOR_CONTEXT "filesystem.file_descriptor"
#define NETDATA_FS_FILEDESCRIPTOR_ERROR_CONTEXT "filesystem.file_error"

#define NETDATA_CGROUP_FD_OPEN_CONTEXT "cgroup.fd_open"
#define NETDATA_CGROUP_FD_OPEN_ERR_CONTEXT "cgroup.fd_open_error"
#define NETDATA_CGROUP_FD_CLOSE_CONTEXT "cgroup.fd_close"
#define NETDATA_CGROUP_FD_CLOSE_ERR_CONTEXT "cgroup.fd_close_error"

#define NETDATA_SYSTEMD_FD_OPEN_CONTEXT "systemd.service.fd_open"
#define NETDATA_SYSTEMD_FD_OPEN_ERR_CONTEXT "systemd.service.fd_open_error"
#define NETDATA_SYSTEMD_FD_CLOSE_CONTEXT "systemd.service.fd_close"
#define NETDATA_SYSTEMD_FD_CLOSE_ERR_CONTEXT "systemd.service.fd_close_error"

// ARAL name
#define NETDATA_EBPF_FD_ARAL_NAME "ebpf_fd"

enum fd_tables {
    NETDATA_FD_PID_STATS,
    NETDATA_FD_GLOBAL_STATS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_FD_CONTROLLER
};

enum fd_counters {
    NETDATA_KEY_CALLS_DO_SYS_OPEN,
    NETDATA_KEY_ERROR_DO_SYS_OPEN,

    NETDATA_KEY_CALLS_CLOSE_FD,
    NETDATA_KEY_ERROR_CLOSE_FD,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_FD_COUNTER
};

enum fd_syscalls {
    NETDATA_FD_SYSCALL_OPEN,
    NETDATA_FD_SYSCALL_CLOSE,

    // Do not insert nothing after this value
    NETDATA_FD_SYSCALL_END
};

enum fd_close_syscall {
    NETDATA_FD_CLOSE_FD,
    NETDATA_FD___CLOSE_FD,

    NETDATA_FD_CLOSE_END
};

#define NETDATA_EBPF_MAX_FD_TARGETS 2

void ebpf_fd_thread(void *ptr);
void ebpf_fd_create_apps_charts(struct ebpf_module *em, void *ptr);
void ebpf_fd_release(netdata_fd_stat_t *stat);
extern struct config fd_config;
extern netdata_ebpf_targets_t fd_targets[];

#endif /* NETDATA_EBPF_FD_H */
