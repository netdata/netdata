// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FILESYSTEM_H
#define NETDATA_EBPF_FILESYSTEM_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_FILESYSTEM "filesystem"

#include "ebpf.h"

#define NETDATA_FS_MAX_DIST_NAME 64UL

#define NETDATA_FILESYSTEM_CONFIG_NAME "filesystem"
#define NETDATA_FILESYSTEM_READ_SLEEP_MS 600000ULL

// Process configuration name
#define NETDATA_FILESYSTEM_CONFIG_FILE "filesystem.conf"

typedef struct netdata_fs_hist {
    uint32_t hist_id;
    uint32_t bin;
} netdata_fs_hist_t;

enum filesystem_limit {
    NETDATA_KEY_CALLS_READ = 24,
    NETDATA_KEY_CALLS_WRITE = 48,
    NETDATA_KEY_CALLS_OPEN = 72,
    NETDATA_KEY_CALLS_SYNC = 96
};

enum netdata_filesystem_flags {
    NETDATA_FILESYSTEM_FLAG_NO_PARTITION = 0,
    NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM = 1,
    NETDATA_FILESYSTEM_FLAG_HAS_PARTITION = 2,
    NETDATA_FILESYSTEM_FLAG_CHART_CREATED = 4,
    NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE = 8,
    NETDATA_FILESYSTEM_REMOVE_CHARTS = 16,
    NETDATA_FILESYSTEM_ATTR_CHARTS = 32
};

enum netdata_filesystem_table {
    NETDATA_MAIN_FS_TABLE,
    NETDATA_ADDR_FS_TABLE
};

typedef struct ebpf_filesystem_partitions {
    char *filesystem;
    char *optional_filesystem;
    char *family;
    char *family_name;
    struct bpf_object *objects;
    struct bpf_link **probe_links;

    netdata_ebpf_histogram_t hread;
    netdata_ebpf_histogram_t hwrite;
    netdata_ebpf_histogram_t hopen;
    netdata_ebpf_histogram_t hadditional;

    uint32_t flags;
    uint32_t enabled;

    ebpf_addresses_t addresses;
    uint64_t kernels;
} ebpf_filesystem_partitions_t;

extern void *ebpf_filesystem_thread(void *ptr);
extern struct config fs_config;

#endif /* NETDATA_EBPF_FILESYSTEM_H */
