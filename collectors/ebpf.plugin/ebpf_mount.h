// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_MOUNT_H
#define NETDATA_EBPF_MOUNT_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_MOUNT "mount"

#define NETDATA_EBPF_MOUNT_SYSCALL 2

#define NETDATA_LATENCY_MOUNT_SLEEP_MS 700000ULL

#define NETDATA_EBPF_MOUNT_CALLS "ebpf_calls"
#define NETDATA_EBPF_MOUNT_ERRORS "ebpf_errors"
#define NETDATA_EBPF_MOUNT_FAMILY "mount calls (eBPF)"

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

extern struct config mount_config;
extern void *ebpf_mount_thread(void *ptr);

#endif /* NETDATA_EBPF_MOUNT_H */
