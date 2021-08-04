// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FD_H
#define NETDATA_EBPF_FD_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_FD "filedescriptor"

#define NETDATA_FD_SLEEP_MS 850000ULL

// Menu group
#define NETDATA_FILE_GROUP "File (eBPF)"

// Global chart name
#define NETDATA_FILE_OPEN_CLOSE_COUNT "file_descriptor"
#define NETDATA_FILE_OPEN_ERR_COUNT "file_error"

extern void *ebpf_fd_thread(void *ptr);
extern void ebpf_fd_create_apps_charts(struct ebpf_module *em, void *ptr);
extern struct config fd_config;

enum fd_tables {
    NETDATA_FD_PID_STATS,
    NETDATA_FD_GLOBAL_STATS,

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

#endif /* NETDATA_EBPF_FD_H */
