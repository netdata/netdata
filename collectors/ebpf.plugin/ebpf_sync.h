// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SYNC_H
#define NETDATA_EBPF_SYNC_H 1

#ifdef LIBBPF_MAJOR_VERSION
#include "includes/sync.skel.h"
#endif

// Module name
#define NETDATA_EBPF_MODULE_NAME_SYNC "sync"

// charts
#define NETDATA_EBPF_SYNC_CHART "sync"
#define NETDATA_EBPF_MSYNC_CHART "memory_map"
#define NETDATA_EBPF_FILE_SYNC_CHART "file_sync"
#define NETDATA_EBPF_FILE_SEGMENT_CHART "file_segment"
#define NETDATA_EBPF_SYNC_SUBMENU "synchronization (eBPF)"

#define NETDATA_SYSCALLS_SYNC   "sync"
#define NETDATA_SYSCALLS_SYNCFS "syncfs"
#define NETDATA_SYSCALLS_MSYNC  "msync"
#define NETDATA_SYSCALLS_FSYNC  "fsync"
#define NETDATA_SYSCALLS_FDATASYNC  "fdatasync"
#define NETDATA_SYSCALLS_SYNC_FILE_RANGE  "sync_file_range"

#define NETDATA_EBPF_SYNC_SLEEP_MS 800000ULL

// configuration file
#define NETDATA_SYNC_CONFIG_FILE "sync.conf"
#define NETDATA_SYNC_CONFIG_NAME "syscalls"

typedef enum sync_syscalls_index {
    NETDATA_SYNC_SYNC_IDX,
    NETDATA_SYNC_SYNCFS_IDX,
    NETDATA_SYNC_MSYNC_IDX,
    NETDATA_SYNC_FSYNC_IDX,
    NETDATA_SYNC_FDATASYNC_IDX,
    NETDATA_SYNC_SYNC_FILE_RANGE_IDX,

    NETDATA_SYNC_IDX_END
} sync_syscalls_index_t;

typedef struct ebpf_sync_syscalls {
    char *syscall;
    int enabled;
    uint32_t flags;

    // BTF structure
    struct bpf_object *objects;
    struct bpf_link **probe_links;

    // BPF structure
#ifdef LIBBPF_MAJOR_VERSION
    struct sync_bpf *sync_obj;
#else
    void *sync_obj;
#endif
} ebpf_sync_syscalls_t;

enum netdata_sync_charts {
    NETDATA_SYNC_CALL,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_SYNC_END
};

enum netdata_sync_table {
    NETDATA_SYNC_GLOBAL_TABLE
};

extern void *ebpf_sync_thread(void *ptr);
extern struct config sync_config;
extern netdata_ebpf_targets_t sync_targets[];

#endif /* NETDATA_EBPF_SYNC_H */
