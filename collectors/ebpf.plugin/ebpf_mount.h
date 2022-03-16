// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_MOUNT_H
#define NETDATA_EBPF_MOUNT_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_MOUNT "mount"

#define NETDATA_EBPF_MOUNT_SYSCALL 2

#define NETDATA_LATENCY_MOUNT_SLEEP_MS 700000ULL

#define NETDATA_EBPF_MOUNT_CALLS "call"
#define NETDATA_EBPF_MOUNT_ERRORS "error"
#define NETDATA_EBPF_MOUNT_FAMILY "mount (eBPF)"

// Process configuration name
#define NETDATA_MOUNT_CONFIG_FILE "mount.conf"

enum mount_counters {
    NETDATA_KEY_MOUNT_CALL,
    NETDATA_KEY_UMOUNT_CALL,
    NETDATA_KEY_MOUNT_ERROR,
    NETDATA_KEY_UMOUNT_ERROR,

    NETDATA_MOUNT_END
};

enum mount_tables {
    NETDATA_KEY_MOUNT_TABLE
};

enum netdata_mount_syscalls {
    NETDATA_MOUNT_SYSCALL,
    NETDATA_UMOUNT_SYSCALL,

    NETDATA_MOUNT_SYSCALLS_END
};

extern struct config mount_config;
extern void *ebpf_mount_thread(void *ptr);
extern netdata_ebpf_targets_t mount_targets[];

#endif /* NETDATA_EBPF_MOUNT_H */
