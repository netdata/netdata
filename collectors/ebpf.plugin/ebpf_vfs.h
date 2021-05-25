// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_VFS_H
#define NETDATA_EBPF_VFS_H 1

#define NETDATA_DIRECTORY_VFS_CONFIG_FILE "vfs.conf"

typedef struct netdata_publish_vfs {
    uint64_t pid_tgid;
    uint32_t pid;
    uint32_t pad;

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

extern netdata_publish_vfs_t **vfs_pid;

extern void *ebpf_vfs_thread(void *ptr);
extern void ebpf_vfs_create_apps_charts(struct ebpf_module *em, void *ptr);

extern struct config vfs_config;

#endif /* NETDATA_EBPF_VFS_H */
