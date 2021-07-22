// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_MOUNT_H
#define NETDATA_EBPF_MOUNT_H 1

#define NETDATA_EBPF_MOUNT_SYSCALL 2

#define NETDATA_LATENCY_MOUNT_SLEEP_MS 700000ULL

#define NETDATA_EBPF_MOUNT_CALLS "call"
#define NETDATA_EBPF_MOUNT_ERRORS "error"
#define NETDATA_EBPF_MOUNT_FAMILY "mount (eBPF)"

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
