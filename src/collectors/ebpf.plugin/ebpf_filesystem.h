// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FILESYSTEM_H
#define NETDATA_EBPF_FILESYSTEM_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_FILESYSTEM "filesystem"
#define NETDATA_EBPF_FS_MODULE_DESC "Monitor filesystem latency for: btrfs, ext4, nfs, xfs and zfs."

#include "libnetdata/libnetdata.h"

// Forward declaration to avoid circular dependency
struct ebpf_module;

// Constants
#define NETDATA_FS_MAX_DIST_NAME 64UL
#define NETDATA_FS_TEMP_MAP_SIZE 4192
#define NETDATA_FS_HISTOGRAM_BINS 24
#define NETDATA_PARTITION_UPDATE_INTERVAL_MULTIPLIER 5

// Configuration section and file names
#define NETDATA_FILESYSTEM_CONFIG_NAME "filesystem"
#define NETDATA_FILESYSTEM_CONFIG_FILE "filesystem.conf"

enum filesystem_limit {
    NETDATA_KEY_CALLS_READ = 24,
    NETDATA_KEY_CALLS_WRITE = 48,
    NETDATA_KEY_CALLS_OPEN = 72,
    NETDATA_KEY_CALLS_SYNC = 96
};

enum netdata_filesystem_flags {
    // Flags indicating filesystem module state and operations
    NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
    NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM = 1,
    NETDATA_FILESYSTEM_FLAG_HAS_PARTITION = 2,
    NETDATA_FILESYSTEM_FLAG_CHART_CREATED = 4,
    NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE = 8,
    NETDATA_FILESYSTEM_REMOVE_CHARTS = 16,
    NETDATA_FILESYSTEM_ATTR_CHARTS = 32
};

enum netdata_filesystem_table { NETDATA_MAIN_FS_TABLE, NETDATA_ADDR_FS_TABLE };

enum netdata_filesystem_localfs_idx {
    NETDATA_FS_LOCALFS_EXT4,
    NETDATA_FS_LOCALFS_XFS,
    NETDATA_FS_LOCALFS_NFS,
    NETDATA_FS_LOCALFS_ZFS,
    NETDATA_FS_LOCALFS_BTRFS,

    NETDATA_FS_LOCALFS_END,
};

/**
 * Filesystem eBPF collector thread
 *
 * Main thread function that monitors filesystem operations (btrfs, ext4, nfs, xfs, zfs)
 * and collects latency metrics using eBPF.
 *
 * @param ptr Pointer to module data (struct ebpf_module *)
 */
void ebpf_filesystem_thread(void *ptr);

/**
 * Initialize eBPF data
 *
 * @param em Main thread structure
 *
 * @return 0 on success, -1 on error
 */
int ebpf_filesystem_initialize_ebpf_data(struct ebpf_module *em);

/**
 * Cleanup eBPF data
 *
 * Frees allocated resources and cleans up filesystem partitions
 */
void ebpf_filesystem_cleanup_ebpf_data();

/**
 * Read filesystem hash
 *
 * Reads histogram data from filesystem eBPF maps
 *
 * @param em Pointer to module structure
 */
void ebpf_filesystem_read_hash(struct ebpf_module *em);

/**
 * Filesystem module configuration
 *
 * Stores configuration parameters for the filesystem eBPF collector
 */
extern struct config fs_config;

#endif /* NETDATA_EBPF_FILESYSTEM_H */
