// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_MOUNT_H
#define NETDATA_EBPF_MOUNT_H 1

enum mount_counters {
    NETDATA_KEY_MOUNT_CALL,
    NETDATA_KEY_UMOUNT_CALL,
    NETDATA_KEY_MOUNT_ERROR,
    NETDATA_KEY_UMOUNT_ERROR,

    NETDATA_MOUNT_END
};

extern struct config mount_config;
extern void *ebpf_mount_thread(void *ptr);

#endif /* NETDATA_EBPF_MOUNT_H */
