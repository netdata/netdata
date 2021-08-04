// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FD_H
#define NETDATA_EBPF_FD_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_FD "fd"

extern void *ebpf_fd_thread(void *ptr);
extern void ebpf_fd_create_apps_charts(struct ebpf_module *em, void *ptr);
extern struct config fd_config;

enum fd_syscalls {
    NETDATA_FD_SYSCALL_OPEN,
    NETDATA_FD_SYSCALL_CLOSE,

    // Do not insert nothing after this value
    NETDATA_FD_SYSCALL_END
};

#endif /* NETDATA_EBPF_FD_H */
