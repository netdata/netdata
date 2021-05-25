// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_VFS_H
#define NETDATA_EBPF_VFS_H 1

extern void *ebpf_vfs_thread(void *ptr);
extern void ebpf_vfs_create_apps_charts(struct ebpf_module *em, void *ptr);

extern struct config vfs_config;

#endif /* NETDATA_EBPF_VFS_H */
